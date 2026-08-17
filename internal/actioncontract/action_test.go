package actioncontract

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestStepSummarySuccess(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, ".agentproof", "report.md"), "verification passed\n", 0o644)
	summary := filepath.Join(workspace, "summary.md")
	writeFile(t, summary, "existing\n", 0o644)

	output, err := runStepSummary(t, workspace, summary, nil)
	if err != nil {
		t.Fatalf("step summary failed: %v: %s", err, output)
	}
	assertNoReportLeak(t, output, "verification passed")
	assertFileContent(t, summary, "existing\n## AgentProof\n\nverification passed\n")
}

func TestStepSummaryPreservesReportBytes(t *testing.T) {
	tests := []struct {
		name   string
		report string
	}{
		{name: "no trailing newline", report: "verification passed"},
		{name: "multiple trailing newlines", report: "verification passed\n\n\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := t.TempDir()
			writeFile(t, filepath.Join(workspace, ".agentproof", "report.md"), tt.report, 0o644)
			summary := filepath.Join(workspace, "summary.md")

			output, err := runStepSummary(t, workspace, summary, nil)
			if err != nil {
				t.Fatalf("step summary failed: %v: %s", err, output)
			}
			assertNoReportLeak(t, output, "verification passed")
			assertFileContent(t, summary, "## AgentProof\n\n"+tt.report)
		})
	}
}

func TestStepSummaryPublishesPolicyFailureReport(t *testing.T) {
	workspace := t.TempDir()
	report := "# Verification failed\n\nPolicy threshold exceeded.\n"
	writeFile(t, filepath.Join(workspace, ".agentproof", "report.md"), report, 0o644)
	summary := filepath.Join(workspace, "summary.md")

	output, err := runStepSummary(t, workspace, summary, nil)
	if err != nil {
		t.Fatalf("step summary failed: %v: %s", err, output)
	}
	assertNoReportLeak(t, output, "Policy threshold exceeded")
	assertFileContent(t, summary, "## AgentProof\n\n"+report)
}

func TestStepSummaryMissingReportIsNoOp(t *testing.T) {
	workspace := t.TempDir()
	summary := filepath.Join(workspace, "summary.md")
	writeFile(t, summary, "existing\n", 0o644)

	output, err := runStepSummary(t, workspace, summary, nil)
	if err != nil {
		t.Fatalf("missing report should be a no-op: %v: %s", err, output)
	}
	if output != "" {
		t.Fatalf("missing report produced output: %q", output)
	}
	assertFileContent(t, summary, "existing\n")
}

func TestStepSummaryUnreadableReportFails(t *testing.T) {
	workspace := t.TempDir()
	secret := "unreadable report contents"
	writeFile(t, filepath.Join(workspace, ".agentproof", "report.md"), secret, 0o644)
	summary := filepath.Join(workspace, "summary.md")
	binDir := filepath.Join(workspace, "bin")
	writeFile(t, filepath.Join(binDir, "cat"), "#!/usr/bin/env bash\nexit 41\n", 0o755)

	output, err := runStepSummary(t, workspace, summary, []string{"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH")})
	if err == nil {
		t.Fatal("a failed report read must fail the step")
	}
	assertNoReportLeak(t, output, secret)
	if _, statErr := os.Stat(summary); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("summary should not be written after a failed read: %v", statErr)
	}
}

func TestStepSummaryRejectsFinalSymlink(t *testing.T) {
	workspace := t.TempDir()
	secret := "symlink target contents"
	target := filepath.Join(workspace, "target.md")
	writeFile(t, target, secret, 0o644)
	report := filepath.Join(workspace, ".agentproof", "report.md")
	if err := os.MkdirAll(filepath.Dir(report), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, report); err != nil {
		t.Fatal(err)
	}
	summary := filepath.Join(workspace, "summary.md")

	output, err := runStepSummary(t, workspace, summary, nil)
	if err == nil {
		t.Fatal("a final report symlink must be rejected")
	}
	assertNoReportLeak(t, output, secret)
	if _, statErr := os.Stat(summary); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("summary should not be written for a symlink report: %v", statErr)
	}
}

func TestActionPreservesVerifyExitCode(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is unavailable")
	}
	action := readAction(t)
	verify := namedStep(t, action, "Verify supplied evidence")
	for _, fragment := range []string{"verify_code=$?", "exit \"$verify_code\"", ".findings // []"} {
		if !strings.Contains(verify.run, fragment) {
			t.Fatalf("verify step must contain %q", fragment)
		}
	}

	t.Run("null findings", func(t *testing.T) {
		workspace := t.TempDir()
		writeFile(t, filepath.Join(workspace, ".agentproof", "evidence.json"), `{
  "status": "failed",
  "bundle_id": "bundle",
  "completeness": {"percent": 50},
  "integrity": "valid",
  "findings": null,
  "warning_count": 0
}
`, 0o644)
		outputPath, code, output := runVerifyStep(t, verify.run, workspace, 17)
		if code != 17 {
			t.Fatalf("null findings obscured verify exit code: got %d, want 17: %s", code, output)
		}
		contents := readFile(t, outputPath)
		if !strings.Contains(contents, "critical-violations=0\n") {
			t.Fatalf("null findings should publish zero critical violations: %q", contents)
		}
	})

	t.Run("missing evidence", func(t *testing.T) {
		workspace := t.TempDir()
		outputPath, code, output := runVerifyStep(t, verify.run, workspace, 23)
		if code != 23 {
			t.Fatalf("missing evidence obscured verify exit code: got %d, want 23: %s", code, output)
		}
		if contents := readFile(t, outputPath); contents != "evidence-valid=false\nconclusion=failed\n" {
			t.Fatalf("missing evidence must publish an explicit failure and remain invalid: %q", contents)
		}
	})
}

func TestActionRejectsUnsafeOutputEvidence(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is unavailable")
	}
	verify := namedStep(t, readAction(t), "Verify supplied evidence")
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, ".agentproof", "evidence.json"), `{
  "status": "passed\ninjected-output=true",
  "bundle_id": "bundle",
  "completeness": {"percent": 100},
  "integrity": "valid",
  "findings": [],
  "warning_count": 0
}
`, 0o644)

	outputPath, code, output := runVerifyStep(t, verify.run, workspace, 0)
	if code == 0 {
		t.Fatalf("unsafe evidence must fail output extraction: %s", output)
	}
	if contents := readFile(t, outputPath); strings.Contains(contents, "injected-output=true") {
		t.Fatalf("unsafe evidence injected a GitHub output: %q", contents)
	}
}

func TestActionMalformedOutputExtractionPreservesVerifyExitCode(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is unavailable")
	}
	verify := namedStep(t, readAction(t), "Verify supplied evidence")
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, ".agentproof", "evidence.json"), "{\n", 0o644)

	_, code, output := runVerifyStep(t, verify.run, workspace, 37)
	if code != 37 {
		t.Fatalf("malformed output extraction obscured verify exit code: got %d, want 37: %s", code, output)
	}
}

func TestActionSuccessfulVerifyFailsClosedOnMalformedEvidence(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is unavailable")
	}
	verify := namedStep(t, readAction(t), "Verify supplied evidence")
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, ".agentproof", "evidence.json"), "{\n", 0o644)

	_, code, output := runVerifyStep(t, verify.run, workspace, 0)
	if code == 0 {
		t.Fatalf("successful verification must fail closed when output extraction fails: %s", output)
	}
}

func TestActionEvidenceValidityControlsPublication(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is unavailable")
	}
	verify := namedStep(t, readAction(t), "Verify supplied evidence")
	tests := []struct {
		name       string
		evidence   string
		write      bool
		verifyCode int
		wantCode   int
		wantValid  string
	}{
		{
			name: "valid policy failure",
			evidence: `{
  "status": "failed",
  "bundle_id": "bundle",
  "completeness": {"percent": 50},
  "integrity": "valid",
  "findings": [{"severity": "critical"}],
  "warning_count": 0
}
`,
			write:      true,
			verifyCode: 17,
			wantCode:   17,
			wantValid:  "true",
		},
		{name: "malformed evidence", evidence: "{\n", write: true, verifyCode: 37, wantCode: 37, wantValid: "false"},
		{
			name: "newline evidence",
			evidence: `{
  "status": "passed\ninjected-output=true",
  "bundle_id": "bundle",
  "completeness": {"percent": 100},
  "integrity": "valid",
  "findings": [],
  "warning_count": 0
}
`,
			write: true, verifyCode: 41, wantCode: 41, wantValid: "false",
		},
		{
			name: "schema invalid evidence",
			evidence: `{
  "status": "passed",
  "bundle_id": "bundle",
  "completeness": {"percent": 101},
  "integrity": "valid",
  "findings": [],
  "warning_count": 0
}
`,
			write: true, verifyCode: 43, wantCode: 43, wantValid: "false",
		},
		{name: "missing evidence", verifyCode: 47, wantCode: 47, wantValid: "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := t.TempDir()
			if tt.write {
				writeFile(t, filepath.Join(workspace, ".agentproof", "evidence.json"), tt.evidence, 0o644)
			}
			outputPath, code, output := runVerifyStep(t, verify.run, workspace, tt.verifyCode)
			if code != tt.wantCode {
				t.Fatalf("verify exit code = %d, want %d: %s", code, tt.wantCode, output)
			}
			if contents := readFile(t, outputPath); !strings.Contains(contents, "evidence-valid="+tt.wantValid+"\n") {
				t.Fatalf("evidence-valid = %q, want %q", contents, tt.wantValid)
			}
		})
	}
}

func TestActionRunsSummaryAlways(t *testing.T) {
	action := readAction(t)
	summary := namedStep(t, action, "Step Summary")
	if summary.identifier != "summary" {
		t.Fatalf("summary id = %q, want summary", summary.identifier)
	}
	for _, fragment := range []string{"always()", "steps.verify.outputs.evidence-valid == 'true'"} {
		if !strings.Contains(summary.condition, fragment) {
			t.Fatalf("summary condition must contain %q: %q", fragment, summary.condition)
		}
	}
	if !strings.Contains(summary.run, `"${{ github.action_path }}/scripts/write-step-summary.sh"`) {
		t.Fatalf("summary step must invoke the action-owned script through github.action_path: %q", summary.run)
	}

	upload := namedStep(t, action, "Upload AgentProof reports")
	for _, fragment := range []string{"always()", "steps.verify.outputs.evidence-valid == 'true'", "steps.summary.outcome == 'success'", "hashFiles('.agentproof/report.md') != ''"} {
		if !strings.Contains(upload.condition, fragment) {
			t.Fatalf("upload condition must contain %q: %q", fragment, upload.condition)
		}
	}
}

type actionStep struct {
	identifier string
	condition  string
	run        string
}

func runStepSummary(t *testing.T, workspace, summary string, extraEnv []string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", stepSummaryScript(t))
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), "GITHUB_STEP_SUMMARY="+summary)
	cmd.Env = append(cmd.Env, extraEnv...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func runVerifyStep(t *testing.T, script, workspace string, verifyCode int) (string, int, string) {
	t.Helper()
	runnerTemp := filepath.Join(workspace, "runner")
	writeFile(t, filepath.Join(runnerTemp, "agentproof"), "#!/usr/bin/env bash\nexit \"${VERIFY_CODE:?}\"\n", 0o755)
	outputPath := filepath.Join(workspace, "github-output")
	cmd := exec.Command("bash", "--noprofile", "--norc", "-e", "-o", "pipefail", "-c", script)
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(),
		"RUNNER_TEMP="+runnerTemp,
		"GITHUB_OUTPUT="+outputPath,
		"INPUT_BASE=origin/main",
		"INPUT_TEST_RESULTS=",
		"INPUT_REQUIRE_TESTS=false",
		"INPUT_FAIL_ON=critical",
		"VERIFY_CODE="+strconv.Itoa(verifyCode),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return outputPath, 0, string(output)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("run verify step: %v: %s", err, output)
	}
	return outputPath, exitErr.ExitCode(), string(output)
}

func namedStep(t *testing.T, action, name string) actionStep {
	t.Helper()
	marker := "    - name: " + name + "\n"
	start := strings.Index(action, marker)
	if start < 0 {
		t.Fatalf("action step %q not found", name)
	}
	body := action[start+len(marker):]
	if next := strings.Index(body, "\n    - name: "); next >= 0 {
		body = body[:next+1]
	}

	var step actionStep
	lines := strings.Split(body, "\n")
	for i := 0; i < len(lines); i++ {
		switch {
		case strings.HasPrefix(lines[i], "      id: "):
			step.identifier = strings.TrimSpace(strings.TrimPrefix(lines[i], "      id: "))
		case strings.HasPrefix(lines[i], "      if: "):
			step.condition = strings.TrimSpace(strings.TrimPrefix(lines[i], "      if: "))
		case lines[i] == "      run: |":
			var run []string
			for i++; i < len(lines) && (strings.HasPrefix(lines[i], "        ") || lines[i] == ""); i++ {
				run = append(run, strings.TrimPrefix(lines[i], "        "))
			}
			step.run = strings.Join(run, "\n")
		}
	}
	return step
}

func readAction(t *testing.T) string {
	t.Helper()
	return string(mustReadFile(t, filepath.Join(repositoryRoot(t), "action.yml")))
}

func stepSummaryScript(t *testing.T) string {
	t.Helper()
	return filepath.Join(repositoryRoot(t), "scripts", "write-step-summary.sh")
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve action contract test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func writeFile(t *testing.T, path, contents string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	return string(mustReadFile(t, path))
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	if got := readFile(t, path); got != want {
		t.Fatalf("%s content = %q, want %q", path, got, want)
	}
}

func assertNoReportLeak(t *testing.T, output, secret string) {
	t.Helper()
	if strings.Contains(output, secret) {
		t.Fatalf("report contents leaked to process output: %q", output)
	}
}
