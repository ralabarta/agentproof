# Contributing to AgentProof

AgentProof welcomes focused fixes, provider fixtures, deterministic risk rules, parsers, and platform support.

Participation is governed by the [Code of Conduct](CODE_OF_CONDUCT.md). Report vulnerabilities and conduct concerns through the private channel in [SECURITY.md](SECURITY.md), never in a public issue.

## Development

Requirements: Go 1.22+, Git, and GNU Make.

```bash
make test
make build
```

Before opening a pull request, run:

```bash
go test ./...
go vet ./...
```

Keep changes reviewable and add tests for malformed, oversized, truncated, adversarial, and version-drifted input where relevant.

## Evidence language

- Use the published vocabularies, which are three separate axes and never interchangeable. Evidence state is `observed`, `missing`, `unsupported`, `unknown`, or `not_observed`. Claim confidence is `observed`, `derived`, or `inferred`. Git association is `clean-baseline`, `contaminated-baseline`, or `unknown-uncaptured-worktree`.
- Adding a value means adding it to `States()`, `ClaimConfidences()`, or `Associations()` in `internal/evidence`, and documenting it in the README. Tests check the documents against those functions, so an undocumented value fails the build.
- Do not claim authorship from timing, commits, file changes, or session proximity.
- Do not describe a passing report as proof that code is safe.
- Missing or indeterminate required evidence must fail closed.

## Dependencies and licensing

The runtime intentionally uses the Go standard library only. New runtime dependencies need a documented reason, a compatible license, pinned versions, and a supply-chain review. Generated code and copied implementations must retain all required notices and attribution.

By contributing, you agree that your contribution is licensed under the repository's MIT License.

