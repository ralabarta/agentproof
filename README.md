<div align="center">

# AgentProof

**Know exactly what your coding agent changed — and collect the evidence needed to decide whether it is safe to merge.**

[![CI](https://github.com/ralabarta/agentproof/actions/workflows/ci.yml/badge.svg)](https://github.com/ralabarta/agentproof/actions/workflows/ci.yml)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go 1.22+](https://img.shields.io/badge/go-1.22%2B-00ADD8.svg)](https://go.dev)
[![Zero dependencies](https://img.shields.io/badge/dependencies-stdlib%20only-6f42c1.svg)](#contributing)
[![Local by default](https://img.shields.io/badge/data-local%20by%20default-1f9d55.svg)](#privacy-and-trust-model)

[Quickstart](#quickstart) · [Why](#why-agentproof) · [GitHub Action](#github-action) · [Real report](docs/example-report.md) · [Architecture](docs/architecture.md) · [Threat model](docs/threat-model.md)

</div>

---

AgentProof is a **local-first Go CLI** that associates Codex and Claude Code sessions with Git changes, ingests test-result artifacts, detects deterministic risks, estimates code impact, and emits reproducible **Markdown, HTML, and JSON evidence**.

No account. No service. No telemetry. No network request.

## Quickstart

```bash
go install github.com/ralabarta/agentproof/cmd/agentproof@latest

agentproof init
agentproof record --objective "Protect refresh tokens from replay" --agent codex -- codex
agentproof verify --test-result test-results.jsonl
```

```text
AgentProof verification: WARNING
✓ Required evidence complete: 3/3
✓ Canonical manifest integrity passed
✓ 28 test results passed
✓ No secret patterns detected in captured added lines
⚠ Authentication or authorization code modified
Affected components: internal/auth, internal/api
Bundle ID: 7f0c…d91a
```

> [!IMPORTANT]
> AgentProof does **not** certify code as safe. It separates observed evidence, deterministic derivations, associations, unsupported checks, and unknowns so reviewers can make a better merge decision.

## Why AgentProof

Coding-agent evidence is normally scattered across terminal sessions, Git, CI logs, and review tools. AgentProof compiles those sources into one integrity-checked bundle while keeping raw session content on your machine.

| | Principle | What it means in practice |
|---|---|---|
| 🔒 | **Local by default** | No account, service, telemetry, or network request |
| 🧭 | **Honest linkage** | A Git/session match is an *association*, never an authorship claim |
| 🚧 | **Fail-closed evidence** | Missing or indeterminate required sources can never become a pass |
| 🎯 | **Deterministic checks** | Stable finding IDs, ordered output, bounded parsers, canonical manifests |
| 🍴 | **Fork-aware CI** | Verification ingests artifacts; it never runs tests, hooks, builds, or repo commands |

### Evidence vocabularies

Every human-facing claim is classified on three independent axes. This is the core of the trust model, and it is enforced: the vocabularies are enumerated in `internal/evidence` and a test fails if this section drifts from them.

**Evidence state** — was the source captured?

| State | Meaning |
|---|---|
| `observed` | Captured directly from Git, a process result, or a supplied artifact |
| `missing` | Required and declared, but not present where it was declared |
| `unsupported` | Not handled by the current version |
| `unknown` | Attempted but indeterminate, with a stated reason |
| `not_observed` | Never declared or discovered, so nothing was expected |

**Claim confidence** — how far is the conclusion from the captured bytes?

| Confidence | Meaning |
|---|---|
| `observed` | Read straight from captured evidence |
| `derived` | Computed deterministically from it |
| `inferred` | Weakened by conditions AgentProof observed but could not control |

**Association** — how firmly do the changes bind to the recorded window? Never a claim of authorship.

| Association | Meaning |
|---|---|
| `clean-baseline` | The baseline was clean, so the Git range is exact |
| `contaminated-baseline` | Uncommitted work predated recording and cannot be separated out |
| `unknown-uncaptured-worktree` | Changed content could not be captured, so the range is incomplete |

## Architecture

AgentProof is a local-first, capability-oriented Go modular monolith. Claude Code or Codex remains an external process: AgentProof wraps and observes it but makes no model calls; its core work is deterministic parsing, hashing, scanning, graph traversal, verification, and rendering.

### 1. System context and trust boundaries

```mermaid
flowchart LR
  Developer["Developer"]
  Reviewer["Reviewer"]

  subgraph External["External AI process"]
    Agent["Claude Code or Codex"]
  end

  subgraph Local["Local machine boundary"]
    CLI["AgentProof Go CLI"]
    Repo["Git repository"]
    Sessions["Native session JSONL"]
    Tests["JUnit or test2json artifacts"]
    Store["Local .agentproof evidence"]
    Outputs["Markdown, HTML, JSON, SARIF"]
  end

  subgraph CI["CI boundary"]
    Action["Workflow and composite Action"]
    Checkout["Exact-head Git checkout"]
    CIArtifacts["Supplied test artifacts"]
  end

  Developer -->|"starts record or verify"| CLI
  CLI -->|"wraps and observes; no model API"| Agent
  Agent -->|"changes files"| Repo
  Agent -->|"emits native history"| Sessions
  CLI -->|"snapshots and reads"| Repo
  CLI -->|"bounded parsing"| Sessions
  CLI -->|"ingests as data; never executes"| Tests
  CLI -->|"atomic 0600 writes"| Store
  Store --> Outputs
  Reviewer -->|"reviews evidence"| Outputs
  Action -->|"runs bounded verification"| CLI
  Checkout --> Action
  CIArtifacts --> Action
```

### 2. Internal modular architecture

```mermaid
flowchart TB
  subgraph Entry["CLI entry"]
    Cmd["cmd/agentproof"]
    App["internal/app"]
    Cmd --> App
  end

  subgraph Orchestration["Use-case orchestrators"]
    Record["record"]
    Verify["verify"]
    Status["status"]
    Doctor["doctor"]
    Purge["purge"]
  end

  subgraph Capabilities["Adapters and capabilities"]
    Gitx["gitx"]
    Session["session"]
    TestResult["testresult"]
    Scan["scan"]
    Impact["impact"]
  end

  subgraph Shared["Shared contracts and publication"]
    Config["config"]
    Evidence["evidence contracts"]
    SafeFile["safefile"]
    Report["report renderers"]
  end

  App --> Record
  App --> Verify
  App --> Status
  App --> Doctor
  App --> Purge
  Record --> Gitx
  Record --> Session
  Record --> Scan
  Verify --> Gitx
  Verify --> TestResult
  Verify --> Scan
  Verify --> Impact
  Record --> Evidence
  Verify --> Evidence
  Status --> Config
  Doctor --> Config
  Doctor --> Record
  Doctor --> Status
  Purge --> SafeFile
  Record --> SafeFile
  Verify --> SafeFile
  Verify --> Report
  Report --> Evidence
```

### 3. Evidence lifecycle

```mermaid
sequenceDiagram
  actor Dev as Developer
  participant CLI as AgentProof
  participant Lock as Kernel lock
  participant Git as Git repository
  participant Store as .agentproof
  participant Agent as Claude Code or Codex
  participant Session as Session JSONL
  participant Tests as Test artifacts
  participant Reports as Rendered reports

  Dev->>CLI: record objective and agent command
  CLI->>Lock: Acquire kernel-backed lock
  CLI->>Git: Capture initial snapshot
  CLI->>Store: Atomically persist recording lifecycle
  CLI->>Agent: Execute external process
  Agent->>Git: Modify worktree
  Agent-->>Session: Emit native session history
  alt Process completes
    Agent-->>CLI: Exit result
    CLI->>Git: Capture final snapshot and delta
    CLI->>CLI: Deterministic scan and redaction
    CLI->>Session: Bounded discovery and summarization
    CLI->>Store: Atomically persist complete run
  else Signal interrupts recording
    CLI->>Store: Atomically persist abandoned run
  end
  CLI->>Lock: Release lock

  Dev->>CLI: verify latest run or base comparison
  CLI->>Store: Load latest run or comparison inputs
  CLI->>CLI: Scan and import impact graph
  CLI->>Tests: Ingest bounded artifacts as data only
  CLI->>CLI: Build canonical manifest
  CLI->>CLI: Compute SHA-256 bundle identity, completeness, and status
  CLI->>Store: Atomically publish manifest and evidence
  CLI->>Store: Publish attestation
  CLI->>Reports: Render Markdown, HTML, and SARIF
  Reports-->>Dev: Reviewable evidence bundle
```

> **Architecture guarantees.** AgentProof treats sessions, patches, and test artifacts as untrusted data; bounds parsers and graph traversal; enforces containment and symlink checks; redacts before persistence; uses atomic `0600` writes; and emits escaped offline reports. Its CI path verifies the exact checked-out HEAD with read-only repository permissions, and verification never executes supplied tests. SHA-256 detects mutation; it does **not** prove authorship, causality, completeness, correctness, merge safety, or that an agent produced a change.

## Install

Download a release archive and verify it against `checksums.txt`, or build from source with Go 1.22+:

```bash
go install github.com/ralabarta/agentproof/cmd/agentproof@latest
```

<details>
<summary><b>Local development</b></summary>

```bash
git clone https://github.com/ralabarta/agentproof.git
cd agentproof
make test
make build
```

The runtime depends only on the Go standard library — there is nothing else to install.

</details>

<details>
<summary><b>Shell completions</b></summary>

```bash
# bash
source <(agentproof completion bash)
# zsh
source <(agentproof completion zsh)
# fish
agentproof completion fish | source
```

Scripts are generated from the same command and option table the CLI help uses.

</details>

## Record an agent session

Initialize inside an existing Git repository and commit the generated configuration before the first recording:

```bash
agentproof init
git add .agentproof/config.json .agentproof/.gitignore
git commit -m "chore: configure AgentProof"
```

Start from a clean worktree for the strongest association:

```bash
agentproof record \
  --objective "Add session rotation" \
  --agent codex \
  --model gpt-5 \
  -- codex
```

Claude Code uses the same wrapper:

```bash
agentproof record --objective "Add audit logging" --agent claude -- claude
```

Native adapters are heuristic and versioned as such. Unsupported or malformed artifacts become visible `unknown` evidence — they are never silently accepted.

<details>
<summary><b>Raw output retention (opt-in, off by default)</b></summary>

```bash
agentproof record --retain-raw --objective "Reproduce issue" -- codex
agentproof purge --raw            # preview files older than seven days
agentproof purge --raw --confirm  # delete the previewed selection
```

</details>

<details>
<summary><b>Run retention (abandoned and stuck runs)</b></summary>

```bash
agentproof purge --runs           # preview abandoned or stuck run directories
agentproof purge --runs --confirm # delete the previewed run directories
```

Retention is state-based: only runs that can never produce evidence are selected — abandoned by a signal, or stuck in the recording state after a crash that bypassed signal handling. Runs in progress, completed runs, and the evidence they carry are never touched.

</details>

## Inspect local state

```bash
agentproof status   # initialization, run counts, abandoned/stuck runs, last verification
agentproof runs     # list recorded runs
agentproof doctor   # diagnostics with actionable findings
```

Each run records a lifecycle in `state.json`:

| State | Meaning |
|---|---|
| `recording` | The agent command is running |
| `complete` | The command finished and the run was saved |
| `abandoned` | Interrupted by SIGINT/SIGTERM; the signal is recorded |

A run left in `recording` with no live lock is *stuck* — its record process crashed without finishing (SIGKILL, power loss). `status` and `doctor` surface abandoned and stuck runs, and `purge --runs` removes them.

## Supply test evidence

AgentProof deliberately does not run repository code during verification. Generate results in your existing test job, then supply one or more artifacts:

```bash
mkdir -p .agentproof/inputs
go test -json ./... > .agentproof/inputs/test-results.jsonl
agentproof verify --test-result .agentproof/inputs/test-results.jsonl --require-tests
```

Supported formats today:

- Go `go test -json` / `test2json` JSON Lines
- JUnit XML from existing test runners

Declared files are bounded, path-checked, rejected if symlinked, hashed, and parsed as data. **Commands embedded in artifacts are never executed.**

## Verify a Git range

Without a recorded local session, verify a pull-request range without claiming agent provenance:

```bash
agentproof verify --base origin/main --test-result junit.xml
```

Outputs are written under `.agentproof/`:

| File | Purpose |
|---|---|
| `manifest.json` | Canonical source census and computed bundle identity |
| `evidence.json` | Full normalized verification result |
| `attestation.json` | Hash of the emitted manifest plus bundle ID |
| `report.md` | Pull-request and terminal-friendly report |
| `report.html` | Offline self-contained report with restrictive CSP |
| `report.sarif` | SARIF 2.1.0 findings for integration with code-scanning tooling |

AgentProof verifies its own pull requests with the Action below. **[`docs/example-report.md`](docs/example-report.md) is real generated output from this repository, not a mock-up.**

Exit codes:

| Code | Meaning |
|---:|---|
| `0` | Passed or warning-only result |
| `1` | Required evidence, tests, or configured deterministic policy failed |
| `2` | Invalid command usage or configuration |
| `3` | Adapter, analyzer, repository, or internal processing failure |

## GitHub Action

Pin AgentProof to a full release commit SHA. Floating tags are not recommended for verification infrastructure.

```yaml
permissions:
  contents: read

steps:
  - uses: actions/checkout@9f698171ed81b15d1823a05fc7211befd50c8ae0 # v6.0.3
    with:
      fetch-depth: 0
  - uses: ralabarta/agentproof@<full-agentproof-release-commit-sha>
    with:
      base: origin/${{ github.base_ref }}
      test-results: |
        .agentproof/inputs/go-test.jsonl
        .agentproof/inputs/junit.xml
      require-tests: "true"
      fail-on: critical
```

The Action requests **read-only** repository permissions, emits machine-readable outputs, and uploads the report bundle. It also writes the verification report to the job step summary, so the result renders directly in the pull-request Checks tab. It does not comment on pull requests by default — comment publication should be a separate, explicitly trusted workflow that consumes only the generated report.

<details>
<summary><b>Action inputs and outputs</b></summary>

| Input | Default | Purpose |
|---|---|---|
| `base` | *required* | Git baseline to diff against |
| `test-results` | – | Newline-separated artifact paths |
| `require-tests` | `"false"` | Fail when no test evidence is present |
| `fail-on` | `critical` | Severity threshold that fails the job |

| Output | Purpose |
|---|---|
| `conclusion` | `passed`, `warning`, or `failed` |
| `bundle-id` | SHA-256 bundle identity |
| `completeness` | Required-evidence completeness percentage |
| `integrity` | Canonical manifest integrity result |
| `critical-violations` | Count of critical findings |
| `warnings` | Count of warnings |
| `report` | Path to the generated Markdown report |

</details>

## Current analysis

| Language | Import analysis |
|---|---|
| Go | Standard `go/parser` |
| TypeScript / JavaScript | Lexical extraction resolved against the on-disk file index |
| Python | Lexical extraction resolved against the on-disk file index |
| Rust, Java, C#, Ruby, PHP, Kotlin, Swift, Scala | Reported as `unsupported` — never as zero impact |

Only first-party files enter the graph. TypeScript `baseUrl` and `paths` aliases, directory `index` files, and ESM `.js` specifiers that resolve to `.ts` sources are handled. `node_modules`, `vendor`, and build output are never traversed.

Traversal is deterministic and bounded:

| Boundary | Limit |
|---|---:|
| Files | 20,000 |
| Parsed bytes | 512 MiB |
| Graph edges | 1,000,000 |
| Graph depth | 5 |
| Artifact size | 32 MiB per file |

Built-in rules cover secret-like additions and high-risk paths including authentication, migrations, dependency manifests, environment configuration, APIs, and CI workflows.

**Roadmap:** adapters for the remaining languages, Ed25519/Sigstore signing, test-to-change mapping, and versioned cost tables.

## Privacy and trust model

- Native session artifacts are summarized and hashed; raw prompts are never copied into reports.
- Secret-like values are removed before patches are persisted.
- Paths, JSON/XML records, file sizes, graph expansion, and nesting are all bounded.
- Recording uses a persistent `.record.lock` file with an OS-backed lock, while `doctor` and `purge` use run-ID-aware liveness checks; see the [architecture](docs/architecture.md) and [threat model](docs/threat-model.md).
- Reports escape untrusted Markdown and HTML; the HTML report has no scripts and no external assets.
- SHA-256 detects post-capture mutation. It does **not** establish source authenticity, authorship, completeness, correctness, or safety.

> [!WARNING]
> Read the [architecture](docs/architecture.md), [evidence contract](docs/evidence-schema.md), [threat model](docs/threat-model.md), and [security policy](SECURITY.md) before using AgentProof as a required merge gate.

Release history is in the [changelog](CHANGELOG.md). The [foundation archive review](docs/archive-review.md) records what was reused, strengthened, deferred, and excluded from the supplied project archive.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). The runtime intentionally depends only on the Go standard library — new dependencies require a licensing and supply-chain review.

## License

MIT © AgentProof contributors. See [LICENSE](LICENSE) and [third-party notices](THIRD_PARTY_NOTICES.md).
