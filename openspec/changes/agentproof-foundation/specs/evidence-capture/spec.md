# Evidence Capture Specification

## Purpose

Define safe, reproducible capture of Claude Code, Codex, and declared evidence sources without claiming authenticity or safety.

## ADDED Requirements

### Requirement: Evidence Discovery and States

Every source that is discovered or policy-required MUST be recorded with exactly one lowercase serialized state: `observed`, `missing`, `unsupported`, or `unknown`; no such source MAY be silently omitted. The required-source completeness denominator MUST be the union of discovered and policy-required sources, with each source participating exactly once. `not_observed` MAY be used only for an optional, non-required source that was neither discovered nor policy-required, and such a source MUST remain outside that denominator. Go identifiers MAY remain idiomatic, but their serialized representation MUST use these lowercase wire values. An observed item MUST include its source locator, provider/parser identity and version, normalized digest, capture time, and redaction summary. `unknown` MUST include a reason.

#### Scenario: Complete source census

- GIVEN required and optional sources are declared and discovery finds three sources
- WHEN capture completes
- THEN each discovered and declared source has exactly one state
- AND completeness reports observed over the union of discovered and policy-required sources as a count and percentage

### Requirement: Normalization and Redaction

Normalization MUST produce identical bytes for semantically identical supported input under the same parser version. Secrets and configured sensitive fields MUST be redacted before normalized content is persisted or rendered; redactions MUST preserve type and location metadata without preserving the secret value. Invalid input MUST become `unknown`, not partial `observed`.

#### Scenario: Secret-bearing transcript

- GIVEN equivalent transcripts differ only in line endings and contain a recognized credential
- WHEN both are captured with the same versions and policy
- THEN their normalized redacted bytes and SHA-256 digests are identical
- AND neither output contains the credential

### Requirement: Raw Evidence Lifecycle

Raw evidence retention MUST be disabled by default, local-only, explicitly opted in per capture, excluded from public reports and Action artifacts, and purgeable independently. Purge MUST report selected, deleted, and failed counts and MUST NOT invalidate retained normalized digests.

#### Scenario: Purge opted-in raw data

- GIVEN raw retention was enabled for one local capture
- WHEN that capture's raw evidence is purged
- THEN no selected raw bytes remain and normalized evidence remains verifiable

### Requirement: Hostile Input Bounds and Version Drift

Capture MUST reject over-limit input without unbounded work. Default limits MUST include 32 MiB per source, 256 MiB per run, 100,000 records, nesting depth 64, and locator length 4,096 bytes; configured limits MUST be recorded. Unknown provider/parser versions MUST NOT be interpreted by a different version and MUST yield `unsupported` or `unknown` with the detected and supported versions.

#### Scenario: Oversized or drifted input

- GIVEN a source exceeds a recorded limit or declares an unsupported format version
- WHEN capture is attempted
- THEN the source is not `observed`
- AND the result names the breached limit or version mismatch without exposing source content
