package wiim

import "strings"

// InputStatus describes the active audio input on the device.
type InputStatus struct {
	Input string `json:"input,omitempty"`
	Mode  string `json:"mode,omitempty"`
}

var inputAliases = map[string]string{
	"wifi":      "wifi",
	"network":   "wifi",
	"wi-fi":     "wifi",
	"bluetooth": "bluetooth",
	"bt":        "bluetooth",
	"line":      "line-in",
	"line-in":   "line-in",
	"aux":       "line-in",
	"optical":   "optical",
	"spdif":     "optical",
	"coax":      "coaxial",
	"coaxial":   "coaxial",
	"hdmi":      "hdmi",
	"hdmi-arc":  "hdmi",
	"arc":       "hdmi",
	"phono":     "phono",
	"usb":       "usb",
}

var playerModeInputs = map[string]string{
	"10": "wifi",
	"11": "airplay",
	"16": "spotify",
	"31": "bluetooth",
	"40": "line-in",
	"41": "bluetooth",
	"43": "optical",
	"47": "coaxial",
	"49": "hdmi",
	"51": "phono",
	"52": "usb",
}

// NormalizeInputName converts a user-supplied input name (e.g. "aux", "bt") to a
// canonical form ("line-in", "bluetooth"). Returns a UsageError for unknown inputs.
func NormalizeInputName(value string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(value))
	if normalized, ok := inputAliases[key]; ok {
		return normalized, nil
	}
	return "", usagef("unsupported input %q; supported inputs: wifi, bluetooth, line-in, optical, coaxial, hdmi, phono, usb", value)
}

// InputFromPlayer extracts the current input information from a getPlayerStatus map.
func InputFromPlayer(player map[string]any) InputStatus {
	mode := stringValue(player["mode"])
	return InputStatus{Input: playerModeInputs[mode], Mode: mode}
}

// FormatInputStatus formats an InputStatus as human-readable text or JSON.
func FormatInputStatus(status InputStatus, asJSON bool) (string, error) {
	if asJSON {
		return jsonText(status)
	}
	if status.Input != "" {
		return status.Input, nil
	}
	if status.Mode != "" {
		return "unknown (mode " + status.Mode + ")", nil
	}
	return "unknown", nil
}
