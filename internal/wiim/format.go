package wiim

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Status holds aggregated device information from statusEx, player, and Cast sources.
type Status struct {
	Name          string `json:"name,omitempty"`
	Host          string `json:"host"`
	Model         string `json:"model,omitempty"`
	Firmware      string `json:"firmware,omitempty"`
	Release       string `json:"release,omitempty"`
	MAC           string `json:"mac,omitempty"`
	IPAddress     string `json:"ipAddress,omitempty"`
	Online        *bool  `json:"online,omitempty"`
	WiFi          WiFi   `json:"wifi"`
	Volume        *int   `json:"volume,omitempty"`
	Muted         *bool  `json:"muted,omitempty"`
	PlaybackState string `json:"playbackState,omitempty"`
}

// WiFi holds wireless connection metrics reported by the device.
type WiFi struct {
	Frequency string `json:"frequency,omitempty"`
	RSSI      *int   `json:"rssi,omitempty"`
	SNR       *int   `json:"snr,omitempty"`
	Noise     *int   `json:"noise,omitempty"`
	DataRate  string `json:"dataRate,omitempty"`
}

// Now holds the current playback metadata (track info, volume, player state).
type Now struct {
	PlaybackState string `json:"playbackState,omitempty"`
	Volume        *int   `json:"volume,omitempty"`
	Muted         *bool  `json:"muted,omitempty"`
	Title         string `json:"title,omitempty"`
	Artist        string `json:"artist,omitempty"`
	Album         string `json:"album,omitempty"`
	AlbumArtURI   string `json:"albumArtURI,omitempty"`
	SampleRate    string `json:"sampleRate,omitempty"`
	BitDepth      string `json:"bitDepth,omitempty"`
	BitRate       string `json:"bitRate,omitempty"`
	Position      string `json:"position,omitempty"`
	Duration      string `json:"duration,omitempty"`
}

// NormalizeStatus merges data from getStatusEx, getPlayerStatus, and CastInfo into a
// single Status struct, picking the best available value for each field.
func NormalizeStatus(host string, statusEx, player, cast map[string]any) Status {
	return Status{
		Name:          firstString(cast, "name", firstString(statusEx, "DeviceName", firstString(statusEx, "ssid", ""))),
		Host:          host,
		Model:         stringValue(statusEx["project"]),
		Firmware:      stringValue(statusEx["firmware"]),
		Release:       stringValue(statusEx["Release"]),
		MAC:           stringValue(statusEx["MAC"]),
		IPAddress:     firstString(statusEx, "apcli0", stringValue(cast["ip_address"])),
		Online:        bool01(statusEx["internet"]),
		WiFi:          WiFi{Frequency: stringValue(statusEx["wlanFreq"]), RSSI: intPtr(statusEx["RSSI"]), SNR: intPtr(statusEx["wlanSnr"]), Noise: intPtr(statusEx["wlanNoise"]), DataRate: stringValue(statusEx["wlanDataRate"])},
		Volume:        intPtr(player["vol"]),
		Muted:         bool01(player["mute"]),
		PlaybackState: stringValue(player["status"]),
	}
}

// NormalizeNow merges player status and track metadata (from getMetaInfo) into a Now struct.
// Titles and artist names are hex-decoded and cleaned of "unknown" placeholders.
func NormalizeNow(player, meta map[string]any) Now {
	metadata, _ := meta["metaData"].(map[string]any)
	return Now{
		PlaybackState: stringValue(player["status"]),
		Volume:        intPtr(player["vol"]),
		Muted:         bool01(player["mute"]),
		Title:         cleanMetadataText(firstString(metadata, "title", DecodeHexText(stringValue(player["Title"])))),
		Artist:        cleanMetadataText(firstString(metadata, "artist", DecodeHexText(stringValue(player["Artist"])))),
		Album:         cleanMetadataText(firstString(metadata, "album", DecodeHexText(stringValue(player["Album"])))),
		AlbumArtURI:   stringValue(metadata["albumArtURI"]),
		SampleRate:    stringValue(metadata["sampleRate"]),
		BitDepth:      stringValue(metadata["bitDepth"]),
		BitRate:       stringValue(metadata["bitRate"]),
		Position:      stringValue(player["curpos"]),
		Duration:      stringValue(player["totlen"]),
	}
}

// FormatStatus formats a Status struct as human-readable key: value lines or as JSON.
func FormatStatus(status Status, asJSON bool) (string, error) {
	if asJSON {
		return jsonText(status)
	}
	var lines []string
	if status.Name != "" {
		lines = append(lines, "Name: "+status.Name)
	}
	lines = append(lines, "Host: "+status.Host)
	if status.Model != "" {
		lines = append(lines, "Model: "+status.Model)
	}
	if status.Firmware != "" {
		lines = append(lines, "Firmware: "+status.Firmware)
	}
	if status.Online != nil {
		lines = append(lines, "Online: "+yesNo(*status.Online))
	}
	var wifi []string
	if freq := formatWiFiFrequency(status.WiFi.Frequency); freq != "" {
		wifi = append(wifi, freq)
	}
	if status.WiFi.RSSI != nil {
		wifi = append(wifi, fmt.Sprintf("RSSI %d dBm", *status.WiFi.RSSI))
	}
	if status.WiFi.SNR != nil {
		wifi = append(wifi, fmt.Sprintf("SNR %d", *status.WiFi.SNR))
	}
	if len(wifi) > 0 {
		lines = append(lines, "Wi-Fi: "+strings.Join(wifi, ", "))
	}
	if status.Volume != nil {
		lines = append(lines, fmt.Sprintf("Volume: %d", *status.Volume))
	}
	if status.Muted != nil {
		lines = append(lines, "Muted: "+yesNo(*status.Muted))
	}
	if status.PlaybackState != "" {
		lines = append(lines, "State: "+status.PlaybackState)
	}
	return strings.Join(lines, "\n"), nil
}

// FormatNow formats a Now struct as human-readable key: value lines or as JSON.
// Empty fields are omitted from the output.
// FormatGroupStatus formats a GroupStatus as compact labeled fields or JSON.
func FormatGroupStatus(status GroupStatus, asJSON bool) (string, error) {
	if asJSON {
		return jsonText(status)
	}
	var lines []string
	if status.Name != "" {
		lines = append(lines, "Name: "+status.Name)
	}
	lines = append(lines, "Host: "+status.Host)
	if status.Model != "" {
		lines = append(lines, "Model: "+status.Model)
	}
	if status.Firmware != "" {
		lines = append(lines, "Firmware: "+status.Firmware)
	}
	lines = append(lines,
		"Role: "+status.Role,
		"Grouped: "+yesNo(status.Grouped),
	)
	if status.GroupName != "" {
		lines = append(lines, "Group name: "+status.GroupName)
	}
	lines = append(lines, fmt.Sprintf("Member count: %d", status.MemberCount))
	if status.WMRMVersion != "" {
		lines = append(lines, "WMRM version: "+status.WMRMVersion)
	}
	if status.MasterUUID != "" {
		lines = append(lines, "Master UUID: "+status.MasterUUID)
	}
	return strings.Join(lines, "\n"), nil
}

// FormatGroupMembers formats group members in the API's order as labeled
// blocks, or serializes the stable GroupMembers schema as JSON.
func FormatGroupMembers(group GroupMembers, asJSON bool) (string, error) {
	if asJSON {
		if group.Members == nil {
			group.Members = []GroupMember{}
		}
		return jsonText(group)
	}
	if len(group.Members) == 0 {
		return "No group members.", nil
	}
	blocks := make([]string, 0, len(group.Members))
	for index, member := range group.Members {
		lines := []string{fmt.Sprintf("Member %d:", index+1)}
		for _, field := range [][2]string{
			{"Name", member.Name},
			{"UUID", member.UUID},
			{"IP", member.IP},
			{"Version", member.Version},
			{"Type", member.Type},
		} {
			if field[1] != "" {
				lines = append(lines, field[0]+": "+field[1])
			}
		}
		if member.Channel != nil {
			lines = append(lines, fmt.Sprintf("Channel: %d", *member.Channel))
		}
		if member.Volume != nil {
			lines = append(lines, fmt.Sprintf("Volume: %d", *member.Volume))
		}
		if member.Muted != nil {
			lines = append(lines, "Muted: "+yesNo(*member.Muted))
		}
		if member.BatteryPercent != nil {
			lines = append(lines, fmt.Sprintf("Battery percent: %d", *member.BatteryPercent))
		}
		if member.BatteryCharging != nil {
			lines = append(lines, "Battery charging: "+yesNo(*member.BatteryCharging))
		}
		if member.Masked != nil {
			lines = append(lines, "Masked: "+yesNo(*member.Masked))
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n"), nil
}

func FormatNow(now Now, asJSON bool) (string, error) {
	if asJSON {
		return jsonText(now)
	}
	fields := [][2]string{{"State", now.PlaybackState}, {"Title", now.Title}, {"Artist", now.Artist}, {"Album", now.Album}, {"Sample rate", now.SampleRate}, {"Bit depth", now.BitDepth}, {"Bit rate", now.BitRate}, {"Position", now.Position}, {"Duration", now.Duration}}
	var lines []string
	for _, f := range fields {
		if f[1] != "" {
			lines = append(lines, f[0]+": "+f[1])
		}
	}
	if now.Volume != nil {
		lines = append(lines, fmt.Sprintf("Volume: %d", *now.Volume))
	}
	if now.Muted != nil {
		lines = append(lines, "Muted: "+yesNo(*now.Muted))
	}
	return strings.Join(lines, "\n"), nil
}

// FormatPresets formats the getPresetInfo response as a human-readable list.
// Non-map values are passed through FormatRaw.
func FormatPresets(value any) (string, error) {
	m, ok := value.(map[string]any)
	if !ok {
		return FormatRaw(value)
	}
	list, ok := m["preset_list"].([]any)
	if !ok || len(list) == 0 {
		return "No presets configured", nil
	}
	var lines []string
	for _, item := range list {
		preset, _ := item.(map[string]any)
		number := stringValue(preset["number"])
		if number == "" {
			number = stringValue(preset["id"])
		}
		name := firstString(preset, "name", "Unnamed")
		url := stringValue(preset["url"])
		line := strings.TrimSpace(fmt.Sprintf("%s: %s", number, name))
		if url != "" && url != "unknow" {
			line += " — " + url
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

// FormatCastMediaInfo formats a CastMediaInfo struct as human-readable lines or as JSON.
func FormatCastMediaInfo(info CastMediaInfo, asJSON bool) (string, error) {
	if asJSON {
		return jsonText(info)
	}
	var lines []string
	if info.App != "" {
		lines = append(lines, "App: "+info.App)
	}
	if info.PlayerState != "" {
		lines = append(lines, "State: "+info.PlayerState)
	}
	if info.Title != "" {
		lines = append(lines, "Title: "+info.Title)
	}
	if info.Artist != "" {
		lines = append(lines, "Artist: "+info.Artist)
	}
	if info.Album != "" {
		lines = append(lines, "Album: "+info.Album)
	}
	if info.ContentType != "" {
		lines = append(lines, "Content type: "+info.ContentType)
	}
	if info.ContentID != "" {
		lines = append(lines, "Content ID: "+info.ContentID)
	}
	return strings.Join(lines, "\n"), nil
}

// FormatCliampInfo formats a CliampInfo struct as human-readable key: value lines.
func FormatCliampInfo(info CliampInfo) string {
	var lines []string
	if info.Status != "" {
		lines = append(lines, "Status: "+info.Status)
	}
	if info.Title != "" {
		lines = append(lines, "Title: "+info.Title)
	}
	if info.Artist != "" {
		lines = append(lines, "Artist: "+info.Artist)
	}
	if info.Album != "" {
		lines = append(lines, "Album: "+info.Album)
	}
	if info.URL != "" {
		lines = append(lines, "URL: "+info.URL)
	}
	return strings.Join(lines, "\n")
}

// FormatSpotifyDevices formats a Spotify devices/list response as human-readable lines or JSON.
func FormatSpotifyDevices(value any) (string, error) {
	m, ok := value.(map[string]any)
	if !ok {
		return FormatRaw(value)
	}
	devices, ok := m["devices"].([]any)
	if !ok || len(devices) == 0 {
		return "No Spotify devices found", nil
	}
	var lines []string
	for _, item := range devices {
		device, _ := item.(map[string]any)
		name := firstString(device, "name", "Unnamed")
		id := stringValue(device["id"])
		typeName := stringValue(device["type"])
		line := fmt.Sprintf("%s (%s)", name, typeName)
		if b, ok := device["is_active"].(bool); ok && b {
			line += " active"
		}
		if id != "" {
			line += " " + id
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

// FormatDiscovered formats devices found by Discover as human-readable blocks
// or as a JSON array. An empty slice is a normal "nothing found" result, not
// an error, and is rendered as such (or as "[]" in JSON mode).
func FormatDiscovered(devices []DiscoveredDevice, asJSON bool) (string, error) {
	if asJSON {
		if devices == nil {
			devices = []DiscoveredDevice{}
		}
		data, err := json.Marshal(devices)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	if len(devices) == 0 {
		return "No devices found on the local network.", nil
	}
	var blocks []string
	for _, d := range devices {
		lines := []string{"Name: " + d.Name, "Host: " + d.IP}
		if d.Model != "" {
			lines = append(lines, "Model: "+d.Model)
		}
		if d.Firmware != "" {
			lines = append(lines, "Firmware: "+d.Firmware)
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n"), nil
}

// FormatDeviceProfiles formats saved device profiles as a sorted table or JSON array.
func FormatDeviceProfiles(cfg Config, asJSON bool) (string, error) {
	if err := ValidateDeviceProfiles(cfg); err != nil {
		return "", err
	}
	profiles := ListDeviceProfiles(cfg)
	if asJSON {
		data, err := json.Marshal(profiles)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	if len(profiles) == 0 {
		return "No saved devices.", nil
	}
	lines := []string{"NAME\tHOST\tDEFAULT"}
	for _, profile := range profiles {
		marker := ""
		if profile.Default {
			marker = "*"
		}
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s", profile.Name, profile.Host, marker))
	}
	return strings.Join(lines, "\n"), nil
}

// FormatRaw formats an arbitrary value: maps and slices are pretty-printed as JSON;
// scalars are converted with fmt.Sprint.
func FormatRaw(value any) (string, error) {
	switch value.(type) {
	case map[string]any, []any:
		return jsonText(value)
	default:
		return fmt.Sprint(value), nil
	}
}

func jsonText(value any) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func intPtr(value any) *int {
	if value == nil {
		return nil
	}
	switch value := value.(type) {
	case json.Number:
		return decimalIntPtr(value.String())
	case string:
		return decimalIntPtr(value)
	case float32:
		return floatIntPtr(float64(value))
	case float64:
		return floatIntPtr(value)
	default:
		return decimalIntPtr(stringValue(value))
	}
}

func decimalIntPtr(value string) *int {
	i, err := strconv.ParseInt(value, 10, strconv.IntSize)
	if err != nil {
		return nil
	}
	result := int(i)
	return &result
}

func floatIntPtr(value float64) *int {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return nil
	}
	if strconv.IntSize == 32 {
		if value < -1<<31 || value > 1<<31-1 {
			return nil
		}
	} else if value < -1<<63 || value >= 1<<63 {
		return nil
	}
	result := int(value)
	if float64(result) != value {
		return nil
	}
	return &result
}

func bool01(value any) *bool {
	s := stringValue(value)
	if s == "1" {
		b := true
		return &b
	}
	if s == "0" {
		b := false
		return &b
	}
	return nil
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func firstString(m map[string]any, key, fallback string) string {
	if m != nil {
		if value := stringValue(m[key]); value != "" {
			return value
		}
	}
	return fallback
}

func formatWiFiFrequency(value string) string {
	if value == "" {
		return ""
	}
	freq, err := strconv.Atoi(value)
	if err != nil {
		return value
	}
	if freq >= 5925 {
		return fmt.Sprintf("6 GHz, %d MHz", freq)
	}
	if freq >= 4900 {
		return fmt.Sprintf("5 GHz, %d MHz", freq)
	}
	if freq >= 2400 {
		return fmt.Sprintf("2.4 GHz, %d MHz", freq)
	}
	return fmt.Sprintf("%d GHz", freq)
}

func cleanMetadataText(value string) string {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "unknown") || strings.EqualFold(value, "unknow") {
		return ""
	}
	return value
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
