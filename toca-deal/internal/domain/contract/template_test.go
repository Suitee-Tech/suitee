package contract

import "testing"

func TestValidTemplateKind(t *testing.T) {
	for _, k := range []TemplateKind{TemplateKindContract, TemplateKindDistrato} {
		if !ValidTemplateKind(k) {
			t.Errorf("ValidTemplateKind(%q) = false, want true", k)
		}
	}
	// Vazio e valores desconhecidos não são kinds válidos explícitos.
	for _, k := range []TemplateKind{"", "outro", "CONTRACT"} {
		if ValidTemplateKind(k) {
			t.Errorf("ValidTemplateKind(%q) = true, want false", k)
		}
	}
}

func TestContractTemplate_EffectiveKind(t *testing.T) {
	// Template legado (sem kind) é tratado como contrato.
	if got := (&ContractTemplate{}).EffectiveKind(); got != TemplateKindContract {
		t.Errorf("EffectiveKind() de template sem kind = %q, want contract", got)
	}
	if got := (&ContractTemplate{Kind: TemplateKindDistrato}).EffectiveKind(); got != TemplateKindDistrato {
		t.Errorf("EffectiveKind() distrato = %q, want distrato", got)
	}
}
