# Publishing checklist

## Repository settings

- Confirm the repository URL is `github.com/ralabarta/agentproof`.
- Enable private vulnerability reporting and branch protection.
- Require the CI workflow on `main`.
- Disable unnecessary Actions permissions; default to read-only.
- Add a short repository description and topics such as `coding-agents`, `code-review`, `supply-chain`, `codex`, and `claude-code`.
- Verify the private security-report link in the issue chooser after enabling private vulnerability reporting.

## First release

1. Run `go test ./...`, `go vet ./...`, and `go build ./cmd/agentproof` with Go 1.22+.
2. Run a local `record → verify` smoke test against a clean fixture repository.
3. Review `SECURITY.md`, `THIRD_PARTY_NOTICES.md`, and the MIT copyright holder.
4. Replace the Action placeholder in documentation with the full commit SHA of the release.
5. Create and push an annotated tag:

   ```bash
   git tag -s v0.1.0 -m "AgentProof v0.1.0"
   git push origin v0.1.0
   ```

6. The release workflow tests the source, cross-compiles six archives, normalizes archive timestamps, creates `checksums.txt`, and publishes the GitHub release.
7. Download one release archive, verify its checksum, and run `agentproof version` before announcing it.

## Marketplace readiness

The root `action.yml` includes branding, documented inputs, machine-readable outputs, read-only behavior, and an uploaded report bundle. Publish the Action from an immutable release and instruct consumers to pin its full commit SHA.

Do not enable automatic PR comments until a separate trusted comment workflow has been reviewed against fork and `pull_request_target` threats.
