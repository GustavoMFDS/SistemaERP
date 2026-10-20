# Observability and alerting runbook

SistemaEmGo already exposes:

- `/metrics` with Prometheus metrics;
- `/health/live` for process liveness;
- `/health/ready` for PostgreSQL/Redis readiness.

This repository now includes a small observability stack intended for local validation and pilot drills:

- `docker-compose.observability.yml`
- `ops/monitoring/prometheus.yml`
- `ops/monitoring/blackbox.yml`
- `ops/monitoring/alerts.yml`

## Start the validation stack

Run the API on port 8080, then:

```bash
docker compose -f docker-compose.observability.yml up -d
```

Prometheus is available on port `9090` by default.

The stack scrapes:

- `http://host.docker.internal:8080/metrics`
- `http://host.docker.internal:8080/health/ready` through Blackbox Exporter.

Linux Docker uses `host-gateway` in the compose file so the containers can reach the host API.

## Baseline alerts

The repository ships four baseline rules:

1. **SistemaEmGoMetricsUnavailable**
   - critical;
   - fires when the API metrics target is down for 2 minutes.

2. **SistemaEmGoReadinessFailed**
   - critical;
   - fires when `/health/ready` fails for 1 minute;
   - PostgreSQL and Redis are the first dependencies to inspect.

3. **SistemaEmGoHighHTTP5xxRate**
   - warning;
   - fires when more than 2% of HTTP requests are 5xx for 10 minutes.

4. **SistemaEmGoHighHTTPLatency**
   - warning;
   - fires when HTTP p95 latency is above 1 second for 10 minutes.

These are baseline values for pilot validation, not permanent business SLOs. Tune them after observing real traffic.

## Production metrics protection

In staging/production, `/metrics` must remain protected with the configured metrics bearer token or basic authentication.

The repository compose overlay intentionally targets a local API without credentials. Do not copy that scrape configuration verbatim to production.

For production, configure the actual monitoring platform to authenticate using:

- `METRICS_BEARER_TOKEN`; or
- `METRICS_BASIC_USER` and `METRICS_BASIC_PASS`.

Never commit production monitoring credentials.

## Recommended production signals

At minimum, retain and dashboard:

- `sistemaemgo_http_requests_total`;
- `sistemaemgo_http_request_duration_seconds`;
- `sistemaemgo_http_in_flight_requests`;
- `sistemaemgo_sales_total`;
- `sistemaemgo_sales_revenue_total_brl`;
- `sistemaemgo_sales_cancellations_total`;
- `sistemaemgo_sales_duration_seconds`;
- `sistemaemgo_inventory_low_stock_alerts_total`;
- `sistemaemgo_db_query_duration_seconds`;
- `sistemaemgo_cache_hits_total`;
- `sistemaemgo_cache_misses_total`;
- external readiness probe success.

Avoid putting customer or payment identifiers into metric labels.

## Pilot acceptance

Before a real-store pilot:

1. deploy the API in the production-like environment;
2. configure the real monitoring system with authenticated `/metrics` access;
3. continuously probe `/health/ready`;
4. deliberately stop PostgreSQL or Redis in a safe test environment and confirm the readiness alert fires;
5. simulate an API outage and confirm the availability alert fires;
6. route warning/critical alerts to an actual responsible person;
7. record screenshots/exported alert events in the pilot evidence pack.

A config file in the repository is preparation only. The monitoring/alerting production-readiness item is closed only after the alerts are proven end-to-end in the target environment.

## Validation in CI

The main CI workflow validates:

- Docker Compose syntax for `docker-compose.observability.yml`;
- Prometheus configuration with `promtool check config`;
- alert rules with `promtool check rules`.

This prevents invalid observability configuration from being merged when CI runners are available.
