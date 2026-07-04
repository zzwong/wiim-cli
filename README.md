# wiim-cli

[![CI](https://github.com/zzwong/wiim-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/zzwong/wiim-cli/actions/workflows/ci.yml)

Small, scriptable Go CLI for inspecting and controlling a WiiM device on the local network.

```console
$ wiim --host wiim-ultra.local status
Name: WiiM Ultra
Host: wiim-ultra.local
Model: WiiM_Ultra
Firmware: Linkplay.5.2.813259
Online: yes
Wi-Fi: 5 GHz, 5745 MHz, RSSI -62 dBm
Volume: 35
Muted: no
State: play

$ wiim volume 40
Volume set to 40

$ wiim now
State: play
Title: Classical Sunrise
Artist: BBC Radio 3
Volume: 40
```

## Requirements

- A **WiiM device** (Pro, Ultra, Mini, etc.) on the same LAN.
- **Go 1.25+** — to install or build from source; not needed for prebuilt binaries.
- **Linux keyring** (Secret Service / GNOME Keyring or KWallet) — only for `wiim spotify` commands; macOS uses Keychain automatically.
- **playerctl** — only for `wiim cliamp` commands (Linux-only).

## Install

Prebuilt binaries for Linux, macOS, and Windows are on the [Releases page](https://github.com/zzwong/wiim-cli/releases). With a Go toolchain:

```bash
go install github.com/zzwong/wiim-cli/cmd/wiim@latest
```

Or build from source as described below.

## Build / run from source

```bash
# One-off development runs
go run ./cmd/wiim --help

# Normal build
git clone https://github.com/zzwong/wiim-cli.git
cd wiim-cli
make build           # writes ./wiim
./wiim --host <host> status

# Day-to-day use
make install
wiim status
```

`make build` and `make install` embed `git describe --tags --always --dirty` into `wiim version`. Untagged builds show the commit hash; dirty builds include `-dirty`.

## Configuration

Host resolution order:

1. `--host` flag
2. `WIIM_HOST` environment variable
3. `~/.config/wiim-cli/config.json` key `defaultHost`
4. No fallback — host is required

Initialize config:

```bash
wiim setup --host <wiim-host>
wiim config show
wiim config set defaultHost <wiim-host>
wiim config set maxVolume 55
wiim config set spotifyRedirectURI http://127.0.0.1:19872/login
wiim config path
wiim config unset spotifyRedirectURI
```

Config example:

```json
{
  "defaultHost": "wiim-ultra.local",
  "timeout": 3.0,
  "spotifyRedirectURI": "http://127.0.0.1:19872/login",
  "maxVolume": 55
}
```

`maxVolume` defaults to `55` and is enforced for absolute volume sets and relative volume increases.

## Commands

```bash
wiim status
wiim --json status
wiim now
wiim cast-now
wiim input
wiim input hdmi
wiim volume
wiim volume 30
wiim volume +5
wiim volume -5
wiim mute
wiim unmute
wiim play
wiim pause
wiim stop
wiim next
wiim prev
wiim seek 30
wiim play-url https://example.com/song.mp3
wiim play-m3u https://example.com/station.m3u
wiim prompt-url https://example.com/alert.mp3
wiim play-file ./song.flac
wiim clear
wiim preset list
wiim preset play 1
wiim cliamp status          # Linux only — requires playerctl
wiim cliamp handoff         # Linux only — requires playerctl
wiim spotify credentials set
wiim spotify credentials set-secret
wiim spotify credentials import-clipboard id
wiim spotify credentials import-clipboard secret
wiim spotify credentials status
wiim spotify credentials clear
wiim spotify login
wiim spotify logout
wiim spotify devices [--reauth]
wiim spotify transfer <spotify-device-id> [--no-play] [--reauth]
wiim spotify play spotify:playlist:<id> [spotify-device-id] [--reauth]
wiim version
wiim raw getStatusEx
wiim completion bash        # also: fish, zsh, powershell
```

Global options (`--host`, `--timeout`, `--config`, `--json`) can appear before or after the command. Normal use should prefer config; `--host` is primarily an override for scripts/testing.

**Spotify commands** use the Spotify Web API. Store your client ID/secret in the OS keychain, then login:

```bash
wiim spotify credentials set        # prompts for both ID and secret
wiim spotify credentials set-secret # prompts for secret only (if ID already stored)
wiim spotify login
```

For clipboard imports, use explicit commands so the CLI never guesses whether clipboard contents are an ID or a secret:

```bash
wiim spotify credentials import-clipboard id
wiim spotify credentials import-clipboard secret
```

The default redirect URI is `http://127.0.0.1:19872/login`; configure that in the Spotify developer app, or set `spotifyRedirectURI` / `WIIM_SPOTIFY_REDIRECT_URI` if you prefer a different loopback port/path. Tokens are stored in the OS keychain and refreshed automatically. If refresh fails, the stale token is cleared and the command tells you to reauthorize; add `--reauth` to Spotify playback commands to launch the browser login flow automatically and retry. `WIIM_SPOTIFY_TOKEN` or `SPOTIFY_TOKEN` can still override the token for one-off use. See [`docs/security.md`](docs/security.md) for secret storage notes.

**cliamp commands** use `playerctl -p cliamp` on Linux. `cliamp handoff` can send HTTP/HTTPS URLs exposed by cliamp over MPRIS to WiiM. Local files should use `play-file`; Spotify should use the Spotify Connect commands.

**play-file** starts a local HTTP file server and runs until stopped so the WiiM can fetch the file.

WiiM HTTP API calls use HTTPS with certificate verification disabled, which is expected for LAN devices with self-signed or invalid certificates. Cast info is read from `http://<host>:8008/setup/eureka_info`.

## API notes

See [`docs/api.md`](docs/api.md) for WiiM/Linkplay API references, endpoint mappings, verified fields, and quirks.

## Output and errors

Human-readable output is the default. Use `--json` for normalized JSON where useful. Runtime/API errors are printed to stderr with exit code `1`; validation/usage errors use exit code `2`.

## Tests

Tests do not require a real WiiM device.

```bash
go test ./...
```

## License

MIT — see [LICENSE](LICENSE).
