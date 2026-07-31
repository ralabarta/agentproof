# Design: AgentProof Foundation

## Technical Approach

Build one Go binary as a capability-oriented modular monolith. `cmd/agentproof` is the sole composition root. Domain/application packages own ports; Git, provider, SQLite, Tree-sitter, filesystem, renderer, and CI adapters depend inward. No plugins, services, or browser application. Architecture import tests target Clean Architecture 10/10.

## Architecture Decisions

| Decision | Choice and rationale | Rejected alternative / tradeoff |
|---|---|---|
| Boundaries | `internal/evidence` is the kernel. `capture`, `linkage`, `verification`, `impact`, `policy`, and `reporting` depend on it, not each other’s adapters; each owns narrow ports. | Horizontal framework layers obscure capability ownership; distributed/plugin designs add premature protocols. |
| Evidence | `agentproof.dev/evidence/v1` manifests contain records, locators, repository/base/head, versions, findings, and unknowns. Serialized states are lowercase `observed`, `missing`, `unsupported`, `unknown` (attempted, indeterminate), and `not_observed`; Go identifiers may remain idiomatic. `not_observed` is limited to optional, non-required sources that were neither discovered nor policy-required and is outside the required-source completeness denominator. Every discovered or policy-required source resolves to one of the other four states and participates exactly once in that denominator. Confidence is integer 0–100 plus reason codes. Reject new majors; preserve unknown minor fields. | Booleans and floating confidence hide uncertainty. |
| Identity | Dedicated canonical JSON: UTF-8, sorted keys/semantic arrays, integers, normalized paths, UTC times. Presentation-only metadata stays outside the canonical manifest and its bytes. `bundleId` is lowercase SHA-256 of the canonical manifest bytes computed by excluding only the manifest's own `bundleId` field; there are no other manifest-field exclusions. Records include source and normalized digests. | Incidental Go encoding is not a contract; signing overclaims authenticity. |
| Storage | Canonical bundles are authoritative. SQLite WAL is a disposable index with embedded numbered transactional migrations and `user_version`; quarantine/rebuild on corruption or incompatibility. | Database authority prevents reproducible recovery. |
| Ingestion | Versioned Claude Code/Codex adapters expose bounded discover/parse operations and normalized diagnostics, never command execution. Golden fixtures cover valid, malformed, truncated, oversized, and drifted formats. Test adapters ingest JUnit XML and Go `test2json`; they never run tests. | One generic parser conceals drift. |
| Linkage | Inputs: repository identity, HEAD/base/parents, tree/index digests, changed paths, dirtiness, timestamps, hook/checkpoint IDs. Output confidence reasons include `explicit-marker`, `commit-match`, `tree-match`, `time-only`, `dirty-worktree`, `rebase-divergence`; never authorship. | Timestamp/line attribution creates false certainty. |
| Analysis | Statically linked Tree-sitter grammars initially support Go, TypeScript/TSX, and Python. Native parsing has byte/time/cancellation limits. Sorted graph traversal has depth/node caps and emits truncation/unsupported unknowns. Versioned built-in rules produce stable finding IDs, severity, evidence references, and order. | Regex-only parsing is weak; repository-loaded rules and unbounded traversal are unsafe. |
| Rendering | Markdown escapes tables, controls, bidi, links, and unsafe schemes. Self-contained HTML uses contextual escaping, no scripts/assets, and CSP `default-src 'none'; img-src data:; style-src 'unsafe-inline'`. | TypeScript viewer increases supply-chain/XSS surface. |

## Data Flow

```text
capture: CLI -> discover -> bounded parse -> redact -> Git link -> bundle/hash -> atomic store -> index
verify:  CLI -> integrity -> tests -> impact -> rules/completeness -> manifest/hash -> report
Action:  checkout + artifacts -> same verify -> summary + HTML artifact -> optional trusted comment
```

## Operational Contracts

Commands: `capture`, `verify`, `report`, `purge`, `version`. Exit codes: `0` pass/warnings; `1` missing required evidence/critical policy; `2` usage/config; `3` adapter/analyzer failure; `4` integrity failure. Diagnostics use stderr or deterministic `--json`; no telemetry or network.

Use XDG data/cache/config paths and platform equivalents; directories `0700`, files `0600`, atomic writes, canonical repository-relative locators, and traversal/symlink rejection. Normalized bundles retain 30 days. Raw capture requires `--retain-raw`, is isolated, never rendered, and expires after 7 days. `purge` supports age, repository, raw-only, and all.

The Action defaults to `contents: read`, no secrets, SHA-pinned actions/checksummed binary, `pull_request`, and summary/artifact output. Fork content is parsed, never executed. Comments require a separate same-repository job with `pull-requests: write`; never privileged `pull_request_target` checkout.

Adapter panic recovery, limits, cancellation, parameterized SQL, early redaction, and atomic publication isolate failures. STRIDE threats cover spoofed logs, tampering, repudiation, disclosure, parser exhaustion, and CI privilege escalation. Hashes detect mutation, not truthful/comprehensive sources.

## Testing Strategy and Threat Matrix

Strict RED-GREEN-REFACTOR: table-driven unit tests; adapter contracts; canonical goldens/properties; SQLite migration/rebuild integration; hostile parser/renderer tests; CLI/Action fixture E2E; import-cycle/direction checks.

| Boundary | Applicability; safe/failure behavior | Planned RED tests |
|---|---|---|
| Documentation-like paths | Applicable; data only, never execute; reject limits/path escape. | `requirements.txt`, `CMakeLists.txt`, executable MDX, `README.sh` |
| Git repository selection | Applicable; canonical explicit root; reject escape/symlink ambiguity. | relative, absolute, `git -C`-equivalent roots |
| Commit state | Applicable; preserve index/worktree state and unknowns. | staged, `commit -a`, empty index |
| Push state | N/A; no push/destination resolution. | None |
| PR commands | N/A; no PR or shell commands. | None |

## File Changes

| Path | Action / purpose |
|---|---|
| `cmd/agentproof/` | Create composition root/CLI. |
| `internal/{evidence,capture,linkage,verification,impact,policy,reporting}/` | Create capability policy, ports, tests. |
| `internal/adapters/{provider,git,sqlite,treesitter,testresult,render,fs}/` | Create infrastructure and fixtures. |
| `.github/workflows/agentproof.yml` | Create fork-safe integration. |

## Migration / Rollout / Rollback

Roll out local warning-only capture, reports, optional analyzers, then configured gates. Bundle migration writes and verifies beside originals; support one prior major. Roll back by disabling the gate, restoring the prior binary, rebuilding SQLite, and regenerating reports from unchanged bundles.

Release reproducibly for Linux/macOS `amd64`/`arm64` and Windows `amd64`. Matching pinned native runners build CGO grammars, then run tests, cross-platform fixture determinism, checksum, SBOM, and smoke gates. Other targets require identical native tests.

## Open Questions

None blocking; provider fixture revisions and later language expansion must preserve these contracts.
