# Evidence contract `agentproof.dev/evidence/v1`

The authoritative source census is `.agentproof/manifest.json`. It contains:

- `schemaVersion`: exact evidence contract identifier;
- `bundleId`: lowercase SHA-256 of canonical manifest bytes excluding only this field;
- `records`: deterministically ordered evidence records.

## Record states

| State | Meaning |
|---|---|
| `observed` | Successfully normalized and digested |
| `missing` | Required or declared source was unavailable |
| `unsupported` | Source format or analyzer is not supported by this version |
| `unknown` | Processing was attempted but the result is indeterminate; a reason is required |
| `not_observed` | Optional, undiscovered, non-required source; excluded from completeness |

Every discovered or policy-required source appears exactly once in the completeness denominator. Completeness is satisfied only when all denominator records are `observed`.

## Canonicalization

Canonical bytes use UTF-8 JSON with:

- lexicographically ordered object keys;
- deterministically ordered records and confidence reasons;
- normalized repository-relative `/` locators;
- integer confidence scores from 0 through 100;
- stable field presence;
- no presentation-only metadata.

Absolute, traversal, empty, NUL-bearing, overlong, and duplicate locators are rejected. Observed records require a lowercase `sha256:` digest. Non-observed records require a reason.

The emitted manifest includes its computed `bundleId`; canonical identity bytes omit only that self-referential field. `.agentproof/attestation.json` separately hashes the exact emitted manifest file.

## Completeness versus integrity

Completeness describes whether required and discovered sources were successfully observed. Integrity describes whether canonical publication and retained-artifact digests match. They are reported independently; neither means the underlying source is truthful or the code is safe.

## Related evidence

`.agentproof/evidence.json` contains the normalized run, Git association, tests, findings, impact, claims, completeness, integrity, and bundle ID used by both reports. It is not itself the canonical identity input.

To verify the emitted manifest file hash:

```bash
sha256sum .agentproof/manifest.json
```

Compare the result with `evidence_hash` in `.agentproof/attestation.json`.

