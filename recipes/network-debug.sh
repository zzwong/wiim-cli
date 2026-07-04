#!/usr/bin/env bash
set -euo pipefail

# Pretty-print WiiM Wi-Fi diagnostics.
# Exits non-zero when the device is unreachable.

WIIM=${WIIM:-wiim}
HOST_ARG=()
if [[ -n "${WIIM_HOST:-}" ]]; then
  HOST_ARG=(--host "$WIIM_HOST")
fi

JSON=$("$WIIM" "${HOST_ARG[@]}" --json status)

if command -v jq &>/dev/null; then
  echo "$JSON" | jq -r '
    "Host: \(.host)",
    "RSSI: \(.wifi.rssi // "N/A") dBm",
    "Frequency: \((.wifi.frequency // "N/A") | tostring) MHz",
    "SNR: \(.wifi.snr // "N/A") dB"'
else
  echo "$JSON"
fi
