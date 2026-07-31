package impact

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const maxSourceFileBytes = 2 << 20

type sourceKind uint8

const (
	kindOther sourceKind = iota
	kindGo
	kindWeb
	kindPython
)

var (
	webFromImport = regexp.MustCompile(`\bfrom\s*['"]([^'"\n]+)['"]`)
	webBareImport = regexp.MustCompile(`(?m)^\s*(?:import|export)\s*['"]([^'"\n]+)['"]`)
	webCallImport = regexp.MustCompile(`\b(?:require|import)\s*\(\s*['"]([^'"\n]+)['"]`)
	pythonImport  = regexp.MustCompile(`(?m)^[ \t]*import[ \t]+([^\n#;]+)`)
	pythonFrom    = regexp.MustCompile(`(?m)^[ \t]*from[ \t]+(\.*[\w.]*)[ \t]+import\b`)
	jsoncTrailing = regexp.MustCompile(`,(\s*[}\]])`)
)

var skippedDirs = map[string]bool{
	".agentproof": true, ".git": true, ".mypy_cache": true, ".next": true,
	".nuxt": true, ".pytest_cache": true, ".ruff_cache": true, ".svelte-kit": true,
	".tox": true, ".venv": true, "__pycache__": true, "build": true,
	"coverage": true, "dist": true, "node_modules": true, "out": true,
	"site-packages": true, "target": true, "vendor": true, "venv": true,
}

func skipDir(name string) bool { return skippedDirs[name] }

func classify(rel string) sourceKind {
	lower := strings.ToLower(rel)
	if strings.HasSuffix(lower, ".d.ts") {
		return kindWeb
	}
	switch path.Ext(lower) {
	case ".go":
		return kindGo
	case ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs":
		return kindWeb
	case ".py", ".pyi":
		return kindPython
	}
	return kindOther
}

// webSpecifiers and pythonSpecifiers are heuristic: they match import syntax
// lexically, so a specifier inside a string literal or comment can be picked up.
// Resolution against the on-disk file index discards anything that is not a real
// first-party file, which keeps false positives out of the emitted graph.
func webSpecifiers(content string) []string {
	var specs []string
	for _, pattern := range []*regexp.Regexp{webFromImport, webBareImport, webCallImport} {
		for _, match := range pattern.FindAllStringSubmatch(content, -1) {
			specs = append(specs, match[1])
		}
	}
	return specs
}

func pythonSpecifiers(content string) []string {
	var specs []string
	for _, match := range pythonImport.FindAllStringSubmatch(content, -1) {
		for _, part := range strings.Split(match[1], ",") {
			part = strings.TrimSpace(part)
			if index := strings.Index(part, " as "); index >= 0 {
				part = strings.TrimSpace(part[:index])
			}
			if part != "" {
				specs = append(specs, part)
			}
		}
	}
	for _, match := range pythonFrom.FindAllStringSubmatch(content, -1) {
		specs = append(specs, match[1])
	}
	return specs
}

// resolveWeb maps a TypeScript/JavaScript specifier to a repository-relative
// file. Bare specifiers stay unresolved unless a tsconfig alias or baseUrl maps
// them into the repository, so npm dependencies never enter the graph.
func resolveWeb(files map[string]bool, aliases *webAliases, fromRel, spec string) string {
	if strings.HasPrefix(spec, ".") {
		return webCandidate(files, path.Join(path.Dir(fromRel), spec))
	}
	for _, rule := range aliases.rules {
		if rule.wildcard {
			if !strings.HasPrefix(spec, rule.from) {
				continue
			}
			if resolved := webCandidate(files, path.Join(rule.to, strings.TrimPrefix(spec, rule.from))); resolved != "" {
				return resolved
			}
			continue
		}
		if spec == rule.from {
			if resolved := webCandidate(files, rule.to); resolved != "" {
				return resolved
			}
		}
	}
	if aliases.baseURL != "" {
		return webCandidate(files, path.Join(aliases.baseURL, spec))
	}
	return ""
}

func webCandidate(files map[string]bool, base string) string {
	base = path.Clean(base)
	if base == "." || base == "/" || strings.HasPrefix(base, "..") {
		return ""
	}
	if files[base] && classify(base) == kindWeb {
		return base
	}
	// An ESM specifier may carry a .js extension that resolves to a .ts source.
	trimmed := base
	for _, ext := range []string{".js", ".mjs", ".cjs", ".jsx"} {
		if strings.HasSuffix(base, ext) {
			trimmed = strings.TrimSuffix(base, ext)
			break
		}
	}
	for _, suffix := range []string{
		".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs", ".d.ts",
		"/index.ts", "/index.tsx", "/index.js", "/index.jsx", "/index.mjs", "/index.cjs",
	} {
		if candidate := trimmed + suffix; files[candidate] {
			return candidate
		}
	}
	return ""
}

// resolvePython maps a module path to a repository file. Relative imports are
// resolved against the importing package; absolute ones are tried at the
// repository root and under a src/ layout.
func resolvePython(files map[string]bool, fromRel, spec string) string {
	if spec == "" {
		return ""
	}
	if strings.HasPrefix(spec, ".") {
		dots := len(spec) - len(strings.TrimLeft(spec, "."))
		base := path.Dir(fromRel)
		for i := 1; i < dots; i++ {
			base = path.Dir(base)
			if base == "." || base == "/" {
				return ""
			}
		}
		rest := strings.TrimLeft(spec, ".")
		if rest == "" {
			return pythonCandidate(files, base)
		}
		return pythonCandidate(files, path.Join(base, strings.ReplaceAll(rest, ".", "/")))
	}
	relative := strings.ReplaceAll(spec, ".", "/")
	if resolved := pythonCandidate(files, relative); resolved != "" {
		return resolved
	}
	return pythonCandidate(files, path.Join("src", relative))
}

func pythonCandidate(files map[string]bool, base string) string {
	base = path.Clean(base)
	if base == "." || base == "/" || strings.HasPrefix(base, "..") {
		return ""
	}
	for _, suffix := range []string{".py", ".pyi", "/__init__.py", "/__init__.pyi"} {
		if candidate := base + suffix; files[candidate] {
			return candidate
		}
	}
	return ""
}

type aliasRule struct {
	from     string
	to       string
	wildcard bool
}

type webAliases struct {
	baseURL string
	rules   []aliasRule
}

// loadWebAliases reads tsconfig.json or jsconfig.json path mappings. Unreadable
// or unparsable configuration yields no aliases rather than an error: alias
// support is best-effort and its absence only means fewer resolved edges.
func loadWebAliases(root string) *webAliases {
	aliases := &webAliases{}
	for _, name := range []string{"tsconfig.json", "jsconfig.json"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || len(data) > maxSourceFileBytes {
			continue
		}
		var config struct {
			CompilerOptions struct {
				BaseURL string              `json:"baseUrl"`
				Paths   map[string][]string `json:"paths"`
			} `json:"compilerOptions"`
		}
		if json.Unmarshal(data, &config) != nil {
			if json.Unmarshal(stripJSONC(data), &config) != nil {
				continue
			}
		}
		if base := normalizeRel(config.CompilerOptions.BaseURL); base != "" {
			aliases.baseURL = base
		}
		for from, targets := range config.CompilerOptions.Paths {
			for _, target := range targets {
				normalized := normalizeRel(target)
				if normalized == "" {
					continue
				}
				if strings.HasSuffix(from, "*") && strings.HasSuffix(target, "*") {
					aliases.rules = append(aliases.rules, aliasRule{
						from:     strings.TrimSuffix(from, "*"),
						to:       strings.TrimSuffix(normalized, "*"),
						wildcard: true,
					})
					continue
				}
				aliases.rules = append(aliases.rules, aliasRule{from: from, to: normalized})
			}
		}
		break
	}
	sort.Slice(aliases.rules, func(i, j int) bool {
		if len(aliases.rules[i].from) != len(aliases.rules[j].from) {
			return len(aliases.rules[i].from) > len(aliases.rules[j].from)
		}
		return aliases.rules[i].from < aliases.rules[j].from
	})
	return aliases
}

func normalizeRel(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(filepath.ToSlash(value), "./"))
	if value == "" || value == "." || path.IsAbs(value) || strings.HasPrefix(value, "..") {
		return ""
	}
	return value
}

func stripJSONC(data []byte) []byte {
	var out []byte
	inString, escaped := false, false
	for index := 0; index < len(data); index++ {
		char := data[index]
		if inString {
			out = append(out, char)
			switch {
			case escaped:
				escaped = false
			case char == '\\':
				escaped = true
			case char == '"':
				inString = false
			}
			continue
		}
		switch {
		case char == '"':
			inString = true
			out = append(out, char)
		case char == '/' && index+1 < len(data) && data[index+1] == '/':
			for index < len(data) && data[index] != '\n' {
				index++
			}
			out = append(out, '\n')
		case char == '/' && index+1 < len(data) && data[index+1] == '*':
			index += 2
			for index+1 < len(data) && !(data[index] == '*' && data[index+1] == '/') {
				index++
			}
			index++
		default:
			out = append(out, char)
		}
	}
	return jsoncTrailing.ReplaceAll(out, []byte("$1"))
}
