package evidence

import (
	"strings"
	"testing"
)

func TestSchemaAndStateWireValues(t *testing.T) {
	if SchemaVersion != "agentproof.dev/evidence/v1" {
		t.Fatalf("SchemaVersion = %q", SchemaVersion)
	}

	states := []State{Observed, Missing, Unsupported, Unknown, NotObserved}
	want := []string{"observed", "missing", "unsupported", "unknown", "not_observed"}
	for i := range states {
		if string(states[i]) != want[i] {
			t.Errorf("state %d = %q, want %q", i, states[i], want[i])
		}
	}
}

func TestCanonicalBytesRejectsInvalidRecords(t *testing.T) {
	valid := Record{Locator: "logs/result.json", State: Observed, Confidence: Confidence{Score: 100}}
	tests := []struct {
		name    string
		record  Record
		wantErr string
	}{
		{name: "unsupported state", record: func() Record {
			record := valid
			record.State = State("invalid")
			return record
		}(), wantErr: "unsupported state"},
		{name: "confidence score above 100", record: func() Record {
			record := valid
			record.Confidence.Score = 101
			return record
		}(), wantErr: "confidence score"},
		{name: "unknown without non-empty reason", record: func() Record {
			record := valid
			record.State = Unknown
			record.Reason = " \t"
			return record
		}(), wantErr: "unknown state requires a reason"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := NewManifest([]Record{tt.record})
			content, err := manifest.CanonicalBytes()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("CanonicalBytes() error = %v, want containing %q", err, tt.wantErr)
			}
			if content != nil {
				t.Fatalf("CanonicalBytes() = %q on validation error, want nil", content)
			}
			identity, err := manifest.Identity()
			if err == nil || identity != "" {
				t.Fatalf("Identity() = %q, %v; want empty identity and error", identity, err)
			}
		})
	}
}

func TestCanonicalIdentityIsDeterministic(t *testing.T) {
	records := []Record{
		{
			Locator: "logs/z.json", State: Unknown, Required: true,
			Digest: "sha256:z", Reason: "parser-drift",
			Confidence: Confidence{Score: 20, Reasons: []string{"version-mismatch", "format-unknown"}},
		},
		{
			Locator: "logs/a.json", State: Observed, Required: true,
			Digest: "sha256:a", Reason: "",
			Confidence: Confidence{Score: 100, Reasons: []string{"normalized"}},
		},
	}

	first := NewManifest(records)
	second := NewManifest([]Record{records[1], records[0]})
	first.BundleID = "ignored-existing-id"
	second.BundleID = "another-ignored-id"

	firstBytes, err := first.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := second.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBytes) != string(secondBytes) {
		t.Fatalf("canonical bytes differ:\n%s\n%s", firstBytes, secondBytes)
	}
	if strings.Contains(string(firstBytes), "bundleId") {
		t.Fatalf("canonical bytes include bundleId: %s", firstBytes)
	}

	firstID, err := first.Identity()
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := second.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if firstID != secondID || len(firstID) != 64 {
		t.Fatalf("identities = %q and %q", firstID, secondID)
	}

	changed := NewManifest(records)
	changed.Records[0].Digest = "sha256:changed"
	changedID, err := changed.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if changedID == firstID {
		t.Fatal("canonical field change did not change identity")
	}
}

func TestCanonicalBytesUsesLexicographicallySortedObjectKeys(t *testing.T) {
	bundle := Bundle{
		Manifest: NewManifest([]Record{{
			Locator: "logs/result.json", State: Observed, Required: true,
			Digest: "sha256:result", Reason: "verified",
			Confidence: Confidence{Score: 90, Reasons: []string{"signature-valid", "digest-match"}},
		}}),
		Presentation: PresentationMetadata{Title: "Ignored title"},
	}
	bundle.Manifest.BundleID = "ignored-bundle-id"

	got, err := bundle.Manifest.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"records":[{"confidence":{"reasons":["digest-match","signature-valid"],"score":90},"digest":"sha256:result","locator":"logs/result.json","reason":"verified","required":true,"state":"observed"}],"schemaVersion":"agentproof.dev/evidence/v1"}`
	if string(got) != want {
		t.Fatalf("canonical bytes =\n%s\nwant:\n%s", got, want)
	}
}

func TestCanonicalIdentityOrdersByCompleteSerializedRecord(t *testing.T) {
	records := []Record{
		{Locator: "same.json", State: Observed, Required: true, Digest: "sha256:same", Reason: "z"},
		{Locator: "same.json", State: Observed, Required: false, Digest: "sha256:same", Reason: "a"},
	}
	canonical := func(records []Record) (string, string) {
		manifest := NewManifest(records)
		content, err := manifest.CanonicalBytes()
		if err != nil {
			t.Fatal(err)
		}
		identity, err := manifest.Identity()
		if err != nil {
			t.Fatal(err)
		}
		return string(content), identity
	}

	firstBytes, firstID := canonical(records)
	secondBytes, secondID := canonical([]Record{records[1], records[0]})
	if firstBytes != secondBytes || firstID != secondID {
		t.Fatalf("reversed input changed canonical result:\nbytes: %s\n       %s\nidentity: %s\n          %s", firstBytes, secondBytes, firstID, secondID)
	}
}

func TestPresentationMetadataIsOutsideCanonicalManifest(t *testing.T) {
	manifest := NewManifest([]Record{{
		Locator: "session.json", State: Observed, Required: true,
		Digest: "sha256:session", Confidence: Confidence{Score: 100, Reasons: []string{"digest-match"}},
	}})
	first := Bundle{Manifest: manifest, Presentation: PresentationMetadata{Title: "First"}}
	second := Bundle{Manifest: manifest, Presentation: PresentationMetadata{Title: "Second"}}

	firstBytes, err := first.Manifest.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := second.Manifest.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBytes) != string(secondBytes) {
		t.Fatal("presentation metadata changed canonical manifest bytes")
	}
}
