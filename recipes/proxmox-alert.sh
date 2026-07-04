#!/usr/bin/env bash
set -euo pipefail

# Proxmox backup/job alert via WiiM prompt URL.
# Usage: ALERT_URL=https://example.com/alert.mp3 ./proxmox-alert.sh failed
# If ALERT_URL is unset, prints status instead.

WIIM=${WIIM:-wiim}
HOST_ARG=()
if [[ -n "${WIIM_HOST:-}" ]]; then
  HOST_ARG=(--host "$WIIM_HOST")
fi

if [[ "${1:-ok}" != "ok" ]]; then
  if [[ -n "${ALERT_URL:-}" ]]; then
    "$WIIM" "${HOST_ARG[@]}" prompt-url "$ALERT_URL"
  else
    # shellcheck disable=SC2016
    echo 'Set $ALERT_URL to a direct audio URL (e.g. https://example.com/alert.mp3) to play a sound.' >&2
    "$WIIM" "${HOST_ARG[@]}" status
  fi
fi
