#!/usr/bin/env bash
set -euo pipefail

# Morning radio: set volume and play preset 1 (a radio station).
# Usage: WIIM_HOST=wiim-ultra.local ./morning-radio.sh

WIIM=${WIIM:-wiim}
HOST_ARG=()
if [[ -n "${WIIM_HOST:-}" ]]; then
  HOST_ARG=(--host "$WIIM_HOST")
fi

"$WIIM" "${HOST_ARG[@]}" volume "${MORNING_VOLUME:-20}"
"$WIIM" "${HOST_ARG[@]}" preset play 1
