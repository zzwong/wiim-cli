package wiim

import (
	"strings"
	"testing"
)

func TestCliampStatusParsing(t *testing.T) {
	tests := []struct {
		name     string
		statusFn func(args ...string) (string, error)
		want     CliampInfo
		wantErr  bool
	}{
		{
			name: "all fields populated",
			statusFn: func(args ...string) (string, error) {
				switch args[0] {
				case "status":
					return "Playing", nil
				case "metadata":
					switch args[1] {
					case "xesam:url":
						return "https://example.com/song.mp3", nil
					case "xesam:title":
						return "Song Title", nil
					case "artist":
						return "Artist Name", nil
					case "album":
						return "Album Name", nil
					}
				}
				return "", nil
			},
			want: CliampInfo{
				Status: "Playing",
				Title:  "Song Title",
				Artist: "Artist Name",
				Album:  "Album Name",
				URL:    "https://example.com/song.mp3",
			},
		},
		{
			name: "minimal fields",
			statusFn: func(args ...string) (string, error) {
				switch args[0] {
				case "status":
					return "Paused", nil
				case "metadata":
					return "", nil
				}
				return "", nil
			},
			want: CliampInfo{
				Status: "Paused",
			},
		},
		{
			name: "empty status with no url or title returns error",
			statusFn: func(_ ...string) (string, error) {
				return "", nil
			},
			wantErr: true,
		},
		{
			name: "url present but empty status is ok",
			statusFn: func(args ...string) (string, error) {
				if len(args) == 2 && args[0] == "metadata" && args[1] == "xesam:url" {
					return "http://example.com/stream", nil
				}
				return "", nil
			},
			want: CliampInfo{
				URL: "http://example.com/stream",
			},
		},
		{
			name: "title present but empty status is ok",
			statusFn: func(args ...string) (string, error) {
				if len(args) == 2 && args[0] == "metadata" && args[1] == "xesam:title" {
					return "Some Track", nil
				}
				return "", nil
			},
			want: CliampInfo{
				Title: "Some Track",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			old := runPlayerctl
			runPlayerctl = tc.statusFn
			defer func() { runPlayerctl = old }()

			info, err := CliampStatus()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if info.Status != tc.want.Status {
				t.Fatalf("Status = %q, want %q", info.Status, tc.want.Status)
			}
			if info.Title != tc.want.Title {
				t.Fatalf("Title = %q, want %q", info.Title, tc.want.Title)
			}
			if info.Artist != tc.want.Artist {
				t.Fatalf("Artist = %q, want %q", info.Artist, tc.want.Artist)
			}
			if info.Album != tc.want.Album {
				t.Fatalf("Album = %q, want %q", info.Album, tc.want.Album)
			}
			if info.URL != tc.want.URL {
				t.Fatalf("URL = %q, want %q", info.URL, tc.want.URL)
			}
		})
	}
}

func TestCliampHandoffFiltersURLs(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOK    bool
		wantInOut string
	}{
		{
			name:      "http URL forwarded",
			url:       "http://example.com/stream.mp3",
			wantOK:    true,
			wantInOut: "Sent cliamp URL to WiiM",
		},
		{
			name:      "https URL forwarded",
			url:       "https://example.com/stream.mp3",
			wantOK:    true,
			wantInOut: "Sent cliamp URL to WiiM",
		},
		{
			name:   "file URL rejected",
			url:    "file:///home/user/song.mp3",
			wantOK: false,
		},
		{
			name:   "spotify URL rejected",
			url:    "spotify:track:123",
			wantOK: false,
		},
		{
			name:   "empty URL rejected",
			url:    "",
			wantOK: false,
		},
		{
			name:   "unknown scheme rejected",
			url:    "rtsp://example.com/stream",
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			old := runPlayerctl
			runPlayerctl = func(args ...string) (string, error) {
				if len(args) == 2 && args[0] == "metadata" && args[1] == "xesam:url" {
					return tc.url, nil
				}
				if args[0] == "status" {
					if tc.url == "" {
						return "", nil
					}
					return "Playing", nil
				}
				return "Unknown", nil
			}
			defer func() { runPlayerctl = old }()

			// Use a fakeDevice that records PlayURL calls
			fd := &fakeDevice{}
			out, err := CliampHandoff(fd)

			if tc.wantOK {
				if err != nil {
					t.Fatalf("expected success, got error: %v", err)
				}
				if !strings.Contains(out, tc.wantInOut) {
					t.Fatalf("output = %q, want %q", out, tc.wantInOut)
				}
				// Should have called PlayURL
				if len(fd.calls) != 1 || fd.calls[0] != "play-url:"+tc.url {
					t.Fatalf("calls %#v", fd.calls)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error for URL %q but got output %q, calls %#v", tc.url, out, fd.calls)
				}
				// Should NOT have called PlayURL
				if containsCall(fd.calls, "play-url") {
					t.Fatalf("should not have called PlayURL for rejected URL: calls %#v", fd.calls)
				}
			}
		})
	}
}

func TestCliampStatusErrorWhenPlayerctlFails(t *testing.T) {
	old := runPlayerctl
	runPlayerctl = func(_ ...string) (string, error) {
		return "", nil
	}
	defer func() { runPlayerctl = old }()

	_, err := CliampStatus()
	if err == nil {
		t.Fatal("expected error when all fields empty")
	}
	if !strings.Contains(err.Error(), "cliamp") {
		t.Fatalf("error should mention cliamp: %v", err)
	}
}
