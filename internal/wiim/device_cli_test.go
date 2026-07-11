package wiim

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestDeviceCommandTreeAndEmptyListDoNotResolveHost(t *testing.T) {
	a := newApp(&bytes.Buffer{}, &bytes.Buffer{})
	for _, name := range []string{"list", "add", "remove", "use", "discover"} {
		cmd, _, err := a.root.Find([]string{"device", name})
		if err != nil || cmd == nil || cmd.Name() != name {
			t.Fatalf("device %s command = %v, %v, want command", name, cmd, err)
		}
	}

	created := false
	old := newDevice
	newDevice = func(_ string, _ float64) device { created = true; return &fakeDevice{} }
	t.Cleanup(func() { newDevice = old })

	path := t.TempDir() + "/config.json"
	code, out, errText := runTest("--config", path, "device", "list")
	if code != 0 || strings.TrimSpace(out) != "No saved devices." || errText != "" {
		t.Fatalf("human list: code %d out %q err %q", code, out, errText)
	}
	code, out, errText = runTest("--config", path, "device", "list", "--json")
	if code != 0 || strings.TrimSpace(out) != "[]" || errText != "" {
		t.Fatalf("JSON list: code %d out %q err %q", code, out, errText)
	}
	if created {
		t.Fatal("device list created a client")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("device list created config: %v", err)
	}
}

func TestDeviceCRUDHumanJSONErrorsAndPreservation(t *testing.T) {
	// Device mutations must persist even when an unrelated environment override
	// is invalid; SaveConfig validates the persisted URI independently.
	t.Setenv("WIIM_SPOTIFY_REDIRECT_URI", "https://example.com/invalid")
	path := t.TempDir() + "/config.json"
	const initial = `{"defaultHost":"legacy-host","timeout":7,"spotifyRedirectURI":"http://127.0.0.1:19999/callback","maxVolume":77,"defaultDevice":"kitchen","devices":{"kitchen":{"host":"kitchen-host"}}}`
	if err := os.WriteFile(path, []byte(initial), 0600); err != nil {
		t.Fatal(err)
	}

	code, out, errText := runTest("--config", path, "device", "add", "office", "office-host")
	if code != 0 || strings.TrimSpace(out) != `Added device "office" (office-host)` || errText != "" {
		t.Fatalf("human add: code %d out %q err %q", code, out, errText)
	}

	code, out, errText = runTest("--config", path, "device", "add", "den", "den-host", "--json")
	if code != 0 || errText != "" {
		t.Fatalf("JSON add: code %d out %q err %q", code, out, errText)
	}
	var added map[string]any
	if err := json.Unmarshal([]byte(out), &added); err != nil {
		t.Fatalf("JSON add output: %v: %q", err, out)
	}
	if len(added) != 3 || added["name"] != "den" || added["host"] != "den-host" || added["default"] != false {
		t.Fatalf("JSON add = %#v, want exactly name, host, default:false", added)
	}

	code, out, errText = runTest("--config", path, "device", "list")
	if code != 0 || errText != "" || !strings.Contains(out, "NAME\tHOST\tDEFAULT") || !strings.Contains(out, "kitchen\tkitchen-host\t*") {
		t.Fatalf("human list: code %d out %q err %q", code, out, errText)
	}
	code, out, errText = runTest("--config", path, "device", "list", "--json")
	if code != 0 || errText != "" {
		t.Fatalf("JSON list: code %d out %q err %q", code, out, errText)
	}
	var listed []DeviceProfileView
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatalf("JSON list output: %v: %q", err, out)
	}
	if len(listed) != 3 || listed[0].Name != "den" || listed[1].Name != "kitchen" || !listed[1].Default {
		t.Fatalf("JSON list = %#v", listed)
	}

	code, out, errText = runTest("--config", path, "device", "use", "office")
	if code != 0 || strings.TrimSpace(out) != "Default device: office" || errText != "" {
		t.Fatalf("human use: code %d out %q err %q", code, out, errText)
	}
	code, out, errText = runTest("--config", path, "device", "use", "den", "--json")
	if code != 0 || errText != "" || strings.TrimSpace(out) != "{\n  \"defaultDevice\": \"den\"\n}" {
		t.Fatalf("JSON use: code %d out %q err %q", code, out, errText)
	}

	code, out, errText = runTest("--config", path, "device", "remove", "office")
	if code != 0 || strings.TrimSpace(out) != `Removed device "office"` || errText != "" {
		t.Fatalf("human remove: code %d out %q err %q", code, out, errText)
	}
	code, out, errText = runTest("--config", path, "device", "remove", "den", "--json")
	if code != 0 || errText != "" || strings.TrimSpace(out) != "{\n  \"removed\": \"den\"\n}" {
		t.Fatalf("JSON remove: code %d out %q err %q", code, out, errText)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultHost != "legacy-host" || cfg.Timeout != 7 || cfg.SpotifyRedirectURI != "http://127.0.0.1:19999/callback" || cfg.MaxVolume != 77 || cfg.DefaultDevice != "" || len(cfg.Devices) != 1 || cfg.Devices["kitchen"].Host != "kitchen-host" {
		t.Fatalf("CRUD did not preserve unrelated config: %#v", cfg)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"device", "add", "kitchen", "duplicate-host"},
		{"device", "use", "missing"},
		{"device", "remove", "missing"},
	} {
		invocation := append([]string{"--config", path}, args...)
		code, _, errText = runTest(invocation...)
		if code != 2 || !strings.Contains(errText, "device profile") {
			t.Fatalf("%q: code %d err %q", invocation, code, errText)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(after, before) {
			t.Fatalf("%q mutated config on error:\n%s", invocation, after)
		}
	}
}

func TestDeviceSelectionAndVolumeDeviceFlags(t *testing.T) {
	t.Setenv("WIIM_HOST", "")
	fd, done := withFake(t)
	defer done()

	path := t.TempDir() + "/config.json"
	if err := os.WriteFile(path, []byte(`{"defaultHost":"fallback-host","timeout":4,"maxVolume":55,"devices":{"office":{"host":"office-host"}}}`), 0600); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"--config", path, "--device", "office", "status"},
		{"--config", path, "status", "--device=office"},
		{"--config", path, "--device=office", "volume"},
		{"--config", path, "volume", "--device", "office"},
		{"--config", path, "volume", "30", "--device", "office"},
		{"--config", path, "volume", "30", "--device=office"},
	} {
		code, _, errText := runTest(args...)
		if code != 0 || errText != "" || fd.host != "office-host" {
			t.Fatalf("%q: code %d err %q host %q", args, code, errText, fd.host)
		}
	}

	t.Setenv("WIIM_HOST", "env-host")
	code, _, errText := runTest("--config", path, "--device", "office", "status")
	if code != 0 || errText != "" || fd.host != "env-host" {
		t.Fatalf("environment precedence: code %d err %q host %q", code, errText, fd.host)
	}
	code, _, errText = runTest("--config", path, "--device", "office", "--host", "cli-host", "status")
	if code != 0 || errText != "" || fd.host != "cli-host" {
		t.Fatalf("host precedence: code %d err %q host %q", code, errText, fd.host)
	}
	t.Setenv("WIIM_HOST", "")

	for _, args := range [][]string{
		{"--config", path, "--device=", "status"},
		{"--config", path, "status", "--device="},
		{"--config", path, "status", "--device"},
	} {
		code, _, errText := runTest(args...)
		if code != 2 || errText != "wiim: flag --device requires a value" {
			t.Fatalf("%q: code %d err %q", args, code, errText)
		}
	}

	code, _, errText = runTest("--config", path, "volume", "--", "--device=office")
	if code != 2 || !strings.Contains(errText, "invalid relative volume") || fd.host != "fallback-host" {
		t.Fatalf("terminator: code %d err %q host %q", code, errText, fd.host)
	}
	for _, args := range [][]string{
		{"--config", path, "volume", "--device"},
		{"--config", path, "volume", "--device="},
		{"--config", path, "volume", "--device", "--"},
	} {
		code, _, errText = runTest(args...)
		if code != 2 || errText != "wiim: flag --device requires a value" {
			t.Fatalf("%q: code %d err %q", args, code, errText)
		}
	}
}

func TestVolumeDeviceEmptySeparateValueFailsWithoutSideEffects(t *testing.T) {
	old := newDevice
	created := 0
	newDevice = func(_ string, _ float64) device {
		created++
		return &fakeDevice{}
	}
	t.Cleanup(func() { newDevice = old })

	for _, tc := range []struct {
		name string
		args []string
		json bool
	}{
		{name: "plain", args: []string{"--host", "test-host", "volume", "--device", ""}},
		{name: "json", args: []string{"--host", "test-host", "volume", "--device", "", "--json"}, json: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(tc.args, &stdout, &stderr)
			usageErr, ok := err.(UsageError)
			if !ok || usageErr.Msg != "flag --device requires a value" {
				t.Fatalf("Run(%q) error = %T %v, want UsageError", tc.args, err, err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("Run(%q) stdout = %q, want empty", tc.args, stdout.String())
			}
			if tc.json {
				var envelope struct {
					Error errorDetail `json:"error"`
				}
				if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
					t.Fatalf("Run(%q) JSON error = %v: %q", tc.args, err, stderr.String())
				}
				if envelope.Error.Kind != "usage" || envelope.Error.Message != usageErr.Msg || envelope.Error.ExitCode != 2 {
					t.Fatalf("Run(%q) JSON error = %#v", tc.args, envelope.Error)
				}
			} else if stderr.String() != "wiim: flag --device requires a value\n" {
				t.Fatalf("Run(%q) stderr = %q", tc.args, stderr.String())
			}
			if created != 0 {
				t.Fatalf("Run(%q) created %d device clients", tc.args, created)
			}
		})
	}
}

func TestDeviceDiscoverAliasDoesNotMutateConfig(t *testing.T) {
	t.Setenv("WIIM_HOST", "")
	done := withFakeDiscovery(t, []string{"10.0.0.1"}, map[string]*fakeDiscoveryDevice{
		"10.0.0.1": {statusEx: map[string]any{"project": "WiiM_Ultra"}, cast: map[string]any{"name": "WiiM Ultra"}},
	})
	defer done()

	path := t.TempDir() + "/config.json"
	const config = `{"defaultHost":"legacy-host","timeout":2,"maxVolume":70,"defaultDevice":"office","devices":{"office":{"host":"office-host"}}}`
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	code, rootOut, errText := runTest("--config", path, "discover")
	if code != 0 || errText != "" {
		t.Fatalf("root discover: code %d out %q err %q", code, rootOut, errText)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	code, aliasOut, errText := runTest("--config", path, "device", "discover")
	if code != 0 || errText != "" || aliasOut != rootOut {
		t.Fatalf("device discover: code %d out %q root %q err %q", code, aliasOut, rootOut, errText)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("device discover mutated config:\n%s", after)
	}
}

func TestDeviceAndConfigMutationsPreserveFutureSettings(t *testing.T) {
	path := t.TempDir() + "/config.json"
	initial := `{"defaultHost":"legacy-host","timeout":4,"spotifyRedirectURI":"http://127.0.0.1:19999/callback","maxVolume":60,"defaultDevice":"kitchen","devices":{"kitchen":{"host":"kitchen-host"}},"futureSetting":{"nested":[1,2],"enabled":true}}`
	if err := os.WriteFile(path, []byte(initial), 0600); err != nil {
		t.Fatal(err)
	}

	assertFutureSetting := func() map[string]json.RawMessage {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatalf("config JSON: %v", err)
		}
		var future struct {
			Enabled bool  `json:"enabled"`
			Nested  []int `json:"nested"`
		}
		if err := json.Unmarshal(fields["futureSetting"], &future); err != nil || !future.Enabled || len(future.Nested) != 2 {
			t.Fatalf("futureSetting = %s, error = %v", fields["futureSetting"], err)
		}
		return fields
	}
	for _, args := range [][]string{
		{"device", "add", "office", "office-host"},
		{"device", "use", "office"},
		{"config", "set", "maxVolume", "70"},
		{"config", "unset", "defaultDevice"},
		{"device", "remove", "office"},
	} {
		code, _, errText := runTest(append([]string{"--config", path}, args...)...)
		if code != 0 || errText != "" {
			t.Fatalf("%q: code %d err %q", args, code, errText)
		}
		fields := assertFutureSetting()
		if args[0] == "config" && args[2] == "defaultDevice" {
			if _, ok := fields["defaultDevice"]; ok {
				t.Fatalf("unset defaultDevice retained key: %s", fields["defaultDevice"])
			}
		}
	}
	code, _, errText := runTest("--config", path, "device", "remove", "kitchen")
	if code != 0 || errText != "" {
		t.Fatalf("remove final profile: code %d err %q", code, errText)
	}
	fields := assertFutureSetting()
	if _, ok := fields["devices"]; ok {
		t.Fatalf("empty devices key was retained: %s", fields["devices"])
	}
}

func TestDeviceListRejectsInvalidManualProfilesWithoutControlOutput(t *testing.T) {
	for _, tc := range []struct {
		name    string
		profile string
	}{
		{name: "invalid name", profile: `{"bad\u001bname":{"host":"valid-host"}}`},
		{name: "invalid host", profile: `{"valid-name":{"host":"bad\u001bhost"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := t.TempDir() + "/config.json"
			if err := os.WriteFile(path, []byte(`{"devices":`+tc.profile+`}`), 0600); err != nil {
				t.Fatal(err)
			}
			for _, asJSON := range []bool{false, true} {
				args := []string{"--config", path, "device", "list"}
				if asJSON {
					args = append(args, "--json")
				}
				var stdout, stderr bytes.Buffer
				err := Run(args, &stdout, &stderr)
				if _, ok := err.(UsageError); !ok {
					t.Fatalf("Run(%q) error = %T %v, want UsageError", args, err, err)
				}
				if stdout.Len() != 0 || bytes.Contains(stderr.Bytes(), []byte{'\x1b'}) {
					t.Fatalf("Run(%q) emitted unsafe output: stdout %q stderr %q", args, stdout.String(), stderr.String())
				}
				if asJSON {
					var envelope struct {
						Error errorDetail `json:"error"`
					}
					if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil || envelope.Error.Kind != "usage" || envelope.Error.ExitCode != 2 {
						t.Fatalf("Run(%q) JSON error = %q, parse error = %v", args, stderr.String(), err)
					}
				}
			}
		})
	}
}

func TestDeviceFlagRejectedForNonTargetCommandsBeforeSideEffects(t *testing.T) {
	path := t.TempDir() + "/config.json"
	initial := []byte(`{"futureSetting":{"preserve":true}}`)
	if err := os.WriteFile(path, initial, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		args   []string
		target string
	}{
		{name: "setup", args: []string{"setup", "--host", "test-host"}, target: "setup"},
		{name: "config", args: []string{"config", "set", "defaultHost", "test-host"}, target: "config"},
		{name: "device list", args: []string{"device", "list"}, target: "device"},
		{name: "device add", args: []string{"device", "add", "office", "test-host"}, target: "device"},
		{name: "device remove", args: []string{"device", "remove", "office"}, target: "device"},
		{name: "device use", args: []string{"device", "use", "office"}, target: "device"},
		{name: "spotify", args: []string{"spotify", "credentials", "status"}, target: "spotify"},
		{name: "version", args: []string{"version"}, target: "version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, asJSON := range []bool{false, true} {
				args := append([]string{"--config", path, "--device", "office"}, tc.args...)
				if asJSON {
					args = append(args, "--json")
				}
				var stdout, stderr bytes.Buffer
				err := Run(args, &stdout, &stderr)
				usageErr, ok := err.(UsageError)
				wantMessage := "flag --device is not valid with " + tc.target
				if !ok || usageErr.Msg != wantMessage {
					t.Fatalf("Run(%q) error = %T %v, want UsageError %q", args, err, err, wantMessage)
				}
				if stdout.Len() != 0 {
					t.Fatalf("Run(%q) stdout = %q, want empty", args, stdout.String())
				}
				if asJSON {
					var envelope struct {
						Error errorDetail `json:"error"`
					}
					if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil || envelope.Error.Kind != "usage" || envelope.Error.Message != wantMessage || envelope.Error.ExitCode != 2 {
						t.Fatalf("Run(%q) JSON error = %q, parse error = %v", args, stderr.String(), err)
					}
				} else if stderr.String() != "wiim: "+wantMessage+"\n" {
					t.Fatalf("Run(%q) stderr = %q", args, stderr.String())
				}
				got, readErr := os.ReadFile(path)
				if readErr != nil || !bytes.Equal(got, initial) {
					t.Fatalf("Run(%q) changed config: got %q read error %v", args, got, readErr)
				}
			}
		})
	}
}

func TestConfigSetAndUnsetDefaultDeviceAfterLegacySetup(t *testing.T) {
	t.Setenv("WIIM_SPOTIFY_REDIRECT_URI", "")
	path := t.TempDir() + "/config.json"
	code, _, errText := runTest("--config", path, "setup", "--host", "legacy-host")
	if code != 0 || errText != "" {
		t.Fatalf("legacy setup: code %d err %q", code, errText)
	}
	cfg, err := LoadConfig(path)
	if err != nil || cfg.DefaultHost != "legacy-host" || cfg.DefaultDevice != "" || cfg.Devices != nil {
		t.Fatalf("legacy setup config = %#v, %v", cfg, err)
	}

	code, _, errText = runTest("--config", path, "device", "add", "office", "office-host")
	if code != 0 || errText != "" {
		t.Fatalf("add profile: code %d err %q", code, errText)
	}
	code, _, errText = runTest("--config", path, "config", "set", "defaultDevice", "office")
	if code != 0 || errText != "" {
		t.Fatalf("set defaultDevice: code %d err %q", code, errText)
	}
	cfg, err = LoadConfig(path)
	if err != nil || cfg.DefaultDevice != "office" {
		t.Fatalf("set config = %#v, %v", cfg, err)
	}

	code, _, errText = runTest("--config", path, "config", "unset", "defaultDevice")
	if code != 0 || errText != "" {
		t.Fatalf("unset defaultDevice: code %d err %q", code, errText)
	}
	cfg, err = LoadConfig(path)
	if err != nil || cfg.DefaultDevice != "" {
		t.Fatalf("unset config = %#v, %v", cfg, err)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	code, _, errText = runTest("--config", path, "config", "set", "defaultDevice", "missing")
	if code != 2 || !strings.Contains(errText, `device profile "missing" is not configured`) {
		t.Fatalf("set missing defaultDevice: code %d err %q", code, errText)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("failed config set mutated config:\n%s", after)
	}

	code, out, errText := runTest("config", "set", "--help")
	if code != 0 || errText != "" || !strings.Contains(out, "defaultDevice") {
		t.Fatalf("config set help: code %d out %q err %q", code, out, errText)
	}
	code, out, errText = runTest("config", "unset", "--help")
	if code != 0 || errText != "" || !strings.Contains(out, "defaultDevice") {
		t.Fatalf("config unset help: code %d out %q err %q", code, out, errText)
	}
}
