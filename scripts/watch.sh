#!/usr/bin/env bash

set -euo pipefail

air_bin=${1:-}
admin_bootstrap_bin=${2:-}

admin_email=${ADMIN_EMAIL-}
admin_password=${ADMIN_PASSWORD-}
http_address=${HTTP_ADDR-}
app_environment=${APP_ENV:-development}
database_url=${DATABASE_URL-}
hashid_salt=${HASHID_SALT-}
secure_cookies=${SECURE_COOKIES-}
trusted_proxy_cidrs=${TRUSTED_PROXY_CIDRS-}
using_default_admin=false
make_metadata="${MAKEFLAGS-} ${MAKEOVERRIDES-} ${MFLAGS-} ${GNUMAKEFLAGS-}"

if [[ $make_metadata == *ADMIN_EMAIL* || $make_metadata == *ADMIN_PASSWORD* ]]; then
  echo 'administrator credentials must be provided through the environment, not Make command-line variables' >&2
  exit 2
fi
make_metadata=
unset MAKEFLAGS MAKEOVERRIDES MFLAGS GNUMAKEFLAGS

for environment_name in "${!PG@}"; do
  echo 'make watch requires PostgreSQL settings in DATABASE_URL and rejects PG* environment variables' >&2
  exit 2
done

if [[ -z "$admin_email" && -z "$admin_password" ]]; then
  # Public local-development credentials. Docker and production do not use them.
  admin_email='admin@localhost'
  admin_password='admin12345678'
elif [[ -z "$admin_email" || -z "$admin_password" ]]; then
  echo 'ADMIN_EMAIL and ADMIN_PASSWORD must be set together' >&2
  exit 2
fi

if [[ "$admin_email" == 'admin@localhost' && "$admin_password" == 'admin12345678' ]]; then
  using_default_admin=true
fi

if [[ -z "$http_address" ]]; then
  http_address='127.0.0.1:8080'
fi

is_loopback_http_address() {
  [[ $1 =~ ^(127\.0\.0\.1|localhost):[0-9]+$ || $1 =~ ^\[::1\]:[0-9]+$ ]]
}

is_loopback_database_url() {
  [[ $1 =~ ^postgres(ql)?://([^/@]+@)?(127\.0\.0\.1|localhost|\[::1\])(:[0-9]+)?/ ]]
}

if [[ "$using_default_admin" == true ]]; then
  if [[ ${app_environment,,} != development ]]; then
    echo 'default watch administrator is only allowed in development' >&2
    exit 2
  fi
  if ! is_loopback_http_address "$http_address"; then
    echo 'default watch administrator requires a loopback HTTP_ADDR' >&2
    exit 2
  fi
  if ! is_loopback_database_url "$database_url"; then
    echo 'default watch administrator requires a loopback PostgreSQL DATABASE_URL' >&2
    exit 2
  fi
fi

# Prevent backend-only configuration from reaching npm, installers, or build processes.
unset ADMIN_EMAIL ADMIN_PASSWORD HTTP_ADDR DATABASE_URL HASHID_SALT APP_ENV SECURE_COOKIES TRUSTED_PROXY_CIDRS

make web-build air watch-admin-bootstrap

if [[ -z "$air_bin" || ! -x "$air_bin" || -z "$admin_bootstrap_bin" || ! -x "$admin_bootstrap_bin" ]]; then
  echo "usage: $0 AIR_BINARY ADMIN_BOOTSTRAP_BINARY" >&2
  exit 2
fi

DATABASE_URL="$database_url" \
HASHID_SALT="$hashid_salt" \
APP_ENV="$app_environment" \
SECURE_COOKIES="$secure_cookies" \
TRUSTED_PROXY_CIDRS="$trusted_proxy_cidrs" \
HTTP_ADDR="$http_address" \
ADMIN_EMAIL="$admin_email" \
ADMIN_PASSWORD="$admin_password" \
  "$admin_bootstrap_bin"
admin_email=
admin_password=

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

DATABASE_URL="$database_url" \
HASHID_SALT="$hashid_salt" \
APP_ENV="$app_environment" \
SECURE_COOKIES="$secure_cookies" \
TRUSTED_PROXY_CIDRS="$trusted_proxy_cidrs" \
HTTP_ADDR="$http_address" \
  "$air_bin" -c .air.toml &
air_pid=$!
npm --prefix web run dev &
vite_pid=$!

wait -n "$air_pid" "$vite_pid"
