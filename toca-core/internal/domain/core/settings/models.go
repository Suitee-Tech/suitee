package settings

import (
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/auth"
)

type TimeWindow struct {
	Start string `json:"start" bson:"start"`
	End   string `json:"end" bson:"end"`
}

type BusinessHours struct {
	Timezone string                `json:"timezone,omitempty" bson:"timezone,omitempty"`
	Windows  map[string]TimeWindow `json:"windows,omitempty" bson:"windows,omitempty"`
}

type CompanySettings struct {
	ID            interface{}    `json:"id" bson:"_id,omitempty"`
	TenantID      string         `json:"tenant_id" bson:"tenant_id"`
	CRECI         string         `json:"creci" bson:"creci"`
	Website       string         `json:"website" bson:"website"`
	OpeningHours  string         `json:"horarioAbertura" bson:"horario_abertura"`
	ClosingHours  string         `json:"horarioFechamento" bson:"horario_fechamento"`
	OperatingDays []string       `json:"diasOperacao" bson:"dias_operacao"`
	BusinessHours *BusinessHours `json:"businessHours,omitempty" bson:"business_hours,omitempty"`
	LogoURL       string         `json:"logoPreview" bson:"logo_url"`
	Currency      string         `json:"currency" bson:"currency"`
	// Timezone é o fuso da IMOBILIÁRIA (Configurações → Geral → "Fuso horário").
	// Manda nos dias civis dos fluxos (lançamentos, relatórios, agendamentos):
	// sem ele o sistema usaria o dia UTC e uma baixa às 22h em Brasília cairia
	// no dia seguinte. Vazio = padrão do ambiente (America/Sao_Paulo).
	Timezone      string `json:"timezone,omitempty" bson:"timezone,omitempty"`
	AutoSave      bool   `json:"autoSave" bson:"auto_save"`
	CompactTables bool   `json:"compactTables" bson:"compact_tables"`
}

type AppearanceSettings struct {
	ID              interface{} `json:"id" bson:"_id,omitempty"`
	TenantID        string      `json:"tenant_id" bson:"tenant_id"`
	PrimaryColor    string      `json:"primaryColor" bson:"primary_color"`
	AccentColor     string      `json:"accentColor" bson:"accent_color"`
	SidebarStyle    string      `json:"sidebarStyle" bson:"sidebar_style"`
	Theme           string      `json:"theme" bson:"theme"`
	FontFamily      string      `json:"fontFamily" bson:"font_family"`
	IconStyle       string      `json:"iconStyle" bson:"icon_style"`
	ShowLogoLogin   bool        `json:"showLogoLogin" bson:"show_logo_login"`
	DarkModeEnabled bool        `json:"darkModeEnabled" bson:"dark_mode_enabled"`
	CustomDomain    string      `json:"customDomain" bson:"custom_domain"`
	DomainVerified  bool        `json:"domainVerified" bson:"domain_verified"`
	LoginTagline    string      `json:"loginTagline" bson:"login_tagline"`
	// LoginSubtitle é a segunda linha do painel de login; LoginBannerPreset é o
	// id de um preset de toka/src/shared/loginBanners.ts e LoginBannerUrl é a
	// imagem enviada pela imobiliária. Todos são white-label e valem para todos
	// os usuários do tenant.
	//
	// LoginBannerSource diz QUAL das duas imagens está ativa
	// (LoginBannerSourceCustom | LoginBannerSourcePreset). Foi separado da URL
	// porque a imobiliária precisa poder alternar entre a imagem enviada e um
	// preset sem perder a enviada: antes, voltar para um preset apagava a URL e
	// obrigava a subir o arquivo de novo. Vazio = registro anterior a este
	// campo, resolvido pela regra de compatibilidade em
	// `EffectiveLoginBannerSource` (URL preenchida → custom).
	//
	// Não há migration: `core.appearance_settings.metadata` é JSONB e
	// appearance_postgres.go serializa este struct inteiro.
	LoginSubtitle     string `json:"loginSubtitle" bson:"login_subtitle"`
	LoginBannerPreset string `json:"loginBannerPreset" bson:"login_banner_preset"`
	LoginBannerURL    string `json:"loginBannerUrl" bson:"login_banner_url"`
	LoginBannerSource string `json:"loginBannerSource" bson:"login_banner_source"`
	EmailFooter       string `json:"emailFooter" bson:"email_footer"`
}

type Permission struct {
	ID     string          `json:"id" bson:"id"`
	Label  string          `json:"label" bson:"label"`
	Module string          `json:"module" bson:"module"`
	Roles  map[string]bool `json:"roles" bson:"roles"`
}

// CustomRole é um PERFIL DE ACESSO personalizado do tenant — a unidade de
// autorização criada em Configurações → Permissões → Perfis Personalizados e
// atribuída às pessoas por um Grupo de Usuários (toca-auth grava o id do perfil
// como `functional_role` do membro).
//
// Antes disso o perfil era DECORATIVO: guardava só nome/cor/papel-base e nada
// consultava `Permissions` — o operador criava "Corretor Sênior", a tela dizia
// que estava criado e a autorização continuava saindo do papel do catálogo.
// Agora o perfil carrega o conjunto EFETIVO de permissões e é ele que decide
// (`EffectivePermissionsForRole`), no mesmo caminho do enforcement.
type CustomRole struct {
	ID          string `json:"id" bson:"id"`
	Name        string `json:"name" bson:"name"`
	Description string `json:"description" bson:"description"`
	// BaseRole é o papel do catálogo de onde o editor parte (dá nome à herança
	// inicial e ao rótulo "Baseado em"). NÃO é consultado na decisão: um perfil
	// concede exatamente o que está em `Permissions`, nem mais nem menos.
	BaseRole string `json:"baseRole" bson:"base_role"`
	Color    string `json:"color" bson:"color"`
	// Permissions são os IDs do catálogo concedidos ao perfil (conjunto
	// explícito, efetivo). IDs fora do catálogo são descartados na validação.
	Permissions []string `json:"permissions" bson:"permissions"`
}

// Grants informa se o perfil concede a permissão pedida.
func (c CustomRole) Grants(permissionID string) bool {
	for _, id := range c.Permissions {
		if id == permissionID {
			return true
		}
	}
	return false
}

// NewCustomRoleID gera o id de um perfil de acesso. O prefixo `cr_`
// (pkg/auth.AccessProfilePrefix) é reservado: um perfil nunca colide com — nem
// se passa por — um papel do catálogo.
func NewCustomRoleID() string {
	return auth.AccessProfilePrefix + uuid.NewString()
}

type RolePermissions struct {
	ID          interface{}  `json:"id" bson:"_id,omitempty"`
	TenantID    string       `json:"tenant_id" bson:"tenant_id"`
	DefaultRole string       `bson:"default_role" json:"defaultRole"` // NOVO
	Permissions []Permission `json:"permissions" bson:"permissions"`
	CustomRoles []CustomRole `json:"customRoles" bson:"custom_roles"`
}

type NotificationChannel struct {
	ID       string `json:"id" bson:"id"`
	Label    string `json:"label" bson:"label"`
	Email    bool   `json:"email" bson:"email"`
	WhatsApp bool   `json:"whatsapp" bson:"whatsapp"`
	InApp    bool   `json:"inApp" bson:"in_app"`
	Category string `json:"category" bson:"category"`
}

type NotificationSettings struct {
	ID              interface{}           `json:"id" bson:"_id,omitempty"`
	TenantID        string                `json:"tenant_id" bson:"tenant_id"`
	Channels        []NotificationChannel `json:"channels" bson:"channels"`
	DeliveryFreq    string                `json:"deliveryFreq" bson:"delivery_freq"`
	QuietHoursStart string                `json:"quietHoursStart" bson:"quiet_hours_start"`
	QuietHoursEnd   string                `json:"quietHoursEnd" bson:"quiet_hours_end"`
}

type WebhookEndpoint struct {
	ID       interface{} `json:"id" bson:"_id,omitempty"`
	TenantID string      `json:"tenant_id" bson:"tenant_id"`
	URL      string      `json:"url" bson:"url"`
	Events   []string    `json:"events" bson:"events"`
	Active   bool        `json:"active" bson:"active"`
	// Secret é o signing secret (whsec_…). Write-only: nunca é serializado em
	// resposta HTTP (P0/A1 — a listagem vazava o segredo para qualquer
	// autenticado). Ele aparece UMA única vez, na resposta do POST de criação
	// (handlers.WebhookEndpointCreated), e continua sendo PERSISTIDO em
	// core.webhooks.metadata pela projeção explícita do repositório.
	Secret        string    `json:"-" bson:"secret"`
	LastTriggered time.Time `json:"lastTriggered" bson:"last_triggered"`
}

type APIKey struct {
	ID        interface{} `json:"id" bson:"_id,omitempty"`
	TenantID  string      `json:"tenant_id" bson:"tenant_id"`
	Name      string      `json:"name" bson:"name"`
	KeyPrefix string      `json:"keyPrefix" bson:"key_prefix"`
	KeyHash   string      `json:"-" bson:"key_hash"` // Never return hash to client
	Scopes    []string    `json:"scopes" bson:"scopes"`
	IsActive  bool        `json:"isActive" bson:"is_active"`
	LastUsed  time.Time   `json:"lastUsed" bson:"last_used"`
	ExpiresAt time.Time   `json:"expiresAt" bson:"expires_at"`
}
