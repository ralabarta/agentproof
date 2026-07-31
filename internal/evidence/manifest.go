package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
)

const EvidenceSchemaVersion = "agentproof.dev/evidence/v1"

type State string

const (
	Observed    State = "observed"
	Missing     State = "missing"
	Unsupported State = "unsupported"
	Unknown     State = "unknown"
	NotObserved State = "not_observed"
)

type Confidence struct {
	Score   uint8    `json:"score"`
	Reasons []string `json:"reasons"`
}

type Record struct {
	Locator    string     `json:"locator"`
	State      State      `json:"state"`
	Required   bool       `json:"required"`
	Discovered bool       `json:"discovered"`
	Digest     string     `json:"digest"`
	Reason     string     `json:"reason"`
	Confidence Confidence `json:"confidence"`
}

type Manifest struct {
	BundleID      string   `json:"bundleId"`
	SchemaVersion string   `json:"schemaVersion"`
	Records       []Record `json:"records"`
}

func NewManifest(records []Record) Manifest {
	return Manifest{
		SchemaVersion: EvidenceSchemaVersion,
		Records:       append([]Record(nil), records...),
	}
}

func (m Manifest) CanonicalBytes() ([]byte, error) {
	records, err := m.normalizedRecords()
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"records":       canonicalRecords(records),
		"schemaVersion": m.SchemaVersion,
	})
}

func (m Manifest) Identity() (string, error) {
	canonical, err := m.CanonicalBytes()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func (m Manifest) FinalBytes() ([]byte, error) {
	records, err := m.normalizedRecords()
	if err != nil {
		return nil, err
	}
	identity, err := m.Identity()
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"bundleId":      identity,
		"records":       canonicalRecords(records),
		"schemaVersion": m.SchemaVersion,
	})
}

func (m Manifest) Completeness() Completeness {
	result := Completeness{}
	for _, record := range m.Records {
		if !record.Required && !record.Discovered {
			continue
		}
		result.Required++
		if record.State == Observed {
			result.Observed++
		}
	}
	if result.Required == 0 {
		result.Percent = 100
		result.Complete = true
		return result
	}
	result.Percent = float64(result.Observed) / float64(result.Required) * 100
	result.Complete = result.Observed == result.Required
	return result
}

func (m Manifest) normalizedRecords() ([]Record, error) {
	if m.SchemaVersion != EvidenceSchemaVersion {
		return nil, fmt.Errorf("unsupported evidence schema %q", m.SchemaVersion)
	}
	records := append([]Record(nil), m.Records...)
	seen := map[string]bool{}
	for i := range records {
		record := &records[i]
		normalized, err := normalizeLocator(record.Locator)
		if err != nil {
			return nil, fmt.Errorf("record %d: %w", i, err)
		}
		record.Locator = normalized
		if seen[normalized] {
			return nil, fmt.Errorf("record %d: duplicate locator %q", i, normalized)
		}
		seen[normalized] = true
		if err := validateRecord(*record, i); err != nil {
			return nil, err
		}
		record.Confidence.Reasons = normalizeStrings(record.Confidence.Reasons)
	}
	sort.Slice(records, func(i, j int) bool {
		left, _ := json.Marshal(records[i])
		right, _ := json.Marshal(records[j])
		return bytes.Compare(left, right) < 0
	})
	return records, nil
}

func validateRecord(record Record, index int) error {
	switch record.State {
	case Observed, Missing, Unsupported, Unknown, NotObserved:
	default:
		return fmt.Errorf("record %d: unsupported state %q", index, record.State)
	}
	if record.Confidence.Score > 100 {
		return fmt.Errorf("record %d: confidence score %d exceeds 100", index, record.Confidence.Score)
	}
	if record.State == NotObserved && (record.Required || record.Discovered) {
		return fmt.Errorf("record %d: not_observed is limited to optional undiscovered sources", index)
	}
	if record.State == Observed {
		if !validDigest(record.Digest) {
			return fmt.Errorf("record %d: observed state requires a lowercase sha256 digest", index)
		}
	} else if strings.TrimSpace(record.Reason) == "" {
		return fmt.Errorf("record %d: state %q requires a reason", index, record.State)
	}
	return nil
}

func normalizeLocator(locator string) (string, error) {
	if len(locator) == 0 || len(locator) > 4096 || strings.ContainsRune(locator, 0) {
		return "", fmt.Errorf("invalid locator length or content")
	}
	locator = strings.ReplaceAll(locator, "\\", "/")
	if strings.HasPrefix(locator, "/") || hasDrivePrefix(locator) {
		return "", fmt.Errorf("locator must be repository-relative")
	}
	cleaned := path.Clean(locator)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("locator escapes its evidence root")
	}
	return cleaned, nil
}

func hasDrivePrefix(locator string) bool {
	return len(locator) >= 2 && ((locator[0] >= 'A' && locator[0] <= 'Z') || (locator[0] >= 'a' && locator[0] <= 'z')) && locator[1] == ':'
}

func validDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && value == strings.ToLower(value)
}

func normalizeStrings(values []string) []string {
	sort.Strings(values)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || (len(result) > 0 && result[len(result)-1] == value) {
			continue
		}
		result = append(result, value)
	}
	return result
}

func canonicalRecords(records []Record) []map[string]any {
	result := make([]map[string]any, len(records))
	for i, record := range records {
		result[i] = map[string]any{
			"confidence": map[string]any{
				"reasons": record.Confidence.Reasons,
				"score":   record.Confidence.Score,
			},
			"digest":     record.Digest,
			"discovered": record.Discovered,
			"locator":    record.Locator,
			"reason":     record.Reason,
			"required":   record.Required,
			"state":      record.State,
		}
	}
	return result
}
