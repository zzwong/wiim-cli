package wiim

import (
	"net/url"
	"os"
	"os/exec"
	"strings"
)

// CliampInfo holds playback metadata obtained from cliamp via MPRIS/playerctl.
type CliampInfo struct {
	Status string `json:"status,omitempty"`
	Title  string `json:"title,omitempty"`
	Artist string `json:"artist,omitempty"`
	Album  string `json:"album,omitempty"`
	URL    string `json:"url,omitempty"`
}

var runPlayerctl = func(args ...string) (string, error) {
	// #nosec G204 -- fixed argv, no shell
	out, err := exec.Command("playerctl", append([]string{"-p", "cliamp"}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

// CliampStatus queries cliamp through playerctl and returns the current playback
// status, title, artist, album, and URL. Returns RuntimeError if no data is found.
func CliampStatus() (CliampInfo, error) {
	status, _ := runPlayerctl("status")
	urlValue, _ := runPlayerctl("metadata", "xesam:url")
	title, _ := runPlayerctl("metadata", "xesam:title")
	artist, _ := runPlayerctl("metadata", "artist")
	album, _ := runPlayerctl("metadata", "album")
	if status == "" && urlValue == "" && title == "" {
		return CliampInfo{}, runtimef("could not read cliamp via playerctl; is cliamp running with MPRIS enabled?")
	}
	return CliampInfo{Status: status, Title: title, Artist: artist, Album: album, URL: urlValue}, nil
}

// CliampHandoff reads cliamp's current playback URL via MPRIS and sends it to
// the WiiM device for playback. Only http/https URLs are supported directly;
// local file and Spotify URLs return a descriptive error.
func CliampHandoff(client device) (string, error) {
	info, err := CliampStatus()
	if err != nil {
		return "", err
	}
	if info.URL == "" {
		return "", runtimef("cliamp did not expose a playable URL via MPRIS")
	}
	u, err := url.Parse(info.URL)
	if err != nil {
		return "", runtimef("cliamp URL is invalid: %s", info.URL)
	}
	switch u.Scheme {
	case "http", "https":
		if err := client.PlayURL(info.URL); err != nil {
			return "", err
		}
		return "Sent cliamp URL to WiiM", nil
	case "file":
		return "", runtimef("cliamp is playing a local file; use play-file to serve local files to WiiM")
	case "spotify":
		return "", runtimef("cliamp is playing Spotify; use spotify transfer/play with a Spotify token")
	default:
		if _, err := os.Stat(info.URL); err == nil {
			return "", runtimef("cliamp is playing a local file; use play-file to serve local files to WiiM")
		}
		return "", runtimef("cliamp URL scheme %q cannot be handed to WiiM directly", u.Scheme)
	}
}
