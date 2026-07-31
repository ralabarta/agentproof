# Security policy

## Supported versions

Until AgentProof reaches `v1`, only the latest tagged release receives security fixes. Evidence schemas remain versioned independently and incompatible schema changes require a new major schema identifier.

## Reporting a vulnerability

Please use the repository's private **Security → Report a vulnerability** flow. Do not open a public issue for vulnerabilities involving secret disclosure, path traversal, parser exhaustion, evidence tampering, or GitHub Actions privilege boundaries.

Include the affected version, operating system, minimal reproduction, expected impact, and whether any real credentials were exposed. Use synthetic credentials in reproductions.

Maintainers should acknowledge a report within seven days. No bounty or response deadline is promised.

## Security model

AgentProof treats session logs, repository paths, test artifacts, patches, and pull-request content as untrusted input. Its hashes detect changes after capture; they do not prove source authenticity, authorship, completeness, correctness, or merge safety.

Raw command output is disabled by default. If explicitly retained, it remains local, is excluded from Git and reports, and can be previewed and purged with `agentproof purge --raw`.

