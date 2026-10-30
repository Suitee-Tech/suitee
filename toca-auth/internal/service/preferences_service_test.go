package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/oginoino/toca-auth/internal/domain/preferences"
)

type fakePrefsRepo struct {
	mu   sync.Mutex
	data map[string]*preferences.UserPreferences
}

func newFakePrefsRepo() *fakePrefsRepo {
	return &fakePrefsRepo{data: map[string]*preferences.UserPreferences{}}
}

func (r *fakePrefsRepo) GetByAccountID(_ context.Context, accountID string) (*preferences.UserPreferences, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	prefs, ok := r.data[accountID]
	if !ok {
		return nil, nil
	}
	// deep copy to mimic persistence boundary
	raw, _ := json.Marshal(prefs)
	clone := &preferences.UserPreferences{}
	_ = json.Unmarshal(raw, clone)
	return clone, nil
}

func (r *fakePrefsRepo) Upsert(_ context.Context, prefs *preferences.UserPreferences) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	raw, _ := json.Marshal(prefs)
	clone := &preferences.UserPreferences{}
	_ = json.Unmarshal(raw, clone)
	r.data[prefs.AccountID] = clone
	return nil
}

func TestPatchPreferences_PreservesUntouchedFields(t *testing.T) {
	repo := newFakePrefsRepo()
	svc := NewPreferencesService(repo)
	ctx := context.Background()
	const accountID = "acc-1"

	// seed: focus_mode=true and language already set
	if err := svc.UpdatePreferences(ctx, accountID, &preferences.UserPreferences{
		AccountID: accountID,
		Language:  "pt-BR",
		FocusMode: true,
	}); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	// patch only view_preferences for "leads"
	patch := mustRawJSON(t, map[string]any{
		"view_preferences": map[string]any{
			"leads": map[string]any{"viewMode": "list"},
		},
	})
	if err := svc.PatchPreferences(ctx, accountID, patch); err != nil {
		t.Fatalf("patch failed: %v", err)
	}

	got, err := svc.GetPreferences(ctx, accountID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if !got.FocusMode {
		t.Error("expected focus_mode to remain true after partial patch")
	}
	if got.Language != "pt-BR" {
		t.Errorf("expected language pt-BR preserved, got %q", got.Language)
	}
	if got.ViewPreferences["leads"]["viewMode"] != "list" {
		t.Errorf("expected leads.viewMode=list, got %+v", got.ViewPreferences)
	}
}

func TestPatchPreferences_MergesViewPreferencesByScope(t *testing.T) {
	repo := newFakePrefsRepo()
	svc := NewPreferencesService(repo)
	ctx := context.Background()
	const accountID = "acc-2"

	// seed two scopes with the same arbitrary shape the UI sends
	patch1 := mustRawJSON(t, map[string]any{
		"view_preferences": map[string]any{
			"leads":   map[string]any{"viewMode": "list", "statusFilter": "Todos"},
			"esteira": map[string]any{"viewMode": "kanban", "sortBy": "recent"},
		},
	})
	if err := svc.PatchPreferences(ctx, accountID, patch1); err != nil {
		t.Fatalf("seed patch failed: %v", err)
	}

	// patch only "leads" — must not wipe "esteira"
	patch2 := mustRawJSON(t, map[string]any{
		"view_preferences": map[string]any{
			"leads": map[string]any{"viewMode": "board"},
		},
	})
	if err := svc.PatchPreferences(ctx, accountID, patch2); err != nil {
		t.Fatalf("second patch failed: %v", err)
	}

	got, err := svc.GetPreferences(ctx, accountID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.ViewPreferences["leads"]["viewMode"] != "board" {
		t.Errorf("expected leads.viewMode=board, got %v", got.ViewPreferences["leads"]["viewMode"])
	}
	if got.ViewPreferences["esteira"]["viewMode"] != "kanban" {
		t.Errorf("expected esteira.viewMode preserved, got %+v", got.ViewPreferences["esteira"])
	}
	if got.ViewPreferences["esteira"]["sortBy"] != "recent" {
		t.Errorf("expected esteira.sortBy preserved, got %v", got.ViewPreferences["esteira"]["sortBy"])
	}
}

func TestPatchPreferences_TogglesBoolFalse(t *testing.T) {
	repo := newFakePrefsRepo()
	svc := NewPreferencesService(repo)
	ctx := context.Background()
	const accountID = "acc-3"

	if err := svc.UpdatePreferences(ctx, accountID, &preferences.UserPreferences{
		AccountID: accountID,
		FocusMode: true,
	}); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	patch := mustRawJSON(t, map[string]any{"focus_mode": false})
	if err := svc.PatchPreferences(ctx, accountID, patch); err != nil {
		t.Fatalf("patch failed: %v", err)
	}

	got, _ := svc.GetPreferences(ctx, accountID)
	if got.FocusMode {
		t.Error("expected focus_mode=false after patch with explicit false")
	}
}

func TestPatchPreferences_LastCompanyID(t *testing.T) {
	repo := newFakePrefsRepo()
	svc := NewPreferencesService(repo)
	ctx := context.Background()
	const accountID = "acc-4"

	patch := mustRawJSON(t, map[string]any{"last_company_id": "company-xyz"})
	if err := svc.PatchPreferences(ctx, accountID, patch); err != nil {
		t.Fatalf("patch failed: %v", err)
	}
	got, _ := svc.GetPreferences(ctx, accountID)
	if got.LastCompanyID != "company-xyz" {
		t.Errorf("expected last_company_id=company-xyz, got %q", got.LastCompanyID)
	}
}

// ───────────── #478 Consent ─────────────

func TestPatchPreferences_ConsentOptIn(t *testing.T) {
	repo := newFakePrefsRepo()
	svc := NewPreferencesService(repo)
	ctx := context.Background()
	const accountID = "acc-consent-1"

	patch := mustRawJSON(t, map[string]any{
		"consent": map[string]any{
			"marketing_emails":   true,
			"marketing_whatsapp": true,
		},
	})
	if err := svc.PatchPreferences(ctx, accountID, patch); err != nil {
		t.Fatalf("patch consent failed: %v", err)
	}
	got, _ := svc.GetPreferences(ctx, accountID)
	if !got.Consent.MarketingEmails {
		t.Error("marketing_emails not set")
	}
	if !got.Consent.MarketingWhatsApp {
		t.Error("marketing_whatsapp not set")
	}
	if got.Consent.MarketingSMS {
		t.Error("marketing_sms should remain default false (não veio no patch)")
	}
	if got.Consent.MarketingEmailsAt == nil {
		t.Error("MarketingEmailsAt timestamp não setado")
	}
}

func TestPatchPreferences_ConsentRevocation(t *testing.T) {
	repo := newFakePrefsRepo()
	svc := NewPreferencesService(repo)
	ctx := context.Background()
	const accountID = "acc-consent-revoke"

	// Opt-in primeiro.
	patchOptIn := mustRawJSON(t, map[string]any{
		"consent": map[string]any{"marketing_emails": true},
	})
	if err := svc.PatchPreferences(ctx, accountID, patchOptIn); err != nil {
		t.Fatalf("opt-in: %v", err)
	}

	// Revoga.
	patchRevoke := mustRawJSON(t, map[string]any{
		"consent": map[string]any{"marketing_emails": false},
	})
	if err := svc.PatchPreferences(ctx, accountID, patchRevoke); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	got, _ := svc.GetPreferences(ctx, accountID)
	if got.Consent.MarketingEmails {
		t.Error("marketing_emails deveria estar revoked")
	}
	if got.Consent.MarketingEmailsAt == nil {
		t.Error("MarketingEmailsAt deveria ter sido atualizado para o momento da revogação")
	}
}

func TestPatchPreferences_ConsentIdempotent(t *testing.T) {
	// Patch repetido com mesmo valor não deve mexer timestamp.
	repo := newFakePrefsRepo()
	svc := NewPreferencesService(repo)
	ctx := context.Background()
	const accountID = "acc-consent-idempotent"

	patch := mustRawJSON(t, map[string]any{
		"consent": map[string]any{"marketing_emails": true},
	})
	if err := svc.PatchPreferences(ctx, accountID, patch); err != nil {
		t.Fatalf("first patch: %v", err)
	}
	first, _ := svc.GetPreferences(ctx, accountID)
	firstTimestamp := *first.Consent.MarketingEmailsAt

	// Mesma chamada — timestamp não pode mover.
	if err := svc.PatchPreferences(ctx, accountID, patch); err != nil {
		t.Fatalf("idempotent patch: %v", err)
	}
	second, _ := svc.GetPreferences(ctx, accountID)
	if !second.Consent.MarketingEmailsAt.Equal(firstTimestamp) {
		t.Errorf("timestamp sobrescrito: %v vs %v", second.Consent.MarketingEmailsAt, firstTimestamp)
	}
}

func TestPatchPreferences_ConsentTermsAcceptance(t *testing.T) {
	repo := newFakePrefsRepo()
	svc := NewPreferencesService(repo)
	ctx := context.Background()
	const accountID = "acc-terms"

	patch := mustRawJSON(t, map[string]any{
		"consent": map[string]any{"terms_version": "2026-05-20"},
	})
	if err := svc.PatchPreferences(ctx, accountID, patch); err != nil {
		t.Fatalf("patch: %v", err)
	}
	got, _ := svc.GetPreferences(ctx, accountID)
	if got.Consent.TermsVersion != "2026-05-20" {
		t.Errorf("TermsVersion: %q", got.Consent.TermsVersion)
	}
	if got.Consent.TermsAcceptedAt == nil {
		t.Error("TermsAcceptedAt não setado")
	}
}

func TestPatchPreferences_ConsentDoesNotErasePreferences(t *testing.T) {
	// Patch só de consent não pode zerar Language/Timezone/etc.
	repo := newFakePrefsRepo()
	svc := NewPreferencesService(repo)
	ctx := context.Background()
	const accountID = "acc-consent-preserve"

	// Seed prefs diversas.
	if err := svc.UpdatePreferences(ctx, accountID, &preferences.UserPreferences{
		AccountID: accountID,
		Language:  "pt-BR",
		Timezone:  "America/Fortaleza",
		FocusMode: true,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	patch := mustRawJSON(t, map[string]any{
		"consent": map[string]any{"marketing_emails": true},
	})
	if err := svc.PatchPreferences(ctx, accountID, patch); err != nil {
		t.Fatalf("patch: %v", err)
	}

	got, _ := svc.GetPreferences(ctx, accountID)
	if got.Language != "pt-BR" {
		t.Errorf("Language perdida: %q", got.Language)
	}
	if got.Timezone != "America/Fortaleza" {
		t.Errorf("Timezone perdida: %q", got.Timezone)
	}
	if !got.FocusMode {
		t.Error("FocusMode perdido")
	}
}

func mustRawJSON(t *testing.T, v map[string]any) map[string]json.RawMessage {
	t.Helper()
	out := map[string]json.RawMessage{}
	for k, val := range v {
		raw, err := json.Marshal(val)
		if err != nil {
			t.Fatalf("marshal %s: %v", k, err)
		}
		out[k] = raw
	}
	return out
}

// M12 — `two_factor_enabled` era preferência SEM EFEITO: a tela dizia "2FA
// ativado" e nada acontecia no login (não existe enroll/step-up).
//
// Contrato novo: a flag saiu do domínio; um cliente antigo que ainda a envie tem
// o valor DESCARTADO (nunca persistido como se fosse fato) e o descarte fica
// registrado para a transição.
func TestPatchPreferences_DropsUnsupportedTwoFactorFlag(t *testing.T) {
	repo := newFakePrefsRepo()
	svc := NewPreferencesService(repo)
	ctx := context.Background()
	accountID := "acc-m12"

	patch := map[string]json.RawMessage{
		"language":           json.RawMessage(`"en-US"`),
		"two_factor_enabled": json.RawMessage(`true`),
	}
	if err := svc.PatchPreferences(ctx, accountID, patch); err != nil {
		t.Fatalf("PatchPreferences: %v", err)
	}

	stored, err := repo.GetByAccountID(ctx, accountID)
	if err != nil || stored == nil {
		t.Fatalf("preferências não persistidas: %v", err)
	}
	if stored.Language != "en-US" {
		t.Fatalf("campo suportado deveria ter sido aplicado: %q", stored.Language)
	}

	// A preferência persistida não pode carregar a afirmação de 2FA.
	raw, _ := json.Marshal(stored)
	if strings.Contains(string(raw), "two_factor_enabled") {
		t.Fatalf("flag de 2FA persistida sem implementação: %s", raw)
	}
}

// O retrato devolvido pela API também não anuncia 2FA.
func TestPreferences_ResponseDoesNotClaimTwoFactor(t *testing.T) {
	svc := NewPreferencesService(newFakePrefsRepo())
	prefs, err := svc.GetPreferences(context.Background(), "acc-1")
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	raw, _ := json.Marshal(prefs)
	if strings.Contains(string(raw), "two_factor") {
		t.Fatalf("resposta ainda anuncia 2FA: %s", raw)
	}
}
