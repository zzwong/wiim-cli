# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2026-07-05

### Added

- `wiim discover`: finds Linkplay/WiiM devices on the local network via SSDP, without needing a host configured in advance. Validates each candidate against the WiiM HTTP API, so it works for any Linkplay device, not just WiiM. (#12)
- JSON error envelope: with `--json`, a failing command now writes a structured `{"error": {"kind", "message", "exitCode"}}` object to stderr instead of a plain-text message, so scripts don't have to string-match prose. Plain-text output is unchanged. (#11)

### Changed

- Documented broader Linkplay device compatibility (Arylic, Audio Pro, etc.) in `docs/api.md`, alongside what's actually been verified against a WiiM Ultra. (#3)

## [0.1.0] - 2026-07-04

### Added

- Full WiiM device control CLI: status, playback (play/pause/stop/next/prev/seek), and volume management with a safety cap.
- Preset listing and playback by index.
- Input source switching (HDMI, optical, line-in, etc.).
- URL, M3U, and local file playback with an embedded HTTP file server.
- Spotify Connect integration with OS keychain credential storage (client ID, client secret, OAuth token).
- Cast metadata display via the Cast v2 protocol over TLS on port 8009 (port 8008 `/setup/eureka_info` is used for device status info).
- `cliamp` integration for handoff from MPRIS-backed local players.
- JSON output mode (`--json`) for scripting.
- Recipes (morning radio, Proxmox alert, network debug) and an agent skill definition.
