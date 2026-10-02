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
	pythonFrom    = regexp.MustCompile(`(?m)^[ \t]*from[ \t]+(\.*[\w.]*)[ \t]+import[ \t]+([^\n#;]+)`)
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
		if strings.TrimLeft(match[1], ".") != "" {
			continue
		}
		for _, part := range strings.Split(match[2], ",") {
			part = strings.TrimSpace(part)
			if index := strings.Index(part, " as "); index >= 0 {
				part = strings.TrimSpace(part[:index])
			}
			if part != "" {
				specs = append(specs, match[1]+part)
			}
		}
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
		"/index.ts", "/index.tsx", "/index.mts", "/index.cts", "/index.js", "/index.jsx", "/index.mjs", "/index.cjs",
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

type aliasConfig struct {
	Extends        json.RawMessage `json:"extends"`
	CompilerOptions struct {
		BaseURL string              `json:"baseUrl"`
		Paths   map[string][]string `json:"paths"`
	} `json:"compilerOptions"`
}

// Bounds for the relative extends walk. Best-effort alias support only means
// fewer resolved edges when exceeded, never an error.
const (
	maxExtendsDepth = 16
	maxExtendsFiles = 8
)

// loadWebAliases reads tsconfig.json or jsconfig.json path mappings, walking
// relative "extends" chains (base configs first, the extending config
// overriding). Unreadable or unparsable configuration yields no aliases
// rather than an error: alias support is best-effort and its absence only
// means fewer resolved edges. Package extends targets are skipped.
func loadWebAliases(root string) *webAliases {
	for _, name := range []string{"tsconfig.json", "jsconfig.json"} {
		if aliases := loadAliasesFor(root, name); aliases != nil {
			return aliases
		}
	}
	return &webAliases{}
}

// loadAliasesFor returns nil when the root config itself is missing or
// unparsable, so the caller can fall through to the next candidate file
// (matching the pre-extends behavior). A parsable root returns aliases even
// when individual extends links had to be skipped.
func loadAliasesFor(root, name string) *webAliases {
	chain := extendsChain(root, name)
	if chain == nil {
		return nil
	}
	aliases := &webAliases{}
	// chain is root-first; apply deepest base first so each extending config
	// overrides what it inherits.
	for i := len(chain) - 1; i >= 0; i-- {
		rel := chain[i]
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || len(data) > maxSourceFileBytes {
			continue
		}
		config, ok := parseAliasConfig(data)
		if !ok {
			continue
		}
		dir := path.Dir(rel)
		if base := normalizeRel(config.CompilerOptions.BaseURL); base != "" {
			// baseUrl is relative to the config file that declares it.
			aliases.baseURL = path.Join(dir, base)
		}
		for from, targets := range config.CompilerOptions.Paths {
			for _, target := range targets {
				normalized := normalizeRel(target)
				if normalized == "" {
					continue
				}
				if aliases.baseURL != "" {
					normalized = path.Join(aliases.baseURL, normalized)
				} else {
					normalized = path.Join(dir, normalized)
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
	}
	sort.Slice(aliases.rules, func(i, j int) bool {
		if len(aliases.rules[i].from) != len(aliases.rules[j].from) {
			return len(aliases.rules[i].from) > len(aliases.rules[j].from)
		}
		return aliases.rules[i].from < aliases.rules[j].from
	})
	return aliases
}

// extendsChain returns the root-relative config files to merge, root first
// (apply in reverse so bases land first), or nil when the root file cannot be
// read or parsed. Relative parents are followed with a visited-set cycle
// guard, a depth cap, and a file-count cap; package targets and reads outside
// the root are skipped.
func extendsChain(root, name string) []string {
	seen := map[string]bool{}
	var order []string
	var visit func(current string, depth int) bool
	visit = func(current string, depth int) bool {
		cleaned := path.Clean(current)
		if strings.HasPrefix(cleaned, "..") || path.IsAbs(cleaned) {
			return depth > 0
		}
		if seen[cleaned] || depth > maxExtendsDepth || len(seen) >= maxExtendsFiles {
			return true
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(cleaned)))
		if err != nil || len(data) > maxSourceFileBytes {
			return depth > 0
		}
		config, ok := parseAliasConfig(data)
		if !ok {
			return depth > 0
		}
		seen[cleaned] = true
		order = append(order, cleaned)
		for _, parent := range extendsTargets(config.Extends) {
			visit(path.Join(path.Dir(cleaned), parent), depth+1)
		}
		return true
	}
	if !visit(name, 0) {
		return nil
	}
	return order
}

// extendsTargets returns relative parent config paths. An array of extends is
// visited last-entry-first so that, after the reverse apply order, later
// entries override earlier ones the way TypeScript does.
func extendsTargets(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return relativeExtends(single)
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		var out []string
		for index := len(many) - 1; index >= 0; index-- {
			out = append(out, relativeExtends(many[index])...)
		}
		return out
	}
	return nil
}

func relativeExtends(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || (!strings.HasPrefix(value, "./") && !strings.HasPrefix(value, "../")) {
		return nil
	}
	return []string{value}
}

func parseAliasConfig(data []byte) (aliasConfig, bool) {
	var config aliasConfig
	if json.Unmarshal(data, &config) == nil {
		return config, true
	}
	if json.Unmarshal(stripJSONC(data), &config) == nil {
		return config, true
	}
	return config, false
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
