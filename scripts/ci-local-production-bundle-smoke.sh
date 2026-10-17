#!/usr/bin/env bash
set -euo pipefail

# A real server + built React bundle on the same HTTP origin.
# This is a disposable CI fixture; LAN and fiscal emission remain disabled.
if [[ "${GITHUB_ACTIONS:-}" != "true" || "${CI:-}" != "true" ]]; then
  echo "Refusing: this smoke is only for disposable GitHub CI." >&2
  exit 2
fi
if [[ ! -s web/dist/index.html || ! -d web/dist/assets ]]; then
  echo "Compiled frontend missing; build web/dist before this smoke." >&2
  exit 2
fi

tmp="$(mktemp -d)"
server_pid=""
cleanup() {
  if [[ -n "$server_pid" ]]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -rf -- "$tmp"
}
trap cleanup EXIT

(
  cd backend
  go build -trimpath -o "$tmp/esterium-api" ./cmd/api
)
export APP_ENV=test
export FISCAL_PROVIDER=disabled
export NFCE_SEFAZ_PRODUCTION_ENABLED=false
export NFCE_SEFAZ_HOMOLOGATION_ENABLED=false
export HTTP_ADDR=127.0.0.1:18080
export ESTERIUM_WEB_DIR="$PWD/web/dist"
export DATABASE_URL='postgres://sistemaemgo:sistemaemgo@127.0.0.1:5433/sistemaemgo?sslmode=disable'
export REDIS_ADDR=127.0.0.1:6379
export JWT_SECRET='temporary-ci-only-local-bundle-smoke-secret-2026'
export OTEL_ENABLED=false
export CORS_ALLOWED_ORIGINS=http://127.0.0.1:18080
"$tmp/esterium-api" >"$tmp/api.log" 2>&1 &
server_pid=$!

ready=""
for attempt in $(seq 1 40); do
  if ! kill -0 "$server_pid" 2>/dev/null; then
    echo 'Backend failed to start:' >&2
    tail -n 100 "$tmp/api.log" >&2
    exit 1
  fi
  if curl --fail --silent --max-time 2 http://127.0.0.1:18080/health/ready >/dev/null; then
    ready=1
    break
  fi
  sleep 1
done
if [[ "$ready" != "1" ]]; then
  echo 'Go backend never became healthy.' >&2
  tail -n 100 "$tmp/api.log" >&2
  exit 1
fi

# The frontend must be served by Go, not Vite; API remains at the same origin.
origin=http://127.0.0.1:18080
curl --fail --silent --show-error -D "$tmp/index.headers" -o "$tmp/index.body" "$origin/"
grep -qi '<html' "$tmp/index.body"
grep -qi "content-security-policy:.*script-src 'self'" "$tmp/index.headers"
grep -qi 'cache-control: no-store' "$tmp/index.headers"

curl --fail --silent --show-error -H 'Accept: text/html' -o "$tmp/spa.body" "$origin/products"
cmp -s "$tmp/index.body" "$tmp/spa.body" || {
  echo 'Built SPA history route did not return same index.' >&2
  exit 1
}

js_path="$(find web/dist/assets -maxdepth 2 -type f -name '*.js' -print -quit)"
if [[ -z "$js_path" ]]; then
  echo 'Compiled bundle has no JavaScript asset.' >&2
  exit 1
fi
asset_url="/assets/$(basename "$js_path")"
curl --fail --silent --show-error -D "$tmp/js.headers" -o "$tmp/js.body" "$origin$asset_url"
test -s "$tmp/js.body"
grep -qi 'cache-control: public, max-age=31536000, immutable' "$tmp/js.headers"

missing_status="$(curl --silent --show-error -o "$tmp/missing.body" -w '%{http_code}' "$origin/assets/non-existent-ci-build.js")"
[[ "$missing_status" == 404 ]] || {
  echo "Missing JS was not a 404 (got $missing_status)." >&2
  exit 1
}
unknown_status="$(curl --silent --show-error -H 'Accept: text/html' -o "$tmp/api.body" -w '%{http_code}' "$origin/api/v1/ci-no-such-route")"
[[ "$unknown_status" != 200 ]] || {
  echo 'API route was replaced by SPA index.' >&2
  exit 1
}
if grep -qi '<html' "$tmp/api.body"; then
  echo 'Backend returned SPA HTML for API request.' >&2
  exit 1
fi

echo 'PASS: compiled Vite frontend, SPA history, static JS, security headers and API all served from one Go origin.'
