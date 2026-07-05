package wiim

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestFormatErrorPlainTextUnchanged(t *testing.T) {
	err := usagef("host must be set")
	if got := FormatError(err, false); got != err.Error() {
		t.Fatalf("FormatError(false) = %q, want %q", got, err.Error())
	}
	if FormatError(nil, false) != "" {
		t.Fatalf("FormatError(nil, false) should be empty")
	}
	if FormatError(nil, true) != "" {
		t.Fatalf("FormatError(nil, true) should be empty")
	}
}

func TestFormatErrorJSONEnvelope(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantKind string
		wantCode int
	}{
		{"usage", usagef("host must be a hostname or IP"), "usage", 2},
		{"runtime", runtimef("WiiM API returned HTTP 500"), "runtime", 1},
		{"unwrapped", errors.New("boom"), "runtime", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := FormatError(tc.err, true)
			var envelope struct {
				Error struct {
					Kind     string `json:"kind"`
					Message  string `json:"message"`
					ExitCode int    `json:"exitCode"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(out), &envelope); err != nil {
				t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
			}
			if envelope.Error.Kind != tc.wantKind {
				t.Fatalf("kind = %q, want %q", envelope.Error.Kind, tc.wantKind)
			}
			if envelope.Error.ExitCode != tc.wantCode {
				t.Fatalf("exitCode = %d, want %d", envelope.Error.ExitCode, tc.wantCode)
			}
			if envelope.Error.Message == "" {
				t.Fatal("message should not be empty")
			}
			if strings.HasPrefix(envelope.Error.Message, "wiim:") {
				t.Fatalf("message should not carry the wiim: prefix used for plain-text display: %q", envelope.Error.Message)
			}
			if got := ExitCode(tc.err); got != tc.wantCode {
				t.Fatalf("ExitCode = %d, want %d (envelope should always match ExitCode)", got, tc.wantCode)
			}
		})
	}
}
