#!/usr/bin/env bash

set -euo pipefail

air_bin=${1:-}
if [[ -z "$air_bin" || ! -x "$air_bin" ]]; then
  echo "usage: $0 AIR_BINARY" >&2
  exit 2
fi

air_pid=
vite_pid=

cleanup() {
  trap - EXIT INT TERM HUP
  [[ -z "$air_pid" ]] || kill "$air_pid" 2>/dev/null || true
  [[ -z "$vite_pid" ]] || kill "$vite_pid" 2>/dev/null || true
  [[ -z "$air_pid" ]] || wait "$air_pid" 2>/dev/null || true
  [[ -z "$vite_pid" ]] || wait "$vite_pid" 2>/dev/null || true
}
trap cleanup EXIT INT TERM HUP

"$air_bin" -c .air.toml &
air_pid=$!
npm --prefix web run dev &
vite_pid=$!

wait -n "$air_pid" "$vite_pid"
