package wiim

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestListDeviceProfilesSortsAndMarksDefault(t *testing.T) {
	cfg := Config{
		DefaultDevice: "office",
		Devices: map[string]DeviceProfile{
			"zulu":   {Host: "zulu-host"},
			"office": {Host: "office-host"},
			"alpha":  {Host: "alpha-host"},
		},
	}

	got := ListDeviceProfiles(cfg)
	want := []DeviceProfileView{
		{Name: "alpha", Host: "alpha-host"},
		{Name: "office", Host: "office-host", Default: true},
		{Name: "zulu", Host: "zulu-host"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListDeviceProfiles() = %#v, want %#v", got, want)
	}
}

func TestListDeviceProfilesEmptyIsNonNil(t *testing.T) {
	profiles := ListDeviceProfiles(Config{})
	if profiles == nil || len(profiles) != 0 {
		t.Fatalf("ListDeviceProfiles() = %#v, want non-nil empty slice", profiles)
	}
}

func TestAddDeviceProfileValidatesBeforeMutating(t *testing.T) {
	base := Config{
		DefaultHost:        "default-host",
		Timeout:            2.5,
		SpotifyRedirectURI: "http://127.0.0.1:19872/login",
		MaxVolume:          70,
		DefaultDevice:      "existing",
		Devices:            map[string]DeviceProfile{"existing": {Host: "existing-host"}},
	}

	for _, tc := range []struct {
		name string
		host string
	}{
		{name: "bad/name", host: "valid-host"},
		{name: "new", host: "https://bad"},
		{name: "existing", host: "new-host"},
	} {
		t.Run(tc.name+"/"+tc.host, func(t *testing.T) {
			cfg := cloneConfig(base)
			before := cloneConfig(cfg)
			err := AddDeviceProfile(&cfg, tc.name, tc.host)
			requireDeviceProfileUsageError(t, err)
			if !reflect.DeepEqual(cfg, before) {
				t.Fatalf("config mutated on error: got %#v, want %#v", cfg, before)
			}
		})
	}
}

func TestAddDeviceProfileInitializesMap(t *testing.T) {
	cfg := Config{DefaultHost: "default-host", Timeout: 2, MaxVolume: 60}
	if err := AddDeviceProfile(&cfg, "living-room", "living-host"); err != nil {
		t.Fatalf("AddDeviceProfile() error = %v", err)
	}
	want := map[string]DeviceProfile{"living-room": {Host: "living-host"}}
	if !reflect.DeepEqual(cfg.Devices, want) {
		t.Fatalf("Devices = %#v, want %#v", cfg.Devices, want)
	}
}

func TestAddDeviceProfileDoesNotMutateShallowSnapshot(t *testing.T) {
	cfg := Config{
		DefaultDevice: "office",
		Devices: map[string]DeviceProfile{
			"office": {Host: "office-host"},
		},
	}
	snapshot := cfg
	if err := AddDeviceProfile(&cfg, "living-room", "living-host"); err != nil {
		t.Fatalf("AddDeviceProfile() error = %v", err)
	}
	if snapshot.DefaultDevice != cfg.DefaultDevice {
		t.Fatalf("snapshot default = %q, cfg default = %q", snapshot.DefaultDevice, cfg.DefaultDevice)
	}
	if len(snapshot.Devices) != 1 || snapshot.Devices["office"].Host != "office-host" {
		t.Fatalf("snapshot devices = %#v, want only office profile", snapshot.Devices)
	}
	if _, ok := snapshot.Devices["living-room"]; ok {
		t.Fatal("shallow snapshot observed added profile")
	}
	if cfg.Devices["living-room"].Host != "living-host" {
		t.Fatalf("cfg devices = %#v, want added profile", cfg.Devices)
	}
}

func TestRemoveDeviceProfile(t *testing.T) {
	base := Config{
		DefaultHost:   "default-host",
		Timeout:       2,
		MaxVolume:     60,
		DefaultDevice: "office",
		Devices: map[string]DeviceProfile{
			"office": {Host: "office-host"},
			"other":  {Host: "other-host"},
		},
	}

	t.Run("unknown does not mutate", func(t *testing.T) {
		cfg := cloneConfig(base)
		before := cloneConfig(cfg)
		err := RemoveDeviceProfile(&cfg, "missing")
		requireDeviceProfileUsageError(t, err)
		if !reflect.DeepEqual(cfg, before) {
			t.Fatalf("config mutated on error: got %#v, want %#v", cfg, before)
		}
	})

	t.Run("active clears default only", func(t *testing.T) {
		cfg := cloneConfig(base)
		snapshot := cfg
		if err := RemoveDeviceProfile(&cfg, "office"); err != nil {
			t.Fatalf("RemoveDeviceProfile() error = %v", err)
		}
		if cfg.DefaultDevice != "" || cfg.DefaultHost != base.DefaultHost || cfg.Timeout != base.Timeout || cfg.MaxVolume != base.MaxVolume {
			t.Fatalf("unexpected config after removal: %#v", cfg)
		}
		if !reflect.DeepEqual(cfg.Devices, map[string]DeviceProfile{"other": {Host: "other-host"}}) {
			t.Fatalf("Devices = %#v", cfg.Devices)
		}
		if snapshot.DefaultDevice != "office" || snapshot.Devices["office"].Host != "office-host" {
			t.Fatalf("shallow snapshot changed: %#v", snapshot)
		}
	})

	t.Run("inactive leaves default", func(t *testing.T) {
		cfg := cloneConfig(base)
		if err := RemoveDeviceProfile(&cfg, "other"); err != nil {
			t.Fatalf("RemoveDeviceProfile() error = %v", err)
		}
		if cfg.DefaultDevice != "office" || cfg.DefaultHost != base.DefaultHost {
			t.Fatalf("unexpected config after removal: %#v", cfg)
		}
	})
}

func TestUseDeviceProfile(t *testing.T) {
	base := Config{
		DefaultHost:   "default-host",
		DefaultDevice: "office",
		Devices: map[string]DeviceProfile{
			"office": {Host: "office-host"},
			"other":  {Host: "other-host"},
		},
	}

	t.Run("unknown does not mutate", func(t *testing.T) {
		cfg := cloneConfig(base)
		before := cloneConfig(cfg)
		err := UseDeviceProfile(&cfg, "missing")
		requireDeviceProfileUsageError(t, err)
		if !reflect.DeepEqual(cfg, before) {
			t.Fatalf("config mutated on error: got %#v, want %#v", cfg, before)
		}
	})

	t.Run("sets default", func(t *testing.T) {
		cfg := cloneConfig(base)
		if err := UseDeviceProfile(&cfg, "other"); err != nil {
			t.Fatalf("UseDeviceProfile() error = %v", err)
		}
		if cfg.DefaultDevice != "other" || cfg.DefaultHost != base.DefaultHost {
			t.Fatalf("unexpected config after use: %#v", cfg)
		}
	})
}

func TestDeviceProfileHelpersPreserveConfigOnRoundTrip(t *testing.T) {
	cfg := Config{
		DefaultHost:        "legacy-host",
		Timeout:            4.25,
		SpotifyRedirectURI: "http://127.0.0.1:19999/callback",
		MaxVolume:          83,
	}
	if err := AddDeviceProfile(&cfg, "office", "office-host"); err != nil {
		t.Fatal(err)
	}
	if err := UseDeviceProfile(&cfg, "office"); err != nil {
		t.Fatal(err)
	}

	path := t.TempDir() + "/config.json"
	if _, err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if loaded.DefaultHost != cfg.DefaultHost || loaded.Timeout != cfg.Timeout || loaded.SpotifyRedirectURI != cfg.SpotifyRedirectURI || loaded.MaxVolume != cfg.MaxVolume || loaded.DefaultDevice != "office" {
		t.Fatalf("unrelated config changed after round trip: got %#v", loaded)
	}
	if !reflect.DeepEqual(loaded.Devices, cfg.Devices) {
		t.Fatalf("Devices after round trip = %#v, want %#v", loaded.Devices, cfg.Devices)
	}
}

func requireDeviceProfileUsageError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("helper succeeded, want UsageError")
	}
	if _, ok := err.(UsageError); !ok {
		t.Fatalf("error type %T, want UsageError: %v", err, err)
	}
	if code := ExitCode(err); code != 2 {
		t.Fatalf("ExitCode = %d, want 2", code)
	}
	var envelope struct {
		Error struct {
			Kind     string `json:"kind"`
			ExitCode int    `json:"exitCode"`
		} `json:"error"`
	}
	if jsonErr := json.Unmarshal([]byte(FormatError(err, true)), &envelope); jsonErr != nil {
		t.Fatalf("JSON error envelope is invalid: %v", jsonErr)
	}
	if envelope.Error.Kind != "usage" || envelope.Error.ExitCode != 2 {
		t.Fatalf("JSON error envelope = %+v, want usage/2", envelope.Error)
	}
}

func cloneConfig(cfg Config) Config {
	clone := cfg
	if cfg.Devices != nil {
		clone.Devices = make(map[string]DeviceProfile, len(cfg.Devices))
		for name, profile := range cfg.Devices {
			clone.Devices[name] = profile
		}
	}
	return clone
}
