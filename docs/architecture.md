# AgentProof architecture

AgentProof is a local-first evidence compiler implemented as one Go binary. The current release is a capability-oriented modular monolith with no runtime network access and no third-party Go modules.

## Trust model

Every human-facing claim is classified as:

- `observed`: captured directly from Git, a process result, or a supplied artifact;
- `derived`: calculated deterministically from observed bytes;
- `associated`: linked by Git state, content, or time without claiming causality or authorship;
- `unsupported`: not handled by the current version;
- `unknown`: attempted but indeterminate, with a reason.

Hashes detect modification after capture. They do not prove authenticity, authorship, completeness, correctness, runtime reachability, or safety.

## Components

| Package | Responsibility |
|---|---|
| `internal/record` | Explicitly wrap a user-selected agent process and capture its Git window |
| `internal/session` | Bounded native Codex/Claude JSONL discovery and normalization |
| `internal/gitx` | Git snapshots, committed/working changes, patches, and merge-base association |
| `internal/testresult` | Data-only JUnit and Go test2json ingestion |
| `internal/evidence` | Canonical source census, states, confidence, and bundle identity |
| `internal/impact` | Bounded Go, TypeScript/JavaScript, and Python import graph and unsupported-language reporting |
| `internal/scan` | Versioned deterministic secret and high-risk rules |
| `internal/report` | Escaped Markdown and offline CSP-constrained HTML |
| `internal/safefile` | Symlink-resistant atomic local publication |
| `internal/purge` | Preview-first retention for raw logs and dead run directories |
| `internal/status` | Read local state: initialization, run counts, abandoned/stuck runs, last verification |
| `internal/doctor` | Surface actionable diagnostics from the local state |
| `internal/completion` | Render bash, zsh, and fish completion scripts from the CLI command table |

## Data flows

### Record

1. Resolve the Git repository, capture HEAD plus working-tree state, and acquire an advisory lock that rejects parallel records.
2. Write the per-run lifecycle state (`recording`) and run the explicitly supplied agent command; SIGINT and SIGTERM mark the run `abandoned` with the signal recorded.
3. Capture final Git state and separate committed from working-tree changes.
4. Scan the in-memory patch for deterministic findings.
5. Redact secret-like values before persisting the patch.
6. Discover bounded native session artifacts modified in the command window.
7. Write normalized run evidence atomically with mode `0600` and mark the run `complete`.

Raw command output is retained only with `--retain-raw`.

### Verify

1. Load a recorded run or associate a Git range through its merge base.
2. Scan captured added lines and build a bounded first-party import graph over Go, TypeScript/JavaScript, and Python sources.
3. Ingest declared JUnit XML or Go test2json as untrusted data; execute nothing.
4. Resolve every discovered or policy-required source to one evidence state.
5. Canonicalize and order manifest records.
6. Compute the bundle ID over canonical bytes excluding only `bundleId`.
7. Atomically publish manifest, normalized evidence, attestation, Markdown, and HTML.

## Operational limits

| Boundary | Default |
|---|---:|
| Native/test artifact | 32 MiB per file |
| Test artifact set | 256 MiB |
| JSONL/XML records | 100,000 |
| JSON nesting | 64 |
| Impact files | 20,000 |
| Impact parsed bytes | 512 MiB |
| Impact edges | 1,000,000 |
| Impact depth | 5 |

Limit exhaustion is visible and cannot become a complete analysis result.

## CI boundary

The published Action source is trusted by pinning its full commit SHA. It builds that pinned AgentProof source, reads pull-request Git content and supplied test artifacts, and uploads reports. It does not execute pull-request tests, hooks, builds, or embedded instructions. Standard repository CI may execute tests in a separate unprivileged job and pass the resulting artifact to AgentProof.

The default Action requests no write permission and does not comment. A comment publisher, if configured, must be a separate trusted workflow consuming only verified report output.

