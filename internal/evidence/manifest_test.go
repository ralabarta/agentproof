package evidence

import (
	"strings"
	"testing"
)

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

func digest(seed string) string {
	return "sha256:" + strings.Repeat(seed, 64)
}
