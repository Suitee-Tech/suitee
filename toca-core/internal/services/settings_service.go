package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/oginoino/toca-core/internal/domain/core/settings"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/storage"
)

// PermissionsVersionInvalidator invalida o cache de permissões dos
// consumidores (achado A5). Opcional: nil = sem invalidação imediata.
type PermissionsVersionInvalidator interface {
	BumpPermissionsVersion(ctx context.Context, tenantID string) error
}

type SettingsService struct {
	permissionsInvalidator PermissionsVersionInvalidator
	companyRepo            settings.CompanySettingsRepository
	appearanceRepo         settings.AppearanceRepository
	permissionsRepo        settings.PermissionsRepository
	notificationsRepo      settings.NotificationSettingsRepository
	webhookRepo            settings.WebhookRepository
	apiKeyRepo             settings.APIKeyRepository
	storageService         storage.Service
}

func NewSettingsService(
	companyRepo settings.CompanySettingsRepository,
	appearanceRepo settings.AppearanceRepository,
	permissionsRepo settings.PermissionsRepository,
	notificationsRepo settings.NotificationSettingsRepository,
	webhookRepo settings.WebhookRepository,
	apiKeyRepo settings.APIKeyRepository,
	storageService storage.Service,
) *SettingsService {
	return &SettingsService{
		companyRepo:       companyRepo,
		appearanceRepo:    appearanceRepo,
		permissionsRepo:   permissionsRepo,
		notificationsRepo: notificationsRepo,
		webhookRepo:       webhookRepo,
		apiKeyRepo:        apiKeyRepo,
		storageService:    storageService,
	}
}

// Company Settings
func (s *SettingsService) GetCompanySettings(ctx context.Context, tenantID string) (*settings.CompanySettings, error) {
	result, err := s.companyRepo.GetByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return &settings.CompanySettings{
			TenantID:      tenantID,
			Currency:      "BRL",
			AutoSave:      true,
			CompactTables: false,
		}, nil
	}
	if result.Currency == "" {
		result.Currency = "BRL"
	}
	return result, nil
}

// DefaultTimezone é o fuso padrão do produto (Brasília, GMT-3) — o mesmo de
// pkg/timeutil. Duplicado de propósito: o core não deve depender do runtime dos
// serviços de fluxo para responder o fuso do tenant.
const DefaultTimezone = "America/Sao_Paulo"

// ErrInvalidTimezone recusa um fuso que o sistema operacional não conhece: sem
// essa validação o valor inválido seria aceito, o serviço cairia no padrão em
// silêncio e o operador acharia que configurou.
var ErrInvalidTimezone = errors.New("invalid timezone")

func normalizeTimezone(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if _, err := time.LoadLocation(trimmed); err != nil {
		return "", fmt.Errorf("%w: %q", ErrInvalidTimezone, trimmed)
	}
	return trimmed, nil
}

// TenantTimezone devolve o fuso configurado da imobiliária, com o padrão do
// produto quando não houver configuração. Consumido pela rota interna que os
// serviços de fluxo usam para montar dias civis.
func (s *SettingsService) TenantTimezone(ctx context.Context, tenantID string) (string, error) {
	company, err := s.GetCompanySettings(ctx, tenantID)
	if err != nil {
		return DefaultTimezone, err
	}
	if company != nil && strings.TrimSpace(company.Timezone) != "" {
		return strings.TrimSpace(company.Timezone), nil
	}
	return DefaultTimezone, nil
}

func (s *SettingsService) UpdateCompanySettings(ctx context.Context, tenantID string, req *settings.CompanySettings) error {
	req.TenantID = tenantID
	normalized, err := normalizeTimezone(req.Timezone)
	if err != nil {
		return err
	}
	req.Timezone = normalized
	if idStr, ok := req.ID.(string); ok && idStr == "" {
		req.ID = nil
	}
	return s.companyRepo.Upsert(ctx, req)
}

func (s *SettingsService) UploadCompanyLogo(ctx context.Context, tenantID string, filename string, contentType string, reader io.Reader) (string, error) {
	if s.storageService == nil {
		return "", errors.New("storage service not configured")
	}

	comp, err := s.companyRepo.GetByTenantID(ctx, tenantID)
	if err != nil {
		return "", err
	}
	if comp == nil {
		// Se não existir, criamos um default
		comp = &settings.CompanySettings{
			TenantID: tenantID,
		}
	}

	// Padrão de bucket: toca-logos
	path, err := s.storageService.Upload(ctx, "toca-logos", filename, contentType, reader)
	if err != nil {
		return "", err
	}

	comp.LogoURL = path

	if err := s.companyRepo.Upsert(ctx, comp); err != nil {
		return "", err
	}

	return path, nil
}

// UploadLoginBanner grava a imagem de banner da tela de login enviada pela
// imobiliária e a persiste em AppearanceSettings.LoginBannerURL, marcando
// `LoginBannerSource = custom`: enviar uma imagem é a intenção explícita de
// usá-la. O preset anterior fica guardado, então a troca de volta para um preset
// (e para a imagem de novo) é só mudar a fonte, sem reenviar o arquivo —
// resolução em toka/src/shared/loginBanners.ts. Reusa o bucket `toca-logos`: é o
// bucket de ativos de marca já provisionado (5MB, jpeg/png/webp/svg) e evita
// exigir um bucket novo na infraestrutura.
func (s *SettingsService) UploadLoginBanner(ctx context.Context, tenantID string, filename string, contentType string, reader io.Reader) (string, error) {
	if s.storageService == nil {
		return "", errors.New("storage service not configured")
	}

	appearance, err := s.appearanceRepo.GetByTenantID(ctx, tenantID)
	if err != nil {
		return "", err
	}
	if appearance == nil {
		appearance = &settings.AppearanceSettings{TenantID: tenantID}
	}

	path, err := s.storageService.Upload(ctx, "toca-logos", filename, contentType, reader)
	if err != nil {
		return "", err
	}

	appearance.TenantID = tenantID
	appearance.LoginBannerURL = path
	// Enviar a imagem a ativa: é o que o usuário acabou de pedir. O preset
	// escolhido antes continua gravado para a troca rápida.
	appearance.LoginBannerSource = LoginBannerSourceCustom

	if err := s.appearanceRepo.Upsert(ctx, appearance); err != nil {
		return "", err
	}

	return path, nil
}

// ErrInvalidAppearance marca valor recusado na aparência (o handler responde
// 400, não 500).
var ErrInvalidAppearance = errors.New("invalid appearance setting")

// appearanceAssetHosts devolve os hosts que a plataforma considera próprios para
// mídia pública. A fonte primária é o PRÓPRIO provedor de storage (a mesma
// função que grava a URL); as envs ficam como reserva quando não há storage
// injetado.
//
// N4 (reauditoria de 2026-09-20): `loginBannerUrl` e `customDomain` eram
// gravados sem nenhuma validação. A tela de login é PÚBLICA e carrega a imagem
// para todos os usuários do tenant — aceitar URL arbitrária significa permitir
// requisição a terceiros (pixel de rastreio), conteúdo misto e defacement se o
// host remoto mudar de conteúdo. Não é XSS (renderiza em `<img src>`), mas não é
// um campo que o tenant deva apontar para fora.
func (s *SettingsService) appearanceAssetHosts() map[string]struct{} {
	hosts := map[string]struct{}{}
	add := func(raw string) {
		if parsed, err := url.Parse(strings.TrimSpace(raw)); err == nil && parsed.Host != "" {
			hosts[strings.ToLower(parsed.Host)] = struct{}{}
		}
	}

	// FONTE PRIMÁRIA: a própria implementação de storage. `GetURL` é a MESMA
	// função que gravou a URL do banner, então a validação não pode divergir
	// dela. Foi exatamente essa divergência que passou a recusar (400) a imagem
	// recém-enviada: com `STORAGE_PROVIDER=supabase` a URL é montada com
	// `SUPABASE_PUBLIC_URL` (o host que o NAVEGADOR resolve), enquanto a
	// validação olhava só `SUPABASE_URL` — a URL interna do container
	// (`host.docker.internal`). As três implementações só formatam string, então
	// a chamada com nome fictício não toca a rede.
	if s.storageService != nil {
		add(s.storageService.GetURL("toca-logos", "probe"))
	}

	// Reserva para quando não há storage injetado: as envs que pkg/storage usa
	// para montar a URL pública, nas três variantes de provedor.
	for _, envKey := range []string{
		"PUBLIC_UPLOADS_URL", "PUBLIC_GATEWAY_URL", "PUBLIC_API_URL",
		"SUPABASE_PUBLIC_URL", "SUPABASE_URL", "S3_PUBLIC_ENDPOINT",
	} {
		add(os.Getenv(envKey))
	}
	return hosts
}

// Hostname (com ou sem subdomínios) e porta opcional. Aceita rótulo único para
// não recusar `localhost:3000` em desenvolvimento; o que importa é recusar
// esquema, caminho, espaço e afins.
var customDomainRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*(:\d{1,5})?$`)

// validateAppearanceFields recusa o que a tela de login pública não deve carregar.
var allowedAppearanceFonts = map[string]bool{
	"Inter": true, "DM Sans": true, "Poppins": true,
	"Roboto": true, "Nunito": true, "Open Sans": true,
}

var allowedAppearanceIconStyles = map[string]bool{
	"regular": true, "light": true, "bold": true, "sharp": true,
}

func (s *SettingsService) validateAppearanceFields(req *settings.AppearanceSettings) error {
	if domain := strings.TrimSpace(req.CustomDomain); domain != "" && !customDomainRe.MatchString(domain) {
		return fmt.Errorf("%w: customDomain deve ser um domínio (ex.: crm.suaimobiliaria.com.br), sem esquema nem caminho", ErrInvalidAppearance)
	}
	if font := strings.TrimSpace(req.FontFamily); font != "" && !allowedAppearanceFonts[font] {
		return fmt.Errorf("%w: fontFamily deve ser uma fonte suportada", ErrInvalidAppearance)
	}
	if style := strings.TrimSpace(req.IconStyle); style != "" && !allowedAppearanceIconStyles[style] {
		return fmt.Errorf("%w: iconStyle deve ser regular, light, bold ou sharp", ErrInvalidAppearance)
	}
	// A fonte precisa ser uma das duas: valor livre viraria "nenhuma imagem"
	// silenciosa na tela de login, porque a resolução não reconhece o valor.
	if source := strings.TrimSpace(req.LoginBannerSource); source != "" &&
		source != LoginBannerSourceCustom && source != LoginBannerSourcePreset {
		return fmt.Errorf("%w: loginBannerSource deve ser %q ou %q", ErrInvalidAppearance, LoginBannerSourceCustom, LoginBannerSourcePreset)
	}
	// A URL é validada mesmo quando a fonte ativa é o preset: ela fica gravada
	// para a troca rápida e volta a ser carregada numa página pública no
	// primeiro clique.
	return validateAppearanceAssetURL(req.LoginBannerURL, s.appearanceAssetHosts())
}

// validateAppearanceAssetURL aceita vazio, caminho relativo (mesma origem) ou
// URL absoluta cujo host seja da própria plataforma.
func validateAppearanceAssetURL(raw string, allowed map[string]struct{}) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	// Caminho relativo: o frontend resolve contra a origem da própria aplicação.
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("%w: loginBannerUrl inválida", ErrInvalidAppearance)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: loginBannerUrl deve ser http(s) ou um caminho iniciado por /", ErrInvalidAppearance)
	}
	if len(allowed) == 0 {
		// Sem host de plataforma conhecido não há como decidir: falha fechado e
		// exige caminho relativo.
		return fmt.Errorf("%w: loginBannerUrl absoluta exige host da plataforma configurado; use um caminho iniciado por /", ErrInvalidAppearance)
	}
	host := strings.ToLower(parsed.Host)
	if _, ok := allowed[host]; ok {
		return nil
	}
	// Compara também sem porta: a env pode trazer a porta e a URL gravada não.
	hostname := strings.ToLower(parsed.Hostname())
	for candidate := range allowed {
		if bare, _, err := net.SplitHostPort(candidate); err == nil && bare == hostname {
			return nil
		}
		if candidate == hostname {
			return nil
		}
	}
	return fmt.Errorf("%w: loginBannerUrl aponta para host fora da plataforma (%s)", ErrInvalidAppearance, parsed.Host)
}

// Appearance

// Defaults de white-label do painel de login. Fonte única no backend: espelham
// DEFAULT_THEME em toka/src/shared/context/ThemeContext.tsx e o registro de
// presets em toka/src/shared/loginBanners.ts.
const (
	DefaultLoginTagline      = "Sua plataforma imobiliária"
	DefaultLoginSubtitle     = "Gerencie leads, automatize sua esteira digital e feche mais negócios com inteligência de dados."
	DefaultLoginBannerPreset = "equipe"
)

// Fontes possíveis da imagem do painel de login. `LoginBannerPreset` e
// `LoginBannerURL` são mantidos AMBOS no registro: a escolha é qual deles está
// ativo, não qual existe. É isso que permite alternar entre a imagem enviada e
// um preset sem reenviar o arquivo.
const (
	LoginBannerSourceCustom = "custom"
	LoginBannerSourcePreset = "preset"
)

// EffectiveLoginBannerSource resolve a fonte quando o campo está vazio (registro
// gravado antes dele existir, ou payload antigo do localStorage): a URL
// preenchida continua ganhando do preset, que era a regra anterior a esta
// mudança. Valor desconhecido também cai nessa regra, em vez de derrubar a
// imagem do painel — a validação de escrita é quem recusa valor inválido.
func EffectiveLoginBannerSource(source, bannerURL string) string {
	switch source {
	case LoginBannerSourceCustom, LoginBannerSourcePreset:
		return source
	}
	if strings.TrimSpace(bannerURL) != "" {
		return LoginBannerSourceCustom
	}
	return LoginBannerSourcePreset
}

func (s *SettingsService) GetAppearance(ctx context.Context, tenantID string) (*settings.AppearanceSettings, error) {
	result, err := s.appearanceRepo.GetByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return &settings.AppearanceSettings{
			TenantID:        tenantID,
			PrimaryColor:    "#1C905B",
			AccentColor:     "#00BABC",
			SidebarStyle:    "default",
			Theme:           "auto",
			FontFamily:      "Inter",
			IconStyle:       "regular",
			ShowLogoLogin:   true,
			DarkModeEnabled: true,
			CustomDomain:    "",
			DomainVerified:  false,
			LoginTagline:    DefaultLoginTagline,
			LoginSubtitle:   DefaultLoginSubtitle,
			// Sem imagem própria: a tela de login usa o preset padrão.
			LoginBannerPreset: DefaultLoginBannerPreset,
			LoginBannerSource: LoginBannerSourcePreset,
			EmailFooter:       "Enviado por Suitee",
		}, nil
	}
	// Fallbacks para campos vazios se o documento existir mas faltar dados
	if result.PrimaryColor == "" {
		result.PrimaryColor = "#1C905B"
	}
	if result.AccentColor == "" {
		result.AccentColor = "#00BABC"
	}
	if result.SidebarStyle == "" {
		result.SidebarStyle = "default"
	}
	if result.Theme == "" {
		result.Theme = "auto"
	}
	if result.FontFamily == "" || !allowedAppearanceFonts[result.FontFamily] {
		result.FontFamily = "Inter"
	}
	if result.IconStyle == "" || !allowedAppearanceIconStyles[result.IconStyle] {
		result.IconStyle = "regular"
	}
	if result.LoginTagline == "" {
		result.LoginTagline = DefaultLoginTagline
	}
	if result.LoginSubtitle == "" {
		result.LoginSubtitle = DefaultLoginSubtitle
	}
	// O preset padrão só entra quando o tenant NUNCA escolheu um preset: com a
	// fonte em `custom` o preset é só a alternativa guardada para a troca
	// rápida, e sobrescrevê-lo apagaria a escolha dele.
	if result.LoginBannerPreset == "" && EffectiveLoginBannerSource(result.LoginBannerSource, result.LoginBannerURL) == LoginBannerSourcePreset {
		result.LoginBannerPreset = DefaultLoginBannerPreset
	}
	// Normaliza para o cliente nunca receber vazio: a aba de Aparência usa este
	// valor para marcar qual das imagens está selecionada.
	result.LoginBannerSource = EffectiveLoginBannerSource(result.LoginBannerSource, result.LoginBannerURL)

	return result, nil
}

func (s *SettingsService) UpdateAppearance(ctx context.Context, tenantID string, req *settings.AppearanceSettings) error {
	// N4 (reauditoria de 2026-09-20): o upsert gravava qualquer string. Ver
	// `validateAppearanceFields`.
	if err := s.validateAppearanceFields(req); err != nil {
		return err
	}
	req.TenantID = tenantID
	if idStr, ok := req.ID.(string); ok && idStr == "" {
		req.ID = nil
	}
	return s.appearanceRepo.Upsert(ctx, req)
}

// Permissions
func (s *SettingsService) GetPermissions(ctx context.Context, tenantID string) (*settings.RolePermissions, error) {
	result, err := s.permissionsRepo.GetByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return &settings.RolePermissions{
			TenantID:    tenantID,
			DefaultRole: "corretor",
			Permissions: settings.BuildDefaultMatrix(),
		}, nil
	}
	if result.DefaultRole == "" {
		result.DefaultRole = "corretor"
	}
	return result, nil
}

func (s *SettingsService) UpdatePermissions(ctx context.Context, tenantID string, req *settings.RolePermissions) error {
	req.TenantID = tenantID
	if idStr, ok := req.ID.(string); ok && idStr == "" {
		req.ID = nil
	}

	// P0.2/A6: validação ESTRITA contra o catálogo canônico. Antes os ids
	// desconhecidos eram descartados em silêncio (200 "permissions updated") e
	// as chaves de papel dentro de `Roles` não eram validadas — o operador
	// salvava uma matriz que o servidor não aplicava, e toggles do frontend
	// (catálogo divergente) não tinham efeito.
	catalogIDs := settings.CatalogIDs()

	// PERFIS DE ACESSO não são gravados por esta rota.
	//
	// A matriz é um documento inteiro (replace): se o PUT trouxesse a lista de
	// perfis, um cliente que salvasse só a matriz — ou uma versão antiga da tela,
	// que mandava `{id, name, baseRole, color}` sem permissões — apagaria os
	// perfis e o acesso de quem os usa. Aqui os perfis PERSISTIDOS são mantidos
	// como estão; criar/editar/apagar perfil é operação própria e auditada
	// (`/company-settings/access-profiles`).
	storedDoc, err := s.permissionsRepo.GetByTenantID(ctx, tenantID)
	if err != nil {
		return err
	}
	profiles := []settings.CustomRole(nil)
	if storedDoc != nil {
		profiles = storedDoc.CustomRoles
	}
	if req.CustomRoles != nil && len(req.CustomRoles) != len(profiles) {
		// Trilha do engano (não erro): o cliente tentou mexer em perfis por uma
		// rota que não é deles; a lista persistida é a fonte.
		logger.Warn("permissions update ignored custom roles payload",
			"event", "settings.permissions_custom_roles_ignored",
			"tenant_id", tenantID,
			"payload_profiles", len(req.CustomRoles),
			"stored_profiles", len(profiles))
	}
	req.CustomRoles = profiles

	// O papel padrão de novos usuários pode ser um perfil do tenant — mas só um
	// que exista: cair num id órfão deixaria todo usuário novo sem permissão
	// nenhuma logo no primeiro login.
	if !auth.IsValidFunctionalRole(req.DefaultRole) {
		if _, ok := settings.FindCustomRole(profiles, req.DefaultRole); !ok {
			return &settings.InvalidPermissionMatrixError{UnknownRoles: []string{req.DefaultRole}}
		}
	}

	validRoles := make(map[string]bool, len(auth.AllFunctionalRoles))
	for _, role := range auth.AllFunctionalRoles {
		validRoles[role] = true
	}

	normalized, err := settings.NormalizePermissionMatrix(req.Permissions, catalogIDs, validRoles, auth.AllFunctionalRoles)
	if err != nil {
		return err
	}
	req.Permissions = normalized

	if err := s.permissionsRepo.Upsert(ctx, req); err != nil {
		return err
	}

	s.invalidatePermissions(ctx, tenantID, "settings.permissions_updated")

	// Achado M5 (P2): a matriz de permissões É o mapa de autorização do tenant —
	// sem trilha não há como responder "quem concedeu finance:write a corretor, e
	// quando". A trilha guarda a matriz resultante (ids do catálogo, sem PII).
	audit.Record(ctx, audit.Entry{
		Action:     "settings.permissions_updated",
		EntityType: "role_permissions",
		EntityID:   tenantID,
		After: map[string]any{
			"default_role": req.DefaultRole,
			"permissions":  req.Permissions,
			"custom_roles": req.CustomRoles,
		},
	})

	return nil
}

// invalidatePermissions invalida o cache de permissões dos consumidores e deixa
// a falha registrada (achado A5). Best-effort de propósito: salvar a matriz não
// pode falhar porque o Redis está fora; o TTL de 30 s segue como rede.
func (s *SettingsService) invalidatePermissions(ctx context.Context, tenantID, event string) {
	if s.permissionsInvalidator == nil {
		return
	}
	if err := s.permissionsInvalidator.BumpPermissionsVersion(ctx, tenantID); err != nil {
		logger.Warn("permissions saved but cache invalidation failed",
			"event", event, "tenant_id", tenantID, "error", err.Error())
	}
}

// WithPermissionsInvalidator injeta o invalidador de cache (achado A5).
func (s *SettingsService) WithPermissionsInvalidator(inv PermissionsVersionInvalidator) *SettingsService {
	s.permissionsInvalidator = inv
	return s
}

func (s *SettingsService) GetDefaultRole(ctx context.Context, tenantID string) (string, error) {
	result, err := s.permissionsRepo.GetByTenantID(ctx, tenantID)
	if err != nil {
		return "corretor", err
	}
	if result == nil || result.DefaultRole == "" {
		return "corretor", nil
	}
	return result.DefaultRole, nil
}

// GetEffectivePermissions devolve as permissões EFETIVAS do papel (ou perfil de
// acesso) para o tenant.
//
// FONTE DE DECISÃO ÚNICA (achado A5): usa `settings.RolePermissionsFor`, a mesma
// regra que o middleware aplica em `RequirePermission`. Antes havia um
// curto-circuito `functional_role == "admin"` → catálogo inteiro, que fazia a
// tela mostrar permissões que o enforcement negava (quando o tenant salvava
// `admin: false` numa linha). Bypass de root continua existindo, mas no eixo de
// PERFIL (`IsRootProfile`), antes de qualquer consulta à matriz.
//
// Perfil de acesso personalizado (Configurações → Permissões) entra por aqui
// também: o `functional_role` do membro pode ser o id de um perfil do tenant, e
// quem concede é o conjunto do próprio perfil. Id de perfil que não existe mais
// devolve lista VAZIA (fail-closed) — nunca herda o `baseRole`.
func (s *SettingsService) GetEffectivePermissions(ctx context.Context, tenantID, functionalRole string) ([]string, error) {
	result, err := s.permissionsRepo.GetByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	var stored []settings.Permission
	var profiles []settings.CustomRole
	if result != nil {
		stored = result.Permissions
		profiles = result.CustomRoles
	}

	return settings.RolePermissionsFor(stored, profiles, functionalRole), nil
}

// Notifications
func (s *SettingsService) GetNotificationSettings(ctx context.Context, tenantID string) (*settings.NotificationSettings, error) {
	result, err := s.notificationsRepo.GetByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return &settings.NotificationSettings{TenantID: tenantID}, nil
	}
	return result, nil
}

func (s *SettingsService) UpdateNotificationSettings(ctx context.Context, tenantID string, req *settings.NotificationSettings) error {
	req.TenantID = tenantID
	if idStr, ok := req.ID.(string); ok && idStr == "" {
		req.ID = nil
	}
	return s.notificationsRepo.Upsert(ctx, req)
}

// Webhooks
func (s *SettingsService) CreateWebhook(ctx context.Context, tenantID string, req *settings.WebhookEndpoint) error {
	req.TenantID = tenantID
	// Generate secret
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	req.Secret = "whsec_" + hex.EncodeToString(b)
	if err := s.webhookRepo.Create(ctx, req); err != nil {
		return err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "settings.webhook_created",
		EntityType: "webhook_endpoint",
		EntityID:   fmt.Sprint(req.ID),
		After:      map[string]any{"url": req.URL, "events": req.Events},
	})
	return nil
}

func (s *SettingsService) ListWebhooks(ctx context.Context, tenantID string) ([]settings.WebhookEndpoint, error) {
	return s.webhookRepo.ListByTenantID(ctx, tenantID)
}

func (s *SettingsService) UpdateWebhook(ctx context.Context, tenantID string, req *settings.WebhookEndpoint) error {
	req.TenantID = tenantID
	if err := s.webhookRepo.Update(ctx, req); err != nil {
		return err
	}

	// Achado M5 (P2): endpoint de webhook carrega SEGREDO (assinatura HMAC do
	// outbound) — mudar URL/estado é mutação de configuração sensível. O segredo
	// em si nunca entra na trilha.
	audit.Record(ctx, audit.Entry{
		Action:     "settings.webhook_updated",
		EntityType: "webhook_endpoint",
		EntityID:   fmt.Sprint(req.ID),
		After: map[string]any{
			"url":    req.URL,
			"events": req.Events,
			"active": req.Active,
		},
	})
	return nil
}

// DeleteWebhook remove (soft delete) o webhook do tenant do chamador. O
// tenantID é obrigatório: sem ele o repositório filtraria só pelo id e o
// tenant A apagaria o webhook do tenant B (P0/C6).
func (s *SettingsService) DeleteWebhook(ctx context.Context, tenantID, id string) error {
	return s.webhookRepo.Delete(ctx, tenantID, id)
}

// API Keys
func (s *SettingsService) GenerateAPIKey(ctx context.Context, tenantID string, req *settings.APIKey) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	rawKey := "toca_" + hex.EncodeToString(b)

	req.TenantID = tenantID
	req.KeyPrefix = rawKey[:12]

	hash := sha256.Sum256([]byte(rawKey))
	req.KeyHash = hex.EncodeToString(hash[:])

	if err := s.apiKeyRepo.Create(ctx, req); err != nil {
		return "", err
	}
	return rawKey, nil
}

func (s *SettingsService) ListAPIKeys(ctx context.Context, tenantID string) ([]settings.APIKey, error) {
	return s.apiKeyRepo.ListByTenantID(ctx, tenantID)
}

// RevokeAPIKey desativa a API key do tenant do chamador. O tenantID é
// obrigatório: sem ele o repositório filtraria só pelo id e o tenant A
// revogaria a chave do tenant B (P0/C6).
func (s *SettingsService) RevokeAPIKey(ctx context.Context, tenantID, id string) error {
	if err := s.apiKeyRepo.Revoke(ctx, tenantID, id); err != nil {
		return err
	}
	// Achado M5 (P2): revogar credencial é evento de controle de acesso — nunca
	// com o valor da chave (só o id).
	audit.Record(ctx, audit.Entry{
		Action:     "settings.api_key_revoked",
		EntityType: "api_key",
		EntityID:   id,
	})
	return nil
}
