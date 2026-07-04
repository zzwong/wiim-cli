# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
