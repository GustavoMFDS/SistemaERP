#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'ERROR: %s\n' "$1" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

require_cmd docker
require_cmd curl
require_cmd date
require_cmd mkdir

if [ "${ALLOW_DEPENDENCY_FAILURE_DRILL:-}" != "1" ]; then
  fail "set ALLOW_DEPENDENCY_FAILURE_DRILL=1 to acknowledge that PostgreSQL and Redis will be stopped temporarily"
fi

context="$(docker context show 2>/dev/null || true)"
if [ "$context" != "default" ] && [ "${ALLOW_REMOTE_DOCKER_CONTEXT:-0}" != "1" ]; then
  fail "docker context '$context' is not 'default'; set ALLOW_REMOTE_DOCKER_CONTEXT=1 only for an explicitly approved drill target"
fi

compose_file="${COMPOSE_FILE:-docker-compose.yml}"
api_base="${API_BASE_URL:-http://127.0.0.1:8080}"
api_base="${api_base%/}"
evidence_dir="${EVIDENCE_DIR:-artifacts/resilience}"
mkdir -p "$evidence_dir"
evidence_file="$evidence_dir/dependency-failure-$(date -u +%Y%m%dT%H%M%SZ).txt"

compose() {
  docker compose -f "$compose_file" "$@"
}

http_status() {
  curl --silent --show-error --output /tmp/sistemaemgo-readiness-drill.json \
    --write-out '%{http_code}' \
    --max-time 3 \
    "$api_base/health/ready" || true
}

wait_status() {
  local expected="$1"
  local mode="$2"
  local attempts="${3:-30}"
  local status
  for _ in $(seq 1 "$attempts"); do
    status="$(http_status)"
    if [ "$mode" = "eq" ] && [ "$status" = "$expected" ]; then
      return 0
    fi
    if [ "$mode" = "ne" ] && [ "$status" != "$expected" ]; then
      return 0
    fi
    sleep 2
  done
  printf 'last readiness response: HTTP %s body=%s\n' "${status:-unknown}" "$(cat /tmp/sistemaemgo-readiness-drill.json 2>/dev/null || true)" >&2
  return 1
}

restore_dependencies() {
  compose start db redis >/dev/null 2>&1 || true
}
trap restore_dependencies EXIT

printf 'Checking healthy baseline...\n'
wait_status 200 eq 30 || fail "baseline /health/ready did not become HTTP 200"

printf 'Stopping Redis...\n'
compose stop redis >/dev/null
wait_status 200 ne 15 || fail "/health/ready stayed HTTP 200 after Redis was stopped"
printf 'PASS redis outage detected: HTTP %s\n' "$(http_status)" | tee -a "$evidence_file"

printf 'Restoring Redis...\n'
compose start redis >/dev/null
wait_status 200 eq 30 || fail "/health/ready did not recover after Redis restart"
printf 'PASS redis recovery detected\n' | tee -a "$evidence_file"

printf 'Stopping PostgreSQL...\n'
compose stop db >/dev/null
wait_status 200 ne 15 || fail "/health/ready stayed HTTP 200 after PostgreSQL was stopped"
printf 'PASS postgres outage detected: HTTP %s\n' "$(http_status)" | tee -a "$evidence_file"

printf 'Restoring PostgreSQL...\n'
compose start db >/dev/null
wait_status 200 eq 30 || fail "/health/ready did not recover after PostgreSQL restart"
printf 'PASS postgres recovery detected\n' | tee -a "$evidence_file"

trap - EXIT
restore_dependencies

{
  printf 'UTC completed: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf 'Compose file: %s\n' "$compose_file"
  printf 'API base: %s\n' "$api_base"
  printf 'Docker context: %s\n' "$context"
  printf 'Result: PASS\n'
} >> "$evidence_file"

printf '\nPASS: dependency failure drill completed. Evidence: %s\n' "$evidence_file"
