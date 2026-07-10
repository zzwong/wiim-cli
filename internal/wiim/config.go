package wiim

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"
)

const (
	defaultSpotifyRedirectURI = "http://127.0.0.1:19872/login"
	defaultMaxVolume          = 55
	timeoutRangeErrorMessage  = "timeout must be a positive number within the supported duration range"
)

var (
	userHomeDir        = os.UserHomeDir
	beforeAtomicRename = func(string) error { return nil }
)

// DeviceProfile holds the connection settings for a named WiiM device.
type DeviceProfile struct {
	Host string `json:"host"`
}

// Config holds persistent settings for connecting to a WiiM device.
type Config struct {
	DefaultHost        string                   `json:"defaultHost"`
	Timeout            float64                  `json:"timeout"`
	SpotifyRedirectURI string                   `json:"spotifyRedirectURI"`
	MaxVolume          int                      `json:"maxVolume"`
	DefaultDevice      string                   `json:"defaultDevice,omitempty"`
	Devices            map[string]DeviceProfile `json:"devices,omitempty"`
}

// ConfigPath returns the config file path. If path is non-empty it is used as-is;
// otherwise the default path ~/.config/wiim-cli/config.json is returned.
func ConfigPath(path string) (string, error) {
	if path != "" {
		return path, nil
	}
	home, err := userHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "wiim-cli", "config.json"), nil
}

// LoadConfig reads and unmarshals the config file. A missing file or empty path
// returns a zero-value Config with no error.
func LoadConfig(path string) (Config, error) {
	path, pathErr := ConfigPath(path)
	if pathErr != nil {
		return Config{}, runtimef("could not determine config path: %v", pathErr)
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, usagef("could not read config %s: %v", path, err)
	}
	if configTimeoutOverflowsFloat64(data) {
		return Config{}, timeoutRangeError()
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, usagef("invalid config JSON in %s: %v", path, err)
	}
	return cfg, nil
}

func configTimeoutOverflowsFloat64(data []byte) bool {
	var raw struct {
		Timeout json.RawMessage `json:"timeout"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || len(raw.Timeout) == 0 {
		return false
	}

	decoder := json.NewDecoder(bytes.NewReader(raw.Timeout))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return false
	}
	number, ok := value.(json.Number)
	if !ok {
		return false
	}
	value64, err := strconv.ParseFloat(number.String(), 64)
	if errors.Is(err, strconv.ErrRange) {
		return true
	}
	return err == nil && value64 == 0 && jsonNumberHasNonzeroMantissa(number.String())
}

func jsonNumberHasNonzeroMantissa(value string) bool {
	for i := 0; i < len(value); i++ {
		switch {
		case value[i] == 'e' || value[i] == 'E':
			return false
		case value[i] >= '1' && value[i] <= '9':
			return true
		}
	}
	return false
}

var (
	hostPattern       = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	deviceNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// ValidateDeviceName validates a name used as a key in Config.Devices.
func ValidateDeviceName(name string) error {
	if name == "." || name == ".." || !deviceNamePattern.MatchString(name) {
		return usagef("device name must contain only letters, numbers, '.', '_', or '-'")
	}
	return nil
}

// ValidateHost validates a WiiM hostname or IP address without consulting
// process state such as environment variables or configuration.
func ValidateHost(host string) error {
	if !hostPattern.MatchString(host) {
		return usagef("host must be a hostname or IP address, not a URL")
	}
	return nil
}

// ResolveHost returns the WiiM hostname from (in order of precedence) the CLI
// host, WIIM_HOST, an explicitly selected device profile, the configured
// default device profile, or defaultHost. Returns a UsageError if no source
// is set or a selected profile is missing or malformed.
func ResolveHost(cliHost, cliDevice string, cfg Config) (string, error) {
	if cliHost != "" {
		if err := ValidateHost(cliHost); err != nil {
			return "", err
		}
		return cliHost, nil
	}
	if host := os.Getenv("WIIM_HOST"); host != "" {
		if err := ValidateHost(host); err != nil {
			return "", err
		}
		return host, nil
	}

	selectedDevice := cliDevice
	if selectedDevice == "" {
		selectedDevice = cfg.DefaultDevice
	}
	if selectedDevice != "" {
		if err := ValidateDeviceName(selectedDevice); err != nil {
			return "", err
		}
		profile, ok := cfg.Devices[selectedDevice]
		if !ok {
			return "", usagef("device profile %q is not configured", selectedDevice)
		}
		if err := ValidateHost(profile.Host); err != nil {
			return "", err
		}
		return profile.Host, nil
	}
	if cfg.DefaultHost != "" {
		if err := ValidateHost(cfg.DefaultHost); err != nil {
			return "", err
		}
		return cfg.DefaultHost, nil
	}
	return "", usagef("host is required; pass --host, set WIIM_HOST, use --device with one of the configured profiles, or configure defaultHost/defaultDevice")
}

// ResolveSpotifyRedirectURI returns the Spotify OAuth redirect URI from (in
// order of precedence) WIIM_SPOTIFY_REDIRECT_URI, the config file, or the
// default http://127.0.0.1:19872/login. Validates it is a loopback HTTP URL.
func ResolveSpotifyRedirectURI(cfg Config) (string, error) {
	redirectURI := os.Getenv("WIIM_SPOTIFY_REDIRECT_URI")
	if redirectURI == "" {
		redirectURI = cfg.SpotifyRedirectURI
	}
	if redirectURI == "" {
		redirectURI = defaultSpotifyRedirectURI
	}
	if !regexp.MustCompile(`^http://127\.0\.0\.1:[0-9]+/[A-Za-z0-9._~/-]+$`).MatchString(redirectURI) {
		return "", usagef("spotifyRedirectURI must be a loopback http URL like http://127.0.0.1:19872/login")
	}
	return redirectURI, nil
}

// ResolveMaxVolume returns the maximum allowed volume (1–100), defaulting to 55
// if the config value is zero.
func ResolveMaxVolume(cfg Config) (int, error) {
	if cfg.MaxVolume == 0 {
		return defaultMaxVolume, nil
	}
	if cfg.MaxVolume < 1 || cfg.MaxVolume > 100 {
		return 0, usagef("maxVolume must be between 1 and 100")
	}
	return cfg.MaxVolume, nil
}

// WriteInitialConfig validates and saves a config with a required defaultHost.
// Returns the path to which the config was written.
func WriteInitialConfig(path string, cfg Config) (string, error) {
	if cfg.DefaultHost == "" {
		return "", usagef("setup requires a host; pass --host or set WIIM_HOST")
	}
	return SaveConfig(path, cfg)
}

// SaveConfig marshals cfg to JSON and writes it to the config file, applying
// defaults for zero-valued fields (timeout=3.0, maxVolume=55, spotifyRedirectURI set).
// Returns the path of the written file.
func SaveConfig(path string, cfg Config) (string, error) {
	path, err := ConfigPath(path)
	if err != nil {
		return "", err
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 3.0
	} else if err := validateTimeout(cfg.Timeout); err != nil {
		return "", err
	}
	if cfg.MaxVolume == 0 {
		cfg.MaxVolume = defaultMaxVolume
	}
	if cfg.SpotifyRedirectURI == "" {
		cfg.SpotifyRedirectURI = defaultSpotifyRedirectURI
	}
	if cfg.DefaultHost != "" {
		if err := ValidateHost(cfg.DefaultHost); err != nil {
			return "", err
		}
	}
	profileNames := make([]string, 0, len(cfg.Devices))
	for name := range cfg.Devices {
		profileNames = append(profileNames, name)
	}
	sort.Strings(profileNames)
	for _, name := range profileNames {
		profile := cfg.Devices[name]
		if err := ValidateDeviceName(name); err != nil {
			return "", err
		}
		if err := ValidateHost(profile.Host); err != nil {
			if usageErr, ok := err.(UsageError); ok {
				return "", usagef("device profile %q: %s", name, usageErr.Msg)
			}
			return "", err
		}
	}
	if cfg.DefaultDevice != "" {
		if _, ok := cfg.Devices[cfg.DefaultDevice]; !ok {
			return "", usagef("default device profile %q is not configured", cfg.DefaultDevice)
		}
	}
	if _, err := ResolveSpotifyRedirectURI(cfg); err != nil {
		return "", err
	}
	if _, err := ResolveMaxVolume(cfg); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := writeFileAtomic(path, data); err != nil {
		return "", err
	}
	return path, nil
}

// writeFileAtomic replaces path only after a complete, durable temporary file
// has been written in the target directory.
func writeFileAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config directory %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("set config directory permissions %s: %w", dir, err)
	}

	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary config file in %s: %w", dir, err)
	}
	tempPath := temp.Name()
	defer func() {
		if temp != nil {
			_ = temp.Close()
		}
		if err == nil {
			return
		}
		if removeErr := os.Remove(tempPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = fmt.Errorf("%w; remove temporary config %s: %v", err, tempPath, removeErr)
		}
	}()

	if err = temp.Chmod(0600); err != nil {
		return fmt.Errorf("set temporary config permissions %s: %w", tempPath, err)
	}
	if err = writeAll(temp, data); err != nil {
		return fmt.Errorf("write temporary config %s: %w", tempPath, err)
	}
	if err = temp.Sync(); err != nil {
		return fmt.Errorf("sync temporary config %s: %w", tempPath, err)
	}
	if err = temp.Close(); err != nil {
		return fmt.Errorf("close temporary config %s: %w", tempPath, err)
	}
	temp = nil
	if err = beforeAtomicRename(path); err != nil {
		return fmt.Errorf("prepare config replacement %s: %w", path, err)
	}
	if err = os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace config %s: %w", path, err)
	}
	return nil
}

func writeAll(file *os.File, data []byte) error {
	for len(data) > 0 {
		n, err := file.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

const defaultTimeout = 3.0

var maxTimeoutSeconds = float64(math.MaxInt64) / float64(time.Second)

func timeoutRangeError() error {
	return usagef(timeoutRangeErrorMessage)
}

// validateTimeout verifies that seconds can be converted to a positive
// time.Duration without overflowing. A zero Config.Timeout represents an
// omitted setting and is handled by resolveTimeout and SaveConfig before this
// helper is called.
func validateTimeout(timeout float64) error {
	if timeout <= 0 || math.IsNaN(timeout) || math.IsInf(timeout, 0) || timeout >= maxTimeoutSeconds {
		return timeoutRangeError()
	}
	// The range check above makes this conversion safe. Check its result as
	// well: a positive sub-nanosecond value would otherwise truncate to zero.
	if time.Duration(timeout*float64(time.Second)) <= 0 {
		return timeoutRangeError()
	}
	return nil
}

// ResolveTimeout validates and returns an explicitly supplied CLI timeout.
// Callers that need omitted-flag, config, and default selection use
// resolveTimeout with an explicit presence indicator.
func ResolveTimeout(cliTimeout float64, cfg Config) (float64, error) {
	return resolveTimeout(cliTimeout, true, cfg)
}

// resolveTimeout is the CLI-aware resolver. cliTimeoutSet distinguishes an
// absent flag from an explicitly supplied value, including an explicit -1.
func resolveTimeout(cliTimeout float64, cliTimeoutSet bool, cfg Config) (float64, error) {
	if cliTimeoutSet {
		if err := validateTimeout(cliTimeout); err != nil {
			return 0, err
		}
		return cliTimeout, nil
	}
	if cfg.Timeout != 0 {
		if err := validateTimeout(cfg.Timeout); err != nil {
			return 0, err
		}
		return cfg.Timeout, nil
	}
	return defaultTimeout, nil
}
