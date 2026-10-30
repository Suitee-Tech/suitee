package services

import (
	"strings"
	"testing"
)

func TestBuildNoToolSuggestionsFinanceDomain(t *testing.T) {
	tools := []DeepseekTool{
		{ReadOnly: true, Function: DeepseekToolFunction{Name: "get_financial_dashboard", Description: "Resumo financeiro"}},
		{ReadOnly: true, Function: DeepseekToolFunction{Name: "search_charges", Description: "Cobranças"}},
		{ReadOnly: true, Function: DeepseekToolFunction{Name: "search_transfers", Description: "Repasses"}},
	}
	ctx := map[string]any{"label": "Financeiro", "path": "/financeiro"}
	out := buildNoToolSuggestions(ctx, tools, "resumo do mês")
	if !strings.Contains(out, "Resumo financeiro") {
		t.Fatalf("expected finance card, got:\n%s", out)
	}
	if !strings.Contains(out, "Cobranças em atraso") || !strings.Contains(out, "Repasses pendentes") {
		t.Fatalf("expected charges/transfers cards, got:\n%s", out)
	}
	if strings.Contains(out, "get_financial_summary") {
		t.Fatalf("card still references removed get_financial_summary:\n%s", out)
	}
}

func TestBuildNoToolSuggestionsLeadsDomain(t *testing.T) {
	tools := []DeepseekTool{
		{ReadOnly: true, Function: DeepseekToolFunction{Name: "search_leads", Description: "Buscar leads"}},
		{ReadOnly: true, Function: DeepseekToolFunction{Name: "get_calendar_events", Description: "Compromissos"}},
	}
	ctx := map[string]any{"label": "Leads", "path": "/leads"}
	out := buildNoToolSuggestions(ctx, tools, "liste os leads")
	if !strings.Contains(out, "Listar leads recentes") {
		t.Fatalf("expected leads card, got:\n%s", out)
	}
}
