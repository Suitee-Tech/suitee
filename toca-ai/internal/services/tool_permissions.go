package services

import (
	"context"
	"errors"
	"strings"

	pkgAuth "github.com/oginoino/toca/pkg/auth"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/logger"
)

// P1/A8 (continuação) — autorização por TOOL do agente.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A8) registrou que a execução de tools do agente não tinha permissão
// nenhuma: a autorização repousava (a) no rank do MCP — hoje derivado de claims
// verificados (P0/C7) — e (b) nos guards dos serviços de destino.
//
// O que falta e é feito aqui: o agente poder tocar um domínio que o PAPEL do
// usuário não pode. O rank do MCP é grosso (lê/escreve); a permissão do catálogo
// é a granularidade que o tenant governa. Então toda tool de ESCRITA exige a
// permissão de escrita do domínio que ela toca, na matriz do tenant.
//
// Tools de LEITURA não entram no mapa de propósito: quem lê já passa pelos guards
// HTTP do destino, e mapear leitura aqui duplicaria decisões que o HTTP toma com
// as mesmas permissões — inclusive as isenções deliberadas (a LISTA de corretores,
// por exemplo, é dado de referência liberado a qualquer autenticado).

// ErrToolPermissionDenied é devolvido quando o papel do chamador não tem a
// permissão exigida pela tool.
var ErrToolPermissionDenied = errors.New("permission denied for this tool")

// toolWritePermissions mapeia tool de escrita → permissão do catálogo.
//
// O domínio vem da metadata da própria tool no toca-mcp
// (`toolWithMeta(t, "<domain>", "<minRole>", readOnly, destructive)`), e o teste
// `TestEveryWriteToolHasAPermission` lê aquele arquivo para garantir que uma tool
// de escrita nova não entre sem decisão explícita aqui.
var toolWritePermissions = map[string]string{
	// crm — leads
	"create_lead":               "leads:write",
	"update_lead":               "leads:write",
	"add_lead_note":             "leads:write",
	"add_lead_note_from_screen": "leads:write",

	// deals (esteira) — inclui checklist, que é mutação do negócio
	"create_deal":                        "deals:write",
	"create_deal_from_lead_and_property": "deals:write",
	"update_deal_status":                 "deals:write",
	"init_deal_checklist":                "deals:write",
	"add_deal_checklist_item":            "deals:write",
	"solicit_deal_checklist_item":        "deals:write",
	"update_deal_checklist_item_status":  "deals:write",
	"remove_deal_checklist_item":         "deals:write",

	// imóveis
	"create_property":           "properties:write",
	"add_property_owner":        "properties:write",
	"link_lead_to_property":     "properties:write",
	"unlink_lead_from_property": "properties:write",
	"set_property_active_deal":  "properties:write",

	// contratos
	"create_contract": "contracts:write",

	// agenda — as rotas de /appointments do toca-calendar usam deals:write
	"create_appointment": "deals:write",
	"update_appointment": "deals:write",

	// vistorias
	"create_inspection": "rentals:inspect",
	"update_inspection": "rentals:inspect",
}

// ToolWritePermission devolve a permissão exigida por uma tool de escrita.
func ToolWritePermission(toolName string) (string, bool) {
	perm, ok := toolWritePermissions[strings.TrimSpace(toolName)]
	return perm, ok
}

// authorizeToolCall decide se o chamador pode executar a tool.
//
// Fail-closed: tool de escrita sem permissão no mapa, sem provider configurado ou
// sem papel identificado no contexto é NEGADA. Tool de leitura passa (guards do
// destino), e uma tool desconhecida de escrita (não mapeada) também passa — com
// log `ai.tool_unmapped` — porque o rank do MCP continua valendo; o teste de
// cobertura do mapa é o que evita que isso vire buraco.
func (s *ChatService) authorizeToolCall(ctx context.Context, toolName string) error {
	perm, mapped := ToolWritePermission(toolName)
	if !mapped {
		logger.Info("ai.tool_unmapped", "event", "ai.tool_unmapped", "tool", toolName)
		return nil
	}

	if s.permissionProvider == nil {
		logger.Warn("ai.tool_permission_provider_missing",
			"event", "ai.tool_permission_provider_missing", "tool", toolName, "permission", perm)
		return ErrToolPermissionDenied
	}

	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil || strings.TrimSpace(tenantID) == "" {
		return ErrToolPermissionDenied
	}

	roles, err := s.permissionProvider.GetRolesMap(ctx, tenantID, perm)
	if err != nil {
		logger.Warn("ai.tool_permission_lookup_failed",
			"event", "ai.tool_permission_lookup_failed", "tool", toolName, "permission", perm, "error", err.Error())
		return ErrToolPermissionDenied
	}

	role, _ := pkgcontext.GetFunctionalRole(ctx)
	role = strings.ToLower(strings.TrimSpace(role))
	// O mapa é "quais papéis têm a permissão": o papel do CHAMADOR precisa estar
	// nele. O perfil root (owner/admin) já vem resolvido pelo próprio provider,
	// que devolve o papel do chamador marcado como permitido — não cabe a este
	// código inventar bypass a partir do mapa.
	if allowed, ok := roles[role]; ok && allowed {
		return nil
	}

	logger.Warn("ai.tool_permission_denied",
		"event", "ai.tool_permission_denied",
		"tool", toolName, "permission", perm, "tenant_id", tenantID, "functional_role", role)
	return ErrToolPermissionDenied
}

// WithPermissionProvider habilita a autorização por tool.
func (s *ChatService) WithPermissionProvider(p pkgAuth.PermissionProvider) *ChatService {
	s.permissionProvider = p
	return s
}
