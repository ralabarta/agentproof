# Audit Remediation Design

**Status:** Approved for implementation
**Date:** 2026-08-14
**Audit source:** [`docs/audits/2026-08-13-commits.md`](../../audits/2026-08-13-commits.md)
**Execution reference:** [`docs/superpowers/plans/2026-08-14-audit-remediation.md`](../plans/2026-08-14-audit-remediation.md)

## Decision summary

AgentProof will remediate AP-001 through AP-014 by replacing PID-file ownership with a permanent, kernel-locked `.agentproof/.record.lock`, correlating live work and published artifacts by stable identities, containing destructive filesystem operations, and strengthening platform, Action, CLI, and fuzz contracts.

The kernel lock is the sole authority for record ownership. Lock metadata supports diagnostics and run correlation but never grants or transfers ownership. The lock path remains present after release. Purge and report publication validate at-rest topology before reading or deleting data.

These guarantees apply to supported local filesystems. Active malicious concurrent filesystem replacement remains a residual risk and is out of scope beyond at-rest topology validation.

## Goals

1. Ensure that exactly one local `agentproof record` process owns recording rights.
2. Prevent final-component symlink or reparse traversal when opening the record lock.
3. Correlate active ownership with the exact run directory through `runID`.
4. Keep purge traversal and deletion inside `.agentproof/runs` under validated at-rest topology.
5. Bound lifecycle-state reads and reject unsafe file types.
6. Forward targeted Unix termination signals to the recorded child process group.
7. Prevent status from combining evidence and attestation from different publication generations.
8. Prevent Action Step Summary publication from following a final report symlink or hiding read failures.
9. Restore completion and doctor output to the actual CLI contract.
10. Convert fuzz targets from panic-only checks into behavioral contracts.
11. Preserve existing CLI exit classes, preview-first purge behavior, and supported platform behavior.

## Non-goals

- Providing distributed locking or network-filesystem consistency.
- Defending against an attacker who continuously replaces path components during an operation.
- Implementing descriptor-relative traversal and deletion across all supported platforms.
- Turning evidence and attestation publication into a multi-file atomic transaction.
- Redesigning the CLI command registry or completion architecture.
- Guaranteeing Unix-equivalent signal delivery on Windows.
- Expanding Action permissions, secret access, or repository write access.
- Treating lock metadata, PID liveness, timestamps, or path existence as ownership authority.
- Migrating historical run directories or rewriting historical evidence bundles.

## Audit coverage

| Finding | Priority | Required outcome |
|---|---:|---|
| AP-001 | P1 | Use one nonblocking, OS-backed exclusive lock with a single winner. |
| AP-002 | P1 | Open `.record.lock` without following its final symlink or reparse point. |
| AP-003 | P1 | Reject an at-rest symlinked or out-of-tree runs parent before purge. |
| AP-004 | P2 | Read `state.json` through a no-follow, regular-file, 64 KiB-bounded reader. |
| AP-005 | P2 | Replace PID-only liveness with kernel ownership plus versioned metadata. |
| AP-006 | P2 | Forward targeted Unix wrapper signals to the child process group. |
| AP-007 | P2 | Reject a final report symlink before Step Summary publication. |
| AP-008 | P2 | Use attestation time only when evidence and attestation bundle IDs match. |
| AP-009 | P2 | Preserve only the `recording` directory whose `runID` matches the live lock. |
| AP-010 | P2 | Register directly sourced zsh completion with `compdef`. |
| AP-011 | P3 | Recommend the valid preview command `agentproof purge --runs`. |
| AP-012 | P3 | Include `purge --runs` in generated completions and contract checks. |
| AP-013 | P2 | Add executable Action contracts for summary and exit behavior. |
| AP-014 | P3 | Add deterministic, bounded, semantic fuzz oracles. |

## Requirements

### Record ownership

- The AgentProof runtime must remain Go 1.22+ standard-library-only; lock support must not add an external module dependency.
- `.agentproof/.record.lock` must be a permanent regular file.
- Acquisition must request an exclusive OS lock in nonblocking mode.
- Kernel lock state must determine whether a record is active.
- A second recorder must receive the existing usage-class “another record” failure.
- Metadata must be written only after successful kernel-lock acquisition.
- A process crash must release ownership through kernel handle closure.
- Normal release must unlock and close only the owning handle.
- Release must never unlink, rename, truncate, or otherwise delete the lock path.
- One owner must not be able to release a lock acquired by another owner.

### Identity and lifecycle

- The recorder must generate `runID` before lock acquisition.
- The same `runID` must appear in lock metadata and `runs/<runID>/state.json`.
- `ownerID` must contain 128 bits generated with `crypto/rand` and encoded as lowercase hexadecimal.
- Status probing must return active kernel state independently from metadata validity.
- Purge and doctor must exempt or count only a `recording` run whose directory ID matches valid metadata on the active lock.
- Stale metadata on an acquirable lock file must not make any run active.
- Invalid metadata must not replace kernel state or broaden liveness to every recording directory.

### Filesystem safety

- Lock metadata and run-state reads must be bounded to 64 KiB plus one sentinel byte for oversize detection.
- A bounded reader must reject final symlinks, reparse points where applicable, and non-regular files.
- `.agentproof/runs` must resolve inside the repository metadata tree at rest.
- `.agentproof/runs` itself must be a real directory, not a symlink.
- A purge candidate must be a direct, non-symlink child of the validated runs parent.
- Purge must revalidate the runs parent and final candidate path immediately before confirmed deletion.
- Rejected, malformed, unreadable, or oversized lifecycle state must remain unselected and produce an explicit failure.
- Direct child symlinks must continue to be skipped.
- Removing a final symlink must never traverse into or remove its target.

### Publication and operator interfaces

- Status must source `LastStatus` and `LastBundleID` from evidence.
- Status may populate `LastVerifiedAt` only from an attestation with the same nonempty `bundle_id`.
- A missing, malformed, empty, or mismatched attestation must not supply a timestamp.
- Step Summary publication must run under `if: always()` without changing verification exit-code preservation.
- Directly sourced zsh completion must define `_agentproof` and register it with `compdef _agentproof agentproof`.
- Sourcing zsh completion must not invoke `_agentproof`.
- Completion options for `purge` must match the parser and include `--runs`.
- Doctor must recommend preview-first `agentproof purge --runs`, not an invalid selector or immediate confirmation.

## Architecture

The remediation keeps policy in platform-neutral packages and isolates OS primitives behind build-tagged files. The AgentProof runtime remains Go 1.22+ standard-library-only: lock implementations use `syscall` and narrowly scoped `unsafe`, with separate GOOS files wherever syscall constants, structures, or entry points differ. No external module is introduced for locking.

```text
record startup
  -> generate runID and ownerID
  -> acquire platform lock
  -> write locked metadata
  -> create runs/<runID>/state.json
  -> start child with platform process controls
  -> publish terminal state
  -> release handle without deleting .record.lock

status / doctor / purge
  -> probe kernel lock
  -> bounded-read metadata
  -> correlate active metadata.runID
  -> inspect bounded run state
  -> report, count, select, or delete according to exact identity
```

### Components

| Component | Responsibility |
|---|---|
| `internal/record/lock.go` | Shared metadata model, validation, status model, owner-ID generation, and errors. |
| `internal/record/lock_linux.go` | Linux `syscall` no-follow open, regular-file check, `flock`, sync, unlock, and close. |
| `internal/record/lock_darwin.go` | Darwin-specific `syscall` constants, structures, and lock operations behind a GOOS tag. |
| `internal/record/lock_freebsd.go` | FreeBSD-specific `syscall` constants, structures, and lock operations behind a GOOS tag. |
| `internal/record/lock_windows.go` | Windows `syscall` handles plus narrowly scoped `unsafe` calls for reparse inspection and byte-range locking. |
| `internal/record/process_unix.go` | Dedicated process-group setup and explicit signal forwarding. |
| `internal/record/process_windows.go` | Windows process setup and explicit best-effort forwarding result. |
| `internal/purge/state.go` | No-follow, regular-file, size-bounded lifecycle-state decoding. |
| `internal/purge/purge.go` | Parent containment, run selection, identity-aware liveness, and pre-delete rechecks. |
| `internal/doctor` | Exact-run stuck-state diagnosis and valid remediation guidance. |
| `internal/status` | Evidence/attestation generation correlation. |
| `scripts/write-step-summary.sh` | Fail-closed report-to-Step-Summary publication. |
| `internal/actioncontract` | Executable composite Action and summary-script contracts. |
| `internal/completion` | Shell registration and parser-aligned completion options. |
| Fuzz targets | Determinism, bounds, state validity, redaction, and semantic findings. |

## Lock state machine

| State | Kernel probe | Metadata | Meaning | Allowed transition |
|---|---|---|---|---|
| Path absent | Acquirable after safe create | None | No active owner; first acquisition creates the permanent file. | Acquire to `Held valid`. |
| Idle | Acquirable | Valid or stale | No active owner; retained bytes are diagnostic only. | Acquire to `Held valid`. |
| Held valid | Contended | Valid | Active owner and correlatable run. | Owner release or crash to `Idle`. |
| Held invalid | Contended | Missing, malformed, or unsupported | Active owner with unavailable correlation. | Owner release or crash to `Idle`; contenders remain blocked. |
| Unsafe topology | Not attempted or rejected | Untrusted | Final path is a symlink, reparse point, or non-regular object. | Operator repairs topology; acquisition fails closed. |
| I/O failure | Indeterminate | Unavailable | Ownership cannot be established safely. | Report an operational error; do not infer active or inactive. |

Acquisition follows one direction: safe open, opened-handle type validation, nonblocking kernel lock, metadata replacement while locked, synchronization, and handle retention. Release unlocks and closes that retained handle. There is no stale-owner takeover path and no path deletion on release.

## Lock metadata schema

```json
{
  "version": 1,
  "ownerID": "0123456789abcdef0123456789abcdef",
  "runID": "20260814T120000.000000000Z",
  "pid": 1234,
  "acquiredAt": "2026-08-14T12:00:00Z"
}
```

| Field | Contract |
|---|---|
| `version` | Integer `1`; unknown versions are invalid diagnostic metadata. |
| `ownerID` | Exactly 32 lowercase hexadecimal characters from 128 random bits. |
| `runID` | Nonempty ID generated before acquisition and reused for the run directory. |
| `pid` | Positive process ID for diagnostics only. |
| `acquiredAt` | Valid UTC timestamp recording successful acquisition. |

Readers must reject malformed JSON, unknown versions, malformed owner IDs, empty run IDs, nonpositive PIDs, invalid timestamps, and content larger than 64 KiB. Such rejection does not override a contended kernel lock.

## Platform boundaries

### Unix

- Linux, Darwin, and FreeBSD use separate explicit GOOS build tags so each file compiles only its target's standard-library `syscall` constants, structures, and APIs.
- Open uses `syscall.Open` with `O_RDWR | O_CREAT | O_CLOEXEC | O_NOFOLLOW`.
- `syscall.Fstat` on the opened descriptor must confirm a regular file.
- Acquisition uses `syscall.Flock(LOCK_EX | LOCK_NB)`.
- Metadata replacement, file synchronization, and ownership all occur while the descriptor remains locked.
- Release uses `syscall.Flock(LOCK_UN)` and closes the descriptor without unlinking the path.
- The child starts in a dedicated process group.
- Targeted `SIGINT` or `SIGTERM` is forwarded to that group before the wrapper re-raises the signal.

### Windows

- Open uses standard-library `syscall.CreateFile` with `OPEN_ALWAYS` and reparse-aware flags.
- Procedures not directly wrapped by the standard library are resolved from `kernel32.dll` with `syscall.NewLazyDLL`/`NewProc`; target-defined structures are passed through narrowly scoped `unsafe` pointers.
- Attribute and tag inspection must reject reparse points and non-disk files before locking.
- Acquisition uses `LockFileEx` with exclusive and fail-immediately flags over a fixed byte range.
- Metadata truncation and writing occur only while the handle owns the byte-range lock.
- Release uses `UnlockFileEx` and closes only the owning handle; it never deletes the path.
- Process-signal support reports explicit best-effort behavior when console delivery is unavailable.
- Documentation and tests must not promise Unix-equivalent process-group semantics on Windows.

## RunID liveness flow

1. Generate `runID` and `ownerID`.
2. Acquire the kernel lock and publish metadata containing that `runID`.
3. Create and update only `runs/<runID>/state.json` for this record.
4. Probe liveness through a nonblocking lock attempt when status, doctor, or purge needs ownership state.
5. If the probe is acquirable, report inactive regardless of retained metadata.
6. If the probe is contended and metadata is valid, report active with the exact `runID`.
7. If the probe is contended and metadata is invalid, report active without a correlatable run.
8. Purge exempts only an old `recording` directory matching the active, valid `runID`.
9. Doctor counts only that matching directory as live; unrelated recording directories remain independently diagnosable.

## Purge containment and bounded reads

Purge preserves preview-first selection. It validates before selection and repeats path validation before confirmed deletion.

1. Resolve and validate `.agentproof/runs` within `.agentproof` at rest.
2. Reject a symlinked, reparse-point, non-directory, or out-of-tree runs parent.
3. Enumerate only direct children.
4. Skip direct child symlinks and reject unsafe candidate types.
5. Open `state.json` without following its final component.
6. Validate the opened handle as a regular file.
7. Read at most 64 KiB plus one byte; reject input when the sentinel byte exists.
8. Decode only lifecycle fields needed for state, age, and run identity.
9. Treat unsafe or invalid state as unselected and record a failure.
10. Apply age and terminal-state selection, exempting only the exact live `runID`.
11. Present the unchanged preview before confirmation.
12. Revalidate parent containment and final candidate topology immediately before deletion.
13. Delete only the revalidated direct child.

The design preserves the existing bounded claim that `RemoveAll` removes a final symlink itself rather than traversing its target. It does not generalize that claim to an actively changing ancestor path.

## Signal behavior

On Unix, the wrapper creates a dedicated child process group. When the wrapper alone receives a supported termination signal, it must:

1. record the run as abandoned;
2. forward the same signal to the child process group;
3. preserve forwarding errors as operational diagnostics;
4. re-raise the signal in the wrapper so existing signal exit semantics remain intact.

The abandoned-state write must precede wrapper termination. Tests must use a child that spawns a grandchild and target only the wrapper PID, proving group forwarding rather than direct-child signaling. Ordinary foreground terminal signaling remains compatible but is not the only supported path.

## Publication correlation

Evidence remains the source of `LastStatus` and `LastBundleID`. Attestation contributes `LastVerifiedAt` only when both documents decode, both `bundle_id` values are nonempty, and the values are equal.

A mismatch represents an incomplete publication window. Status must keep the evidence status and bundle ID but suppress the attestation timestamp. Missing or malformed attestation retains existing tolerant status behavior and must not fabricate correlation.

## Action Step Summary safety

The Action must delegate publication to `scripts/write-step-summary.sh` and invoke it through `${{ github.action_path }}` under `if: always()`.

The script must:

- read only `.agentproof/report.md`;
- append only to `$GITHUB_STEP_SUMMARY`;
- return success without writing when the report is absent;
- reject a final report symlink before reading;
- require a regular, readable report;
- append the heading and report through checked operations;
- propagate open, read, and append failures;
- never print report contents to standard output or standard error.

The verification step must continue to capture its status in `verify_code` and exit with that exact code after publication-related setup. Summary execution must not convert a policy failure into success or hide a publication failure.

This control rejects the at-rest final report symlink. It does not claim protection against every active local replacement race.

## Completion and doctor corrections

- Zsh output defines `_agentproof` and then emits `compdef _agentproof agentproof`.
- Zsh output never executes `_agentproof "$@"` while sourced.
- Purge completion includes `--raw`, `--runs`, `--older-than`, and `--confirm` consistently with the parser.
- A contract test compares generated purge options with accepted parser options.
- Doctor recommends `agentproof purge --runs` as the preview command.
- Doctor does not recommend `--confirm` before the operator reviews selection.

## Error model

| Condition | Required behavior |
|---|---|
| Kernel lock contended | Return the existing usage-class concurrent-record error. |
| Unsafe lock topology | Fail closed without reading or modifying the target. |
| Lock metadata invalid while held | Report active ownership with unavailable correlation; do not infer a run ID. |
| Lock metadata stale while acquirable | Report inactive; retained metadata is diagnostic only. |
| Lock probe I/O failure | Return an operational error; do not guess ownership. |
| Runs parent unsafe or outside metadata | Abort run purge and preserve all candidates. |
| State file unsafe, oversized, or malformed | Keep that run unselected and report a failure. |
| Candidate changes before deletion | Refuse that deletion and report a failure. |
| Report absent | Skip Step Summary publication successfully. |
| Report unsafe or unreadable | Fail publication without disclosing report bytes. |
| Publication bundle mismatch | Suppress `LastVerifiedAt`; keep evidence-derived fields. |
| Unsupported Windows signal delivery | Return explicit best-effort diagnostics without Unix-equivalent claims. |

## Compatibility

- Existing command names, selectors, defaults, and exit-code classes remain stable.
- `agentproof purge --runs` remains preview-only unless `--confirm` is supplied.
- The persistent lock file is a compatible internal state change; users must not delete it as a release operation.
- Existing stale lock metadata may remain readable, but only version 1 valid metadata participates in correlation.
- Existing run directories require no migration.
- Existing evidence remains authoritative for status and bundle ID.
- Missing or malformed attestation remains tolerated without a verified timestamp.
- Direct child symlink skipping and final-symlink removal semantics remain unchanged.
- Unix behavior is implemented for Linux, Darwin, and FreeBSD build targets in the execution plan.
- Windows amd64 receives native compile coverage and runtime lock tests.

## Local-filesystem support and residual risk

The ownership and containment contracts require local kernel lock semantics and stable at-rest path inspection. Network, clustered, virtual, or distributed filesystems that weaken `flock`, `LockFileEx`, no-follow, reparse, or rename semantics are unsupported.

Active malicious concurrent filesystem replacement is a residual risk and is explicitly out of scope beyond at-rest topology validation. In particular, an attacker with local write access may replace an ancestor or final component after validation and before a later path-based operation. Closing this gap requires descriptor-relative traversal and deletion with platform-specific primitives and is not part of AP-001 through AP-014 remediation.

## Test contracts

### Lock and process tests

- A synchronized multi-process race produces exactly one lock winner.
- A losing contender receives the existing concurrent-record error.
- A crashed holder releases the kernel lock automatically.
- Release leaves a regular `.record.lock` in place.
- A final lock symlink or Windows reparse point is rejected without modifying its target.
- One handle cannot release another handle's ownership.
- Status reports active only from kernel contention and returns valid metadata separately.
- Unix targeted `SIGINT` and `SIGTERM` reach child and grandchild through the process group.
- Abandoned state is durable before wrapper signal termination.
- Windows runs native contention, crash-release, persistence, and unsafe-topology tests.

### Purge, status, CLI, and Action tests

- A symlinked or out-of-tree runs parent preserves an external sentinel tree.
- Symlinked and oversized `state.json` files remain unselected.
- A changed final candidate path is rejected before deletion.
- Unrelated old recording runs remain selectable while another run is live.
- The exact live run remains exempt.
- Doctor counts only the exact matching live recording run.
- Evidence A with attestation B suppresses verification time; A with A accepts it.
- Step Summary tests cover success, policy failure, absence, unreadability, final symlink rejection, `if: always()`, and verify exit-code preservation.
- Completion tests cover direct zsh sourcing, non-invocation, `--runs`, and parser alignment.
- Doctor tests require the valid preview recommendation.

### Fuzz contracts

| Target | Required properties |
|---|---|
| `FuzzRedact` | No panic, deterministic output, idempotence, and removal of complete seeded secret tokens. |
| `FuzzScanPatch` | No panic, deterministic findings, and exactly one visible `AP-SCAN-001` unknown result for any line over `maxPatchLine`. |
| `FuzzSummarize` | Deterministic summary/error class, bounded oversized-input rejection, and nonnegative normalized counters. |
| `FuzzIngest` | Deterministic result and record state, valid state enumeration, nonnegative counts, and bounded oversized-artifact rejection. |

Filesystem topology, symlink resistance, and no-execution behavior remain unit or subprocess contracts rather than per-iteration fuzz mutations.

## Acceptance criteria

- [ ] AP-001 through AP-014 each map to a passing regression or behavioral contract.
- [ ] AgentProof remains Go 1.22+ standard-library-only, with no external locking dependency.
- [ ] Exactly one local process can own the nonblocking record lock.
- [ ] `.record.lock` is permanent, regular, opened without following the final link, and never deleted on release.
- [ ] Kernel state is authoritative; metadata is versioned, bounded, diagnostic, and correlational.
- [ ] Crash release and ownership isolation pass on native supported platforms.
- [ ] Purge rejects unsafe at-rest parent, candidate, and state-file topology.
- [ ] Purge reads no more than 64 KiB of lifecycle state and reports rejected state.
- [ ] Purge and doctor correlate liveness only through the exact active `runID`.
- [ ] Targeted Unix wrapper signals reach the child process group after abandoned-state publication.
- [ ] Status never combines evidence with another bundle generation's attestation time.
- [ ] Step Summary rejects final report symlinks and preserves verification exit behavior.
- [ ] Zsh direct sourcing registers completion, purge includes `--runs`, and doctor guidance is valid.
- [ ] All four fuzz targets retain no-panic coverage and enforce their semantic properties.
- [ ] Race tests, vet, build, Unix cross-compilation, Windows cross-compilation, and native Windows lock tests pass.
- [ ] Documentation states the local-filesystem boundary and active replacement residual without broader claims.

## Rollout and commit strategy

Implementation follows the detailed plan in dependency order. Each boundary must be independently testable and use a conventional commit without AI attribution:

1. `fix(record): replace PID lock with kernel-backed ownership`
2. `fix(purge): correlate recording liveness by run ID`
3. `fix(purge): contain run deletion and bound state reads`
4. `fix(record): forward targeted signals to child processes`
5. `fix(status): correlate evidence and attestation bundles`
6. `fix(action): harden and test step summary publication`
7. `fix(cli): repair completion and purge guidance contracts`
8. `test(fuzz): assert behavioral parser properties`
9. `docs: clarify record and purge safety boundaries`

Each implementation boundary starts with the named failing contract tests from the plan, adds the smallest production change, and ends with focused race-enabled package tests. Source-mutating normalization occurs before final check-only formatting, full race tests, vet, build, cross-compilation, native Windows validation, and the four bounded fuzz runs.

The rollout requires no data migration or feature flag. Compatibility is established through the regression matrix before documentation claims are updated. A failed platform or containment gate blocks release of the remediation rather than weakening the contract.