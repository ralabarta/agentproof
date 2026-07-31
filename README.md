# AgentProof

> Know exactly what your coding agent changed—and collect the evidence needed to decide whether it is safe to merge.

[![CI](https://github.com/ralabarta/agentproof/actions/workflows/ci.yml/badge.svg)](https://github.com/ralabarta/agentproof/actions/workflows/ci.yml)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Local by default](https://img.shields.io/badge/data-local%20by%20default-1f9d55.svg)](#privacy-and-trust-model)

AgentProof is a local-first Go CLI that associates Codex and Claude Code sessions with Git changes, ingests test-result artifacts, detects deterministic risks, estimates code impact, and emits reproducible Markdown, HTML, and JSON evidence.

```bash
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

AgentProof does not certify code as safe. It distinguishes observed evidence, deterministic derivations, associations, unsupported checks, and unknowns so reviewers can make a better merge decision.

## Why AgentProof

Coding-agent evidence is normally split across terminal sessions, Git, CI logs, and review tools. AgentProof compiles those sources into one integrity-checked bundle while keeping raw session content local.

- **Local by default:** no account, service, telemetry, or network request.
- **Honest linkage:** a Git/session match is an association, never an authorship claim.
- **Fail-closed evidence:** missing or indeterminate required sources cannot become a pass.
- **Deterministic checks:** stable finding IDs, ordered output, bounded parsers, and canonical manifests.
- **Fork-aware CI:** verification ingests artifacts and does not run tests, hooks, builds, or repository commands.

## Install

Download a release archive and verify it against `checksums.txt`, or install from source with Go 1.22+:

```bash
go install github.com/ralabarta/agentproof/cmd/agentproof@latest
```

For local development:

```bash
git clone https://github.com/ralabarta/agentproof.git
cd agentproof
make test
make build
```

## Record an agent session

Initialize AgentProof inside an existing Git repository and commit the generated configuration before the first recording:

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

Native adapters are currently heuristic and versioned as such. Unsupported or malformed artifacts become visible `unknown` evidence; they are not silently accepted.

Raw command output is not saved by default. Local debugging can opt in per run:

```bash
agentproof record --retain-raw --objective "Reproduce issue" -- codex
agentproof purge --raw                 # preview files older than seven days
agentproof purge --raw --confirm       # delete the previewed selection
```

## Supply test evidence

AgentProof deliberately does not run repository code during verification. Generate results in your existing test job, then supply one or more artifacts:

```bash
mkdir -p .agentproof/inputs
go test -json ./... > .agentproof/inputs/test-results.jsonl
agentproof verify --test-result .agentproof/inputs/test-results.jsonl --require-tests
```

Supported initial formats:

- Go `go test -json` / `test2json` JSON Lines.
- JUnit XML produced by existing test runners.

Declared files are bounded, path-checked, rejected if symlinked, hashed, and parsed as data. Commands embedded in artifacts are never executed.

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

The Action uses read-only repository permissions, emits machine-readable outputs, and uploads the report bundle. It does not comment on pull requests by default. Comment publication should be a separate, explicitly trusted workflow that consumes only the generated report.

## Current analysis

- Go imports are analyzed with the standard Go parser.
- TypeScript, JavaScript, and Python imports are extracted lexically and resolved against the on-disk file index, so only first-party files enter the graph. TypeScript `baseUrl` and `paths` aliases, directory `index` files, and ESM `.js` specifiers that resolve to `.ts` sources are handled; `node_modules`, `vendor`, and build output are never traversed.
- Traversal is deterministic and bounded to 20,000 files, 512 MiB, 1,000,000 edges, and depth 5.
- Rust, Java, C#, Ruby, PHP, Kotlin, Swift, and Scala changes are reported as unsupported rather than as zero impact.
- Built-in rules cover secret-like additions and high-risk paths including authentication, migrations, dependency manifests, environment configuration, APIs, and CI workflows.

Adapters for the remaining languages, SARIF, Ed25519/Sigstore signing, test-to-change mapping, and versioned cost tables remain future work.

## Privacy and trust model

- Native session artifacts are summarized and hashed; raw prompts are not copied into reports.
- Secret-like values are removed before patches are persisted.
- Paths, JSON/XML records, file sizes, graph expansion, and nesting are bounded.
- Reports escape untrusted Markdown and HTML and the HTML report has no scripts or external assets.
- SHA-256 detects post-capture mutation. It does not establish source authenticity, authorship, completeness, correctness, or safety.

Read the [architecture](docs/architecture.md), [evidence contract](docs/evidence-schema.md), [threat model](docs/threat-model.md), and [security policy](SECURITY.md) before using AgentProof as a required merge gate. Release history is recorded in the [changelog](CHANGELOG.md).

The [foundation archive review](docs/archive-review.md) records what was reused, strengthened, deferred, and excluded from the supplied project archive.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). The runtime intentionally depends only on the Go standard library. New dependencies require a licensing and supply-chain review.

## License

MIT © AgentProof contributors. See [LICENSE](LICENSE) and [third-party notices](THIRD_PARTY_NOTICES.md).
