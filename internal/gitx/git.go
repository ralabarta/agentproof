package gitx

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ralabarta/agentproof/internal/evidence"
)

const Unborn = "UNBORN"

type Snapshot struct {
	Head   string
	Status string
}

func Root(dir string) (string, error) {
	out, err := run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("not inside a Git repository")
	}
	return filepath.Clean(strings.TrimSpace(out)), nil
}

func TakeSnapshot(root string) (Snapshot, error) {
	head, err := run(root, "rev-parse", "HEAD")
	if err != nil {
		head = Unborn
	}
	status, statusErr := run(root, "status", "--porcelain=v1", "--untracked-files=all")
	if statusErr != nil {
		return Snapshot{}, statusErr
	}
	return Snapshot{Head: strings.TrimSpace(head), Status: status}, nil
}

func Collect(root string, start, end Snapshot) (evidence.Repository, string, error) {
	committedChanges, workingChanges, changes, uncaptured, err := detailedChanges(root, start.Head, end.Head)
	if err != nil {
		return evidence.Repository{}, "", err
	}
	commits, err := commits(root, start.Head, end.Head)
	if err != nil {
		return evidence.Repository{}, "", err
	}
	patch, _ := patch(root, start.Head, end.Head)
	association := "clean-baseline"
	if strings.TrimSpace(start.Status) != "" {
		association = "contaminated-baseline"
	}
	if len(uncaptured) > 0 {
		association = "unknown-uncaptured-worktree"
	}
	return evidence.Repository{
		Root:              ".",
		StartHead:         start.Head,
		EndHead:           end.Head,
		DirtyBefore:       strings.TrimSpace(start.Status) != "",
		DirtyAfter:        strings.TrimSpace(end.Status) != "",
		AssociationStatus: association,
		Changes:           changes,
		CommittedChanges:  committedChanges,
		WorkingChanges:    workingChanges,
		UncapturedPaths:   uncaptured,
		Commits:           commits,
	}, patch, nil
}

func detailedChanges(root, startHead, endHead string) ([]evidence.Change, []evidence.Change, []evidence.Change, []string, error) {
	committed := map[string]evidence.Change{}
	working := map[string]evidence.Change{}
	if startHead != Unborn && startHead != "" && startHead != endHead {
		rangeRef := startHead + ".." + endHead
		if err := mergeNumstat(root, committed, "diff", "--numstat", "--no-renames", rangeRef); err != nil {
			return nil, nil, nil, nil, err
		}
		if err := mergeNameStatus(root, committed, "diff", "--name-status", "--no-renames", rangeRef); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	workingArgs := []string{"diff", "--numstat", "--no-renames"}
	workingStatusArgs := []string{"diff", "--name-status", "--no-renames"}
	if endHead != Unborn && endHead != "" {
		workingArgs = append(workingArgs, endHead)
		workingStatusArgs = append(workingStatusArgs, endHead)
	}
	if err := mergeNumstat(root, working, workingArgs...); err != nil {
		return nil, nil, nil, nil, err
	}
	if err := mergeNameStatus(root, working, workingStatusArgs...); err != nil {
		return nil, nil, nil, nil, err
	}
	uncaptured := mergeUntracked(root, working)
	for path, change := range committed {
		if change.Binary {
			uncaptured = append(uncaptured, path)
		}
	}
	for path, change := range working {
		if change.Binary {
			uncaptured = append(uncaptured, path)
		}
	}
	uncaptured = uniqueStrings(uncaptured)
	combined := map[string]evidence.Change{}
	for _, group := range []map[string]evidence.Change{committed, working} {
		for path, change := range group {
			existing := combined[path]
			existing.Path = path
			existing.Added += change.Added
			existing.Deleted += change.Deleted
			existing.Binary = existing.Binary || change.Binary
			existing.Status = change.Status
			if existing.Added > change.Added || existing.Deleted > change.Deleted {
				existing.Status = "modified"
			}
			combined[path] = existing
		}
	}
	return sortedChanges(committed), sortedChanges(working), sortedChanges(combined), uncaptured, nil
}

func mergeUntracked(root string, target map[string]evidence.Change) []string {
	var uncaptured []string
	status, _ := run(root, "status", "--porcelain=v1", "--untracked-files=all")
	for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
		if !strings.HasPrefix(line, "??") || len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		change := evidence.Change{Path: path, Status: "untracked"}
		if count, ok := lineCount(filepath.Join(root, path)); ok {
			change.Added = count
		} else {
			change.Binary = true
			uncaptured = append(uncaptured, path)
		}
		target[path] = change
	}
	return uncaptured
}

func sortedChanges(values map[string]evidence.Change) []evidence.Change {
	result := make([]evidence.Change, 0, len(values))
	for _, change := range values {
		if strings.HasPrefix(filepath.ToSlash(change.Path), ".agentproof/") || isGenerated(change.Path) {
			continue
		}
		result = append(result, change)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

func CompareBase(root, base string) (evidence.Repository, string, error) {
	end, err := TakeSnapshot(root)
	if err != nil {
		return evidence.Repository{}, "", err
	}
	startHead := base
	if end.Head != Unborn {
		if mergeBase, mergeErr := run(root, "merge-base", base, end.Head); mergeErr == nil {
			startHead = strings.TrimSpace(mergeBase)
		}
	}
	start := Snapshot{Head: startHead}
	return Collect(root, start, end)
}

func changes(root, startHead, endHead string) ([]evidence.Change, error) {
	byPath := map[string]evidence.Change{}
	if startHead != Unborn && startHead != "" && startHead != endHead {
		rangeRef := startHead + ".." + endHead
		if err := mergeNumstat(root, byPath, "diff", "--numstat", "--no-renames", rangeRef); err != nil {
			return nil, err
		}
		if err := mergeNameStatus(root, byPath, "diff", "--name-status", "--no-renames", rangeRef); err != nil {
			return nil, err
		}
	} else if endHead != Unborn && endHead != "" {
		if err := mergeNumstat(root, byPath, "diff", "--numstat", "--no-renames", endHead); err != nil {
			return nil, err
		}
		if err := mergeNameStatus(root, byPath, "diff", "--name-status", "--no-renames", endHead); err != nil {
			return nil, err
		}
	} else {
		if err := mergeNumstat(root, byPath, "diff", "--numstat", "--no-renames"); err != nil {
			return nil, err
		}
		if err := mergeNameStatus(root, byPath, "diff", "--name-status", "--no-renames"); err != nil {
			return nil, err
		}
	}
	if startHead != Unborn && startHead != "" && startHead != endHead && endHead != Unborn {
		if err := mergeNumstat(root, byPath, "diff", "--numstat", "--no-renames", endHead); err != nil {
			return nil, err
		}
		if err := mergeNameStatus(root, byPath, "diff", "--name-status", "--no-renames", endHead); err != nil {
			return nil, err
		}
	}
	status, _ := run(root, "status", "--porcelain=v1", "--untracked-files=all")
	for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if strings.Contains(path, " -> ") {
			path = strings.SplitN(path, " -> ", 2)[1]
		}
		change := byPath[path]
		change.Path = path
		if strings.HasPrefix(line, "??") {
			change.Status = "untracked"
			if count, ok := lineCount(filepath.Join(root, path)); ok {
				change.Added = count
			}
		} else if strings.Contains(line[:2], "A") {
			change.Status = "added"
		} else if strings.Contains(line[:2], "D") {
			change.Status = "deleted"
		} else if change.Status == "" {
			change.Status = "modified"
		}
		byPath[path] = change
	}
	result := make([]evidence.Change, 0, len(byPath))
	for _, change := range byPath {
		if strings.HasPrefix(filepath.ToSlash(change.Path), ".agentproof/") || isGenerated(change.Path) {
			continue
		}
		result = append(result, change)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

func mergeNumstat(root string, byPath map[string]evidence.Change, args ...string) error {
	out, err := run(root, args...)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		added, _ := strconv.Atoi(parts[0])
		deleted, _ := strconv.Atoi(parts[1])
		change := byPath[parts[2]]
		change.Path = parts[2]
		change.Status = "modified"
		change.Added += added
		change.Deleted += deleted
		change.Binary = change.Binary || parts[0] == "-" || parts[1] == "-"
		byPath[parts[2]] = change
	}
	return nil
}

func mergeNameStatus(root string, byPath map[string]evidence.Change, args ...string) error {
	out, err := run(root, args...)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		change := byPath[parts[1]]
		change.Path = parts[1]
		switch strings.TrimSpace(parts[0]) {
		case "A":
			change.Status = "added"
		case "D":
			change.Status = "deleted"
		default:
			change.Status = "modified"
		}
		byPath[parts[1]] = change
	}
	return nil
}

func lineCount(path string) (int, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 2*1024*1024 {
		return 0, false
	}
	b, err := os.ReadFile(path)
	if err != nil || bytes.IndexByte(b, 0) >= 0 {
		return 0, false
	}
	count := bytes.Count(b, []byte{'\n'})
	if len(b) > 0 && b[len(b)-1] != '\n' {
		count++
	}
	return count, true
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = filepath.ToSlash(value)
		if value == "" || seen[value] || strings.HasPrefix(value, ".agentproof/") {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func commits(root, startHead, endHead string) ([]evidence.Commit, error) {
	if startHead == Unborn || endHead == Unborn || startHead == endHead || startHead == "" {
		return []evidence.Commit{}, nil
	}
	out, err := run(root, "log", "--format=%H%x09%s", startHead+".."+endHead)
	if err != nil {
		return nil, err
	}
	var result []evidence.Commit
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			result = append(result, evidence.Commit{Hash: parts[0], Summary: parts[1]})
		}
	}
	return result, nil
}

func patch(root, startHead, endHead string) (string, error) {
	args := []string{"diff", "--no-ext-diff", "--no-renames", "--unified=0"}
	if startHead != Unborn && startHead != "" && startHead != endHead {
		args = append(args, startHead+".."+endHead)
	} else if endHead != Unborn && endHead != "" {
		args = append(args, endHead)
	}
	result, err := run(root, args...)
	if err != nil {
		return result, err
	}
	if startHead != Unborn && startHead != "" && startHead != endHead && endHead != Unborn {
		working, workingErr := run(root, "diff", "--no-ext-diff", "--no-renames", "--unified=0", endHead)
		if workingErr != nil {
			return result, workingErr
		}
		result += working
	}
	status, _ := run(root, "status", "--porcelain=v1", "--untracked-files=all")
	for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
		if !strings.HasPrefix(line, "??") || len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if strings.HasPrefix(filepath.ToSlash(path), ".agentproof/") {
			continue
		}
		if _, ok := lineCount(filepath.Join(root, path)); !ok {
			continue
		}
		untracked, _ := runDiff(root, "diff", "--no-index", "--no-ext-diff", "--unified=0", "--", os.DevNull, path)
		result += untracked
	}
	return result, nil
}

func isGenerated(path string) bool {
	base := filepath.ToSlash(path)
	return base == ".agentproof/manifest.json" || base == ".agentproof/evidence.json" || base == ".agentproof/attestation.json" || base == ".agentproof/report.md" || base == ".agentproof/report.html"
}

func run(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), errors.New(strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func runDiff(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return stdout.String(), nil
	}
	if err != nil {
		return stdout.String(), errors.New(strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
