# Audit Remediation Tracker

- Audit report: [`docs/audits/2026-08-13-commits.md`](../docs/audits/2026-08-13-commits.md)
- Implementation plan: [`docs/superpowers/plans/2026-08-14-audit-remediation.md`](../docs/superpowers/plans/2026-08-14-audit-remediation.md)
- Audited range: `dad3d78^..c027311` (10 commits)

## Phase 1: Record ownership and liveness

- [ ] Load `/superpowers:test-driven-development`, `/go-testing`, `/code-style`, and `/careful`.
- [x] Implement the permanent Linux/Darwin/FreeBSD/Windows OS-backed `.record.lock` protocol with GOOS-tagged standard-library `syscall`/`unsafe` files and no external module dependency (AP-001, AP-002, AP-005).
- [x] Preserve versioned 64 KiB-bounded metadata with `runID`, permanent lock-file retention, no-follow/reparse rejection, and ownership-safe release.
- [x] Add contention, crash-release, ownership, metadata, final-symlink, and subprocess tests, plus a PR-only `windows-record-lock` job that checks out `ref: ${{ github.event.pull_request.head.sha }}`, asserts `git rev-parse HEAD` equals that SHA, and only then runs `go test ./internal/record`.
- [x] Add the explicit Linux amd64, Darwin amd64, FreeBSD amd64, and Windows amd64 cross-build matrix.
- [x] Correlate purge and doctor liveness with the lock metadata run ID (AP-009).

## Phase 2: Filesystem and process hardening

- [x] Reject an at-rest symlinked or out-of-tree `.agentproof/runs` parent (AP-003).
- [x] Add no-follow, regular-file, 64 KiB bounded `state.json` reads (AP-004).
- [x] Preserve tests proving direct run-directory symlinks are skipped and final symlinks are not traversed.
- [x] Document active concurrent path replacement as residual/out of scope without descriptor-relative platform work.
- [x] Forward targeted Unix SIGINT/SIGTERM to the child process group before wrapper termination (AP-006).

## Phase 3: Publication and Action contracts

- [x] Correlate evidence and attestation bundle IDs before using the attestation timestamp (AP-008).
- [x] Reject final report symlinks in Step Summary publication (AP-007).
- [x] Add Action success, policy-failure, missing, unreadable, symlink, `if: always()`, and exit-preservation tests (AP-013).

## Phase 4: CLI and fuzz contracts

- [x] Register directly sourced zsh completion with `compdef` (AP-010).
- [x] Change doctor guidance to `agentproof purge --runs` (AP-011).
- [x] Add `purge --runs` to generated completions and assert parser/table alignment (AP-012).
- [ ] Add deterministic, bounded, and visible-incomplete behavioral fuzz properties without removing no-panic coverage (AP-014).
  - Deferred: new fuzz oracles exposed a production `RedactString` idempotence defect.

## Phase 5: Documentation and delivery

- [ ] Update lifecycle, security, architecture, and threat-model documentation after behavior is green.
- [ ] Create only the conventional commits listed in the implementation plan.
- [ ] Do not add `Co-Authored-By` trailers or AI attribution.
- [ ] Run `/finalize` after normalization and full verification.
- [ ] After the planned commits and authorized push, ensure an open pull request exists for `audit-remediation`, verify its head SHA matches the exact pushed HEAD, find and wait for the `.github/workflows/ci.yml` run associated with that pull request and exact head SHA, and require both the run and its `windows-record-lock` job to conclude `success` before definition of done.

## Quality gates

- [ ] Focused RED commands fail for the intended missing behavior before implementation.
- [ ] Focused GREEN commands pass after each minimal implementation.
- [ ] `go test -race ./...` passes.
- [ ] `go vet ./...` passes.
- [ ] `go build ./cmd/agentproof` passes.
- [ ] AgentProof remains Go 1.22+ standard-library-only with no external locking dependency.
- [ ] Linux amd64, Darwin amd64, FreeBSD amd64, and Windows amd64 record-package cross-builds pass.
- [ ] Native Windows lock/reparse runtime tests pass in the successful `.github/workflows/ci.yml` `pull_request` run associated with the exact pull request and pushed head SHA, under a successful `windows-record-lock` job; cross-building alone is insufficient.
- [ ] `FuzzRedact`, `FuzzScanPatch`, `FuzzSummarize`, and `FuzzIngest` pass with behavioral assertions.
- [ ] Protected local directories `.freebuff/` and `.serena/` remain untouched.
- [ ] Final documentation states only verified guarantees and retains the audit's refutations/down-ranks.

## Review
