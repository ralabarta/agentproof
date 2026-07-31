package verify

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ralabarta/agentproof/internal/config"
)

func TestVerifyGitRangeEndToEnd(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.email", "test@agentproof.dev")
	runGit(t, root, "config", "user.name", "AgentProof Test")
	writeIntegrationFile(t, filepath.Join(root, "go.mod"), "module example.test/smoke\n\ngo 1.22\n")
	writeIntegrationFile(t, filepath.Join(root, "main.go"), "package smoke\n\nfunc Value() int { return 1 }\n")
	runGit(t, root, "add", "go.mod", "main.go")
	runGit(t, root, "commit", "-m", "base")
	if err := config.Init(root, false); err != nil {
		t.Fatal(err)
	}
	writeIntegrationFile(t, filepath.Join(root, "main.go"), "package smoke\n\nfunc Value() int { return 2 }\n")
	input := filepath.Join(root, ".agentproof", "inputs", "test.jsonl")
	if err := os.MkdirAll(filepath.Dir(input), 0o700); err != nil {
		t.Fatal(err)
	}
	writeIntegrationFile(t, input, "{\"Action\":\"pass\",\"Package\":\"example.test/smoke\",\"Test\":\"TestValue\"}\n")

	first, err := Run(root, Options{Base: "HEAD", TestResults: []string{".agentproof/inputs/test.jsonl"}, RequireTests: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.ExitCode != 0 || first.Run.Status != "passed" || !first.Run.Completeness.Complete || first.BundleID == "" {
		t.Fatalf("unexpected verification result: %#v", first)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(root, ".agentproof", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["bundleId"] != first.BundleID {
		t.Fatalf("manifest bundle ID mismatch: %#v", manifest)
	}
	second, err := Run(root, Options{Base: "HEAD", TestResults: []string{".agentproof/inputs/test.jsonl"}, RequireTests: true})
	if err != nil {
		t.Fatal(err)
	}
	if second.BundleID != first.BundleID {
		t.Fatalf("verification was not deterministic: %s != %s", second.BundleID, first.BundleID)
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func writeIntegrationFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
