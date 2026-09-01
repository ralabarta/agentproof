package impact

import (
	"bufio"
	"errors"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ralabarta/agentproof/internal/evidence"
)

func Analyze(root string, changes []evidence.Change) evidence.Impact {
	changed := map[string]bool{}
	var unsupported []string
	for _, change := range changes {
		changed[component(change.Path)] = true
		if unsupportedCode(change.Path) {
			unsupported = append(unsupported, filepath.ToSlash(change.Path))
		}
	}
	graph := buildGraph(root)
	edges := graph.edges
	reverse := map[string][]string{}
	for _, edge := range edges {
		reverse[edge.To] = append(reverse[edge.To], edge.From)
	}
	affected := map[string]int{}
	queue := make([]string, 0, len(changed))
	for item := range changed {
		affected[item] = 0
		queue = append(queue, item)
	}
	radius := 0
	limitReached := graph.limitReached
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, dependent := range reverse[current] {
			if _, seen := affected[dependent]; seen {
				continue
			}
			depth := affected[current] + 1
			if depth > 5 {
				if limitReached == "" {
					limitReached = "graph-depth:5"
				}
				continue
			}
			affected[dependent] = depth
			if affected[dependent] > radius {
				radius = affected[dependent]
			}
			queue = append(queue, dependent)
		}
	}
	result := evidence.Impact{
		Edges: edges, Radius: radius, Analyzer: "go/parser@stdlib+ts-js-py/regex-imports@v1+path-graph/v1",
		Complete: limitReached == "" && len(graph.unknown) == 0, Unsupported: unsupported,
		Unknown: graph.unknown, FilesExamined: graph.filesExamined, BytesParsed: graph.bytesParsed,
		LimitReached: limitReached,
	}
	for item := range changed {
		result.ChangedComponents = append(result.ChangedComponents, item)
	}
	for item := range affected {
		result.AffectedComponents = append(result.AffectedComponents, item)
	}
	sort.Strings(result.ChangedComponents)
	sort.Strings(result.AffectedComponents)
	sort.Strings(result.Unsupported)
	sort.Strings(result.Unknown)
	return result
}

func component(path string) string {
	path = filepath.ToSlash(filepath.Clean(path))
	dir := filepath.ToSlash(filepath.Dir(path))
	if dir == "." {
		return "root"
	}
	parts := strings.Split(dir, "/")
	if len(parts) > 2 {
		return strings.Join(parts[:2], "/")
	}
	return dir
}

type graphResult struct {
	edges         []evidence.Edge
	unknown       []string
	filesExamined int
	bytesParsed   int64
	limitReached  string
}

var errSourceTooLarge = errors.New("source exceeds analysis limit")

// buildGraph walks the repository once, indexing first-party source files, then
// derives component edges from the imports of every supported language. Go uses
// the standard-library parser; TypeScript, JavaScript, and Python use bounded
// regex extraction resolved against the file index.
func buildGraph(root string) graphResult {
	module := readModule(filepath.Join(root, "go.mod"))
	result := graphResult{}

	type sourceFile struct {
		rel  string
		abs  string
		kind sourceKind
	}
	var sources []sourceFile
	files := map[string]bool{}

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			result.unknown = append(result.unknown, "unreadable path: "+filepath.ToSlash(path))
			return nil
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			result.unknown = append(result.unknown, "unresolvable path: "+filepath.ToSlash(path))
			return nil
		}
		rel = filepath.ToSlash(rel)
		kind := classify(rel)
		if kind == kindOther {
			return nil
		}
		result.filesExamined++
		if result.filesExamined > 20_000 {
			result.limitReached = "files:20000"
			return fs.SkipAll
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			result.unknown = append(result.unknown, "unreadable file metadata: "+rel)
			return nil
		}
		result.bytesParsed += info.Size()
		if result.bytesParsed > 512*1024*1024 {
			result.limitReached = "parsed-bytes:536870912"
			return fs.SkipAll
		}
		files[rel] = true
		sources = append(sources, sourceFile{rel: rel, abs: path, kind: kind})
		return nil
	})

	if module == "" {
		for _, source := range sources {
			if source.kind == kindGo {
				result.unknown = append(result.unknown, "go.mod: module path unavailable")
				break
			}
		}
	}

	unique := map[string]evidence.Edge{}
	aliases := loadWebAliases(root)
	addEdge := func(fromRel, toRel string) bool {
		from, to := component(fromRel), component(toRel)
		if from == to {
			return true
		}
		unique[from+"\x00"+to] = evidence.Edge{From: from, To: to}
		if len(unique) > 1_000_000 {
			result.limitReached = "edges:1000000"
			return false
		}
		return true
	}

	for _, source := range sources {
		if result.limitReached == "edges:1000000" {
			break
		}
		switch source.kind {
		case kindGo:
			if module == "" {
				continue
			}
			file, parseErr := parser.ParseFile(token.NewFileSet(), source.abs, nil, parser.ImportsOnly)
			if parseErr != nil {
				result.unknown = append(result.unknown, "parse failed: "+source.rel)
				continue
			}
			for _, imported := range file.Imports {
				value, unquoteErr := strconv.Unquote(imported.Path.Value)
				if unquoteErr != nil || (value != module && !strings.HasPrefix(value, module+"/")) {
					continue
				}
				relImport := strings.TrimPrefix(strings.TrimPrefix(value, module), "/")
				if !addEdge(source.rel, filepath.Join(relImport, "package.go")) {
					break
				}
			}
		case kindWeb, kindPython:
			content, readErr := readBounded(source.abs)
			if errors.Is(readErr, errSourceTooLarge) {
				result.unknown = append(result.unknown, "source exceeds "+strconv.Itoa(maxSourceFileBytes)+"-byte limit: "+source.rel)
				continue
			}
			if readErr != nil {
				result.unknown = append(result.unknown, "unreadable source: "+source.rel)
				continue
			}
			specs := webSpecifiers(content)
			if source.kind == kindPython {
				specs = pythonSpecifiers(content)
			}
			for _, spec := range specs {
				resolved := resolveWeb(files, aliases, source.rel, spec)
				if source.kind == kindPython {
					resolved = resolvePython(files, source.rel, spec)
				}
				if resolved == "" {
					continue
				}
				if !addEdge(source.rel, resolved) {
					break
				}
			}
		}
	}

	result.edges = make([]evidence.Edge, 0, len(unique))
	for _, edge := range unique {
		result.edges = append(result.edges, edge)
	}
	sort.Slice(result.edges, func(i, j int) bool {
		if result.edges[i].From == result.edges[j].From {
			return result.edges[i].To < result.edges[j].To
		}
		return result.edges[i].From < result.edges[j].From
	})
	return result
}

// readBounded reads at most one byte beyond maxSourceFileBytes so oversized
// sources are rejected rather than partially analyzed.
func readBounded(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSourceFileBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxSourceFileBytes {
		return "", errSourceTooLarge
	}
	return string(data), nil
}

func unsupportedCode(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".rs", ".java", ".cs", ".rb", ".php", ".kt", ".swift", ".scala":
		return true
	default:
		return false
	}
}

func readModule(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1]
		}
	}
	return ""
}
