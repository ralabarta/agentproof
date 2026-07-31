# AgentProof verification

✓ **PASSED**

> Verify changes associated with Git baseline HEAD~1

## Summary

- ✓ Test evidence passed (32 passed, 0 failed, 0 skipped)
- ✓ No secret patterns detected in the captured added lines
- 5 files changed; +499/-36 lines
- Impact radius: 3; 6 components affected
- Required evidence completeness: 2/2 (100.00%)
- Canonical manifest integrity: passed

## Provenance

| Field | Value |
|---|---|
| Agent | `unknown` |
| Model | `unknown` |
| Duration | `0.00s` |
| Start commit | `a2ca9a05c2cd` |
| End commit | `d2345ef7f24a` |
| Association | `clean-baseline` |

## Changes

| File | Status | Added | Deleted |
|---|---:|---:|---:|
| `README.md` | modified | 3 | 2 |
| `docs/architecture.md` | modified | 2 | 2 |
| `internal/impact/impact.go` | modified | 118 | 32 |
| `internal/impact/impact_test.go` | modified | 78 | 0 |
| `internal/impact/imports.go` | added | 298 | 0 |

### Commits

- `d2345ef7f24a` feat(impact): resolve TypeScript, JavaScript, and Python imports

## Impact

Analyzer: `go/parser@stdlib+ts-js-py/regex-imports@v1+path-graph/v1` · examined 28 files / 114332 bytes · complete: true

Affected components: cmd/agentproof, docs, internal/app, internal/impact, internal/verify, root

Observed dependency edges:

| Dependent | Imports |
|---|---|
| `cmd/agentproof` | `internal/app` |
| `internal/app` | `internal/config` |
| `internal/app` | `internal/gitx` |
| `internal/app` | `internal/purge` |
| `internal/app` | `internal/record` |
| `internal/app` | `internal/verify` |
| `internal/config` | `internal/safefile` |
| `internal/gitx` | `internal/evidence` |
| `internal/impact` | `internal/evidence` |
| `internal/purge` | `internal/config` |
| `internal/record` | `internal/config` |
| `internal/record` | `internal/evidence` |
| `internal/record` | `internal/gitx` |
| `internal/record` | `internal/safefile` |
| `internal/record` | `internal/scan` |
| `internal/record` | `internal/session` |
| `internal/report` | `internal/evidence` |
| `internal/scan` | `internal/evidence` |
| `internal/session` | `internal/evidence` |
| `internal/testresult` | `internal/evidence` |
| `internal/verify` | `internal/config` |
| `internal/verify` | `internal/evidence` |
| `internal/verify` | `internal/gitx` |
| `internal/verify` | `internal/impact` |
| `internal/verify` | `internal/report` |
| `internal/verify` | `internal/safefile` |
| `internal/verify` | `internal/scan` |
| `internal/verify` | `internal/testresult` |

## Findings

No deterministic findings.

## Reproducibility

Bundle ID: `03dad1ce4f31a0229529c10ad3e8c46e13651b04457521287111d7ae32c8a31d`

The bundle ID is SHA-256 of the canonical manifest excluding only its own identifier. Hashes detect changes after capture; they do not prove authenticity, authorship, completeness, correctness, or safety.

---

Verified with AgentProof
