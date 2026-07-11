// Package wiim provides a client for controlling WiiM audio streamers over the local network.
// It supports the WiiM HTTP API, Google Cast protocol for media metadata, Spotify Connect,
// local file serving, and configuration management.
package wiim

import (
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client communicates with a single WiiM device over its HTTP API and the Cast
// discovery endpoint (port 8008).
type Client struct {
	Host       string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// NewClient creates a Client for the given host with the specified timeout (in
// seconds). The HTTP client uses a TLS config that skips server verification
// because WiiM devices serve self-signed certificates.
func NewClient(host string, timeoutSeconds float64) *Client {
	// WiiM devices serve self-signed certificates on the LAN, so verification is impossible; traffic is device-control on the local network.
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	return &Client{
		Host:    host,
		Timeout: time.Duration(timeoutSeconds * float64(time.Second)),
		HTTPClient: &http.Client{
			Timeout:   time.Duration(timeoutSeconds * float64(time.Second)),
			Transport: transport,
		},
	}
}

// CastInfo queries the Google Cast discovery endpoint (http://host:8008/setup/eureka_info)
// and returns the device information as a map.
func (c *Client) CastInfo() (map[string]any, error) {
	value, err := c.request(fmt.Sprintf("http://%s:8008/setup/eureka_info", c.Host))
	if err != nil {
		return nil, err
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil, runtimef("cast endpoint returned non-JSON response")
	}
	return m, nil
}

// Command sends a raw WiiM HTTP API command via HTTPS and returns the parsed
// JSON response, or the plain-text response if JSON parsing fails.
func (c *Client) Command(command string) (any, error) {
	return c.request(fmt.Sprintf("https://%s/httpapi.asp?command=%s", c.Host, url.QueryEscape(command)))
}

// StatusEx returns the device status as a structured map via the getStatusEx API command.
func (c *Client) StatusEx() (map[string]any, error) { return c.expectMap("getStatusEx") }

// PlayerStatus returns the current player state (volume, mute, playback status) via getPlayerStatus.
func (c *Client) PlayerStatus() (map[string]any, error) { return c.expectMap("getPlayerStatus") }

// MetaInfo fetches track metadata (title, artist, album, sample rate, etc.) via
// getMetaInfo. Returns an empty map on error.
func (c *Client) MetaInfo() map[string]any {
	value, err := c.Command("getMetaInfo")
	if err != nil {
		return map[string]any{}
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

// SetVolume sets the absolute playback volume (0–100).
func (c *Client) SetVolume(volume int) error {
	_, err := c.Command(fmt.Sprintf("setPlayerCmd:vol:%d", volume))
	return err
}

// VolumeUp increases the current volume by the given amount, fetching the
// current volume first.
func (c *Client) VolumeUp(amount int) error {
	current, err := c.currentVolume()
	if err != nil {
		return err
	}
	return c.SetVolume(current + amount)
}

// VolumeDown decreases the current volume by the given amount, clamping at 0.
func (c *Client) VolumeDown(amount int) error {
	current, err := c.currentVolume()
	if err != nil {
		return err
	}
	return c.SetVolume(max(0, current-amount))
}

func (c *Client) currentVolume() (int, error) {
	player, err := c.PlayerStatus()
	if err != nil {
		return 0, err
	}
	volume := intPtr(player["vol"])
	if volume == nil {
		return 0, runtimef("device did not report current volume")
	}
	return *volume, nil
}

// Mute sets the mute state: true to mute, false to unmute.
func (c *Client) Mute(enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := c.Command(fmt.Sprintf("setPlayerCmd:mute:%d", v))
	return err
}

// Playback sends a transport control action: "play", "pause", "stop", "next", "prev".
func (c *Client) Playback(action string) error {
	_, err := c.Command("setPlayerCmd:" + action)
	return err
}

// SwitchInput switches the active audio input/source (e.g. "wifi", "bluetooth", "optical").
func (c *Client) SwitchInput(input string) error {
	_, err := c.Command("setPlayerCmd:switchmode:" + input)
	return err
}

// PlayURL instructs the device to stream a media URL.
func (c *Client) PlayURL(mediaURL string) error {
	_, err := c.Command("setPlayerCmd:play:" + mediaURL)
	return err
}

// PlayM3U instructs the device to load and play an M3U playlist URL.
func (c *Client) PlayM3U(playlistURL string) error {
	_, err := c.Command("setPlayerCmd:playlist:" + playlistURL)
	return err
}

// PlayPromptURL plays a short notification/prompt audio URL on the device.
func (c *Client) PlayPromptURL(mediaURL string) error {
	_, err := c.Command("setPlayerCmd:playPromptUrl:" + mediaURL)
	return err
}

// ClearPlaylist clears the current playback queue.
func (c *Client) ClearPlaylist() error {
	_, err := c.Command("setPlayerCmd:clear_playlist")
	return err
}

// Seek jumps to the given position (in seconds) within the current track.
func (c *Client) Seek(seconds int) error {
	_, err := c.Command(fmt.Sprintf("setPlayerCmd:seek:%d", seconds))
	return err
}

// PlayPreset triggers the specified preset number. An optional index selects a
// specific entry within a multi-entry preset.
func (c *Client) PlayPreset(preset int, index *int) error {
	command := fmt.Sprintf("MCUKeyShortClick:%d", preset)
	if index != nil {
		command = fmt.Sprintf("%s:%d", command, *index)
	}
	_, err := c.Command(command)
	return err
}

func (c *Client) expectMap(command string) (map[string]any, error) {
	value, err := c.Command(command)
	if err != nil {
		return nil, err
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil, runtimef("%s returned non-JSON response", command)
	}
	return m, nil
}

func (c *Client) request(rawURL string) (any, error) {
	resp, err := c.HTTPClient.Get(rawURL)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, fmt.Errorf("could not connect to %s within %.1fs: %w", c.Host, c.Timeout.Seconds(), err)
		}
		return nil, fmt.Errorf("could not connect to %s: %w", c.Host, err)
	}
	defer resp.Body.Close()
	body, err := readLimitedResponse(resp.Body, wiimAPIResponseLimit)
	if err != nil {
		return nil, runtimef("could not read response from %s: %v", c.Host, err)
	}
	text := strings.TrimSpace(string(body))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, runtimef("WiiM API returned HTTP %d: %s", resp.StatusCode, responseSnippet(text))
	}
	if text == "" {
		return "", nil
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	if value, err := decodeJSONValue(decoder, 0); err == nil {
		if _, err := decoder.Token(); err == io.EOF {
			return value, nil
		}
	}
	return text, nil
}

const maxDeviceJSONDepth = 128

// decodeJSONValue decodes one JSON value without allowing duplicate object
// keys to overwrite a previously decoded value. Decoder.Token honors
// UseNumber, so all JSON numbers retain their original representation.
func decodeJSONValue(decoder *json.Decoder, depth int) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	return decodeJSONToken(decoder, token, depth)
}

func decodeJSONToken(decoder *json.Decoder, token json.Token, depth int) (any, error) {
	switch token := token.(type) {
	case json.Delim:
		if depth >= maxDeviceJSONDepth {
			return nil, fmt.Errorf("JSON nesting exceeds maximum depth %d", maxDeviceJSONDepth)
		}
		switch token {
		case '{':
			object := make(map[string]any)
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, fmt.Errorf("JSON object key is not a string")
				}
				if _, exists := object[key]; exists {
					return nil, fmt.Errorf("duplicate JSON object key %q", key)
				}
				value, err := decodeJSONValue(decoder, depth+1)
				if err != nil {
					return nil, err
				}
				object[key] = value
			}
			if end, err := decoder.Token(); err != nil {
				return nil, err
			} else if end != json.Delim('}') {
				return nil, fmt.Errorf("JSON object has unexpected closing token %v", end)
			}
			return object, nil
		case '[':
			array := make([]any, 0)
			for decoder.More() {
				value, err := decodeJSONValue(decoder, depth+1)
				if err != nil {
					return nil, err
				}
				array = append(array, value)
			}
			if end, err := decoder.Token(); err != nil {
				return nil, err
			} else if end != json.Delim(']') {
				return nil, fmt.Errorf("JSON array has unexpected closing token %v", end)
			}
			return array, nil
		default:
			return nil, fmt.Errorf("unexpected JSON delimiter %q", token)
		}
	case string, bool, nil, json.Number:
		return token, nil
	default:
		return nil, fmt.Errorf("unexpected JSON token %v", token)
	}
}

func responseSnippet(text string) string {
	const snippetMaxLen = 200
	text = strings.ReplaceAll(text, "\n", " ")
	if text == "" {
		return "empty response body"
	}
	if len(text) > snippetMaxLen {
		return text[:snippetMaxLen] + "..."
	}
	return text
}

// DecodeHexText decodes a hex-encoded string. If the input is not valid hex
// the original string is returned unchanged.
func DecodeHexText(value string) string {
	if value == "" {
		return ""
	}
	out, err := hex.DecodeString(value)
	if err != nil {
		return value
	}
	return string(out)
}
