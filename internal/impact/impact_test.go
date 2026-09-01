package impact

import (
	"os"
	"path/filepath"
	"strings"
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

func TestAnalyzeResolvesTypeScriptRelativeImports(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "src", "auth", "token.ts"), "export const token = 1;\n")
	write(t, filepath.Join(root, "src", "api", "handler.ts"), "import { token } from '../auth/token';\n")
	result := Analyze(root, []evidence.Change{{Path: "src/auth/token.ts"}})
	if !contains(result.AffectedComponents, "src/api") {
		t.Fatalf("expected src/api affected: %#v", result.AffectedComponents)
	}
	if len(result.Unsupported) != 0 {
		t.Fatalf("typescript must not be reported unsupported: %#v", result.Unsupported)
	}
}

func TestAnalyzeResolvesTypeScriptAliasAndIndexImports(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "tsconfig.json"), `{
  // path aliases
  "compilerOptions": {"baseUrl": ".", "paths": {"@/*": ["./src/*"]}}
}`)
	write(t, filepath.Join(root, "src", "auth", "index.ts"), "export const auth = 1;\n")
	write(t, filepath.Join(root, "src", "api", "route.ts"), "import { auth } from '@/auth';\n")
	result := Analyze(root, []evidence.Change{{Path: "src/auth/index.ts"}})
	if !contains(result.AffectedComponents, "src/api") {
		t.Fatalf("expected alias import resolved to src/api: %#v", result.AffectedComponents)
	}
}

func TestAnalyzeResolvesPythonImports(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "app", "auth", "__init__.py"), "")
	write(t, filepath.Join(root, "app", "auth", "tokens.py"), "TOKEN = 1\n")
	write(t, filepath.Join(root, "app", "api", "views.py"), "from app.auth.tokens import TOKEN\n")
	result := Analyze(root, []evidence.Change{{Path: "app/auth/tokens.py"}})
	if !contains(result.AffectedComponents, "app/api") {
		t.Fatalf("expected app/api affected: %#v", result.AffectedComponents)
	}
}

func TestAnalyzeIgnoresExternalDependenciesAndVendorDirs(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "src", "app.ts"), "import React from 'react';\nimport './local';\n")
	write(t, filepath.Join(root, "src", "local.ts"), "export const local = 1;\n")
	write(t, filepath.Join(root, "node_modules", "react", "index.js"), "module.exports = {};\n")
	result := Analyze(root, []evidence.Change{{Path: "src/local.ts"}})
	for _, edge := range result.Edges {
		if strings.Contains(edge.To, "node_modules") || strings.Contains(edge.From, "node_modules") {
			t.Fatalf("node_modules must not enter the graph: %#v", result.Edges)
		}
	}
	if result.FilesExamined != 2 {
		t.Fatalf("expected only the two src files examined, got %d", result.FilesExamined)
	}
}

func TestAnalyzeReportsOversizedSourceAsUnknown(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "src", "auth", "token.ts"), "export const token = 1;\n")
	write(t, filepath.Join(root, "src", "api", "route.ts"), strings.Repeat(" ", maxSourceFileBytes)+"\nimport '../auth/token';\n")

	result := Analyze(root, []evidence.Change{{Path: "src/auth/token.ts"}})

	if result.Complete {
		t.Fatalf("expected oversized source to make analysis incomplete: %#v", result)
	}
	wantUnknown := []string{"source exceeds 2097152-byte limit: src/api/route.ts"}
	if len(result.Unknown) != len(wantUnknown) || result.Unknown[0] != wantUnknown[0] {
		t.Fatalf("expected unknown %#v, got %#v", wantUnknown, result.Unknown)
	}
}

func TestAnalyzeWithoutGoSourcesReportsNoModuleUnknown(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "src", "app.py"), "VALUE = 1\n")
	result := Analyze(root, []evidence.Change{{Path: "src/app.py"}})
	for _, unknown := range result.Unknown {
		if strings.Contains(unknown, "go.mod") {
			t.Fatalf("a repository without Go sources must not report go.mod unknown: %#v", result.Unknown)
		}
	}
	if !result.Complete {
		t.Fatalf("expected complete analysis for a pure Python repository: %#v", result)
	}
}

func TestAnalyzeStillReportsTrulyUnsupportedLanguages(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "src", "main.rs"), "fn main() {}\n")
	result := Analyze(root, []evidence.Change{{Path: "src/main.rs"}})
	if !contains(result.Unsupported, "src/main.rs") {
		t.Fatalf("expected rust reported unsupported: %#v", result.Unsupported)
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
