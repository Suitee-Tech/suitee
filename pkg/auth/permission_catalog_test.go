package auth

import (
	"strings"
	"testing"
)

// P0.2 / A6 — higiene do catálogo canônico.
//
// A revisão de permissões de 2026-09-19 mostrou que backend e frontend mantinham
// catálogos separados, divergentes em 46 de 63 permissões. A partir da correção
// o catálogo Go é a ÚNICA fonte: `pkg/cmd/gen-permission-catalog` gera o TS e
// `make check-permission-catalog` falha quando ele está defasado.
//
// Estes testes protegem a fonte: ids únicos e estáveis, papéis válidos, rótulos
// presentes e nenhuma permissão "fantasma" (usada em rota sem existir aqui).

func TestPermissionCatalog_UniqueIDsAndLabels(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range PermissionCatalog {
		if strings.TrimSpace(p.ID) == "" {
			t.Fatal("permissão sem ID no catálogo")
		}
		if seen[p.ID] {
			t.Fatalf("permissão duplicada no catálogo: %s", p.ID)
		}
		seen[p.ID] = true

		if strings.TrimSpace(p.Label) == "" {
			t.Errorf("permissão %s sem label — a tela de Permissões mostraria vazio", p.ID)
		}
		if strings.TrimSpace(p.Module) == "" {
			t.Errorf("permissão %s sem module — não apareceria agrupada na tela", p.ID)
		}
		if !strings.Contains(p.ID, ":") {
			t.Errorf("permissão %s fora do formato recurso:acao", p.ID)
		}
	}
}

func TestPermissionCatalog_DefaultRolesAreValid(t *testing.T) {
	for _, p := range PermissionCatalog {
		for role := range p.DefaultRoles {
			if !IsValidFunctionalRole(role) {
				t.Errorf("permissão %s concede papel desconhecido %q (não está em AllFunctionalRoles)", p.ID, role)
			}
		}
	}
}

// Todo papel precisa de rótulo: o gerador usa RoleLabels para o TS e o
// middleware usa AllFunctionalRoles como allowlist.
func TestRoleLabels_CoverEveryFunctionalRole(t *testing.T) {
	if len(AllFunctionalRoles) == 0 {
		t.Fatal("AllFunctionalRoles vazio")
	}
	for _, role := range AllFunctionalRoles {
		if _, ok := RoleLabels[role]; !ok {
			t.Errorf("papel %s sem rótulo em RoleLabels", role)
		}
	}
	// E o inverso: rótulo para papel que não existe é sinal de renomeação
	// incompleta.
	for role := range RoleLabels {
		if !IsValidFunctionalRole(role) {
			t.Errorf("RoleLabels tem %q, que não está em AllFunctionalRoles", role)
		}
	}
}

// Capacidades que a matriz documentada promete e que a revisão precisou alinhar
// (o backend era mais restritivo que o documento; com a rota passando a exigir a
// permissão, a omissão viraria 403 funcional).
func TestPermissionCatalog_DocumentedCapabilitiesAreGranted(t *testing.T) {
	cases := []struct {
		permission string
		wantRoles  []string
		why        string
	}{
		{"leads:import_export", []string{"admin", "gestor", "operacoes", "marketing"}, "ROLES-MATRIX.md:43"},
		{"rentals:inspect", []string{"admin", "gestor", "corretor", "operacoes"}, "ROLES-MATRIX.md:75"},
		{"contracts:sign", []string{"admin", "gestor", "operacoes", "financeiro"}, "ROLES-MATRIX.md:59"},
	}
	for _, tc := range cases {
		var def *PermissionDef
		for i := range PermissionCatalog {
			if PermissionCatalog[i].ID == tc.permission {
				def = &PermissionCatalog[i]
				break
			}
		}
		if def == nil {
			t.Fatalf("permissão %s não está no catálogo", tc.permission)
		}
		for _, role := range tc.wantRoles {
			if !def.DefaultRoles[role] {
				t.Errorf("%s deveria conceder %q por %s", tc.permission, role, tc.why)
			}
		}
	}
}

// Ids que só existiam no catálogo do frontend nunca tiveram enforcement: mantê-los
// no catálogo canônico criaria permissões decorativas (achado A7).
func TestPermissionCatalog_NoFrontendOnlyPhantoms(t *testing.T) {
	phantoms := []string{
		"leads:receive_roulette",
		"settings:manage_commission",
		"company:manage_groups",
		"company:manage_group_members",
		"properties:create",
		// P1/A7: removidas do catálogo por não governarem nada — sem rota que as
		// exija e sem consumidor no frontend.
		"leads:delete",
		"rentals:write",
		"properties:publish",
		// M3: citada numa rota do toca-financial sem existir aqui (tornava
		// `DELETE /transactions/:id` inalcançável para todo papel funcional).
		"finance:delete",
	}
	ids := map[string]bool{}
	for _, p := range PermissionCatalog {
		ids[p.ID] = true
	}
	for _, phantom := range phantoms {
		if ids[phantom] {
			t.Errorf("%s foi reintroduzida no catálogo; se a capacidade existe, ela precisa de enforcement (rota + teste)", phantom)
		}
	}
}

// TestPermissionCatalog_StableOrderAndFormat protege o contrato do gerador: a
// ordem do catálogo é a ordem de exibição na tela de Permissões, então uma
// reordenação acidental vira ruído no diff do arquivo gerado.
func TestPermissionCatalog_ModuleOrder(t *testing.T) {
	var modules []string
	seen := map[string]bool{}
	for _, p := range PermissionCatalog {
		if !seen[p.Module] {
			seen[p.Module] = true
			modules = append(modules, p.Module)
		}
	}
	if len(modules) == 0 {
		t.Fatal("catálogo sem módulos")
	}
	// "Menus" vem primeiro: a tela mostra os módulos de navegação no topo.
	if modules[0] != "Menus" {
		t.Errorf("primeiro módulo = %q, esperava \"Menus\"", modules[0])
	}
}

// P1/A7 — a capacidade "editar cobranças" da matriz inclui `operacoes`
// (docs/features/ROLES-MATRIX.md, "Cobranças — editar"). Sem isso, a tela de
// locação mostrava a `operacoes` os botões de conta de consumo / régua de
// lembretes (que o frontend gateava por `rentals:write`) e o servidor respondia
// 403, porque essas rotas são guardadas por `charges:write`.
func TestPermissionCatalog_ChargesWriteMatchesMatrix(t *testing.T) {
	for _, p := range PermissionCatalog {
		if p.ID != "charges:write" {
			continue
		}
		for _, role := range []string{"admin", "operacoes", "financeiro"} {
			if !p.DefaultRoles[role] {
				t.Errorf("charges:write deveria incluir %q (matriz: Cobranças — editar)", role)
			}
		}
		for _, role := range []string{"corretor", "marketing", "auditor", "parceiro", "cliente"} {
			if p.DefaultRoles[role] {
				t.Errorf("charges:write não deveria incluir %q", role)
			}
		}
		return
	}
	t.Fatal("charges:write não encontrada no catálogo")
}

// P1/A7 — capacidades de UI que continuam no catálogo de propósito: a exportação
// é gerada no navegador (ExportReportModal), então a permissão é a única
// enforcement possível e esconder o botão é o efeito pretendido.
func TestPermissionCatalog_ExportPermissionsAreUIOnly(t *testing.T) {
	want := map[string]bool{"finance:export": false, "reports:export": false}
	for _, p := range PermissionCatalog {
		if _, ok := want[p.ID]; ok {
			want[p.ID] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Errorf("%s saiu do catálogo: ela é a única guarda do botão de exportação no cliente (export é client-side)", id)
		}
	}
}

// P1/A8 — as permissões criadas para o chat (toca-comm) existem e a população
// delas espelha a tela (`/mensagens` exige `module:crm`: admin, gestor,
// corretor, operacoes, marketing e auditor).
func TestPermissionCatalog_MessagesPermissionsMatchChatScreen(t *testing.T) {
	want := map[string]map[string]bool{
		"messages:read":  {"admin": true, "gestor": true, "corretor": true, "operacoes": true, "marketing": true, "auditor": true},
		"messages:write": {"admin": true, "gestor": true, "corretor": true, "operacoes": true, "marketing": true},
	}
	seen := map[string]bool{}
	for _, p := range PermissionCatalog {
		expected, ok := want[p.ID]
		if !ok {
			continue
		}
		seen[p.ID] = true
		for role := range expected {
			if !p.DefaultRoles[role] {
				t.Errorf("%s deveria incluir %q", p.ID, role)
			}
		}
		for _, role := range []string{"financeiro", "parceiro", "cliente"} {
			if p.DefaultRoles[role] {
				t.Errorf("%s não deveria incluir %q (não usa a tela de chat)", p.ID, role)
			}
		}
		if p.ID == "messages:write" && p.DefaultRoles["auditor"] {
			t.Error("messages:write não deveria incluir auditor (papel de leitura)")
		}
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("%s não está no catálogo", id)
		}
	}
}

// P1/A8 — `ai:use` existe e cobre os papéis que hoje têm o painel do assistente
// (o `AIChat` é renderizado no layout do admin para todo usuário autenticado).
// `cliente` fica de fora porque as rotas do serviço rejeitam esse papel.
func TestPermissionCatalog_AIUseMatchesAssistantPopulation(t *testing.T) {
	var found bool
	for _, p := range PermissionCatalog {
		if p.ID != "ai:use" {
			continue
		}
		found = true
		for _, role := range []string{"admin", "gestor", "corretor", "operacoes", "financeiro", "marketing", "auditor", "parceiro"} {
			if !p.DefaultRoles[role] {
				t.Errorf("ai:use deveria incluir %q (o painel aparece no layout desse papel)", role)
			}
		}
		if p.DefaultRoles["cliente"] {
			t.Error("ai:use não deveria incluir cliente (as rotas do serviço rejeitam o papel)")
		}
	}
	if !found {
		t.Fatal("ai:use não está no catálogo")
	}
}
