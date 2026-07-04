package wiim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
)

const (
	defaultSpotifyRedirectURI = "http://127.0.0.1:19872/login"
	defaultMaxVolume          = 55
)

// Config holds persistent settings for connecting to a WiiM device.
type Config struct {
	DefaultHost        string  `json:"defaultHost"`
	Timeout            float64 `json:"timeout"`
	SpotifyRedirectURI string  `json:"spotifyRedirectURI"`
	MaxVolume          int     `json:"maxVolume"`
}

// ConfigPath returns the config file path. If path is non-empty it is used as-is;
// otherwise the default path ~/.config/wiim-cli/config.json is returned.
func ConfigPath(path string) (string, error) {
	if path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
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
		return Config{}, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, usagef("could not read config %s: %v", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, usagef("invalid config JSON in %s: %v", path, err)
	}
	return cfg, nil
}

var hostPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ResolveHost returns the WiiM hostname from (in order of precedence) the CLI
// argument, the WIIM_HOST environment variable, or the config file. Returns a
// UsageError if none is set or the value is malformed.
func ResolveHost(cliHost string, cfg Config) (string, error) {
	host := cliHost
	if host == "" {
		host = os.Getenv("WIIM_HOST")
	}
	if host == "" {
		host = cfg.DefaultHost
	}
	if host == "" {
		return "", usagef("host is required; pass --host, set WIIM_HOST, or configure defaultHost")
	}
	if !hostPattern.MatchString(host) {
		return "", usagef("host must be a hostname or IP address, not a URL")
	}
	return host, nil
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
	}
	if cfg.MaxVolume == 0 {
		cfg.MaxVolume = defaultMaxVolume
	}
	if cfg.SpotifyRedirectURI == "" {
		cfg.SpotifyRedirectURI = defaultSpotifyRedirectURI
	}
	if cfg.DefaultHost != "" && !hostPattern.MatchString(cfg.DefaultHost) {
		return "", usagef("host must be a hostname or IP address, not a URL")
	}
	if _, err := ResolveSpotifyRedirectURI(cfg); err != nil {
		return "", err
	}
	if _, err := ResolveMaxVolume(cfg); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", err
	}
	return path, nil
}

// ResolveTimeout returns the request timeout in seconds. A non-negative CLI timeout
// takes precedence; otherwise the config value is used; otherwise 3.0.
func ResolveTimeout(cliTimeout float64, cfg Config) (float64, error) {
	if cliTimeout >= 0 {
		if cliTimeout == 0 {
			return 0, usagef("timeout must be a positive number")
		}
		return cliTimeout, nil
	}
	if cfg.Timeout > 0 {
		return cfg.Timeout, nil
	}
	return 3.0, nil
}
