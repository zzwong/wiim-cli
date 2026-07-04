package wiim

import "fmt"

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
