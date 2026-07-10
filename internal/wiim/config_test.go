package wiim

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
)

func withUserHomeDirError(t *testing.T, err error) {
	t.Helper()
	old := userHomeDir
	userHomeDir = func() (string, error) { return "", err }
	t.Cleanup(func() { userHomeDir = old })
}

func TestLoadConfigReportsConfigPathError(t *testing.T) {
	homeErr := errors.New("home unavailable")
	withUserHomeDirError(t, homeErr)

	_, err := LoadConfig("")
	if err == nil {
		t.Fatal("expected config path error")
	}
	if _, ok := err.(RuntimeError); !ok {
		t.Fatalf("error type %T, want RuntimeError: %v", err, err)
	}
	if got, want := err.Error(), "wiim: could not determine config path: home unavailable"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestLoadConfigKeepsFloat64ValuesForResolution(t *testing.T) {
	path := t.TempDir() + "/config.json"
	if err := os.WriteFile(path, []byte(`{"timeout":1e100}`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Timeout != 1e100 {
		t.Fatalf("LoadConfig() timeout = %v, want 1e100", cfg.Timeout)
	}
	if _, err := resolveTimeout(0, false, cfg); err == nil {
		t.Fatal("expected resolution error")
	} else {
		requireTimeoutUsageError(t, err)
	}
}

func TestLoadConfigRejectsTimeoutFloat64Overflow(t *testing.T) {
	path := t.TempDir() + "/config.json"
	if err := os.WriteFile(path, []byte(`{"timeout":1e1000}`), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected timeout range error")
	}
	requireTimeoutUsageError(t, err)
}

func TestLoadConfigRejectsTimeoutFloat64Underflow(t *testing.T) {
	path := t.TempDir() + "/config.json"
	if err := os.WriteFile(path, []byte(`{"timeout":1e-1000}`), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected timeout range error")
	}
	requireTimeoutUsageError(t, err)
}

func TestConfigShowReportsConfigPathErrorPlain(t *testing.T) {
	withUserHomeDirError(t, errors.New("home unavailable"))

	var stdout, stderr bytes.Buffer
	err := Run([]string{"config", "show"}, &stdout, &stderr)
	if ExitCode(err) != 1 {
		t.Fatalf("exit code = %d, want 1", ExitCode(err))
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if got, want := strings.TrimSpace(stderr.String()), "wiim: could not determine config path: home unavailable"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestConfigShowReportsConfigPathErrorJSON(t *testing.T) {
	withUserHomeDirError(t, errors.New("home unavailable"))

	var stdout, stderr bytes.Buffer
	err := Run([]string{"config", "show", "--json"}, &stdout, &stderr)
	if ExitCode(err) != 1 {
		t.Fatalf("exit code = %d, want 1", ExitCode(err))
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	var envelope struct {
		Error struct {
			Kind     string `json:"kind"`
			Message  string `json:"message"`
			ExitCode int    `json:"exitCode"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\n%s", err, stderr.String())
	}
	if envelope.Error.Kind != "runtime" || envelope.Error.ExitCode != 1 {
		t.Fatalf("error envelope = %+v", envelope.Error)
	}
	if envelope.Error.Message != "could not determine config path: home unavailable" {
		t.Fatalf("error message = %q", envelope.Error.Message)
	}
}

func TestConfigPathReportsConfigPathErrorPlain(t *testing.T) {
	withUserHomeDirError(t, errors.New("home unavailable"))

	var stdout, stderr bytes.Buffer
	err := Run([]string{"config", "path"}, &stdout, &stderr)
	if _, ok := err.(RuntimeError); !ok {
		t.Fatalf("error type %T, want RuntimeError: %v", err, err)
	}
	if ExitCode(err) != 1 {
		t.Fatalf("exit code = %d, want 1", ExitCode(err))
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if got, want := strings.TrimSpace(stderr.String()), "wiim: could not determine config path: home unavailable"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestConfigPathReportsConfigPathErrorJSON(t *testing.T) {
	withUserHomeDirError(t, errors.New("home unavailable"))

	var stdout, stderr bytes.Buffer
	err := Run([]string{"config", "path", "--json"}, &stdout, &stderr)
	if _, ok := err.(RuntimeError); !ok {
		t.Fatalf("error type %T, want RuntimeError: %v", err, err)
	}
	if ExitCode(err) != 1 {
		t.Fatalf("exit code = %d, want 1", ExitCode(err))
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	var envelope struct {
		Error struct {
			Kind     string `json:"kind"`
			Message  string `json:"message"`
			ExitCode int    `json:"exitCode"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\n%s", err, stderr.String())
	}
	if envelope.Error.Kind != "runtime" || envelope.Error.ExitCode != 1 {
		t.Fatalf("error envelope = %+v", envelope.Error)
	}
	if envelope.Error.Message != "could not determine config path: home unavailable" {
		t.Fatalf("error message = %q", envelope.Error.Message)
	}
}

func requireTimeoutUsageError(t *testing.T, err error) {
	t.Helper()
	usageErr, ok := err.(UsageError)
	if !ok {
		t.Fatalf("error type %T, want UsageError: %v", err, err)
	}
	if got, want := usageErr.Msg, "timeout must be a positive number within the supported duration range"; got != want {
		t.Fatalf("error message = %q, want %q", got, want)
	}
}

func TestResolveTimeoutValidatesCLIValue(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cliTimeout float64
		cfg        Config
		want       float64
		wantErr    bool
	}{
		{name: "valid explicit value", cliTimeout: 2, cfg: Config{Timeout: 7}, want: 2},
		{name: "negative", cliTimeout: -1, wantErr: true},
		{name: "other negative", cliTimeout: -2, wantErr: true},
		{name: "zero", cliTimeout: 0, wantErr: true},
		{name: "tiny", cliTimeout: 1e-10, wantErr: true},
		{name: "NaN", cliTimeout: math.NaN(), wantErr: true},
		{name: "positive infinity", cliTimeout: math.Inf(1), wantErr: true},
		{name: "negative infinity", cliTimeout: math.Inf(-1), wantErr: true},
		{name: "too large", cliTimeout: 1e100, wantErr: true},
		{name: "duration cutoff", cliTimeout: maxTimeoutSeconds, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveTimeout(tc.cliTimeout, tc.cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				requireTimeoutUsageError(t, err)
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("ResolveTimeout() = %v, %v; want %v, nil", got, err, tc.want)
			}
		})
	}
}

func TestResolveTimeoutValidatesSelectedValue(t *testing.T) {
	largestAccepted := math.Nextafter(maxTimeoutSeconds, 0)
	cases := []struct {
		name       string
		cliTimeout float64
		cliSet     bool
		cfg        Config
		want       float64
		wantErr    bool
	}{
		{name: "default", cfg: Config{}, want: defaultTimeout},
		{name: "config", cfg: Config{Timeout: 7}, want: 7},
		{name: "explicit zero", cliSet: true, wantErr: true},
		{name: "explicit negative", cliTimeout: -1, cliSet: true, wantErr: true},
		{name: "explicit tiny", cliTimeout: 1e-10, cliSet: true, wantErr: true},
		{name: "explicit NaN", cliTimeout: math.NaN(), cliSet: true, wantErr: true},
		{name: "explicit infinity", cliTimeout: math.Inf(1), cliSet: true, wantErr: true},
		{name: "explicit huge", cliTimeout: 1e100, cliSet: true, wantErr: true},
		{name: "config negative", cfg: Config{Timeout: -1}, wantErr: true},
		{name: "config tiny", cfg: Config{Timeout: 1e-10}, wantErr: true},
		{name: "config NaN", cfg: Config{Timeout: math.NaN()}, wantErr: true},
		{name: "config infinity", cfg: Config{Timeout: math.Inf(1)}, wantErr: true},
		{name: "config huge", cfg: Config{Timeout: 1e100}, wantErr: true},
		{name: "cutoff", cliTimeout: maxTimeoutSeconds, cliSet: true, wantErr: true},
		{name: "largest accepted", cliTimeout: largestAccepted, cliSet: true, want: largestAccepted},
		{name: "CLI overrides config", cliTimeout: 2, cliSet: true, cfg: Config{Timeout: -1}, want: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveTimeout(tc.cliTimeout, tc.cliSet, tc.cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				requireTimeoutUsageError(t, err)
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("resolveTimeout() = %v, %v; want %v, nil", got, err, tc.want)
			}
		})
	}
}

func TestSaveConfigRejectsInvalidTimeout(t *testing.T) {
	for _, timeout := range []float64{-1, 1e-10, math.NaN(), math.Inf(1), 1e100, maxTimeoutSeconds} {
		t.Run("invalid", func(t *testing.T) {
			path := t.TempDir() + "/config.json"
			_, err := SaveConfig(path, Config{Timeout: timeout})
			if err == nil {
				t.Fatal("expected error")
			}
			requireTimeoutUsageError(t, err)
		})
	}
}

func TestSaveConfigAcceptsLargestSupportedTimeout(t *testing.T) {
	path := t.TempDir() + "/config.json"
	timeout := math.Nextafter(maxTimeoutSeconds, 0)
	if _, err := SaveConfig(path, Config{Timeout: timeout}); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil || cfg.Timeout != timeout {
		t.Fatalf("LoadConfig() = %#v, %v; want timeout %v", cfg, err, timeout)
	}
}

func TestSaveConfigTreatsZeroTimeoutAsDefault(t *testing.T) {
	path := t.TempDir() + "/config.json"
	if _, err := SaveConfig(path, Config{}); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil || cfg.Timeout != defaultTimeout {
		t.Fatalf("LoadConfig() = %#v, %v; want timeout %v", cfg, err, defaultTimeout)
	}
}

func TestResolveHostPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cliHost   string
		cliDevice string
		envHost   string
		cfg       Config
		want      string
		wantError string
	}{
		{
			name:    "cli host overrides every source including dangling default",
			cliHost: "cli-host",
			envHost: "env-host",
			cfg:     Config{DefaultDevice: "missing", DefaultHost: "config-host"},
			want:    "cli-host",
		},
		{
			name:    "environment overrides every configured source including dangling default",
			envHost: "env-host",
			cfg:     Config{DefaultDevice: "missing", DefaultHost: "config-host"},
			want:    "env-host",
		},
		{
			name:      "explicit device",
			cliDevice: "office",
			cfg: Config{DefaultDevice: "living-room", DefaultHost: "config-host", Devices: map[string]DeviceProfile{
				"office":      {Host: "office-host"},
				"living-room": {Host: "living-host"},
			}},
			want: "office-host",
		},
		{
			name: "configured default device",
			cfg: Config{DefaultDevice: "office", DefaultHost: "config-host", Devices: map[string]DeviceProfile{
				"office": {Host: "office-host"},
			}},
			want: "office-host",
		},
		{
			name: "default host",
			cfg:  Config{DefaultHost: "config-host"},
			want: "config-host",
		},
		{
			name:      "no host",
			wantError: "--device",
		},
		{
			name:      "explicit missing device does not fall back",
			cliDevice: "missing",
			cfg:       Config{DefaultHost: "config-host"},
			wantError: "missing",
		},
		{
			name:      "dangling configured default does not fall back",
			cfg:       Config{DefaultDevice: "missing", DefaultHost: "config-host"},
			wantError: "missing",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("WIIM_HOST", tc.envHost)
			got, err := ResolveHost(tc.cliHost, tc.cliDevice, tc.cfg)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("ResolveHost() error = %v, want substring %q", err, tc.wantError)
				}
				if _, ok := err.(UsageError); !ok {
					t.Fatalf("error type %T, want UsageError", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("ResolveHost() = %q, %v; want %q, nil", got, err, tc.want)
			}
		})
	}
}

func TestSaveConfigRejectsInvalidProfiles(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
	}{
		{name: "invalid profile name", cfg: Config{Devices: map[string]DeviceProfile{"bad/name": {Host: "valid-host"}}}},
		{name: "invalid profile host", cfg: Config{Devices: map[string]DeviceProfile{"valid": {Host: "https://bad"}}}},
		{name: "invalid default host", cfg: Config{DefaultHost: "https://bad"}},
		{name: "dangling default device", cfg: Config{DefaultDevice: "missing", Devices: map[string]DeviceProfile{"other": {Host: "other-host"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := SaveConfig(t.TempDir()+"/config.json", tc.cfg)
			if err == nil {
				t.Fatal("SaveConfig() succeeded, want UsageError")
			}
			if _, ok := err.(UsageError); !ok {
				t.Fatalf("error type %T, want UsageError: %v", err, err)
			}
		})
	}
}

func TestLoadConfigOldJSONRemainsCompatible(t *testing.T) {
	path := t.TempDir() + "/config.json"
	if err := os.WriteFile(path, []byte(`{"defaultHost":"old-host","timeout":2,"spotifyRedirectURI":"http://127.0.0.1:19872/login","maxVolume":60}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.DefaultHost != "old-host" || cfg.Timeout != 2 || cfg.MaxVolume != 60 || cfg.DefaultDevice != "" || cfg.Devices != nil {
		t.Fatalf("LoadConfig() = %#v, want old fields and no profile fields", cfg)
	}
}

func TestSaveLoadConfigProfileRoundTrip(t *testing.T) {
	want := Config{
		DefaultHost:   "legacy-host",
		DefaultDevice: "office",
		Devices: map[string]DeviceProfile{
			"office":   {Host: "office-host"},
			"upstairs": {Host: "upstairs-host"},
		},
	}
	path := t.TempDir() + "/config.json"
	if _, err := SaveConfig(path, want); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got.DefaultDevice != want.DefaultDevice || len(got.Devices) != len(want.Devices) {
		t.Fatalf("LoadConfig() = %#v, want profiles %#v", got, want)
	}
	for name, profile := range want.Devices {
		if got.Devices[name] != profile {
			t.Fatalf("profile %q = %#v, want %#v", name, got.Devices[name], profile)
		}
	}
}
