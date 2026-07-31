# Threat model

AgentProof processes hostile repository content, session metadata, Git paths, patches, test artifacts, and CI inputs. This document covers the local CLI and the published GitHub Action.

| Threat | Example | Current control | Residual risk |
|---|---|---|---|
| Secret disclosure | Token appears in an added line or command output | In-memory detection; patch redaction before persistence; raw output off by default; reports omit values | Unknown secret formats may not match rules |
| Evidence tampering | Report or manifest changed after verification | Canonical bundle ID and exact emitted-file attestation | Hashes do not authenticate the original source |
| Path traversal | Artifact path escapes the repository | Repository-relative normalization, traversal rejection, symlink rejection | Git path quoting and platform edge cases require continued testing |
| Parser exhaustion | Oversized/deep JSON, XML, or source graph | Per-file/run/record/depth/graph limits | CPU deadlines are not yet enforced for every parser |
| CI privilege escalation | Fork PR runs commands with secrets or write token | Read-only default Action; data-only ingestion; no default comments; immutable Action SHAs | Repository owners can configure unsafe surrounding workflows |
| Misleading attribution | Timestamp proximity treated as authorship | Association terminology, dirty-tree warning, clean/dirty evidence, no line authorship | Users may still overinterpret the report |
| Renderer injection | Filename or objective contains Markdown/HTML payload | Markdown control/bidi escaping, `html/template`, restrictive CSP, no scripts/assets | Renderer fuzzing should expand before `v1` |
| Supply-chain substitution | Mutable Action tags change behavior | Official Actions pinned to full commit SHA; runtime has no third-party modules | Go toolchain and runner images remain external trust roots |

## Security invariants

- `verify` never executes repository code.
- Missing or unknown required evidence cannot pass.
- Secret values never appear in deterministic finding descriptions.
- Raw command output is explicit, local, ignored by Git, previewable, and purgeable.
- Unsupported analysis is visible rather than represented as zero impact.
- Reports never claim authenticity, authorship, completeness beyond the enumerated census, correctness, or safety.

## Out of scope for the current release

- Host compromise or a malicious Go/Git executable.
- Authenticating the coding-agent provider.
- Cryptographic signer identity; SHA-256 is integrity-only.
- Complete semantic or runtime impact analysis.
- Legal or license-compliance conclusions.

