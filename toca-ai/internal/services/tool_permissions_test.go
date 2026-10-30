package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	pkgAuth "github.com/oginoino/toca/pkg/auth"
	pkgcontext "github.com/oginoino/toca/pkg/context"
)

// P1/A8 (continuação) — autorização por TOOL do agente.
//
// O rank do MCP é grosso (lê/escreve) e os guards dos destinos cobrem o que o
// usuário JÁ pode fazer por HTTP; o que faltava era o agente tocar um domínio que
// o PAPEL do usuário não pode. Toda tool de escrita passa a exigir a permissão de
// escrita do domínio na matriz do tenant.

type fakePermissionProvider struct {
	// allowed mapeia permissão → papéis permitidos.
	allowed map[string]map[string]bool
	err     error
	calls   []string
}

func (p *fakePermissionProvider) GetRolesMap(_ context.Context, _, permission string) (map[string]bool, error) {
	p.calls = append(p.calls, permission)
	if p.err != nil {
		return nil, p.err
	}
	if roles, ok := p.allowed[permission]; ok {
		return roles, nil
	}
	return map[string]bool{}, nil
}

func (p *fakePermissionProvider) GetPermissions(context.Context, string, string) ([]string, error) {
	return nil, nil
}

func ctxWithActor(tenantID, role string) context.Context {
	ctx := pkgcontext.WithTenantID(context.Background(), tenantID)
	ctx = pkgcontext.WithAccountID(ctx, "acc-1")
	if role != "" {
		ctx = pkgcontext.WithFunctionalRole(ctx, role)
	}
	return ctx
}

// Tool de escrita sem a permissão do domínio: NEGADA.
func TestAuthorizeToolCall_DeniesWriteToolWithoutPermission(t *testing.T) {
	svc := NewChatService(nil, nil, nil, nil, nil, 1, 1).
		WithPermissionProvider(&fakePermissionProvider{allowed: map[string]map[string]bool{
			"leads:write": {"admin": true, "gestor": true},
		}})

	err := svc.authorizeToolCall(ctxWithActor("tenant-1", "corretor"), "create_lead")
	if !errors.Is(err, ErrToolPermissionDenied) {
		t.Fatalf("corretor sem leads:write executaria create_lead: %v", err)
	}
}

// Com a permissão, a tool passa — e a consulta é feita com a permissão DO DOMÍNIO.
func TestAuthorizeToolCall_AllowsWriteToolWithPermission(t *testing.T) {
	provider := &fakePermissionProvider{allowed: map[string]map[string]bool{
		"leads:write": {"corretor": true},
	}}
	svc := NewChatService(nil, nil, nil, nil, nil, 1, 1).WithPermissionProvider(provider)

	if err := svc.authorizeToolCall(ctxWithActor("tenant-1", "corretor"), "create_lead"); err != nil {
		t.Fatalf("corretor com leads:write deveria executar create_lead: %v", err)
	}
	if len(provider.calls) != 1 || provider.calls[0] != "leads:write" {
		t.Fatalf("permissão consultada = %v, esperado [leads:write]", provider.calls)
	}
}

// Cada domínio de escrita mapeia para a permissão do seu domínio (não para uma
// permissão genérica): é isso que dá granularidade ao tenant.
func TestAuthorizeToolCall_UsesDomainPermission(t *testing.T) {
	for _, tc := range []struct{ tool, permission string }{
		{"create_property", "properties:write"},
		{"set_property_active_deal", "properties:write"},
		{"create_deal", "deals:write"},
		{"update_deal_status", "deals:write"},
		{"create_contract", "contracts:write"},
		{"create_inspection", "rentals:inspect"},
		{"update_inspection", "rentals:inspect"},
		{"create_appointment", "deals:write"},
		{"update_appointment", "deals:write"},
		{"update_lead", "leads:write"},
	} {
		provider := &fakePermissionProvider{allowed: map[string]map[string]bool{tc.permission: {"gestor": true}}}
		svc := NewChatService(nil, nil, nil, nil, nil, 1, 1).WithPermissionProvider(provider)
		if err := svc.authorizeToolCall(ctxWithActor("tenant-1", "gestor"), tc.tool); err != nil {
			t.Fatalf("%s deveria passar com %s: %v", tc.tool, tc.permission, err)
		}
		if len(provider.calls) != 1 || provider.calls[0] != tc.permission {
			t.Fatalf("%s consultou %v, esperado [%s]", tc.tool, provider.calls, tc.permission)
		}
	}
}

// Tool de leitura não é barrada aqui (os guards HTTP do destino decidem) e tool
// desconhecida também passa, registrada como não mapeada — o rank do MCP segue
// valendo. É o teste de cobertura do mapa que impede que isso vire buraco.
func TestAuthorizeToolCall_AllowsReadAndUnmappedTools(t *testing.T) {
	svc := NewChatService(nil, nil, nil, nil, nil, 1, 1).
		WithPermissionProvider(&fakePermissionProvider{})
	ctx := ctxWithActor("tenant-1", "corretor")

	for _, tool := range []string{"search_leads", "get_deal_by_id", "get_financial_dashboard"} {
		if err := svc.authorizeToolCall(ctx, tool); err != nil {
			t.Fatalf("leitura %s não deveria ser barrada: %v", tool, err)
		}
	}
	if err := svc.authorizeToolCall(ctx, "ferramenta_nova_desconhecida"); err != nil {
		t.Fatalf("tool não mapeada não deveria ser barrada (rank do MCP decide): %v", err)
	}
}

// Fail-closed: sem provider, sem tenant ou com erro na consulta, a tool de
// escrita é negada.
func TestAuthorizeToolCall_FailsClosed(t *testing.T) {
	t.Run("sem provider", func(t *testing.T) {
		svc := NewChatService(nil, nil, nil, nil, nil, 1, 1)
		if err := svc.authorizeToolCall(ctxWithActor("tenant-1", "admin"), "create_lead"); !errors.Is(err, ErrToolPermissionDenied) {
			t.Fatalf("sem provider deveria negar: %v", err)
		}
	})

	t.Run("sem tenant no contexto", func(t *testing.T) {
		svc := NewChatService(nil, nil, nil, nil, nil, 1, 1).
			WithPermissionProvider(&fakePermissionProvider{allowed: map[string]map[string]bool{"leads:write": {"admin": true}}})
		if err := svc.authorizeToolCall(context.Background(), "create_lead"); !errors.Is(err, ErrToolPermissionDenied) {
			t.Fatalf("sem tenant deveria negar: %v", err)
		}
	})

	t.Run("provider fora do ar", func(t *testing.T) {
		svc := NewChatService(nil, nil, nil, nil, nil, 1, 1).
			WithPermissionProvider(&fakePermissionProvider{err: errors.New("core indisponível")})
		if err := svc.authorizeToolCall(ctxWithActor("tenant-1", "admin"), "create_lead"); !errors.Is(err, ErrToolPermissionDenied) {
			t.Fatalf("erro do provider deveria negar: %v", err)
		}
	})
}

// Cobertura do mapa: toda tool que o toca-mcp declara como ESCRITA
// (`toolWithMeta(..., readOnly=false)`) precisa ter decisão explícita aqui. Uma
// tool de escrita nova sem permissão mapeada falha o teste em vez de entrar
// silenciosamente.
func TestEveryWriteToolHasAPermission(t *testing.T) {
	root := aiRepoRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "toca-mcp", "main.go"))
	if err != nil {
		t.Fatalf("ler toca-mcp/main.go: %v", err)
	}

	toolRe := regexp.MustCompile(`(?s)toolWithMeta\(mcp\.NewTool\("([a-z_]+)",.*?\),\s*"([a-z]+)",\s*"([a-z_]+)",\s*(true|false),\s*(true|false)\)`)
	matches := toolRe.FindAllStringSubmatch(string(src), -1)
	if len(matches) < 40 {
		t.Fatalf("só %d tools encontradas no toca-mcp — a varredura perdeu o alvo", len(matches))
	}

	writes := 0
	for _, m := range matches {
		name, readOnly := m[1], m[4]
		if readOnly == "true" {
			continue
		}
		writes++
		if _, ok := ToolWritePermission(name); !ok {
			t.Errorf("tool de escrita %q não tem permissão mapeada — decida o domínio dela em toolWritePermissions", name)
		}
	}
	if writes < 15 {
		t.Fatalf("só %d tools de escrita encontradas — esperado ~22", writes)
	}
}

// aiRepoRoot sobe até a raiz do monorepo (go.work).
func aiRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 12; i++ {
		// go.work é gerado localmente e não é versionado; o checkout público
		// tem .git como marcador estável da raiz do monorepo.
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("go.work não encontrado a partir de %s", dir)
	return ""
}

var _ pkgAuth.PermissionProvider = (*fakePermissionProvider)(nil)
