package wiim

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func withUserHomeDirError(t *testing.T, err error) {
	t.Helper()
	old := userHomeDir
	userHomeDir = func() (string, error) { return "", err }
	t.Cleanup(func() { userHomeDir = old })
}

func TestLoadConfigReportsConfigPathError(t *testing.T) {
	homeErr := errors.New("home unavailable")
	withUserHomeDirError(t, homeErr)

	_, err := LoadConfig("")
	if err == nil {
		t.Fatal("expected config path error")
	}
	if _, ok := err.(RuntimeError); !ok {
		t.Fatalf("error type %T, want RuntimeError: %v", err, err)
	}
	if got, want := err.Error(), "wiim: could not determine config path: home unavailable"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestConfigShowReportsConfigPathErrorPlain(t *testing.T) {
	withUserHomeDirError(t, errors.New("home unavailable"))

	var stdout, stderr bytes.Buffer
	err := Run([]string{"config", "show"}, &stdout, &stderr)
	if ExitCode(err) != 1 {
		t.Fatalf("exit code = %d, want 1", ExitCode(err))
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if got, want := strings.TrimSpace(stderr.String()), "wiim: could not determine config path: home unavailable"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestConfigShowReportsConfigPathErrorJSON(t *testing.T) {
	withUserHomeDirError(t, errors.New("home unavailable"))

	var stdout, stderr bytes.Buffer
	err := Run([]string{"config", "show", "--json"}, &stdout, &stderr)
	if ExitCode(err) != 1 {
		t.Fatalf("exit code = %d, want 1", ExitCode(err))
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	var envelope struct {
		Error struct {
			Kind     string `json:"kind"`
			Message  string `json:"message"`
			ExitCode int    `json:"exitCode"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\n%s", err, stderr.String())
	}
	if envelope.Error.Kind != "runtime" || envelope.Error.ExitCode != 1 {
		t.Fatalf("error envelope = %+v", envelope.Error)
	}
	if envelope.Error.Message != "could not determine config path: home unavailable" {
		t.Fatalf("error message = %q", envelope.Error.Message)
	}
}
