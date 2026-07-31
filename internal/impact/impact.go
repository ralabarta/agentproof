package impact

import (
	"bufio"
	"go/parser"
	"go/token"
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
	graph := goEdges(root)
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
		Edges: edges, Radius: radius, Analyzer: "go/parser@stdlib+path-graph/v1",
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

func goEdges(root string) graphResult {
	module := readModule(filepath.Join(root, "go.mod"))
	if module == "" {
		return graphResult{unknown: []string{"go.mod: module path unavailable"}}
	}
	unique := map[string]evidence.Edge{}
	result := graphResult{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			result.unknown = append(result.unknown, "unreadable path: "+filepath.ToSlash(path))
			return nil
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == ".agentproof" || d.Name() == "vendor") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		result.filesExamined++
		if result.filesExamined > 20_000 {
			result.limitReached = "files:20000"
			return fs.SkipAll
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			result.unknown = append(result.unknown, "unreadable file metadata: "+filepath.ToSlash(path))
			return nil
		}
		result.bytesParsed += info.Size()
		if result.bytesParsed > 512*1024*1024 {
			result.limitReached = "parsed-bytes:536870912"
			return fs.SkipAll
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			rel, _ := filepath.Rel(root, path)
			result.unknown = append(result.unknown, "parse failed: "+filepath.ToSlash(rel))
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		from := component(rel)
		for _, imported := range file.Imports {
			value, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil || (value != module && !strings.HasPrefix(value, module+"/")) {
				continue
			}
			relImport := strings.TrimPrefix(strings.TrimPrefix(value, module), "/")
			to := component(filepath.Join(relImport, "package.go"))
			if from == to {
				continue
			}
			key := from + "\x00" + to
			unique[key] = evidence.Edge{From: from, To: to}
			if len(unique) > 1_000_000 {
				result.limitReached = "edges:1000000"
				return fs.SkipAll
			}
		}
		return nil
	})
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

func unsupportedCode(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".java", ".cs", ".rb", ".php":
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
