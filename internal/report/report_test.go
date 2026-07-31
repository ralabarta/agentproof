package report

import (
	"strings"
	"testing"

	"github.com/ralabarta/agentproof/internal/evidence"
)

// Report values come from the repository and agent under observation, so a
// reviewer reading report.md must not be shown markup those inputs forged.
func TestMarkdownEscapesInlineActiveCharacters(t *testing.T) {
	run := evidence.Run{
		Objective:  "see [report](javascript:alert) and <img src=x> _stress_",
		Status:     "warning",
		Repository: evidence.Repository{Changes: []evidence.Change{{Path: "internal/a_b*.go", Status: "modified"}}},
		Findings:   []evidence.Finding{{ID: "AP001", Severity: "high", Title: "token in [config](http://evil.example)", Path: "a_b.go", Line: 3}},
	}
	markdown := string(Markdown(run, strings.Repeat("a", 64)))
	for _, forged := range []string{"](javascript:alert)", "](http://evil.example)", "<img src=x>"} {
		if strings.Contains(markdown, forged) {
			t.Fatalf("hostile inline markup survived escaping: %q\n%s", forged, markdown)
		}
	}
	if !strings.Contains(markdown, `\_stress\_`) {
		t.Fatalf("emphasis characters were not escaped:\n%s", markdown)
	}
	// Code spans render backslashes literally, so escaping there would corrupt
	// every path and hash the report shows.
	if !strings.Contains(markdown, "`internal/a_b*.go`") {
		t.Fatalf("code spans must stay verbatim, not backslash-mangled:\n%s", markdown)
	}
}

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
