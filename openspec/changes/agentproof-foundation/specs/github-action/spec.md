# GitHub Action Specification

## Purpose

Define fork-safe, least-privilege verification delivery for pull requests.

## ADDED Requirements

### Requirement: Least Privilege and Fork Safety

The Action MUST operate with read-only repository permissions by default and MUST NOT require secrets for fork pull requests. It MUST NOT execute pull-request repository code, test commands, hooks, or build scripts. Untrusted pull-request content MUST NOT be evaluated in a privileged context. Any comment-writing mode MUST be explicit, use only the minimum pull-request write permission, and consume only verified generated output.

#### Scenario: Fork pull request

- GIVEN a pull request from a fork with no secrets
- WHEN the Action runs in default mode
- THEN it reads repository and supplied evidence only, executes no repository code, and completes verification
- AND no write permission or secret is requested

### Requirement: Version and Artifact Integrity

The Action release and every third-party Action dependency MUST be pinned to immutable revisions. Generated or downloaded verification artifacts MUST be digest-checked before use. Action, provider adapter, parser, policy, and report versions MUST appear in outputs; unsupported drift MUST be warning or failure according to whether the affected evidence/check is optional or required.

#### Scenario: Artifact digest mismatch

- GIVEN a downloaded manifest differs from its declared digest
- WHEN the Action verifies it
- THEN verification fails before report publication
- AND no success output or comment is produced

### Requirement: Outputs and Comments

The Action MUST expose machine-readable conclusion, bundle digest, required completeness count and percentage, integrity result, critical violation count, warning count, and report artifact locator. Default mode MUST publish outputs and a report artifact but MUST NOT comment. If comments are enabled, it MUST create or update one identifiable comment per pull request and bundle, avoid duplicate comments on rerun, escape untrusted text, and summarize rather than embed raw evidence.

#### Scenario: Deterministic rerun with comments enabled

- GIVEN the same pull request and bundle are verified twice with commenting enabled
- WHEN the second run completes
- THEN one existing AgentProof comment is updated or left unchanged
- AND its facts match the machine-readable outputs without raw evidence

### Requirement: CI Conclusion Semantics

The Action MUST map verification success to success and verification failure to failure. Warning-only conditions MUST produce success with warnings unless repository policy promotes a deterministic condition to a required gate. Cancellation and infrastructure errors MUST remain distinct and MUST NOT be reported as evidence success.

#### Scenario: Ambiguous linkage only

- GIVEN integrity and required completeness pass and linkage is ambiguous
- WHEN the Action concludes
- THEN its conclusion is success with a linkage warning
- AND outputs do not claim authorship or safety
