# AgentProof Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement 14 improvements spanning DX, trust model, and robustness for the AgentProof CLI.

**Architecture:** All new code goes into focused packages under `internal/`; `internal/app/app.go` is the single wiring point for new CLI commands. Zero external runtime dependencies — stdlib only. Strict TDD throughout.

**Tech Stack:** Go 1.22+, stdlib only. GoReleaser (build tool, not runtime dep).

---

## Critical Constraints

- **Zero runtime dependencies.** `go.mod` must stay dependency-free. `crypto/ed25519` is stdlib since Go 1.13.
- **Conventional commits only.** No `Co-Authored-By`. Format: `feat(pkg): description`.
- **TDD.** Write the failing test first. Run it. Implement. Run again. Commit.
- **All existing tests must pass** after every commit: `go test ./... && go vet ./...`

---

## File Structure

### New packages
| Path | Responsibility |
|---|---|
| `internal/status/status.go` | Read local state: initialized, run count, last result |
| `internal/doctor/doctor.go` | Run diagnostic checks, surface actionable failures |
| `internal/completion/completion.go` | Generate bash/zsh/fish shell completions |
| `internal/coverage/parse.go` | Parse Go coverprofile and lcov; map to changed lines |
| `internal/signing/signing.go` | Ed25519 keygen, manifest signing, verification |
| `internal/testmapping/map.go` | Heuristic: find test files for each changed source file |

### Modified files
| Path | Change |
|---|---|
| `internal/app/app.go` | Add: `status`, `runs`, `doctor`, `completion` commands; `--format` on `verify` |
| `internal/config/config.go` | Add `HighRiskPaths []string` field |
| `internal/scan/scan.go` | Accept extra high-risk paths from config |
| `internal/record/record.go` | Add lockfile guard + interrupt recovery (`state.json`) |
| `internal/purge/purge.go` | Add `--runs` flag for full run retention |
| `internal/testresult/ingest.go` | Add pytest-json-report format |
| `internal/verify/verify.go` | Thread `--format` option through |
| `internal/report/report.go` | Add JSON stdout format |
| `internal/impact/imports.go` | Add Rust, Java, Kotlin lexical extraction |
| `action.yml` | Write `$GITHUB_STEP_SUMMARY` after verify step |
| `.goreleaser.yml` | New: release config with SBOM and checksums |

---

## Phase 1: Quick Wins

### Task 1: `agentproof status` command

**Files:**
- Create: `internal/status/status.go`
- Create: `internal/status/status_test.go`
- Modify: `internal/app/app.go` (add `case "status":`)

- [ ] **Write failing test** (`internal/status/status_test.go`):

```go
package status_test

import (
    "os"
    "path/filepath"
    "testing"
    "github.com/ralabarta/agentproof/internal/status"
)

func TestRead_NotInitialized(t *testing.T) {
    dir := t.TempDir()
    s, err := status.Read(dir)
    if err != nil { t.Fatal(err) }
    if s.Initialized { t.Error("want not initialized") }
    if s.RunCount != 0 { t.Errorf("want 0 runs, got %d", s.RunCount) }
}

func TestRead_Initialized(t *testing.T) {
    dir := t.TempDir()
    if err := os.MkdirAll(filepath.Join(dir, ".agentproof", "runs"), 0o700); err != nil {
        t.Fatal(err)
    }
    // write minimal config.json
    cfg := filepath.Join(dir, ".agentproof", "config.json")
    os.WriteFile(cfg, []byte(`{"schemaVersion":"1"}`), 0o600)
    s, err := status.Read(dir)
    if err != nil { t.Fatal(err) }
    if !s.Initialized { t.Error("want initialized") }
}
```

- [ ] **Run test, verify fails:** `cd agentproof && go test ./internal/status/... 2>&1 | head -5`
  Expected: `cannot find package`

- [ ] **Wire into app.go** — add `case "status": return statusCommand(args[1:])` and implement `statusCommand` that calls `status.Read(cwd)` and prints a table.

- [ ] **Run all tests, verify pass:** `go test ./... && go vet ./...`

- [ ] **Commit:** `git commit -m "feat(status): add agentproof status and runs list commands"`

---

### Task 2: `agentproof runs` subcommand

**Files:**
- Extend `internal/status/status.go` with `ListRuns(root string) ([]RunSummary, error)`
- Modify: `internal/app/app.go` (add `case "runs":`)

- [ ] **Write failing test:**

```go
func TestListRuns_Empty(t *testing.T) {
    dir := t.TempDir()
    os.MkdirAll(filepath.Join(dir, ".agentproof", "runs"), 0o700)
    runs, err := status.ListRuns(dir)
    if err != nil { t.Fatal(err) }
    if len(runs) != 0 { t.Errorf("want 0, got %d", len(runs)) }
}
```

- [ ] **Implement** `RunSummary` struct and `ListRuns`:

```go
type RunSummary struct {
    ID        string
    Objective string
    Agent     string
    StartedAt time.Time
    State     string // "complete", "abandoned", "recording"
}
// ListRuns reads run dirs, parses state.json + run metadata per dir.
func ListRuns(root string) ([]RunSummary, error)
```

Each run dir contains `state.json` (written by record, Task 8). For now, derive State from `state.json`; fall back to `"complete"` if file is absent (pre-Task-8 runs).

- [ ] **Run tests, commit:** `feat(status): add runs list command`

---

### Task 3: `agentproof doctor` command

**Files:**
- Create: `internal/doctor/doctor.go`
- Create: `internal/doctor/doctor_test.go`
- Modify: `internal/app/app.go`

- [ ] **Write failing test:**

```go
func TestRun_NotGitRepo(t *testing.T) {
    dir := t.TempDir()
    r, err := doctor.Run(dir)
    if err != nil { t.Fatal(err) }
    found := false
    for _, f := range r.Findings {
        if f.Name == "git-repo" && f.Severity == doctor.Error { found = true }
    }
    if !found { t.Error("expected git-repo error finding") }
}
```

- [ ] **Implement** `doctor.go`:

```go
type Severity string
const (OK Severity = "ok"; Warn Severity = "warn"; Error Severity = "error")

type Finding struct {
    Name     string
    Severity Severity
    Detail   string
}
type Report struct {
    Findings []Finding
    Healthy  bool
}

func Run(cwd string) (Report, error) {
    var r Report
    // check: git repo reachable
    // check: .agentproof/config.json exists and parses
    // check: no abandoned runs
    // check: manifest integrity if evidence.json present
    r.Healthy = !hasErrors(r.Findings)
    return r, nil
}
```

Implement each check calling existing `gitx.Root`, `config.Load`, `status.ListRuns`.

- [ ] **Run tests, commit:** `feat(doctor): add agentproof doctor diagnostic command`

---

### Task 4: Shell completions

**Files:**
- Create: `internal/completion/completion.go`
- Create: `internal/completion/completion_test.go`
- Modify: `internal/app/app.go` (add `case "completion":`)

- [ ] **Write failing test:**

```go
func TestGenerate_Bash(t *testing.T) {
    var buf bytes.Buffer
    if err := completion.Generate("bash", &buf); err != nil { t.Fatal(err) }
    out := buf.String()
    if !strings.Contains(out, "agentproof") { t.Error("expected agentproof in bash completion") }
    for _, cmd := range []string{"init", "record", "verify", "purge", "status", "runs", "doctor"} {
        if !strings.Contains(out, cmd) { t.Errorf("missing command %q in bash completion", cmd) }
    }
}
```

- [ ] **Implement** — use `text/template` (stdlib) with embedded completion templates:

```go
func Generate(shell string, w io.Writer) error {
    switch shell {
    case "bash":  return tmpl(bashTmpl, w)
    case "zsh":   return tmpl(zshTmpl, w)
    case "fish":  return tmpl(fishTmpl, w)
    default:      return fmt.Errorf("unsupported shell %q; use bash, zsh, or fish", shell)
    }
}
```

Bash template: `_agentproof() { ... }; complete -F _agentproof agentproof`
Zsh template: `#compdef agentproof\n...`
Fish template: `complete -c agentproof ...`

- [ ] **Run tests, commit:** `feat(completion): add bash/zsh/fish shell completions`

---

### Task 5: GitHub Step Summary

**Files:**
- Modify: `action.yml` only

- [ ] **Edit `action.yml`** — add after the verify step, before upload:

```yaml
    - name: Write Step Summary
      shell: bash
      if: always()
      run: |
        if [ -f .agentproof/report.md ]; then
          printf '## AgentProof\n\n' >> "$GITHUB_STEP_SUMMARY"
          cat .agentproof/report.md >> "$GITHUB_STEP_SUMMARY"
        fi
```

- [ ] **Verify** the step is positioned after verify and before artifact upload.
- [ ] **Commit:** `feat(action): write GitHub Step Summary`

---

### Task 6: `--format json` and `--format quiet` on verify

**Files:**
- Modify: `internal/app/app.go` (add `--format` flag to `verifyCommand`)
- Modify: `internal/verify/verify.go` (pass format through)
- Modify: `internal/report/report.go` (add `WriteJSON(w io.Writer, result Result)`)
- Modify: `internal/report/report_test.go`

- [ ] **Write failing test** in `internal/report/report_test.go`:

```go
func TestWriteJSON(t *testing.T) {
    r := report.Result{ /* minimal */ }
    var buf bytes.Buffer
    if err := report.WriteJSON(&buf, r); err != nil { t.Fatal(err) }
    if !json.Valid(buf.Bytes()) { t.Error("WriteJSON produced invalid JSON") }
}
```

- [ ] **Implement** `report.WriteJSON`:

```go
func WriteJSON(w io.Writer, r Result) error {
    enc := json.NewEncoder(w)
    enc.SetIndent("", "  ")
    return enc.Encode(r)
}
```

- [ ] **Wire into app.go** — when `--format json`, call `report.WriteJSON(os.Stdout, result)` instead of printing the status line. When `--format quiet`, suppress all stdout.

- [ ] **Run tests, commit:** `feat(verify): add --format json and --format quiet flags`

---

## Phase 2: Medium Effort

### Task 7: Custom risk rules in config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/scan/scan.go`
- Modify: `internal/scan/scan_test.go`

- [ ] **Write failing test** in `internal/scan/scan_test.go`:

```go
func TestScan_CustomHighRiskPath(t *testing.T) {
    patch := "+++ b/billing/invoice.go\n@@ -0,0 +1 @@\n+package billing"
    result, err := scan.Run(scan.Options{Patch: patch, ExtraHighRisk: []string{"billing/"}})
    if err != nil { t.Fatal(err) }
    found := false
    for _, f := range result.Findings {
        if strings.Contains(f.Path, "billing") { found = true }
    }
    if !found { t.Error("expected billing finding") }
}
```

- [ ] **Modify `config.Config`:**

```go
type Config struct {
    SchemaVersion string   `json:"schemaVersion"`
    HighRiskPaths []string `json:"highRiskPaths,omitempty"`
}
```

- [ ] **Modify `scan.Options`** to accept `ExtraHighRisk []string`, appended to the built-in rule list.

- [ ] **Run tests, commit:** `feat(config): add custom high-risk-path rules`

---

### Task 8: Interrupted record recovery + parallel guard

**Files:**
- Modify: `internal/record/record.go`
- Modify: `internal/record/record_test.go`

State file written on record start at `<runDir>/state.json`:

```json
{"status":"recording","startedAt":"2026-08-06T17:00:00Z"}
```

On clean exit: `{"status":"complete","startedAt":"...","completedAt":"..."}`
On interrupt/signal: `{"status":"abandoned","startedAt":"...","signal":"SIGINT"}`

- [ ] **Write test for state transitions:**

```go
func TestStateFile_WrittenOnStart(t *testing.T) {
    // Use a no-op command (true / cmd /c exit 0) and verify state.json is "complete"
}
func TestStateFile_Abandoned_OnKill(t *testing.T) {
    // Harder to test directly; test state.json marshal/unmarshal round-trip
}
```

- [ ] **Implement** in `record.go`:

```go
type runState struct {
    Status      string    `json:"status"`
    StartedAt   time.Time `json:"startedAt"`
    CompletedAt time.Time `json:"completedAt,omitempty"`
    Signal      string    `json:"signal,omitempty"`
}

// Write state.json at run start (status=recording)
// Install os/signal handler for SIGINT/SIGTERM -> write abandoned + exit
// On success: write complete
```

Lockfile: create `.agentproof/.record.lock` containing PID at start; check it before starting; remove on exit. If lockfile PID is dead, proceed (stale lock).

- [ ] **Run tests, commit:** `feat(record): add state.json tracking and parallel record guard`

---

### Task 9: Coverage artifact ingestion

**Files:**
- Create: `internal/coverage/parse.go`
- Create: `internal/coverage/parse_test.go`
- Modify: `internal/app/app.go` (add `--coverage` flag to `verify`)
- Modify: `internal/verify/verify.go`

- [ ] **Write failing tests:**

```go
func TestParseGoProfile_Basic(t *testing.T) {
    input := "mode: set\ngithub.com/foo/bar/pkg.go:10.20,12.5 1 1\n"
    p, err := coverage.ParseGoProfile(strings.NewReader(input))
    if err != nil { t.Fatal(err) }
    if len(p.Lines) == 0 { t.Error("expected lines") }
}

func TestParseLCOV_Basic(t *testing.T) {
    input := "SF:pkg/foo.go\nDA:5,1\nDA:6,0\nend_of_record\n"
    p, err := coverage.ParseLCOV(strings.NewReader(input))
    if err != nil { t.Fatal(err) }
    if len(p.Lines) == 0 { t.Error("expected lines") }
}
```

- [ ] **Implement** `internal/coverage/parse.go`:

```go
type Line struct { File string; LineNo int; Hits int }
type Profile struct { Lines []Line }

// ParseGoProfile parses `go test -coverprofile` output.
// Format: "mode: set\n<file>:<startline>.<startcol>,<endline>.<endcol> <stmts> <count>\n"
func ParseGoProfile(r io.Reader) (Profile, error) { ... }

// ParseLCOV parses lcov .info format: SF:/path\nDA:<line>,<hits>\nend_of_record
func ParseLCOV(r io.Reader) (Profile, error) { ... }

// CoverageForPatch returns per-file coverage of the lines added in a unified diff.
func CoverageForPatch(p Profile, unifiedDiff string) []FileCoverage { ... }
```

Add `--coverage <path>` to verify; ingest and include in evidence output.

- [ ] **Run tests, commit:** `feat(coverage): add Go coverprofile and lcov ingestion`

---

### Task 10: Run retention policy (`purge --runs`)

**Files:**
- Modify: `internal/purge/purge.go`
- Modify: `internal/purge/purge_test.go`
- Modify: `internal/app/app.go` (add `--runs` flag to purge command)

- [ ] **Write failing test:**

```go
func TestPurge_Runs_OlderThan(t *testing.T) {
    dir := t.TempDir()
    // Create 2 run dirs: one old (mtime -10d), one new (mtime -1h)
    // Purge --runs --older-than 48h --confirm
    // Expect: old dir deleted, new dir kept
}
```

- [ ] **Implement** in `purge.go` — add `Runs bool` to `Options`; when set, scan `<root>/.agentproof/runs/` for dirs older than `OlderThan`; preview or delete.

- [ ] **Run tests, commit:** `feat(purge): add run retention policy with --runs flag`

---

### Task 11: Pytest-json-report format

**Files:**
- Modify: `internal/testresult/ingest.go`
- Modify: `internal/testresult/ingest_test.go`

- [ ] **Write failing test:**

```go
func TestIngest_PytestJSONReport(t *testing.T) {
    data := `{"tests":[
        {"nodeid":"test_foo.py::test_bar","outcome":"passed","duration":0.1},
        {"nodeid":"test_foo.py::test_baz","outcome":"failed","duration":0.2}
    ],"summary":{"passed":1,"failed":1}}`
    result, err := testresult.IngestJSON(strings.NewReader(data))
    if err != nil { t.Fatal(err) }
    if result.Passed != 1 || result.Failed != 1 { t.Errorf("got %+v", result) }
}
```

- [ ] **Implement** `IngestJSON` — detect pytest-json-report by presence of `"tests"` + `"summary"` keys, parse outcomes.

- [ ] **Run tests, commit:** `feat(testresult): add pytest-json-report ingestion`

---

## Phase 3: Trust Model

### Task 12: Ed25519 signing

**Files:**
- Create: `internal/signing/signing.go`
- Create: `internal/signing/signing_test.go`
- Modify: `internal/app/app.go` (add `agentproof sign` command + `--verify-signature` to verify)

- [ ] **Write failing test:**

```go
func TestSignVerify_RoundTrip(t *testing.T) {
    priv, pub, err := signing.GenerateKeyPair()
    if err != nil { t.Fatal(err) }
    data := []byte("bundle-id-abc123")
    sig, err := signing.Sign(priv, data)
    if err != nil { t.Fatal(err) }
    if !signing.Verify(pub, data, sig) { t.Error("signature verification failed") }
}

func TestVerify_WrongKey(t *testing.T) {
    _, pub, _ := signing.GenerateKeyPair()
    _, wrongPub, _ := signing.GenerateKeyPair()
    sig, _ := signing.Sign(mustPriv(t), []byte("data"))
    if signing.Verify(wrongPub, []byte("data"), sig) { t.Error("should fail with wrong key") }
    _ = pub
}
```

- [ ] **Implement** using `crypto/ed25519` + `encoding/pem` + `crypto/x509`:

```go
import "crypto/ed25519"

func GenerateKeyPair() (ed25519.PrivateKey, ed25519.PublicKey, error) {
    pub, priv, err := ed25519.GenerateKey(rand.Reader)
    return priv, pub, err
}
func Sign(priv ed25519.PrivateKey, data []byte) ([]byte, error) {
    return ed25519.Sign(priv, data), nil
}
func Verify(pub ed25519.PublicKey, data, sig []byte) bool {
    return ed25519.Verify(pub, data, sig)
}
// SavePrivateKey / SavePublicKey: write PEM files to .agentproof/signing/
// LoadPrivateKey / LoadPublicKey: read PEM files
```

- [ ] **Wire**: `agentproof sign --bundle-id <id>` signs manifest; `agentproof verify --verify-signature` checks it.
- [ ] **Run tests, commit:** `feat(signing): add Ed25519 manifest signing`

---

### Task 13: Test-to-change mapping

**Files:**
- Create: `internal/testmapping/map.go`
- Create: `internal/testmapping/map_test.go`
- Modify: `internal/verify/verify.go` (include mapping in evidence output)

- [ ] **Write failing test:**

```go
func TestMap_GoFile(t *testing.T) {
    dir := t.TempDir()
    os.WriteFile(filepath.Join(dir, "foo.go"), []byte("package p"), 0o600)
    os.WriteFile(filepath.Join(dir, "foo_test.go"), []byte("package p"), 0o600)
    m, err := testmapping.Map(dir, []string{"foo.go"})
    if err != nil { t.Fatal(err) }
    if len(m) == 0 || len(m[0].TestFiles) == 0 { t.Error("expected foo_test.go in mapping") }
}
```

- [ ] **Implement** heuristics per language:
  - Go: same dir, `<base>_test.go` or any `*_test.go`
  - TS/JS: same dir, `<base>.test.ts`, `<base>.spec.ts`, `__tests__/<base>.ts`
  - Python: same dir, `test_<base>.py`, `<base>_test.py`

```go
type Mapping struct {
    File      string
    TestFiles []string
    Lang      string // "go", "ts", "py", "unknown"
}
func Map(root string, changedFiles []string) ([]Mapping, error)
```

- [ ] **Run tests, commit:** `feat(testmapping): add test-to-change mapping`

---

### Task 14: Language adapters — Rust, Java, Kotlin

**Files:**
- Modify: `internal/impact/imports.go` (add `extractRustImports`, `extractJavaImports`, `extractKotlinImports`)
- Modify: `internal/impact/impact.go` (register new extractors)
- Modify: `internal/impact/impact_test.go`

- [ ] **Write failing tests:**

```go
func TestExtractRustImports(t *testing.T) {
    src := []byte("use std::collections::HashMap;\nuse crate::auth::token;")
    got := impact.ExtractRustImports(src)
    if !slices.Contains(got, "crate::auth::token") { t.Error("missing crate import") }
}

func TestExtractJavaImports(t *testing.T) {
    src := []byte("import com.example.auth.Token;\nimport java.util.List;")
    got := impact.ExtractJavaImports(src)
    if !slices.Contains(got, "com.example.auth.Token") { t.Error("missing import") }
}

func TestExtractKotlinImports(t *testing.T) {
    src := []byte("import com.example.billing.Invoice\nimport kotlin.io.path.*")
    got := impact.ExtractKotlinImports(src)
    if !slices.Contains(got, "com.example.billing.Invoice") { t.Error("missing import") }
}
```

- [ ] **Implement** lexical extraction (regex-based, stdlib `regexp`):
  - Rust: `use crate::`, `use super::`, `extern crate` → extract path component
  - Java: `^import\s+(\S+);` → filter stdlib (`java.`, `javax.`, `android.`)
  - Kotlin: `^import\s+(\S+)` → filter stdlib

- [ ] **Register** in the language dispatch table in `impact.go`.
- [ ] **Run tests, commit:** `feat(impact): add Rust, Java, Kotlin import adapters`

---

## Phase 4: Robustness

### Task 15: Fuzz tests

**Files:**
- Create: `internal/scan/fuzz_test.go`
- Create: `internal/session/fuzz_test.go`
- Create: `internal/testresult/fuzz_test.go`

- [ ] **Create `internal/scan/fuzz_test.go`:**

```go
//go:build go1.18
package scan_test

import (
    "testing"
    "github.com/ralabarta/agentproof/internal/scan"
)

func FuzzRedact(f *testing.F) {
    f.Add("normal text")
    f.Add("PRIVATE KEY-----\nabc")
    f.Add("password=secret123")
    f.Fuzz(func(t *testing.T, s string) {
        _ = scan.RedactString(s) // must not panic
    })
}
```

- [ ] **Create `internal/session/fuzz_test.go`:**

```go
func FuzzIngestJSONL(f *testing.F) {
    f.Add(`{"type":"message"}` + "\n")
    f.Add("{}\n{invalid\n")
    f.Fuzz(func(t *testing.T, s string) {
        _, _ = session.ParseJSONL(strings.NewReader(s)) // must not panic
    })
}
```

- [ ] **Create `internal/testresult/fuzz_test.go`:**

```go
func FuzzIngestXML(f *testing.F) {
    f.Add(`<testsuite tests="1"><testcase name="x"/></testsuite>`)
    f.Fuzz(func(t *testing.T, s string) {
        _, _ = testresult.IngestXML(strings.NewReader(s)) // must not panic
    })
}
```

- [ ] **Verify fuzz tests compile:** `go test -run=^$ ./internal/scan/... ./internal/session/... ./internal/testresult/...`
- [ ] **Commit:** `test(fuzz): add fuzz tests for untrusted parsers`

---

### Task 16: GoReleaser + SBOM

**Files:**
- Create: `.goreleaser.yml`
- Create: `.github/workflows/release.yml` (if not present)

- [ ] **Create `.goreleaser.yml`:**

```yaml
version: 2
project_name: agentproof

builds:
  - id: agentproof
    main: ./cmd/agentproof
    binary: agentproof
    env: [CGO_ENABLED=0]
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ldflags: ["-s -w -X main.version={{.Version}}"]

archives:
  - format: tar.gz
    format_overrides:
      - goos: windows
        format: zip

checksum:
  name_template: "checksums.txt"

sboms:
  - artifacts: binary
    args: ["$artifact", "--file", "$sbom", "--output", "spdx-json"]

signs:
  - cmd: cosign
    certificate: "${artifact}.pem"
    args:
      - "sign-blob"
      - "--output-certificate=${certificate}"
      - "--output-signature=${signature}"
      - "${artifact}"
      - "--yes"
    artifacts: checksum
    output: true
```

- [ ] **Verify GoReleaser config validates:** `goreleaser check` (or skip if goreleaser not installed locally)
- [ ] **Update GitHub Actions release workflow** to use `goreleaser/goreleaser-action` with `--clean`.
- [ ] **Commit:** `ci: add GoReleaser config with SBOM and cosign attestation`

---

## Execution Sequence

Run these tasks **in order** — `internal/app/app.go` is modified in every phase, so sequential commits prevent conflicts.

```
Phase 1: Tasks 1 → 2 → 3 → 4 → 5 → 6
Phase 2: Tasks 7 → 8 → 9 → 10 → 11
Phase 3: Tasks 12 → 13 → 14
Phase 4: Tasks 15 → 16
Final:   go test ./... && go vet ./... && git push
```

import (
    "encoding/json"
    "os"
    "path/filepath"
    "time"
    "github.com/ralabarta/agentproof/internal/config"
)

type State struct {
    Initialized   bool
    RunCount      int
    AbandonedRuns int
    LastBundleID  string
    LastStatus    string    // "passed", "warning", "failed", or ""
    LastVerifiedAt time.Time
}

func Read(root string) (State, error) {
    var s State
    cfgPath := filepath.Join(root, config.DirName, "config.json")
    if _, err := os.Stat(cfgPath); err != nil {
        return s, nil // not initialized — not an error
    }
    s.Initialized = true
    runsDir := filepath.Join(root, config.DirName, "runs")
    entries, _ := os.ReadDir(runsDir)
    for _, e := range entries {
        if !e.IsDir() { continue }
        s.RunCount++
        stateFile := filepath.Join(runsDir, e.Name(), "state.json")
        if data, err := os.ReadFile(stateFile); err == nil {
            var rs struct{ Status string `json:"status"` }
            if json.Unmarshal(data, &rs) == nil && rs.Status == "abandoned" {
                s.AbandonedRuns++
            }
        }
    }
    evPath := filepath.Join(root, config.DirName, "evidence.json")
    if data, err := os.ReadFile(evPath); err == nil {
        var ev struct {
            Run struct {
                Status   string    `json:"status"`
                BundleID string    `json:"bundleID"`
                At       time.Time `json:"at"`
            } `json:"run"`
        }
        if json.Unmarshal(data, &ev) == nil {
            s.LastStatus = ev.Run.Status
            s.LastBundleID = ev.Run.BundleID
            s.LastVerifiedAt = ev.Run.At
        }
    }
    return s, nil
}
```

