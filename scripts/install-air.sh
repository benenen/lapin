#!/bin/sh

set -eu

version=${1:-}
destination=${2:-}

case "$version" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *)
    echo "usage: $0 vMAJOR.MINOR.PATCH DESTINATION" >&2
    exit 2
    ;;
esac

if [ -z "$destination" ]; then
  echo "usage: $0 vMAJOR.MINOR.PATCH DESTINATION" >&2
  exit 2
fi

case "$destination" in
  "$PWD"/bin/*) ;;
  *)
    echo "refusing to install Air outside $PWD/bin" >&2
    exit 2
    ;;
esac

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *)
    echo "unsupported operating system: $(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  i386 | i686) arch=386 ;;
  *)
    echo "unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

release=${version#v}
asset="air_${release}_${os}_${arch}"
base_url="https://github.com/air-verse/air/releases/download/${version}"
download="${destination}.download.$$"

case "${version}:${os}:${arch}" in
  v1.67.3:darwin:amd64) expected=bf678b41f5dc12642f73a1e1d5f67dd1eb10817546bd960375770b1675080b3c ;;
  v1.67.3:darwin:arm64) expected=34dba56d365b8514d8caee96a416d21290855686a715a90cb717e2d178d3c28f ;;
  v1.67.3:linux:386) expected=8790565cd022df63f8f18e62b9f1bd3da2c33fba60e6e7a4eb4c61f6e0e344a3 ;;
  v1.67.3:linux:amd64) expected=ff4febd6c2ff76027535f8fc0a4a4189428f9cef84702147da91c21e1cb72863 ;;
  v1.67.3:linux:arm64) expected=6d0b5536caa8a8d352a2394ad04ae6248fe177fbb7f6246d85bce242b4176242 ;;
  *)
    echo "no pinned checksum for Air ${version} on ${os}/${arch}" >&2
    exit 1
    ;;
esac

cleanup() {
  rm -f -- "$download"
}
trap cleanup EXIT HUP INT TERM

mkdir -p -- "$(dirname "$destination")"

checksum() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{ print $1 }'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{ print $1 }'
  else
    echo "sha256sum or shasum is required" >&2
    exit 1
  fi
}

if [ -x "$destination" ] && [ "$(checksum "$destination")" = "$expected" ]; then
  exit 0
fi

curl --fail --location --silent --show-error --retry 3 --retry-delay 1 \
  "${base_url}/${asset}" --output "$download"

actual=$(checksum "$download")

if [ "$actual" != "$expected" ]; then
  echo "checksum mismatch for $asset" >&2
  exit 1
fi

install -m 0755 "$download" "$destination"
