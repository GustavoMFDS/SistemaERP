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

There is also a tenant-scoped database kill switch. Preparing/changing the NFC-e
configuration always resets `enabled=false`. After the deployment-level production
gate is enabled and the complete SEFAZ stack is wired, an authorized operator must
explicitly enable that tenant through:

```http
POST /api/v1/fiscal/nfce/config/transmission
{"enabled": true}
```

The action is audited as `fiscal.nfce_config.transmission`. Setting `enabled=false`
stops new production reservations/signing/submission immediately. Consultation of an
already submitted document and cancellation of an already authorized NFC-e remain
available so an emergency kill switch does not strand an ambiguous fiscal state or
prevent a required cancellation.

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

1. Apply the full fiscal migration chain through migration 0029.
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

The SEFAZ provider resolves those references server-side at runtime using the deployment identity and does not expose the resolved secret to handlers or the browser.

## SEFAZ provider coverage

The SEFAZ adapter is separate from the MVP preview and the MVP XML is never used as an authorization payload.

Implemented for the normal online MG flow and the controlled offline-contingency path:

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
- number inutilization through `NFeInutilizacao4`, with signed `inutNFe`, pinned XSD validation, immutable persistence, unused-range checks, overlap locking and no blind retransmission after an ambiguous response;
- offline contingency with `tpEmis=9`, fresh fiscal numbering, persisted `dhCont`/justification, QR Code v3 signed with RSA/SHA-1 by the same A1 key and later transmission through the normal authorization lifecycle;
- DANFE-NFC-e generated server-side from the stored fiscal XML, including pre-authorization printing for a signed offline-contingency document and authorized/cancelled printing for the normal flow;
- access to DANFE protected by `invoice:read` and audited on every open;
- immutable audit events without secret leakage.

Still required before a store may enable production transmission:

- external SEFAZ homologation evidence for the exact release SHA, including authorization, cancellation and inutilization;
- an operational contingency drill in the target store, including reconnect/transmission within the applicable deadline;
- operational DANFE-NFC-e validation on the real printer/media used by the store;
- accountant/fiscal approval of the configured tax rules and the store-specific operating procedure.

## Number allocation

The application reserves tenant-scoped model 65 numbers transactionally and persists the
access key before signing. Once a signed document transitions to `submitted`, retries
consult by access key rather than reissuing the document, so an ambiguous response cannot
silently reuse the fiscal number.

The development MVP preview remains separate and must not be treated as fiscal numbering.

Offline contingency uses the same tenant/model/series sequence allocator and therefore
always consumes a new fiscal number. Calling the normal reservation path for a sale that
already owns an offline-contingency reservation fails with a conflict instead of replacing
or reusing that document. The inutilization service also rejects any range containing a
number already present in `invoices`, including a number issued with `tpEmis=9`.

## Offline contingency flow

Offline contingency is an explicit operator/recovery path; it is never selected merely
because an HTTP request to SEFAZ timed out.

1. Reserve a new contingency NFC-e:
   `POST /api/v1/fiscal/nfce/reservations/offline-contingency` with
   `sale_id`, `issued_at`, `contingency_started_at` and a 15–256 character
   `justification`.
2. Complete the same immutable item-tax calculations required by normal NFC-e.
3. Sign through `POST /api/v1/fiscal/nfce/invoices/{invoiceID}/sign`.
   The server signs both the NFC-e XML and the QR Code v3 payload with the A1 key.
4. Print/open the signed contingency DANFE through
   `GET /api/v1/fiscal/nfce/invoices/{invoiceID}/danfe`.
   Before authorization it is explicitly marked as contingency and as pending
   SEFAZ transmission; no authorization protocol is invented.
5. When communication returns, call
   `POST /api/v1/fiscal/nfce/invoices/{invoiceID}/authorize`.
   The existing submitted/consult lifecycle prevents a blind duplicate transmission
   after an ambiguous response.

## Number inutilization flow

Inutilization is restricted to unused model-65 numbers. The request is signed and
persisted before the remote call, and a network error leaves the operation in
`submitted`; replay returns the pending record rather than resending the range blindly.

Use:

```http
POST /api/v1/fiscal/nfce/inutilizations
Content-Type: application/json

{
  "year": 2026,
  "series": 1,
  "start_number": 101,
  "end_number": 110,
  "justification": "Falha operacional pulou esta faixa fiscal."
}
```

The SEFAZ deployment configuration must pin an inutilization schema entrypoint through
`NFCE_INUTILIZATION_SCHEMA_ENTRYPOINT` in addition to the NFC-e and event schemas.

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
- offline contingency issuance, signed QR Code, DANFE and delayed authorization are tested;
- number inutilization is homologated and the ambiguous-response procedure is tested;
- DANFE-NFC-e is validated on the real target printer;
- secret rotation is tested;
- certificate expiry monitoring exists;
- per-tenant isolation is proven;
- accountant/fiscal reviewer approves the configured tax rules;
- the store/UF requirements are reviewed.

Only after that deployment gate should an operator enable the tenant-scoped production switch (`enabled=true`). Preparing or changing NFC-e configuration resets the switch to `false`.

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
