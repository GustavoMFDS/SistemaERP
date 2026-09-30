# NFC-e model 65 foundation and homologation gate

This document describes the repository state after migration 0023 and the conditions that must be met before SistemaEmGo can authorize NFC-e documents.

## Current repository boundary

The repository currently implements **preparation**, not fiscal authorization:

- tenant-scoped issuer profile (CNPJ/IE/CRT/address/IBGE municipality code);
- product NCM and optional CEST;
- tenant-scoped NFC-e configuration;
- series validation (0..889);
- external secret references for CSC and A1 certificate;
- fiscal-document sequence storage for model 65;
- invoice fields for number, access key, protocol, authorization/rejection metadata;
- readiness API/UI;
- development-only NFC-e model 65 preview;
- integration/E2E coverage for tenant isolation and safety.

Production-like environments must keep:

```text
FISCAL_PROVIDER=disabled
```

The preparation API never sets `nfce_configs.enabled=true`.

## Current official baseline checked on 2026-09-30

Before implementing the real provider, re-read the Portal Nacional da NF-e because fiscal schemas and validation rules change over time.

The current portal lists, among the official material in use:

- NF-e/NFC-e schema packages published in 2026;
- NT 2025.002 updates for Reforma Tributária do Consumo;
- NT 2026.002 and NT 2026.003;
- NT 2026.004 for CNPJ alfanumérico;
- NT 2025.001 for NFC-e QR Code version 3;
- NT 2024.001 allowing CRT 4 for MEI.

Do not vendor an old XSD package and assume it remains current. Pin the exact official package/version used by the provider and add a controlled upgrade process.

## Preparation flow

1. Apply migration 0023.
2. Keep fiscal provider disabled.
3. Complete the issuer through `GET/PUT /api/v1/fiscal/nfce/issuer`.
4. Classify active products with NCM; add CEST where applicable.
5. Store the CSC and certificate in the deployment secret manager.
6. Save only references through `GET/PUT /api/v1/fiscal/nfce/config`.
7. Check `GET /api/v1/fiscal/nfce/readiness`.
8. Resolve every blocking reason.
9. Only then begin provider implementation/homologation work.

`ready_for_homologation_data=true` means data preparation is complete. It is **not** permission to issue fiscal documents.

## Secret handling

Never store in PostgreSQL, Git, logs, audit metadata, frontend state persistence, or evidence files:

- CSC secret value;
- PFX/A1 bytes;
- certificate password;
- private key.

The database contains only opaque references such as a secret-manager path/identifier.

A future provider must resolve those references server-side at runtime using the deployment identity and must not expose the resolved secret to handlers or the browser.

## Real provider requirements

The real SEFAZ provider must be a separate adapter and must not reuse the MVP preview as an authorization payload.

At minimum it needs:

- official XML schema validation for the pinned current package;
- correct NFC-e model 65 `ide` data;
- issuer data and tax regime;
- customer rules when identification is required;
- product NCM/CEST and operation-derived CFOP;
- ICMS/CSOSN/CST and current IBS/CBS/IS groups where applicable;
- payment-method mapping;
- access-key generation and check digit;
- XML digital signature;
- QR Code using the current NFC-e specification;
- UF-specific authorization endpoints;
- service-status check;
- authorization submission and response parsing;
- protocol persistence;
- rejected-document state;
- timeout/ambiguous-response recovery using access-key consultation;
- contingency flow;
- cancellation event;
- inutilization where legally applicable;
- DANFE-NFC-e generation/printing;
- immutable audit events without secret leakage.

## Number allocation

Migration 0023 provides `fiscal_document_sequences`, but the current application does not consume numbers for real fiscal documents.

Before provider activation, define and test the number-allocation state machine so that a number is not silently reused after an external transmission attempt. Allocation/commit behavior must account for timeouts and ambiguous SEFAZ responses.

Do not call the development preview number/series values fiscal numbering.

## Authorization state

The `invoices` table now has fields for:

- `model`;
- `series`;
- `document_number`;
- `environment`;
- `access_key`;
- `authorization_protocol`;
- `authorized_at`;
- `rejection_code`;
- `rejection_message`.

The real provider should persist explicit lifecycle states instead of treating XML generation as authorization. The existing historical `xml_generated` state belongs to the old preview flow and must not be interpreted as SEFAZ acceptance.

## Homologation gate

Do not enable production transmission until there is evidence for the exact release SHA that:

- unit/integration/E2E suites are green;
- official XSD validation passes;
- homologation authorization succeeds for representative sales;
- QR Code opens the correct homologation consultation;
- duplicate/retry behavior produces a single fiscal document;
- timeout after send is recovered by consultation rather than blind reissue;
- rejection codes are persisted and surfaced safely;
- cancellation is homologated;
- contingency procedure is tested;
- DANFE-NFC-e is validated;
- secret rotation is tested;
- certificate expiry monitoring exists;
- per-tenant isolation is proven;
- accountant/fiscal reviewer approves the configured tax rules;
- the store/UF requirements are reviewed.

Only after that gate should a future migration/configuration path allow `enabled=true`.

## Development preview

`FISCAL_PROVIDER=mvp` remains development/test-only. It:

- uses model 65;
- requires an 8-digit NCM;
- is explicitly marked `NAO FISCAL / NAO TRANSMITIR`;
- does not sign;
- does not generate a real access key;
- does not generate the official QR Code;
- does not contact SEFAZ;
- does not produce an authorization protocol.

It exists to preserve test coverage and historical XML storage while the real provider is developed.
