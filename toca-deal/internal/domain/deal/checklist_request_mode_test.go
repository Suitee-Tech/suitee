package deal

import "testing"

func TestNormalizeRequestMode(t *testing.T) {
	cases := map[string]ChecklistItemRequestMode{
		"":       RequestModeUpload,
		"upload": RequestModeUpload,
		"form":   RequestModeForm,
		"FORM":   RequestModeUpload, // case-sensitive: valor inválido cai no default
		"x":      RequestModeUpload,
	}
	for in, want := range cases {
		if got := NormalizeRequestMode(in); got != want {
			t.Errorf("NormalizeRequestMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAddChecklistItemConfiguredCarriesRequestModeAndFields(t *testing.T) {
	d := &Deal{}
	d.AddChecklistItemConfigured("CPF", CategoryPessoal, true, "", "form", []ChecklistRequestedField{
		{Key: "cpf", Label: "CPF", Type: "text", Required: true},
	})
	if len(d.Checklist) != 1 {
		t.Fatalf("expected 1 item, got %d", len(d.Checklist))
	}
	item := d.Checklist[0]
	if item.RequestMode != RequestModeForm {
		t.Fatalf("request mode = %q, want form", item.RequestMode)
	}
	if len(item.RequestedFields) != 1 || item.RequestedFields[0].Key != "cpf" {
		t.Fatalf("requested fields = %+v", item.RequestedFields)
	}
	if item.Status != ChecklistPending {
		t.Fatalf("status = %q, want PENDING", item.Status)
	}

	// Legacy call site (sem modo) continua com upload.
	d.AddChecklistItem("RG", CategoryPessoal, true, "")
	if d.Checklist[1].RequestMode != RequestModeUpload {
		t.Fatalf("legacy add request mode = %q, want upload", d.Checklist[1].RequestMode)
	}
}

func TestInitChecklistNormalizesRequestMode(t *testing.T) {
	d := &Deal{}
	d.InitChecklist([]ChecklistItem{
		{Name: "CPF", Category: CategoryPessoal, Required: true, RequestMode: RequestModeForm},
		{Name: "RG", Category: CategoryPessoal, Required: true},
	})
	if d.Checklist[0].RequestMode != RequestModeForm {
		t.Fatalf("item 0 mode = %q, want form", d.Checklist[0].RequestMode)
	}
	if d.Checklist[1].RequestMode != RequestModeUpload {
		t.Fatalf("item 1 mode = %q, want upload", d.Checklist[1].RequestMode)
	}
}

func TestSolicitChecklistItemScopedSetsFormMode(t *testing.T) {
	d := &Deal{}
	d.AddChecklistItemConfigured("Telefone", CategoryPessoal, false, "", "", nil)
	id := d.Checklist[0].ID
	d.SolicitChecklistItemScoped(id, "party-1", []ChecklistRequestedField{
		{Key: "phone", Label: "Telefone/WhatsApp", Type: "text", Required: true},
	}, "form")

	item := d.Checklist[0]
	if !item.Solicited {
		t.Fatal("item should be solicited")
	}
	if item.RequestMode != RequestModeForm {
		t.Fatalf("request mode = %q, want form", item.RequestMode)
	}
	if item.PartyID != "party-1" {
		t.Fatalf("party id = %q", item.PartyID)
	}
}
