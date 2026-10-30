package services

import (
	"strings"
	"testing"
)

// A Central de Ajuda (search_help/get_help_topic) precisa estar declarada no
// prompt da Suitee IA: regras de uso + mapa rápido de ferramentas.
func TestSystemPromptDeclaresHelpTools(t *testing.T) {
	b := NewPromptBuilder()
	prompt := b.SystemPrompt(PromptContext{
		HasMCP: true,
		AvailableTools: []DeepseekTool{
			{ReadOnly: true, Function: DeepseekToolFunction{Name: "search_help", Description: "Buscar na Central de Ajuda"}},
			{ReadOnly: true, Function: DeepseekToolFunction{Name: "get_help_topic", Description: "Ler tópico completo"}},
		},
	})
	for _, want := range []string{
		"search_help",
		"get_help_topic",
		"Central de Ajuda",
		"Ajuda/documentação de uso",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt deveria conter %q, veio:\n%s", want, prompt)
		}
	}
}

// Sem MCP o prompt não pode prometer as tools de ajuda.
func TestSystemPromptWithoutMCPDoesNotPromiseHelpTools(t *testing.T) {
	b := NewPromptBuilder()
	prompt := b.SystemPrompt(PromptContext{HasMCP: false})
	if strings.Contains(prompt, "search_help") {
		t.Fatalf("prompt sem MCP não deveria citar search_help:\n%s", prompt)
	}
}
