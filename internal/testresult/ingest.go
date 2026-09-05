package testresult

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ralabarta/agentproof/internal/evidence"
	"github.com/ralabarta/agentproof/internal/safefile"
)

const (
	maxArtifactBytes = 32 * 1024 * 1024
	maxTotalBytes    = 256 * 1024 * 1024
	maxRecords       = 100_000
)

func Ingest(root string, declared []string, requireTests bool) (evidence.TestResult, []evidence.Record) {
	declared = unique(declared)
	if len(declared) == 0 {
		state := evidence.NotObserved
		if requireTests {
			state = evidence.Missing
		}
		return evidence.TestResult{Summary: "no test result artifact supplied"}, []evidence.Record{{
			Locator: "test-results", State: state, Required: requireTests,
			Reason: "no test result artifact was declared", Confidence: evidence.Confidence{Score: 100, Reasons: []string{"policy-census"}},
		}}
	}

	result := evidence.TestResult{Ingested: true, Passed: true}
	records := make([]evidence.Record, 0, len(declared))
	var totalBytes int64
	for _, declaredPath := range declared {
		artifact, size := ingestOne(root, declaredPath, maxTotalBytes-totalBytes)
		totalBytes += size
		result.Artifacts = append(result.Artifacts, artifact)
		result.PassedTests += artifact.PassedTests
		result.FailedTests += artifact.FailedTests
		result.SkippedTests += artifact.SkippedTests
		result.DurationMS += artifact.DurationMS
		if artifact.State != evidence.Observed || artifact.FailedTests > 0 {
			result.Passed = false
		}
		confidence := evidence.Confidence{Score: 0, Reasons: []string{"artifact-unavailable"}}
		if artifact.State == evidence.Observed {
			confidence = evidence.Confidence{Score: 100, Reasons: []string{"bounded-parser", artifact.Format}}
		}
		records = append(records, evidence.Record{
			Locator: "test-results/" + filepath.ToSlash(artifact.Path), State: artifact.State,
			Required: true, Discovered: artifact.State != evidence.Missing, Digest: artifact.Digest,
			Reason: artifact.Reason, Confidence: confidence,
		})
	}
	if result.Passed {
		result.Summary = fmt.Sprintf("ingested: %d passed, %d failed, %d skipped", result.PassedTests, result.FailedTests, result.SkippedTests)
	} else {
		result.Summary = fmt.Sprintf("not passing: %d passed, %d failed, %d skipped", result.PassedTests, result.FailedTests, result.SkippedTests)
	}
	return result, records
}

func ingestOne(root, declared string, remaining int64) (evidence.TestArtifact, int64) {
	artifact := evidence.TestArtifact{Path: filepath.ToSlash(filepath.Clean(declared)), State: evidence.Unknown}
	full, rel, err := securePath(root, declared)
	if err != nil {
		artifact.Reason = err.Error()
		return artifact, 0
	}
	artifact.Path = filepath.ToSlash(rel)
	info, err := os.Lstat(full)
	if errors.Is(err, os.ErrNotExist) {
		artifact.State = evidence.Missing
		artifact.Reason = "declared test result does not exist"
		return artifact, 0
	}
	if err != nil {
		artifact.Reason = "cannot inspect declared test result"
		return artifact, 0
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		artifact.Reason = "test result must be a regular file and not a symlink"
		return artifact, 0
	}
	if info.Size() > maxArtifactBytes || info.Size() > remaining {
		artifact.Reason = "test result exceeds configured byte limit"
		return artifact, info.Size()
	}
	b, err := os.ReadFile(full)
	if err != nil {
		artifact.Reason = "cannot read declared test result"
		return artifact, info.Size()
	}
	artifact.Digest = digest(b)
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 {
		artifact.Reason = "test result is empty"
		return artifact, info.Size()
	}
	if trimmed[0] == '<' {
		artifact.Format = "junit-xml"
		artifact.Producer = "junit"
		artifact.FormatVersion = "generic"
		err = parseJUnit(trimmed, &artifact)
	} else {
		artifact.Format = "go-test2json"
		artifact.Producer = "go test -json"
		artifact.FormatVersion = "go1"
		err = parseGoTestJSON(trimmed, &artifact)
	}
	if err != nil {
		artifact.Reason = err.Error()
		artifact.State = evidence.Unknown
		return artifact, info.Size()
	}
	artifact.State = evidence.Observed
	return artifact, info.Size()
}

func parseGoTestJSON(b []byte, artifact *evidence.TestArtifact) error {
	scanner := bufio.NewScanner(bytes.NewReader(b))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	type terminalKey struct {
		packageName string
		testName    string
	}
	terminalActions := map[terminalKey]string{}
	count := 0
	eventSeen := false
	terminalSeen := false
	packageFailed := false
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		count++
		if count > maxRecords {
			return errors.New("test result exceeds record limit")
		}
		var event struct {
			Action  string  `json:"Action"`
			Package string  `json:"Package"`
			Test    string  `json:"Test"`
			Elapsed float64 `json:"Elapsed"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return errors.New("malformed Go test2json event")
		}
		switch event.Action {
		case "start", "run", "pause", "cont", "pass", "bench", "fail", "output", "skip":
			eventSeen = true
		}
		if event.Action == "pass" || event.Action == "fail" || event.Action == "skip" || event.Action == "bench" {
			terminalSeen = true
			key := terminalKey{packageName: event.Package, testName: event.Test}
			if state, ok := terminalActions[key]; ok && state != event.Action {
				return errors.New("Go test2json contains contradictory terminal outcomes")
			}
			terminalActions[key] = event.Action
		}
		if event.Test == "" {
			if event.Action == "fail" {
				packageFailed = true
			}
			continue
		}
		switch event.Action {
		case "pass", "fail", "skip":
			artifact.DurationMS += int64(event.Elapsed * 1000)
		}
	}
	if err := scanner.Err(); err != nil {
		return errors.New("Go test2json event exceeds line limit")
	}
	if !eventSeen {
		return errors.New("test result contains no events")
	}
	if !terminalSeen {
		return errors.New("test result contains no terminal outcome")
	}
	for key, state := range terminalActions {
		if key.testName == "" {
			continue
		}
		switch state {
		case "pass":
			artifact.PassedTests++
		case "fail":
			artifact.FailedTests++
		case "skip":
			artifact.SkippedTests++
		}
	}
	if packageFailed && artifact.FailedTests == 0 {
		artifact.FailedTests = 1
	}
	return nil
}

func parseJUnit(b []byte, artifact *evidence.TestArtifact) error {
	decoder := xml.NewDecoder(bytes.NewReader(b))
	count := 0
	suiteSeen := false
	testCaseSeen := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return errors.New("malformed JUnit XML")
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		count++
		if count > maxRecords {
			return errors.New("JUnit XML exceeds element limit")
		}
		if count == 1 {
			if start.Name.Local != "testsuite" && start.Name.Local != "testsuites" {
				return errors.New("XML is not a JUnit test suite")
			}
		}
		if start.Name.Local == "testsuite" {
			suiteSeen = true
		}
		if start.Name.Local != "testcase" {
			continue
		}
		testCaseSeen = true
		var testCase struct {
			Time    string    `xml:"time,attr"`
			Failure *struct{} `xml:"failure"`
			Error   *struct{} `xml:"error"`
			Skipped *struct{} `xml:"skipped"`
		}
		if err := decoder.DecodeElement(&testCase, &start); err != nil {
			return errors.New("malformed JUnit testcase")
		}
		if seconds, parseErr := strconv.ParseFloat(testCase.Time, 64); parseErr == nil {
			if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
				return errors.New("invalid JUnit testcase duration")
			}
			milliseconds := seconds * 1000
			if milliseconds >= float64(math.MaxInt64) {
				return errors.New("invalid JUnit testcase duration")
			}
			durationMS := int64(milliseconds)
			if artifact.DurationMS > math.MaxInt64-durationMS {
				return errors.New("invalid JUnit testcase duration")
			}
			artifact.DurationMS += durationMS
		}
		switch {
		case testCase.Failure != nil || testCase.Error != nil:
			artifact.FailedTests++
		case testCase.Skipped != nil:
			artifact.SkippedTests++
		default:
			artifact.PassedTests++
		}
	}
	if !suiteSeen {
		return errors.New("XML is not a JUnit test suite")
	}
	if !testCaseSeen {
		return errors.New("JUnit test suite contains no testcases")
	}
	return nil
}

func securePath(root, declared string) (string, string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", "", errors.New("cannot resolve repository root")
	}
	full := declared
	if !filepath.IsAbs(full) {
		full = filepath.Join(rootAbs, full)
	}
	full, err = filepath.Abs(full)
	if err != nil {
		return "", "", errors.New("cannot resolve test result path")
	}
	rel, err := filepath.Rel(rootAbs, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", errors.New("test result path escapes repository root")
	}
	if err := safefile.Contained(rootAbs, full); err != nil {
		return "", "", errors.New("test result path escapes repository root")
	}
	return full, rel, nil
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func unique(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
