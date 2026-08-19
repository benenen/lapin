#!/usr/bin/env bash

# Stops a detached `make watch` tree: watch.sh -> air -> the Go server, and vite -> esbuild.
# Matching by pattern with pkill is not safe here — the caller's own command line usually
# contains "make watch" too, so pkill kills the shell that invoked it. Collect PIDs first,
# drop this script and its ancestors, then signal what is left.

set -uo pipefail

self=$$
patterns=('make watch' 'scripts/watch.sh' 'air -c .air.toml' 'bin/air/lapin' 'node_modules/.bin/vite' 'copy-excalidraw-assets.mjs && vite')

ancestors=()
pid=$self
while [[ -n $pid && $pid != 1 ]]; do
  ancestors+=("$pid")
  pid=$(ps -o ppid= -p "$pid" 2>/dev/null | tr -d ' ')
done

targets=()
for pattern in "${patterns[@]}"; do
  while read -r candidate; do
    [[ -z $candidate ]] && continue
    skip=false
    for ancestor in "${ancestors[@]}"; do
      [[ $candidate == "$ancestor" ]] && skip=true && break
    done
    $skip || targets+=("$candidate")
  done < <(pgrep -f -- "$pattern" 2>/dev/null)
done

if [[ ${#targets[@]} -eq 0 ]]; then
  echo 'no development server was running'
else
  printf 'stopping %s\n' "${targets[*]}"
  kill -TERM "${targets[@]}" 2>/dev/null
  for _ in $(seq 1 15); do
    remaining=0
    for target in "${targets[@]}"; do
      kill -0 "$target" 2>/dev/null && remaining=$((remaining + 1))
    done
    [[ $remaining -eq 0 ]] && break
    sleep 1
  done
  kill -KILL "${targets[@]}" 2>/dev/null
fi

for port in 5173 8080; do
  for _ in $(seq 1 15); do
    curl -s -o /dev/null --max-time 1 "http://127.0.0.1:$port/" || break
    sleep 1
  done
done
echo 'ports 5173 and 8080 are free'
