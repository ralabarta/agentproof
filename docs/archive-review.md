# Review of the supplied AgentProof foundation archive

The supplied archive contained one committed Go evidence kernel plus OpenSpec product, security, Git-linkage, impact, reporting, and GitHub Action requirements. It did not contain third-party repositories or vendored dependencies.

## Integrated

- Canonical `agentproof.dev/evidence/v1` manifest with deterministic ordering and SHA-256 bundle identity.
- Explicit `observed`, `missing`, `unsupported`, `unknown`, and `not_observed` states.
- Integer confidence with stable reason codes.
- Required/discovered source completeness instead of silent omission.
- Repository-relative locator normalization, duplicate/traversal rejection, and canonical validation.
- Association terminology that explicitly rejects Git/session authorship claims.
- Separation of committed and working-tree changes.
- Unknown evidence for uncaptured binary, symlinked, oversized, or unreadable changes.
- Data-only JUnit and Go test2json ingestion; `verify` executes no repository code.
- Default-off raw output, pre-persistence secret redaction, and preview-first raw purge.
- Bounded JSONL, XML, and Go graph analysis.
- Escaped deterministic Markdown and offline HTML with restrictive CSP.
- Read-only, fork-aware Action behavior with immutable third-party Action SHAs and no default comments.
- Publication material: release workflow, cross-platform archives, checksums, security policy, threat model, contribution guide, and third-party notices.

## Strengthened during integration

The original manifest implementation normalized traversal-like locators with `path.Clean`; this integration rejects escaping, absolute, NUL-bearing, duplicate, invalid-state, invalid-digest, and overlong records instead. It also distinguishes discovered sources from policy-required sources so the completeness denominator follows the supplied specification.

The original design required presentation data outside canonical identity. AgentProof now emits `manifest.json` as the canonical source census while `evidence.json`, Markdown, HTML, and timestamped attestation remain presentation/operational artifacts.

## Deliberately deferred

- SQLite disposable indexing and migrations.
- Tree-sitter grammars for TypeScript/TSX and Python.
- Golden provider fixtures tied to documented Codex and Claude format versions.
- CPU deadline/cancellation enforcement across every parser.
- SARIF, Ed25519/Sigstore identity signing, and test-to-change coverage mapping.
- A separate trusted PR-comment publisher.
- Homebrew and Windows installer packaging.

These remain post-`v0.1.0` work and are not claimed by the current README or reports.

## Excluded archive content

Local development metadata, editor/agent registries, embedded Git runtime records, review transaction logs, and task memory were not incorporated into the publishable source tree.

