# NFC-e model 65 foundation and homologation gate

This document describes the NFC-e model 65 implementation through migration 0027 and the release gates for SEFAZ authorization.

## Current repository boundary

The repository implements the **normal online NFC-e lifecycle for MG**, with transmission fail-closed by configuration:

- tenant-scoped issuer profile (CNPJ/IE/CRT/address/IBGE municipality code);
- product NCM and optional CEST;
- tenant-scoped NFC-e configuration;
- series validation (0..889);
- external secret reference for the A1 certificate; optional legacy CSC references can be retained for QR Code v2 compatibility;
- fiscal-document sequence storage for model 65;
- invoice fields for number, access key, protocol, authorization/rejection metadata;
- readiness API/UI;
- immutable fiscal classification snapshots and per-item tax calculations;
- NFC-e 4.00 XML candidate generation with 2026 IBS/CBS groups and totals;
- XMLDSig using an A1 certificate resolved only on the server;
- offline validation against a pinned official XSD bundle;
- QR Code v3, access-key generation, MG endpoint catalog and mTLS/SOAP transport;
- transactional lifecycle `reserved -> signed -> submitted -> authorized/rejected`;
- ambiguous-response recovery by access-key consultation instead of blind retransmission;
- cancellation event build/sign/schema validation/submission/persistence;
- development-only NFC-e model 65 preview kept separate from the SEFAZ payload;
- integration/E2E coverage for tenant isolation and safety.

Default production configuration remains:

```text
FISCAL_PROVIDER=disabled
NFCE_SEFAZ_PRODUCTION_ENABLED=false
```

Homologation uses `FISCAL_PROVIDER=sefaz` with
`NFCE_SEFAZ_HOMOLOGATION_ENABLED=true`. Production transmission requires both
`FISCAL_PROVIDER=sefaz` and `NFCE_SEFAZ_PRODUCTION_ENABLED=true`; those flags
are mutually exclusive and configuration validation fails closed on invalid combinations.

The preparation API never enables transmission by itself.

## Current official baseline checked on 2026-09-30

Before implementing the real provider, re-read the Portal Nacional da NF-e because fiscal schemas and validation rules change over time.

The current portal lists, among the official material in use:

- NF-e/NFC-e schema packages published in 2026;
- NT 2025.002 updates for Reforma Tributária do Consumo;
- NT 2026.002 and NT 2026.003;
- NT 2026.004 for CNPJ alfanumérico;

Current DFe compatibility rules used by this foundation:

- normalized CNPJ format: `[A-Z0-9]{12}[0-9]{2}`;
- 44-character access-key format: `[0-9]{6}[A-Z0-9]{12}[0-9]{26}`;
- access-key DV converts every base character using ASCII minus 48 before modulo 11, preserving the historical numeric result for numeric-only CNPJ.
- NT 2025.001 for NFC-e QR Code version 3, where online consultation uses the access key, QR version and environment without CSC; contingency adds the signed parameters defined by the NT;
- NT 2024.001 allowing CRT 4 for MEI.

Do not vendor an old XSD package and assume it remains current. Pin the exact official package/version used by the provider and add a controlled upgrade process.

## Preparation flow

1. Apply migration 0023.
2. Keep fiscal provider disabled.
3. Complete the issuer through `GET/PUT /api/v1/fiscal/nfce/issuer`.
4. Classify active products with NCM; add CEST where applicable.
5. Store the A1 certificate in the deployment secret manager.
6. Save only its reference through `GET/PUT /api/v1/fiscal/nfce/config`. CSC is not required by QR Code v3; legacy CSC fields are optional.
7. Check `GET /api/v1/fiscal/nfce/readiness`.
8. Resolve every blocking reason.
9. Only then begin provider implementation/homologation work.

`ready_for_homologation_data=true` means data preparation is complete. It is **not** permission to issue fiscal documents.

## Secret handling

Never store in PostgreSQL, Git, logs, audit metadata, frontend state persistence, or evidence files:

- legacy CSC secret value, if legacy QR Code v2 compatibility is ever retained;
- PFX/A1 bytes;
- certificate password;
- private key.

The database contains only opaque references such as a secret-manager path/identifier.

A future provider must resolve those references server-side at runtime using the deployment identity and must not expose the resolved secret to handlers or the browser.

## SEFAZ provider coverage

The SEFAZ adapter is separate from the MVP preview and the MVP XML is never used as an authorization payload.

Implemented for the normal online MG flow:

- official XML schema validation for the pinned current package;
- correct NFC-e model 65 `ide` data;
- issuer data and tax regime;
- customer rules when identification is required;
- product NCM/CEST and operation-derived CFOP;
- ICMS/CSOSN/CST and current IBS/CBS/IS groups where applicable;
- payment-method mapping;
- access-key generation and check digit;
- XML digital signature;
- QR Code v3 using the current NFC-e specification (online without CSC; contingency with the required signed parameters);
- UF-specific authorization endpoints;
- service-status check;
- authorization submission and response parsing;
- protocol persistence;
- rejected-document state;
- timeout/ambiguous-response recovery using access-key consultation;
- cancellation event;
- immutable audit events without secret leakage.

Still required before a store may enable production transmission:

- contingency procedure/implementation appropriate to the store and current NFC-e rules;
- inutilization workflow where legally applicable;
- DANFE-NFC-e generation/printing validated against the current manual;
- external SEFAZ homologation evidence for the exact release SHA.

## Number allocation

The application reserves tenant-scoped model 65 numbers transactionally and persists the
access key before signing. Once a signed document transitions to `submitted`, retries
consult by access key rather than reissuing the document, so an ambiguous response cannot
silently reuse the fiscal number.

The development MVP preview remains separate and must not be treated as fiscal numbering.

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
