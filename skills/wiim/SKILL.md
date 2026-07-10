---
name: wiim
description: Use this skill when operating this repository's WiiM CLI or controlling a WiiM device from scripts.
---

# WiiM CLI Operator Guidance

Known development device details should be configured in `~/.config/wiim-cli/config.json` with `wiim setup --host <wiim-host>` or named with `wiim device add <name> <host>`. For targeted commands, host resolution precedence is exactly `--host` > `WIIM_HOST` > explicit `--device` > `defaultDevice` > `defaultHost`; `--host` and `--device` do not mutate config. Do not assume a hardcoded host.

If no host is configured and none was given, run `wiim discover` (or its `wiim device discover`
alias; neither takes a target flag) before asking the user for one — it finds Linkplay/WiiM
devices on the LAN via SSDP in a few seconds. Both discovery forms reject explicit `--host` and
`--device` flags and ignore ambient `WIIM_HOST` plus configured `defaultDevice`/`defaultHost`.
Discovery is read-only and safe to run unprompted; an empty result just means nothing answered,
not an error.

## Safe first commands

Run read-only commands before mutating anything:

```bash
wiim discover
wiim --host <wiim-host> status
wiim --device <name> status          # use one saved profile for this invocation
wiim status                          # use the configured precedence/default
wiim --device <name> now
wiim --device <name> cast-now
wiim --device <name> input
wiim --device <name> volume
wiim --device <name> preset list
wiim device list
wiim device discover                  # same read-only path as wiim discover
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
wiim --host <wiim-host> seek <seconds>
wiim --host <wiim-host> clear
wiim --host <wiim-host> play-url <url>
wiim --host <wiim-host> play-m3u <url>
wiim --host <wiim-host> prompt-url <url>
wiim --host <wiim-host> play-file <path>
wiim --host <wiim-host> preset play <n>
wiim spotify play <spotify-uri-or-url> [device-id]
wiim spotify transfer <spotify-device-id>
```

`spotify play`/`spotify transfer` start audio on the device just like `play-url` or
`preset play` — the safety rule applies to them too, even though they go through Spotify
Connect instead of the WiiM HTTP API.

`play-file <path>` starts a local HTTP server and **blocks in the foreground until stopped**
(Ctrl-C) — it does not return control after the WiiM starts playing. Do not run it and wait
synchronously: launch it as a background/detached process, give it a timeout, or stop the
process yourself once playback has started, or the invocation will hang indefinitely.

## Spotify / cliamp

Spotify commands use Spotify Web API credentials and tokens stored in the OS keychain:

```bash
wiim spotify credentials status
wiim spotify login
wiim spotify devices
wiim spotify devices --reauth
```

`spotify play` and `spotify transfer` are mutating (see above) and need explicit permission;
the commands above are read-only/auth-only and safe to run first.

Never print or reveal keychain secrets. `credentials status` masks client IDs and only reports whether secrets/tokens exist. See `docs/security.md`.

cliamp bridge commands use MPRIS via `playerctl -p cliamp`:

```bash
wiim cliamp status
wiim cliamp handoff
```

`cliamp handoff` only works directly for HTTP/HTTPS URLs. Local files need `play-file`; Spotify needs Spotify Connect commands.

## Other commands

Administrative commands that touch local config/keychain state, not device audio, so the
playback safety rule above doesn't apply to them — but still confirm before clearing
someone's stored credentials. Device profile commands are local-only: `device list`, `device
add`, `device remove`, and `device use` do not contact or change a WiiM. The full profile/discovery
subcommand set is `device list`, `device add <name> <host>`, `device remove <name>`, `device use
<name>`, and `device discover`.

```bash
wiim version
wiim config show
wiim config path
wiim config set <key> <value>
wiim config unset <key>
wiim device list
wiim device add <name> <host>
wiim device remove <name>
wiim device use <name>
wiim spotify credentials set
wiim spotify credentials set-secret
wiim spotify credentials import-clipboard <id|secret>
wiim spotify credentials clear
wiim spotify logout
```

## Troubleshooting

- Connection refused on plain HTTP port 80 is expected for the WiiM API; use the CLI's HTTPS API path.
- The Cast setup endpoint is on port `8008` and uses HTTP.
- The WiiM HTTPS API may use a self-signed/invalid certificate; the CLI intentionally disables certificate verification for LAN device calls.
- For targeted device commands, apply the exact precedence `--host` > `WIIM_HOST` > explicit
  `--device` > `defaultDevice` > `defaultHost`. Inspect `wiim device list` and `wiim config show`;
  `--device <name>` must name a configured profile. Existing `defaultHost` is a legacy fallback
  and is not automatically migrated. Discovery is different: explicit `--host`/`--device` is
  rejected, and `WIIM_HOST` plus configured host/profile selection is ignored.
- Spotify redirect URI defaults to `http://127.0.0.1:19872/login`; override with `spotifyRedirectURI` or `WIIM_SPOTIFY_REDIRECT_URI`.
- On Fedora/Linux, keychain access uses Secret Service; `secret-tool lookup` can print secrets, so avoid it except for debugging.

## Raw exploration

Use `raw` to inspect or verify endpoints:

```bash
wiim --host <wiim-host> raw getStatusEx
wiim --host <wiim-host> raw getPlayerStatus
```
