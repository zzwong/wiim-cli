package wiim

import (
	"encoding/json"
	"fmt"
)

// UsageError represents an error caused by invalid user input or command syntax.
// ExitCode returns 2 for usage errors.
type UsageError struct{ Msg string }

func (e UsageError) Error() string { return "wiim: " + e.Msg }

// RuntimeError represents an error from device communication or other runtime failures.
// ExitCode returns 1 for runtime errors.
type RuntimeError struct{ Msg string }

func (e RuntimeError) Error() string { return "wiim: " + e.Msg }

func usagef(format string, args ...any) error {
	return UsageError{Msg: fmt.Sprintf(format, args...)}
}

func runtimef(format string, args ...any) error {
	return RuntimeError{Msg: fmt.Sprintf(format, args...)}
}

// classifyError is the single source of truth for how an error maps to a
// JSON-envelope kind, an exit code, and a display message without the
// "wiim: " prefix Error() adds for human display. ExitCode and FormatError
// both go through this so they can't independently drift on a new error type.
//
// Note: this only recognizes UsageError/RuntimeError by exact type, not by
// errors.As/Unwrap — if either type is ever wrapped with fmt.Errorf("...: %w",
// ...) elsewhere, it will fall through to the runtime/err.Error() default
// below instead of being classified correctly. Nothing in this codebase
// wraps them today.
func classifyError(err error) (kind, message string, code int) {
	switch e := err.(type) {
	case UsageError:
		return "usage", e.Msg, 2
	case RuntimeError:
		return "runtime", e.Msg, 1
	default:
		return "runtime", err.Error(), 1
	}
}

// ExitCode maps an error to a process exit code: nil → 0, UsageError → 2, anything else → 1.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	_, _, code := classifyError(err)
	return code
}

type errorEnvelope struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Kind     string `json:"kind"`
	Message  string `json:"message"`
	ExitCode int    `json:"exitCode"`
}

// FormatError renders err for display on stderr: a plain "wiim: <message>"
// string normally (unchanged from before --json errors existed), or a JSON
// envelope when asJSON is true, so scripts/agents that requested --json get a
// structured failure reason instead of prose they'd have to string-match.
// Returns "" for a nil err.
func FormatError(err error, asJSON bool) string {
	if err == nil {
		return ""
	}
	if !asJSON {
		return err.Error()
	}
	kind, message, code := classifyError(err)
	out, marshalErr := json.MarshalIndent(errorEnvelope{Error: errorDetail{
		Kind:     kind,
		Message:  message,
		ExitCode: code,
	}}, "", "  ")
	if marshalErr != nil {
		return err.Error()
	}
	return string(out)
}
