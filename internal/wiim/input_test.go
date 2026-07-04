package wiim

import (
	"testing"
)

func TestValidateHTTPURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid http", "http://example.com/stream.mp3", false},
		{"valid https", "https://example.com/stream.mp3", false},
		{"valid with port", "https://example.com:8080/path", false},
		{"valid with query", "https://example.com/path?a=1&b=2", false},
		{"valid localhost", "http://127.0.0.1:8080/file.mp3", false},
		{"file scheme", "file:///tmp/song.mp3", true},
		{"ftp scheme", "ftp://example.com/song.mp3", true},
		{"relative path", "/path/to/song.mp3", true},
		{"relative no scheme", "path/to/song.mp3", true},
		{"empty string", "", true},
		{"no host", "http:///path", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateHTTPURL(tc.url)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for URL %q", tc.url)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for URL %q: %v", tc.url, err)
			}
		})
	}
}

func TestParseVolume(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantMode  string
		wantAmt   int
		wantErr   bool
		wantErrFn func(error) bool
	}{
		{"absolute 0", "0", "set", 0, false, nil},
		{"absolute 50", "50", "set", 50, false, nil},
		{"absolute 100", "100", "set", 100, false, nil},
		{"relative +10", "+10", "up", 10, false, nil},
		{"relative +1", "+1", "up", 1, false, nil},
		{"relative -5", "-5", "down", 5, false, nil},
		{"garbage text", "abc", "", 0, true, nil},
		{"relative -1 (decrement)", "-1", "down", 1, false, nil},
		{"plus plus 10 (++ treated as relative +)", "++10", "up", 10, false, nil},
		{"garbage +abc", "+abc", "", 0, true, nil},
		{"garbage -abc", "-abc", "", 0, true, nil},
		{"relative +0 (zero amount error)", "+0", "", 0, true, nil},
		{"relative -0 (zero amount error)", "-0", "", 0, true, nil},
		{"out of range 101", "101", "", 0, true, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mode, amt, err := parseVolume(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q", tc.input)
				}
				if tc.wantErrFn != nil && !tc.wantErrFn(err) {
					t.Fatalf("error check failed for %q: %v", tc.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tc.input, err)
			}
			if mode != tc.wantMode {
				t.Fatalf("mode = %q, want %q", mode, tc.wantMode)
			}
			if amt != tc.wantAmt {
				t.Fatalf("amount = %d, want %d", amt, tc.wantAmt)
			}
		})
	}
}

func TestNormalizeInputName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"wifi", "wifi", "wifi", false},
		{"network alias", "network", "wifi", false},
		{"wi-fi alias", "wi-fi", "wifi", false},
		{"bluetooth", "bluetooth", "bluetooth", false},
		{"bt alias", "bt", "bluetooth", false},
		{"line-in", "line-in", "line-in", false},
		{"line alias", "line", "line-in", false},
		{"aux alias", "aux", "line-in", false},
		{"optical", "optical", "optical", false},
		{"spdif alias", "spdif", "optical", false},
		{"coaxial", "coaxial", "coaxial", false},
		{"coax alias", "coax", "coaxial", false},
		{"hdmi", "hdmi", "hdmi", false},
		{"hdmi-arc", "hdmi-arc", "hdmi", false},
		{"arc alias", "arc", "hdmi", false},
		{"phono", "phono", "phono", false},
		{"usb", "usb", "usb", false},
		{"case insensitive", "WIFI", "wifi", false},
		{"case insensitive bt", "BT", "bluetooth", false},
		{"leading/trailing space", "  wifi  ", "wifi", false},
		{"unsupported input", "tuner", "", true},
		{"empty string", "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeInputName(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
