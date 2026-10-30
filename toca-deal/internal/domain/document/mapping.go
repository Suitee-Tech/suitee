package document

import (
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
)

// StatusFromContractStatus maps a contract.Status into the equivalent
// document.DocumentStatus. The two enums live in different bounded
// contexts and use different vocabularies (English uppercase contract
// lifecycle vs Portuguese lowercase document state surfaced in the UI),
// so this function is the single place they're bridged.
//
// Returns "" for unknown values so callers can decide whether to fail
// loudly or skip the sync (current callers skip and log).
//
// When a new contract.Status is introduced, add the corresponding case
// here and a row in `mapping_test.go` — the exhaustiveness test pins
// the expected mapping.
func StatusFromContractStatus(s contract.Status) DocumentStatus {
	switch s {
	case contract.StatusDraft:
		return StatusRascunho
	case contract.StatusPending:
		return StatusPendenteAssinatura
	case contract.StatusPartiallySigned:
		return StatusParcialmenteAssinado
	case contract.StatusSigned, contract.StatusActive:
		return StatusAssinado
	case contract.StatusExpired:
		return StatusExpirado
	case contract.StatusTerminated:
		return StatusCancelado
	}
	return ""
}

// CategoryFromChecklistCategory maps the deal-level checklist taxonomy
// (`pessoal | financeiro | imovel | juridico | outros` — see
// `deal.ChecklistItemCategory`) into a `document.Category` for when a
// checklist item is promoted into the central document repository.
//
// Input is a plain string to keep this package free of an import cycle
// on `domain/deal`. Returns `CategoryOutros` for unknown inputs rather
// than failing — the caller is typically a best-effort copy that
// shouldn't block the upload if a new checklist category was added
// upstream without a matching mapping here.
//
// Mapping rationale:
//
//   - pessoal   → identidade — RG/CNH/CPF are personal identification
//   - financeiro→ financeiro
//   - imovel    → juridico   — escritura/matrícula/IPTU are legal-adjacent
//   - juridico  → juridico
//   - outros    → outros
//
// When the checklist taxonomy gains a new value, add the matching case
// here and a test row in `mapping_test.go`.
func CategoryFromChecklistCategory(checklistCategory string) Category {
	switch checklistCategory {
	case "pessoal":
		return CategoryIdentidade
	case "financeiro":
		return CategoryFinanceiro
	case "imovel":
		return CategoryJuridico
	case "juridico":
		return CategoryJuridico
	case "outros":
		return CategoryOutros
	}
	return CategoryOutros
}
