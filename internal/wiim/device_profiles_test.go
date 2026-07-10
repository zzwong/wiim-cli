package wiim

import (
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
			if err := AddDeviceProfile(&cfg, tc.name, tc.host); err == nil {
				t.Fatal("AddDeviceProfile() succeeded, want error")
			}
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
		if err := RemoveDeviceProfile(&cfg, "missing"); err == nil {
			t.Fatal("RemoveDeviceProfile() succeeded, want error")
		}
		if !reflect.DeepEqual(cfg, before) {
			t.Fatalf("config mutated on error: got %#v, want %#v", cfg, before)
		}
	})

	t.Run("active clears default only", func(t *testing.T) {
		cfg := cloneConfig(base)
		if err := RemoveDeviceProfile(&cfg, "office"); err != nil {
			t.Fatalf("RemoveDeviceProfile() error = %v", err)
		}
		if cfg.DefaultDevice != "" || cfg.DefaultHost != base.DefaultHost || cfg.Timeout != base.Timeout || cfg.MaxVolume != base.MaxVolume {
			t.Fatalf("unexpected config after removal: %#v", cfg)
		}
		if !reflect.DeepEqual(cfg.Devices, map[string]DeviceProfile{"other": {Host: "other-host"}}) {
			t.Fatalf("Devices = %#v", cfg.Devices)
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
		if err := UseDeviceProfile(&cfg, "missing"); err == nil {
			t.Fatal("UseDeviceProfile() succeeded, want error")
		}
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
