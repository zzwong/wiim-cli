package wiim

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
)

func dispatchSetup(args []string, opts options, cfg Config) (string, error) {
	if len(args) != 0 {
		return "", usagef("setup takes no arguments; use --host/--config/--timeout")
	}
	host := opts.host
	if host == "" {
		host = os.Getenv("WIIM_HOST")
	}
	if host == "" {
		host = cfg.DefaultHost
	}
	cfg.DefaultHost = host
	if opts.timeout > 0 {
		cfg.Timeout = opts.timeout
	}
	path, err := WriteInitialConfig(opts.config, cfg)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Wrote config to %s", path), nil
}

func stripSpotifyReauth(args []string) ([]string, bool) {
	out := make([]string, 0, len(args))
	allow := false
	for _, arg := range args {
		if arg == "--reauth" {
			allow = true
			continue
		}
		out = append(out, arg)
	}
	return out, allow
}

func validateHTTPURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return usagef("URL must be an absolute http or https URL")
	}
	return nil
}

func dispatchPreset(args []string, opts options, client device) (string, error) {
	if len(args) == 0 {
		return "", usagef("preset requires subcommand: list or play")
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return "", usagef("preset list takes no arguments")
		}
		value, err := client.Command("getPresetInfo")
		if err != nil {
			return "", err
		}
		if opts.asJSON {
			return FormatRaw(value)
		}
		return FormatPresets(value)
	case "play":
		if len(args) < 2 || len(args) > 3 {
			return "", usagef("preset play requires preset number and optional index")
		}
		preset, err := strconv.Atoi(args[1])
		if err != nil || preset < 1 {
			return "", usagef("preset number must be 1 or higher")
		}
		var index *int
		if len(args) == 3 {
			i, err := strconv.Atoi(args[2])
			if err != nil || i < 1 {
				return "", usagef("preset index must be 1 or higher")
			}
			index = &i
		}
		if err := client.PlayPreset(preset, index); err != nil {
			return "", err
		}
		if opts.asJSON {
			out := map[string]any{"preset": preset}
			if index != nil {
				out["index"] = *index
			}
			return FormatRaw(out)
		}
		return fmt.Sprintf("Playing preset %d", preset), nil
	default:
		return "", usagef("unknown preset subcommand %s", args[0])
	}
}

func dispatchCliamp(args []string, opts options, client device) (string, error) {
	if len(args) != 1 {
		return "", usagef("cliamp requires subcommand: status or handoff")
	}
	switch args[0] {
	case "status":
		info, err := CliampStatus()
		if err != nil {
			return "", err
		}
		if opts.asJSON {
			return FormatRaw(map[string]any{"status": info.Status, "title": info.Title, "artist": info.Artist, "album": info.Album, "url": info.URL})
		}
		return FormatCliampInfo(info), nil
	case "handoff":
		return CliampHandoff(client)
	default:
		return "", usagef("unknown cliamp subcommand %s", args[0])
	}
}

func dispatchSpotify(args []string, opts options, cfg Config, stdin io.Reader, stdout io.Writer) (string, error) {
	if len(args) == 0 {
		return "", usagef("spotify requires subcommand: credentials, login, logout, devices, transfer, or play")
	}
	switch args[0] {
	case "credentials":
		if len(args) < 2 {
			return "", usagef("spotify credentials requires subcommand: set, status, import-clipboard, or clear")
		}
		switch args[1] {
		case "set":
			if len(args) != 2 {
				return "", usagef("spotify credentials set takes no arguments")
			}
			if err := SpotifyCredentialsSet(stdin, stdout); err != nil {
				return "", err
			}
			return "Spotify credentials stored in OS keychain", nil
		case "import-clipboard":
			if len(args) != 3 {
				return "", usagef("spotify credentials import-clipboard requires id or secret")
			}
			if err := SpotifyCredentialsImportClipboard(args[2]); err != nil {
				return "", err
			}
			return "Spotify credential imported from clipboard into OS keychain", nil
		case "set-secret":
			if len(args) != 2 {
				return "", usagef("spotify credentials set-secret takes no arguments")
			}
			if err := SpotifyCredentialsSetSecretPrompt(stdout); err != nil {
				return "", err
			}
			return "Spotify client secret stored in OS keychain", nil
		case "clear":
			if len(args) != 2 {
				return "", usagef("spotify credentials clear takes no arguments")
			}
			if err := SpotifyCredentialsClear(); err != nil {
				return "", err
			}
			return "Spotify credentials and token cleared from OS keychain", nil
		case "status":
			status, err := SpotifyCredentialsStatus()
			if err != nil {
				return "", err
			}
			return FormatRaw(status)
		default:
			return "", usagef("unknown spotify credentials subcommand %s", args[1])
		}
	case "login":
		if len(args) != 1 {
			return "", usagef("spotify login takes no arguments")
		}
		redirectURI, err := ResolveSpotifyRedirectURI(cfg)
		if err != nil {
			return "", err
		}
		if err := SpotifyLogin(stdout, redirectURI); err != nil {
			return "", err
		}
		return "", nil
	case "logout":
		if len(args) != 1 {
			return "", usagef("spotify logout takes no arguments")
		}
		if err := SpotifyLogout(); err != nil {
			return "", err
		}
		return "Spotify token cleared from OS keychain", nil
	}
	allowReauth := false
	args, allowReauth = stripSpotifyReauth(args)
	if len(args) == 0 {
		return "", usagef("spotify requires subcommand after --reauth")
	}
	redirectURI, err := ResolveSpotifyRedirectURI(cfg)
	if err != nil {
		return "", err
	}
	client, err := NewSpotifyClient(allowReauth, stdout, redirectURI)
	if err != nil {
		return "", err
	}
	switch args[0] {
	case "devices":
		if len(args) != 1 {
			return "", usagef("spotify devices takes no arguments")
		}
		value, err := client.Devices()
		if err != nil {
			return "", err
		}
		if opts.asJSON {
			return FormatRaw(value)
		}
		return FormatSpotifyDevices(value)
	case "transfer":
		if len(args) < 2 || len(args) > 3 {
			return "", usagef("spotify transfer requires device ID and optional --no-play")
		}
		play := true
		if len(args) == 3 {
			if args[2] != "--no-play" {
				return "", usagef("unknown spotify transfer option %s", args[2])
			}
			play = false
		}
		if err := client.Transfer(args[1], play); err != nil {
			return "", err
		}
		if opts.asJSON {
			return FormatRaw(map[string]any{"deviceID": args[1], "play": play})
		}
		return "Transferred Spotify playback", nil
	case "play":
		if len(args) < 2 || len(args) > 3 {
			return "", usagef("spotify play requires spotify URI/URL and optional device ID")
		}
		deviceID := ""
		if len(args) == 3 {
			deviceID = args[2]
		}
		if err := client.Play(deviceID, args[1]); err != nil {
			return "", err
		}
		if opts.asJSON {
			return FormatRaw(map[string]any{"uri": spotifyURI(args[1]), "deviceID": deviceID})
		}
		return "Started Spotify playback", nil
	default:
		return "", usagef("unknown spotify subcommand %s", args[0])
	}
}
