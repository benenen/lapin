#!/usr/bin/env bash

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf -- "$test_root"' EXIT

fake_bin="$test_root/bin"
mkdir -p "$fake_bin"

cat >"$fake_bin/air" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n' \
  "${ADMIN_EMAIL-unset}" "${ADMIN_PASSWORD-unset}" "${HTTP_ADDR-unset}" \
  "${DATABASE_URL-unset}" "${HASHID_SALT-unset}" "${APP_ENV-unset}" \
  "${SECURE_COOKIES-unset}" "${TRUSTED_PROXY_CIDRS-unset}" "${ASSET_DIR-unset}" \
  >"$TEST_AIR_ENV"
while [[ ! -f "$TEST_NPM_ENV" ]]; do
  sleep 0.01
done
EOF

cat >"$fake_bin/bootstrap-admin" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n' \
  "${ADMIN_EMAIL-unset}" "${ADMIN_PASSWORD-unset}" "${DATABASE_URL-unset}" \
  "${HASHID_SALT-unset}" "${APP_ENV-unset}" "${SECURE_COOKIES-unset}" \
  "${TRUSTED_PROXY_CIDRS-unset}" "${ASSET_DIR-unset}" \
  >"$TEST_BOOTSTRAP_ENV"
EOF

cat >"$fake_bin/npm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n' \
  "${ADMIN_EMAIL-unset}" "${ADMIN_PASSWORD-unset}" "${DATABASE_URL-unset}" \
  "${HASHID_SALT-unset}" "${APP_ENV-unset}" "${SECURE_COOKIES-unset}" \
  "${TRUSTED_PROXY_CIDRS-unset}" "${ASSET_DIR-unset}" \
  >"$TEST_NPM_ENV"
sleep 1
EOF

cat >"$fake_bin/make" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n' \
  "${ADMIN_EMAIL-unset}" "${ADMIN_PASSWORD-unset}" \
  "${DATABASE_URL-unset}" "${HASHID_SALT-unset}" "${APP_ENV-unset}" \
  "${SECURE_COOKIES-unset}" "${TRUSTED_PROXY_CIDRS-unset}" "${ASSET_DIR-unset}" \
  "${MAKEFLAGS-unset}" "${MAKEOVERRIDES-unset}" "${MFLAGS-unset}" "${GNUMAKEFLAGS-unset}" \
  >"$TEST_PREPARE_ENV"
EOF

chmod +x "$fake_bin/air" "$fake_bin/bootstrap-admin" "$fake_bin/npm" "$fake_bin/make"

run_watch() {
  local name=$1
  shift
  TEST_AIR_ENV="$test_root/$name-air.env" \
  TEST_BOOTSTRAP_ENV="$test_root/$name-bootstrap.env" \
  TEST_NPM_ENV="$test_root/$name-npm.env" \
  TEST_PREPARE_ENV="$test_root/$name-prepare.env" \
  PATH="$fake_bin:$PATH" \
    "$@" bash "$repo_root/scripts/watch.sh" "$fake_bin/air" "$fake_bin/bootstrap-admin"
}

assert_environment() {
  local path=$1
  shift
  mapfile -t values <"$path"
  [[ ${#values[@]} -eq $# ]]
  local index=0
  for expected in "$@"; do
    [[ ${values[$index]-} == "$expected" ]]
    index=$((index + 1))
  done
}

local_database='postgres://postgres:postgres@127.0.0.1:5433/lapin_test?sslmode=disable'
private_hashid_salt='test-private-hashid-salt'
asset_directory="$test_root/assets"

run_watch default env -u ADMIN_EMAIL -u ADMIN_PASSWORD -u HTTP_ADDR \
  DATABASE_URL="$local_database" HASHID_SALT="$private_hashid_salt" APP_ENV='development' \
  SECURE_COOKIES='false' TRUSTED_PROXY_CIDRS='127.0.0.1/32' ASSET_DIR="$asset_directory"
assert_environment "$test_root/default-bootstrap.env" \
  'admin@localhost' 'admin12345678' "$local_database" "$private_hashid_salt" 'development' 'false' '127.0.0.1/32' "$asset_directory"
assert_environment "$test_root/default-prepare.env" \
  'unset' 'unset' 'unset' 'unset' 'unset' 'unset' 'unset' 'unset' 'unset' 'unset' 'unset' 'unset'
assert_environment "$test_root/default-air.env" \
  'unset' 'unset' '127.0.0.1:8080' "$local_database" "$private_hashid_salt" 'development' 'false' '127.0.0.1/32' "$asset_directory"
assert_environment "$test_root/default-npm.env" \
  'unset' 'unset' 'unset' 'unset' 'unset' 'unset' 'unset' 'unset'

run_watch override env ADMIN_EMAIL='owner@example.com' ADMIN_PASSWORD='custom-development-password' \
  DATABASE_URL="$local_database" HASHID_SALT="$private_hashid_salt" APP_ENV='development' \
  SECURE_COOKIES='false' TRUSTED_PROXY_CIDRS='127.0.0.1/32' ASSET_DIR="$asset_directory"
assert_environment "$test_root/override-bootstrap.env" \
  'owner@example.com' 'custom-development-password' "$local_database" "$private_hashid_salt" 'development' 'false' '127.0.0.1/32' "$asset_directory"
assert_environment "$test_root/override-prepare.env" \
  'unset' 'unset' 'unset' 'unset' 'unset' 'unset' 'unset' 'unset' 'unset' 'unset' 'unset' 'unset'
assert_environment "$test_root/override-air.env" \
  'unset' 'unset' '127.0.0.1:8080' "$local_database" "$private_hashid_salt" 'development' 'false' '127.0.0.1/32' "$asset_directory"
assert_environment "$test_root/override-npm.env" \
  'unset' 'unset' 'unset' 'unset' 'unset' 'unset' 'unset' 'unset'

assert_rejected() {
  local name=$1
  local expected=$2
  shift 2
  set +e
  output=$(TEST_AIR_ENV="$test_root/$name-air.env" \
    TEST_BOOTSTRAP_ENV="$test_root/$name-bootstrap.env" \
    TEST_NPM_ENV="$test_root/$name-npm.env" \
    TEST_PREPARE_ENV="$test_root/$name-prepare.env" \
    PATH="$fake_bin:$PATH" \
      "$@" bash "$repo_root/scripts/watch.sh" "$fake_bin/air" "$fake_bin/bootstrap-admin" 2>&1)
  status=$?
  set -e
  [[ $status -eq 2 ]]
  [[ $output == *"$expected"* ]]
  [[ ! -e "$test_root/$name-air.env" ]]
  [[ ! -e "$test_root/$name-bootstrap.env" ]]
  [[ ! -e "$test_root/$name-npm.env" ]]
  [[ ! -e "$test_root/$name-prepare.env" ]]
}

assert_rejected partial 'ADMIN_EMAIL and ADMIN_PASSWORD must be set together' \
  env -u ADMIN_PASSWORD ADMIN_EMAIL='owner@example.com' DATABASE_URL="$local_database"
assert_rejected production 'default watch administrator is only allowed in development' \
  env -u ADMIN_EMAIL -u ADMIN_PASSWORD APP_ENV='production' DATABASE_URL="$local_database"
assert_rejected public-http 'default watch administrator requires a loopback HTTP_ADDR' \
  env -u ADMIN_EMAIL -u ADMIN_PASSWORD HTTP_ADDR='0.0.0.0:8080' DATABASE_URL="$local_database"
assert_rejected explicit-public-pair 'default watch administrator requires a loopback HTTP_ADDR' \
  env ADMIN_EMAIL='admin@localhost' ADMIN_PASSWORD='admin12345678' HTTP_ADDR='0.0.0.0:8080' DATABASE_URL="$local_database"
assert_rejected remote-database 'default watch administrator requires a loopback PostgreSQL DATABASE_URL' \
  env -u ADMIN_EMAIL -u ADMIN_PASSWORD DATABASE_URL='postgres://user:password@db.example.com/lapin'
assert_rejected make-command-secret 'administrator credentials must be provided through the environment, not Make command-line variables' \
  env ADMIN_EMAIL='owner@example.com' ADMIN_PASSWORD='custom-development-password' \
  MAKEFLAGS=' -- ADMIN_EMAIL=owner@example.com ADMIN_PASSWORD=custom-development-password' \
  MAKEOVERRIDES='ADMIN_EMAIL=owner@example.com ADMIN_PASSWORD=custom-development-password'
assert_rejected postgres-environment 'make watch requires PostgreSQL settings in DATABASE_URL and rejects PG* environment variables' \
  env -u ADMIN_EMAIL -u ADMIN_PASSWORD DATABASE_URL="$local_database" PGPASSWORD='postgres-environment-secret'
[[ $output != *'postgres-environment-secret'* ]]

echo 'watch development administrator tests passed'
