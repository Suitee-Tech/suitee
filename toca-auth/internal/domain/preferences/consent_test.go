package preferences

import (
	"testing"
	"time"
)

func TestMergeConsentChange_SetsTimestampOnFirstFlip(t *testing.T) {
	var c Consent
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	next := Consent{MarketingEmails: true}

	changed := c.MergeConsentChange(next, now)

	if len(changed) != 1 || changed[0] != "marketing_emails" {
		t.Errorf("expected ['marketing_emails'] changed, got %v", changed)
	}
	if !c.MarketingEmails {
		t.Error("MarketingEmails not applied")
	}
	if c.MarketingEmailsAt == nil || !c.MarketingEmailsAt.Equal(now) {
		t.Errorf("timestamp wrong: %v want %v", c.MarketingEmailsAt, now)
	}
}

func TestMergeConsentChange_NoChangeNoTimestampUpdate(t *testing.T) {
	original := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := Consent{MarketingEmails: true, MarketingEmailsAt: &original}

	// Mesmo valor — não deve mexer no timestamp.
	later := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	changed := c.MergeConsentChange(Consent{MarketingEmails: true}, later)

	if len(changed) != 0 {
		t.Errorf("expected no change, got %v", changed)
	}
	if !c.MarketingEmailsAt.Equal(original) {
		t.Errorf("timestamp overwritten: %v want %v", c.MarketingEmailsAt, original)
	}
}

func TestMergeConsentChange_RevocationFlipsFalse(t *testing.T) {
	original := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := Consent{MarketingEmails: true, MarketingEmailsAt: &original}

	// LGPD Art. 8 §4º — consentimento pode ser revogado a qualquer
	// momento. Flip false marca o timestamp da revogação no mesmo
	// campo (audit log distingue antes/depois via Before/After snapshot).
	revokeAt := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	changed := c.MergeConsentChange(Consent{MarketingEmails: false}, revokeAt)

	if len(changed) != 1 || changed[0] != "marketing_emails" {
		t.Errorf("expected change, got %v", changed)
	}
	if c.MarketingEmails {
		t.Error("MarketingEmails should be false after revocation")
	}
	if !c.MarketingEmailsAt.Equal(revokeAt) {
		t.Errorf("revoke timestamp wrong: %v want %v", c.MarketingEmailsAt, revokeAt)
	}
}

func TestMergeConsentChange_MultipleChannels(t *testing.T) {
	var c Consent
	now := time.Now()
	next := Consent{
		MarketingEmails:   true,
		MarketingSMS:      true,
		MarketingWhatsApp: true,
	}

	changed := c.MergeConsentChange(next, now)
	if len(changed) != 3 {
		t.Errorf("expected 3 channels changed, got %d (%v)", len(changed), changed)
	}
}

func TestMergeConsentChange_PartialUpdate(t *testing.T) {
	// Cenário: user já tem MarketingEmails=true; agora liga SMS sem
	// mexer no email. Channel changes só lista SMS.
	original := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := Consent{MarketingEmails: true, MarketingEmailsAt: &original}

	now := time.Now()
	changed := c.MergeConsentChange(Consent{MarketingEmails: true, MarketingSMS: true}, now)

	if len(changed) != 1 || changed[0] != "marketing_sms" {
		t.Errorf("expected only marketing_sms changed, got %v", changed)
	}
	if !c.MarketingEmailsAt.Equal(original) {
		t.Errorf("email timestamp should be preserved, got %v", c.MarketingEmailsAt)
	}
}

func TestMergeConsentChange_TermsVersionAccepted(t *testing.T) {
	var c Consent
	now := time.Now()
	next := Consent{TermsVersion: "2026-05-20"}

	changed := c.MergeConsentChange(next, now)

	if len(changed) != 1 || changed[0] != "terms_version" {
		t.Errorf("expected terms_version changed, got %v", changed)
	}
	if c.TermsVersion != "2026-05-20" {
		t.Errorf("TermsVersion wrong: %q", c.TermsVersion)
	}
	if c.TermsAcceptedAt == nil || !c.TermsAcceptedAt.Equal(now) {
		t.Error("TermsAcceptedAt not set")
	}
}

func TestMergeConsentChange_TermsVersionUnchanged(t *testing.T) {
	// User reabre o app, frontend manda mesma TermsVersion no patch
	// (idempotência). Não deve disparar audit nem mover timestamp.
	original := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := Consent{TermsVersion: "2026-05-20", TermsAcceptedAt: &original}

	later := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	changed := c.MergeConsentChange(Consent{TermsVersion: "2026-05-20"}, later)

	if len(changed) != 0 {
		t.Errorf("expected no change on same TermsVersion, got %v", changed)
	}
	if !c.TermsAcceptedAt.Equal(original) {
		t.Error("TermsAcceptedAt should be preserved")
	}
}

func TestMergeConsentChange_EmptyTermsVersionIgnored(t *testing.T) {
	// Patch sem TermsVersion (empty string) não deve resetar/erase
	// a versão atual — só atualizar quando explicitamente um valor
	// novo vier.
	original := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := Consent{TermsVersion: "2026-05-20", TermsAcceptedAt: &original}

	now := time.Now()
	next := Consent{TermsVersion: ""}
	changed := c.MergeConsentChange(next, now)

	if len(changed) != 0 {
		t.Errorf("empty TermsVersion should not trigger change, got %v", changed)
	}
	if c.TermsVersion != "2026-05-20" {
		t.Errorf("TermsVersion erased: %q", c.TermsVersion)
	}
}
