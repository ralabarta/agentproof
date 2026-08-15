# Audit Remediation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remediate every confirmed finding AP-001 through AP-014 from [`docs/audits/2026-08-13-commits.md`](../../audits/2026-08-13-commits.md) without expanding the threat model beyond supported local-filesystem guarantees.

**Architecture:** Replace the PID file protocol with a permanent regular `.record.lock` whose OS lock is authoritative and whose versioned JSON metadata identifies the owner and run. Build retention, doctor, and status behavior on correlated identities rather than timing or global liveness. Harden at-rest filesystem reads and Action publication, preserve explicit residual-risk boundaries, and add strict RED/GREEN tests before each implementation change.

**Tech Stack:** Go 1.22+ standard library only, build-tagged `syscall`/`unsafe` platform primitives, subprocess tests, Go fuzzing, Bash, GitHub composite Actions.

**Command verification status:** The existing baseline commands were verified by the audit run. Commands that target files or behavior created by this plan cannot be executed before implementation; their package paths and flags were checked against the current Go 1.22 project layout, and each step states its expected RED or GREEN result.

---

## Task Tracking

At the start of implementation, use `TaskCreate` to create one task for each step:

1. Implement the permanent OS-backed record lock.
2. Make run liveness identity-aware.
3. Contain and bound purge reads and deletion.
4. Forward targeted wrapper signals to the child.
5. Correlate status publication generations.
6. Harden and contract-test Action Step Summary behavior.
7. Repair zsh completion, purge completion, and doctor guidance.
8. Add behavioral fuzz properties.
9. Update security and lifecycle documentation.
10. Run full verification and `/finalize`.

## Skills

After plan approval and before making edits, run `/superpowers:test-driven-development`, `/go-testing`, `/code-style`, and `/careful`. Run `/finalize` only after all source-mutating formatters have converged and the full verification matrix is green.

## Existing patterns

| Pattern | Existing location | Alignment |
|---|---|---|
| Platform-specific process behavior behind build-tagged files | `internal/record/raise_unix.go`, `internal/record/raise_windows.go` | Follow this split for Unix and Windows lock/process primitives. Keep shared policy in platform-neutral files. |
| Symlink-resistant atomic publication | `internal/safefile/write.go`, `internal/safefile/write_test.go` | Reuse `safefile.Contained` where it answers the containment question. Do not treat atomic rename as inter-file transactionality. |
| Subprocess helper tests | `internal/record/record_test.go` (`TestRecordHelper`) | Extend this pattern for lock contention, crash release, and targeted-signal behavior. |
| Preview-first destructive operations | `internal/purge/purge.go`, `internal/purge/purge_test.go` | Preserve selection before `Confirm`; add containment and identity checks without changing preview semantics. |
| Per-package tests with named behavior | `internal/status/status_test.go`, `internal/completion/completion_test.go`, `internal/doctor/doctor_test.go` | Add narrow regression cases beside the implementation. |
| Fuzzing hostile data through real entry points | `internal/scan/fuzz_test.go`, `internal/session/fuzz_test.go`, `internal/testresult/fuzz_test.go` | Keep no-panic assertions and add stable semantic invariants. |

**Intentional deviation:** The existing completion command table is manually synchronized with `internal/app`. This plan does not introduce a CLI registry refactor; it adds explicit contract assertions for the confirmed drift and keeps the remediation bounded.

## Fixed architectural decisions

### Permanent record lock contract

`.agentproof/.record.lock` is a permanent regular file on a local filesystem.

- Acquisition opens the final path without following a symlink, verifies a regular file, and requests an OS-backed exclusive lock in nonblocking mode.
- Linux, Darwin, and FreeBSD use explicit GOOS-tagged files with the standard library `syscall` package for no-follow open, descriptor validation, and nonblocking `flock`; keep targets separate where constants, structures, or syscall APIs differ.
- Windows uses the standard library `syscall` package for file handles and `syscall.NewLazyDLL`/`NewProc` plus `unsafe` for the required `LockFileEx`, `UnlockFileEx`, and reparse-inspection calls that the standard library does not wrap directly.
- No external module may be added for locking; the Go 1.22+ runtime remains standard-library-only.
- The kernel lock is authoritative. Metadata is diagnostic and correlational only.
- Metadata is written only while the kernel lock is held and has this schema:

```json
{
  "version": 1,
  "ownerID": "128-bit-random-hex",
  "runID": "20260814T120000.000000000Z",
  "pid": 1234,
  "acquiredAt": "2026-08-14T12:00:00Z"
}
```

- A crash releases the kernel lock when the process handle closes.
- Normal release unlocks and closes the handle but never removes `.record.lock`.
- Stale metadata may remain after release; readers must probe the kernel lock before declaring a record active.
- Guarantees apply to supported local filesystems. Network or distributed filesystems with non-local lock semantics are outside the contract.

### Purge containment contract

- At rest, `.agentproof/runs` must resolve inside the repository metadata tree and must itself be a real directory, not a symlink.
- Each selected run remains a direct, non-symlink child; `state.json` must be a no-follow regular file no larger than 64 KiB.
- Re-check the runs parent and final run path immediately before confirmed deletion.
- Preserve the current true claim that direct child symlinks are skipped and that `RemoveAll` removes a final symlink rather than traversing its target.
- Active local concurrent replacement of path components remains a documented residual risk. Closing that gap requires descriptor-relative, platform-specific traversal/deletion and is out of scope for this remediation.

## Step 1: Implement the permanent OS-backed record lock (AP-001, AP-002, AP-005)

**Files:**

- Create: `internal/record/lock.go`
- Create: `internal/record/lock_linux.go`
- Create: `internal/record/lock_darwin.go`
- Create: `internal/record/lock_freebsd.go`
- Create: `internal/record/lock_windows.go`
- Create: `internal/record/lock_test.go`
- Modify: `internal/record/record.go:35-57,235-270`
- Modify: `.github/workflows/ci.yml`

- [ ] **Write RED metadata and lifecycle tests.**

Add tests named:

- `TestRecordLockMetadataContract`
- `TestRecordLockPersistsAfterRelease`
- `TestRecordLockRejectsFinalSymlink`
- `TestRecordLockCrashAutoReleases`
- `TestRecordLockConcurrentAcquisitionHasSingleWinner`
- `TestRecordLockReleaseCannotUnlockAnotherOwner`

Use a start barrier and `os/exec` helper processes for contention. Assert exactly one concurrent process acquires the lock, the loser receives the existing usage-class “another record” error, a crashed holder releases the kernel lock, release leaves a regular `.record.lock`, and a symlink target's bytes remain unchanged.

- [ ] **Run RED tests.**

```bash
go test ./internal/record -run 'TestRecordLock(MetadataContract|PersistsAfterRelease|RejectsFinalSymlink|CrashAutoReleases|ConcurrentAcquisitionHasSingleWinner|ReleaseCannotUnlockAnotherOwner)$' -count=1
```

Expected RED behavior: current acquisition permits a race, deletes the path on release, follows the symlink, has PID-only metadata, or cannot demonstrate kernel crash release.

- [ ] **Add the shared lock model.**

In `lock.go`, define the versioned metadata, held-lock type, metadata validation, 128-bit `crypto/rand` owner-ID generation, and shared errors. Keep `acquireRecordLock(root, runID)` and `statusRecordLock(root)` platform-neutral; dispatch only open/lock/unlock primitives to tagged files. Bound metadata reads to 64 KiB and reject unknown versions, malformed IDs, nonpositive PIDs, and empty run IDs as diagnostic metadata without overriding kernel state.

- [ ] **Implement build-tagged Unix acquisition.**

Use separate `lock_linux.go`, `lock_darwin.go`, and `lock_freebsd.go` files with explicit GOOS build tags so target-specific `syscall` constants, structures, and APIs are compiled only on their target. Use standard-library `syscall.Open` with `O_RDWR|O_CREAT|O_CLOEXEC|O_NOFOLLOW`, `syscall.Fstat` regular-file validation, and `syscall.Flock(fd, LOCK_EX|LOCK_NB)`. Write metadata only after acquiring the lock; synchronize, retain the open descriptor for ownership, and unlock/close without unlinking. Do not add a module dependency.

- [ ] **Implement Windows acquisition.**

In `lock_windows.go`, use standard-library `syscall.CreateFile` with `OPEN_ALWAYS` and reparse-aware flags. Resolve the required `LockFileEx`, `UnlockFileEx`, and reparse-inspection procedures with `syscall.NewLazyDLL`/`NewProc`, passing only target-defined structures through `unsafe` pointers. Inspect file attributes/tags, reject reparse points and non-disk files, then acquire an exclusive fail-immediately lock over a fixed byte range. Truncate/write metadata only while locked. Release by unlocking and closing the owning handle, never deleting the path.

- [ ] **Integrate acquisition into record startup.**

Generate the run ID before lock acquisition. Acquire once, defer handle release, and use the same run ID for lock metadata and `runs/<runID>/state.json`. Remove PID-liveness takeover and unconditional path deletion. Keep user-facing contention classified as usage failure.

- [ ] **Run GREEN tests and race coverage.**

```bash
go test -race ./internal/record -run 'TestRecordLock' -count=1
```

Expected GREEN behavior: one owner, nonblocking contention, immutable symlink target, persistent regular path, crash auto-release, and handle-scoped release.

- [ ] **Cross-compile every platform implementation.**

```bash
GOOS=linux GOARCH=amd64 go test -c -o /tmp/agentproof-record-linux.test ./internal/record
GOOS=darwin GOARCH=amd64 go test -c -o /tmp/agentproof-record-darwin.test ./internal/record
GOOS=freebsd GOARCH=amd64 go test -c -o /tmp/agentproof-record-freebsd.test ./internal/record
GOOS=windows GOARCH=amd64 go test -c -o /tmp/agentproof-record-windows.test.exe ./internal/record
```

Expected: all four target-specific test binaries compile against the Go 1.22+ standard library with no added module dependency. The matrix is mandatory: Linux amd64, Darwin amd64, FreeBSD amd64, and Windows amd64. Add a native Windows CI job named `windows-record-lock` to `.github/workflows/ci.yml` with `if: github.event_name == 'pull_request'`. The job must use `actions/checkout` with `ref: ${{ github.event.pull_request.head.sha }}`, then assert `git rev-parse HEAD` equals `${{ github.event.pull_request.head.sha }}` before running `go test ./internal/record`. This runtime-verifies contention, crash release, permanent-file behavior, ownership-safe release, and reparse rejection on the pull request's exact head; cross-building does not satisfy this gate.

- [ ] **Commit boundary:** `fix(record): replace PID lock with kernel-backed ownership`

## Step 2: Make run liveness identity-aware (AP-005, AP-009)

**Files:**

- Modify: `internal/record/lock.go`
- Modify: `internal/record/lock_test.go`
- Modify: `internal/purge/purge.go:111-129`
- Modify: `internal/purge/purge_test.go`
- Modify: `internal/doctor/doctor.go:70-86`
- Modify: `internal/doctor/doctor_test.go`

- [ ] **Write RED run-ID correlation tests.**

Add:

- `TestRecordLockStatusReportsOnlyKernelHeldOwner`
- `TestRunsSelectsOldRecordingRunWhileDifferentRunIsLive`
- `TestRunsSparesRecordingRunWithMatchingLiveLock`
- `TestDoctorCountsOnlyMatchingLiveRecordingRun`

The tests must hold the real OS lock in a subprocess. Use one old `recording` run ID and one active run ID to prove that only the matching directory is live.

- [ ] **Run RED tests.**

```bash
go test ./internal/record ./internal/purge ./internal/doctor -run 'Test(RecordLockStatusReportsOnlyKernelHeldOwner|RunsSelectsOldRecordingRunWhileDifferentRunIsLive|RunsSparesRecordingRunWithMatchingLiveLock|DoctorCountsOnlyMatchingLiveRecordingRun)$' -count=1
```

Expected RED behavior: global `LiveRecord` preserves or subtracts unrelated recording runs.

- [ ] **Expose kernel-probed status.**

Return `{Active, Metadata}` from a bounded status probe. `Active` comes from nonblocking kernel-lock contention; metadata supplies the run ID only when valid. Stale metadata with an acquirable kernel lock must report inactive.

- [ ] **Correlate purge and doctor by run ID.**

For `recording` state, compare the directory's run ID with active lock metadata. Purge spares only the matching active run. Doctor subtracts only that matching run from `RecordingRuns`; malformed metadata fails closed for deletion and remains visible as a warning rather than being treated as a valid owner.

- [ ] **Run GREEN tests.**

```bash
go test -race ./internal/record ./internal/purge ./internal/doctor -count=1
```

Expected GREEN behavior: unrelated stale recording directories remain eligible while a new record runs, and the active directory is spared.

- [ ] **Commit boundary:** `fix(purge): correlate recording liveness by run ID`

## Step 3: Contain and bound purge operations (AP-003, AP-004)

**Files:**

- Create: `internal/purge/state.go`
- Create: `internal/purge/state_test.go`
- Modify: `internal/purge/purge.go:66-129`
- Modify: `internal/purge/purge_test.go`
- Reuse: `internal/safefile/write.go`

- [ ] **Write RED containment and bounded-read tests.**

Add:

- `TestRunsRejectsSymlinkedRunsParent`
- `TestRunsRejectsRunsParentOutsideMetadataTree`
- `TestRunsRejectsSymlinkedStateFile`
- `TestRunsRejectsOversizedStateFile`
- `TestRunsRechecksFinalPathBeforeDelete`
- `TestRunsContinuesSkippingDirectChildSymlinks`
- `TestRunsRemoveAllFinalSymlinkDoesNotTouchTarget`

The last two tests preserve the audit's refutations. Use an external sentinel directory and assert its files survive every case.

- [ ] **Run RED tests.**

```bash
go test ./internal/purge -run 'TestRuns(RejectsSymlinkedRunsParent|RejectsRunsParentOutsideMetadataTree|RejectsSymlinkedStateFile|RejectsOversizedStateFile|RechecksFinalPathBeforeDelete|ContinuesSkippingDirectChildSymlinks|RemoveAllFinalSymlinkDoesNotTouchTarget)$' -count=1
```

Expected RED behavior: the parent symlink is traversed and `state.json` symlinks or oversized files are read.

- [ ] **Implement the bounded state reader.**

Open `state.json` without following the final component, verify a regular file from the opened handle, read at most 64 KiB plus one sentinel byte, reject oversized content, and decode exactly the lifecycle fields needed by purge. Treat rejected state as unselectable and increment `Failed`; do not silently classify it as dead.

- [ ] **Validate at-rest containment.**

Before enumeration, establish that `.agentproof/runs` is a real directory contained by the repository's `.agentproof` tree. Before confirmed deletion, re-run parent containment and `Lstat` the final run path. If the final path is now a symlink or no longer a direct child directory, count a failure and skip it.

- [ ] **Keep the threat-model boundary explicit in code comments.**

State that these checks protect at-rest parent/final symlinks. Do not claim descriptor-relative race resistance. Active local concurrent replacement remains residual and out of scope unless future platform-specific descriptor-relative traversal/deletion is added.

- [ ] **Run GREEN and race tests.**

```bash
go test -race ./internal/purge -count=1
```

Expected GREEN behavior: external sentinels survive, malformed/unbounded state fails closed, direct child symlinks remain skipped, and preview/age semantics are unchanged.

- [ ] **Commit boundary:** `fix(purge): contain run deletion and bound state reads`

## Step 4: Forward targeted wrapper signals to the child (AP-006)

**Files:**

- Create: `internal/record/process_unix.go`
- Create: `internal/record/process_windows.go`
- Create: `internal/record/process_unix_test.go`
- Modify: `internal/record/record.go:64-132`
- Modify or retire only if redundant: `internal/record/raise_unix.go`, `internal/record/raise_windows.go`

- [ ] **Write RED subprocess tests.**

Add Unix tests:

- `TestTargetedSIGTERMReachesChildProcessGroup`
- `TestTargetedSIGINTReachesChildProcessGroup`
- `TestSignalWritesAbandonedStateBeforeWrapperExit`

The helper child must record receipt of the signal and spawn a grandchild so the test proves process-group forwarding, not only direct-child termination. Signal only the wrapper PID.

- [ ] **Run RED tests on Unix.**

```bash
go test ./internal/record -run 'Test(TargetedSIGTERMReachesChildProcessGroup|TargetedSIGINTReachesChildProcessGroup|SignalWritesAbandonedStateBeforeWrapperExit)$' -count=1
```

Expected RED behavior: targeted wrapper signals do not reliably reach the child tree.

- [ ] **Implement platform process control.**

On Unix, start the child in a dedicated process group and explicitly forward the received signal to that group before the wrapper re-raises it. Preserve the abandoned-state write ordering. On Windows, isolate process setup/forwarding behind the same shared API and return an explicit best-effort result where console signal delivery is unavailable; do not claim Unix-equivalent semantics.

- [ ] **Run GREEN and cross-compile checks.**

```bash
go test -race ./internal/record -run 'Test(TargetedSIG|SignalWritesAbandoned)' -count=1
GOOS=windows GOARCH=amd64 go test -c -o /tmp/agentproof-record-windows.test.exe ./internal/record
```

Expected GREEN behavior: targeted Unix signals reach the child group, state remains abandoned, and Windows code compiles with documented semantics.

- [ ] **Commit boundary:** `fix(record): forward targeted signals to child processes`

## Step 5: Correlate status publication generations (AP-008)

**Files:**

- Modify: `internal/status/status.go:57-83`
- Modify: `internal/status/status_test.go`

- [ ] **Write RED generation tests.**

Add:

- `TestReadCorrelatesEvidenceAndAttestationBundleIDs`
- `TestReadSuppressesTimestampForMismatchedBundle`
- `TestReadAcceptsMatchingBundleGeneration`

Use evidence bundle A with attestation bundle B, then matching A/A. Preserve tests for missing and malformed attestation.

- [ ] **Run RED tests.**

```bash
go test ./internal/status -run 'TestRead(CorrelatesEvidenceAndAttestationBundleIDs|SuppressesTimestampForMismatchedBundle|AcceptsMatchingBundleGeneration)$' -count=1
```

Expected RED behavior: a mismatched attestation timestamp is accepted.

- [ ] **Parse and compare attestation `bundle_id`.**

Populate `LastVerifiedAt` only when evidence and attestation both decode, both bundle IDs are nonempty, and they match. Keep `LastStatus` and `LastBundleID` sourced from evidence. A mismatch represents an in-progress or incomplete publication and must not synthesize a correlated timestamp.

- [ ] **Run GREEN tests.**

```bash
go test -race ./internal/status ./internal/doctor -count=1
```

Expected GREEN behavior: status never combines a bundle with another generation's timestamp.

- [ ] **Commit boundary:** `fix(status): correlate evidence and attestation bundles`

## Step 6: Harden and contract-test Step Summary publication (AP-007, AP-013)

**Files:**

- Create: `scripts/write-step-summary.sh`
- Create: `internal/actioncontract/action_test.go`
- Modify: `action.yml:69-96`

- [ ] **Write RED Action contract tests.**

Add:

- `TestStepSummarySuccess`
- `TestStepSummaryPublishesPolicyFailureReport`
- `TestStepSummaryMissingReportIsNoOp`
- `TestStepSummaryUnreadableReportFails`
- `TestStepSummaryRejectsFinalSymlink`
- `TestActionPreservesVerifyExitCode`
- `TestActionRunsSummaryAlways`

Execute the summary script through Bash in a subprocess. For unreadable behavior, inject a failing `cat` earlier in `PATH` so the test is reliable even under a privileged test user. For Action structure, inspect the named steps and assert that verification captures and exits with `verify_code`, while Step Summary remains `if: always()`.

- [ ] **Run RED tests.**

```bash
go test ./internal/actioncontract -run 'Test(StepSummary|Action)' -count=1
```

Expected RED behavior: no harness/script exists and the current summary follows a final symlink.

- [ ] **Implement a fail-closed summary script.**

The script must:

1. use `.agentproof/report.md` and `$GITHUB_STEP_SUMMARY`;
2. fail before reading when the report's final component is a symlink;
3. return success without writing when the report is absent;
4. require a regular readable report;
5. append the heading and report through a checked read whose failure propagates;
6. avoid printing report contents to standard output or error.

Call the script from the `if: always()` Action step using `${{ github.action_path }}`. Keep the verification step's original exit code behavior unchanged.

- [ ] **Run GREEN contract tests.**

```bash
go test -race ./internal/actioncontract -count=1
```

Expected GREEN behavior: success and policy-failure reports publish; missing reports are no-ops; symlink/unreadable reports fail without disclosure; verification exit preservation remains explicit.

- [ ] **Commit boundary:** `fix(action): harden and test step summary publication`

## Step 7: Repair completion and doctor contracts (AP-010, AP-011, AP-012)

**Files:**

- Modify: `internal/completion/completion.go:20-73`
- Modify: `internal/completion/completion_test.go`
- Modify: `internal/doctor/doctor.go:58-67`
- Modify: `internal/doctor/doctor_test.go`
- Modify: `internal/app/app_test.go`

- [ ] **Write RED CLI contract tests.**

Add:

- `TestGenerateZshRegistersDirectSourceWithCompdef`
- `TestGenerateZshDoesNotInvokeCompletionDuringSource`
- `TestGeneratePurgeIncludesRunsSelector`
- `TestDoctorRecommendsValidRunPurgeCommand`
- `TestCompletionPurgeOptionsMatchPurgeParser`

The zsh test should run `zsh -fc 'autoload -Uz compinit; compinit; source /dev/stdin; (( $+_comps[agentproof] ))'` with generated input when zsh is available, and retain deterministic text assertions for environments without zsh.

- [ ] **Run RED tests.**

```bash
go test ./internal/completion ./internal/doctor ./internal/app -run 'Test(GenerateZsh|GeneratePurge|DoctorRecommends|CompletionPurge)' -count=1
```

Expected RED behavior: no direct-source registration, `--runs` absent, and doctor prints an invalid command.

- [ ] **Fix the contracts.**

Generate `compdef _agentproof agentproof` for zsh after defining `_agentproof`; do not call `_agentproof "$@"` during source. Add `--runs` to the purge command specification. Change doctor guidance to the valid preview command `agentproof purge --runs`; do not add `--confirm` to the first recommendation.

- [ ] **Run GREEN tests.**

```bash
go test -race ./internal/completion ./internal/doctor ./internal/app -count=1
```

Expected GREEN behavior: the documented zsh source path registers completion, all purge selectors match, and doctor recommends a valid preview.

- [ ] **Commit boundary:** `fix(cli): repair completion and purge guidance contracts`

## Step 8: Add behavioral fuzz properties (AP-014)

**Files:**

- Modify: `internal/scan/fuzz_test.go`
- Modify: `internal/session/fuzz_test.go`
- Modify: `internal/testresult/fuzz_test.go`

- [ ] **Strengthen `FuzzRedact` without removing panic coverage.**

Assert idempotence (`RedactString(RedactString(x)) == RedactString(x)`), deterministic output, and removal of any complete known seeded secret token. Keep arbitrary-input no-panic coverage.

- [ ] **Strengthen `FuzzScanPatch`.**

For any patch containing a line longer than `maxPatchLine`, assert exactly one visible `AP-SCAN-001` finding with result `unknown`. Assert deterministic findings for repeated input and preserve no-panic behavior.

- [ ] **Strengthen `FuzzSummarize`.**

Assert deterministic summary/error class for repeated identical bytes, bounded oversized-input rejection, and nonnegative normalized counters. Do not require malformed inputs to succeed.

- [ ] **Strengthen `FuzzIngest`.**

Assert deterministic result/record state for repeated artifacts, valid state enumeration, nonnegative counts, and bounded oversized-artifact rejection. Preserve symlink and no-execution guarantees from unit tests rather than attempting filesystem topology mutation in every fuzz iteration.

- [ ] **Run short RED/GREEN fuzz smoke commands during development.**

```bash
go test ./internal/scan -run '^$' -fuzz FuzzRedact -fuzztime 10s
go test ./internal/scan -run '^$' -fuzz FuzzScanPatch -fuzztime 10s
go test ./internal/session -run '^$' -fuzz FuzzSummarize -fuzztime 10s
go test ./internal/testresult -run '^$' -fuzz FuzzIngest -fuzztime 10s
```

Expected GREEN behavior: all four targets retain no-panic coverage and enforce their new semantic properties.

- [ ] **Commit boundary:** `test(fuzz): assert behavioral parser properties`

## Step 9: Update documented guarantees

**Files:**

- Modify: `README.md`
- Modify: `SECURITY.md`
- Modify: `docs/architecture.md`
- Modify: `docs/threat-model.md`

- [ ] Document the permanent lock path, kernel-authoritative ownership, metadata fields, crash auto-release, and local-filesystem contract.
- [ ] Document run-ID-aware retention and the 64 KiB no-follow lifecycle-state read.
- [ ] State the purge residual precisely: active concurrent component replacement is not descriptor-relative and remains out of scope.
- [ ] Keep the Action claim limited to final-report symlink rejection; do not claim protection against every active local replacement race.
- [ ] Verify every documented command against the CLI tests and completion table.

```bash
go test ./internal/app ./internal/completion ./internal/doctor ./internal/purge ./internal/record -count=1
```

Expected: PASS before the documentation commit.

- [ ] **Commit boundary:** `docs: clarify record and purge safety boundaries`

## Step 10: Full verification and finalization

- [ ] Run source-mutating normalization before final checks.

```bash
gofmt -w internal/record internal/purge internal/status internal/completion internal/doctor internal/actioncontract
```

- [ ] Confirm formatting is converged with a check-only pass.

```bash
test -z "$(gofmt -l internal/record internal/purge internal/status internal/completion internal/doctor internal/actioncontract)"
```

- [ ] Run the focused package matrix.

```bash
go test -race ./internal/record ./internal/purge ./internal/status ./internal/doctor ./internal/completion ./internal/actioncontract -count=1
```

- [ ] Run the complete project baseline.

```bash
go test -race ./...
go vet ./...
go build ./cmd/agentproof
```

- [ ] Repeat cross-compilation after all edits.

```bash
GOOS=linux GOARCH=amd64 go test -c -o /tmp/agentproof-record-linux.test ./internal/record
GOOS=darwin GOARCH=amd64 go test -c -o /tmp/agentproof-record-darwin.test ./internal/record
GOOS=freebsd GOARCH=amd64 go test -c -o /tmp/agentproof-record-freebsd.test ./internal/record
GOOS=windows GOARCH=amd64 go test -c -o /tmp/agentproof-record-windows.test.exe ./internal/record
```

- [ ] Run 30-second fuzz verification for each of the four targets.

```bash
go test ./internal/scan -run '^$' -fuzz FuzzRedact -fuzztime 30s
go test ./internal/scan -run '^$' -fuzz FuzzScanPatch -fuzztime 30s
go test ./internal/session -run '^$' -fuzz FuzzSummarize -fuzztime 30s
go test ./internal/testresult -run '^$' -fuzz FuzzIngest -fuzztime 30s
```

- [ ] Run `/finalize` to simplify, review, and verify the exact candidate. Do not allow finalization to combine the planned commit boundaries or add AI attribution.

- [ ] After the planned conventional commits are created and the already-authorized delivery workflow pushes the branch, ensure an open pull request exists for `audit-remediation`, then verify the exact pull request and head SHA produced the successful native Windows CI run before declaring the work done. Do not push or create the pull request while editing or reviewing this plan.

```bash
workflow_file=ci.yml
branch=audit-remediation
commit_sha="$(git rev-parse HEAD)"
pr_number="$(gh pr list --head "$branch" --state open --json number --jq '.[0].number')"
if [ -z "$pr_number" ]; then
  gh pr create --base main --head "$branch" --fill
  pr_number="$(gh pr view "$branch" --json number --jq '.number')"
fi
pr_head_sha="$(gh pr view "$pr_number" --json headRefOid --jq '.headRefOid')"
test "$pr_head_sha" = "$commit_sha"
run_id=
for attempt in $(seq 1 30); do
  run_id="$(gh run list --workflow "$workflow_file" --event pull_request --commit "$pr_head_sha" --limit 1 --json databaseId --jq '.[0].databaseId')"
  [ -n "$run_id" ] && break
  sleep 10
done
test -n "$run_id"
run_event="$(gh run view "$run_id" --json event --jq '.event')"
run_head_sha="$(gh run view "$run_id" --json headSha --jq '.headSha')"
run_pr_number="$(gh api "repos/{owner}/{repo}/actions/runs/$run_id" --jq ".pull_requests[] | select(.number == $pr_number) | .number")"
test "$run_event" = pull_request
test "$run_head_sha" = "$pr_head_sha"
test "$run_pr_number" = "$pr_number"
printf 'pr=%s commit=%s workflow=%s run=%s job=%s\n' "$pr_number" "$pr_head_sha" "$workflow_file" "$run_id" windows-record-lock
gh run watch "$run_id" --exit-status
run_conclusion="$(gh run view "$run_id" --json conclusion --jq '.conclusion')"
test "$run_conclusion" = success
job_conclusion="$(gh run view "$run_id" --json jobs --jq '.jobs[] | select(.name == "windows-record-lock") | .conclusion')"
test "$job_conclusion" = success
```

Expected: the open pull request targets the exact pushed HEAD; the selected `.github/workflows/ci.yml` run is a `pull_request` run associated with that pull request and exact head SHA; the run concludes `success`; and its `windows-record-lock` job exists and concludes `success`. A missing pull request, head mismatch, wrong PR association, wrong event or run head, missing run, missing job, failed workflow, or non-success job conclusion blocks definition of done.

## Definition of done

- [ ] Exactly one local record can hold the nonblocking kernel lock.
- [ ] `.record.lock` is permanent, regular, no-follow opened, and never deleted on release.
- [ ] Metadata contains version, owner ID, run ID, PID, and acquisition time; kernel state remains authoritative.
- [ ] Crash release, contention, ownership, final-symlink, subprocess, Windows runtime, and cross-compilation tests pass.
- [ ] Purge rejects an at-rest symlinked/out-of-tree runs parent and symlinked/oversized state files.
- [ ] Purge and doctor correlate the active lock to the exact run ID.
- [ ] The active concurrent path-replacement residual is documented without inflated claims.
- [ ] Targeted Unix wrapper signals reach the child process group before wrapper termination.
- [ ] Status uses attestation time only when bundle IDs match.
- [ ] Step Summary rejects a final report symlink and has success, policy-failure, missing, unreadable, and exit-preservation contract tests.
- [ ] Directly sourced zsh completion registers with `compdef`; `purge --runs` and doctor guidance are correct.
- [ ] Fuzz targets preserve no-panic coverage and assert behavioral properties.
- [ ] `go test -race ./...`, `go vet ./...`, build, cross-compilation, and all four fuzz runs pass.
- [ ] Documentation matches the implemented local-filesystem and residual-risk contracts.

## Planned conventional commits

1. `fix(record): replace PID lock with kernel-backed ownership`
2. `fix(purge): correlate recording liveness by run ID`
3. `fix(purge): contain run deletion and bound state reads`
4. `fix(record): forward targeted signals to child processes`
5. `fix(status): correlate evidence and attestation bundles`
6. `fix(action): harden and test step summary publication`
7. `fix(cli): repair completion and purge guidance contracts`
8. `test(fuzz): assert behavioral parser properties`
9. `docs: clarify record and purge safety boundaries`

Do not add `Co-Authored-By` trailers or any AI attribution.
