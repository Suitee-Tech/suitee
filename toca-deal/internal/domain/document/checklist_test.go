package document

import "testing"

// A classificação é o que faz o documento enviado pelo cliente aparecer
// ORGANIZADO no painel da imobiliária em vez de cair em "outros". A tabela
// abaixo cobre os nomes que aparecem de fato na esteira (o item vem nomeado pelo
// operador, às vezes só com a categoria preenchida).
func TestClassifyChecklistItem(t *testing.T) {
	cases := []struct {
		name        string
		itemName    string
		itemCat     string
		fieldLabels []string
		want        Category
	}{
		{name: "CPF é identidade", itemName: "CPF", itemCat: "pessoal", want: CategoryIdentidade},
		{name: "RG com acento/caixa", itemName: "R.G.", itemCat: "pessoal", want: CategoryIdentidade},
		{name: "CNH", itemName: "cnh do comprador", itemCat: "pessoal", want: CategoryIdentidade},
		{name: "comprovante de renda", itemName: "Comprovante de Renda", itemCat: "financeiro", want: CategoryComprovante},
		{name: "holerite", itemName: "Holerite 3 últimos meses", itemCat: "financeiro", want: CategoryComprovante},
		{name: "residência acentuada", itemName: "Comprovante de residência", itemCat: "pessoal", want: CategoryComprovante},
		{name: "matrícula do imóvel", itemName: "Matrícula do imóvel", itemCat: "imovel", want: CategoryJuridico},
		{name: "certidão de ônus", itemName: "Certidão de ônus e gravames", itemCat: "imovel", want: CategoryJuridico},
		{name: "procuração", itemName: "Procuração", itemCat: "juridico", want: CategoryJuridico},
		{name: "IPTU é fiscal", itemName: "IPTU 2026", itemCat: "imovel", want: CategoryFiscal},
		{name: "vistoria", itemName: "Vistoria de entrada", itemCat: "imovel", want: CategoryVistoria},
		{name: "laudo", itemName: "Laudo de avaliação", itemCat: "imovel", want: CategoryLaudo},
		{name: "nota fiscal", itemName: "NF Cancelada.pdf", itemCat: "financeiro", want: CategoryFinanceiro},
		{name: "foto do imóvel", itemName: "Foto da fachada", itemCat: "imovel", want: CategoryFoto},
		// Fallback pela categoria do checklist quando o nome não diz nada.
		{name: "fallback pessoal", itemName: "Documento 1", itemCat: "pessoal", want: CategoryIdentidade},
		{name: "fallback financeiro", itemName: "Anexo", itemCat: "financeiro", want: CategoryFinanceiro},
		{name: "fallback imóvel", itemName: "Anexo", itemCat: "imovel", want: CategoryJuridico},
		{name: "sem sinal nenhum", itemName: "Anexo", itemCat: "", want: CategoryOutros},
		// Rótulo do campo pedido também classifica ("CPF" com título genérico).
		{name: "rótulo do campo classifica", itemName: "Documento solicitado", itemCat: "outros", fieldLabels: []string{"CPF"}, want: CategoryIdentidade},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyChecklistItem(c.itemName, c.itemCat, c.fieldLabels...)
			if got.Category != c.want {
				t.Errorf("categoria = %q, want %q", got.Category, c.want)
			}
			if len(got.Tags) == 0 || got.Tags[0] != "esteira" {
				t.Errorf("tags = %v, quero começar por 'esteira' (organização da esteira no painel)", got.Tags)
			}
			found := false
			for _, tag := range got.Tags {
				if tag == string(got.Category) {
					found = true
				}
			}
			if !found {
				t.Errorf("tags = %v, quero incluir a categoria %q", got.Tags, got.Category)
			}
		})
	}
}

// O ID do documento espelho é determinístico: reenvio do cliente (ou replay do
// evento) atualiza a mesma linha, não cria documento duplicado no painel.
func TestChecklistDocumentIDIsDeterministic(t *testing.T) {
	const itemID = "66936966-63a5-4a18-be1e-63aa7e2bffd9"
	first := ChecklistDocumentID(itemID)
	if first != ChecklistDocumentID(itemID) {
		t.Fatalf("IDs diferentes para o mesmo item: %s", first)
	}
	if first == ChecklistDocumentID("outro-item") {
		t.Fatal("itens diferentes produziram o mesmo ID")
	}
	if len(first) != 36 {
		t.Fatalf("ID não parece UUID: %q", first)
	}
	if got := ChecklistDocumentID("  " + itemID + "  "); got != first {
		t.Errorf("espaços mudaram o ID: %q != %q", got, first)
	}
}
