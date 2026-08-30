package evidence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A manifest is the census a bundle identity is computed over, so a record
// carrying a state outside the published vocabulary must never be serialized:
// it would describe evidence in terms no reader can interpret.
func TestManifestRejectsStatesOutsideTheVocabulary(t *testing.T) {
	for _, state := range States() {
		record := Record{Locator: "logs/a.json", State: state, Reason: "stated"}
		if state == Observed {
			record.Reason = ""
			record.Digest = digest("a")
		}
		manifest := NewManifest([]Record{record})
		if _, err := manifest.CanonicalBytes(); err != nil {
			t.Errorf("published state %q was rejected: %v", state, err)
		}
	}
	manifest := NewManifest([]Record{{Locator: "logs/a.json", State: State("totally-made-up"), Reason: "stated"}})
	if _, err := manifest.CanonicalBytes(); err == nil {
		t.Fatal("a state outside the vocabulary was accepted")
	}
}

func TestCanonicalIdentityIsDeterministic(t *testing.T) {
	records := []Record{
		{Locator: "logs/z.json", State: Unknown, Required: true, Reason: "parser-drift", Confidence: Confidence{Score: 20, Reasons: []string{"version-mismatch", "format-unknown"}}},
		{Locator: "logs/a.json", State: Observed, Discovered: true, Digest: digest("a"), Confidence: Confidence{Score: 100, Reasons: []string{"normalized"}}},
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
	firstID, _ := first.Identity()
	secondID, _ := second.Identity()
	if firstID != secondID || len(firstID) != 64 {
		t.Fatalf("identities = %q and %q", firstID, secondID)
	}
}

func TestFinalBytesIncludesOnlyComputedBundleIdentity(t *testing.T) {
	manifest := NewManifest([]Record{{Locator: "git/patch", State: Observed, Required: true, Digest: digest("b"), Confidence: Confidence{Score: 100}}})
	manifest.BundleID = "untrusted"
	identity, err := manifest.Identity()
	if err != nil {
		t.Fatal(err)
	}
	final, err := manifest.FinalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(final), `"bundleId":"`+identity+`"`) || strings.Contains(string(final), "untrusted") {
		t.Fatalf("unexpected final manifest: %s", final)
	}
}

func TestManifestRejectsUnsafeOrInvalidRecords(t *testing.T) {
	tests := []Record{
		{Locator: "../secret", State: Observed, Digest: digest("x")},
		{Locator: "/absolute", State: Observed, Digest: digest("x")},
		{Locator: "missing", State: Missing, Required: true},
		{Locator: "optional", State: NotObserved, Required: true, Reason: "not supplied"},
		{Locator: "bad-digest", State: Observed, Digest: "sha256:nope"},
	}
	for _, record := range tests {
		if _, err := NewManifest([]Record{record}).CanonicalBytes(); err == nil {
			t.Fatalf("expected validation error for %#v", record)
		}
	}
}

func TestCompletenessCountsDiscoveredAndRequiredOnce(t *testing.T) {
	manifest := NewManifest([]Record{
		{Locator: "required", State: Observed, Required: true, Digest: digest("a")},
		{Locator: "discovered", State: Missing, Discovered: true, Reason: "unreadable"},
		{Locator: "optional", State: NotObserved, Reason: "not supplied"},
	})
	got := manifest.Completeness()
	if got.Observed != 1 || got.Required != 2 || got.Percent != 50 || got.Complete {
		t.Fatalf("unexpected completeness: %#v", got)
	}
}

func TestManifestCanonicalizationDoesNotMutateConfidenceReasons(t *testing.T) {
	tests := []struct {
		name string
		run  func(Manifest) error
	}{
		{name: "canonical bytes", run: func(manifest Manifest) error {
			_, err := manifest.CanonicalBytes()
			return err
		}},
		{name: "identity", run: func(manifest Manifest) error {
			_, err := manifest.Identity()
			return err
		}},
		{name: "final bytes", run: func(manifest Manifest) error {
			_, err := manifest.FinalBytes()
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reasons := []string{"z", "a", "a"}
			want := slices.Clone(reasons)
			manifest := NewManifest([]Record{{
				Locator: "logs/result.json", State: Observed, Digest: digest("c"),
				Confidence: Confidence{Score: 90, Reasons: reasons},
			}})

			if err := tt.run(manifest); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(reasons, want) {
				t.Fatalf("confidence reasons mutated: got %q, want %q", reasons, want)
			}
		})
	}
}

func TestCanonicalBytesNormalizesPathsAndReasons(t *testing.T) {
	manifest := NewManifest([]Record{{
		Locator: "logs\\nested\\..\\result.json", State: Observed, Discovered: true,
		Digest: digest("c"), Confidence: Confidence{Score: 90, Reasons: []string{"z", "a", "a"}},
	}})
	got, err := manifest.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"locator":"logs/result.json"`) || !strings.Contains(string(got), `"reasons":["a","z"]`) {
		t.Fatalf("normalization missing: %s", got)
	}
}

// The README's evidence-state table is the trust model users read before they
// trust a bundle. It drifted from the code once already — documenting a state
// that never existed and omitting three that did — so the vocabularies are
// enumerable here and the document is checked against them, not against prose.
func TestREADMEDocumentsEveryEvidenceVocabulary(t *testing.T) {
	text := readDoc(t, "README.md")
	for _, value := range publishedVocabulary() {
		if !strings.Contains(text, "`"+value+"`") {
			t.Errorf("README does not document the %q vocabulary value", value)
		}
	}
	assertNoPhantomState(t, "README.md", text)
}

// CONTRIBUTING.md tells contributors which words a claim may use, so a word it
// blesses that no vocabulary emits teaches the drift instead of preventing it.
func TestContributingTeachesOnlyEmittedVocabulary(t *testing.T) {
	assertNoPhantomState(t, "CONTRIBUTING.md", readDoc(t, "CONTRIBUTING.md"))
}

func assertNoPhantomState(t *testing.T, name, text string) {
	t.Helper()
	// "associated" was documented as an evidence state for releases in which no
	// such state was ever emitted. A vocabulary the code cannot produce is a
	// false claim about the trust model.
	if strings.Contains(text, "**associated**") || strings.Contains(text, "`associated`") {
		t.Errorf("%s documents `associated`, which no vocabulary emits", name)
	}
}

func publishedVocabulary() []string {
	var values []string
	for _, state := range States() {
		values = append(values, string(state))
	}
	for _, confidence := range ClaimConfidences() {
		values = append(values, string(confidence))
	}
	for _, association := range Associations() {
		values = append(values, string(association))
	}
	return values
}

func readDoc(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Two records are too few to exercise a sort: the bundle identity is a hash of
// the ordered census, so ordering must hold at a size where the sort actually
// swaps, and must not depend on the order records were discovered in.
func TestCanonicalOrderIsIndependentOfInputOrderAtScale(t *testing.T) {
	const count = 64
	const hexAlphabet = "0123456789abcdef"
	records := make([]Record, count)
	for i := range records {
		records[i] = Record{
			Locator: fmt.Sprintf("logs/%02d.json", i), State: Observed, Required: true,
			Digest: digest(string(hexAlphabet[i%16])), Confidence: Confidence{Score: 100},
		}
	}
	reversed := make([]Record, count)
	for i, record := range records {
		reversed[count-1-i] = record
	}

	forward, err := NewManifest(records).CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	backward, err := NewManifest(reversed).CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(forward) != string(backward) {
		t.Fatal("canonical bytes depend on the order records were discovered in")
	}

	var decoded struct {
		Records []map[string]any `json:"records"`
	}
	if err := json.Unmarshal(forward, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Records) != count {
		t.Fatalf("expected %d records, got %d", count, len(decoded.Records))
	}
	// Every emitted record must still carry its own digest. A sort that moves a
	// record without its ordering key silently rewrites the census it hashes.
	for _, record := range decoded.Records {
		locator, _ := record["locator"].(string)
		var index int
		if _, err := fmt.Sscanf(locator, "logs/%d.json", &index); err != nil {
			t.Fatalf("unexpected locator %q", locator)
		}
		if want := digest(string(hexAlphabet[index%16])); record["digest"] != want {
			t.Fatalf("%s carries digest %v, want %v", locator, record["digest"], want)
		}
	}
	for i := 1; i < len(decoded.Records); i++ {
		if decoded.Records[i-1]["locator"].(string) >= decoded.Records[i]["locator"].(string) {
			t.Fatalf("records are not ordered at index %d: %v", i, decoded.Records)
		}
	}
}

func digest(seed string) string {
	return "sha256:" + strings.Repeat(seed, 64)
}
