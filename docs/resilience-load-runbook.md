# Resilience and load-test runbook

This runbook covers two repository tools used to prepare a real-store pilot:

- `backend/cmd/loadcheck`: bounded HTTP load smoke test with latency/error thresholds;
- `scripts/dependency-failure-drill.sh`: controlled PostgreSQL/Redis outage drill for `/health/ready`.

These tools prepare repeatable tests. They do not replace execution against the target environment.

## HTTP load smoke

The load checker uses only the Go standard library and is built as part of the backend module.

Example against readiness:

```bash
cd backend

go run ./cmd/loadcheck \
  -url https://staging.example.test/health/ready \
  -duration 30s \
  -concurrency 16 \
  -timeout 3s \
  -max-error-rate 0.01 \
  -max-p95 500ms
```

It reports:

- total requests;
- failed requests and error rate;
- requests per second;
- p50/p95/p99/max latency;
- HTTP status distribution.

The command exits non-zero when the configured error-rate or p95 threshold is exceeded.

### Authenticated read endpoints

Headers are repeatable:

```bash
go run ./cmd/loadcheck \
  -url https://staging.example.test/api/v1/products/ \
  -header 'Authorization: Bearer REDACTED' \
  -duration 60s \
  -concurrency 20
```

Do not commit tokens or put long-lived production credentials into shell scripts.

### Write safety

Only GET and HEAD are allowed by default.

POST/PUT/PATCH/DELETE require the explicit `-allow-writes` flag. This is deliberate: load tests against write endpoints can create sales, inventory movements, financial records, or other irreversible business effects.

Use write-load testing only in an isolated dataset/environment approved for destructive testing.

### TLS

`-insecure` exists only for local/internal-certificate drills. Production-like validation should use valid trust roots and normal certificate verification.

## Dependency failure drill

The failure drill temporarily stops the local Compose `redis` and `db` services and verifies that:

1. baseline `/health/ready` returns 200;
2. Redis outage makes readiness fail;
3. Redis restart restores readiness;
4. PostgreSQL outage makes readiness fail;
5. PostgreSQL restart restores readiness.

It automatically attempts to restart both dependencies on exit.

Run:

```bash
export ALLOW_DEPENDENCY_FAILURE_DRILL=1
export API_BASE_URL=http://127.0.0.1:8080

bash scripts/dependency-failure-drill.sh
```

Evidence is written under `artifacts/resilience/`.

### Safety controls

The script:

- requires explicit `ALLOW_DEPENDENCY_FAILURE_DRILL=1`;
- defaults to `docker-compose.yml`;
- refuses non-default Docker contexts unless `ALLOW_REMOTE_DOCKER_CONTEXT=1` is also set;
- stops services but does not remove volumes;
- restores PostgreSQL and Redis in an EXIT trap.

Do not set `ALLOW_REMOTE_DOCKER_CONTEXT=1` unless the remote target is explicitly approved for a destructive availability drill.

## Pilot test sequence

Recommended order in the production-like pilot environment:

1. confirm backup/restore drill passes;
2. confirm monitoring is ingesting metrics and readiness probes;
3. run a low-concurrency read load smoke;
4. establish baseline p95/error rate;
5. increase concurrency gradually while observing DB/Redis/API metrics;
6. run Redis failure drill in the approved non-production target;
7. confirm readiness and alert firing;
8. run PostgreSQL failure drill;
9. confirm readiness and alert firing;
10. verify recovery and data integrity;
11. retain command output, monitoring graphs, alert events, and evidence files.

## Acceptance guidance

Do not copy the example thresholds blindly into a production SLO.

For the pilot, define thresholds from the actual hardware/network and business flow. At minimum:

- no unexplained 5xx;
- no data-integrity errors;
- p95 within the operator-acceptable response time;
- readiness fails promptly when a mandatory dependency is unavailable;
- readiness returns only after dependency recovery;
- alerts reach a responsible person.

The repository implementation closes the tooling gap. The production-readiness item remains environment-dependent until these drills are executed and evidenced on the real target.
