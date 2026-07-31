# Git Linkage Specification

## Purpose

Define evidence association with Git state while explicitly avoiding authorship claims.

## ADDED Requirements

### Requirement: Linkage Basis and Confidence

A linkage result MUST identify the repository, target commit or range, observed HEAD, working-tree state, association basis, confidence level (`high`, `medium`, `low`, or `unknown`), and reasons. Reports MUST describe linkage as association and MUST NOT infer author, agent authorship, safety, or completeness from a match.

#### Scenario: Exact clean association

- GIVEN evidence records the same repository identity and commit as a clean checkout
- WHEN linkage is evaluated
- THEN the target association is `high` confidence with its matching evidence listed
- AND no authorship statement is emitted

### Requirement: Dirty Working Tree

Uncommitted tracked, staged, or untracked changes MUST be represented separately from committed changes. A dirty tree MUST prevent `high` confidence unless all relevant dirty paths and content digests were captured; uncaptured relevant dirt MUST make linkage `unknown`.

#### Scenario: Uncaptured dirty path

- GIVEN the worktree contains a relevant changed file absent from captured evidence
- WHEN linkage is evaluated
- THEN confidence is `unknown`
- AND the omitted path is reported without attributing its change

### Requirement: Rebase and History Ambiguity

When commit identifiers no longer match after rebase, squash, cherry-pick, or history rewrite, the system MAY use content and patch equivalence as evidence but MUST report every candidate and ambiguity. Multiple plausible candidates MUST produce `unknown`; a unique equivalent match MUST be no higher than `medium`.

#### Scenario: Ambiguous rebased commits

- GIVEN captured content matches two commits after history rewriting
- WHEN linkage is evaluated
- THEN both candidates and the comparison basis are reported
- AND linkage confidence is `unknown` and verification emits a warning

### Requirement: Git Errors

Unreadable repositories, invalid revisions, shallow-history gaps, and unavailable objects MUST produce explicit error or unknown records. They MUST NOT be converted into clean, empty, or confidently linked states.

#### Scenario: Required object unavailable

- GIVEN a declared target commit is absent from a shallow clone
- WHEN linkage is evaluated
- THEN the missing object is reported as an unknown linkage reason
- AND no target commit is asserted as equivalent
