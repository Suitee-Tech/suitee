package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/oginoino/toca-auth/internal/domain/preferences"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/logger"
)

type PreferencesService struct {
	preferencesRepo preferences.Repository
}

func NewPreferencesService(prefsRepo preferences.Repository) *PreferencesService {
	return &PreferencesService{
		preferencesRepo: prefsRepo,
	}
}

func defaultPreferences(accountID string) *preferences.UserPreferences {
	return &preferences.UserPreferences{
		AccountID:      accountID,
		Language:       "pt-BR",
		Timezone:       "America/Sao_Paulo",
		DateFormat:     "DD/MM/YYYY",
		SessionTimeout: 30,
		FocusMode:      false,
	}
}

func (s *PreferencesService) GetPreferences(ctx context.Context, accountID string) (*preferences.UserPreferences, error) {
	prefs, err := s.preferencesRepo.GetByAccountID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if prefs == nil {
		return defaultPreferences(accountID), nil
	}
	return prefs, nil
}

func (s *PreferencesService) UpdatePreferences(ctx context.Context, accountID string, req *preferences.UserPreferences) error {
	req.AccountID = accountID
	return s.preferencesRepo.Upsert(ctx, req)
}

// PatchPreferences merges only the fields present in the raw JSON payload onto
// the user's existing preferences, preserving keys the client did not send.
// For view_preferences the merge is per scope key.
func (s *PreferencesService) PatchPreferences(ctx context.Context, accountID string, patch map[string]json.RawMessage) error {
	current, err := s.preferencesRepo.GetByAccountID(ctx, accountID)
	if err != nil {
		return err
	}
	if current == nil {
		current = defaultPreferences(accountID)
	}
	current.AccountID = accountID

	if raw, ok := patch["language"]; ok {
		if err := json.Unmarshal(raw, &current.Language); err != nil {
			return err
		}
	}
	if raw, ok := patch["timezone"]; ok {
		if err := json.Unmarshal(raw, &current.Timezone); err != nil {
			return err
		}
	}
	if raw, ok := patch["date_format"]; ok {
		if err := json.Unmarshal(raw, &current.DateFormat); err != nil {
			return err
		}
	}
	if raw, ok := patch["session_timeout"]; ok {
		if err := json.Unmarshal(raw, &current.SessionTimeout); err != nil {
			return err
		}
	}
	// P2/M12 — `two_factor_enabled` era PREFERÊNCIA PURA: o usuário (ou o
	// admin) marcava 2FA, a tela exibia "2FA ativado" e nada acontecia no login
	// (não há enroll/step-up). A flag saiu do domínio; um cliente antigo que
	// ainda a envie tem o valor DESCARTADO (nunca gravado como se fosse fato) e
	// o descarte fica registrado para a transição.
	if _, ok := patch["two_factor_enabled"]; ok {
		logger.Warn("auth.preference_dropped_unsupported_2fa",
			"event", "auth.preference_dropped_unsupported_2fa",
			"account_id", current.AccountID,
			"hint", "2FA não implementado: a flag era preferência sem efeito e foi removida (achado M12)",
		)
	}
	if raw, ok := patch["focus_mode"]; ok {
		if err := json.Unmarshal(raw, &current.FocusMode); err != nil {
			return err
		}
	}
	if raw, ok := patch["last_company_id"]; ok {
		if err := json.Unmarshal(raw, &current.LastCompanyID); err != nil {
			return err
		}
	}
	if raw, ok := patch["view_preferences"]; ok {
		incoming := map[string]preferences.ViewPreference{}
		if err := json.Unmarshal(raw, &incoming); err != nil {
			return err
		}
		if current.ViewPreferences == nil {
			current.ViewPreferences = map[string]preferences.ViewPreference{}
		}
		for scope, vp := range incoming {
			current.ViewPreferences[scope] = vp
		}
	}

	// Consent (#478). Merge granular — só os canais que vieram no
	// patch mudam; o resto preserva. Auditoria por canal mudado para
	// satisfazer LGPD Art. 8 §1º (consentimento demonstrável).
	if raw, ok := patch["consent"]; ok {
		var incoming preferences.Consent
		if err := json.Unmarshal(raw, &incoming); err != nil {
			return err
		}
		before := current.Consent
		changed := current.Consent.MergeConsentChange(incoming, time.Now())
		if len(changed) > 0 {
			audit.Record(ctx, audit.Entry{
				Action:     "user.consent_changed",
				EntityType: "UserPreferences",
				EntityID:   accountID,
				Before:     consentSnapshot(before),
				After:      consentSnapshot(current.Consent),
				Metadata: map[string]any{
					"changed_fields": changed,
				},
			})
		}
	}

	return s.preferencesRepo.Upsert(ctx, current)
}

// consentSnapshot — serialização compacta de Consent para audit log.
// LGPD não permite reter consentimento desnecessariamente, mas o
// audit existe justamente para demonstrar o estado da decisão; aqui
// gravamos só os flags + timestamps que importam para reconstituição.
func consentSnapshot(c preferences.Consent) map[string]any {
	return map[string]any{
		"marketing_emails":   c.MarketingEmails,
		"marketing_sms":      c.MarketingSMS,
		"marketing_whatsapp": c.MarketingWhatsApp,
		"terms_version":      c.TermsVersion,
	}
}
