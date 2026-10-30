package services

import (
	"context"
	"strings"

	"github.com/oginoino/toca-core/internal/domain/core/settings"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/auth"
)

// Perfis de acesso personalizados — a segunda metade de "Permissões e Acesso".
//
// Antes desta entrega os perfis eram um JSON decorativo dentro das configurações
// (`CustomRole` com nome/cor/papel-base) e NADA consultava aquilo: o operador
// criava "Corretor Sênior" na tela de Permissões, o toast dizia "criado com
// sucesso" e a autorização continuava saindo do papel do catálogo do membro.
// Agora o perfil:
//
//   - carrega o conjunto EFETIVO de permissões (settings.CustomRole.Permissions);
//   - é a unidade ATRIBUÍVEL por um Grupo de Usuários (o grupo aponta para o
//     perfil e o toca-auth grava esse id como `functional_role` do membro);
//   - é decidido no MESMO lugar que os papéis do catálogo
//     (settings.RolePermissionsFor), inclusive para o enforcement do core.
//
// Todas as escritas passam por aqui e: validam contra o catálogo canônico,
// auditam (M5) e invalidam o cache de permissões do tenant (A5) — sem isso, um
// perfil alterado continuaria valendo a versão antiga por até 30 s no cache dos
// consumidores.

// AccessProfileInput é o payload de criação/edição de um perfil de acesso.
type AccessProfileInput struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	// BaseRole é o papel do catálogo usado como ponto de partida (semente das
	// permissões e rótulo "Baseado em"). Opcional na edição.
	BaseRole string `json:"baseRole"`
	Color    string `json:"color"`
	// Permissions — conjunto efetivo concedido ao perfil. `nil` na criação
	// significa "copiar do papel base" (o efetivo do tenant, não o default do
	// catálogo); `nil` na edição preserva o conjunto atual.
	Permissions []string `json:"permissions"`
}

// permissionsDocument carrega o documento de permissões do tenant, criando o
// default em memória quando ainda não existe (mesma regra de GetPermissions).
func (s *SettingsService) permissionsDocument(ctx context.Context, tenantID string) (*settings.RolePermissions, error) {
	doc, err := s.permissionsRepo.GetByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		doc = &settings.RolePermissions{
			TenantID:    tenantID,
			DefaultRole: "corretor",
			Permissions: settings.BuildDefaultMatrix(),
		}
	}
	if doc.DefaultRole == "" {
		doc.DefaultRole = "corretor"
	}
	return doc, nil
}

// GetAccessProfileBaseRole devolve o papel do catálogo em que o PERFIL DE
// ACESSO se baseia, ou "" quando o id não é um perfil do tenant (papel do
// catálogo, perfil apagado, etc.).
//
// É o que permite ao frontend decidir visibilidade (carteira x tenant) com a
// MESMA regra do servidor (`pkg/auth.HasTenantWideScope`) para quem tem perfil
// personalizado.
func (s *SettingsService) GetAccessProfileBaseRole(ctx context.Context, tenantID, profileID string) (string, error) {
	if !auth.IsAccessProfileID(profileID) {
		return "", nil
	}
	doc, err := s.permissionsDocument(ctx, tenantID)
	if err != nil {
		return "", err
	}
	if profile, ok := settings.FindCustomRole(doc.CustomRoles, profileID); ok {
		return profile.BaseRole, nil
	}
	return "", nil
}

// ListAccessProfiles devolve os perfis de acesso do tenant.
func (s *SettingsService) ListAccessProfiles(ctx context.Context, tenantID string) ([]settings.CustomRole, error) {
	doc, err := s.permissionsDocument(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if doc.CustomRoles == nil {
		return []settings.CustomRole{}, nil
	}
	return doc.CustomRoles, nil
}

// CreateAccessProfile cria um perfil de acesso no tenant.
//
// Sem `permissions` no payload, o perfil nasce como CÓPIA do papel base efetivo
// do tenant (o que a matriz daquele tenant concede àquele papel) — é o que o
// operador espera de "Baseado em: Corretor": um ponto de partida igual ao
// corretor de hoje, que ele então ajusta.
func (s *SettingsService) CreateAccessProfile(ctx context.Context, tenantID string, in AccessProfileInput) (*settings.CustomRole, error) {
	doc, err := s.permissionsDocument(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	baseRole := strings.ToLower(strings.TrimSpace(in.BaseRole))
	if baseRole == "" {
		baseRole = "corretor"
	}
	if !auth.IsValidFunctionalRole(baseRole) {
		return nil, &settings.InvalidAccessProfileError{UnknownBaseRoles: []string{baseRole}}
	}

	permissions := in.Permissions
	if permissions == nil {
		permissions = settings.RolePermissionsFor(doc.Permissions, nil, baseRole)
	}

	profile := settings.CustomRole{
		ID:          settings.NewCustomRoleID(),
		Name:        in.Name,
		Description: in.Description,
		BaseRole:    baseRole,
		Color:       in.Color,
		Permissions: permissions,
	}

	profiles, err := settings.NormalizeCustomRoles(append(doc.CustomRoles, profile), settings.CatalogIDs())
	if err != nil {
		return nil, err
	}
	doc.CustomRoles = profiles

	if err := s.permissionsRepo.Upsert(ctx, doc); err != nil {
		return nil, err
	}

	s.invalidatePermissions(ctx, tenantID, "settings.access_profile_created")

	created, _ := settings.FindCustomRole(profiles, profile.ID)
	audit.Record(ctx, audit.Entry{
		Action:     "settings.access_profile_created",
		EntityType: "access_profile",
		EntityID:   profile.ID,
		After: map[string]any{
			"name":        created.Name,
			"base_role":   created.BaseRole,
			"permissions": created.Permissions,
			"tenant_id":   tenantID,
		},
	})

	return &created, nil
}

// UpdateAccessProfile edita nome/descrição/cor/papel base e, quando enviado, o
// conjunto de permissões do perfil. O ID nunca muda (é o que está gravado no
// vínculo dos membros e no grupo): trocar o id de um perfil existente seria
// reescrever autorização sem trilha.
func (s *SettingsService) UpdateAccessProfile(ctx context.Context, tenantID, profileID string, in AccessProfileInput) (*settings.CustomRole, error) {
	doc, err := s.permissionsDocument(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	before, ok := settings.FindCustomRole(doc.CustomRoles, profileID)
	if !ok {
		return nil, &settings.ErrAccessProfileNotFound{ID: profileID}
	}

	updated := before
	updated.Name = in.Name
	updated.Description = in.Description
	updated.Color = in.Color
	if baseRole := strings.ToLower(strings.TrimSpace(in.BaseRole)); baseRole != "" {
		updated.BaseRole = baseRole
	}
	if in.Permissions != nil {
		updated.Permissions = in.Permissions
	}

	profiles := make([]settings.CustomRole, 0, len(doc.CustomRoles))
	for _, p := range doc.CustomRoles {
		if p.ID == profileID {
			profiles = append(profiles, updated)
			continue
		}
		profiles = append(profiles, p)
	}

	normalized, err := settings.NormalizeCustomRoles(profiles, settings.CatalogIDs())
	if err != nil {
		return nil, err
	}
	doc.CustomRoles = normalized

	if err := s.permissionsRepo.Upsert(ctx, doc); err != nil {
		return nil, err
	}

	s.invalidatePermissions(ctx, tenantID, "settings.access_profile_updated")

	after, _ := settings.FindCustomRole(normalized, profileID)
	audit.Record(ctx, audit.Entry{
		Action:     "settings.access_profile_updated",
		EntityType: "access_profile",
		EntityID:   profileID,
		Before: map[string]any{
			"name":        before.Name,
			"base_role":   before.BaseRole,
			"permissions": before.Permissions,
		},
		After: map[string]any{
			"name":        after.Name,
			"base_role":   after.BaseRole,
			"permissions": after.Permissions,
			"tenant_id":   tenantID,
		},
	})

	return &after, nil
}

// DeleteAccessProfile remove o perfil do tenant e, com ele, todo o acesso que
// ele concedia.
//
// O vínculo dos membros é do toca-auth e NÃO é reescrito aqui (core não conhece
// pessoas): quem tem o perfil apagado passa a não ter permissão nenhuma —
// fail-closed, nunca herança do `baseRole`. A tela de Permissões mostra quantos
// usuários/grupos usam o perfil ANTES de excluir e oferece a reatribuição; a
// lista de Equipe marca quem ficou com "perfil removido" para o reparo ser
// visível em vez de silencioso.
func (s *SettingsService) DeleteAccessProfile(ctx context.Context, tenantID, profileID string) error {
	doc, err := s.permissionsDocument(ctx, tenantID)
	if err != nil {
		return err
	}

	removed, ok := settings.FindCustomRole(doc.CustomRoles, profileID)
	if !ok {
		return &settings.ErrAccessProfileNotFound{ID: profileID}
	}

	profiles := make([]settings.CustomRole, 0, len(doc.CustomRoles))
	for _, p := range doc.CustomRoles {
		if p.ID == profileID {
			continue
		}
		profiles = append(profiles, p)
	}
	doc.CustomRoles = profiles

	// O papel padrão do tenant não pode continuar apontando para um perfil que
	// deixou de existir — todo usuário novo entraria sem permissão nenhuma.
	if doc.DefaultRole == profileID {
		doc.DefaultRole = "corretor"
	}

	if err := s.permissionsRepo.Upsert(ctx, doc); err != nil {
		return err
	}

	s.invalidatePermissions(ctx, tenantID, "settings.access_profile_deleted")

	audit.Record(ctx, audit.Entry{
		Action:     "settings.access_profile_deleted",
		EntityType: "access_profile",
		EntityID:   profileID,
		Before: map[string]any{
			"name":        removed.Name,
			"base_role":   removed.BaseRole,
			"permissions": removed.Permissions,
		},
		Metadata: map[string]any{"tenant_id": tenantID},
	})

	return nil
}
