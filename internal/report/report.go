package report

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/ralabarta/agentproof/internal/evidence"
)

func Markdown(run evidence.Run, hash string) []byte {
	var b strings.Builder
	icon := "⚠"
	if run.Status == "passed" {
		icon = "✓"
	} else if run.Status == "failed" {
		icon = "✗"
	}
	fmt.Fprintf(&b, "# AgentProof verification\n\n%s **%s**\n\n", icon, strings.ToUpper(run.Status))
	fmt.Fprintf(&b, "> %s\n\n", mdText(run.Objective))
	b.WriteString("## Summary\n\n")
	if run.Tests.Ingested {
		if run.Tests.Passed {
			fmt.Fprintf(&b, "- ✓ Test evidence passed (%d passed, %d failed, %d skipped)\n", run.Tests.PassedTests, run.Tests.FailedTests, run.Tests.SkippedTests)
		} else {
			fmt.Fprintf(&b, "- ✗ Test evidence is not passing (%d passed, %d failed, %d skipped)\n", run.Tests.PassedTests, run.Tests.FailedTests, run.Tests.SkippedTests)
		}
	} else {
		b.WriteString("- ⚠ No test-result artifact was supplied; AgentProof did not execute repository code\n")
	}
	critical := countSeverity(run.Findings, "critical")
	if critical == 0 {
		b.WriteString("- ✓ No secret patterns detected in the captured added lines\n")
	} else {
		fmt.Fprintf(&b, "- ✗ %d critical finding(s)\n", critical)
	}
	fmt.Fprintf(&b, "- %d files changed; +%d/-%d lines\n", len(run.Repository.Changes), additions(run.Repository.Changes), deletions(run.Repository.Changes))
	fmt.Fprintf(&b, "- Impact radius: %d; %d components affected\n", run.Impact.Radius, len(run.Impact.AffectedComponents))
	fmt.Fprintf(&b, "- Required evidence completeness: %d/%d (%.2f%%)\n", run.Completeness.Observed, run.Completeness.Required, run.Completeness.Percent)
	fmt.Fprintf(&b, "- Canonical manifest integrity: %s\n", mdText(run.Integrity))
	if run.Repository.DirtyBefore {
		b.WriteString("- ⚠ Repository was dirty before recording; Git association confidence is low\n")
	}
	if len(run.Repository.UncapturedPaths) > 0 {
		fmt.Fprintf(&b, "- ✗ Uncaptured changed content: %s\n", mdText(strings.Join(run.Repository.UncapturedPaths, ", ")))
	}
	b.WriteString("\n## Provenance\n\n")
	fmt.Fprintf(&b, "| Field | Value |\n|---|---|\n| Agent | `%s` |\n| Model | `%s` |\n| Duration | `%s` |\n| Start commit | `%s` |\n| End commit | `%s` |\n| Association | `%s` |\n", mdCode(safe(run.Agent)), mdCode(safe(run.Model)), duration(run.DurationMS), mdCode(short(run.Repository.StartHead)), mdCode(short(run.Repository.EndHead)), mdCode(string(run.Repository.AssociationStatus)))
	b.WriteString("\n## Changes\n\n| File | Status | Added | Deleted |\n|---|---:|---:|---:|\n")
	if len(run.Repository.Changes) == 0 {
		b.WriteString("| _No changes detected_ | | 0 | 0 |\n")
	}
	for _, change := range run.Repository.Changes {
		fmt.Fprintf(&b, "| `%s` | %s | %d | %d |\n", mdCode(change.Path), mdText(change.Status), change.Added, change.Deleted)
	}
	if len(run.Repository.Commits) > 0 {
		b.WriteString("\n### Commits\n\n")
		for _, commit := range run.Repository.Commits {
			fmt.Fprintf(&b, "- `%s` %s\n", mdCode(short(commit.Hash)), mdText(commit.Summary))
		}
	}
	if len(run.Sessions) > 0 {
		b.WriteString("\n## Agent sessions\n\n| Adapter | Artifact | Prompts | Tools | SHA-256 |\n|---|---|---:|---|---|\n")
		for _, session := range run.Sessions {
			fmt.Fprintf(&b, "| %s | `%s` | %d | %s | `%s` |\n", mdText(session.Adapter), mdCode(session.File), session.PromptCount, mdText(strings.Join(session.Tools, ", ")), mdCode(short(session.Digest)))
		}
		fmt.Fprintf(&b, "\nTokens: %d input · %d output · %d cached\n\n", run.Usage.InputTokens, run.Usage.OutputTokens, run.Usage.CachedTokens)
	}
	b.WriteString("\n## Impact\n\n")
	fmt.Fprintf(&b, "Analyzer: `%s` · examined %d files / %d bytes · complete: %t\n\n", mdCode(run.Impact.Analyzer), run.Impact.FilesExamined, run.Impact.BytesParsed, run.Impact.Complete)
	if len(run.Impact.AffectedComponents) > 0 {
		fmt.Fprintf(&b, "Affected components: %s\n\n", mdText(strings.Join(run.Impact.AffectedComponents, ", ")))
	}
	if len(run.Impact.Edges) > 0 {
		b.WriteString("Observed dependency edges:\n\n| Dependent | Imports |\n|---|---|\n")
		for _, edge := range run.Impact.Edges {
			fmt.Fprintf(&b, "| `%s` | `%s` |\n", mdCode(edge.From), mdCode(edge.To))
		}
		b.WriteString("\n")
	}
	if len(run.Impact.Unsupported) > 0 {
		fmt.Fprintf(&b, "Unsupported changed code: %s\n\n", mdText(strings.Join(run.Impact.Unsupported, ", ")))
	}
	if run.Impact.LimitReached != "" {
		fmt.Fprintf(&b, "Analysis limit reached: `%s`\n\n", mdCode(run.Impact.LimitReached))
	}
	b.WriteString("## Findings\n\n")
	if len(run.Findings) == 0 {
		b.WriteString("No deterministic findings.\n\n")
	} else {
		b.WriteString("| Severity | Finding | Location |\n|---|---|---|\n")
		for _, finding := range run.Findings {
			location := finding.Path
			if finding.Line > 0 {
				location = fmt.Sprintf("%s:%d", location, finding.Line)
			}
			fmt.Fprintf(&b, "| %s | %s — %s | `%s` |\n", mdText(strings.ToUpper(finding.Severity)), mdText(finding.ID), mdText(finding.Title), mdCode(location))
		}
		b.WriteString("\n")
	}
	b.WriteString("## Reproducibility\n\n")
	fmt.Fprintf(&b, "Bundle ID: `%s`\n\n", mdCode(hash))
	b.WriteString("The bundle ID is SHA-256 of the canonical manifest excluding only its own identifier. Hashes detect changes after capture; they do not prove authenticity, authorship, completeness, correctness, or safety.\n\n")
	b.WriteString("---\n\nVerified with AgentProof\n")
	return []byte(b.String())
}

func HTML(run evidence.Run, hash string) ([]byte, error) {
	const page = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'"><meta name="viewport" content="width=device-width,initial-scale=1"><title>AgentProof verification</title><style>:root{color-scheme:light dark;--ok:#1f9d55;--warn:#d97706;--bad:#dc2626}body{font:15px/1.5 ui-sans-serif,system-ui;max-width:1080px;margin:0 auto;padding:40px 24px}h1{font-size:30px}.hero,.card{border:1px solid #8885;border-radius:14px;padding:20px;margin:18px 0}.passed{border-left:6px solid var(--ok)}.warning{border-left:6px solid var(--warn)}.failed{border-left:6px solid var(--bad)}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.metric{background:#8881;border-radius:10px;padding:14px}.metric strong{display:block;font-size:24px}table{width:100%;border-collapse:collapse}th,td{text-align:left;padding:9px;border-bottom:1px solid #8884}code{overflow-wrap:anywhere}.footer{opacity:.7;margin-top:28px}</style></head><body><section class="hero {{.Run.Status}}"><h1>AgentProof verification</h1><h2>{{.Status}}</h2><p>{{.Run.Objective}}</p></section><section class="grid"><div class="metric"><strong>{{len .Run.Repository.Changes}}</strong>files changed</div><div class="metric"><strong>{{.Added}} / {{.Deleted}}</strong>lines added / deleted</div><div class="metric"><strong>{{len .Run.Impact.AffectedComponents}}</strong>components affected</div><div class="metric"><strong>{{printf "%.2f" .Run.Completeness.Percent}}%</strong>evidence complete</div></section><section class="card"><h2>Verification</h2><p><b>Test evidence:</b> {{.Run.Tests.Summary}}</p><p><b>Manifest integrity:</b> {{.Run.Integrity}}</p><p><b>Findings:</b> {{len .Run.Findings}}</p><p><b>Git association:</b> {{.Run.Repository.AssociationStatus}}</p><p><b>Impact analysis complete:</b> {{.Run.Impact.Complete}}</p></section><section class="card"><h2>Changes</h2><table><thead><tr><th>File</th><th>Status</th><th>+</th><th>-</th></tr></thead><tbody>{{range .Run.Repository.Changes}}<tr><td><code>{{.Path}}</code></td><td>{{.Status}}</td><td>{{.Added}}</td><td>{{.Deleted}}</td></tr>{{else}}<tr><td colspan="4">No changes detected</td></tr>{{end}}</tbody></table></section><section class="card"><h2>Findings</h2><table><thead><tr><th>Severity</th><th>Finding</th><th>Path</th></tr></thead><tbody>{{range .Run.Findings}}<tr><td>{{.Severity}}</td><td>{{.ID}} — {{.Title}}</td><td><code>{{.Path}}</code></td></tr>{{else}}<tr><td colspan="3">No deterministic findings</td></tr>{{end}}</tbody></table></section><section class="card"><h2>Evidence</h2><p>Bundle ID: <code>{{.Hash}}</code></p><p>Agent: <code>{{.Run.Agent}}</code> · Model: <code>{{.Run.Model}}</code></p><p>Hashes detect post-capture changes; they do not prove authenticity, authorship, completeness, correctness, or safety.</p></section><p class="footer">Verified with AgentProof</p></body></html>`
	tmpl, err := template.New("report").Parse(page)
	if err != nil {
		return nil, err
	}
	data := struct {
		Run     evidence.Run
		Hash    string
		Status  string
		Added   int
		Deleted int
	}{run, hash, strings.ToUpper(run.Status), additions(run.Repository.Changes), deletions(run.Repository.Changes)}
	var b bytes.Buffer
	if err := tmpl.Execute(&b, data); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func additions(changes []evidence.Change) int {
	total := 0
	for _, change := range changes {
		total += change.Added
	}
	return total
}

func deletions(changes []evidence.Change) int {
	total := 0
	for _, change := range changes {
		total += change.Deleted
	}
	return total
}

func countSeverity(findings []evidence.Finding, severity string) int {
	total := 0
	for _, finding := range findings {
		if finding.Severity == severity {
			total++
		}
	}
	return total
}

func duration(ms int64) string {
	return fmt.Sprintf("%.2fs", float64(ms)/1000)
}

func short(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

func safe(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

// Every character CommonMark can activate inline. Report values originate in
// the repository and agent under observation, so unescaped prose would let
// them forge links, raw HTML, and emphasis inside a reviewer-facing document.
const mdActive = "\\`*_[]()#<>&|"

func mdText(value string) string {
	value = sanitize(value)
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if strings.ContainsRune(mdActive, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// Code spans deliberately skip mdActive: backslashes are literal inside
// backticks, so escaping there would corrupt every path and hash the report
// shows. Only the span delimiter and the table cell delimiter need handling.
func mdCode(value string) string {
	value = strings.ReplaceAll(sanitize(value), "`", "'")
	return strings.TrimSpace(strings.ReplaceAll(value, "|", "\\|"))
}

func sanitize(value string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f:
			return -1
		case r == 0x202a || r == 0x202b || r == 0x202d || r == 0x202e || r == 0x202c || r == 0x2066 || r == 0x2067 || r == 0x2068 || r == 0x2069:
			return -1
		default:
			return r
		}
	}, value)
}
