package service

import (
	"context"
	"fmt"

	"github.com/oginoino/toca/toca-comm/internal/repository"
)

// EmailPrefsService expõe as preferências de e-mail transacional por
// usuário/categoria (beta F4 — comm.notification_prefs). A ausência de linha
// significa "ligado".
type EmailPrefsService struct {
	prefs repository.EmailPrefsRepository
}

// EmailPrefCategory é a visão pública de uma categoria: estado atual + label
// pt-BR + locked (não desligável para aquela persona).
type EmailPrefCategory struct {
	Category string `json:"category"`
	Label    string `json:"label"`
	Enabled  bool   `json:"enabled"`
	Locked   bool   `json:"locked"`
}

var emailPrefCategories = []struct {
	Category EmailCategory
	Label    string
}{
	{CategoryLeadAssigned, "Novos leads atribuídos"},
	{CategoryDealUpdates, "Atualizações do negócio"},
	{CategoryProposals, "Propostas e contrapropostas"},
	{CategoryAppointments, "Agenda e visitas"},
	{CategoryRecommendations, "Sugestões do corretor"},
	{CategoryChatReminders, "Lembretes de mensagens não lidas"},
}

// EmailPrefCategories é a lista canônica de categorias (ordem de exibição).
func EmailPrefCategories() []EmailCategory {
	out := make([]EmailCategory, 0, len(emailPrefCategories))
	for _, c := range emailPrefCategories {
		out = append(out, c.Category)
	}
	return out
}

func emailPrefLabel(category EmailCategory) string {
	for _, c := range emailPrefCategories {
		if c.Category == category {
			return c.Label
		}
	}
	return string(category)
}

// IsEmailPrefCategoryLocked aplica a regra de negócio do beta: cliente do
// portal não desliga "appointments" (compromisso assumido); corretores/
// operadores não desligam "lead_assigned" (canal de trabalho). Outras
// categorias são opt-out livre.
func IsEmailPrefCategoryLocked(category EmailCategory, functionalRole string) bool {
	switch category {
	case CategoryAppointments:
		return functionalRole == "cliente"
	case CategoryLeadAssigned:
		return functionalRole != "cliente"
	default:
		return false
	}
}

func NewEmailPrefsService(prefs repository.EmailPrefsRepository) *EmailPrefsService {
	return &EmailPrefsService{prefs: prefs}
}

// List devolve todas as categorias com o estado atual do usuário.
func (s *EmailPrefsService) List(ctx context.Context, userID, tenantID, functionalRole string) ([]EmailPrefCategory, error) {
	out := make([]EmailPrefCategory, 0, len(emailPrefCategories))
	for _, c := range emailPrefCategories {
		enabled := true
		if s.prefs != nil {
			ok, err := s.prefs.IsEnabled(ctx, userID, tenantID, string(c.Category))
			if err != nil {
				return nil, fmt.Errorf("read email pref %s: %w", c.Category, err)
			}
			enabled = ok
		}
		out = append(out, EmailPrefCategory{
			Category: string(c.Category),
			Label:    c.Label,
			Enabled:  enabled,
			Locked:   IsEmailPrefCategoryLocked(c.Category, functionalRole),
		})
	}
	return out, nil
}

// Update aplica os toggles pedidos. Categorias desconhecidas são rejeitadas;
// categorias travadas para a persona são ignoradas (defesa em profundidade —
// a UI já esconde o toggle).
func (s *EmailPrefsService) Update(ctx context.Context, userID, tenantID, functionalRole string, updates map[string]bool) ([]EmailPrefCategory, error) {
	for raw, enabled := range updates {
		var category EmailCategory
		found := false
		for _, c := range emailPrefCategories {
			if string(c.Category) == raw {
				category = c.Category
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown email pref category: %s", raw)
		}
		if IsEmailPrefCategoryLocked(category, functionalRole) {
			continue
		}
		if s.prefs != nil {
			if err := s.prefs.Set(ctx, userID, tenantID, raw, enabled); err != nil {
				return nil, fmt.Errorf("write email pref %s: %w", raw, err)
			}
		}
	}
	return s.List(ctx, userID, tenantID, functionalRole)
}
