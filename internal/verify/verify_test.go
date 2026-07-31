package verify

import "testing"

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
