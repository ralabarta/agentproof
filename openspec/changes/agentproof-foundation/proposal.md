# Proposal: AgentProof Foundation

## Intent
Agent-assisted changes are hard to review because session, Git, test, and risk evidence is fragmented, sensitive, and overstated. Build a local-first evidence compiler as agent adoption outpaces review practice. Users are developers, maintainers, and small teams using Claude Code or Codex before merge.

**Honest promise:** AgentProof reports observed evidence, transformations, checks, confidence, and unknowns. Observed evidence is not proof of safety. Hashes show tampering after capture, not authenticity, authorship, completeness, or correctness.

## MVP Outcomes
- Primary: every discovered required source is observed, missing, unsupported, or unknown, with reproducible digests.
- Secondary: detect deterministic risk and shorten evidence gathering without noisy CI.
- Failure: hidden omissions, leaked secrets, nondeterminism, safety overclaims, or abandoned gates.

## Scope
### In Scope
- Redacted Claude Code/Codex evidence linked to Git with confidence and unknowns.
- Test ingestion, policy checks, canonical manifests, reports, and local verification.
- Limited impact analysis and a fork-safe GitHub Action.

### Out of Scope
Cloud/enterprise products, owned agents, broad provider/language coverage, test execution during ingestion, line authorship, safety certification, legal conclusions, architecture truth, signing, and interactive viewers.

## Capabilities
### New Capabilities
- `evidence-capture`: normalization, redaction, locators, digests, and evidence states.
- `git-linkage`: confidence-scored association, never authorship.
- `verification-reporting`: completeness, integrity, findings, unknowns, and reports.
- `impact-analysis`: bounded observations for supported languages.
- `github-action`: least-privilege delivery.

### Modified Capabilities
None.

## Approach and Staging
Use a Go single-binary modular monolith with capability-owned ports and adapters. Stage: evidence kernel; capture/linkage; test evidence/reports; impact/risk; Action. SQLite is a disposable index, not the trust anchor.

Default storage is normalized/redacted evidence, locator, and digest. Raw retention/export is local, purgeable opt-in, never public by default. Treat inputs as hostile: bound parsing, canonicalize paths, redact early, escape output, never execute repository code, and pin CI artifacts.

`verify` fails only for broken integrity, missing required inputs, analyzer failure, or critical deterministic violations. Ambiguous linkage, unsupported analysis, and missing optional evidence warn.

Dependencies are Git, SQLite, Tree-sitter, provider/test formats, and GitHub Actions, isolated behind versioned adapters and contract fixtures.

## Risks, Rollout, and Rollback
Provider drift, ambiguous linkage, parser risk, secret exposure, nondeterminism, and overclaiming are mitigated by fixtures, visible confidence, limits, redaction, canonical ordering, and restrained language. Dogfood on fixtures, then opt-in repositories, then warning-only CI before configured gates. Roll back by disabling the gate and regenerating canonical bundles; analyzers stay optional.

## Rejected Alternatives
Rust raises contribution cost; TypeScript adds premature complexity; cloud-first storage harms privacy; raw-by-default creates a transcript warehouse; early signing implies authenticity AgentProof cannot establish.
