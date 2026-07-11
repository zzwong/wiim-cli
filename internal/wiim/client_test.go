package wiim

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDecodeHexText(t *testing.T) {
	if got := DecodeHexText("48656c6c6f"); got != "Hello" {
		t.Fatalf("got %q", got)
	}
	if got := DecodeHexText("not-hex"); got != "not-hex" {
		t.Fatalf("got %q", got)
	}
}

func TestCommandBuildsHTTPSURLAndDecodesJSON(t *testing.T) {
	var seen string
	client := NewClient("192.0.2.10", 1.5)
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		seen = r.URL.String()
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":"yes"}`)), Header: make(http.Header)}, nil
	})}
	value, err := client.Command("getStatusEx")
	if err != nil {
		t.Fatal(err)
	}
	if seen != "https://192.0.2.10/httpapi.asp?command=getStatusEx" {
		t.Fatalf("url %s", seen)
	}
	m := value.(map[string]any)
	if m["ok"] != "yes" {
		t.Fatalf("value %#v", value)
	}
}

func TestCommandUsesJSONNumbersAndRejectsTrailingGarbage(t *testing.T) {
	responses := []string{
		`{"large":9007199254740993,"vol":38}`,
		" \n{\"ok\":true} trailing\t",
	}
	client := NewClient("host", 3)
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		body := responses[0]
		responses = responses[1:]
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}

	value, err := client.Command("first")
	if err != nil {
		t.Fatal(err)
	}
	decoded, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value = %T, want object", value)
	}
	if number, ok := decoded["large"].(json.Number); !ok || number != "9007199254740993" {
		t.Fatalf("large = %#v (%T), want json.Number", decoded["large"], decoded["large"])
	}
	if volume := intPtr(decoded["vol"]); volume == nil || *volume != 38 {
		t.Fatalf("volume = %#v, want 38", volume)
	}

	value, err = client.Command("second")
	if err != nil {
		t.Fatal(err)
	}
	if value != `{"ok":true} trailing` {
		t.Fatalf("value = %#v, want trimmed text fallback", value)
	}
}

func TestCommandRejectsDuplicateJSONObjectKeys(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "top level", body: `{"slaves":0,"slaves":1,"slave_list":[]}`},
		{name: "nested member", body: `{"slaves":1,"slave_list":[{"name":"Kitchen","name":"Office"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewClient("host", 3)
			client.HTTPClient = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}

			value, err := client.Command("getStatusEx")
			if err != nil {
				t.Fatal(err)
			}
			if value != tc.body {
				t.Fatalf("value = %#v, want plain-text fallback %q", value, tc.body)
			}
		})
	}
}

func TestCommandFallsBackForMultipleTopLevelJSONValues(t *testing.T) {
	const body = `{"ok":true} {"next":true}`
	client := NewClient("host", 3)
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}

	value, err := client.Command("getStatusEx")
	if err != nil {
		t.Fatal(err)
	}
	if value != body {
		t.Fatalf("value = %#v, want plain-text fallback %q", value, body)
	}
}

func TestCommandDecodesScalarJSONValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want any
	}{
		{name: "string", body: `"ready"`, want: "ready"},
		{name: "bool", body: `true`, want: true},
		{name: "null", body: `null`, want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewClient("host", 3)
			client.HTTPClient = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}

			value, err := client.Command("getStatusEx")
			if err != nil {
				t.Fatal(err)
			}
			if value != tc.want {
				t.Fatalf("value = %#v (%T), want %#v (%T)", value, value, tc.want, tc.want)
			}
		})
	}
}

func TestCommandBoundsDeviceJSONNesting(t *testing.T) {
	for _, tc := range []struct {
		name     string
		open     string
		close    string
		wantType string
	}{
		{name: "arrays", open: "[", close: "]", wantType: "array"},
		{name: "objects", open: `{"value":`, close: "}", wantType: "object"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, depth := range []int{maxDeviceJSONDepth, maxDeviceJSONDepth + 1} {
				body := strings.Repeat(tc.open, depth) + "null" + strings.Repeat(tc.close, depth)
				client := NewClient("host", 3)
				client.HTTPClient = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
				})}

				value, err := client.Command("getStatusEx")
				if err != nil {
					t.Fatal(err)
				}
				if depth == maxDeviceJSONDepth {
					var ok bool
					if tc.wantType == "array" {
						_, ok = value.([]any)
					} else {
						_, ok = value.(map[string]any)
					}
					if !ok {
						t.Fatalf("depth %d value = %T, want %s", depth, value, tc.wantType)
					}
				} else if value != body {
					t.Fatalf("depth %d value = %#v, want plain-text fallback %q", depth, value, body)
				}
			}
		})
	}
}

func TestCommandDecodesUniqueNestedJSON(t *testing.T) {
	const body = `{"slaves":1,"slave_list":[{"name":"Kitchen","details":{"volume":9007199254740993}}],"active":true,"unset":null}`
	client := NewClient("host", 3)
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}

	value, err := client.Command("multiroom:getSlaveList")
	if err != nil {
		t.Fatal(err)
	}
	response, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value = %T, want object", value)
	}
	if response["active"] != true {
		t.Fatalf("active = %#v, want true", response["active"])
	}
	if unset, exists := response["unset"]; !exists || unset != nil {
		t.Fatalf("unset = %#v, exists = %t; want present null", unset, exists)
	}
	members, ok := response["slave_list"].([]any)
	if !ok || len(members) != 1 {
		t.Fatalf("slave_list = %#v, want one-element array", response["slave_list"])
	}
	member, ok := members[0].(map[string]any)
	if !ok || member["name"] != "Kitchen" {
		t.Fatalf("member = %#v", members[0])
	}
	details, ok := member["details"].(map[string]any)
	if !ok {
		t.Fatalf("details = %#v, want object", member["details"])
	}
	if number, ok := details["volume"].(json.Number); !ok || number != "9007199254740993" {
		t.Fatalf("volume = %#v (%T), want json.Number", details["volume"], details["volume"])
	}
}

func TestPlaybackURLCommands(t *testing.T) {
	var seen []string
	client := NewClient("host", 3)
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		seen = append(seen, r.URL.Query().Get("command"))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`OK`)), Header: make(http.Header)}, nil
	})}
	if err := client.PlayURL("https://example.com/a.mp3"); err != nil {
		t.Fatal(err)
	}
	if err := client.PlayM3U("https://example.com/a.m3u"); err != nil {
		t.Fatal(err)
	}
	idx := 1
	if err := client.PlayPreset(2, &idx); err != nil {
		t.Fatal(err)
	}
	want := []string{"setPlayerCmd:play:https://example.com/a.mp3", "setPlayerCmd:playlist:https://example.com/a.m3u", "MCUKeyShortClick:2:1"}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("seen %#v", seen)
		}
	}
}

func TestRelativeVolumeUsesAbsoluteSetFromCurrentVolume(t *testing.T) {
	var seen []string
	client := NewClient("host", 3)
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		command := r.URL.Query().Get("command")
		seen = append(seen, command)
		body := `OK`
		if command == "getPlayerStatus" {
			body = `{"vol":"38"}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	if err := client.VolumeUp(2); err != nil {
		t.Fatal(err)
	}
	if err := client.VolumeDown(3); err != nil {
		t.Fatal(err)
	}
	want := []string{"getPlayerStatus", "setPlayerCmd:vol:40", "getPlayerStatus", "setPlayerCmd:vol:35"}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("seen %#v", seen)
		}
	}
}

func TestCommandURLEncodesCommand(t *testing.T) {
	var seen string
	client := NewClient("host", 3)
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		seen = r.URL.RawQuery
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`OK`)), Header: make(http.Header)}, nil
	})}
	value, err := client.Command("setPlayerCmd:vol:30")
	if err != nil {
		t.Fatal(err)
	}
	if value != "OK" {
		t.Fatalf("value %#v", value)
	}
	if seen != "command=setPlayerCmd%3Avol%3A30" {
		t.Fatalf("query %s", seen)
	}
}

func TestNon2xxResponseIsRuntimeError(t *testing.T) {
	client := NewClient("host", 3)
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader(`server exploded`)), Header: make(http.Header)}, nil
	})}
	_, err := client.Command("getStatusEx")
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") || !strings.Contains(err.Error(), "server exploded") {
		t.Fatalf("err %v", err)
	}
}

func TestClientRejectsOversizedSuccessAndErrorResponses(t *testing.T) {
	const marker = "oversized-wiim-response"
	body := marker + strings.Repeat("x", int(wiimAPIResponseLimit)-len(marker)+1)
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := NewClient("host", 3)
			client.HTTPClient = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			_, err := client.Command("getStatusEx")
			if err == nil {
				t.Fatal("expected oversized response error")
			}
			if _, ok := err.(RuntimeError); !ok {
				t.Fatalf("error type %T, want RuntimeError: %v", err, err)
			}
			if !strings.Contains(err.Error(), "response exceeds 1048576 bytes") {
				t.Fatalf("error %v", err)
			}
			if strings.Contains(err.Error(), marker) {
				t.Fatalf("error reflected oversized body: %v", err)
			}
		})
	}
}

type testNetError struct {
	msg       string
	timeout   bool
	temporary bool
}

func (e testNetError) Error() string   { return e.msg }
func (e testNetError) Timeout() bool   { return e.timeout }
func (e testNetError) Temporary() bool { return e.temporary }

func TestTimeoutErrorIncludesWithinMessage(t *testing.T) {
	client := NewClient("host", 3)
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, testNetError{msg: "dial tcp host:443: i/o timeout", timeout: true}
	})}
	_, err := client.Command("getStatusEx")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "within 3.0s") {
		t.Fatalf("timeout error should mention the timeout duration, got: %v", err)
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("timeout error should unwrap to a net.Error with Timeout()=true, got: %v", err)
	}
}

func TestNonTimeoutErrorIncludesUnderlyingError(t *testing.T) {
	client := NewClient("host", 3)
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, testNetError{msg: "dial tcp host:443: connect: connection refused", timeout: false}
	})}
	_, err := client.Command("getStatusEx")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "within") {
		t.Fatalf("non-timeout error should not mention timeout duration, got: %v", err)
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("non-timeout error should include underlying error message, got: %v", err)
	}
	var netErr net.Error
	if !errors.As(err, &netErr) {
		t.Fatalf("non-timeout error should unwrap to a net.Error, got: %v", err)
	}
}

func TestCastInfoUsesHTTP8008(t *testing.T) {
	var seen string
	client := NewClient("192.0.2.10", 2)
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		seen = r.URL.String()
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"name":"WiiM Ultra"}`)), Header: make(http.Header)}, nil
	})}
	info, err := client.CastInfo()
	if err != nil {
		t.Fatal(err)
	}
	if seen != "http://192.0.2.10:8008/setup/eureka_info" {
		t.Fatalf("url %s", seen)
	}
	if info["name"] != "WiiM Ultra" {
		t.Fatalf("info %#v", info)
	}
}
