# Impact Analysis Specification

## Purpose

Define bounded, deterministic impact and risk observations without claiming architectural truth.

## ADDED Requirements

### Requirement: Supported Analysis

For each changed file in a supported language, analysis MUST report the language/parser version, examined paths and symbols, direct syntactic dependents found within configured repository scope, deterministic risk findings, and limitations. Results MUST be observations, not claims of runtime reachability, safety, or complete architecture.

#### Scenario: Supported changed symbol

- GIVEN a changed symbol and two direct dependents in supported files within scope
- WHEN impact analysis completes
- THEN both dependents, their evidence locations, and parser version are reported
- AND transitive or runtime impact is not asserted without evidence

### Requirement: Bounded Operation

Analysis MUST enforce and record configured limits for files, bytes, symbols, edges, depth, and elapsed time. Defaults MUST be 20,000 files, 512 MiB parsed bytes, 500,000 symbols, 1,000,000 edges, depth 5, and 60 seconds. Reaching any limit MUST stop expansion deterministically, preserve completed observations, and mark the result incomplete with the exact limit reached.

#### Scenario: Graph edge limit reached

- GIVEN analysis reaches the configured edge limit
- WHEN results are emitted
- THEN completed observations remain available
- AND completeness is `unknown` with the edge limit and observed count reported

### Requirement: Unsupported and Failed Analysis

Unsupported languages MUST produce `unsupported` records per changed file and warnings when analysis is optional. A required analyzer that is unsupported, crashes, times out, or cannot parse required input MUST fail verification; optional analyzer failure MUST warn and MUST NOT be represented as no impact.

#### Scenario: Unsupported changed language

- GIVEN a changed file uses an unsupported language and impact analysis is optional
- WHEN analysis runs
- THEN the file is marked `unsupported`
- AND the report warns that impact is unknown rather than reporting zero impact

### Requirement: Deterministic Risk Findings

Risk rules MUST identify their rule ID and version, triggering evidence, severity, and result (`violation`, `clear`, or `unknown`). The same canonical inputs, configuration, rule versions, and parser versions MUST yield identical ordered findings. Only configured critical deterministic violations MAY fail verification.

#### Scenario: Critical deterministic violation

- GIVEN canonical input triggers a configured critical rule
- WHEN analysis and verification run
- THEN the finding includes reproducible evidence and rule version
- AND verification fails for that violation without claiming the change is otherwise unsafe
