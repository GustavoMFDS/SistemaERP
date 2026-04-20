# Etapa 3/4 — API (endpoints + exemplos)

Base URL: `/api/v1`

## Autenticação
### POST `/auth/login`
Request:
```json
{ "email": "admin@sistema.local", "password": "admin123" }
```
Response:
```json
{ "access_token": "...", "token_type": "Bearer", "expires_in": 43200 }
```

### GET `/auth/me`
Response:
```json
{ "id": "...", "email": "...", "name": "...", "roles": ["admin"] }
```

## Produtos
### GET `/products?query=...&limit=...&offset=...`
Response:
```json
{ "items": [ { "id":"...","sku":"SKU001","name":"Coca 2L","price_cash":10.9,"active":true } ], "total": 1 }
```

### POST `/products`
Request:
```json
{ "category_id": null, "sku":"SKU001", "barcode":"789...", "name":"Coca 2L", "description":"", "unit":"UN", "cost_price": 7.0, "price_cash": 10.9, "promo_price": null, "min_stock": 5, "active": true }
```

## Estoque
### GET `/inventory/low-stock`
Retorna produtos com `qty_on_hand <= min_stock`.

### POST `/inventory/adjust`
Request:
```json
{ "product_id":"...", "delta": 10, "reason":"Entrada por compra", "type":"purchase" }
```

## Vendas / PDV
### POST `/sales`
Request:
```json
{
  "cash_session_id": "...",
  "customer_id": null,
  "discount_value": 0,
  "items": [
    { "product_id": "...", "qty": 2, "unit_price": 10.9, "discount_value": 0 }
  ],
  "payments": [
    { "method": "pix", "amount": 21.8 }
  ]
}
```
Response:
```json
{ "id":"...", "status":"finalized", "total": 21.8 }
```

### POST `/sales/{id}/cancel`
Request:
```json
{ "reason":"Erro de operação" }
```

## Financeiro
### GET `/finance/dashboard?from=2026-01-01&to=2026-01-31`
Response (MVP): totais agregados de vendas/entradas/saídas.

## Fiscal (NF-e XML MVP)
### POST `/fiscal/nfe/xml`
Request:
```json
{ "sale_id": "..." }
```
Response:
```json
{ "invoice_id":"...", "xml_file_id":"..." }
```

### GET `/fiscal/nfe/xml/{id}/download`
Retorna `application/xml`.

## Erros (formato sugerido)
O MVP ainda usa respostas simples (`http.Error`). Próxima etapa: padronizar em `{"code","message","details","request_id"}`.
