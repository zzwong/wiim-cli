package wiim

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type options struct {
	host    string
	timeout float64
	config  string
	asJSON  bool
}

type device interface {
	CastInfo() (map[string]any, error)
	StatusEx() (map[string]any, error)
	PlayerStatus() (map[string]any, error)
	MetaInfo() map[string]any
	Command(string) (any, error)
	SetVolume(int) error
	VolumeUp(int) error
	VolumeDown(int) error
	Mute(bool) error
	Playback(string) error
	PlayURL(string) error
	PlayM3U(string) error
	PlayPromptURL(string) error
	ClearPlaylist() error
	Seek(int) error
	PlayPreset(int, *int) error
	SwitchInput(string) error
}

var newDevice = func(host string, timeout float64) device { return NewClient(host, timeout) }

// Run parses command-line arguments, loads configuration, connects to the WiiM
// device, and dispatches the requested subcommand. On failure it writes the
// error to stderr itself (as a JSON envelope if --json was requested, plain
// text otherwise) and returns an error suitable for ExitCode.
func Run(args []string, stdout, stderr io.Writer) error {
	app := newApp(stdout, stderr)
	app.root.SetArgs(args)
	err := app.root.Execute()
	if err != nil {
		// app.opts.asJSON only reflects a --json the parser actually reached
		// before failing: an unresolvable command or an unknown flag makes
		// cobra/pflag abort before persistent flags are bound at all, and the
		// hand-rolled volume flag parser (DisableFlagParsing) returns on the
		// first bad token, before it necessarily reaches a later --json. Both
		// are common typo shapes, so fall back to scanning the raw args for a
		// bare --json anywhere before a "--" terminator, matching what the
		// volume parser treats as ending flag parsing.
		fmt.Fprintln(stderr, FormatError(err, app.opts.asJSON || argsRequestJSON(args)))
	}
	return err
}

// argsRequestJSON reports whether a bare --json appears anywhere in args
// before a "--" argument terminator (after "--", tokens are positional
// values, not flags). It does not recognize --json=true/--json=false — this
// is only a best-effort fallback for formatting an error that occurred
// before normal flag binding got a chance to see --json; successful/typo-free
// invocations are handled entirely by the real --json persistent flag.
func argsRequestJSON(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "--json" {
			return true
		}
	}
	return false
}

type app struct {
	root   *cobra.Command
	opts   options
	stdout io.Writer
	stderr io.Writer
}

func newApp(stdout, stderr io.Writer) *app {
	a := &app{stdout: stdout, stderr: stderr}
	root := &cobra.Command{
		Use:           "wiim",
		Short:         "Control and inspect a WiiM device on the local network",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.PersistentFlags().StringVar(&a.opts.host, "host", "", "WiiM host or IP address; setup saves it as defaultHost")
	root.PersistentFlags().Float64Var(&a.opts.timeout, "timeout", 3.0, "request timeout in seconds")
	root.PersistentFlags().StringVar(&a.opts.config, "config", "", "path to config JSON")
	root.PersistentFlags().BoolVar(&a.opts.asJSON, "json", false, "emit JSON where supported")
	a.root = root
	a.addCommands()
	return a
}

func (a *app) addCommands() {
	a.root.AddCommand(&cobra.Command{Use: "setup", Aliases: []string{"init"}, Short: "write initial config", Args: cobra.NoArgs, RunE: a.runSetup})
	a.root.AddCommand(a.configCommand())

	for _, spec := range []struct{ use, short string }{
		{"status", "show device status"}, {"now", "show current playback metadata"}, {"mute", "mute playback"}, {"unmute", "unmute playback"},
		{"play", "resume playback"}, {"pause", "pause playback"}, {"stop", "stop playback"}, {"next", "skip to next track"}, {"prev", "skip to previous track"},
		{"clear", "clear current playlist"},
	} {
		use, short := spec.use, spec.short
		a.root.AddCommand(&cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error { return a.runDevice([]string{use}) }})
	}
	a.root.AddCommand(&cobra.Command{Use: "cast-now", Short: "show current Google Cast media metadata", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error { return a.runDevice([]string{"cast-now"}) }})
	a.root.AddCommand(&cobra.Command{Use: "input [name]", Short: "show or switch input/source", Args: cobra.MaximumNArgs(1), RunE: func(_ *cobra.Command, args []string) error { return a.runDevice(append([]string{"input"}, args...)) }})
	a.root.AddCommand(&cobra.Command{Use: "volume [VALUE]", Short: "get or set volume", DisableFlagParsing: true, RunE: a.runVolumeCommand})
	a.root.AddCommand(&cobra.Command{Use: "seek <seconds>", Short: "seek within current media", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error { return a.runDevice([]string{"seek", args[0]}) }})
	for _, spec := range []struct{ cmd, arg, short string }{
		{"play-url", "<url>", "play a direct media/stream URL"}, {"play-m3u", "<url>", "play an M3U/playlist URL"}, {"prompt-url", "<url>", "play a notification/prompt URL"}, {"play-file", "<path>", "serve a local file and play it; runs until stopped"},
	} {
		cmdName := spec.cmd
		a.root.AddCommand(&cobra.Command{Use: cmdName + " " + spec.arg, Short: spec.short, Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error { return a.runDevice([]string{cmdName, args[0]}) }})
	}
	a.root.AddCommand(&cobra.Command{Use: "raw <command>", Short: "send a raw WiiM API command", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error { return a.runDevice([]string{"raw", args[0]}) }})
	a.root.AddCommand(a.presetCommand())
	a.root.AddCommand(a.cliampCommand())
	a.root.AddCommand(a.spotifyCommand())
	a.root.AddCommand(&cobra.Command{Use: "version", Short: "print version", Args: cobra.NoArgs, RunE: a.runVersion})
}

func (a *app) runVolumeCommand(cmd *cobra.Command, args []string) error {
	values := make([]string, 0, 1)
	parseFlags := true
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if parseFlags && arg == "--" {
			parseFlags = false
			continue
		}
		if parseFlags && (arg == "--help" || arg == "-h") {
			return cmd.Help()
		}
		if parseFlags && arg == "--json" {
			a.opts.asJSON = true
			continue
		}
		if parseFlags && strings.HasPrefix(arg, "--host=") {
			a.opts.host = strings.TrimPrefix(arg, "--host=")
			continue
		}
		if parseFlags && arg == "--host" {
			if i+1 >= len(args) {
				return usagef("flag --host requires a value")
			}
			i++
			a.opts.host = args[i]
			continue
		}
		if parseFlags && strings.HasPrefix(arg, "--config=") {
			a.opts.config = strings.TrimPrefix(arg, "--config=")
			continue
		}
		if parseFlags && arg == "--config" {
			if i+1 >= len(args) {
				return usagef("flag --config requires a value")
			}
			i++
			a.opts.config = args[i]
			continue
		}
		if parseFlags && strings.HasPrefix(arg, "--timeout=") {
			value := strings.TrimPrefix(arg, "--timeout=")
			if err := a.setTimeoutFlag(value); err != nil {
				return err
			}
			continue
		}
		if parseFlags && arg == "--timeout" {
			if i+1 >= len(args) {
				return usagef("flag --timeout requires a value")
			}
			i++
			if err := a.setTimeoutFlag(args[i]); err != nil {
				return err
			}
			continue
		}
		if parseFlags && strings.HasPrefix(arg, "-") && !looksLikeRelativeVolume(arg) {
			return usagef("unknown volume option %s", arg)
		}
		values = append(values, arg)
	}
	if len(values) > 1 {
		return usagef("volume accepts at most one value")
	}
	return a.runDevice(append([]string{"volume"}, values...))
}

func (a *app) setTimeoutFlag(value string) error {
	timeout, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return usagef("invalid timeout %q", value)
	}
	a.opts.timeout = timeout
	if flag := a.root.PersistentFlags().Lookup("timeout"); flag != nil {
		flag.Changed = true
	}
	return nil
}

func looksLikeRelativeVolume(value string) bool {
	if len(value) < 2 || value[0] != '-' {
		return false
	}
	_, err := strconv.Atoi(value)
	return err == nil
}

func (a *app) configCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "show or update configuration"}
	cmd.AddCommand(&cobra.Command{Use: "show", Short: "show effective config", Args: cobra.NoArgs, RunE: a.runConfigShow})
	cmd.AddCommand(&cobra.Command{Use: "path", Short: "print config path", Args: cobra.NoArgs, RunE: a.runConfigPath})
	cmd.AddCommand(&cobra.Command{Use: "set <key> <value>", Short: "set defaultHost, timeout, maxVolume, or spotifyRedirectURI", Args: cobra.ExactArgs(2), RunE: a.runConfigSet})
	cmd.AddCommand(&cobra.Command{Use: "unset <key>", Short: "unset defaultHost or spotifyRedirectURI", Args: cobra.ExactArgs(1), RunE: a.runConfigUnset})
	return cmd
}

func (a *app) presetCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "preset", Short: "list or play WiiM presets"}
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "list presets", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error { return a.runDevice([]string{"preset", "list"}) }})
	cmd.AddCommand(&cobra.Command{Use: "play <number> [index]", Short: "play a preset", Args: cobra.RangeArgs(1, 2), RunE: func(_ *cobra.Command, args []string) error {
		return a.runDevice(append([]string{"preset", "play"}, args...))
	}})
	return cmd
}

func (a *app) cliampCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "cliamp", Short: "inspect or hand off cliamp playback"}
	cmd.AddCommand(&cobra.Command{Use: "status", Short: "show cliamp MPRIS status", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error { return a.runDevice([]string{"cliamp", "status"}) }})
	cmd.AddCommand(&cobra.Command{Use: "handoff", Short: "send cliamp HTTP URL to WiiM", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error { return a.runDevice([]string{"cliamp", "handoff"}) }})
	return cmd
}

func (a *app) spotifyCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "spotify", Short: "control Spotify Connect via Web API"}
	cred := &cobra.Command{Use: "credentials", Short: "manage Spotify credentials in OS keychain"}
	cred.AddCommand(&cobra.Command{Use: "set", Short: "store the Spotify client ID and secret in the OS keychain", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error { return a.runSpotify([]string{"credentials", "set"}) }})
	cred.AddCommand(&cobra.Command{Use: "set-secret", Short: "store the Spotify client secret in the OS keychain", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		return a.runSpotify([]string{"credentials", "set-secret"})
	}})
	cred.AddCommand(&cobra.Command{Use: "import-clipboard <id|secret>", Short: "import a Spotify credential from the clipboard", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		return a.runSpotify([]string{"credentials", "import-clipboard", args[0]})
	}})
	cred.AddCommand(&cobra.Command{Use: "status", Short: "show stored Spotify credential status", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error { return a.runSpotify([]string{"credentials", "status"}) }})
	cred.AddCommand(&cobra.Command{Use: "clear", Short: "clear stored Spotify credentials", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error { return a.runSpotify([]string{"credentials", "clear"}) }})
	cmd.AddCommand(cred)
	cmd.AddCommand(&cobra.Command{Use: "login", Short: "authenticate with Spotify and store the token", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error { return a.runSpotify([]string{"login"}) }})
	cmd.AddCommand(&cobra.Command{Use: "logout", Short: "forget the stored Spotify access token", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error { return a.runSpotify([]string{"logout"}) }})
	cmd.AddCommand(a.spotifyReauthCommand("devices", "list Spotify Connect devices", cobra.NoArgs, nil))
	cmd.AddCommand(a.spotifyReauthCommand("transfer <device-id>", "transfer playback to a Spotify device", cobra.RangeArgs(1, 2), func(args []string) []string { return append([]string{"transfer"}, args...) }))
	cmd.AddCommand(a.spotifyReauthCommand("play <spotify-uri-or-url> [device-id]", "start Spotify playback", cobra.RangeArgs(1, 2), func(args []string) []string { return append([]string{"play"}, args...) }))
	return cmd
}

func (a *app) spotifyReauthCommand(use, short string, argsFn cobra.PositionalArgs, build func([]string) []string) *cobra.Command {
	var reauth bool
	var noPlay bool
	cmd := &cobra.Command{Use: use, Short: short, Args: argsFn, RunE: func(_ *cobra.Command, args []string) error {
		payload := []string{strings.Fields(use)[0]}
		if build != nil {
			payload = build(args)
		}
		if noPlay && strings.HasPrefix(use, "transfer") {
			payload = append(payload, "--no-play")
		}
		if reauth {
			payload = append(payload, "--reauth")
		}
		return a.runSpotify(payload)
	}}
	cmd.Flags().BoolVar(&reauth, "reauth", false, "launch browser login automatically if token is missing or stale")
	if strings.HasPrefix(use, "transfer") {
		cmd.Flags().BoolVar(&noPlay, "no-play", false, "transfer without starting playback")
	}
	return cmd
}

func (a *app) loadConfig() (Config, error) { return LoadConfig(a.opts.config) }

func (a *app) cliTimeout() float64 {
	if flag := a.root.PersistentFlags().Lookup("timeout"); flag != nil && flag.Changed {
		return a.opts.timeout
	}
	return -1
}

func (a *app) runDevice(args []string) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	timeout, err := ResolveTimeout(a.cliTimeout(), cfg)
	if err != nil {
		return err
	}
	host, err := ResolveHost(a.opts.host, cfg)
	if err != nil {
		return err
	}
	out, err := dispatch(args, a.opts, host, newDevice(host, timeout), cfg, os.Stdin, a.stdout)
	if err != nil {
		return err
	}
	if out != "" {
		fmt.Fprintln(a.stdout, out)
	}
	return nil
}

func (a *app) runSpotify(args []string) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	out, err := dispatchSpotify(args, a.opts, cfg, os.Stdin, a.stdout)
	if err != nil {
		return err
	}
	if out != "" {
		fmt.Fprintln(a.stdout, out)
	}
	return nil
}

func (a *app) runSetup(_ *cobra.Command, _ []string) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	opts := a.opts
	if a.cliTimeout() < 0 {
		opts.timeout = 0
	}
	out, err := dispatchSetup(nil, opts, cfg)
	if err != nil {
		return err
	}
	fmt.Fprintln(a.stdout, out)
	return nil
}

func (a *app) runVersion(_ *cobra.Command, _ []string) error {
	fmt.Fprintln(a.stdout, Version)
	return nil
}

func (a *app) runConfigPath(_ *cobra.Command, _ []string) error {
	path, err := ConfigPath(a.opts.config)
	if err != nil {
		return err
	}
	fmt.Fprintln(a.stdout, path)
	return nil
}

func (a *app) runConfigShow(_ *cobra.Command, _ []string) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	if cfg.MaxVolume == 0 {
		cfg.MaxVolume = defaultMaxVolume
	}
	if cfg.SpotifyRedirectURI == "" {
		cfg.SpotifyRedirectURI = defaultSpotifyRedirectURI
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 3.0
	}
	out, err := jsonText(cfg)
	if err != nil {
		return err
	}
	fmt.Fprintln(a.stdout, out)
	return nil
}

func (a *app) runConfigUnset(_ *cobra.Command, args []string) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	key := args[0]
	switch key {
	case "defaultHost", "host":
		cfg.DefaultHost = ""
	case "spotifyRedirectURI":
		cfg.SpotifyRedirectURI = ""
	case "timeout":
		cfg.Timeout = 0
	case "maxVolume":
		cfg.MaxVolume = 0
	default:
		return usagef("unknown or non-unsettable config key %s", key)
	}
	path, err := SaveConfig(a.opts.config, cfg)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.stdout, "Unset %s in %s\n", key, path)
	return nil
}

func (a *app) runConfigSet(_ *cobra.Command, args []string) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	key, value := args[0], args[1]
	switch key {
	case "defaultHost", "host":
		if !hostPattern.MatchString(value) {
			return usagef("host must be a hostname or IP address, not a URL")
		}
		cfg.DefaultHost = value
	case "timeout":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil || v <= 0 {
			return usagef("timeout must be a positive number")
		}
		cfg.Timeout = v
	case "maxVolume":
		v, err := strconv.Atoi(value)
		if err != nil {
			return usagef("maxVolume must be between 1 and 100")
		}
		cfg.MaxVolume = v
		if _, err := ResolveMaxVolume(cfg); err != nil {
			return err
		}
	case "spotifyRedirectURI":
		cfg.SpotifyRedirectURI = value
		if _, err := ResolveSpotifyRedirectURI(cfg); err != nil {
			return err
		}
	default:
		return usagef("unknown config key %s", key)
	}
	path, err := SaveConfig(a.opts.config, cfg)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.stdout, "Updated %s in %s\n", key, path)
	return nil
}

func dispatch(args []string, opts options, host string, client device, cfg Config, _ io.Reader, stdout io.Writer) (string, error) {
	cmd := args[0]
	switch cmd {
	case "status":
		cast, _ := client.CastInfo()
		statusEx, err := client.StatusEx()
		if err != nil {
			return "", err
		}
		player, err := client.PlayerStatus()
		if err != nil {
			return "", err
		}
		return FormatStatus(NormalizeStatus(host, statusEx, player, cast), opts.asJSON)
	case "now":
		player, err := client.PlayerStatus()
		if err != nil {
			return "", err
		}
		return FormatNow(NormalizeNow(player, client.MetaInfo()), opts.asJSON)
	case "volume":
		return dispatchVolume(args, opts, cfg, client)
	case "input":
		return dispatchInput(args, opts, client)
	case "cast-now":
		timeout, err := ResolveTimeout(opts.timeout, cfg)
		if err != nil {
			return "", err
		}
		info, err := CastMediaStatus(host, timeout)
		if err != nil {
			return "", err
		}
		return FormatCastMediaInfo(info, opts.asJSON)
	case "mute", "unmute":
		enabled := cmd == "mute"
		if err := client.Mute(enabled); err != nil {
			return "", err
		}
		if opts.asJSON {
			return FormatRaw(map[string]any{"muted": enabled})
		}
		if enabled {
			return "Muted", nil
		}
		return "Unmuted", nil
	case "play-url", "play-m3u", "prompt-url":
		if len(args) < 2 {
			return "", usagef("missing URL argument")
		}
		if err := validateHTTPURL(args[1]); err != nil {
			return "", err
		}
		var err error
		switch cmd {
		case "play-url":
			err = client.PlayURL(args[1])
		case "play-m3u":
			err = client.PlayM3U(args[1])
		case "prompt-url":
			err = client.PlayPromptURL(args[1])
		}
		if err != nil {
			return "", err
		}
		if opts.asJSON {
			return FormatRaw(map[string]any{"url": args[1], "command": cmd})
		}
		return "Sent URL to WiiM", nil
	case "play-file":
		if len(args) < 2 {
			return "", usagef("missing file path argument")
		}
		return PlayFile(client, host, args[1], stdout)
	case "clear":
		if err := client.ClearPlaylist(); err != nil {
			return "", err
		}
		if opts.asJSON {
			return FormatRaw(map[string]any{"cleared": true})
		}
		return "Cleared playlist", nil
	case "seek":
		if len(args) < 2 {
			return "", usagef("missing seconds argument")
		}
		seconds, err := strconv.Atoi(args[1])
		if err != nil || seconds < 0 {
			return "", usagef("seek seconds must be a non-negative integer")
		}
		if err := client.Seek(seconds); err != nil {
			return "", err
		}
		if opts.asJSON {
			return FormatRaw(map[string]any{"position": seconds})
		}
		return fmt.Sprintf("Seeked to %d seconds", seconds), nil
	case "preset":
		return dispatchPreset(args[1:], opts, client)
	case "cliamp":
		return dispatchCliamp(args[1:], opts, client)
	case "play", "pause", "stop", "next", "prev":
		if err := client.Playback(cmd); err != nil {
			return "", err
		}
		if opts.asJSON {
			return FormatRaw(map[string]any{"playbackState": cmd})
		}
		return strings.ToUpper(cmd[:1]) + cmd[1:], nil
	case "raw":
		if len(args) < 2 {
			return "", usagef("missing command argument")
		}
		value, err := client.Command(args[1])
		if err != nil {
			return "", err
		}
		return FormatRaw(value)
	default:
		return "", usagef("unknown command %s", cmd)
	}
}

func dispatchInput(args []string, opts options, client device) (string, error) {
	if len(args) == 1 {
		player, err := client.PlayerStatus()
		if err != nil {
			return "", err
		}
		return FormatInputStatus(InputFromPlayer(player), opts.asJSON)
	}
	input, err := NormalizeInputName(args[1])
	if err != nil {
		return "", err
	}
	if err := client.SwitchInput(input); err != nil {
		return "", err
	}
	if opts.asJSON {
		return FormatRaw(map[string]any{"input": input})
	}
	return "Switched input to " + input, nil
}

func dispatchVolume(args []string, opts options, cfg Config, client device) (string, error) {
	if len(args) == 1 {
		player, err := client.PlayerStatus()
		if err != nil {
			return "", err
		}
		vol := intPtr(player["vol"])
		if opts.asJSON {
			if vol == nil {
				return "{}", nil
			}
			return FormatRaw(map[string]any{"volume": *vol})
		}
		if vol == nil {
			return "", nil
		}
		return fmt.Sprint(*vol), nil
	}
	mode, amount, err := parseVolume(args[1])
	if err != nil {
		return "", err
	}
	maxVolume, err := ResolveMaxVolume(cfg)
	if err != nil {
		return "", err
	}
	if mode == "set" {
		if amount > maxVolume {
			return "", usagef("volume %d exceeds configured maxVolume %d", amount, maxVolume)
		}
		if err := client.SetVolume(amount); err != nil {
			return "", err
		}
		if opts.asJSON {
			return FormatRaw(map[string]any{"volume": amount})
		}
		return fmt.Sprintf("Volume set to %d", amount), nil
	}
	if mode == "up" {
		player, err := client.PlayerStatus()
		if err != nil {
			return "", err
		}
		current := intPtr(player["vol"])
		if current != nil && *current+amount > maxVolume {
			return "", usagef("relative volume would exceed configured maxVolume %d", maxVolume)
		}
		if err := client.VolumeUp(amount); err != nil {
			return "", err
		}
		if opts.asJSON {
			return FormatRaw(map[string]any{"volumeDelta": amount})
		}
		return fmt.Sprintf("Volume increased by %d", amount), nil
	}
	if err := client.VolumeDown(amount); err != nil {
		return "", err
	}
	if opts.asJSON {
		return FormatRaw(map[string]any{"volumeDelta": -amount})
	}
	return fmt.Sprintf("Volume decreased by %d", amount), nil
}

func parseVolume(value string) (string, int, error) {
	if strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		amount, err := strconv.Atoi(value[1:])
		if err != nil {
			return "", 0, usagef("invalid relative volume '%s'", value)
		}
		if amount <= 0 {
			return "", 0, usagef("relative volume amount must be positive")
		}
		if strings.HasPrefix(value, "+") {
			return "up", amount, nil
		}
		return "down", amount, nil
	}
	volume, err := strconv.Atoi(value)
	if err != nil || volume < 0 || volume > 100 {
		return "", 0, usagef("volume must be between 0 and 100")
	}
	return "set", volume, nil
}
