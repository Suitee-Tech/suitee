package document

import (
	"testing"

	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
)

func TestStatusFromContractStatus_Mapping(t *testing.T) {
	cases := []struct {
		name     string
		input    contract.Status
		expected DocumentStatus
	}{
		{"draft", contract.StatusDraft, StatusRascunho},
		{"pending signature", contract.StatusPending, StatusPendenteAssinatura},
		{"partially signed", contract.StatusPartiallySigned, StatusParcialmenteAssinado},
		{"signed", contract.StatusSigned, StatusAssinado},
		{"active treated as signed", contract.StatusActive, StatusAssinado},
		{"expired", contract.StatusExpired, StatusExpirado},
		{"terminated", contract.StatusTerminated, StatusCancelado},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := StatusFromContractStatus(tc.input)
			if got != tc.expected {
				t.Errorf("StatusFromContractStatus(%q) = %q; want %q",
					tc.input, got, tc.expected)
			}
		})
	}
}

// TestStatusFromContractStatus_Exhaustive guards against silently dropping
// a new contract.Status when one is introduced upstream. If this test
// fails after a contract status enum change, add the matching case to
// `mapping.go` (and to the table above) — don't relax this assertion.
func TestStatusFromContractStatus_Exhaustive(t *testing.T) {
	allContractStatuses := []contract.Status{
		contract.StatusDraft,
		contract.StatusPending,
		contract.StatusPartiallySigned,
		contract.StatusSigned,
		contract.StatusActive,
		contract.StatusExpired,
		contract.StatusTerminated,
	}
	for _, s := range allContractStatuses {
		if got := StatusFromContractStatus(s); got == "" {
			t.Errorf("contract.Status %q has no document.DocumentStatus mapping — "+
				"add a case to StatusFromContractStatus", s)
		}
	}
}

func TestStatusFromContractStatus_UnknownReturnsEmpty(t *testing.T) {
	if got := StatusFromContractStatus(contract.Status("UNKNOWN_FUTURE_VALUE")); got != "" {
		t.Errorf("unknown status should map to empty, got %q", got)
	}
}

// TestAllDocumentStatuses_ParityWithFrontend pins the canonical status
// set so a divergent enum on either side breaks the build. The matching
// TS test lives in `toka/src/app/lib/documents.test.ts` (it.each over
// `DOCUMENT_STATUSES`). If you change one list, change the other AND
// update `docs/domain/documents-status-mapping.md`.
func TestAllDocumentStatuses_ParityWithFrontend(t *testing.T) {
	expected := []DocumentStatus{
		StatusRascunho,
		StatusPendente,
		StatusPendenteAssinatura,
		StatusParcialmenteAssinado,
		StatusAssinado,
		StatusExpirado,
		StatusCancelado,
	}
	if len(AllDocumentStatuses) != len(expected) {
		t.Fatalf("AllDocumentStatuses has %d entries; expected %d. "+
			"Update docs/domain/documents-status-mapping.md and the TS "+
			"DOCUMENT_STATUSES at the same time.",
			len(AllDocumentStatuses), len(expected))
	}
	for i, want := range expected {
		if AllDocumentStatuses[i] != want {
			t.Errorf("AllDocumentStatuses[%d] = %q; want %q (order matters "+
				"for the FE table)", i, AllDocumentStatuses[i], want)
		}
	}
}

func TestAllCategories_ParityWithFrontend(t *testing.T) {
	expected := []Category{
		CategoryContrato,
		CategoryAditivo,
		CategoryLaudo,
		CategoryVistoria,
		CategoryProcuracao,
		CategoryRecibo,
		CategoryNotificacao,
		CategoryFinanceiro,
		CategoryJuridico,
		CategoryFiscal,
		CategoryIdentidade,
		CategoryComprovante,
		CategoryFoto,
		CategoryOutros,
	}
	if len(AllCategories) != len(expected) {
		t.Fatalf("AllCategories has %d entries; expected %d. "+
			"Update docs/domain/documents-status-mapping.md and the TS "+
			"DOCUMENT_CATEGORIES at the same time.",
			len(AllCategories), len(expected))
	}
	for i, want := range expected {
		if AllCategories[i] != want {
			t.Errorf("AllCategories[%d] = %q; want %q", i, AllCategories[i], want)
		}
	}
}

func TestCategoryFromChecklistCategory_Mapping(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected Category
	}{
		{"pessoal maps to identidade", "pessoal", CategoryIdentidade},
		{"financeiro maps to financeiro", "financeiro", CategoryFinanceiro},
		{"imovel maps to juridico", "imovel", CategoryJuridico},
		{"juridico maps to juridico", "juridico", CategoryJuridico},
		{"outros maps to outros", "outros", CategoryOutros},
		{"unknown falls back to outros", "novo_grupo_futuro", CategoryOutros},
		{"empty falls back to outros", "", CategoryOutros},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CategoryFromChecklistCategory(tc.input); got != tc.expected {
				t.Errorf("CategoryFromChecklistCategory(%q) = %q; want %q",
					tc.input, got, tc.expected)
			}
		})
	}
}
