# Recipes

Copy-pasteable shell examples for using `wiim` in scripts. Set `WIIM_HOST` or pass `--host` before running them.

These examples avoid surprising audio playback unless noted.

| Script | Description |
|--------|-------------|
| `morning-radio.sh` | Set a modest volume and play WiiM preset 1 (assumed radio station). |
| `proxmox-alert.sh` | On job failure, play a notification URL via `prompt-url`; prints status as fallback. |
| `network-debug.sh` | Pretty-print Wi-Fi RSSI/frequency/SNR from `wiim --json status` with `jq`; exits non-zero on unreachable host. |
