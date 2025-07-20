#!/usr/bin/env bash
set -euo pipefail

action="${1:-discover}"
echo "[debug] action='$action'" >&2
echo "[debug] all args: $@" >&2

case "$action" in
  discover)
    echo "[debug] returning tools.json content" >&2
    cat tools.json
    ;;

  call)
    # Read the full payload from stdin
    payload=$(cat)

    # Extract the name
    name=$(echo "$payload" | jq -r .name)
    echo "[debug] extracted name: '$name'" >&2

    # Build the result JSON
    jq -n --arg g "Hello, $name!" '{greeting: $g}' \
      | tee /dev/stderr  # echo debug if you like
    ;;

  *)
    echo "Usage: $0 [discover|call]" >&2
    exit 1
    ;;
esac
