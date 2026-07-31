# Tasks: AgentProof Foundation

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | ~6,680 |
| 800-line budget risk | High |
| Chained PRs recommended | Yes |
| Delivery strategy | auto-chain |
| Chain strategy | stacked-to-main |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
800-line budget risk: High
400-line budget risk: High

IDs: EC1–4 evidence capture; GL1–4 Git linkage; VR1–6 verification/reporting; IA1–4 impact/risk; GA1–4 GitHub Action.

## Chained PR Plan

| PR | Slice / lines | Depends | Evidence command | Runtime; rollback |
|---|---|---|---|---|
| 1 | Tooling/kernel 650 | — | `go test ./internal/evidence/...` | `go test ./...`; revert module/kernel |
| 2 | Capture/providers 780 | 1 | `go test ./internal/capture/... ./internal/adapters/{provider,fs}/...` | fixture capture; revert slice |
| 3 | Git linkage 700 | 1 | `go test ./internal/linkage/... ./internal/adapters/git/...` | temp repos; revert slice |
| 4 | Verify/tests 790 | 2,3 | `go test ./internal/verification/... ./internal/adapters/testresult/...` | fixture verify; revert slice |
| 5 | Reports/retention 680 | 4 | `go test ./internal/reporting/... ./internal/adapters/render/...` | offline render/purge; revert slice |
| 6 | Impact/risk 790 | 3,4 | `go test ./internal/impact/... ./internal/policy/... ./internal/adapters/treesitter/...` | language fixtures; revert slice |
| 7 | SQLite 500 | 4 | `go test ./internal/adapters/sqlite/...` | rebuild index; delete index |
| 8 | CLI 700 | 2–7 | `go test ./cmd/agentproof/...` | CLI fixture E2E; revert root |
| 9 | Action 600 | 8 | `go test ./internal/ci/...` | fork fixture; disable workflow |
| 10 | Release/dogfood/docs 490 | 9 | `go test -race ./... && go vet ./...` | matrix smoke; revert slice |

## Ordered Test-First Tasks

- [x] **1.1 [sequential; EC1,VR1]** Initialize `go.mod`; RED wire/canonical/import-direction tests, then GREEN `internal/evidence` types and ports. Acceptance: PR1 command.
- [ ] **2.1 [after 1.1; EC1–4]** RED census, redaction, raw purge, bounds/drift, traversal/symlink and hostile `requirements.txt`/`CMakeLists.txt`/MDX/`README.sh` tests; GREEN `internal/capture`, `internal/adapters/{provider,fs}`. Acceptance: PR2.
- [ ] **3.1 [after 1.1; parallel with 2.1; GL1–4]** RED clean/staged/`commit -a`/empty-index/dirty/rebase/shallow/root tests; GREEN `internal/linkage`, `internal/adapters/git`, association-only wording. Acceptance: PR3.
- [ ] **4.1 [after 2.1,3.1; VR1–4]** RED canonical/golden, tamper, completeness, JUnit/test2json no-execution, required/optional exit tests; GREEN `internal/verification`, `internal/adapters/testresult`. Acceptance: PR4.
- [ ] **5.1 [after 4.1; VR5–6,EC3]** RED deterministic hostile-markup Markdown/HTML and preview/purge tests; GREEN `internal/reporting`, `internal/adapters/render`, retention ports, disclaimer/CSP. Acceptance: PR5.
- [ ] **6.1 [after 3.1,4.1; parallel with 5.1; IA1–4]** RED Go/TSX/Python dependents, every limit, unsupported/failure and critical-rule tests; GREEN `internal/{impact,policy}`, Tree-sitter adapter. Acceptance: PR6.
- [ ] **7.1 [after 4.1; parallel with 5.1,6.1; VR1]** RED migration/WAL/parameterization/corruption-rebuild tests; GREEN disposable `internal/adapters/sqlite` index. Acceptance: PR7.
- [ ] **8.1 [after 2.1–7.1; EC1–4,GL1–4,VR1–6,IA1–4]** RED CLI E2E for five commands, JSON/stderr, exits 0–4, XDG permissions/atomic writes; GREEN `cmd/agentproof`. Acceptance: PR8.
- [ ] **9.1 [after 8.1; GA1–4]** RED fork, digest, outputs, idempotent escaped comment, warning/failure/cancel tests; GREEN `internal/ci`, SHA-pinned read-only `.github/workflows/agentproof.yml`. Acceptance: PR9.
- [ ] **10.1 [after 9.1; all]** RED cross-platform fixture determinism/smoke expectations; add release matrix, checksums/SBOM, fixture and opt-in warning-only dogfood, essential `docs/` usage/security limits. Acceptance: PR10.
