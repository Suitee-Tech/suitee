package auth

import "testing"

// Incidente de 2026-09-21 — fonte única de "quem vê o tenant inteiro".
//
// O CRM olhava PERFIL e PAPEL; o toca-deal olhava só o perfil. Atribuir um papel
// pelo Grupo de Usuários grava `functional_role` e deixa o perfil do vínculo
// como `member` — então quem era promovido a gestor/operações via a Esteira
// vazia, e "Operações não estava vendo nada". Estes casos travam a regra nos
// DOIS eixos e o limite dela (corretor e auditor continuam na própria carteira).
func TestHasTenantWideScope(t *testing.T) {
	cases := []struct {
		profile string
		role    string
		base    string
		want    bool
		why     string
	}{
		{"member", "gestor", "", true, "papel atribuído pelo grupo, perfil do vínculo continua member"},
		{"member", "operacoes", "", true, "operações enxerga o tenant (era o 'não está vendo nada')"},
		{"member", "admin", "", true, "admin atribuído depois"},
		{"member", "manager", "", true, "papel legado vindo do eixo de papel"},
		{"manager", "corretor", "", true, "eixo de perfil"},
		{"admin", "", "", true, "perfil root sem papel"},
		{"owner", "", "", true, "dono da empresa"},
		{"gestor", "", "", true, "perfil legado carregando o nome do papel"},
		{"  GESTOR ", "  ", "", true, "caixa e espaços do JWT"},
		// Eixo do PERFIL DE ACESSO: o id é `cr_…` (não casa com o catálogo), mas o
		// papel base é o que a pessoa realmente é.
		{"member", "cr_3fd39ef5-6d9c-4f1a-af33-4a7cfa6f6301", "gestor", true, "perfil personalizado baseado em gestor"},
		{"member", "cr_abc", "admin", true, "perfil personalizado baseado em admin"},
		{"member", "cr_abc", "operacoes", true, "perfil personalizado baseado em operações"},
		{"member", "cr_abc", "corretor", false, "perfil pessoal baseado em corretor segue na própria carteira"},
		{"member", "cr_abc", "", false, "perfil sem base resolvida não eleva"},
		{"member", "corretor", "", false, "corretor é a própria carteira"},
		{"member", "", "", false, "sem papel resolvido não eleva"},
		{"", "", "", false, "sem nada não eleva"},
		{"member", "auditor", "", false, "auditor não tem visão ampla (decisão de produto, igual ao CRM)"},
		{"member", "cliente", "", false, "cliente do portal nunca vê o tenant"},
	}
	for _, tc := range cases {
		if got := HasTenantWideScope(tc.profile, tc.role, tc.base); got != tc.want {
			t.Errorf("HasTenantWideScope(%q, %q, %q) = %v, esperado %v (%s)", tc.profile, tc.role, tc.base, got, tc.want, tc.why)
		}
	}
}
