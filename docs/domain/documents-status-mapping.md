# Status mapping: Contract ↔ Document ↔ Esteira ↔ Frontend

This document is the single source of truth for how a document moves
through the toca system end-to-end — from a contract draft in
`toca-deal`, through envelope signing in `toca-signature`, into the
Document repository, and finally surfaced on the Esteira Digital and
the Central de Documentos pages in `toka`.

Each row in the canonical table represents one user-visible stage. When
any of the columns drift, update **all sides simultaneously** —
operators reason about these labels in audit reviews and legal
disputes, so silent re-labeling is not acceptable.

## Canonical table

| toca-deal `contract.Status` | toca-deal `document.DocumentStatus` | toca-signature `envelope.Status` | Frontend display (`getStatusMeta`) |
|---|---|---|---|
| `DRAFT` | `rascunho` | `PENDING` / `DRAFT` | "Rascunho" |
| `PENDING_SIGNATURE` | `pendente_assinatura` | `RUNNING` | "Aguardando Assinatura" |
| `PARTIALLY_SIGNED` | `parcialmente_assinado` | `PARTIALLY_SIGNED` | "Parcialmente Assinado" |
| `SIGNED` / `ACTIVE` | `assinado` | `FINISHED` | "Assinado" |
| `EXPIRED` | `expirado` | `EXPIRED` | "Expirado" |
| `TERMINATED` | `cancelado` | `CANCELED` / `REFUSED` / `ERROR` | "Cancelado" |
| — (deprecated) | `pendente` | — | "Pendente" (normalized to "Aguardando Assinatura" by `normalizeBackendStatus`) |

### Notes

- **`SIGNED` vs `ACTIVE`**: contract.Status splits "fully signed" and
  "active in production" — both collapse to `assinado` for the document
  layer because the UI doesn't care about the distinction. A signed
  contract that hasn't yet been activated still surfaces as "Assinado"
  in the Central de Documentos.
- **`PENDING_SIGNATURE` vs `RUNNING`**: the envelope runtime
  distinguishes "waiting for the first signer to open" from "in
  progress with at least one viewed/signed" — neither difference
  surfaces in document statuses since the document UI only cares
  whether the file is signable yet.
- **Legacy `pendente`**: emitted by seed contracts created before the
  granular signature statuses landed. `normalizeBackendStatus` in
  `lib/documents.ts` rewrites it to `pendente_assinatura` so the UI
  doesn't render two near-identical "waiting" buckets.
- **`TERMINATED` / signer refusals**: a single refusal collapses the
  whole document to `cancelado`. We don't model "partially refused" —
  Clicksign treats a refusal as a terminal envelope state.

## Code references

| Layer | File | Notes |
|---|---|---|
| Go domain | [`toca-deal/internal/domain/document/mapping.go`](../../toca-deal/internal/domain/document/mapping.go) | `StatusFromContractStatus(contract.Status) DocumentStatus` — the only allowed bridge between the two Go enums. |
| Go tests | [`toca-deal/internal/domain/document/mapping_test.go`](../../toca-deal/internal/domain/document/mapping_test.go) | Includes an exhaustiveness assertion that fails if a new `contract.Status` is added without a matching mapping. |
| TS canonical enums | [`toka/src/app/lib/documents.ts`](../../toka/src/app/lib/documents.ts) | `DOCUMENT_STATUSES` + `getStatusMeta` + `normalizeBackendStatus`. |
| TS tests | [`toka/src/app/lib/documents.test.ts`](../../toka/src/app/lib/documents.test.ts) | Pin the legacy `pendente` normalization + the parity table for checklist↔document categories. |
| Signature webhook handlers | [`toca-deal/internal/services/document_service.go`](../../toca-deal/internal/services/document_service.go) | `PersistSignedContractPDF` flips the Document into `assinado` when the Clicksign signed PDF lands. |

## When to update this file

Any of the following changes **must** also be reflected here:

1. New `contract.Status` value introduced.
2. New `document.DocumentStatus` value introduced.
3. Frontend display label changed (operator-facing copy).
4. New collapse rule between envelope events and document status.

The exhaustiveness tests in `mapping_test.go` and `documents.test.ts`
will fail the build if Go/TS enum sets drift — they're the safety net.
