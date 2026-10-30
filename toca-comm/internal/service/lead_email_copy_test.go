package service

import (
	"strings"
	"testing"
)

func TestLeadStatusAndMethodLabels(t *testing.T) {
	cases := map[string]string{
		"NEW":                "Novo",
		"PENDING_ACCEPTANCE": "Em Atendimento",
		"VISIT_SCHEDULED":    "Visita Agendada",
		"NEGOTIATION":        "Em Negociação",
		"DISQUALIFIED":       "Desqualificado",
		"ALGO_NOVO":          "Algo Novo", // desconhecido é humanizado, nunca cru
		"":                   "",
	}
	for in, want := range cases {
		if got := leadStatusLabel(in); got != want {
			t.Errorf("leadStatusLabel(%q) = %q, want %q", in, got, want)
		}
	}

	methods := map[string]string{
		"":             "Manual (pelo painel)",
		"sequential":   "Roleta sequencial",
		"load_balance": "Roleta com carga equilibrada",
		"weighted":     "Roleta ponderada",
		"priority":     "Roleta por prioridade",
		"SEQUENTIAL":   "Roleta sequencial",
	}
	for in, want := range methods {
		if got := leadMethodLabel(in); got != want {
			t.Errorf("leadMethodLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLeadTagGroups_CollapsesAliasesAndGroupsSemantics(t *testing.T) {
	// Cenário real do e-mail de atribuição: MANUAL, BUYER, COMPRA_VENDA,
	// "COMPRA E VENDA" (alias duplicado), OTHER, HIGH_PRICE, "INTERESSE GERAL".
	groups := leadTagGroups([]string{"MANUAL", "BUYER", "COMPRA_VENDA", "COMPRA E VENDA", "OTHER", "HIGH_PRICE", "INTERESSE GERAL"})

	got := map[string][]string{}
	for _, g := range groups {
		got[g.Label] = g.Values
	}

	if vals := got["Origem"]; len(vals) != 1 || vals[0] != "Cadastro manual" {
		t.Errorf("Origem = %v", vals)
	}
	// OTHER é ambíguo/no-op no domínio — não deve poluir o e-mail.
	if vals := got["Outros"]; len(vals) != 0 {
		t.Errorf("Outros deveria ficar vazio para OTHER, got %v", vals)
	}
	if vals := got["Perfil"]; len(vals) != 1 || vals[0] != "Comprador" {
		t.Errorf("Perfil = %v", vals)
	}
	// Alias com e sem acento/underscore vira UM valor só.
	if vals := got["Interesse"]; len(vals) != 1 || vals[0] != "Compra e venda" {
		t.Errorf("Interesse = %v (aliases devem colapsar)", vals)
	}
	if vals := got["Faixa de preço"]; len(vals) != 1 || vals[0] != "R$ 800 mil a R$ 2 milhões" {
		t.Errorf("Faixa de preço = %v", vals)
	}
	if vals := got["Região"]; len(vals) != 1 || vals[0] != "Interesse Geral" {
		t.Errorf("Região = %v", vals)
	}
}

func TestLeadTagGroups_UnknownEnumAndFreeText(t *testing.T) {
	groups := leadTagGroups([]string{"NOVO_ENUM_INTERNO", "Centro Histórico"})
	got := map[string][]string{}
	for _, g := range groups {
		got[g.Label] = g.Values
	}
	// Enum novo (com underscore) → "Outros", humanizado.
	if vals := got["Outros"]; len(vals) != 1 || vals[0] != "Novo Enum Interno" {
		t.Errorf("Outros = %v", vals)
	}
	// Texto livre (localização) → "Região".
	if vals := got["Região"]; len(vals) != 1 || vals[0] != "Centro Histórico" {
		t.Errorf("Região = %v", vals)
	}
}

func TestLeadTagGroups_EmptyAndDuplicates(t *testing.T) {
	if groups := leadTagGroups(nil); len(groups) != 0 {
		t.Fatalf("nil tags = %v, want empty", groups)
	}
	// Mesma tag repetida e o mesmo rótulo por caminhos diferentes não duplicam.
	groups := leadTagGroups([]string{"LOCACAO", "LOCAÇÃO", "TENANT", "tenant"})
	for _, g := range groups {
		if g.Label == "Interesse" && (len(g.Values) != 1 || g.Values[0] != "Locação") {
			t.Errorf("Interesse = %v", g.Values)
		}
		if g.Label == "Perfil" && (len(g.Values) != 1 || g.Values[0] != "Locatário") {
			t.Errorf("Perfil = %v", g.Values)
		}
	}
}

func TestLeadAssignedBody_EscapesOperatorInput(t *testing.T) {
	// Região é texto livre do operador: precisa sair escapada no HTML.
	body := leadAssignedBody(leadAssignedPayload{
		LeadName: "Ana <script>alert(1)</script>",
		Stage:    "NEW",
		Method:   "manual",
		Tags:     []string{"<b>Centro</b>"},
	})
	if strings.Contains(body, "<script>") || strings.Contains(body, "<b>Centro</b>") {
		t.Fatalf("dynamic values must be HTML-escaped: %s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("escaped name missing: %s", body)
	}
	if !strings.Contains(body, "Manual (pelo painel)") {
		t.Fatalf("manual method label missing: %s", body)
	}
}

func TestHumanizeEnum(t *testing.T) {
	cases := map[string]string{
		"HIGH_PRICE":   "High Price",
		"algum-valor":  "Algum Valor",
		"JA LEGIVEL":   "Ja Legivel",
		"":             "",
		"MULTI  space": "Multi Space",
	}
	for in, want := range cases {
		if got := humanizeEnum(in); got != want {
			t.Errorf("humanizeEnum(%q) = %q, want %q", in, got, want)
		}
	}
}
