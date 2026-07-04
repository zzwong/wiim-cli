# Security Notes

## Spotify credentials

WiiM CLI stores Spotify client credentials and OAuth token cache in the OS keychain through `github.com/zalando/go-keyring`.

On Fedora/Linux this uses the Freedesktop Secret Service API, typically backed by GNOME Keyring or KWallet. On macOS it uses Keychain.

Stored keys:

| Service | Username | Value |
| --- | --- | --- |
| `wiim-cli` | `spotify-client-id` | Spotify app client ID |
| `wiim-cli` | `spotify-client-secret` | Spotify app client secret |
| `wiim-cli` | `spotify-token` | JSON OAuth token cache |

The CLI never prints the client secret or OAuth tokens. `spotify credentials status` masks the client ID and only reports booleans for secret/token presence.

## Linux `secret-tool`

On Fedora, the same secrets may be accessible with `secret-tool` while your desktop keyring is unlocked:

```bash
secret-tool lookup service wiim-cli username spotify-client-id
secret-tool lookup service wiim-cli username spotify-client-secret
```

`secret-tool lookup` intentionally prints secrets to stdout. Avoid running it unless debugging, and do not paste terminal output into logs or chats.

Security model:

- Secrets are encrypted at rest when the keyring is locked.
- In a logged-in/unlocked desktop session, processes running as your user may be able to request secrets from the keyring.
- This is still safer than storing secrets in shell history, dotfiles, or project files.

## Clipboard handling

The CLI does not implicitly decide whether clipboard contents are a client ID or secret. Clipboard import requires an explicit target:

```bash
wiim spotify credentials import-clipboard id
wiim spotify credentials import-clipboard secret
```

Clipboard contents may be visible to other local applications depending on your desktop environment. Clear or overwrite the clipboard after importing sensitive values if desired.

## Clearing credentials

Spotify access tokens are refreshed automatically before expiry. If refresh fails, the CLI clears the stale token and asks you to reauthorize. Add `--reauth` to Spotify playback commands to launch the browser login flow automatically for that command.

Clear Spotify OAuth token only:

```bash
wiim spotify logout
```

Clear client ID, client secret, and token:

```bash
wiim spotify credentials clear
```

## Environment variable overrides

For temporary debugging, these environment variables are supported:

```bash
WIIM_SPOTIFY_TOKEN
SPOTIFY_TOKEN
WIIM_SPOTIFY_CLIENT_ID
WIIM_SPOTIFY_CLIENT_SECRET
```

Environment variables can leak through shell history, process inspection, crash reports, logs, or inherited child environments. Prefer keychain storage for normal use.

## LAN exposure of play-file

While `wiim play-file` is running, the CLI serves the chosen file over plain HTTP on the
local interface that routes to the target WiiM device, at an unguessable random-token URL
(32 hex characters from `crypto/rand`). The server listens on a randomly assigned ephemeral
port. Only the exact URL path (token + filename) serves the file; any other path returns 404.

The server stays up until the user presses Ctrl-C. Anyone on the same LAN segment who
obtains the exact URL during that session can fetch the file (any number of times while
the server runs). The random token protects against casual LAN scanning, but a determined local attacker
who can sniff DNS, mDNS, or device-control traffic could reconstruct the URL.

Because the file is served over plain HTTP (WiiM devices do not support HTTPS for local
media sources), the file content is also visible to anyone on the LAN who can observe
HTTP traffic to the serving host.

## Device TLS

WiiM devices present self-signed TLS certificates for their HTTP API and Cast (Chromecast)
control socket. The CLI therefore disables TLS certificate verification (`InsecureSkipVerify:
true`) for all device traffic. See `internal/wiim/client.go`.

This means:

- A LAN attacker who can spoof the WiiM device IP address (e.g. via ARP spoofing, DHCP
  hijacking, or malicious DHCP lease) can intercept or alter device-control commands and
  responses.
- The attacker cannot passively decrypt existing TLS sessions, but can perform an
  active machine-in-the-middle attack because the CLI does not validate the certificate
  chain.
- Control traffic includes volume changes, playback state, input switching, and the
  media URL passed to `play-file`.

This limitation is inherent to the WiiM hardware — the devices ship without a trusted
certificate authority. The recommended mitigation is to treat the LAN as a trusted
environment. If untrusted parties share your network, consider network-level isolation
(VLAN, guest network) to separate the WiiM from other devices.

## Known limitations

- **Spotify OAuth flow**: The CLI uses the authorization-code grant with a state
  parameter to prevent CSRF, but does not use PKCE (Proof Key for Code Exchange).
  PKCE would add an extra layer of protection against interception of the authorization
  code by a malicious application on the same machine. Because the same-user scenario is
  considered out of scope — a local process running as the same user can already read
  secrets directly from the unlocked keyring — the lack of PKCE is an accepted trade-off.
