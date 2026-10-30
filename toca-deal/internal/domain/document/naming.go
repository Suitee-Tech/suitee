package document

import "fmt"

// Naming conventions for documents persisted by toca-deal.
//
// All physical objects in the `toca-documents` Supabase bucket follow:
//
//   {tenant_id}/{category}/{uuid8}_{sanitized_name}.{ext}
//
// (See DocumentService.Upload — that prefix is non-negotiable since it
// scopes objects per tenant and category for IAM/listing.)
//
// The *logical* filename (the value handed to Upload as `filename`,
// stamped into Content-Disposition headers and the trailing segment of
// the storage key) varies by origin:
//
//   • Operator/portal upload  → user-provided filename, preserved verbatim
//   • Contract draft DOCX     → `rascunho-{contract_id}-{hash8}.docx`
//   • Signed contract PDF     → `contrato-assinado-v{version}-{contract_id}.pdf`
//   • Contract attachment PDF → `anexo-{contract_id}-{idx}-{sanitized}.pdf`
//
// The signed-PDF and attachment forms are centralized in this file so a
// single grep (`SignedContractFilename`) surfaces every place naming is
// constructed. When changing convention, update callers AND backfill a
// rename script for existing objects — operators reference these names
// in audit logs.

// SignedContractFilename returns the canonical filename for the signed
// PDF a Clicksign envelope produces. The `version` argument is the
// Document.Version at the moment of upload — incrementing it (via
// CreateVersion) yields a new object instead of overwriting the prior
// signed copy. Version 0/1 are both rendered as `v1` to keep the visible
// progression starting at 1.
func SignedContractFilename(contractID string, version int) string {
	v := version
	if v < 1 {
		v = 1
	}
	return fmt.Sprintf("contrato-assinado-v%d-%s.pdf", v, contractID)
}
