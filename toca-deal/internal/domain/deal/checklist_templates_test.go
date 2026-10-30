package deal_test

import (
	"testing"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

func TestBuiltinChecklistTemplates_FiveExpectedIDs(t *testing.T) {
	want := map[string]bool{
		"pf_compra_financiamento": true,
		"pf_compra_avista":        true,
		"pj_compra":               true,
		"locacao_pf":              true,
		"locacao_pj":              true,
	}
	got := deal.BuiltinChecklistTemplates()
	if len(got) != len(want) {
		t.Fatalf("template count = %d, want %d", len(got), len(want))
	}
	for _, tpl := range got {
		if !want[tpl.ID] {
			t.Errorf("unexpected template id %q — verify FE ChecklistTemplateId union before removing", tpl.ID)
		}
		if tpl.Label == "" {
			t.Errorf("template %q missing label", tpl.ID)
		}
		if len(tpl.Items) == 0 {
			t.Errorf("template %q has zero items", tpl.ID)
		}
	}
}

func TestBuiltinChecklistTemplateByID_LookupHit(t *testing.T) {
	tpl := deal.BuiltinChecklistTemplateByID("pj_compra")
	if tpl == nil {
		t.Fatal("expected pj_compra template to exist")
	}
	if tpl.ID != "pj_compra" {
		t.Errorf("lookup returned wrong template: %s", tpl.ID)
	}
}

func TestBuiltinChecklistTemplateByID_LookupMiss(t *testing.T) {
	if tpl := deal.BuiltinChecklistTemplateByID("does_not_exist"); tpl != nil {
		t.Errorf("expected nil for unknown id, got %+v", tpl)
	}
}
