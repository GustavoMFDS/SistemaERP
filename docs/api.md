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
  "user": { "id": "...", "email": "...", "name": "...", "tenant_id": "...", "roles": ["admin"], "permissions": ["sale:write", "sale:discount"] }
}
```

The refresh token is set only as an `HttpOnly`, `SameSite=Strict` cookie when available. The frontend keeps the access token in memory and should not log tokens.

Access tokens are short-lived, and protected requests recheck current user status and tenant membership. A deactivated user or a user removed from the token's tenant is rejected before the token's original expiration time. Effective RBAC permissions are loaded from the tenant-scoped repository on each protected request.

### POST `/auth/refresh`

Accepts the refresh token only from the HttpOnly refresh cookie. Staging/production use the Secure `__Host-refresh_token` cookie; local HTTP development/tests use `sistemaemgo_refresh_token` because the reserved `__Host-` prefix requires HTTPS/Secure. JSON body refresh tokens are not accepted.

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
{ "id": "...", "email": "...", "name": "...", "tenant_id": "...", "roles": ["admin"], "permissions": ["sale:write", "sale:discount"] }
```


`permissions` reflects the effective tenant-scoped RBAC for the tenant in the current access token. It is useful for UI capability hints; protected endpoints still enforce permissions server-side.
## Products

### GET `/products?query=...&limit=...&offset=...`

Response:

```json
{ "items": [{ "id": "...", "sku": "SKU001", "name": "Coca 2L", "price_cash": 10.9, "active": true }], "total": 1 }
```

### GET `/products/barcode/{barcode}`

Returns the active tenant's product with an exact barcode match. Barcode uniqueness is tenant-scoped, so independent stores can register the same manufacturer EAN/GTIN without sharing catalog data.

This endpoint is intended for PDV scanner fallback when the product is not already available in the browser's local catalog cache.

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

### GET `/inventory/low-stock?limit=50`

Requires `inventory:read`. Returns tenant-scoped active products with `qty_on_hand <= min_stock`, sorted by deficit. The response contains `items` (limited to at most 500) and `total` (the **full** tenant-wide count, not truncated to the list limit). The list can be empty even when other tenants have low stock. Product cost is hidden without `finance:read`.

### POST `/inventory/opening-stock`

Requires `inventory:adjust`, authentication, and `Idempotency-Key` of 8–128 characters.
For a **new store's first stock count only**, submit the SKU and absolute quantity
(not a delta). Each item must have an existing active SKU in the authenticated tenant.

```json
{
  "items": [
    { "sku": "PROD-001", "quantity": 15 },
    { "sku": "PROD-002", "quantity": 4.5 }
  ]
}
```

The request accepts **1–100 distinct SKUs**, positive quantities with at most
three decimal places, and applies the batch atomically. The database acquires
row locks and refuses **every row** if any SKU is missing/inactive, has nonzero
stock, or has ever had an inventory movement—even if its balance is zero now.
Repeated requests with the same tenant/key and equivalent sorted payload return
`200` with `replayed: true`, preserving the original `batch_id`, and never
write a second movement. A changed payload under the same key, or a second
opening batch for already used products, returns `409 conflict`.

Successful first submission returns `201` with `batch_id`, `item_count`
and `replayed: false`. `404` means at least one SKU does not belong to
that tenant; `422` means malformed input. The batch, movements, balances and
audit event are committed in **one transaction**; it is never a partial import.
Operation `opening_stock` is reserved for initialization, not normal adjustments.

### POST `/inventory/adjust`

```json
{ "product_id": "...", "delta": -2, "reason": "Perda identificada no inventário", "type": "loss" }
```

## Cash / PDV


Manual inventory adjustment accepts only `adjustment`, `loss`, and `damage`. `purchase` and `return` movements are reserved for the transactional procurement and return workflows.
### GET `/cash/sessions/current`

Returns the currently open session for the tenant's default cash register. This endpoint is used to recover browser state after a lost or ambiguous open-session response. It returns `404 not_found` when no session is open.

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

## Suppliers and Purchases

All routes are tenant-scoped.

### GET `/suppliers`

Supports `query`, `limit`, and `offset`.

### POST `/suppliers`

Requires `Idempotency-Key`. Replaying the same key with the same normalized request returns the original supplier with `"replayed": true`; reusing the key with a different request returns `409 conflict`.

```json
{
  "name": "Distribuidora Exemplo",
  "document": "11222333000199",
  "email": "compras@example.com",
  "phone": "34999999999",
  "contact_name": "Representante",
  "notes": null,
  "active": true
}
```

### PUT `/suppliers/{id}`

Updates one tenant-scoped supplier.

### GET `/purchases`

Supports `status` with `ordered`, `partially_received`, `received`, or `cancelled`, plus `limit` and `offset`.

### POST `/purchases`

Requires `Idempotency-Key`. Replaying the same key and request returns the original purchase instead of creating another order or account payable. A changed request with the same key returns `409 conflict`.

Creates an ordered purchase. Creating the purchase does **not** change inventory.

```json
{
  "supplier_id": "...",
  "invoice_number": "NF-123",
  "payment_due_date": "2026-12-31",
  "notes": "Entrega em duas etapas",
  "items": [
    { "product_id": "...", "qty": 10, "unit_cost": 7.5 }
  ]
}
```

When `payment_due_date` is present, an open account-payable record is linked to the purchase.

### GET `/purchases/{id}`

Returns the purchase, ordered/received quantities per item, and receipt history.

### POST `/purchases/{id}/receive`

Requires `Idempotency-Key`. The key is persisted in the same transaction as the receipt and stock movement, so retrying after an ambiguous/lost response cannot credit inventory twice.

Receives any positive quantity up to the remaining ordered quantity.

```json
{
  "items": [
    { "purchase_item_id": "...", "qty": 4.5 }
  ],
  "notes": "Primeira entrega"
}
```

The operation is transactional: it updates purchase quantities, credits inventory, writes `purchase` inventory movements with `purchase_receipt` references, updates the current product cost, and records a receipt. Partial receipts set the purchase to `partially_received`; the final receipt sets it to `received`.

### POST `/purchases/{id}/cancel`

Cancels only purchases with no received quantity. An associated open account payable is cancelled as part of the same transaction.

## Finance

### GET `/finance/dashboard?from=2026-01-01&to=2026-01-31`

Returns aggregated ledger totals for the period.

### GET `/finance/overview?from=2026-01-01&to=2026-01-31`

Requires `finance:read`, and uses **only** the authenticated tenant. Date filters use the ledger posting date (inclusive on the start date and exclusive after the end date). Returns:

```json
{
  "from": "2026-01-01",
  "to": "2026-01-31",
  "overview": {
    "sales_after_cancellations": 1200.00,
    "estimated_gross_profit": 250.00,
    "refunds_recorded": 40.00,
    "sales_count": 80,
    "cancelled_count": 2
  }
}
```

The numbers above are an **illustrative response**, not real store data. Profit is a **gross estimate** derived from posted sale/cancellation ledger entries: it **does not** account for returns, taxes, provider fees, costs outside item cost, or operating expenses. Sales after cancellations do **not** subtract return refunds. Refunds are shown separately. Cancelled transactions may originate in a different period than the initial sale, so the report must not be presented as a bank settlement or accounting profit.

### GET `/finance/payments`

Lists tenant-scoped payments. Optional filters: `from`, `to`, `method`, `status`, `limit`, and `offset`.

Non-cash payments start as `pending`; cash payments use `not_applicable` because physical cash is reconciled at cash-session close.

Sales may carry provider-neutral transaction metadata:

```json
{
  "method": "credit",
  "amount": 120,
  "provider": "acquirer-name",
  "transaction_ref": "provider-transaction-id",
  "authorization_code": "ABC123",
  "installments": 3
}
```

All metadata fields except `method` and `amount` remain optional.

### POST `/finance/payments/{id}/reconcile`

Requires `finance:reconcile` and `Idempotency-Key`.

```json
{
  "received_amount": 120,
  "fee_amount": 3.5,
  "provider": "acquirer-name",
  "external_ref": "settlement-batch-id",
  "notes": "Conciliação do lote"
}
```

The backend stores the initial reconciliation as immutable history. A gross received amount different from the sale payment marks the payment `divergent`; provider fees are tracked separately and do not change the original sale. If `provider` or `external_ref` is supplied, both fields are required as a pair. Payment `transaction_ref` remains the original sale/payment transaction identifier; reconciliation `external_ref` is a separate settlement/reconciliation identifier and never overwrites it.

A second initial reconciliation for the same payment returns `409 conflict`. Corrections use the explicit adjustment endpoint below.

### POST `/finance/payments/{id}/reconciliation-adjustments`

Requires `finance:reconcile` and `Idempotency-Key`. It is valid only after a non-cash payment has an initial `reconciled` or `divergent` result.

```json
{
  "received_amount": 120,
  "fee_amount": 3.5,
  "notes": "Correção após conferência do extrato"
}
```

The original reconciliation row is not changed. A new adjustment row records the previous and corrected received/fee values, the resulting status, operator, justification and timestamp. Same-key replay returns the original adjustment; a no-op adjustment is rejected.

### GET `/finance/payments/{id}/reconciliation-history`

Requires `finance:read`. Returns the immutable initial reconciliation plus all subsequent adjustment rows in chronological order.

```json
{
  "initial": {
    "status": "divergent",
    "received_amount": 119,
    "fee_amount": 3.5,
    "external_ref": "settlement-batch-id"
  },
  "adjustments": [
    {
      "previous_received_amount": 119,
      "new_received_amount": 120,
      "previous_fee_amount": 3.5,
      "new_fee_amount": 3.5,
      "status": "reconciled",
      "notes": "Correção após conferência do extrato"
    }
  ]
}
```

### GET `/finance/refunds`

Lists return refund obligations and derives `pending`, `partial`, or `settled` from the amount already paid back.

### POST `/finance/returns/{id}/refunds`

Requires `finance:reconcile` and `Idempotency-Key`.

```json
{
  "method": "pix",
  "amount": 15,
  "provider": "bank-or-acquirer",
  "external_ref": "refund-id",
  "cash_session_id": "optional-open-session-id",
  "notes": "Saldo devolvido ao cliente"
}
```

The sum of settlements can never exceed the return's `refund_due`. Cash refunds require an open cash session, sufficient physical cash, and create a real cash withdrawal. Cash refunds cannot carry provider/external-reference metadata. For digital refunds, provider and external reference remain optional, but when one is supplied the other is required. Digital refunds may optionally be associated with an open session so the method-level cash close reconciliation uses the net value. Every settlement also creates a negative `return_refund` ledger entry.

## Fiscal

The repository currently implements **NFC-e model 65 preparation**, not real SEFAZ authorization. Production-like environments must keep `FISCAL_PROVIDER=disabled` until a SEFAZ-ready provider is implemented and homologated.

All routes below are tenant-scoped and require JWT authentication plus the indicated RBAC permission.

### GET `/fiscal/nfce/readiness`

Requires `invoice:read`.

Returns non-secret readiness indicators for NFC-e homologation preparation:

```json
{
  "tenant_id": "...",
  "model": 65,
  "issuer_identity_configured": true,
  "issuer_address_configured": true,
  "municipality_code_configured": true,
  "config_exists": true,
  "transmission_enabled": false,
  "environment": "homologation",
  "series": 1,
  "csc_reference_configured": false,
  "certificate_reference_configured": true,
  "active_products": 120,
  "products_missing_ncm": 0,
  "ready_for_homologation_data": true,
  "blocking_reasons": []
}
```

`ready_for_homologation_data=true` means only that the repository has the minimum non-secret data prepared. It does **not** mean the tenant is authorized or homologated by SEFAZ.

### GET `/fiscal/nfce/issuer`

Requires `invoice:read`.

Returns legal identity plus the editable NFC-e issuer profile. Legal name and CNPJ are read-only through this fiscal endpoint.

### PUT `/fiscal/nfce/issuer`

Requires `invoice:generate`.

Updates IE, CRT and issuer address data used for NFC-e preparation:

```json
{
  "ie": "110042490114",
  "crt": "4",
  "address_street": "Avenida Fiscal",
  "address_number": "100",
  "address_complement": null,
  "address_neighborhood": "Centro",
  "address_city": "Uberlandia",
  "address_city_code": "3170206",
  "address_state": "MG",
  "address_zip": "38400000"
}
```

`crt` accepts `1`, `2`, `3`, or `4`. Municipality code must contain 7 digits and ZIP 8 digits.

### GET `/fiscal/nfce/config`

Requires `invoice:read`.

Returns the tenant NFC-e preparation state. Secret-store references are never returned. `certificate_reference_configured` is the current required secret-reference indicator. `csc_reference_configured` is informational/legacy because QR Code v3 does not require CSC.

### PUT `/fiscal/nfce/config`

Requires `invoice:generate`.

Stores the **reference** to the A1 certificate material in an external secret store:

```json
{
  "environment": "homologation",
  "series": 1,
  "certificate_secret_ref": "secret://nfce/certificate"
}
```

QR Code v3 does not require CSC. The API still accepts `csc_id` plus `csc_secret_ref` as an optional legacy pair, but they are not a readiness requirement. The backend forces `enabled=false` regardless of input. Certificate/legacy CSC secret values must not be sent to this endpoint or stored in source control.

### GET `/fiscal/nfe/xml`

Requires `invoice:read`. Lists historical XML files and development previews already stored for the tenant.

### GET `/fiscal/nfe/xml/{id}/download`

Requires `invoice:read`. Returns `application/xml`.

### POST `/fiscal/nfe/xml` — development preview only

Available only when `FISCAL_PROVIDER=mvp` (development/test). The MVP provider now produces an explicitly marked **NFC-e model 65 preview** and requires product NCM, but it is not signed, authorized, transmitted, protocolled, or suitable for fiscal use.

This route is not registered when `FISCAL_PROVIDER=disabled`.

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


## Returns and exchanges

### POST `/sales/{id}/returns`

Requires permission `sale:return` and header `Idempotency-Key`.

Registers a partial or full return against a finalized sale. The backend computes the refundable amount from the original net sale value; clients cannot choose the refund amount.

```json
{
  "kind": "return",
  "reason": "Tamanho incorreto",
  "items": [
    { "sale_item_id": "uuid", "qty": 1, "restock": true }
  ]
}
```

Use `kind: "exchange"` when the returned merchandise is part of an exchange. The replacement is created as a normal new sale in the PDV. `restock=false` records the return without crediting sellable stock (for example, damaged merchandise).

Response includes `refund_due` and `refund_status: "pending"`. Financial settlement/refund is intentionally separate.

### GET `/returns`

Lists return/exchange records. Optional query: `sale_id`.

### GET `/returns/{id}`

Returns the header and returned items for one return.
