package scan

import (
	"bufio"
	"regexp"
	"sort"
	"strings"

	"github.com/ralabarta/agentproof/internal/evidence"
)

type secretRule struct {
	id      string
	title   string
	pattern *regexp.Regexp
}

var secretRules = []secretRule{
	{"AP-SECRET-001", "Potential AWS access key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"AP-SECRET-002", "Potential GitHub token", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9_]{30,}`)},
	{"AP-SECRET-003", "Potential private key", regexp.MustCompile(`-----BEGIN (RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY( BLOCK)?-----`)},
	{"AP-SECRET-004", "Potential hard-coded credential", regexp.MustCompile(`(?i)(api[_-]?key|password|passwd|secret|token)\s*[:=]\s*["']?[^\s"']{8,}["']?`)},
	{"AP-SECRET-005", "Potential Slack token", regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`)},
	{"AP-SECRET-006", "Potential Google API key", regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`)},
	{"AP-SECRET-007", "Potential Stripe secret key", regexp.MustCompile(`sk_(live|test)_[0-9A-Za-z]{16,}`)},
	{"AP-SECRET-008", "Potential GitHub fine-grained token", regexp.MustCompile(`github_pat_[0-9A-Za-z_]{20,}`)},
	{"AP-SECRET-009", "Potential JSON Web Token", regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`)},
}

// A header only marks where key material starts; redaction must consume the
// base64 body through the matching footer or the key survives in the patch.
var privateKeyBlock = regexp.MustCompile(`(?s)-----BEGIN [^-\n]*PRIVATE KEY( BLOCK)?-----.*?-----END [^-\n]*PRIVATE KEY( BLOCK)?-----`)

// Bound per-line scanning so a minified bundle cannot exhaust memory, while
// still being far above any realistic source line.
const maxPatchLine = 4 * 1024 * 1024

var dangerousShell = regexp.MustCompile(`(?i)^\s*(sudo\s+)?chmod\s+(-R\s+)?777\b`)
var destructiveSQL = regexp.MustCompile(`(?i)DROP\s+TABLE`)

var riskPaths = []struct {
	contains string
	id       string
	severity string
	title    string
}{
	{"auth", "AP-RISK-001", "high", "Authentication or authorization code modified"},
	{"migration", "AP-RISK-002", "high", "Database migration modified"},
	{".github/workflows", "AP-RISK-003", "medium", "CI/CD workflow modified"},
	{"go.mod", "AP-RISK-004", "medium", "Dependency or license review required"},
	{"go.sum", "AP-RISK-004", "medium", "Dependency lock file modified"},
	{"package.json", "AP-RISK-004", "medium", "Dependency or license review required"},
	{"package-lock.json", "AP-RISK-004", "medium", "Dependency lock file modified"},
	{".env", "AP-RISK-005", "high", "Environment configuration modified"},
	{"route", "AP-RISK-006", "medium", "Public routing surface may have changed"},
	{"api/", "AP-RISK-006", "medium", "Public API code may have changed"},
}

func Run(changes []evidence.Change, patch string) []evidence.Finding {
	var findings []evidence.Finding
	seen := map[string]bool{}
	for _, change := range changes {
		lower := strings.ToLower(change.Path)
		for _, rule := range riskPaths {
			if strings.Contains(lower, rule.contains) {
				key := rule.id + "\x00" + change.Path
				if !seen[key] {
					findings = append(findings, evidence.Finding{
						ID: rule.id, Severity: rule.severity, Title: rule.title, Path: change.Path,
						Source: "deterministic-path-rule", RuleVersion: "v1", Result: "violation",
						Description: "Review this change explicitly before merge.",
					})
					seen[key] = true
				}
			}
		}
	}
	findings = append(findings, scanPatch(patch)...)
	sort.SliceStable(findings, func(i, j int) bool {
		if rank(findings[i].Severity) == rank(findings[j].Severity) {
			return findings[i].ID < findings[j].ID
		}
		return rank(findings[i].Severity) > rank(findings[j].Severity)
	})
	return findings
}

func RedactPatch(patch string) (string, int) {
	redacted := patch
	count := 0
	redacted = privateKeyBlock.ReplaceAllStringFunc(redacted, func(string) string {
		count++
		return "[REDACTED:AP-SECRET-003]"
	})
	for _, rule := range secretRules {
		redacted = rule.pattern.ReplaceAllStringFunc(redacted, func(string) string {
			count++
			return "[REDACTED:" + rule.id + "]"
		})
	}
	return redacted, count
}

func RedactString(value string) string {
	redacted := privateKeyBlock.ReplaceAllString(value, "[REDACTED:AP-SECRET-003]")
	for _, rule := range secretRules {
		redacted = rule.pattern.ReplaceAllString(redacted, "[REDACTED:"+rule.id+"]")
	}
	return redacted
}

func scanPatch(patch string) []evidence.Finding {
	var findings []evidence.Finding
	scanner := bufio.NewScanner(strings.NewReader(patch))
	scanner.Buffer(make([]byte, 64*1024), maxPatchLine)
	path := ""
	newLine := 0
	hunk := regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)`)
	seen := map[string]bool{}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "+++ b/") {
			path = strings.TrimPrefix(line, "+++ b/")
			continue
		}
		if match := hunk.FindStringSubmatch(line); len(match) == 2 {
			newLine = 0
			for _, r := range match[1] {
				newLine = newLine*10 + int(r-'0')
			}
			continue
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			content := strings.TrimPrefix(line, "+")
			for _, rule := range secretRules {
				if rule.pattern.MatchString(content) {
					key := rule.id + "\x00" + path
					if !seen[key] {
						findings = append(findings, evidence.Finding{
							ID: rule.id, Severity: "critical", Title: rule.title, Path: path, Line: newLine,
							Source: "deterministic-secret-rule", RuleVersion: "v1", Result: "violation",
							Description: "A secret-like value was detected; the value was intentionally omitted from evidence.",
						})
						seen[key] = true
					}
				}
			}
			if dangerousShell.MatchString(content) || destructiveSQL.MatchString(content) {
				key := "AP-DANGER-001\x00" + path
				if !seen[key] {
					findings = append(findings, evidence.Finding{
						ID: "AP-DANGER-001", Severity: "high", Title: "Potentially destructive operation added", Path: path, Line: newLine,
						Source: "deterministic-danger-rule", RuleVersion: "v1", Result: "violation",
						Description: "Inspect the added operation and confirm that its scope is intentional.",
					})
					seen[key] = true
				}
			}
			newLine++
		} else if !strings.HasPrefix(line, "-") {
			newLine++
		}
	}
	if err := scanner.Err(); err != nil {
		// A truncated scan must be visible evidence, never a silent clean result.
		findings = append(findings, evidence.Finding{
			ID: "AP-SCAN-001", Severity: "high", Title: "Patch scan was incomplete", Path: path,
			Source: "deterministic-scan-limit", RuleVersion: "v1", Result: "unknown",
			Description: "The patch contains a line beyond the scanner limit, so later lines were not inspected. Treat secret and danger results as incomplete.",
		})
	}
	return findings
}

func rank(severity string) int {
	switch severity {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func MeetsThreshold(findings []evidence.Finding, threshold string) bool {
	minimum := rank(threshold)
	if minimum == 0 {
		return false
	}
	for _, finding := range findings {
		if rank(finding.Severity) >= minimum {
			return true
		}
	}
	return false
}
