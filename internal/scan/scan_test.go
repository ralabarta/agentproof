package scan

import (
	"regexp"
	"strings"
	"testing"

	"github.com/ralabarta/agentproof/internal/evidence"
)

func TestRunDetectsSecretWithoutCopyingValue(t *testing.T) {
	patch := "+++ b/internal/auth/config.go\n@@ -0,0 +1 @@\n+api_key = \"123456789-super-secret\"\n"
	findings := Run([]evidence.Change{{Path: "internal/auth/config.go", Status: "modified"}}, patch)
	if len(findings) < 2 {
		t.Fatalf("expected path and secret findings, got %d", len(findings))
	}
	foundSecret := false
	for _, finding := range findings {
		if finding.ID == "AP-SECRET-004" {
			foundSecret = true
			if finding.Line != 1 {
				t.Fatalf("expected line 1, got %d", finding.Line)
			}
		}
	}
	if !foundSecret {
		t.Fatal("expected hard-coded credential finding")
	}
}

func TestMeetsThreshold(t *testing.T) {
	findings := []evidence.Finding{{Severity: "medium"}}
	if MeetsThreshold(findings, "high") {
		t.Fatal("medium finding must not meet high threshold")
	}
	if !MeetsThreshold(findings, "medium") {
		t.Fatal("medium finding must meet medium threshold")
	}
}

func TestRedactPatchRemovesSecretValue(t *testing.T) {
	patch := "+password = \"123456789-secret\"\n"
	redacted, count := RedactPatch(patch)
	if count != 1 || redacted == patch || RedactString(patch) == patch {
		t.Fatalf("redaction did not run: %q, %d", redacted, count)
	}
	if regexpForTestSecret().MatchString(redacted) {
		t.Fatalf("secret remained after redaction: %s", redacted)
	}
}

func TestDangerRuleMatchesCommandButNotItsSourceLiteral(t *testing.T) {
	command := Run(nil, "+++ b/script.sh\n@@ -0,0 +1 @@\n+chmod 777 output\n")
	if !hasFinding(command, "AP-DANGER-001") {
		t.Fatal("expected dangerous command finding")
	}
	source := Run(nil, "+++ b/rule.go\n@@ -0,0 +1 @@\n+var example = `chmod 777`\n")
	if hasFinding(source, "AP-DANGER-001") {
		t.Fatal("rule source literal should not be treated as an executed shell command")
	}
}

func hasFinding(findings []evidence.Finding, id string) bool {
	for _, finding := range findings {
		if finding.ID == id {
			return true
		}
	}
	return false
}

func regexpForTestSecret() *regexp.Regexp {
	return regexp.MustCompile(`123456789-secret`)
}

func TestRedactPatchRemovesEntirePrivateKeyBody(t *testing.T) {
	patch := "+++ b/key.pem\n@@ -0,0 +1,4 @@\n" +
		"+-----BEGIN RSA PRIVATE KEY-----\n" +
		"+MIIEowIBAAKCAQEAsecretkeymaterialthatmustnotsurvive\n" +
		"+ZZZZmoresecretkeymaterialZZZZ\n" +
		"+-----END RSA PRIVATE KEY-----\n"
	redacted, count := RedactPatch(patch)
	if count == 0 {
		t.Fatal("expected at least one redaction")
	}
	if strings.Contains(redacted, "secretkeymaterial") {
		t.Fatalf("private key body survived redaction:\n%s", redacted)
	}
}

func TestScanPatchSurvivesVeryLongAddedLine(t *testing.T) {
	long := strings.Repeat("x", 200*1024)
	patch := "+++ b/bundle.js\n@@ -0,0 +1,2 @@\n" +
		"+" + long + "\n" +
		"+password = \"" + strings.Repeat("s", 20) + "\"\n"
	findings := Run(nil, patch)
	found := false
	for _, f := range findings {
		if f.ID == "AP-SECRET-004" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a long line must not stop scanning; findings=%#v", findings)
	}
}
