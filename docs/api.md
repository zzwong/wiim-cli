# WiiM / Linkplay HTTP API Notes

This project uses the WiiM local HTTP API directly. The API is Linkplay-style and is exposed by WiiM devices on the LAN.

## Compatibility

WiiM devices run on the Linkplay platform, and the commands this CLI sends (`getStatusEx`,
`getPlayerStatus`, `setPlayerCmd:*`, etc.) are the shared Linkplay HTTP API documented by
Arylic below, not something WiiM-specific. Other Linkplay-based streamers (Arylic, Audio Pro,
and similar) very likely support the same status/playback/volume/input/preset commands.

That said, this CLI has only been developed and verified against a WiiM Ultra — see
"Verified WiiM Ultra behavior" below. Non-WiiM hardware is untested, and `cast-now`/Cast
metadata specifically depends on the device also exposing Google Cast, which isn't
guaranteed on every Linkplay device. If you run this against other Linkplay hardware,
reports of what works (or doesn't) are welcome.

## Acknowledgments

This CLI's understanding of the API comes directly from the following official documentation:

- WiiM Products HTTP API PDF: <https://www.wiimhome.com/pdf/HTTP%20API%20for%20WiiM%20Products.pdf>
- WiiM Mini HTTP API PDF: <https://www.wiimhome.com/pdf/HTTP%20API%20for%20WiiM%20Mini.pdf>
- Arylic / Linkplay HTTP API docs: <https://developer.arylic.com/httpapi/>

The PDFs are not vendored in this repository because licensing and update cadence are unclear. This file records the subset used and verified by this CLI.

## Protocol shape

Most commands are HTTP GET requests:

```text
GET https://<host>/httpapi.asp?command=<command>
```

Examples:

```bash
curl -k "https://<wiim-host>/httpapi.asp?command=getStatusEx"
curl -k "https://<wiim-host>/httpapi.asp?command=setPlayerCmd:pause"
```

Notes:

- WiiM LAN API uses HTTPS.
- The device certificate may be self-signed or otherwise invalid; this CLI disables certificate verification for device calls.
- Most read endpoints return JSON. Some control endpoints return plain strings.
- Non-2xx HTTP responses are treated as errors by this CLI.

## Cast setup endpoint

WiiM also exposes Google Cast setup info over HTTP:

```text
GET http://<host>:8008/setup/eureka_info
```

This CLI uses it as best-effort supplemental status data, especially for the friendly device name.

Useful fields observed:

- `name`
- `ip_address`
- `connected`
- `build_version`
- `cast_build_revision`

## Discovery

`wiim discover` finds devices without needing a host in advance. It multicasts an SSDP
`M-SEARCH` (`ST: upnp:rootdevice`) to `239.255.255.250:1900` and collects the source IP of
every UDP reply — SSDP replies come back as unicast to the requester's ephemeral port, so
this doesn't join the multicast group or parse `LOCATION`/description XML at all. Because
`upnp:rootdevice` is answered by any UPnP device (smart TVs, printers, routers, not just
Linkplay speakers), every responding IP is then validated with a direct `getStatusEx` call;
only hosts that answer the WiiM HTTP API make it into the result. This validation step is
also why `discover` works for any Linkplay device, not just WiiM — it doesn't check for a
WiiM-specific signature, just that `getStatusEx` responds at all (see
[Compatibility](#compatibility)).

IPv6 isn't supported (IPv4 multicast only), and devices on a different subnet/VLAN than the
CLI won't be found — SSDP multicast doesn't cross routed network boundaries.

## Commands used by this CLI

| CLI command | API command(s) | Notes |
| --- | --- | --- |
| `wiim discover` | SSDP `M-SEARCH`, then `getStatusEx` per candidate | Finds devices on the LAN; see "Discovery" above. |
| `wiim status` | `getStatusEx`, `getPlayerStatus`, Cast `eureka_info` | Combines device/network/player state. Cast lookup is best effort. |
| `wiim now` | `getPlayerStatus`, `getMetaInfo` | Metadata from `getMetaInfo` is preferred; player title/artist/album may be hex encoded. `unknow`/`Unknown` is treated as missing metadata. |
| `wiim cast-now` | Cast protocol on TLS port 8009 | Best-effort Google Cast media-session metadata query. Works only when an active Cast media session is exposed. |
| `wiim input` | `getPlayerStatus` | Maps observed player `mode` codes to source names when known. |
| `wiim input <name>` | `setPlayerCmd:switchmode:<name>` | Switches source input. Supported aliases include `hdmi`/`arc`, `line-in`, `optical`, `coaxial`, `bluetooth`, `wifi`, `phono`, `usb`. |
| `wiim volume` | `getPlayerStatus` | Prints `vol`. |
| `wiim volume <0-100>` | `setPlayerCmd:vol:<n>` | Sets absolute volume after validation. |
| `wiim volume +N` | `getPlayerStatus`, then `setPlayerCmd:vol:<current+N>` | Relative increase, still capped by `maxVolume`. |
| `wiim volume -N` | `getPlayerStatus`, then `setPlayerCmd:vol:<current-N>` | Relative decrease, clamped at `0`. |
| `wiim mute` | `setPlayerCmd:mute:1` | Mutating command. |
| `wiim unmute` | `setPlayerCmd:mute:0` | Mutating command. |
| `wiim play` | `setPlayerCmd:play` | Transport control for current/last active session. |
| `wiim pause` | `setPlayerCmd:pause` | Transport control. |
| `wiim stop` | `setPlayerCmd:stop` | Transport control. |
| `wiim next` | `setPlayerCmd:next` | Skip forward when supported by source. |
| `wiim prev` | `setPlayerCmd:prev` | Skip backward when supported by source. |
| `wiim seek <seconds>` | `setPlayerCmd:seek:<seconds>` | Seek within current media. |
| `wiim play-url <url>` | `setPlayerCmd:play:<url>` | Play a direct media/stream URL. |
| `wiim play-m3u <url>` | `setPlayerCmd:playlist:<url>` | Play a playlist/M3U URL. |
| `wiim prompt-url <url>` | `setPlayerCmd:playPromptUrl:<url>` | Play a notification/prompt URL. |
| `wiim play-file <path>` | `setPlayerCmd:play:<local HTTP URL>` | Starts a local HTTP server and asks WiiM to fetch it. |
| `wiim clear` | `setPlayerCmd:clear_playlist` | Clear current playlist/queue. |
| `wiim preset list` | `getPresetInfo` | List saved presets. |
| `wiim preset play <n>` | `MCUKeyShortClick:<n>` | Play preset slot. Optional index uses `MCUKeyShortClick:<n>:<index>`. |
| `wiim raw <command>` | `<command>` | Escape hatch for exploration. |

## Verified WiiM Ultra behavior

Device used during development:

- Name: `WiiM Ultra`
- Host: `<wiim-host>`

Verified read endpoints:

```text
getStatusEx
getPlayerStatus
getMetaInfo
http://<host>:8008/setup/eureka_info
```

Observed `getStatusEx` fields used by the CLI:

- `ssid`
- `firmware`
- `project`
- `Release`
- `MAC`
- `apcli0`
- `internet`
- `RSSI`
- `wlanSnr`
- `wlanNoise`
- `wlanFreq`
- `wlanDataRate`

Observed `getPlayerStatus` fields used by the CLI:

- `status`
- `vol`
- `mute`
- `Title`
- `Artist`
- `Album`
- `curpos`
- `totlen`

Observed `getMetaInfo` fields used by the CLI:

- `metaData.title`
- `metaData.artist`
- `metaData.album`
- `metaData.albumArtURI`
- `metaData.sampleRate`
- `metaData.bitDepth`
- `metaData.bitRate`

## Spotify / playback notes

Spotify Connect is its own playback/session protocol. WiiM transport commands such as `setPlayerCmd:play` and `setPlayerCmd:pause` can control playback once the WiiM is already the active Spotify Connect target, but they do not browse Spotify, choose a playlist, or start an arbitrary Spotify session by themselves.

## Known quirks

- `getPlayerStatus` title/artist/album may be hex-encoded; the CLI decodes those fields when needed.
- `getPlayerStatus` may report `mode: 0` / `status: none` after switching to an idle input; in that state `wiim input` reports the unknown mode rather than guessing.
- `wlanFreq` appears as a frequency in MHz, e.g. `5745`. Human output formats this as `5 GHz, 5745 MHz`.
- Cast info failures are ignored for `status`; WiiM API failures are not ignored.
- Mutating endpoints should be tested carefully because they affect room audio state.

## Useful raw exploration commands

```bash
wiim raw getStatusEx
wiim raw getPlayerStatus
wiim raw getPlayerStatusEx
wiim raw getMetaInfo
wiim raw getPresetInfo
wiim raw multiroom:getSlaveList
```

Potential future commands to verify before adding first-class CLI support:

```text
setPlayerCmd:playlist:url:<index>
setPlayerCmd:hex_playlist:url:<index>
multiroom:getSlaveList
```

## cliamp bridge

`cliamp` exposes MPRIS metadata on Linux. This CLI can inspect it with `playerctl -p cliamp`:

```bash
wiim cliamp status
wiim cliamp handoff
```

`handoff` sends `xesam:url` to `play-url` only when it is an HTTP/HTTPS URL. Local files require `play-file` so the WiiM can fetch a LAN URL. Spotify items require Spotify Connect transfer/playback because Spotify track identifiers are not direct audio URLs.

## Spotify Connect bridge

Spotify commands are separate from the WiiM HTTP API. They use Spotify's Web API and store credentials/tokens in the OS keychain:

```bash
wiim spotify credentials set
wiim spotify credentials import-clipboard id
wiim spotify credentials import-clipboard secret
wiim spotify credentials status
wiim spotify credentials clear
wiim spotify login
wiim spotify logout
wiim spotify devices [--reauth]
wiim spotify transfer <spotify-device-id> [--no-play] [--reauth]
wiim spotify play spotify:playlist:<id> [spotify-device-id] [--reauth]
```

The Spotify app redirect URI defaults to `http://127.0.0.1:19872/login`; override with config `spotifyRedirectURI` or env `WIIM_SPOTIFY_REDIRECT_URI`. Required scopes are `user-read-playback-state` and `user-modify-playback-state`. Tokens refresh automatically before expiry. If a refresh token is invalid, the stale token is cleared; use `--reauth` to launch the browser login flow automatically for that command. `WIIM_SPOTIFY_TOKEN` or `SPOTIFY_TOKEN` can override the stored token for one-off use.
