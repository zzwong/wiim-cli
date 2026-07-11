package wiim

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestNormalizeStatusCombinesSources(t *testing.T) {
	status := NormalizeStatus("192.0.2.10", map[string]any{"project": "WiiM_Ultra", "firmware": "fw", "internet": "1", "RSSI": "-62", "wlanFreq": "5", "wlanSnr": "28"}, map[string]any{"vol": "38", "mute": "0", "status": "stop"}, map[string]any{"name": "WiiM Ultra"})
	if status.Name != "WiiM Ultra" || status.Host != "192.0.2.10" || status.Model != "WiiM_Ultra" {
		t.Fatalf("status %#v", status)
	}
	if status.Online == nil || !*status.Online {
		t.Fatalf("online %#v", status.Online)
	}
	if status.Volume == nil || *status.Volume != 38 {
		t.Fatalf("volume %#v", status.Volume)
	}
	if status.Muted == nil || *status.Muted {
		t.Fatalf("muted %#v", status.Muted)
	}
}

func TestNormalizeStatusAcceptsJSONNumber(t *testing.T) {
	status := NormalizeStatus("192.0.2.10", nil, map[string]any{"vol": json.Number("38")}, nil)
	if status.Volume == nil || *status.Volume != 38 {
		t.Fatalf("volume %#v", status.Volume)
	}
}

func TestIntPtrNumericConversions(t *testing.T) {
	maxIntValue := int(^uint(0) >> 1)
	maxText := strconv.Itoa(maxIntValue)
	valid := []struct {
		name  string
		value any
		want  int
	}{
		{"numeric string", "38", 38},
		{"json number", json.Number("38"), 38},
		{"float32", float32(38), 38},
		{"float64", float64(38), 38},
		{"maximum numeric string", maxText, maxIntValue},
		{"maximum json number", json.Number(maxText), maxIntValue},
	}
	if strconv.IntSize == 32 {
		valid = append(valid, struct {
			name  string
			value any
			want  int
		}{"maximum float64", float64(maxIntValue), maxIntValue})
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			got := intPtr(tc.value)
			if got == nil || *got != tc.want {
				t.Fatalf("intPtr(%T(%v)) = %v, want %d", tc.value, tc.value, got, tc.want)
			}
		})
	}

	var overflow string
	if strconv.IntSize == 32 {
		overflow = "2147483648"
	} else {
		overflow = "9223372036854775808"
	}
	invalid := []any{
		nil,
		true,
		struct{}{},
		"38.0",
		json.Number("38.0"),
		overflow,
		json.Number(overflow),
		float64(38.5),
		math.NaN(),
		math.Inf(1),
		math.Inf(-1),
	}
	if strconv.IntSize == 32 {
		invalid = append(invalid, float64(maxIntValue)+1)
	} else {
		invalid = append(invalid, float64(maxIntValue))
	}
	for _, value := range invalid {
		if got := intPtr(value); got != nil {
			t.Errorf("intPtr(%T(%v)) = %d, want nil", value, value, *got)
		}
	}
}

func TestFormatStatusJSONAndHuman(t *testing.T) {
	b := false
	v := 10
	status := Status{Name: "WiiM Ultra", Host: "h", WiFi: WiFi{Frequency: "5745"}, Volume: &v, Muted: &b, PlaybackState: "stop"}
	text, err := FormatStatus(status, true)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["name"] != "WiiM Ultra" {
		t.Fatalf("json %s", text)
	}
	human, _ := FormatStatus(status, false)
	for _, want := range []string{"Name: WiiM Ultra", "Wi-Fi: 5 GHz, 5745 MHz", "Volume: 10", "Muted: no"} {
		if !strings.Contains(human, want) {
			t.Fatalf("missing %s in %s", want, human)
		}
	}
}

func TestFormatGroupStatusExactHumanAndJSON(t *testing.T) {
	status := GroupStatus{
		Name:        "Living Room",
		Host:        "speaker.local",
		Model:       "WiiM_Ultra",
		Firmware:    "4.8.123",
		Role:        "master",
		Grouped:     true,
		GroupName:   "Downstairs",
		MemberCount: 2,
		WMRMVersion: "4.2",
		MasterUUID:  "master-uuid",
	}
	human, err := FormatGroupStatus(status, false)
	if err != nil {
		t.Fatal(err)
	}
	wantHuman := "Name: Living Room\nHost: speaker.local\nModel: WiiM_Ultra\nFirmware: 4.8.123\nRole: master\nGrouped: yes\nGroup name: Downstairs\nMember count: 2\nWMRM version: 4.2\nMaster UUID: master-uuid"
	if human != wantHuman {
		t.Fatalf("human = %q, want %q", human, wantHuman)
	}
	output, err := FormatGroupStatus(status, true)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON := `{
  "name": "Living Room",
  "host": "speaker.local",
  "model": "WiiM_Ultra",
  "firmware": "4.8.123",
  "role": "master",
  "grouped": true,
  "groupName": "Downstairs",
  "memberCount": 2,
  "wmrmVersion": "4.2",
  "masterUUID": "master-uuid"
}`
	if output != wantJSON {
		t.Fatalf("JSON = %q, want %q", output, wantJSON)
	}
}

func TestFormatGroupStatusEscapesHumanStrings(t *testing.T) {
	unsafe := "line\nreturn\ttab\ransi\x1b[31m\"slash\\ unicode ☃\x00\x7f"
	escaped := `line\nreturn\ttab\ransi\x1b[31m\"slash\\ unicode ☃\x00\x7f`
	status := GroupStatus{
		Name:        unsafe,
		Host:        unsafe,
		Model:       unsafe,
		Firmware:    unsafe,
		Role:        unsafe,
		Grouped:     true,
		GroupName:   unsafe,
		MemberCount: 1,
		WMRMVersion: unsafe,
		MasterUUID:  unsafe,
	}

	human, err := FormatGroupStatus(status, false)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"Name: " + escaped,
		"Host: " + escaped,
		"Model: " + escaped,
		"Firmware: " + escaped,
		"Role: " + escaped,
		"Grouped: yes",
		"Group name: " + escaped,
		"Member count: 1",
		"WMRM version: " + escaped,
		"Master UUID: " + escaped,
	}, "\n")
	if human != want {
		t.Fatalf("human = %q, want %q", human, want)
	}
	assertNoRawControlsExceptNewline(t, human)
}

func TestFormatGroupMembersEscapesHumanStrings(t *testing.T) {
	unsafe := "line\nreturn\ttab\ransi\x1b[31m\"slash\\ unicode ☃\x00\x7f"
	escaped := `line\nreturn\ttab\ransi\x1b[31m\"slash\\ unicode ☃\x00\x7f`
	group := GroupMembers{Members: []GroupMember{{
		Name: unsafe, UUID: unsafe, IP: unsafe, Version: unsafe, Type: unsafe,
	}}}

	human, err := FormatGroupMembers(group, false)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"Member 1:",
		"Name: " + escaped,
		"UUID: " + escaped,
		"IP: " + escaped,
		"Version: " + escaped,
		"Type: " + escaped,
	}, "\n")
	if human != want {
		t.Fatalf("human = %q, want %q", human, want)
	}
	assertNoRawControlsExceptNewline(t, human)
}

func assertNoRawControlsExceptNewline(t *testing.T, text string) {
	t.Helper()
	for _, r := range text {
		if r != '\n' && (r < 0x20 || r == 0x7f) {
			t.Errorf("human output contains raw control U+%04X: %q", r, text)
		}
	}
}

func TestFormatGroupMembersExactHumanJSONAndOrder(t *testing.T) {
	zero := 0
	no := false
	yes := true
	group := GroupMembers{
		WMRMVersion: "4.2",
		Count:       2,
		Members: []GroupMember{
			{Name: "First", UUID: "u1", IP: "192.0.2.1", Version: "4.2", Type: "A31", Channel: &zero, Volume: &zero, Muted: &no, BatteryPercent: &zero, BatteryCharging: &no, Masked: &no},
			{Name: "Second", Volume: &zero, Muted: &yes},
		},
	}
	human, err := FormatGroupMembers(group, false)
	if err != nil {
		t.Fatal(err)
	}
	wantHuman := "Member 1:\nName: First\nUUID: u1\nIP: 192.0.2.1\nVersion: 4.2\nType: A31\nChannel: 0\nVolume: 0\nMuted: no\nBattery percent: 0\nBattery charging: no\nMasked: no\n\nMember 2:\nName: Second\nVolume: 0\nMuted: yes"
	if human != wantHuman {
		t.Fatalf("human = %q, want %q", human, wantHuman)
	}
	output, err := FormatGroupMembers(group, true)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON := `{
  "wmrmVersion": "4.2",
  "count": 2,
  "members": [
    {
      "name": "First",
      "uuid": "u1",
      "ip": "192.0.2.1",
      "version": "4.2",
      "type": "A31",
      "channel": 0,
      "volume": 0,
      "muted": false,
      "batteryPercent": 0,
      "batteryCharging": false,
      "masked": false
    },
    {
      "name": "Second",
      "volume": 0,
      "muted": true
    }
  ]
}`
	if output != wantJSON {
		t.Fatalf("JSON = %q, want %q", output, wantJSON)
	}
}

func TestFormatGroupMembersEmptyUsesNonNilJSONArray(t *testing.T) {
	human, err := FormatGroupMembers(GroupMembers{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if human != "No group members." {
		t.Fatalf("human = %q", human)
	}
	output, err := FormatGroupMembers(GroupMembers{}, true)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON := `{
  "count": 0,
  "members": []
}`
	if output != wantJSON {
		t.Fatalf("JSON = %q, want %q", output, wantJSON)
	}
}

func TestNormalizeNowPrefersMetaAndDecodesHex(t *testing.T) {
	now := NormalizeNow(map[string]any{"status": "play", "vol": "20", "mute": "1", "Title": "486578", "Artist": "417274697374"}, map[string]any{"metaData": map[string]any{"title": "Meta Title", "artist": "Meta Artist", "sampleRate": "44100"}})
	if now.Title != "Meta Title" || now.Artist != "Meta Artist" || now.SampleRate != "44100" {
		t.Fatalf("now %#v", now)
	}
	if now.Volume == nil || *now.Volume != 20 {
		t.Fatalf("volume %#v", now.Volume)
	}
	if now.Muted == nil || !*now.Muted {
		t.Fatalf("muted %#v", now.Muted)
	}
}

func TestNormalizeNowSuppressesUnknownMetadata(t *testing.T) {
	now := NormalizeNow(map[string]any{"Title": "556E6B6E6F776E", "Artist": "417274697374"}, map[string]any{"metaData": map[string]any{"title": "unknow"}})
	if now.Title != "" {
		t.Fatalf("expected empty unknown title, got %q", now.Title)
	}
	if now.Artist != "Artist" {
		t.Fatalf("expected decoded artist, got %q", now.Artist)
	}
}

func TestFormatCastMediaInfoHumanAndJSON(t *testing.T) {
	info := CastMediaInfo{App: "YouTube", PlayerState: "PLAYING", Title: "Song", Artist: "Singer", Album: "Album", ContentType: "video/mp4", ContentID: "abc123"}

	// Human readable
	human, err := FormatCastMediaInfo(info, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"App: YouTube", "State: PLAYING", "Title: Song", "Artist: Singer", "Album: Album", "Content type: video/mp4", "Content ID: abc123"} {
		if !strings.Contains(human, want) {
			t.Fatalf("missing %q in human output: %s", want, human)
		}
	}

	// Empty fields omitted
	empty := CastMediaInfo{PlayerState: "IDLE"}
	human, err = FormatCastMediaInfo(empty, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(human, "State: IDLE") || strings.Contains(human, "App:") {
		t.Fatalf("unexpected output for empty info: %s", human)
	}

	// JSON
	js, err := FormatCastMediaInfo(info, true)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(js), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["app"] != "YouTube" || decoded["title"] != "Song" {
		t.Fatalf("json %s", js)
	}
}

func TestFormatSpotifyDevicesActiveAndInactive(t *testing.T) {
	// Inactive device — ensure no double space
	inactive := map[string]any{"devices": []any{
		map[string]any{"name": "Speaker", "id": "abc", "type": "speaker"},
	}}
	out, err := FormatSpotifyDevices(inactive)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "  ") {
		t.Fatalf("double space in inactive output: %q", out)
	}
	if !strings.Contains(out, "Speaker (speaker) abc") {
		t.Fatalf("unexpected inactive output: %q", out)
	}

	// Active device
	active := map[string]any{"devices": []any{
		map[string]any{"name": "Living Room", "id": "xyz", "type": "speaker", "is_active": true},
	}}
	out, err = FormatSpotifyDevices(active)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Living Room (speaker) active xyz") {
		t.Fatalf("unexpected active output: %q", out)
	}

	// Empty devices
	empty := map[string]any{"devices": []any{}}
	out, err = FormatSpotifyDevices(empty)
	if err != nil {
		t.Fatal(err)
	}
	if out != "No Spotify devices found" {
		t.Fatalf("unexpected empty output: %q", out)
	}

	// Non-map value falls through to FormatRaw
	out, err = FormatSpotifyDevices("raw")
	if err != nil {
		t.Fatal(err)
	}
	if out != "raw" {
		t.Fatalf("expected raw string, got %q", out)
	}

	// Missing devices key
	missing := map[string]any{"foo": "bar"}
	out, err = FormatSpotifyDevices(missing)
	if err != nil {
		t.Fatal(err)
	}
	if out != "No Spotify devices found" {
		t.Fatalf("unexpected missing output: %q", out)
	}
}

func TestFormatDiscoveredHumanAndJSON(t *testing.T) {
	devices := []DiscoveredDevice{
		{IP: "10.0.0.1", Name: "WiiM Ultra", Model: "WiiM_Ultra", Firmware: "fw1"},
		{IP: "10.0.0.2", Name: "WiiM Mini"},
	}

	human, err := FormatDiscovered(devices, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(human, "Name: WiiM Ultra\nHost: 10.0.0.1\nModel: WiiM_Ultra\nFirmware: fw1") {
		t.Fatalf("missing first device block: %q", human)
	}
	if !strings.Contains(human, "Name: WiiM Mini\nHost: 10.0.0.2") || strings.Contains(human, "Mini\nModel:") {
		t.Fatalf("second device should omit empty Model/Firmware lines: %q", human)
	}

	js, err := FormatDiscovered(devices, true)
	if err != nil {
		t.Fatal(err)
	}
	var got []DiscoveredDevice
	if err := json.Unmarshal([]byte(js), &got); err != nil {
		t.Fatalf("not valid JSON: %v: %s", err, js)
	}
	if len(got) != 2 || got[0].IP != "10.0.0.1" {
		t.Fatalf("got = %+v", got)
	}

	emptyHuman, err := FormatDiscovered(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if emptyHuman != "No devices found on the local network." {
		t.Fatalf("unexpected empty output: %q", emptyHuman)
	}

	emptyJSON, err := FormatDiscovered(nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if emptyJSON != "[]" {
		t.Fatalf("empty JSON should be [], got %q", emptyJSON)
	}
}

func TestFormatPresets(t *testing.T) {
	// With presets
	presets := map[string]any{"preset_list": []any{
		map[string]any{"number": "1", "name": "Radio One", "url": "http://stream.example.com/radio"},
		map[string]any{"id": "2", "name": "Podcast"},
		map[string]any{"number": "3"}, // unnamed, no URL
	}}
	out, err := FormatPresets(presets)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1: Radio One — http://stream.example.com/radio") {
		t.Fatalf("unexpected output: %q", out)
	}
	if !strings.Contains(out, "2: Podcast") {
		t.Fatalf("unexpected output: %q", out)
	}
	if !strings.Contains(out, "3: Unnamed") {
		t.Fatalf("expected unnamed preset, got: %q", out)
	}

	// "unknow" URL should be suppressed
	unknow := map[string]any{"preset_list": []any{
		map[string]any{"number": "1", "name": "N", "url": "unknow"},
	}}
	out, err = FormatPresets(unknow)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, " — ") {
		t.Fatalf("should not include URL for 'unknow': %q", out)
	}

	// No presets
	none := map[string]any{}
	out, err = FormatPresets(none)
	if err != nil {
		t.Fatal(err)
	}
	if out != "No presets configured" {
		t.Fatalf("expected no presets, got: %q", out)
	}

	// Non-map value
	out, err = FormatPresets("text")
	if err != nil {
		t.Fatal(err)
	}
	if out != "text" {
		t.Fatalf("expected raw string, got: %q", out)
	}
}

func TestDecodeHexTextEdgeCases(t *testing.T) {
	// Empty string
	if got := DecodeHexText(""); got != "" {
		t.Fatalf("empty: got %q", got)
	}

	// Invalid hex — returns original
	if got := DecodeHexText("not-hex"); got != "not-hex" {
		t.Fatalf("invalid hex: got %q", got)
	}

	// Valid UTF-8
	if got := DecodeHexText("48656c6c6f"); got != "Hello" {
		t.Fatalf("valid hex: got %q", got)
	}

	// Odd-length hex (invalid)
	if got := DecodeHexText("48656c6c6"); got != "48656c6c6" {
		t.Fatalf("odd-length hex: got %q", got)
	}

	// Non-UTF-8 bytes (should still produce string)
	got := DecodeHexText("80")
	if len(got) != 1 || got[0] != 128 { // 0x80 is valid Go string byte
		t.Fatalf("non-utf8 hex: got %q (len=%d)", got, len(got))
	}
}

func TestFormatNowHumanHandlesMissingMetadata(t *testing.T) {
	v := 5
	m := false
	text, err := FormatNow(Now{PlaybackState: "stop", Volume: &v, Muted: &m}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "State: stop") || !strings.Contains(text, "Volume: 5") {
		t.Fatalf("text %s", text)
	}
}

func TestFormatDeviceProfilesHumanAndJSON(t *testing.T) {
	cfg := Config{
		DefaultDevice: "office",
		Devices: map[string]DeviceProfile{
			"zulu":   {Host: "zulu-host"},
			"office": {Host: "office-host"},
			"alpha":  {Host: "alpha-host"},
		},
	}

	human, err := FormatDeviceProfiles(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	wantHuman := "NAME\tHOST\tDEFAULT\nalpha\talpha-host\t\noffice\toffice-host\t*\nzulu\tzulu-host\t"
	if human != wantHuman {
		t.Fatalf("human output = %q, want %q", human, wantHuman)
	}

	jsonOutput, err := FormatDeviceProfiles(cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON := `[{"name":"alpha","host":"alpha-host","default":false},{"name":"office","host":"office-host","default":true},{"name":"zulu","host":"zulu-host","default":false}]`
	if jsonOutput != wantJSON {
		t.Fatalf("JSON output = %q, want %q", jsonOutput, wantJSON)
	}
	var decoded any
	if err := json.Unmarshal([]byte(jsonOutput), &decoded); err != nil {
		t.Fatalf("JSON output is not valid JSON: %v", err)
	}
	profiles, ok := decoded.([]any)
	if !ok || len(profiles) != 3 {
		t.Fatalf("decoded profiles = %#v, want array of 3 objects", decoded)
	}
	for i, value := range profiles {
		profile, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("profile %d = %T, want object", i, value)
		}
		if _, ok := profile["name"].(string); !ok {
			t.Fatalf("profile %d name = %T, want string", i, profile["name"])
		}
		if _, ok := profile["host"].(string); !ok {
			t.Fatalf("profile %d host = %T, want string", i, profile["host"])
		}
		defaultValue, ok := profile["default"].(bool)
		if !ok || defaultValue != (i == 1) {
			t.Fatalf("profile %d default = %#v, want bool %t", i, profile["default"], i == 1)
		}
	}

	emptyHuman, err := FormatDeviceProfiles(Config{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if emptyHuman != "No saved devices." {
		t.Fatalf("empty human output = %q", emptyHuman)
	}
	emptyJSON, err := FormatDeviceProfiles(Config{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if emptyJSON != "[]" {
		t.Fatalf("empty JSON output = %q, want []", emptyJSON)
	}
}
