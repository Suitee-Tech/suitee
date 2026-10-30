package services

import (
	"strings"
	"testing"
)

func TestTruncateToolResult(t *testing.T) {
	small := "resposta curta"
	if got := truncateToolResult(small); got != small {
		t.Fatalf("small result changed: %q", got)
	}

	big := strings.Repeat("x", maxToolResultChars+5000)
	got := truncateToolResult(big)
	if len(got) > maxToolResultChars+120 {
		t.Fatalf("truncated result still huge: %d", len(got))
	}
	if !strings.Contains(got, "truncado") || !strings.Contains(got, "5000") {
		t.Fatalf("truncated result missing marker: %q", got[len(got)-80:])
	}
}

func TestBudgetHistoryForDeepseekTruncatesToolContent(t *testing.T) {
	big := strings.Repeat("y", maxToolResultChars+1000)
	messages := []DeepseekMessage{
		{Role: "user", Content: "liste leads"},
		{Role: "tool", Content: big},
		{Role: "assistant", Content: "ok"},
	}
	out := budgetHistoryForDeepseek(messages)
	if len(out[1].Content) <= 1000 {
		// A troca de slice (build de out) preserva a ordem; verifica o conteúdo.
	}
	found := false
	for _, m := range out {
		if m.Role == "tool" {
			found = true
			if len(m.Content) > maxToolResultChars+120 {
				t.Fatalf("tool content not truncated: %d", len(m.Content))
			}
		}
	}
	if !found {
		t.Fatal("tool message missing")
	}
}

func TestBudgetHistoryForDeepseekDropsOldestWhenOverBudget(t *testing.T) {
	chunk := strings.Repeat("z", maxToolResultChars-1) // ~40k chars cada
	messages := []DeepseekMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "pergunta 1"},
		{Role: "tool", Content: chunk},
		{Role: "assistant", Content: "resp 1"},
		{Role: "user", Content: "pergunta 2"},
		{Role: "tool", Content: chunk},
		{Role: "assistant", Content: "resp 2"},
	}
	out := budgetHistoryForDeepseek(messages)
	total := 0
	for _, m := range out {
		total += len(m.Content)
	}
	if total > maxHistoryChars {
		t.Fatalf("history still over budget: %d", total)
	}
	// O bloco tool do início deve ter sido descartado ou o início não pode
	// começar em tool órfão (sanitize prefere dropar).
	if out[0].Role == "tool" {
		t.Fatalf("history starts with orphan tool message: %d messages", len(out))
	}
}
