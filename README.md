# wiim-cli

[![CI](https://github.com/zzwong/wiim-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/zzwong/wiim-cli/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/zzwong/wiim-cli.svg)](https://pkg.go.dev/github.com/zzwong/wiim-cli)
[![Release](https://img.shields.io/github/v/release/zzwong/wiim-cli)](https://github.com/zzwong/wiim-cli/releases)
[![Go 1.25+](https://img.shields.io/badge/go-1.25+-00ADD8.svg)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

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

- A **WiiM device** (Pro, Ultra, Mini, etc.) on the same LAN. Other Linkplay-based streamers
  (Arylic, Audio Pro, etc.) share the same API and likely work too — see
  [Compatibility](docs/api.md#compatibility) for what's actually verified.
- **Go 1.25+** to build from source; not needed for prebuilt binaries.
- **Linux keyring** (Secret Service / GNOME Keyring or KWallet) for `wiim spotify` commands; macOS uses Keychain automatically.
- **playerctl** for `wiim cliamp` commands (Linux only).

## Install

Prebuilt binaries for Linux, macOS, and Windows are on the [Releases page](https://github.com/zzwong/wiim-cli/releases).

With a Go toolchain:

```bash
go install github.com/zzwong/wiim-cli/cmd/wiim@latest
```

Or build from source:

```bash
git clone https://github.com/zzwong/wiim-cli.git
cd wiim-cli
make build      # writes ./wiim
make install    # or: go install ./cmd/wiim
```

`make build`/`make install` embed `git describe --tags --always --dirty` into `wiim version`. Untagged builds show the commit hash; dirty builds add `-dirty`.

## Configuration

Host resolution order: `--host` flag → `WIIM_HOST` env var → `defaultHost` in
`~/.config/wiim-cli/config.json` → error (host is required).

```bash
wiim setup --host <wiim-host>              # writes defaultHost to config
wiim config show
wiim config set maxVolume 55
wiim config set spotifyRedirectURI http://127.0.0.1:19872/login
wiim config unset spotifyRedirectURI
wiim config path
```

```json
{
  "defaultHost": "wiim-ultra.local",
  "timeout": 3.0,
  "spotifyRedirectURI": "http://127.0.0.1:19872/login",
  "maxVolume": 55
}
```

`maxVolume` (default `55`) caps absolute volume sets and relative volume increases.

## Commands

```bash
# Status
wiim status
wiim --json status
wiim now
wiim cast-now

# Playback
wiim play
wiim pause
wiim stop
wiim next
wiim prev
wiim seek 30
wiim clear

# Volume
wiim volume
wiim volume 30
wiim volume +5
wiim volume -5
wiim mute
wiim unmute

# Input & presets
wiim input
wiim input hdmi
wiim preset list
wiim preset play 1

# Play media
wiim play-url https://example.com/song.mp3
wiim play-m3u https://example.com/station.m3u
wiim prompt-url https://example.com/alert.mp3
wiim play-file ./song.flac

# Spotify Connect
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

# cliamp (Linux, requires playerctl)
wiim cliamp status
wiim cliamp handoff

# Utility
wiim raw getStatusEx
wiim version
wiim completion bash        # also: fish, zsh, powershell
```

Global options (`--host`, `--timeout`, `--config`, `--json`) work before or after the
command. Prefer config for daily use; `--host` is mainly an override for scripts/testing.

**Spotify** — store credentials once, then log in:

```bash
wiim spotify credentials set   # prompts for client ID and secret
wiim spotify login
```

Clipboard imports use explicit `id`/`secret` subcommands so the CLI never guesses which is
which. Tokens live in the OS keychain and refresh automatically; add `--reauth` to a Spotify
command to relaunch the browser login flow if refresh fails. The default redirect URI is
`http://127.0.0.1:19872/login` — override with config `spotifyRedirectURI` or env
`WIIM_SPOTIFY_REDIRECT_URI`. See [`docs/security.md`](docs/security.md) for storage details.

**cliamp** bridges `playerctl -p cliamp` (MPRIS) to WiiM. `handoff` only works for
HTTP/HTTPS URLs — use `play-file` for local files and Spotify Connect for Spotify.

**play-file** serves the given file over a local HTTP server until stopped, so the WiiM can
fetch it.

## Output and errors

Human-readable output by default; `--json` for scripting. Runtime/API errors exit `1`;
validation/usage errors exit `2`.

## Docs

- [`docs/api.md`](docs/api.md) — WiiM/Linkplay API reference, endpoint mappings, verified
  fields, quirks, and device compatibility.
- [`docs/security.md`](docs/security.md) — credential storage, OAuth token handling, LAN
  file-serving exposure, and TLS caveats.

## Contributing

```bash
go test ./...   # no real WiiM device required
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for dev setup, linting, and PR guidelines, and
[SECURITY.md](SECURITY.md) to report a vulnerability.

## Acknowledgments

This CLI was built directly from official WiiM and Arylic/Linkplay HTTP API documentation —
see [Acknowledgments in `docs/api.md`](docs/api.md#acknowledgments) for the specific sources.

## License

MIT — see [LICENSE](LICENSE).
