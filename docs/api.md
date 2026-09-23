# API

Base URL: `/api/v1`

All normal API errors use JSON:

```json
{
  "code": "validation_error",
  "message": "friendly message",
  "details": {},
  "request_id": "..."
}
```

Common codes: `validation_error`, `authentication_error`, `authorization_error`, `conflict`, `not_found`, `rate_limit`, and `internal_error`.

## Authentication

### POST `/auth/login`

Request:

```json
{ "email": "admin@sistema.local", "password": "admin123" }
```

Response:

```json
{
  "token": {
    "access_token": "...",
    "token_type": "Bearer",
    "expires_in": 900
  },
  "user": { "id": "...", "email": "...", "name": "...", "tenant_id": "...", "roles": ["admin"] }
}
```

The refresh token is set only as an `HttpOnly`, `SameSite=Strict` cookie when available. The frontend keeps the access token in memory and should not log tokens.

Access tokens are short-lived, and protected requests recheck current user status and tenant membership. A deactivated user or a user removed from the token's tenant is rejected before the token's original expiration time. Effective RBAC permissions are loaded from the tenant-scoped repository on each protected request.

### POST `/auth/refresh`

Accepts the refresh token only from the `__Host-refresh_token` HttpOnly cookie. JSON body refresh tokens are no longer accepted.

Refresh tokens are rotated; a consumed token cannot be reused. A rejected/invalid refresh also expires the browser refresh cookie.

Response:

```json
{ "access_token": "...", "token_type": "Bearer", "expires_in": 900 }
```

Cookie-based refresh/logout requests validate `Origin` or `Referer` against configured allowed origins in staging/production.

### POST `/auth/logout`

Revokes the current refresh token and only then clears the refresh cookie. If server-side revocation cannot be persisted (for example, Redis is unavailable), the endpoint returns `503 service_unavailable` and does not clear the cookie or claim that logout completed.

### GET `/auth/me`

Response:

```json
{ "id": "...", "email": "...", "name": "...", "tenant_id": "...", "roles": ["admin"] }
```

## Products

### GET `/products?query=...&limit=...&offset=...`

Response:

```json
{ "items": [{ "id": "...", "sku": "SKU001", "name": "Coca 2L", "price_cash": 10.9, "active": true }], "total": 1 }
```

### POST `/products`

```json
{
  "category_id": null,
  "sku": "SKU001",
  "barcode": "789...",
  "name": "Coca 2L",
  "description": "",
  "unit": "UN",
  "cost_price": 7,
  "price_cash": 10.9,
  "promo_price": null,
  "min_stock": 5,
  "active": true
}
```

Money values accept at most 2 decimal places. Quantity values accept at most 3 decimal places.

## Inventory

### GET `/inventory/low-stock`

Returns products with `qty_on_hand <= min_stock`.

### POST `/inventory/adjust`

```json
{ "product_id": "...", "delta": 10, "reason": "Entrada por compra", "type": "purchase" }
```

## Cash / PDV

### POST `/cash/sessions/open`

Opens the default cash register session for the authenticated tenant. Only one open session is allowed per tenant/register.

### POST `/cash/sessions/{id}/close`

Request:

```json
{ "closing_amount": 148.75, "notes": "Fechamento do turno" }
```

The backend computes expected physical cash as opening cash plus cash-method payments from sales that remain finalized, persists the declared closing amount and difference, and audits both open/close events.

Response:

```json
{
  "status": "closed",
  "expected_cash": 150,
  "closing_amount": 148.75,
  "closing_difference": -1.25
}
```

## Sales / POS

### POST `/sales`

`Idempotency-Key` is required. Requests without it are rejected with `422 validation_error`.

Request:

```json
{
  "cash_session_id": "...",
  "customer_id": null,
  "discount_value": 0,
  "items": [
    { "product_id": "...", "qty": 2, "discount_value": 0 }
  ],
  "payments": [
    { "method": "pix", "amount": 21.8 }
  ]
}
```

`unit_price` is deprecated and ignored if old clients still send it. The backend always loads the product, applies `promo_price` when present, validates stock, and calculates subtotal/total server-side.

Response:

```json
{ "id": "...", "status": "finalized", "total": 21.8, "replayed": false }
```

Idempotency behavior:

- New key: process normally and store `request_hash`.
- Same key and same hash: return the stored result with `replayed: true`.
- Same key and different hash: return `409 conflict`.

### POST `/sales/{id}/cancel`

```json
{ "reason": "Erro de operacao" }
```

## Finance

### GET `/finance/dashboard?from=2026-01-01&to=2026-01-31`

Returns aggregated ledger totals for the period.

## Fiscal

### POST `/fiscal/nfe/xml`

```json
{ "sale_id": "..." }
```

Response:

```json
{ "invoice_id": "...", "xml_file_id": "..." }
```

### GET `/fiscal/nfe/xml/{id}/download`

Returns `application/xml`.

## Metrics

`GET /metrics` is public only in development unless metrics credentials are configured. Staging/production requires either `METRICS_BEARER_TOKEN` or `METRICS_BASIC_USER`/`METRICS_BASIC_PASS`.

## Rate limiting

Sensitive endpoints return `429 rate_limit` after configurable per-minute limits:

- `/auth/login`
- `/auth/refresh`
- `/auth/logout`
- `/sales`
- Fiscal XML generation/download endpoints

Login has layered limits by IP, normalized identifier, and IP plus identifier. Redis counters use one atomic Lua operation that increments and assigns TTL. In staging/production, a Redis limiter failure returns `503 service_unavailable` instead of silently falling back to a per-process limiter; local fallback is development/test-only. Error responses remain generic and do not reveal whether an email exists.

## Privacy / LGPD Operations

All privacy routes require JWT auth and RBAC permissions `privacy:read` or `privacy:write`.

### POST `/privacy/requests`

```json
{
  "subject_type": "customer",
  "subject_id": "...",
  "requester_email": "titular@example.com",
  "request_type": "export",
  "notes": "Solicitacao via canal DPO"
}
```

Supported `subject_type`: `customer`, `user`. Supported `request_type`: `export`, `correction`, `anonymization`, `deletion`, `blocking`.

### GET `/privacy/requests`

Returns tenant-scoped data subject requests.

### GET `/privacy/requests/{id}`

Returns one tenant-scoped request.

### PUT `/privacy/requests/{id}/status`

```json
{ "status": "in_progress", "notes": "Validando identidade do titular" }
```

Supported status values: `open`, `in_progress`, `completed`, `rejected`, `cancelled`.

Allowed transitions are `open -> in_progress`, `open -> rejected`, `open -> cancelled`, `in_progress -> completed`, `in_progress -> rejected`, and `in_progress -> cancelled`. Terminal statuses (`completed`, `rejected`, `cancelled`) are immutable through this endpoint, including same-status updates. Invalid transitions return standardized `409 conflict`.

### POST `/privacy/requests/{id}/export`

Exports available personal data for `customer` or `user` subjects. Fiscal/accounting records are not deleted by this operation.

### POST `/privacy/requests/{id}/anonymize`

Anonymizes supported personal fields while preserving fiscal/accounting record integrity.

### POST `/privacy/requests/{id}/block`

Inactivates supported subjects where applicable. Currently user accounts can be blocked.

### POST `/privacy/consents`

```json
{
  "subject_type": "customer",
  "subject_id": "...",
  "purpose": "marketing",
  "consent_text_version": "v1",
  "source": "web"
}
```

### GET `/privacy/consents`

Returns tenant-scoped consent records.

### POST `/privacy/consents/{id}/revoke`

Records consent withdrawal without deleting the original evidence row.

## Audit Logs

### GET `/audit/logs`

Requires `audit:read`. Returns tenant-scoped audit events with sanitized metadata only.

Supported filters:

- `action`
- `resource_type`
- `actor_user_id`
- `outcome`: `success`, `failure`, or `unknown`
- `from` / `to`: RFC3339 timestamps
- `limit` / `offset`

Response:

```json
{
  "items": [
    {
      "id": "...",
      "action": "sale.create",
      "resource_type": "sale",
      "resource_id": "...",
      "outcome": "success",
      "metadata": { "total": "21.80", "outcome": "success" },
      "request_id": "...",
      "created_at": "..."
    }
  ]
}
```

Metadata keys containing secrets, tokens, cookies, authorization values, credentials, session identifiers, JWTs, API keys, or passwords are redacted before persistence and before API output.

## RBAC Model

RBAC is tenant-scoped. Effective permissions come from `user_tenant_roles` joined with `role_permissions`, filtered by the authenticated `tenant_id` and `user_id`. A role assigned in tenant A does not grant permissions in tenant B.

In staging/production, users must have an explicit `user_tenants` membership. The legacy fallback to the first company is available only for development/test databases that predate tenant membership migrations.
