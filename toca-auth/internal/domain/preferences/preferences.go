package preferences

import "time"

// ViewPreference holds free-form per-scope view state set by the UI (e.g. selected
// view mode, filter values, sort key). The shape is intentionally untyped because
// each scope (leads, esteira, calendar, ...) owns its own keys.
type ViewPreference map[string]interface{}

// Consent — registro de consentimentos LGPD-Art.7 do usuário. Issue #478.
//
// Por que separar em sub-struct em vez de booleans soltos?
//   - LGPD Art. 8 §1º exige que o consentimento seja DEMONSTRÁVEL. Não
//     basta o flag — precisamos do timestamp de quando foi dado/revogado
//   - a versão do termo aceito (para que, num futuro recall de
//     termo, possamos identificar quem aceitou que versão).
//   - Crescimento orgânico: hoje só "marketing"; semanas/meses à frente
//     pode entrar "compartilhamento_terceiros", "analytics_third_party",
//     "cookies_não_essenciais" etc. Sub-struct deixa adicionar sem
//     virar pasta-burra de booleans no topo de UserPreferences.
//   - Audit trail: cada flip vira uma entrada em audit.events com
//     before/after = Consent snapshot — fácil ler depois "quando o
//     usuário Y deu consentimento para X".
type Consent struct {
	// MarketingEmails — autoriza recebimento de e-mails de
	// marketing (newsletter, ofertas, anúncios de novidades).
	// Default false — opt-in explícito, padrão LGPD.
	MarketingEmails   bool       `json:"marketing_emails" bson:"marketing_emails"`
	MarketingEmailsAt *time.Time `json:"marketing_emails_at,omitempty" bson:"marketing_emails_at,omitempty"`
	// MarketingSMS — separado de e-mail porque o usuário pode
	// querer um sim e outro não (SMS tem fricção maior).
	MarketingSMS   bool       `json:"marketing_sms" bson:"marketing_sms"`
	MarketingSMSAt *time.Time `json:"marketing_sms_at,omitempty" bson:"marketing_sms_at,omitempty"`
	// MarketingWhatsApp — idem.
	MarketingWhatsApp   bool       `json:"marketing_whatsapp" bson:"marketing_whatsapp"`
	MarketingWhatsAppAt *time.Time `json:"marketing_whatsapp_at,omitempty" bson:"marketing_whatsapp_at,omitempty"`

	// TermsVersion — versão dos Termos de Uso/Política de Privacidade
	// que o usuário aceitou. Quando os termos mudarem, o caller
	// compara o atual com o salvo aqui e força re-aceite. Vazio quando
	// nunca houve aceite explícito (legacy users); o frontend deve
	// pedir confirmação na primeira visita pós-deploy de Onda 2 (#477).
	TermsVersion    string     `json:"terms_version,omitempty" bson:"terms_version,omitempty"`
	TermsAcceptedAt *time.Time `json:"terms_accepted_at,omitempty" bson:"terms_accepted_at,omitempty"`
}

type UserPreferences struct {
	ID              string                    `json:"id" bson:"-"`
	AccountID       string                    `json:"account_id" bson:"account_id"`
	Language        string                    `json:"language" bson:"language"`
	Timezone        string                    `json:"timezone" bson:"timezone"`
	DateFormat      string                    `json:"date_format" bson:"date_format"`
	SessionTimeout  int                       `json:"session_timeout" bson:"session_timeout"`
	FocusMode       bool                      `json:"focus_mode" bson:"focus_mode"`
	LastCompanyID   string                    `json:"last_company_id,omitempty" bson:"last_company_id,omitempty"`
	ViewPreferences map[string]ViewPreference `json:"view_preferences,omitempty" bson:"view_preferences,omitempty"`
	// Consent — registro LGPD de aceites do usuário. Issue #478.
	Consent Consent `json:"consent" bson:"consent"`
}

// MergeConsentChange aplica a mudança parcial em Consent e marca o
// timestamp do canal que mudou. Retorna a lista de canais que mudaram
// para que o caller possa gravar audit + acionar side effects (ex.: se
// MarketingEmails virou false, remover da lista de envio em batch).
//
// Idempotência: chamadas sem mudança real (mesmo valor) não setam
// timestamp novo — preservamos o original do primeiro consentimento.
// LGPD Art. 8 §4º: pode ser revogado a qualquer momento — quando
// flip false, marcamos o timestamp da revogação no mesmo campo (sem
// distinguir "deu/revogou" por compactação; audit log carrega before
// vs after se precisar reconstituir).
func (c *Consent) MergeConsentChange(next Consent, now time.Time) []string {
	var changed []string
	if c.MarketingEmails != next.MarketingEmails {
		c.MarketingEmails = next.MarketingEmails
		c.MarketingEmailsAt = &now
		changed = append(changed, "marketing_emails")
	}
	if c.MarketingSMS != next.MarketingSMS {
		c.MarketingSMS = next.MarketingSMS
		c.MarketingSMSAt = &now
		changed = append(changed, "marketing_sms")
	}
	if c.MarketingWhatsApp != next.MarketingWhatsApp {
		c.MarketingWhatsApp = next.MarketingWhatsApp
		c.MarketingWhatsAppAt = &now
		changed = append(changed, "marketing_whatsapp")
	}
	if next.TermsVersion != "" && next.TermsVersion != c.TermsVersion {
		c.TermsVersion = next.TermsVersion
		c.TermsAcceptedAt = &now
		changed = append(changed, "terms_version")
	}
	return changed
}
