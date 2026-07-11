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
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSpotifyRedirectURI = "http://127.0.0.1:19872/login"
	defaultMaxVolume          = 55
	timeoutRangeErrorMessage  = "timeout must be a positive number within the supported duration range"
)

var (
	userHomeDir       = os.UserHomeDir
	renameFile        = os.Rename
	preserveOwnership = preserveExistingFileOwnership
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
	knownConfigKeys = map[string]struct{}{
		"defaultHost":        {},
		"defaultDevice":      {},
		"devices":            {},
		"timeout":            {},
		"spotifyRedirectURI": {},
		"maxVolume":          {},
	}
	hostPattern            = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	deviceNamePattern      = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	spotifyRedirectPattern = regexp.MustCompile(`^http://127\.0\.0\.1:[0-9]+/[A-Za-z0-9._~/-]+$`)
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

// validateSpotifyRedirectURI validates one Spotify OAuth redirect URI without
// consulting process state such as environment variables or configuration.
func validateSpotifyRedirectURI(value string) error {
	if !spotifyRedirectPattern.MatchString(value) {
		return usagef("spotifyRedirectURI must be a loopback http URL like http://127.0.0.1:19872/login")
	}
	return nil
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
	if err := validateSpotifyRedirectURI(redirectURI); err != nil {
		return "", err
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
	requestedPath, err := ConfigPath(path)
	if err != nil {
		return "", err
	}
	targetPath, err := resolveConfigWritePath(requestedPath)
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
	if err := validateSpotifyRedirectURI(cfg.SpotifyRedirectURI); err != nil {
		return "", err
	}
	if cfg.DefaultHost != "" {
		if err := ValidateHost(cfg.DefaultHost); err != nil {
			return "", err
		}
	}
	if err := ValidateDeviceProfiles(cfg); err != nil {
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
	data, err = preserveUnknownConfigFields(targetPath, data)
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(targetPath, data); err != nil {
		return "", err
	}
	return requestedPath, nil
}

// preserveUnknownConfigFields keeps forward-compatible settings from an
// existing JSON object. Current Config fields always win, so known keys are
// overwritten or omitted according to the config being saved. Unknown fields
// within a profile are retained only for profiles still present in cfg and only
// when the previous object has exactly one case-insensitive devices key.
func preserveUnknownConfigFields(path string, data []byte) ([]byte, error) {
	existing, err := os.ReadFile(path)
	if err != nil {
		// SaveConfig historically overwrote unreadable, malformed, and absent
		// targets when the atomic replacement itself was possible. Preserve that
		// behavior when there is no readable object to merge.
		return data, nil
	}

	var previous map[string]json.RawMessage
	if err := json.Unmarshal(existing, &previous); err != nil || previous == nil {
		return data, nil
	}
	var current map[string]json.RawMessage
	if err := json.Unmarshal(data, &current); err != nil {
		return nil, err
	}

	preserved := false
	for key, value := range previous {
		if isKnownConfigKey(key) {
			continue
		}
		current[key] = value
		preserved = true
	}
	profileFieldsPreserved, err := preserveUnknownDeviceProfileFields(existing, current)
	if err != nil {
		return nil, err
	}
	preserved = preserved || profileFieldsPreserved
	if !preserved {
		return data, nil
	}

	merged, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(merged, '\n'), nil
}

func isKnownConfigKey(key string) bool {
	for knownKey := range knownConfigKeys {
		if strings.EqualFold(key, knownKey) {
			return true
		}
	}
	return false
}

// preserveUnknownDeviceProfileFields merges unknown fields only into current
// profiles. In particular, it never adds a profile from the previous config.
func preserveUnknownDeviceProfileFields(previousData []byte, current map[string]json.RawMessage) (bool, error) {
	currentDevices, ok := current["devices"]
	if !ok {
		return false, nil
	}
	var currentProfiles map[string]json.RawMessage
	if err := json.Unmarshal(currentDevices, &currentProfiles); err != nil || currentProfiles == nil {
		return false, nil
	}

	previousDevices, ok := uniqueTopLevelDevicesValue(previousData)
	if !ok {
		return false, nil
	}
	var previousProfiles map[string]json.RawMessage
	if err := json.Unmarshal(previousDevices, &previousProfiles); err != nil || previousProfiles == nil {
		return false, nil
	}

	preserved := false
	for name, previousProfile := range previousProfiles {
		currentProfile, ok := currentProfiles[name]
		if !ok {
			continue
		}
		mergedProfile, changed := mergeUnknownDeviceProfileFields(previousProfile, currentProfile)
		if !changed {
			continue
		}
		currentProfiles[name] = mergedProfile
		preserved = true
	}
	if !preserved {
		return false, nil
	}
	mergedDevices, err := json.Marshal(currentProfiles)
	if err != nil {
		return false, err
	}
	current["devices"] = mergedDevices
	return true, nil
}

// uniqueTopLevelDevicesValue returns the devices value only when exactly one
// top-level member key matches devices case-insensitively. It uses a streaming
// decoder because unmarshaling an object into a map loses duplicate members.
func uniqueTopLevelDevicesValue(data []byte) (json.RawMessage, bool) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return nil, false
	}
	opening, ok := token.(json.Delim)
	if !ok || opening != '{' {
		return nil, false
	}

	var devices json.RawMessage
	matches := 0
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, false
		}
		key, ok := token.(string)
		if !ok {
			return nil, false
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, false
		}
		if strings.EqualFold(key, "devices") {
			matches++
			devices = value
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') || matches != 1 {
		return nil, false
	}
	return devices, true
}

func mergeUnknownDeviceProfileFields(previous, current json.RawMessage) (json.RawMessage, bool) {
	var previousFields map[string]json.RawMessage
	if err := json.Unmarshal(previous, &previousFields); err != nil || previousFields == nil {
		return current, false
	}
	var currentFields map[string]json.RawMessage
	if err := json.Unmarshal(current, &currentFields); err != nil || currentFields == nil {
		return current, false
	}

	preserved := false
	for key, value := range previousFields {
		if strings.EqualFold(key, "host") {
			continue
		}
		currentFields[key] = value
		preserved = true
	}
	if !preserved {
		return current, false
	}
	merged, err := json.Marshal(currentFields)
	if err != nil {
		return current, false
	}
	return merged, true
}

// resolveConfigWritePath follows an existing final-component symlink so the
// config referent, rather than the link itself, can be atomically replaced.
// A nonexistent path remains unchanged so a new config can be created.
func resolveConfigWritePath(path string) (string, error) {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return path, nil
		}
		targetPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "", fmt.Errorf("resolve config symlink %s: %w", path, err)
		}
		return targetPath, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	return "", fmt.Errorf("inspect config path %s: %w", path, err)
}

// writeFileAtomic writes data to a complete, durable temporary file before
// renaming it into place. The containing directory is synced after the rename
// on platforms that support directory syncing; the rename's atomicity is
// platform-dependent.
func writeFileAtomic(path string, data []byte) (err error) {
	targetPath, err := resolveConfigWritePath(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config directory %s: %w", dir, err)
	}

	temp, err := os.CreateTemp(dir, "."+filepath.Base(targetPath)+".tmp-*")
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
	if err = preserveOwnership(targetPath, tempPath); err != nil {
		return fmt.Errorf("preserve config ownership %s: %w", targetPath, err)
	}
	if err = temp.Sync(); err != nil {
		return fmt.Errorf("sync temporary config %s: %w", tempPath, err)
	}
	if err = temp.Close(); err != nil {
		return fmt.Errorf("close temporary config %s: %w", tempPath, err)
	}
	temp = nil
	if err = renameFile(tempPath, targetPath); err != nil {
		return fmt.Errorf("replace config %s: %w", targetPath, err)
	}
	if err = syncConfigDirectory(dir); err != nil {
		return fmt.Errorf("sync config directory %s: %w", dir, err)
	}
	return nil
}

func syncConfigDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return err
	}
	return dir.Close()
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
