package verify

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ralabarta/agentproof/internal/apperr"
	"github.com/ralabarta/agentproof/internal/config"
	"github.com/ralabarta/agentproof/internal/evidence"
	"github.com/ralabarta/agentproof/internal/gitx"
	"github.com/ralabarta/agentproof/internal/impact"
	"github.com/ralabarta/agentproof/internal/report"
	"github.com/ralabarta/agentproof/internal/safefile"
	"github.com/ralabarta/agentproof/internal/scan"
	"github.com/ralabarta/agentproof/internal/testresult"
)

type Options struct {
	Base         string
	TestResults  []string
	RequireTests bool
	FailOn       string
}

type Result struct {
	Run      evidence.Run
	BundleID string
	ExitCode int
}

func Run(cwd string, opts Options) (Result, error) {
	root, err := gitx.Root(cwd)
	if err != nil {
		return Result{}, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return Result{}, fmt.Errorf("%w: AgentProof is not initialized; run agentproof init", apperr.ErrUsage)
	}
	if opts.FailOn == "" {
		opts.FailOn = cfg.FailOn
	}
	if len(opts.TestResults) == 0 {
		opts.TestResults = cfg.TestResults
	}
	if cfg.RequireTests {
		opts.RequireTests = true
	}

	run, patch, err := loadRun(root, opts.Base)
	if err != nil {
		return Result{}, err
	}
	run.Findings = mergeFindings(run.Findings, scan.Run(run.Repository.Changes, patch))
	run.Impact = impact.Analyze(root, run.Repository.Changes)
	tests, testRecords := testresult.Ingest(root, opts.TestResults, opts.RequireTests)
	run.Tests = tests

	manifest := evidence.NewManifest(manifestRecords(run, patch, testRecords))
	bundleID, err := manifest.Identity()
	if err != nil {
		return Result{}, fmt.Errorf("build evidence manifest: %w", err)
	}
	manifestBytes, err := manifest.FinalBytes()
	if err != nil {
		return Result{}, fmt.Errorf("serialize evidence manifest: %w", err)
	}
	run.BundleID = bundleID
	run.Integrity = integrityOf(manifestBytes)
	run.Completeness = manifest.Completeness()
	if run.Model == "" && len(run.Sessions) > 0 && len(run.Sessions[0].Models) > 0 {
		run.Model = run.Sessions[0].Models[0]
	}
	run.Status = status(run)
	run.WarningCount = warningCount(run)
	run.Claims = append(run.Claims,
		evidence.Claim{Type: "security-scan", Statement: "Added lines were checked with deterministic secret and danger rules.", Confidence: evidence.ConfidenceDerived, Evidence: "redacted changes.patch"},
		evidence.Claim{Type: "impact-analysis", Statement: "Affected components are bounded syntactic observations, not runtime reachability.", Confidence: evidence.ConfidenceDerived, Evidence: "repository source graph"},
	)

	outputDir := filepath.Join(root, config.DirName)
	if err := safefile.Write(filepath.Join(outputDir, "manifest.json"), append(manifestBytes, '\n'), 0o600); err != nil {
		return Result{}, err
	}
	evidenceBytes, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return Result{}, err
	}
	evidenceBytes = append(evidenceBytes, '\n')
	if err := safefile.Write(filepath.Join(outputDir, "evidence.json"), evidenceBytes, 0o600); err != nil {
		return Result{}, err
	}
	manifestFileHash := sha256.Sum256(append(manifestBytes, '\n'))
	attestation := evidence.Attestation{
		SchemaVersion: "agentproof.dev/attestation/v1", Algorithm: "sha256",
		EvidenceFile: ".agentproof/manifest.json", EvidenceHash: hex.EncodeToString(manifestFileHash[:]),
		BundleID: bundleID, CreatedAt: time.Now().UTC(),
	}
	if err := writeJSON(filepath.Join(outputDir, "attestation.json"), attestation); err != nil {
		return Result{}, err
	}
	if err := safefile.Write(filepath.Join(outputDir, "report.md"), report.Markdown(run, bundleID), 0o600); err != nil {
		return Result{}, err
	}
	html, err := report.HTML(run, bundleID)
	if err != nil {
		return Result{}, err
	}
	if err := safefile.Write(filepath.Join(outputDir, "report.html"), html, 0o600); err != nil {
		return Result{}, err
	}
	exitCode := 0
	if run.Status == "failed" || scan.MeetsThreshold(run.Findings, opts.FailOn) {
		exitCode = 1
	}
	return Result{Run: run, BundleID: bundleID, ExitCode: exitCode}, nil
}

func loadRun(root, base string) (evidence.Run, string, error) {
	if base != "" {
		repo, patch, err := gitx.CompareBase(root, base)
		if err != nil {
			return evidence.Run{}, "", err
		}
		now := time.Now().UTC()
		return evidence.Run{
			SchemaVersion: evidence.RunSchemaVersion, RunID: "git-" + now.Format("20060102T150405Z"),
			Objective: "Verify changes associated with Git baseline " + base, Agent: "unknown",
			StartedAt: now, FinishedAt: now, Repository: repo,
			Claims: []evidence.Claim{{Type: "git-association", Statement: "Changes are associated with a Git range; agent authorship is unknown.", Confidence: evidence.ConfidenceObserved, Evidence: base + "...HEAD"}},
		}, patch, nil
	}
	latestBytes, err := os.ReadFile(filepath.Join(root, config.DirName, "latest.json"))
	if err != nil {
		return evidence.Run{}, "", fmt.Errorf("%w: no recorded session found; run agentproof record or pass --base", apperr.ErrUsage)
	}
	var latest map[string]string
	if json.Unmarshal(latestBytes, &latest) != nil || latest["record"] == "" {
		return evidence.Run{}, "", errors.New("invalid .agentproof/latest.json")
	}
	recordPath, err := containedPath(filepath.Join(root, config.DirName), latest["record"])
	if err != nil {
		return evidence.Run{}, "", err
	}
	recordBytes, err := os.ReadFile(recordPath)
	if err != nil {
		return evidence.Run{}, "", err
	}
	var run evidence.Run
	if err := json.Unmarshal(recordBytes, &run); err != nil {
		return evidence.Run{}, "", err
	}
	patchPath := filepath.Join(filepath.Dir(recordPath), "changes.patch")
	patchBytes, err := os.ReadFile(patchPath)
	if err != nil {
		return evidence.Run{}, "", errors.New("recorded Git patch is missing")
	}
	return run, string(patchBytes), nil
}

// latest.json is repository content, so its record locator is untrusted input:
// resolve it and confirm it stays inside the evidence directory.
func containedPath(dir, declared string) (string, error) {
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return "", errors.New("cannot resolve evidence directory")
	}
	full, err := filepath.Abs(filepath.Join(dirAbs, filepath.FromSlash(declared)))
	if err != nil {
		return "", errors.New("cannot resolve recorded run path")
	}
	rel, err := filepath.Rel(dirAbs, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("recorded run path escapes the evidence directory")
	}
	if err := safefile.Contained(dirAbs, full); err != nil {
		return "", errors.New("recorded run path escapes the evidence directory")
	}
	return full, nil
}

func manifestRecords(run evidence.Run, patch string, testRecords []evidence.Record) []evidence.Record {
	// An empty patch still hashes to a valid sha256, so the state must be
	// derived from whether content was actually captured — never asserted.
	patchRecord := evidence.Record{
		Locator: "git/changes.patch", State: evidence.Observed, Required: true, Discovered: true,
		Digest: digest([]byte(patch)), Confidence: evidence.Confidence{Score: linkageScore(run.Repository.DirtyBefore), Reasons: linkageReasons(run.Repository.DirtyBefore)},
	}
	if patch == "" {
		if len(run.Repository.Changes) == 0 {
			patchRecord.State = evidence.NotObserved
			patchRecord.Required = false
			patchRecord.Discovered = false
			patchRecord.Digest = ""
			patchRecord.Reason = "the range contains no changes, so there is no patch to capture"
			patchRecord.Confidence = evidence.Confidence{Score: 100, Reasons: []string{"empty-range"}}
		} else {
			patchRecord.State = evidence.Missing
			patchRecord.Digest = ""
			patchRecord.Reason = "changes were detected but no patch content was captured"
			patchRecord.Confidence = evidence.Confidence{Score: 0, Reasons: []string{"patch-capture-failed"}}
		}
	}
	records := []evidence.Record{patchRecord}
	if len(run.Sessions) == 0 {
		records = append(records, evidence.Record{
			Locator: "sessions/native", State: evidence.NotObserved, Reason: "no native session artifact was discovered",
			Confidence: evidence.Confidence{Score: 100, Reasons: []string{"discovery-complete"}},
		})
	}
	for i, session := range run.Sessions {
		state := session.State
		if state == "" {
			state = evidence.Unknown
		}
		reason := session.Reason
		if state != evidence.Observed && reason == "" {
			reason = "native session could not be normalized"
		}
		records = append(records, evidence.Record{
			Locator: fmt.Sprintf("sessions/%s/%03d-%s", safeLocatorPart(session.Adapter), i, safeLocatorPart(session.File)),
			State:   state, Discovered: true, Digest: session.Digest, Reason: reason,
			Confidence: evidence.Confidence{Score: stateScore(state), Reasons: []string{session.ParserVersion}},
		})
	}
	for i, path := range run.Repository.UncapturedPaths {
		records = append(records, evidence.Record{
			Locator: fmt.Sprintf("git/uncaptured/%03d-%s", i, safeLocatorPart(path)),
			State:   evidence.Unknown, Discovered: true, Reason: "changed binary, symlink, oversized, or unreadable content was not captured",
			Confidence: evidence.Confidence{Score: 0, Reasons: []string{"uncaptured-worktree-content"}},
		})
	}
	records = append(records, testRecords...)
	return records
}

// Integrity is a reviewer-facing claim, so recompute the canonical identity
// from the emitted bytes instead of asserting success beside them.
func integrityOf(manifestBytes []byte) string {
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.UseNumber()
	var emitted map[string]any
	if err := decoder.Decode(&emitted); err != nil {
		return "unknown"
	}
	claimed, ok := emitted["bundleId"].(string)
	if !ok || claimed == "" {
		return "unknown"
	}
	delete(emitted, "bundleId")
	canonical, err := json.Marshal(emitted)
	if err != nil {
		return "unknown"
	}
	sum := sha256.Sum256(canonical)
	if hex.EncodeToString(sum[:]) != claimed {
		return "failed"
	}
	return "passed"
}

func status(run evidence.Run) string {
	if !run.Completeness.Complete || (run.Tests.Ingested && !run.Tests.Passed) {
		return "failed"
	}
	for _, finding := range run.Findings {
		if finding.Severity == "critical" {
			return "failed"
		}
	}
	if len(run.Findings) > 0 || !run.Tests.Ingested || run.Repository.DirtyBefore || len(run.Impact.Unsupported) > 0 || !run.Impact.Complete {
		return "warning"
	}
	return "passed"
}

func warningCount(run evidence.Run) int {
	total := 0
	for _, finding := range run.Findings {
		if finding.Severity != "critical" {
			total++
		}
	}
	if !run.Tests.Ingested {
		total++
	}
	if run.Repository.DirtyBefore {
		total++
	}
	total += len(run.Repository.UncapturedPaths)
	total += len(run.Impact.Unsupported) + len(run.Impact.Unknown)
	if run.Impact.LimitReached != "" {
		total++
	}
	return total
}

func mergeFindings(groups ...[]evidence.Finding) []evidence.Finding {
	seen := map[string]bool{}
	var result []evidence.Finding
	for _, group := range groups {
		for _, finding := range group {
			key := fmt.Sprintf("%s\x00%s\x00%d", finding.ID, finding.Path, finding.Line)
			if seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, finding)
		}
	}
	return result
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func linkageScore(dirty bool) uint8 {
	if dirty {
		return 20
	}
	return 100
}

// Reasons explain a confidence score; they are not association values. The
// clean case once spelled "clean-baseline" here too, which read as if the
// Association vocabulary had leaked into a different axis.
func linkageReasons(dirty bool) []string {
	if dirty {
		return []string{"dirty-worktree", "window-association-only"}
	}
	return []string{"clean-worktree", "git-range-match"}
}

func stateScore(state evidence.State) uint8 {
	if state == evidence.Observed {
		return 100
	}
	return 0
}

func safeLocatorPart(value string) string {
	value = filepath.Base(strings.ReplaceAll(value, "\\", "/"))
	value = strings.ReplaceAll(value, "..", "_")
	if value == "" || value == "." {
		return "unknown"
	}
	return value
}

func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return safefile.Write(path, append(b, '\n'), 0o600)
}
