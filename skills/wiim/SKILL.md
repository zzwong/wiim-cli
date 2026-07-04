---
name: wiim
description: Use this skill when operating this repository's WiiM CLI or controlling a WiiM device from scripts.
---

# WiiM CLI Operator Guidance

Known development device details should be configured in `~/.config/wiim-cli/config.json` with `wiim setup --host <wiim-host>` or temporarily overridden with `--host`. Do not assume a hardcoded host.

## Safe first commands

Run read-only commands before mutating anything:

```bash
wiim --host <wiim-host> status
wiim --host <wiim-host> now
wiim --host <wiim-host> cast-now
wiim --host <wiim-host> input
wiim --host <wiim-host> volume
```

Use JSON for automation:

```bash
wiim --host <wiim-host> --json status
```

## Safety rule

Do not change input, volume, mute state, playback state, presets, or play audio unless the user explicitly asks for it. Never surprise the room with sound. Respect configured `maxVolume` (default 55); do not bypass it with raw volume commands unless explicitly requested.

## Mutating commands

Only run these with explicit user permission:

```bash
wiim --host <wiim-host> input hdmi
wiim --host <wiim-host> volume 30
wiim --host <wiim-host> mute
wiim --host <wiim-host> unmute
wiim --host <wiim-host> play
wiim --host <wiim-host> pause
wiim --host <wiim-host> stop
wiim --host <wiim-host> next
wiim --host <wiim-host> prev
wiim --host <wiim-host> play-url <url>
wiim --host <wiim-host> play-m3u <url>
wiim --host <wiim-host> preset play <n>
```

`play-file <path>` starts a local HTTP server and runs until stopped.

## Spotify / cliamp

Spotify commands use Spotify Web API credentials and tokens stored in the OS keychain:

```bash
wiim spotify credentials status
wiim spotify login
wiim spotify devices
wiim spotify devices --reauth
```

Never print or reveal keychain secrets. `credentials status` masks client IDs and only reports whether secrets/tokens exist. See `docs/security.md`.

cliamp bridge commands use MPRIS via `playerctl -p cliamp`:

```bash
wiim cliamp status
wiim cliamp handoff
```

`cliamp handoff` only works directly for HTTP/HTTPS URLs. Local files need `play-file`; Spotify needs Spotify Connect commands.

## Troubleshooting

- Connection refused on plain HTTP port 80 is expected for the WiiM API; use the CLI's HTTPS API path.
- The Cast setup endpoint is on port `8008` and uses HTTP.
- The WiiM HTTPS API may use a self-signed/invalid certificate; the CLI intentionally disables certificate verification for LAN device calls.
- If no `--host` is supplied, check `WIIM_HOST` or `~/.config/wiim-cli/config.json`.
- Spotify redirect URI defaults to `http://127.0.0.1:19872/login`; override with `spotifyRedirectURI` or `WIIM_SPOTIFY_REDIRECT_URI`.
- On Fedora/Linux, keychain access uses Secret Service; `secret-tool lookup` can print secrets, so avoid it except for debugging.

## Raw exploration

Use `raw` to inspect or verify endpoints:

```bash
wiim --host <wiim-host> raw getStatusEx
wiim --host <wiim-host> raw getPlayerStatus
```
