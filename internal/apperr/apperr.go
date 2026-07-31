// Package apperr carries the one error classification the command boundary
// needs. Exit codes are a public contract CI systems branch on, so a wrong
// invocation must never be reported as an AgentProof internal failure. This
// package has no internal imports so every producing package can wrap with it.
package apperr

import "errors"

// ErrUsage marks an invalid invocation or invalid local configuration: the
// caller can fix it. Wrap it with fmt.Errorf("%w: ...", apperr.ErrUsage) and
// classify it once at the command boundary with errors.Is.
var ErrUsage = errors.New("invalid usage")

// IsUsage reports whether err was produced by a fixable invocation or a
// fixable local configuration rather than by an AgentProof failure.
func IsUsage(err error) bool {
	return errors.Is(err, ErrUsage)
}
