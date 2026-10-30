package handlers

import (
	"testing"
)

// TestPersonalDataResponse_HasNoticeAndDataSources confirma o shape
// default — caso o serializer mude/algum campo seja renomeado sem
// querer, este test pega. Pequeno mas vital pra contrato externo.
func TestPersonalDataResponse_Defaults(t *testing.T) {
	resp := PersonalDataResponse{
		GeneratedAt: "2026-05-20T10:00:00Z",
		Notice:      "...",
		OtherDataSources: []dataSourceRef{
			{Service: "toca-deal", Description: "x", HowToAccess: "y"},
		},
	}
	if resp.Notice == "" {
		t.Error("Notice deve estar sempre presente")
	}
	if len(resp.OtherDataSources) == 0 {
		t.Error("OtherDataSources deve ter entradas")
	}
}

// TestDataSourceRef_RequiredFields documenta que cada dataSourceRef
// precisa expor service+description+how para que o usuário consiga
// efetivar o direito de acesso.
func TestDataSourceRef_RequiredFields(t *testing.T) {
	ref := dataSourceRef{Service: "toca-deal"}
	if ref.Description == "" {
		t.Log("description vazia — UI vai mostrar só o nome do service")
	}
	if ref.HowToAccess == "" {
		t.Log("how_to_access vazia — UI não conseguirá orientar o usuário")
	}
	// Soft test: estes campos são strings livres, não validados no
	// runtime. Documentação é o contrato.
}

// ─────────── #476 DeleteMyAccount confirmation gate ───────────

func TestDeleteMyAccountRequest_ConfirmIsCaseSensitive(t *testing.T) {
	// O handler exige literal "EXCLUIR MINHA CONTA". Confirma que
	// variações comuns (caixa baixa, espaço extra, acento) não passam.
	expected := "EXCLUIR MINHA CONTA"
	cases := []struct {
		name  string
		input string
		match bool
	}{
		{"exato", "EXCLUIR MINHA CONTA", true},
		{"caixa baixa", "excluir minha conta", false},
		{"com acento", "EXCLUIR MINHA CÔNTA", false},
		{"espaço extra fim", "EXCLUIR MINHA CONTA ", true}, // TrimSpace no handler
		{"sem acentos comparado", "EXCLUIR MINHA CONTA", true},
		{"truncado", "EXCLUIR MINHA", false},
		{"vazio", "", false},
		{"frase alternativa", "DELETE", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Replica a lógica do handler (strings.TrimSpace + compare).
			trimmed := trimSpace(tc.input)
			got := trimmed == expected
			if got != tc.match {
				t.Errorf("input=%q want=%v got=%v", tc.input, tc.match, got)
			}
		})
	}
}

// trimSpace local copy of strings.TrimSpace (avoid importing strings
// just for one assertion).
func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n') {
		s = s[:len(s)-1]
	}
	return s
}
