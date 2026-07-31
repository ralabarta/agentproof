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

const SchemaVersion = "agentproof.dev/evidence/v1"

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
	Digest     string     `json:"digest"`
	Reason     string     `json:"reason"`
	Confidence Confidence `json:"confidence"`
}

type Manifest struct {
	BundleID      string   `json:"bundleId"`
	SchemaVersion string   `json:"schemaVersion"`
	Records       []Record `json:"records"`
}

type PresentationMetadata struct {
	Title string
}

type Bundle struct {
	Manifest     Manifest
	Presentation PresentationMetadata
}

func NewManifest(records []Record) Manifest {
	return Manifest{
		SchemaVersion: SchemaVersion,
		Records:       append([]Record{}, records...),
	}
}

func (m Manifest) CanonicalBytes() ([]byte, error) {
	for i, record := range m.Records {
		switch record.State {
		case Observed, Missing, Unsupported, Unknown, NotObserved:
		default:
			return nil, fmt.Errorf("record %d: unsupported state %q", i, record.State)
		}
		if record.Confidence.Score > 100 {
			return nil, fmt.Errorf("record %d: confidence score %d exceeds 100", i, record.Confidence.Score)
		}
		if record.State == Unknown && strings.TrimSpace(record.Reason) == "" {
			return nil, fmt.Errorf("record %d: unknown state requires a reason", i)
		}
	}

	records := append([]Record(nil), m.Records...)
	for i := range records {
		records[i].Locator = path.Clean(strings.ReplaceAll(records[i].Locator, "\\", "/"))
		records[i].Confidence.Reasons = append([]string{}, records[i].Confidence.Reasons...)
		sort.Strings(records[i].Confidence.Reasons)
	}
	sort.Slice(records, func(i, j int) bool {
		left, _ := json.Marshal(records[i])
		right, _ := json.Marshal(records[j])
		return bytes.Compare(left, right) < 0
	})

	canonicalRecords := make([]map[string]any, len(records))
	for i, record := range records {
		canonicalRecords[i] = map[string]any{
			"confidence": map[string]any{"reasons": record.Confidence.Reasons, "score": record.Confidence.Score},
			"digest":     record.Digest, "locator": record.Locator, "reason": record.Reason,
			"required": record.Required, "state": record.State,
		}
	}
	return json.Marshal(map[string]any{
		"records": canonicalRecords, "schemaVersion": m.SchemaVersion,
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
