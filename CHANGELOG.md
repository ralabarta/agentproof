# Changelog

All notable changes to AgentProof are documented in this file. The project follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- `agentproof completion` command generating bash, zsh, and fish completion scripts from a single command table.
- GitHub Action writes the verification report to the job step summary so results render directly in the pull-request Checks tab.
- Per-run lifecycle `state.json` written by `record` (recording → complete, or abandoned with the signal) and an advisory lock against parallel records.
- `purge --runs` retention policy selecting abandoned and stuck run directories while completed evidence is never touched.
- Fuzz targets for secret redaction, the patch scanner, native session normalization, and test-result ingestion.

### Planned

- Tree-sitter impact adapters for TypeScript, TSX, and Python.
- Identity-backed signing (Ed25519/Sigstore).
- Versioned native-session fixtures for Codex and Claude Code.

## [0.1.0] - 2026-07-31

### Added

- Local-first `init`, `record`, `verify`, and `purge` commands.
- Canonical evidence manifest, deterministic bundle identity, and hash attestation.
- Git range and working-tree capture with explicit unknown evidence for uncaptured content.
- Go import impact graph with bounded traversal and visible unsupported-language results.
- Data-only Go test2json and JUnit ingestion; verification executes no repository code.
- Deterministic secret and high-risk change findings.
- Escaped Markdown and offline HTML reports with a restrictive content security policy.
- Read-only GitHub Action, CI, reproducible cross-platform release archives, and checksums.
- Security policy, threat model, contribution guide, and third-party notices.

[Unreleased]: https://github.com/ralabarta/agentproof/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/ralabarta/agentproof/releases/tag/v0.1.0
