package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/oginoino/toca-auth/internal/app"
	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca-auth/internal/domain/auth"
	domainEvents "github.com/oginoino/toca-auth/internal/domain/events"
	"github.com/oginoino/toca-auth/internal/domain/invitation"
	"github.com/oginoino/toca-auth/internal/domain/preferences"
	"github.com/oginoino/toca-auth/internal/domain/user"
	supabaseauth "github.com/oginoino/toca-auth/internal/infrastructure/supabase"
	"github.com/oginoino/toca/pkg/audit"
	pkgAuth "github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/storage"
	pkgvalidation "github.com/oginoino/toca/pkg/validation"
)

type TenantSettingsRepository interface {
	GetByTenantID(ctx context.Context, tenantID string) (map[string]interface{}, error)
	GetDefaultRole(ctx context.Context, tenantID string) (string, error)
	// GetAccessProfileBaseRole resolve o papel do catálogo em que um PERFIL DE
	// ACESSO personalizado se baseia ("" quando não é um perfil do tenant).
	GetAccessProfileBaseRole(ctx context.Context, tenantID, profileID string) (string, error)
}

type AuthService struct {
	accountRepo        account.Repository
	credentialRepo     auth.Repository
	invitationRepo     invitation.Repository
	preferencesRepo    preferences.Repository
	tenantSettingsRepo TenantSettingsRepository
	storage            storage.Service
	eventBus           events.EventBus
	config             *app.Config
	redis              otpStore
	supabaseAuth       supabaseauth.Client
	// accountServiceRepo/credentialServiceRepo are the service-role (BYPASSRLS)
	// surfaces used by pre-authentication flows (login, OTP, e-mail availability)
	// that run before a JWT exists and are otherwise RLS-denied.
	accountServiceRepo    account.ServiceRepository
	credentialServiceRepo auth.ServiceRepository
	// otpLimiter aplica a cota POR CONTATO do /auth/otp/request (M14).
	otpLimiter *OTPContactLimiter
}

type otpStore interface {
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)
}

func NewAuthService(
	accRepo account.Repository,
	credRepo auth.Repository,
	inviteRepo invitation.Repository,
	prefsRepo preferences.Repository,
	tenantSettingsRepo TenantSettingsRepository,
	storageService storage.Service,
	eventBus events.EventBus,
	cfg *app.Config,
	redis otpStore,
	supabaseClient supabaseauth.Client,
	accountServiceRepo account.ServiceRepository,
	credentialServiceRepo auth.ServiceRepository,
) *AuthService {
	svc := &AuthService{
		accountRepo:           accRepo,
		credentialRepo:        credRepo,
		invitationRepo:        inviteRepo,
		preferencesRepo:       prefsRepo,
		tenantSettingsRepo:    tenantSettingsRepo,
		storage:               storageService,
		eventBus:              eventBus,
		config:                cfg,
		redis:                 redis,
		supabaseAuth:          supabaseClient,
		accountServiceRepo:    accountServiceRepo,
		credentialServiceRepo: credentialServiceRepo,
	}
	// M14: cota por contato do OTP. Usa o mesmo store do OTP (Redis quando
	// configurado) e cai para memória do processo quando não há.
	svc.otpLimiter = NewOTPContactLimiter(redis)
	return svc
}

// contactLimiter devolve o limitador por contato, criando um default em memória
// quando o serviço não passou pelo construtor (M14: a cota não pode depender de
// wiring — um AuthService montado à mão continua limitado).
func (s *AuthService) contactLimiter() *OTPContactLimiter {
	if s != nil && s.otpLimiter != nil {
		return s.otpLimiter
	}
	defaultOTPLimiterOnce.Do(func() { defaultOTPLimiter = NewOTPContactLimiter(nil) })
	return defaultOTPLimiter
}

var (
	defaultOTPLimiterOnce sync.Once
	defaultOTPLimiter     *OTPContactLimiter
)

func (s *AuthService) GetUserInfo(ctx context.Context, userID string) (*account.Account, error) {
	return s.accountRepo.GetByID(ctx, userID)
}

func (s *AuthService) UpdateUserInfo(ctx context.Context, userID string, updates map[string]interface{}) error {
	// First, fetch the account to ensure it exists
	acc, err := s.accountRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if acc == nil {
		return errors.New("account not found")
	}

	// Capture banking-before-update para audit. Comparamos campo a
	// campo na hora de gravar a entry para que o log só liste o que
	// realmente mudou (sem inflar com diff vazio).
	var bankingBefore user.BankingInfo
	if acc.Individual != nil {
		bankingBefore = acc.Individual.Banking
	}

	// Currently, the account repository only has an Update method that takes a full account object
	// We'll update the fields on the object and save it
	bankingChanged := false
	if acc.Individual != nil {
		if val, ok := updates["individual.firstname"].(string); ok {
			acc.Individual.FirstName = val
		}
		if val, ok := updates["individual.lastname"].(string); ok {
			acc.Individual.LastName = val
		}
		if val, ok := updates["individual.phone"].(string); ok {
			acc.Individual.Phone = val
		}
		if val, ok := updates["individual.address"].(*user.Address); ok {
			acc.Individual.Address = *val
		}
		if val, ok := updates["individual.notifications"].(*user.Notifications); ok {
			acc.Individual.Notifications = *val
		}
		if val, ok := updates["individual.banking"].(*user.BankingInfo); ok && val != nil {
			// Re-valida no service como defesa em profundidade. Handler
			// já validou, mas o service pode ser chamado por outros
			// callers (ex.: webhook KYC futuro).
			if err := val.Validate(); err != nil {
				return err
			}
			acc.Individual.Banking = *val
			bankingChanged = true
		}
		// FullName logic
		acc.Individual.FullName = strings.TrimSpace(acc.Individual.FirstName + " " + acc.Individual.LastName)
	}

	if err := s.accountRepo.Update(ctx, acc); err != nil {
		return err
	}

	// Audit trail bancário (#463). Mudança em dados de repasse é
	// alvo clássico de fraude — auditoria é gate regulatório (LGPD +
	// PCI-like) e operacional (rastreio quando o usuário reclama
	// "alguém alterou minha conta"). LGPD: NUNCA gravar pix_key,
	// account number ou holder document inteiros. Em vez disso,
	// resumimos: tipo+último-4 (suficiente para o usuário identificar
	// "essa é a conta que eu mudei") e fingerprint hash (suficiente
	// para investigação determinar se duas mudanças apontam pra
	// mesma conta) — fingerprint não implementado aqui, mantemos
	// apenas tipo+last4 que já satisfaz o caso comum.
	if bankingChanged {
		audit.Record(ctx, audit.Entry{
			Action:     "auth.banking_updated",
			EntityType: "User",
			EntityID:   userID,
			Metadata: map[string]any{
				"before_summary": bankingSummary(bankingBefore),
				"after_summary":  bankingSummary(acc.Individual.Banking),
			},
		})
	}

	return nil
}

// bankingSummary produz um resumo seguro para audit log: tipo de
// dado preenchido + últimos 4 dígitos quando aplicável. Nunca expõe
// o valor completo da chave PIX ou número da conta. Issue #463 + LGPD.
func bankingSummary(b user.BankingInfo) map[string]any {
	summary := map[string]any{
		"has_bank_data": !b.IsEmpty(),
	}
	if b.BankCode != "" {
		summary["bank_code"] = b.BankCode // não é PII — código compe público
	}
	if b.AccountType != "" {
		summary["account_type"] = string(b.AccountType)
	}
	if b.Account != "" {
		summary["account_last4"] = lastN(b.Account, 4)
	}
	if b.PixKeyType != "" {
		summary["pix_key_type"] = string(b.PixKeyType)
	}
	if b.PixKey != "" {
		summary["pix_key_last4"] = lastN(b.PixKey, 4)
	}
	return summary
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func (s *AuthService) Login(ctx context.Context, req *auth.LoginRequest) (*auth.TokenPair, error) {
	if s.supabaseAuth == nil {
		return nil, errors.New("supabase auth not configured")
	}

	session, err := s.supabaseAuth.SignInWithPassword(ctx, req.Email, req.Password)
	if err != nil {
		// Audit login_failed (#468) — pré-requisito da variação de
		// brute-force lockout do J10 BDD. Não grava a senha (óbvio); o
		// email é o identificador necessário para detectar pattern de
		// ataque. EntityID = email para que filtros por entity_id
		// agreguem todas as tentativas do mesmo alvo.
		audit.Record(ctx, audit.Entry{
			Action:     "auth.login_failed",
			EntityType: "auth",
			EntityID:   strings.ToLower(strings.TrimSpace(req.Email)),
			Metadata: map[string]any{
				"reason": "invalid_credentials",
			},
		})
		return nil, errors.New("invalid credentials")
	}

	acc, err := s.accountServiceRepo.GetByIDService(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		acc = &account.Account{
			ID:        session.UserID,
			Type:      account.AccountTypeIndividual,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Individual: &user.User{
				ID:       session.UserID,
				Email:    session.Email,
				Role:     "user",
				IsActive: true,
			},
		}
		if err := s.accountServiceRepo.CreateService(ctx, acc); err != nil {
			return nil, err
		}
	}

	appMeta := map[string]interface{}{
		// P2/M9: identidade em claim própria. `tenant_id` continua sendo o escopo
		// (aqui, a conta pessoal até um switch-context escolher a empresa).
		"account_id": acc.ID,
		"tenant_id":  acc.ID,
		"type":       string(acc.Type),
	}
	// P2/M7: `IsActive` era escrito `true` no cadastro e NUNCA lido — não havia
	// como bloquear um usuário sem apagar a conta (o fluxo LGPD marcava inativo e
	// o login seguia funcionando). A checagem vem antes de tocar app_metadata: a
	// conta desativada não recebe token novo nem sessão renovada.
	if acc.Individual != nil && !acc.Individual.IsActive {
		audit.Record(ctx, audit.Entry{
			Action:     "auth.login_denied_inactive",
			EntityType: "auth",
			EntityID:   session.UserID,
			Metadata:   map[string]any{"reason": "account_inactive"},
		})
		logger.Warn("auth.login_denied_inactive",
			"event", "auth.login_denied_inactive", "account_id", session.UserID)
		return nil, ErrAccountInactive
	}

	if err := s.supabaseAuth.UpdateAppMetadata(ctx, session.UserID, appMeta); err != nil {
		return nil, err
	}

	refreshed, err := s.supabaseAuth.RefreshToken(ctx, session.RefreshToken)
	if err != nil {
		return nil, err
	}

	// Audit login_success (#468). Marca tenant_id no contexto via Record's
	// pickup from JWT claims (a primeira chamada após login pode estar sem
	// claims completas, mas a tabela tem coluna tenant_id default null).
	audit.Record(ctx, audit.Entry{
		Action:     "auth.login_success",
		EntityType: "auth",
		EntityID:   session.UserID,
		Metadata: map[string]any{
			"email":  session.Email,
			"method": "password",
		},
	})

	return &auth.TokenPair{
		AccessToken:  refreshed.AccessToken,
		RefreshToken: refreshed.RefreshToken,
	}, nil
}

func (s *AuthService) OAuthLogin(ctx context.Context, req *auth.OAuthLoginRequest) (*auth.TokenPair, error) {
	if s.supabaseAuth == nil {
		return nil, errors.New("supabase auth not configured")
	}

	session, err := s.supabaseAuth.RefreshToken(ctx, req.RefreshToken)
	if err != nil {
		return nil, err
	}

	acc, err := s.accountServiceRepo.GetByIDService(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		acc = &account.Account{
			ID:        session.UserID,
			Type:      account.AccountTypeIndividual,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Individual: &user.User{
				ID:       session.UserID,
				Email:    session.Email,
				Role:     "user",
				IsActive: true,
			},
		}
		if err := s.accountServiceRepo.CreateService(ctx, acc); err != nil {
			return nil, err
		}
	}

	// P2/M7: mesma checagem do login por senha.
	if acc.Individual != nil && !acc.Individual.IsActive {
		logger.Warn("auth.login_denied_inactive",
			"event", "auth.login_denied_inactive", "account_id", session.UserID, "method", "oauth")
		return nil, ErrAccountInactive
	}

	appMeta := map[string]interface{}{
		"account_id": acc.ID, // P2/M9: identidade em claim própria
		"tenant_id":  acc.ID,
		"type":       string(acc.Type),
	}
	if err := s.supabaseAuth.UpdateAppMetadata(ctx, session.UserID, appMeta); err != nil {
		return nil, err
	}

	refreshed, err := s.supabaseAuth.RefreshToken(ctx, session.RefreshToken)
	if err != nil {
		return nil, err
	}

	return &auth.TokenPair{
		AccessToken:  refreshed.AccessToken,
		RefreshToken: refreshed.RefreshToken,
	}, nil
}

// RefreshToken generates a new token pair from a valid refresh token.
// isPortal specifies if the request is coming from the portal endpoint.
// P2/M7 — sentinelas de vínculo/conta inativos. O handler mapeia para 403 (e não
// 500/400) para que a UI distinga "bloqueado" de "erro interno".
var (
	ErrAccountInactive    = errors.New("account deactivated")
	ErrMembershipInactive = errors.New("membership suspended")
)

func (s *AuthService) RefreshToken(ctx context.Context, refreshTokenStr string, isPortal ...bool) (*auth.TokenPair, error) {
	if s.supabaseAuth == nil {
		return nil, errors.New("supabase auth not configured")
	}

	session, err := s.supabaseAuth.RefreshToken(ctx, refreshTokenStr)
	if err != nil {
		return nil, err
	}

	// P0/A15 (reauditoria de 2026-09-20): o refresh reemite o `app_metadata` que
	// está no Supabase — e a remoção de membro não o limpa. Sem esta revalidação
	// o ex-membro volta a entrar quando a marca do A15 expira. Ver
	// membership_refresh.go. O portal tem modelo de vínculo próprio (lead/deal,
	// não CompanyMember), então a checagem vale só para o contexto de operador.
	portal := len(isPortal) > 0 && isPortal[0]
	if !portal {
		if err := s.assertMembershipOnRefresh(ctx, session); err != nil {
			return nil, err
		}
	}

	return &auth.TokenPair{
		AccessToken:  session.AccessToken,
		RefreshToken: session.RefreshToken,
	}, nil
}

func (s *AuthService) SwitchContext(ctx context.Context, userID, companyID, refreshToken string) (*auth.TokenPair, error) {
	if s.supabaseAuth == nil {
		return nil, errors.New("supabase auth not configured")
	}

	// 1. Verify User
	userAcc, err := s.accountRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if userAcc == nil {
		return nil, errors.New("user not found")
	}

	// 2. Verify Company Membership
	companyAcc, err := s.accountRepo.GetByID(ctx, companyID)
	if err != nil {
		return nil, err
	}
	if companyAcc == nil || companyAcc.Type != account.AccountTypeCompany {
		return nil, errors.New("company not found")
	}

	isMember := false
	memberActive := false
	for _, member := range companyAcc.Company.Members {
		if member.AccountID == userID {
			isMember = true
			memberActive = member.Active()
			break
		}
	}

	if !isMember {
		return nil, errors.New("user is not a member of this company")
	}

	// P2/M7: vínculo suspenso não recebe token novo. Sem isto, o usuário
	// suspenso voltaria a entrar trocando de contexto (a marca de revogação
	// cobre o token vigente, não a emissão de um novo).
	if !memberActive {
		audit.Record(ctx, audit.Entry{
			Action:     "auth.switch_context_denied_inactive_member",
			EntityType: "company_member",
			EntityID:   userID,
			Metadata:   map[string]any{"company_id": companyID},
		})
		logger.Warn("auth.switch_context_denied_inactive_member",
			"event", "auth.switch_context_denied_inactive_member",
			"company_id", companyID, "account_id", userID)
		return nil, ErrMembershipInactive
	}

	// 3. Determine role/profile in the specific company
	permission := "member" // default
	functionalRole := ""
	memberIndex := -1
	for i, member := range companyAcc.Company.Members {
		if member.AccountID == userID {
			permission = member.Profile
			functionalRole = member.FunctionalRole
			memberIndex = i
			break
		}
	}

	// Immediate upgrade if user is owner
	if permission == "owner" && functionalRole != "admin" {
		functionalRole = "admin"
		if memberIndex != -1 {
			companyAcc.Company.Members[memberIndex].FunctionalRole = "admin"
			_ = s.accountRepo.Update(ctx, companyAcc)
		}
	}

	// Lazy migration: if FunctionalRole is empty, fetch default and save it
	if functionalRole == "" {
		if s.tenantSettingsRepo != nil {
			defRole, err := s.tenantSettingsRepo.GetDefaultRole(ctx, companyID)
			if err != nil || defRole == "" {
				functionalRole = "corretor" // fallback
			} else {
				functionalRole = defRole
			}
		} else {
			functionalRole = "corretor" // fallback
		}

		if memberIndex != -1 {
			companyAcc.Company.Members[memberIndex].FunctionalRole = functionalRole
			// Save the updated company member
			_ = s.accountRepo.Update(ctx, companyAcc)
		}
	}

	// Papel BASE do perfil de acesso: quando o membro tem um perfil
	// personalizado (`cr_…`), o token leva também o papel do catálogo de onde ele
	// partiu. É o que faz um perfil "Gestor Comercial" (base gestor) enxergar o
	// tenant inteiro como o gestor enxerga — sem isso, o id do perfil não casa
	// com nenhum papel e o usuário fica recortado pela própria carteira
	// (incidente de 2026-09-21).
	baseRole := ""
	if pkgAuth.IsAccessProfileID(functionalRole) && s.tenantSettingsRepo != nil {
		if resolved, err := s.tenantSettingsRepo.GetAccessProfileBaseRole(ctx, companyID, functionalRole); err == nil {
			baseRole = resolved
		}
	}

	name := ""
	avatarURL := ""
	if userAcc.Individual != nil {
		name = userAcc.Individual.FullName
		avatarURL = userAcc.Individual.AvatarURL
	}

	appMeta := map[string]interface{}{
		// P2/M9: identidade (conta local) e tenant são coisas diferentes — é
		// exatamente aqui que elas se separam para o `cliente` convidado por uma
		// empresa (antes a identidade dele virava o id da empresa).
		"account_id":         userID,
		"tenant_id":          companyID,
		"permission":         permission,
		"functional_role":    functionalRole,
		"base_role":          baseRole,
		"type":               string(userAcc.Type),
		"partner_tenant_ids": []string{},
		"name":               name,
		"avatar_url":         avatarURL,
	}
	if err := s.supabaseAuth.UpdateAppMetadata(ctx, userID, appMeta); err != nil {
		return nil, err
	}

	refreshed, err := s.supabaseAuth.RefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, err
	}

	// Audit company.switched (#468 + gap J09 #466). Multi-tenant trocando
	// de workspace — útil para reconstituir contexto de auditoria
	// ("usuário operou no tenant X às 14h, mudou para Y às 14:30").
	audit.Record(ctx, audit.Entry{
		Action:     "company.switched",
		EntityType: "auth",
		EntityID:   userID,
		After: map[string]any{
			"tenant_id":       companyID,
			"functional_role": functionalRole,
			"permission":      permission,
		},
	})

	return &auth.TokenPair{
		AccessToken:  refreshed.AccessToken,
		RefreshToken: refreshed.RefreshToken,
	}, nil
}

// UpdateProfile updates the individual user profile. FirstName/LastName are
// sanitized and validated up-front because they feed Contract.Signatories.Name
// later, which must match the Clicksign v3 signer-name rules (no digits, ≥2
// chars). Catching it here means the customer sees a clear toast at edit
// time instead of an opaque failure weeks later when the contract is sent
// for signature.
func (s *AuthService) UpdateProfile(ctx context.Context, accountID string, req *user.User) error {
	first := pkgvalidation.SanitizePersonName(req.FirstName)
	last := pkgvalidation.SanitizePersonName(req.LastName)
	if err := pkgvalidation.ValidatePersonName(first, "Nome"); err != nil {
		return err
	}
	if err := pkgvalidation.ValidatePersonName(last, "Sobrenome"); err != nil {
		return err
	}

	acc, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return err
	}
	if acc == nil || acc.Individual == nil {
		return errors.New("account not found or not an individual")
	}

	// Snapshot pré-mutação para o audit diff. Capturamos só campos não-PII
	// — phone, bio, avatar — sem documento ou banking que (quando vierem
	// no PR #463) precisam ser tratados com flag PII separada.
	beforeSnap := audit.Snapshot(acc.Individual,
		"first_name", "last_name", "phone", "bio", "avatar_url",
	)

	acc.Individual.FirstName = first
	acc.Individual.LastName = last
	acc.Individual.FullName = first + " " + last
	acc.Individual.Phone = req.Phone
	acc.Individual.Bio = req.Bio
	if req.AvatarURL != "" {
		acc.Individual.AvatarURL = req.AvatarURL
	}
	acc.UpdatedAt = time.Now()

	if err := s.accountRepo.Update(ctx, acc); err != nil {
		return err
	}

	// Audit user.profile_updated (#468). Cobre rastreio de mudanças de
	// dados básicos do perfil. Mudanças de banking (issue #463 — virão
	// no Sprint 2) terão ação separada user.banking_updated com Metadata
	// PII flag para mascaramento no frontend (#473).
	audit.Record(ctx, audit.Entry{
		Action:     "user.profile_updated",
		EntityType: "auth",
		EntityID:   accountID,
		Before:     beforeSnap,
		After: audit.Snapshot(acc.Individual,
			"first_name", "last_name", "phone", "bio", "avatar_url",
		),
	})
	return nil
}

func normalizePhone(phone string) string {
	p := strings.TrimSpace(phone)
	if p == "" {
		return ""
	}
	hasPlus := strings.HasPrefix(p, "+")

	digits := make([]rune, 0, len(p))
	for _, r := range p {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) == 0 {
		return ""
	}
	if hasPlus {
		return "+" + string(digits)
	}
	return string(digits)
}

func (s *AuthService) lookupClientByContact(ctx context.Context, contact string) (*account.Account, error) {
	if strings.Contains(contact, "@") {
		cred, err := s.credentialServiceRepo.GetByEmailService(ctx, contact)
		if err != nil || cred == nil {
			return nil, err
		}
		return s.accountServiceRepo.GetByIDService(ctx, cred.AccountID)
	}
	return s.accountServiceRepo.GetByIndividualPhoneService(ctx, contact)
}

// recordUnknownContactSkip registra (log + audit) um pedido de OTP descartado
// porque o contato não tem conta de portal. A resposta HTTP continua genérica
// de sucesso — o cliente não descobre se o e-mail existe —, mas o operador
// passa a enxergar o descarte: era o único caminho do fluxo que devolvia 200 e
// não deixava rastro nenhum.
func (s *AuthService) recordUnknownContactSkip(ctx context.Context, contact, method string) {
	contactHash := contactRateLimitKey(contact)
	audit.Record(ctx, audit.Entry{
		Action:     "auth.otp_skipped_unknown_contact",
		EntityType: "auth",
		EntityID:   contactHash,
		Metadata: map[string]any{
			"method":       method,
			"reason":       "unknown_contact",
			"contact_hash": contactHash,
		},
	})
	logger.Info("auth.otp_skipped_unknown_contact",
		"event", "auth.otp_skipped_unknown_contact", "method", method,
		"contact_hash", contactHash)
}

// RequestOTP generates a 6-digit OTP, stores it in Redis, and emits an event.
func (s *AuthService) RequestOTP(ctx context.Context, contact, method string) error {
	if s.supabaseAuth == nil {
		return errors.New("supabase auth not configured")
	}

	contact = strings.TrimSpace(contact)

	// M14: cota POR CONTATO antes de qualquer envio. O limite de borda é por IP;
	// este é o que protege o titular (e o custo de envio) quando o atacante
	// rotaciona IP mantendo o alvo. `contactLimiter()` nunca devolve nil: um
	// serviço montado fora do construtor continua com a cota valendo (a proteção
	// não depende de wiring).
	if limiter := s.contactLimiter(); !limiter.Allow(ctx, contact) {
		limit, window := limiter.Limit()
		audit.Record(ctx, audit.Entry{
			Action:     "auth.otp_rate_limited",
			EntityType: "auth",
			EntityID:   contactRateLimitKey(contact),
			Metadata: map[string]any{
				"method":       method,
				"limit":        limit,
				"window":       window.String(),
				"contact_hash": contactRateLimitKey(contact),
				"reason":       "contact_rate_limit",
			},
		})
		logger.Warn("auth.otp_rate_limited",
			"event", "auth.otp_rate_limited", "method", method,
			"limit", limit, "window", window.String())
		return ErrOTPRateLimited
	}

	if method == "email" {
		acc, err := s.lookupClientByContact(ctx, contact)
		if err != nil {
			return errors.New("failed to process request")
		}
		if acc == nil {
			// Contato desconhecido: a resposta continua genérica de sucesso
			// (não confirma existência de conta), mas o descarte fica
			// registrado. Sem isso o 200 escondia o cliente sem conta de
			// portal — o usuário pedia o código, nada chegava e o suporte não
			// tinha por onde começar (issue "código de acesso nunca recebido").
			s.recordUnknownContactSkip(ctx, contact, method)
			return nil
		}
		if s.eventBus == nil {
			return errors.New("event bus unavailable, cannot deliver OTP")
		}
		code, err := s.supabaseAuth.GenerateEmailOTP(ctx, contact)
		if err != nil {
			return err
		}
		return s.eventBus.Publish(ctx, domainEvents.OTPRequestedEvent{
			Contact: contact,
			Method:  method,
			Code:    code,
		})
	}
	if method == "whatsapp" {
		phone := normalizePhone(contact)

		// M14 — gate de existência: só envia para contato que JÁ é cliente (conta
		// criada pelo convite/ensure-client). Antes, o ramo WhatsApp mandava o OTP
		// com `create_user=true` para qualquer número — o Supabase criava usuário
		// para telefone arbitrário, pagando envio e poluindo a base.
		//
		// Contato desconhecido recebe a MESMA resposta genérica de sucesso (não
		// confirma existência e não gera custo).
		acc, err := s.lookupClientByContact(ctx, phone)
		if err != nil {
			return errors.New("failed to process request")
		}
		if acc == nil {
			s.recordUnknownContactSkip(ctx, contact, method)
			return nil
		}
		// `create_user=false`: a conta já existe, então o Supabase não deve criar
		// outra identidade para o mesmo telefone.
		return s.supabaseAuth.SendOTP(ctx, "", phone, "whatsapp", false)
	}
	return errors.New("unsupported otp method")
}

// VerifyOTP validates the code and returns a token.
func (s *AuthService) VerifyOTP(ctx context.Context, contact, code string) (*auth.TokenPair, error) {
	if s.supabaseAuth == nil {
		return nil, errors.New("supabase auth not configured")
	}

	var sess *supabaseauth.Session
	var err error

	if strings.Contains(contact, "@") {
		// `type=email` casa com o `email_otp` retornado por
		// admin/generate_link (issue #559b1114 trocou para "magiclink",
		// mas esse tipo no /verify procura o token longo da URL, não o
		// código de 6 dígitos — quebrava a primeira tentativa de OTP).
		sess, err = s.supabaseAuth.VerifyOTP(ctx, "email", contact, "", code)
	} else {
		phone := normalizePhone(contact)
		sess, err = s.supabaseAuth.VerifyOTP(ctx, "whatsapp", "", phone, code)
		if err != nil {
			sess, err = s.supabaseAuth.VerifyOTP(ctx, "sms", "", phone, code)
		}
	}
	if err != nil {
		// Audit auth.otp_failed (#468). Útil para detectar brute force
		// no portal cliente. entity_id = contact (email/phone normalizado).
		audit.Record(ctx, audit.Entry{
			Action:     "auth.otp_failed",
			EntityType: "auth",
			EntityID:   strings.ToLower(strings.TrimSpace(contact)),
			Metadata: map[string]any{
				"reason": "invalid_or_expired",
			},
		})
		return nil, errors.New("invalid or expired code")
	}

	acc, err := s.accountServiceRepo.GetByIDService(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		// Reusa a conta criada pelo convite (ensure-client) para o mesmo email —
		// evita DUAS contas por cliente (a do invite e a do OTP), o que quebrava
		// a associação do canal de chat no portal.
		if cred, cerr := s.credentialServiceRepo.GetByEmailService(ctx, sess.Email); cerr == nil && cred != nil {
			if existing, eerr := s.accountServiceRepo.GetByIDService(ctx, cred.AccountID); eerr == nil && existing != nil {
				acc = existing
			}
		}
	}
	if acc == nil {
		acc = &account.Account{
			ID:        sess.UserID,
			Type:      account.AccountTypeIndividual,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Individual: &user.User{
				ID:       sess.UserID,
				Email:    sess.Email,
				Role:     "user",
				IsActive: true,
			},
		}
		if err := s.accountServiceRepo.CreateService(ctx, acc); err != nil {
			return nil, err
		}
	}

	appMeta := map[string]interface{}{
		// P2/M9: aqui `acc.ID` é a CONTA LOCAL do cliente (não uma empresa). Ela
		// vai na claim de identidade; o tenant do cliente sem empresa é a própria
		// conta (comportamento preservado), mas quem lê identidade não depende
		// mais disso.
		"account_id":      acc.ID,
		"tenant_id":       acc.ID,
		"permission":      "",
		"functional_role": "cliente",
		"type":            string(acc.Type),
	}
	if err := s.supabaseAuth.UpdateAppMetadata(ctx, sess.UserID, appMeta); err != nil {
		return nil, err
	}

	refreshed, err := s.supabaseAuth.RefreshToken(ctx, sess.RefreshToken)
	if err != nil {
		return nil, err
	}

	// Audit OTP login success (#468). Diferencia portal cliente (functional
	// role cliente) das demais via metadata.
	audit.Record(ctx, audit.Entry{
		Action:     "auth.login_success",
		EntityType: "auth",
		EntityID:   sess.UserID,
		Metadata: map[string]any{
			"email":  sess.Email,
			"method": "otp",
			"portal": true,
		},
	})

	return &auth.TokenPair{
		AccessToken:  refreshed.AccessToken,
		RefreshToken: refreshed.RefreshToken,
	}, nil
}

// EnsureClientAccount checks if an account exists for the email/phone. If not, creates it.
func (s *AuthService) EnsureClientAccount(ctx context.Context, email, name, phone string) (string, error) {
	// Check if exists by email
	cred, err := s.credentialServiceRepo.GetByEmailService(ctx, email)
	if err == nil && cred != nil {
		return cred.AccountID, nil
	}

	// If not exists, create Account
	nameParts := strings.Fields(name)
	firstName, lastName := "", ""
	if len(nameParts) > 0 {
		firstName = nameParts[0]
		lastName = strings.Join(nameParts[1:], " ")
	}
	acc := &account.Account{
		Type: account.AccountTypeIndividual,
		Individual: &user.User{
			FirstName: firstName,
			LastName:  lastName,
			FullName:  strings.Join(nameParts, " "),
			Email:     email,
			Phone:     normalizePhone(phone),
			Role:      "user",
			IsActive:  true,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.accountServiceRepo.CreateService(ctx, acc); err != nil {
		return "", fmt.Errorf("failed to create account: %w", err)
	}

	// Create dummy credential just to reserve the email (passwordless)
	newCred := &auth.Credential{
		AccountID:    acc.ID,
		Email:        email,
		PasswordHash: "passwordless", // invalid hash, cannot be used for password login
		Role:         "user",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.credentialServiceRepo.CreateService(ctx, newCred); err != nil {
		return "", fmt.Errorf("failed to create credential: %w", err)
	}

	return acc.ID, nil
}

func (s *AuthService) Logout(ctx context.Context, accessToken string) error {
	if s.supabaseAuth == nil {
		return nil
	}
	return s.supabaseAuth.Logout(ctx, accessToken)
}

func (s *AuthService) LogoutAll(ctx context.Context, accessToken string) error {
	if s.supabaseAuth == nil {
		return nil
	}
	return s.supabaseAuth.LogoutAll(ctx, accessToken)
}

func (s *AuthService) UploadAvatar(ctx context.Context, accountID string, filename string, contentType string, reader io.Reader) (string, error) {
	if s.storage == nil {
		return "", errors.New("storage service not configured")
	}

	acc, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return "", err
	}
	if acc == nil {
		return "", errors.New("account not found")
	}

	path, err := s.storage.Upload(ctx, "toca-avatars", filename, contentType, reader)
	if err != nil {
		return "", err
	}

	if acc.Individual != nil {
		acc.Individual.AvatarURL = path
	} else {
		return "", errors.New("account is not an individual")
	}

	if err := s.accountRepo.Update(ctx, acc); err != nil {
		return "", err
	}

	return path, nil
}
