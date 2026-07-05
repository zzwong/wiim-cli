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

// ExitCode maps an error to a process exit code: nil → 0, UsageError → 2, anything else → 1.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if _, ok := err.(UsageError); ok {
		return 2
	}
	return 1
}

// errorKindAndMessage classifies err the same way ExitCode does, and returns
// its message without the "wiim: " prefix used by Error() for human display.
func errorKindAndMessage(err error) (kind, message string) {
	switch e := err.(type) {
	case UsageError:
		return "usage", e.Msg
	case RuntimeError:
		return "runtime", e.Msg
	default:
		return "runtime", err.Error()
	}
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
	kind, message := errorKindAndMessage(err)
	out, marshalErr := json.MarshalIndent(errorEnvelope{Error: errorDetail{
		Kind:     kind,
		Message:  message,
		ExitCode: ExitCode(err),
	}}, "", "  ")
	if marshalErr != nil {
		return err.Error()
	}
	return string(out)
}
