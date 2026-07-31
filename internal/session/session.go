package session

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ralabarta/agentproof/internal/evidence"
)

func Discover(adapter string, since time.Time) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var roots []string
	switch strings.ToLower(adapter) {
	case "codex":
		roots = []string{filepath.Join(home, ".codex", "sessions")}
	case "claude", "claude-code":
		roots = []string{filepath.Join(home, ".claude", "projects")}
	default:
		return nil
	}
	var files []string
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".jsonl") {
				return nil
			}
			info, infoErr := d.Info()
			if infoErr == nil && info.ModTime().After(since.Add(-2*time.Second)) {
				files = append(files, path)
			}
			return nil
		})
	}
	sort.Strings(files)
	if len(files) > 1000 {
		files = files[:1000]
	}
	return files
}

func Summarize(adapter, path string) (evidence.Session, error) {
	result := evidence.Session{
		Adapter: adapter, File: filepath.Base(path), State: evidence.Unknown,
		ParserVersion: "heuristic-jsonl/v1",
	}
	info, err := os.Lstat(path)
	if err != nil {
		result.Reason = "cannot inspect native session artifact"
		return result, nil
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		result.Reason = "native session artifact is not a regular file"
		return result, nil
	}
	result.SourceBytes = info.Size()
	if info.Size() > 32*1024*1024 {
		result.Reason = "native session artifact exceeds 32 MiB limit"
		return result, nil
	}
	f, err := os.Open(path)
	if err != nil {
		result.Reason = "cannot read native session artifact"
		return result, nil
	}
	defer f.Close()
	h := sha256.New()
	tee := io.TeeReader(f, h)
	scanner := bufio.NewScanner(tee)
	scanner.Buffer(make([]byte, 64*1024), 32*1024*1024)
	models := map[string]bool{}
	tools := map[string]bool{}
	invalid := false
	recordCount := 0
	for scanner.Scan() {
		recordCount++
		if recordCount > 100_000 {
			result.Reason = "native session exceeds 100000 record limit"
			return result, nil
		}
		var value any
		if json.Unmarshal(scanner.Bytes(), &value) == nil {
			if !walk(value, "", 0, &result, models, tools) {
				invalid = true
			}
		} else {
			invalid = true
		}
	}
	if err := scanner.Err(); err != nil {
		result.Reason = "native session record exceeds parser limit"
		return result, nil
	}
	if invalid {
		result.Reason = "native session contains malformed or over-nested records"
		return result, nil
	}
	result.Digest = "sha256:" + hex.EncodeToString(h.Sum(nil))
	result.State = evidence.Observed
	for model := range models {
		result.Models = append(result.Models, model)
	}
	for tool := range tools {
		result.Tools = append(result.Tools, tool)
	}
	sort.Strings(result.Models)
	sort.Strings(result.Tools)
	return result, nil
}

func walk(value any, parent string, depth int, result *evidence.Session, models, tools map[string]bool) bool {
	if depth > 64 {
		return false
	}
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
			switch typed := child.(type) {
			case string:
				if normalized == "model" && len(typed) < 100 {
					models[typed] = true
				}
				if (normalized == "tool_name" || (normalized == "name" && strings.Contains(parent, "tool"))) && len(typed) < 100 {
					tools[typed] = true
				}
				if normalized == "prompt" || normalized == "user_prompt" {
					result.PromptCount++
				}
			case float64:
				applyUsage(normalized, int64(typed), &result.Usage)
			}
			if !walk(child, normalized, depth+1, result, models, tools) {
				return false
			}
		}
	case []any:
		for _, child := range v {
			if !walk(child, parent, depth+1, result, models, tools) {
				return false
			}
		}
	}
	return true
}

func applyUsage(key string, value int64, usage *evidence.Usage) {
	switch key {
	case "input_tokens", "prompt_tokens":
		if value > usage.InputTokens {
			usage.InputTokens = value
		}
	case "output_tokens", "completion_tokens":
		if value > usage.OutputTokens {
			usage.OutputTokens = value
		}
	case "cached_input_tokens", "cache_read_input_tokens", "cached_tokens":
		if value > usage.CachedTokens {
			usage.CachedTokens = value
		}
	}
}
