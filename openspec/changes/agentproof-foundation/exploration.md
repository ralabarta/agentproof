## Exploration: agentproof-foundation

### Current State
AgentProof is a greenfield concept with no application scaffold, dependency manifest, tests, or implementation. Its defensible MVP is a local-first evidence compiler for agent-assisted changes: it captures observable session and test artifacts, links them to Git state, performs deterministic analysis, and emits reviewable reports. The initial user is an individual developer or repository maintainer using Claude Code or Codex; the practical buyer is the same person or a small engineering team that needs faster, more defensible merge decisions without sending source or transcripts to a cloud service.

The product must not claim that generated code is objectively safe. Its trust claim should be narrower: AgentProof reports what evidence was observed, how it was linked and transformed, which deterministic checks ran, their results, confidence, and explicit unknowns. It can make evidence tampering detectable after capture, but cannot prove session completeness, truthful provider logs, test adequacy, semantic correctness, authorship, or absence of undiscovered risk.

Clean Architecture score for the recommended foundation: 9/10 if domain evidence types and use cases own ports while Git, providers, SQLite, Tree-sitter, CI, and renderers remain adapters. Reaching 10/10 requires architecture tests that enforce import direction and contract tests for every adapter.

### Affected Areas
- `/home/home/workspace/SideProjects/agentproof/openspec/config.yaml` — establishes the local-first, commit-linked scope, hybrid persistence, strict TDD, and unresolved stack.
- `/home/home/workspace/SideProjects/agentproof/openspec/changes/agentproof-foundation/exploration.md` — required OpenSpec exploration artifact; filesystem persistence was unavailable in this executor runtime.
- Future `cmd/agentproof` composition root — CLI and GitHub Action entry point.
- Future domain/application modules — normalized evidence, commit linkage, verification orchestration, findings, unknowns, and report model.
- Future adapters — Claude Code, Codex, Git, test formats, Tree-sitter, SQLite, Markdown/HTML, and GitHub Actions.
- Future tests — golden determinism, hostile-input, parser contract, linkage-confidence, migration, and end-to-end fixture tests.

### Approaches

| Approach | Pros | Cons | Effort |
|---|---|---|---|
| Go single-binary modular monolith | Closest comparable tools validate Go for CLI, Git/session ingestion, SQLite, and Actions; simple deployment; strong standard library; fast builds | Tree-sitter uses CGO/native grammars and explicit cleanup; cross-platform release matrix required; less expressive type system than Rust | Medium |
| Rust single-binary modular monolith | Strong types and memory safety; ergonomic Tree-sitter bindings; good CLI ecosystem | Higher contributor/build complexity; native grammar and SQLite packaging remain; weaker evidence of fit among closest tools | Medium-High |
| Go core plus TypeScript viewer | Better interactive graph exploration later | Two toolchains, larger supply chain, harder self-contained delivery, premature for static reports | High |

1. **Go modular monolith** — one deployable binary with inward-pointing domain/application dependencies and explicit adapter ports.
   - Pros: Best MVP delivery fit, testable boundaries, no service or plugin protocol overhead, easy local and Action use.
   - Cons: Requires disciplined package boundaries and native release engineering for Tree-sitter.
   - Effort: Medium.

2. **Rust modular monolith** — same boundaries implemented in Rust.
   - Pros: Stronger compile-time modeling and memory guarantees.
   - Cons: Does not remove the difficult native grammar/SQLite distribution work and raises MVP contribution cost.
   - Effort: Medium-High.

3. **Polyglot core and interactive viewer** — Go analysis core with a TypeScript UI.
   - Pros: Rich graph navigation and scalability for large reports.
   - Cons: Solves a post-MVP need; undermines single-binary simplicity and deterministic self-contained output.
   - Effort: High.

### Recommendation
Choose Go and a single-binary modular monolith. Organize by capability, not framework layers: evidence, capture, linkage, verification, impact, policy, and reporting. Domain/application code defines small ports; adapters implement provider parsing, Git inspection, test ingestion, Tree-sitter extraction, SQLite persistence, rendering, and CI delivery. `main` is the composition root. Do not introduce dynamic plugins, services, or a TypeScript viewer in the MVP.

Use an append-only normalized evidence model. Every record should include a stable schema version, evidence ID, kind, source/provider and source locator, repository identity, observed timestamp, payload digest, parser version, redaction status, and optional confidence. Preserve source-specific data only in an adapter-owned envelope. Model explicit `Unknown` and `NotObserved` states rather than interpreting absence as success.

Link evidence to Git using repository identity, commit SHA, parent/base SHA, tree/worktree state, changed paths, timestamps, and explicit hook/checkpoint markers when available. Linkage is an evidence-backed association with a confidence/reason set, not authorship proof. Dirty worktrees and rebases must remain visible. Prefer explicit capture markers over timestamp-only heuristics.

Use SQLite as a disposable local index/cache for normalized sessions, commits, symbol/edge indexes, findings, and report lookup. It supports transactions, migrations, FTS, and incremental analysis, but is not the trust anchor. Canonical evidence bundles and reports should be reproducible from captured inputs; corruption or deletion of the database must not change the meaning of exported evidence.

Build a canonical manifest before presentation: normalized evidence references, input hashes, tool/schema/parser versions, repository/base/head identities, rule configuration, findings, unknowns, and deterministic ordering. Hash canonical bytes with SHA-256; derive Markdown and escaped self-contained HTML from that model. Hash chaining provides tamper evidence only when a prior digest is anchored outside the mutable bundle. Do not call it signing or authenticity.

Verification pipeline: bounded input discovery -> safe parse and normalization -> immediate redaction/classification -> Git linkage -> test-result ingestion -> diff/symbol impact analysis -> deterministic secret/high-risk rules -> completeness and unknown assessment -> canonical manifest/hash -> Markdown/HTML rendering. Ingestion should not execute repository code. Test running is a separate explicit command; MVP can ingest JUnit and selected native JSON formats first.

GitHub Action mode should run the same binary on a checked-out base/head, consume uploaded/cached session and test artifacts, write Markdown to the job summary and upload self-contained HTML. PR comments should be optional and use minimal permissions. For fork PRs, do not expose secrets or run privileged `pull_request_target` code against attacker-controlled checkout. Pin release artifacts by version and checksum; later releases should publish attestations.

Build-vs-integrate decisions:
- `agentsview`: reuse concepts for provider adapters, normalization, and SQLite/FTS; do not copy its product or large parser subsystems. Consider narrow adapter-level reuse only after API/license review.
- `entireio/cli`: reuse concepts for hooks, redaction, and Git checkpoints; implement AgentProof-owned contracts clean-room rather than adopting its checkpoint model wholesale.
- `code-review-graph`: reuse incremental Tree-sitter graph and bounded BFS impact concepts; either depend on low-level Tree-sitter grammars or implement the algorithm in Go. Do not embed its Python/TS application.
- `likec4`: do not integrate for MVP. Static deterministic HTML is enough; revisit only for interactive architecture navigation.
- `open-code-review`: reuse boundary ideas for diff/model/session/viewer/provider separation. Its Go `internal/` packages are not importable; do not copy them.
- Prefer standard Git CLI plumbing behind a port initially, a maintained SQLite driver, official Tree-sitter bindings/grammars, and narrowly reviewed libraries. Record every dependency and license; avoid vendoring large repositories.

Honest MVP reductions:
- **Line-level agent attribution:** exclude as a claim. Report session-to-commit linkage and optional path/hunk overlap with confidence. Exact attribution requires editor/tool-call instrumentation and still cannot establish human vs agent authorship after edits.
- **Generated lines without tests:** replace with “changed files/hunks with no observed relevant test evidence” and list limitations. AgentProof cannot infer generation or prove line coverage from arbitrary test results.
- **Architecture before/after:** defer architecture diagrams. MVP may report deterministic symbol/dependency deltas for supported languages, labeled as structural observations rather than architecture truth.
- **License scanning:** defer legal conclusions. A later slice may inventory declared dependencies and detected license metadata through established ecosystem tools.
- **Cost precision:** accept provider-reported token/cost fields when present; otherwise label estimates with model, pricing snapshot, and uncertainty. Do not present normalized dollar totals as exact.
- **Signing:** defer private-key signing. Ship deterministic SHA-256 manifests first; consider Sigstore/keyless attestations after artifact semantics stabilize.

Trust, security, and privacy boundaries:
- Treat logs, transcripts, repository files, Git metadata, test reports, Tree-sitter input, configuration, and CI event data as hostile.
- Enforce byte/file/event/depth/time limits, strict schema decoding, path canonicalization, traversal and symlink defenses, parameterized SQL, parser timeouts/cancellation, and crash containment where practical.
- Never evaluate transcript text, execute captured commands, load repository-defined code/config implicitly, or interpolate untrusted data into shell commands.
- Escape all Markdown/HTML fields; use a restrictive CSP in HTML and no remote assets/scripts. Prevent formula/link injection and unsafe URI schemes.
- Redact secrets before persistence and rendering; store fingerprints and locations, not secret values. Raw transcript retention should be opt-in, local, permission-restricted, and independently purgeable.
- Separate evidence parsing from policy evaluation. Include rule versions and configuration in the manifest. Findings are deterministic observations, not guarantees.
- CI must use least privilege, avoid privileged fork execution, pin dependencies/actions, and ensure comments cannot become script/HTML injection vectors.
- Hashes detect mutation, not false source data, omitted events, compromised capture hooks, or compromised binaries. Reports must state these unknowns.

Staged MVP slices:
1. **Evidence kernel:** Go module, domain schema, canonical serialization/hash, redaction primitives, CLI skeleton, strict TDD harness, deterministic and hostile-input fixtures.
2. **Capture and linkage:** Claude Code and Codex adapters, local storage/index, explicit capture markers, Git commit/worktree association with confidence and unknowns.
3. **Test evidence and reports:** ingest JUnit plus one provider-native format, canonical finding model, deterministic Markdown and self-contained HTML, no repository code execution.
4. **Impact and risk:** limited language set using Tree-sitter, incremental symbol/edge index, bounded impact traversal, secret/high-risk diff rules, clear unsupported-language fallback.
5. **GitHub Action:** checksum-pinned binary, base/head analysis, artifact/summary output, optional least-privilege PR comment, fork-safe behavior.

Proposal-phase open questions:
- What is the first success metric: reduced review time, caught risky changes, or evidence completeness?
- Which OS/architectures and first Tree-sitter languages are mandatory?
- Where exactly do Claude Code and Codex expose stable session identifiers, hooks, usage, and timestamps, and what parser-version compatibility policy is acceptable?
- Should raw prompts/tool outputs be retained by default, retained redacted, or discarded after normalization?
- Which test formats are required beyond JUnit, and is running tests explicitly out of scope for the first release?
- What evidence-link confidence threshold should block a report versus produce a warning?
- Should the Action only produce summary/artifact output initially, avoiding PR write permissions?
- What local retention, purge, and repository-sharing behavior is expected for evidence bundles?

### Risks
- Provider log formats and hooks can change without notice; adapter fixtures and compatibility versioning are mandatory.
- Session-to-commit linkage can be ambiguous after rebases, parallel agents, dirty worktrees, or missing hooks.
- Tree-sitter CGO/native grammar distribution increases release complexity and expands the parser attack surface.
- Secret scanners produce false positives and can leak matched values if reports are careless.
- CI on hostile pull requests can expose tokens or execute attacker-controlled content if workflow permissions are wrong.
- Determinism can be broken by timestamps, map iteration, absolute paths, locale, parser versions, or nondeterministic graph traversal.
- “Safety” language can overstate what evidence proves and create harmful reviewer complacency.

### Ready for Proposal
Yes. The proposal should select Go, preserve the single-binary modular-monolith boundary, define evidence integrity and unknown semantics, adopt the reduced claims above, and sequence the MVP so deterministic evidence/reporting exists before advanced impact analysis or CI commenting.
