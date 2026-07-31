package impact

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ralabarta/agentproof/internal/evidence"
)

func TestAnalyzeFindsReverseDependency(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module example.test/project\n\ngo 1.22\n")
	write(t, filepath.Join(root, "internal", "auth", "auth.go"), "package auth\n")
	write(t, filepath.Join(root, "internal", "api", "api.go"), "package api\nimport _ \"example.test/project/internal/auth\"\n")
	result := Analyze(root, []evidence.Change{{Path: "internal/auth/auth.go"}})
	if result.Radius != 1 {
		t.Fatalf("expected radius 1, got %d", result.Radius)
	}
	if !contains(result.AffectedComponents, "internal/api") {
		t.Fatalf("expected internal/api in affected components: %#v", result.AffectedComponents)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
