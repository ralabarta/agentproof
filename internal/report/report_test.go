package report

import (
	"strings"
	"testing"

	"github.com/ralabarta/agentproof/internal/evidence"
)

func TestReportsEscapeHostilePresentationData(t *testing.T) {
	run := evidence.Run{
		Objective:    "<script>alert(1)</script>\n| injected",
		Status:       "warning",
		Tests:        evidence.TestResult{Summary: "not supplied"},
		Repository:   evidence.Repository{Changes: []evidence.Change{{Path: "a|b.go", Status: "modified"}}, AssociationStatus: "clean-baseline"},
		Impact:       evidence.Impact{Complete: true},
		Completeness: evidence.Completeness{Observed: 1, Required: 1, Percent: 100, Complete: true},
	}
	markdown := string(Markdown(run, strings.Repeat("a", 64)))
	if strings.Contains(markdown, "\n| injected") || !strings.Contains(markdown, `a\|b.go`) {
		t.Fatalf("Markdown was not safely escaped:\n%s", markdown)
	}
	html, err := HTML(run, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	text := string(html)
	if strings.Contains(text, "<script>alert") || !strings.Contains(text, "Content-Security-Policy") {
		t.Fatalf("HTML was not safely escaped or lacks CSP: %s", text)
	}
}
