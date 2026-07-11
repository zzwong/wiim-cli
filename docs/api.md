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
also why `discover` is designed for compatible Linkplay devices, not just WiiM — it doesn't
check for a WiiM-specific signature, just that `getStatusEx` responds at all (see
[Compatibility](#compatibility)).

IPv6 isn't supported (IPv4 multicast only), and devices on a different subnet/VLAN than the
CLI won't be found — SSDP multicast doesn't cross routed network boundaries. On a multi-homed
host (multiple network interfaces), the multicast request goes out whichever interface the OS
default route picks; a device reachable only via a different, non-default interface won't be
found even though it isn't technically cross-subnet.

`--timeout` doubles as how long `discover` waits for SSDP replies (default `3.0`s); only the
timeout setting is resolved from the flag, config file's `timeout`, or default. Discovery does
not resolve a target host. It rejects explicit `--host` and `--device` flags, and ignores ambient
`WIIM_HOST` plus configured `defaultDevice`/`defaultHost`. A very short `--timeout` can miss
devices that delay their reply toward the end of the window the request itself advertises — the
request's `MX` value is derived from `--timeout` (capped to `[1, 5]`) specifically so it never
asks devices for a longer delay than the CLI is actually willing to wait out.

### CLI targeting and named profiles

Target selection is CLI/config behavior, not part of the WiiM/Linkplay device API. For commands
that target one device, host resolution precedence is exactly `--host` > `WIIM_HOST` > explicit `--device` > `defaultDevice` > `defaultHost`. A named profile is an entry in the local
`devices` config map, with a `host`; `--device <name>` selects it for one invocation and
`device use <name>` saves it as `defaultDevice`. `device list`, `device add`, `device remove`,
and `device use` only read or mutate the local config file and make no device API calls.

`wiim device discover` is an alias for `wiim discover`, not a device API command. Both forms
perform the same hostless SSDP search, validate candidates with read-only status requests, and
never write profiles or other config. To save a result, explicitly run `wiim device add
<name> <host>` after discovery; there is no automatic discovery-to-config migration.

## Commands used by this CLI

| CLI command | API command(s) | Notes |
| --- | --- | --- |
| `wiim discover` / `wiim device discover` | SSDP `M-SEARCH`, then `getStatusEx` per candidate | Hostless, read-only discovery alias pair; explicit `--host`/`--device` are rejected. See "Discovery" above. |
| `wiim device list` | — | Lists named profiles from local config; no device API call. |
| `wiim device add <name> <host>` | — | Adds a named profile to local config; no device API call. |
| `wiim device remove <name>` | — | Removes a named profile from local config; no device API call. |
| `wiim device use <name>` | — | Sets local `defaultDevice`; no device API call. |
| `wiim status` | `getStatusEx`, `getPlayerStatus`, Cast `eureka_info` | Combines device/network/player state. Cast lookup is best effort. |
| `wiim group status` | `getStatusEx`, then `multiroom:getSlaveList` | Read-only group status: maps the selected device's identity and Linkplay group flag to its role, then maps the guest-device list to member count. |
| `wiim group members` | `multiroom:getSlaveList` | Read-only list of guest devices in the selected device's multiroom group; this is the only API call. |
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

## Multiroom group inspection

The published [Arylic / Linkplay HTTP API documentation](https://developer.arylic.com/httpapi/#request-a-list-of-guest-devices-in-a-multiroom-group)
defines `multiroom:getSlaveList` as the read-only request for guest devices in a
multiroom group. The CLI uses that published Arylic/Linkplay contract for both commands;
the API's terminology calls guest devices "slaves".

### Call patterns and targeting

The requests used by the mapping above are:

```text
GET https://<host>/httpapi.asp?command=getStatusEx
GET https://<host>/httpapi.asp?command=multiroom:getSlaveList
```

`group status` always performs the `getStatusEx` call first and then the member query;
`group members` performs only the member query. Neither command calls playback, volume,
or other mutating APIs. The selected device/profile is treated as the group API host:
the CLI does not discover a master, switch to another device, or change the selected
profile. Thus a selected guest can report `slave`; the result describes the selected
host's view of the group.

### Published response shapes and normalization

The published modern shape uses lowercase keys and an array for a populated group:

```json
{
  "slaves": 1,
  "wmrm_version": "4.2",
  "slave_list": [{
    "name": "Kitchen",
    "uuid": "guest-uuid",
    "ip": "192.0.2.21",
    "version": "4.2",
    "type": "A31",
    "channel": 0,
    "volume": 20,
    "mute": 0,
    "battery_percent": 100,
    "battery_charging": 0
  }]
}
```

Older/legacy firmware may return the same shape with capitalized keys and scalar
encodings, for example:

```json
{
  "Slaves": "1",
  "WMRM_Version": "4.2",
  "Slave_list": [{
    "Name": "Kitchen",
    "UUID": "guest-uuid",
    "Ip": "192.0.2.21",
    "Version": "4.2",
    "Type": "A31",
    "Channel": "0",
    "Volume": "20",
    "Mute": "0",
    "Battery_percent": "100",
    "Battery_charging": "false",
    "Mask": "0"
  }]
}
```

The normalizer accepts these modern lowercase and legacy capitalized forms, including a
single guest object in `slave_list`, but emits a strict stable schema. `wiim group members
--json` has these top-level fields:

- `wmrmVersion` (optional string), `count` (integer), and `members` (always an array).
- Each member may contain `name`, `uuid`, `ip`, `version`, and `type` (strings); `channel`,
  `volume`, and `batteryPercent` (integers); and `muted`, `batteryCharging`, and `masked`
  (booleans). Optional fields are omitted when the source does not provide them.

`slaves` is the guest count, not a count including the selected host. `slaves: 0` means the
selected host reports no guest members (the published standalone response); the role is
`standalone` only when `getStatusEx` also reports `group: 0`. A selected guest can report
`slave` even with no guest members of its own. The documented zero-member response may omit
`slave_list` (an empty array is accepted too). For a populated response, `slaves` must equal
the number of `slave_list` entries. The CLI rejects missing lists for nonzero counts,
mismatched counts, malformed required values, and duplicate keys that differ only by
capitalization. The normalized `count` and `members` length are consequently always
consistent.

### Group status schema and role derivation

`wiim group status --json` emits the stable fields `name`, `host`, `model`, `firmware`,
`role`, `grouped`, `groupName`, `memberCount`, `wmrmVersion`, and `masterUUID`; optional
empty identity/group fields are omitted. `name`, `model`, and `firmware` come from
`DeviceName`, `project`, and `firmware` in `getStatusEx`. `groupName`, `masterUUID`, and
`wmrmVersion` come from `GroupName`, `master_uuid`, and `wmrm_version` (with the member
response's `wmrm_version` taking precedence). `groupName` is emitted only when the device
is grouped (`master` or `slave`); `standalone` and `unknown` statuses omit it even if the
device's `GroupName` mirrors its `DeviceName`.

Role derivation uses the selected host's `getStatusEx` `group` flag and normalized guest
list:

- `group: 1` → `slave`, `grouped: true`.
- `group: 0` with one or more guests → `master`, `grouped: true`.
- `group: 0` with no guests → `standalone`, `grouped: false`.
- If `group` is missing or unusable, a nonempty guest list falls back to `master`; with no
  reliable flag and no guests the role is `unknown`.

`master` is the selected host reporting guests, `slave` is a selected guest device,
`standalone` is an ungrouped device with no guests, and `unknown` means the response does
not provide enough reliable information. `memberCount` is always the normalized guest-list
length, not a separately trusted raw field.

### Scope and compatibility

Both commands are read-only and never mutate grouping or audio. First-class group mutation is
out of scope: join/add, leave/remove, kick, group volume, group mute, and group channel
assignment/switching are intentionally not supported. The raw escape hatch is separate and
must not be used for such mutations without explicit permission.

These semantics and field names are based on the published Arylic/Linkplay documentation
linked above, plus the published WiiM HTTP API PDFs listed in [Acknowledgments](#acknowledgments).
They are documentation-based and subject to model-and-firmware variation; no grouped WiiM
hardware has been verified for this implementation. In particular, the examples describe the
published protocol shapes, not a claim of live grouped-hardware testing. The member GET
itself remains read-only.

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
```

The Spotify app redirect URI defaults to `http://127.0.0.1:19872/login`; override with config `spotifyRedirectURI` or env `WIIM_SPOTIFY_REDIRECT_URI`. Required scopes are `user-read-playback-state` and `user-modify-playback-state`. Tokens refresh automatically before expiry. If a refresh token is invalid, the stale token is cleared; use `--reauth` to launch the browser login flow automatically for that command. `WIIM_SPOTIFY_TOKEN` or `SPOTIFY_TOKEN` can override the stored token for one-off use.
