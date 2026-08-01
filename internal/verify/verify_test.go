package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralabarta/agentproof/internal/evidence"
)

// latest.json is repository content an attacker can influence via a pull
// request, so a record locator must never resolve outside .agentproof.
func TestLoadRunRejectsRecordPathOutsideAgentProofDir(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "record.json"), []byte(`{"schema_version":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "changes.patch"), []byte("--- a/x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".agentproof")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "latest.json"), []byte(`{"record":"../outside/record.json"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadRun(root, ""); err == nil {
		t.Fatal("a record locator escaping .agentproof must be rejected")
	} else if !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("expected a containment error, got %v", err)
	}
}

// The lexical check above only inspects the string. A directory symlink keeps
// every component inside .agentproof while the bytes come from elsewhere.
func TestLoadRunRejectsRecordPathBehindDirectorySymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "record.json"), []byte(`{"schema_version":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "changes.patch"), []byte("--- a/x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".agentproof")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "latest.json"), []byte(`{"record":"link/record.json"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadRun(root, ""); err == nil {
		t.Fatal("a record locator resolving outside .agentproof must be rejected")
	} else if !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("expected a containment error, got %v", err)
	}
}

func TestStatus(t *testing.T) {
	if got := status(runWithTests(true)); got != "passed" {
		t.Fatalf("expected passed, got %s", got)
	}
	run := runWithTests(false)
	if got := status(run); got != "failed" {
		t.Fatalf("expected failed, got %s", got)
	}
}

func TestStatusFailsClosedOnIncompleteEvidence(t *testing.T) {
	run := runWithTests(true)
	run.Completeness.Complete = false
	if got := status(run); got != "failed" {
		t.Fatalf("expected failed, got %s", got)
	}
}

// An empty patch still hashes to a valid sha256, so an asserted "observed"
// state would claim a required source was captured when nothing was.
func TestManifestRecordsDoesNotClaimEmptyPatchWasObserved(t *testing.T) {
	run := evidence.Run{Repository: evidence.Repository{Changes: []evidence.Change{{Path: "x", Status: "modified"}}}}
	records := manifestRecords(run, "", nil)
	if len(records) == 0 {
		t.Fatal("expected at least the git/changes.patch record")
	}
	patchRecord := records[0]
	if patchRecord.Locator != "git/changes.patch" {
		t.Fatalf("expected git/changes.patch first, got %s", patchRecord.Locator)
	}
	if patchRecord.State == evidence.Observed {
		t.Fatal("an empty patch must not be reported as observed evidence")
	}
	if patchRecord.Reason == "" {
		t.Fatal("a non-observed record must state why")
	}
	if evidence.NewManifest(records).Completeness().Complete {
		t.Fatal("missing required patch evidence must not be complete")
	}
}

// "Canonical manifest integrity: passed" is printed to reviewers, so it must be
// recomputed from the emitted bytes rather than asserted alongside them.
func TestIntegrityIsRecomputedFromEmittedManifestBytes(t *testing.T) {
	manifest := evidence.NewManifest(manifestRecords(evidence.Run{}, "--- a/x\n", nil))
	emitted, err := manifest.FinalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if got := integrityOf(emitted); got != "passed" {
		t.Fatalf("an untampered manifest must verify, got %s", got)
	}
	tampered := strings.Replace(string(emitted), "git/changes.patch", "git/changes.patcH", 1)
	if tampered == string(emitted) {
		t.Fatal("test could not tamper with the manifest")
	}
	if got := integrityOf([]byte(tampered)); got == "passed" {
		t.Fatal("a mutated manifest must not report passed integrity")
	}
	if got := integrityOf([]byte("not json")); got != "unknown" {
		t.Fatalf("unparseable bytes are indeterminate, got %s", got)
	}
}

// A range with no changes has no patch to capture, so requiring one would
// report a capture failure that never happened.
func TestManifestRecordsDistinguishesNoChangesFromFailedCapture(t *testing.T) {
	empty := manifestRecords(evidence.Run{}, "", nil)[0]
	if empty.Required || empty.Discovered {
		t.Fatal("with no changes there is no required patch source")
	}
	if empty.State != evidence.NotObserved {
		t.Fatalf("expected not_observed for an empty range, got %s", empty.State)
	}
	if !evidence.NewManifest(manifestRecords(evidence.Run{}, "", nil)).Completeness().Complete {
		t.Fatal("an empty range must not be reported as incomplete evidence")
	}

	changed := evidence.Run{Repository: evidence.Repository{Changes: []evidence.Change{{Path: "x", Status: "modified"}}}}
	failed := manifestRecords(changed, "", nil)[0]
	if failed.State == evidence.Observed || !failed.Required {
		t.Fatalf("changes without a patch is a required-source failure, got %s", failed.State)
	}
	if evidence.NewManifest(manifestRecords(changed, "", nil)).Completeness().Complete {
		t.Fatal("a failed patch capture must not be complete")
	}
}

func TestManifestRecordsObservesNonEmptyPatch(t *testing.T) {
	records := manifestRecords(evidence.Run{}, "--- a/x\n+++ b/x\n", nil)
	if records[0].State != evidence.Observed {
		t.Fatalf("a captured patch is observed evidence, got %s", records[0].State)
	}
	if records[0].Digest == "" {
		t.Fatal("observed evidence requires a digest")
	}
}
