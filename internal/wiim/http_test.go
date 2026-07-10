package wiim

import (
	"errors"
	"strings"
	"testing"
)

type errorResponseReader struct{ err error }

func (r errorResponseReader) Read([]byte) (int, error) { return 0, r.err }

func TestReadLimitedResponse(t *testing.T) {
	const limit int64 = 4

	got, err := readLimitedResponse(strings.NewReader("test"), limit)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "test" {
		t.Fatalf("got %q", got)
	}

	_, err = readLimitedResponse(strings.NewReader("tests"), limit)
	if err == nil || err.Error() != "response exceeds 4 bytes" {
		t.Fatalf("oversized response error = %v", err)
	}

	readErr := errors.New("read failed")
	_, err = readLimitedResponse(errorResponseReader{err: readErr}, limit)
	if err != readErr {
		t.Fatalf("read error = %v, want unchanged error %v", err, readErr)
	}

	if _, err := readLimitedResponse(strings.NewReader(""), 0); err != nil {
		t.Fatalf("empty response at zero limit: %v", err)
	}
	if _, err := readLimitedResponse(strings.NewReader("x"), 0); err == nil {
		t.Fatal("expected response over zero limit to fail")
	}
}
