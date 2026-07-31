# Verification and Reporting Specification

## Purpose

Define integrity, completeness, test-evidence ingestion, exit semantics, and deterministic human-readable reports.

## ADDED Requirements

### Requirement: Canonical Manifest and Integrity

The system MUST emit one canonical manifest with stable field presence, path normalization, ordering, number representation, and UTF-8 encoding. Presentation-only metadata MUST remain outside the canonical manifest and therefore outside its canonical bytes. Identical logical evidence, policy, and component versions MUST produce byte-identical manifests. The bundle identifier MUST be lowercase SHA-256 of the exact canonical manifest bytes computed by excluding only the manifest's own identifier field; no other manifest field MAY be excluded. Verification MUST recompute every declared digest.

#### Scenario: Reproducible bundle

- GIVEN the same normalized inputs, policy, and versions in different discovery orders
- WHEN two manifests are generated
- THEN their bytes and bundle identifiers are identical

### Requirement: Completeness and Integrity Results

Verification MUST report source counts by lowercase serialized state, `observed/required` percentage, optional-source counts, digest checks, deterministic findings, warnings, unknowns, and component versions. The required-source completeness denominator MUST be the union of discovered and policy-required sources, counted once each; every member MUST resolve to `observed`, `missing`, `unsupported`, or `unknown`. `not_observed` MUST be limited to optional, non-required sources that were neither discovered nor policy-required and MUST remain outside that denominator. Completeness is satisfied only when 100% of denominator sources are `observed`; integrity is satisfied only when every referenced retained artifact matches its digest.

#### Scenario: Missing required evidence

- GIVEN 3 required sources with 2 observed and 1 missing
- WHEN verification runs
- THEN completeness is `2/3 (66.67%)` and failed
- AND the missing source is named while integrity is reported independently

### Requirement: Test Evidence Ingestion

The system MUST ingest only declared supported test-result artifacts and MUST NOT execute repository code, test commands, hooks, build scripts, or artifact-embedded instructions. Each test result MUST retain producer/format version, counts, status, duration when supplied, and source digest. Malformed or unsupported results MUST be `unknown` or `unsupported`, never passing.

#### Scenario: Executable-looking test artifact

- GIVEN a test artifact contains command text and valid result data
- WHEN it is ingested
- THEN no command is executed
- AND only parsed result data and its provenance appear in evidence

### Requirement: Exit and Error Semantics

`verify` MUST return success only when integrity passes, all required inputs are observed, all required analyzers complete, and no configured critical deterministic violation exists. Ambiguous linkage, unsupported optional analysis, and missing optional evidence MUST warn without failing. Internal errors, malformed required inputs, and indeterminate required checks MUST fail closed and remain distinguishable from policy violations.

#### Scenario: Optional unknown versus required unknown

- GIVEN one optional analyzer is unsupported and one required analyzer is indeterminate
- WHEN verification completes
- THEN both unknowns are visible
- AND the command fails because the required check is indeterminate

### Requirement: Deterministic Reports

Markdown and self-contained HTML reports MUST derive from the same manifest, contain equivalent facts, escape untrusted text, identify the bundle digest, and use deterministic ordering. HTML MUST have no external network, script, font, image, or stylesheet dependency. Reports MUST state that hashes detect post-capture change but do not prove authenticity, authorship, completeness, correctness, or safety.

#### Scenario: Offline deterministic rendering

- GIVEN one canonical manifest and renderer version
- WHEN each report format is rendered twice without network access
- THEN each format is byte-identical across runs
- AND hostile markup is displayed as text rather than executed

### Requirement: Local Retention

Normalized evidence and manifests MUST remain local unless explicitly exported. Retention and purge selection MUST be configurable by age or bundle, previewable before deletion, and auditable by counts; default exports MUST exclude raw evidence.

#### Scenario: Retention preview

- GIVEN bundles inside and outside the configured retention window
- WHEN purge preview runs
- THEN it reports exact candidate counts without deleting data
