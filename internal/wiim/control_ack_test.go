package wiim

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// All fixtures use an in-memory RoundTripper: no sockets or device access.
func acknowledgementClient(body string, calls *int) *Client {
	client := NewClient("fixture.invalid", 3)
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		response := body
		if r.URL.Query().Get("command") == "getPlayerStatus" {
			response = `{"vol":"30"}`
		} else {
			(*calls)++
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(response)), Header: make(http.Header)}, nil
	})}
	return client
}

func TestControlAcknowledgementTypedMethods(t *testing.T) {
	const mediaURL = "https://example.invalid/private?token=do-not-echo"
	index := 2
	methods := []struct {
		name string
		call func(*Client) error
	}{
		{"volume", func(c *Client) error { return c.SetVolume(30) }},
		{"volume-up", func(c *Client) error { return c.VolumeUp(1) }},
		{"volume-down", func(c *Client) error { return c.VolumeDown(1) }},
		{"mute", func(c *Client) error { return c.Mute(true) }},
		{"unmute", func(c *Client) error { return c.Mute(false) }},
		{"playback", func(c *Client) error { return c.Playback("pause") }},
		{"input", func(c *Client) error { return c.SwitchInput("wifi") }},
		{"url", func(c *Client) error { return c.PlayURL(mediaURL) }},
		{"m3u", func(c *Client) error { return c.PlayM3U(mediaURL) }},
		{"prompt", func(c *Client) error { return c.PlayPromptURL(mediaURL) }},
		{"clear", func(c *Client) error { return c.ClearPlaylist() }},
		{"seek", func(c *Client) error { return c.Seek(1) }},
		{"preset", func(c *Client) error { return c.PlayPreset(1, nil) }},
		{"preset-index", func(c *Client) error { return c.PlayPreset(1, &index) }},
	}
	for _, method := range methods {
		for _, body := range []string{"Failed", `"Failed"`, " \r\nFailed\t", `{"status":"Failed"}`, `{"status":"Failed","detail":"private-url\u001b[31m"}`} {
			t.Run(method.name+"/"+body, func(t *testing.T) {
				calls := 0
				err := method.call(acknowledgementClient(body, &calls))
				var runtimeErr RuntimeError
				if !errors.As(err, &runtimeErr) || ExitCode(err) != 1 {
					t.Fatalf("error = %v (%T), want runtime error", err, err)
				}
				if err.Error() != "wiim: WiiM device rejected control command: Failed" {
					t.Fatalf("unexpected or reflected error: %q", err.Error())
				}
				if calls != 1 {
					t.Fatalf("control requests = %d, want one without retry", calls)
				}
			})
		}
	}
}

func TestControlAcknowledgementPreservesUnknownForms(t *testing.T) {
	for _, body := range []string{
		"OK", "", "unknown command", "FAIL", "FAILURE", "failed", "Failed later",
		`" Failed "`, `{"status":"OK"}`, `{"status":"failed"}`, `{"Status":"Failed"}`,
		`{"detail":"Failed"}`, `{"status":false}`, `{"status":null}`, `{"status":1}`,
		`{"nested":{"status":"Failed"}}`, `["Failed"]`, `null`, `true`, `42`,
		`{"status":"Failed","status":"OK"}`, `{"status":"Failed","stat\u0075s":"OK"}`, `{"status":"Failed"} trailing`,
	} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			if err := acknowledgementClient(body, &calls).PlayURL("https://example.invalid/audio"); err != nil {
				t.Fatalf("unknown/accepted response rejected: %v", err)
			}
			if calls != 1 {
				t.Fatalf("requests = %d, want one", calls)
			}
		})
	}
}

func TestControlAcknowledgementPreservesRawAndReads(t *testing.T) {
	for _, body := range []string{"Failed", `{"status":"Failed"}`} {
		calls := 0
		client := acknowledgementClient(body, &calls)
		value, err := client.Command("arbitrary:raw")
		if err != nil {
			t.Fatalf("raw response rejected: %v", err)
		}
		if body == "Failed" {
			if value != "Failed" {
				t.Fatalf("raw = %#v", value)
			}
			continue
		}
		object, ok := value.(map[string]any)
		if !ok || object["status"] != "Failed" {
			t.Fatalf("raw object = %#v", value)
		}
		for _, read := range []func() (map[string]any, error){client.StatusEx, client.CastInfo} {
			value, err := read()
			if err != nil || value["status"] != "Failed" {
				t.Fatalf("read = %#v, %v", value, err)
			}
		}
	}
}

func TestControlAcknowledgementPreservesTransportErrors(t *testing.T) {
	want := errors.New("synthetic transport failure")
	client := NewClient("fixture.invalid", 3)
	calls := 0
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		calls++
		return nil, want
	})}
	if err := client.Playback("pause"); !errors.Is(err, want) {
		t.Fatalf("error = %v, want original transport cause", err)
	}
	if calls != 1 {
		t.Fatalf("requests = %d, want one", calls)
	}
}

func TestControlAcknowledgementCLI(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		for _, body := range []string{"Failed", `{"status":"Failed"}`} {
			t.Run(body+"/json="+map[bool]string{true: "true", false: "false"}[asJSON], func(t *testing.T) {
				calls := 0
				client := acknowledgementClient(body, &calls)
				previous := newDevice
				newDevice = func(_ string, _ float64) device { return client }
				t.Cleanup(func() { newDevice = previous })
				args := []string{"--config", filepath.Join(t.TempDir(), "config.json"), "--host", "fixture.invalid", "play-url", "https://example.invalid/private?token=do-not-echo"}
				if asJSON {
					args = append(args, "--json")
				}
				var stdout, stderr bytes.Buffer
				err := Run(args, &stdout, &stderr)
				if ExitCode(err) != 1 || stdout.Len() != 0 || calls != 1 {
					t.Fatalf("err=%v stdout=%q requests=%d", err, stdout.String(), calls)
				}
				if strings.Contains(stderr.String(), "do-not-echo") || strings.Contains(stderr.String(), "example.invalid") {
					t.Fatalf("private media URL reflected: %q", stderr.String())
				}
				if asJSON {
					var envelope errorEnvelope
					if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
						t.Fatal(err)
					}
					if envelope.Error.Kind != "runtime" || envelope.Error.ExitCode != 1 || envelope.Error.Message != "WiiM device rejected control command: Failed" {
						t.Fatalf("envelope = %#v", envelope)
					}
				} else if stderr.String() != "wiim: WiiM device rejected control command: Failed\n" {
					t.Fatalf("stderr = %q", stderr.String())
				}
			})
		}
	}
}
