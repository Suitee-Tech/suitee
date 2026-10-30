package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca-auth/internal/app"
	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca-auth/internal/domain/auth"
	"github.com/oginoino/toca-auth/internal/domain/company"
	domainevents "github.com/oginoino/toca-auth/internal/domain/events"
	"github.com/oginoino/toca-auth/internal/domain/invitation"
	"github.com/oginoino/toca-auth/internal/domain/user"
	supabaseauth "github.com/oginoino/toca-auth/internal/infrastructure/supabase"
	"github.com/oginoino/toca/pkg/audit"
	pkgAuth "github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/pii"
)

type InvitationService struct {
	accountRepo    account.Repository
	credentialRepo auth.Repository
	invitationRepo invitation.Repository
	authService    *AuthService
	regService     *RegistrationService
	eventBus       events.EventBus
	config         *app.Config
	groupService   *GroupService
	// Service-role (BYPASSRLS) surfaces for the public invite flow, which runs
	// before/outside a JWT for the target company (no tenant_id claim).
	accountServiceRepo    account.ServiceRepository
	credentialServiceRepo auth.ServiceRepository
	invitationServiceRepo invitation.ServiceRepository
}

func NewInvitationService(
	accRepo account.Repository,
	credRepo auth.Repository,
	inviteRepo invitation.Repository,
	authSvc *AuthService,
	regSvc *RegistrationService,
	eventBus events.EventBus,
	cfg *app.Config,
	groupService *GroupService,
	accountServiceRepo account.ServiceRepository,
	credentialServiceRepo auth.ServiceRepository,
	invitationServiceRepo invitation.ServiceRepository,
) *InvitationService {
	return &InvitationService{
		accountRepo:           accRepo,
		credentialRepo:        credRepo,
		invitationRepo:        inviteRepo,
		authService:           authSvc,
		regService:            regSvc,
		eventBus:              eventBus,
		config:                cfg,
		groupService:          groupService,
		accountServiceRepo:    accountServiceRepo,
		credentialServiceRepo: credentialServiceRepo,
		invitationServiceRepo: invitationServiceRepo,
	}
}

const maxInvitationsPerDay = 20

func (s *InvitationService) CreateInvitation(ctx context.Context, requestorID, companyID string, req *invitation.CreateInvitationRequest) (*invitation.Invitation, error) {
	// 0. Verify Company and Permissions
	compAcc, err := s.accountRepo.GetByID(ctx, companyID)
	if err != nil {
		return nil, err
	}
	if compAcc == nil || compAcc.Type != account.AccountTypeCompany {
		return nil, errors.New("company not found")
	}

	requestorProfile := ""
	if compAcc.Company.OwnerID == requestorID {
		requestorProfile = "owner"
	} else {
		for _, m := range compAcc.Company.Members {
			if m.AccountID == requestorID {
				requestorProfile = m.Profile
				break
			}
		}
	}
	if requestorProfile == "" || (requestorProfile != "owner" && requestorProfile != "admin") {
		return nil, errors.New("unauthorized")
	}

	// 1. Rate limit: max invitations per company per day
	invites, err := s.invitationRepo.ListByCompany(ctx, companyID)
	if err != nil {
		return nil, err
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	count := 0
	for _, inv := range invites {
		if inv != nil && inv.CreatedAt.After(cutoff) {
			count++
		}
	}
	if count >= maxInvitationsPerDay {
		return nil, errors.New("invitation limit exceeded: maximum 20 invitations per day per company")
	}

	// 2. Check if user already exists in system
	existingUser, err := s.credentialRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, err
	}
	if existingUser != nil {
		for _, m := range compAcc.Company.Members {
			if m.AccountID == existingUser.AccountID {
				return nil, errors.New("user is already a member")
			}
		}
	}

	// 3. Check for existing pending invitation
	existingInvite, err := s.invitationRepo.GetByEmailAndCompany(ctx, req.Email, companyID)
	if err != nil {
		return nil, err
	}
	if existingInvite != nil && existingInvite.Status == "pending" {
		// Cancel the old pending invitation instead of returning an error
		_ = s.invitationRepo.UpdateStatus(ctx, existingInvite.CompanyID, existingInvite.ID, "canceled")
	}

	// 4. Create Invitation
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(tokenBytes)

	role := req.Role
	if role == "" || !pkgAuth.IsAssignableRole(role) {
		if s.authService.tenantSettingsRepo != nil {
			defRole, _ := s.authService.tenantSettingsRepo.GetDefaultRole(ctx, companyID)
			if defRole != "" {
				role = defRole
			} else {
				role = "corretor"
			}
		} else {
			role = "corretor"
		}
	}

	invite := &invitation.Invitation{
		ID:        uuid.New().String(),
		Email:     req.Email,
		CompanyID: companyID,
		Role:      role,
		GroupIDs:  req.GroupIDs,
		Status:    "pending",
		Token:     token,
		CreatedBy: requestorID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(48 * time.Hour),
	}

	if err := s.invitationRepo.Create(ctx, invite); err != nil {
		return nil, err
	}

	// Achado M5 (P2): convite é criação de acesso — auditado SEM o token (o
	// segredo de aceite não vai para a trilha) e com o e-mail mascarado (PII,
	// retenção de 5 anos).
	audit.Record(ctx, audit.Entry{
		Action:     "company.invitation_created",
		EntityType: "company_invitation",
		EntityID:   invite.ID,
		After: map[string]any{
			"email":      pii.MaskString(pii.KindEmail, invite.Email),
			"role":       invite.Role,
			"group_ids":  invite.GroupIDs,
			"expires_at": invite.ExpiresAt,
		},
		Metadata: map[string]any{"company_id": companyID, "actor_id": requestorID},
	})

	// Publish event
	if s.eventBus != nil {
		base := strings.TrimRight(s.config.FrontendURL, "/")
		if base == "" {
			base = s.config.FrontendURL
		}
		eventPayload := domainevents.InvitationCreatedEvent{
			Email:       invite.Email,
			CompanyID:   invite.CompanyID,
			CompanyName: compAcc.Company.Name,
			Role:        invite.Role,
			Token:       invite.Token,
			InviteURL:   fmt.Sprintf("%s/invite?token=%s", base, invite.Token),
		}
		if err := s.eventBus.Publish(ctx, eventPayload); err != nil {
			logger.Error("Failed to publish invitation created event", "error", err)
		}
	}

	return invite, nil
}

// ListInvitations devolve os convites da empresa.
//
// SECURITY (achado C2 da revisão de permissões de 2026-09-19): antes bastava
// ser MEMBRO para listar, e o objeto devolvido incluía o `Token` do convite —
// um bearer de uso único que cria conta para o e-mail convidado e, quando o
// papel do convite é `admin`, um principal root no tenant. Qualquer membro
// (corretor, auditor, membro removido com token ainda válido) podia ler os
// tokens pendentes e aceitá-los por conta própria. Agora só proprietário/admin
// listam, e o token nunca sai daqui (ver handler, que projeta sem Token).
func (s *InvitationService) ListInvitations(ctx context.Context, requestorID, companyID string) ([]*invitation.Invitation, error) {
	compAcc, err := s.accountRepo.GetByID(ctx, companyID)
	if err != nil {
		return nil, err
	}
	if compAcc == nil || compAcc.Type != account.AccountTypeCompany {
		return nil, errors.New("company not found")
	}

	requestorProfile := ""
	if compAcc.Company.OwnerID == requestorID {
		requestorProfile = "owner"
	} else {
		for _, m := range compAcc.Company.Members {
			if m.AccountID == requestorID {
				requestorProfile = m.Profile
				break
			}
		}
	}
	if requestorProfile != "owner" && requestorProfile != "admin" {
		return nil, errors.New("unauthorized")
	}

	return s.invitationRepo.ListByCompany(ctx, companyID)
}

func (s *InvitationService) ResendInvitation(ctx context.Context, requestorID, companyID, invitationID string) error {
	compAcc, err := s.accountRepo.GetByID(ctx, companyID)
	if err != nil {
		return err
	}
	if compAcc == nil || compAcc.Type != account.AccountTypeCompany {
		return errors.New("company not found")
	}

	requestorProfile := ""
	if compAcc.Company.OwnerID == requestorID {
		requestorProfile = "owner"
	} else {
		for _, m := range compAcc.Company.Members {
			if m.AccountID == requestorID {
				requestorProfile = m.Profile
				break
			}
		}
	}
	if requestorProfile == "" || (requestorProfile != "owner" && requestorProfile != "admin") {
		return errors.New("unauthorized")
	}

	invites, err := s.invitationRepo.ListByCompany(ctx, companyID)
	if err != nil {
		return err
	}

	var invite *invitation.Invitation
	for _, inv := range invites {
		if inv.ID == invitationID {
			invite = inv
			break
		}
	}

	if invite == nil {
		return errors.New("invitation not found")
	}
	if invite.Status == "accepted" {
		return errors.New("invitation already accepted")
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return err
	}
	newToken := hex.EncodeToString(tokenBytes)

	newInvite := &invitation.Invitation{
		ID:        uuid.New().String(),
		Email:     invite.Email,
		CompanyID: companyID,
		Role:      invite.Role,
		Status:    "pending",
		Token:     newToken,
		CreatedBy: requestorID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(48 * time.Hour),
	}

	if err := s.invitationRepo.Create(ctx, newInvite); err != nil {
		return err
	}
	_ = s.invitationRepo.UpdateStatus(ctx, invite.CompanyID, invite.ID, "canceled")

	audit.Record(ctx, audit.Entry{
		Action:     "company.invitation_resent",
		EntityType: "company_invitation",
		EntityID:   newInvite.ID,
		Before:     map[string]any{"invitation_id": invite.ID, "status": "canceled"},
		After:      map[string]any{"invitation_id": newInvite.ID, "status": "pending", "email": pii.MaskString(pii.KindEmail, newInvite.Email)},
		Metadata:   map[string]any{"company_id": companyID, "actor_id": requestorID},
	})

	if s.eventBus != nil {
		base := strings.TrimRight(s.config.FrontendURL, "/")
		if base == "" {
			base = s.config.FrontendURL
		}
		eventPayload := domainevents.InvitationCreatedEvent{
			Email:       newInvite.Email,
			CompanyID:   newInvite.CompanyID,
			CompanyName: compAcc.Company.Name,
			Role:        newInvite.Role,
			Token:       newInvite.Token,
			InviteURL:   fmt.Sprintf("%s/invite?token=%s", base, newInvite.Token),
		}
		if err := s.eventBus.Publish(ctx, eventPayload); err != nil {
			logger.Error("Failed to publish invitation created event", "error", err)
		}
	}

	return nil
}

func (s *InvitationService) CancelInvitation(ctx context.Context, requestorID, companyID, invitationID string) error {
	compAcc, err := s.accountRepo.GetByID(ctx, companyID)
	if err != nil {
		return err
	}
	if compAcc == nil || compAcc.Type != account.AccountTypeCompany {
		return errors.New("company not found")
	}

	// SECURITY (achado C2/M6): cancelar convite é ação administrativa —
	// `ResendInvitation` já exigia owner/admin e o cancel aceitava qualquer
	// membro. Alinhado com M6 da revisão.
	requestorProfile := ""
	if compAcc.Company.OwnerID == requestorID {
		requestorProfile = "owner"
	} else {
		for _, m := range compAcc.Company.Members {
			if m.AccountID == requestorID {
				requestorProfile = m.Profile
				break
			}
		}
	}
	if requestorProfile != "owner" && requestorProfile != "admin" {
		return errors.New("unauthorized")
	}

	invites, err := s.invitationRepo.ListByCompany(ctx, companyID)
	if err != nil {
		return err
	}

	var invite *invitation.Invitation
	for _, inv := range invites {
		if inv.ID == invitationID {
			invite = inv
			break
		}
	}

	if invite == nil {
		return errors.New("invitation not found")
	}
	if invite.Status == "accepted" {
		return errors.New("cannot cancel an accepted invitation")
	}

	if err := s.invitationRepo.UpdateStatus(ctx, companyID, invitationID, "canceled"); err != nil {
		return err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "company.invitation_canceled",
		EntityType: "company_invitation",
		EntityID:   invitationID,
		After:      map[string]any{"status": "canceled"},
		Metadata:   map[string]any{"company_id": companyID, "actor_id": requestorID},
	})
	return nil
}

func (s *InvitationService) GetInvitation(ctx context.Context, token string) (*invitation.Invitation, bool, error) {
	invite, err := s.invitationServiceRepo.GetByTokenService(ctx, token)
	if err != nil {
		return nil, false, err
	}
	if invite == nil {
		return nil, false, errors.New("invalid invitation token")
	}
	if invite.Status != "pending" {
		return nil, false, errors.New("invitation is not pending")
	}
	if time.Now().After(invite.ExpiresAt) {
		return nil, false, errors.New("invitation expired")
	}

	isRegistered := false
	cred, err := s.credentialServiceRepo.GetByEmailService(ctx, invite.Email)
	if err == nil && cred != nil {
		isRegistered = true
	}

	return invite, isRegistered, nil
}

func (s *InvitationService) AcceptPublicInvitation(ctx context.Context, token string, req *auth.RegisterRequest) (*auth.TokenPair, error) {
	invite, _, err := s.GetInvitation(ctx, token)
	if err != nil {
		return nil, err
	}
	if s.authService == nil || s.authService.supabaseAuth == nil {
		return nil, errors.New("supabase auth not configured")
	}
	if req.Password == "" {
		return nil, errors.New("password is required")
	}

	var sess *supabaseauth.Session
	sess, err = s.authService.supabaseAuth.SignInWithPassword(ctx, invite.Email, req.Password)
	if err != nil {
		if req.FirstName == "" {
			return nil, errors.New("firstname is required to create account")
		}
		sess, err = s.authService.supabaseAuth.SignUp(ctx, invite.Email, req.Password)
		if err != nil {
			return nil, err
		}
	}

	userID := sess.UserID
	userAcc, err := s.accountServiceRepo.GetByIDService(ctx, userID)
	if err != nil {
		return nil, err
	}
	if userAcc == nil {
		userAcc = &account.Account{
			ID:        userID,
			Type:      account.AccountTypeIndividual,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Individual: &user.User{
				ID:        userID,
				FirstName: req.FirstName,
				LastName:  req.LastName,
				FullName:  strings.TrimSpace(req.FirstName + " " + req.LastName),
				Email:     invite.Email,
				Phone:     req.Phone,
				Bio:       req.Bio,
				Role:      "user",
				IsActive:  true,
			},
		}
		if err := s.accountServiceRepo.CreateService(ctx, userAcc); err != nil {
			return nil, err
		}
	}

	acceptReq := &invitation.AcceptInvitationRequest{Token: token}
	if err := s.AcceptInvitation(ctx, userID, acceptReq); err != nil {
		return nil, err
	}

	functionalRole := invite.Role
	if !pkgAuth.IsAssignableRole(functionalRole) {
		if s.authService.tenantSettingsRepo != nil {
			defRole, _ := s.authService.tenantSettingsRepo.GetDefaultRole(ctx, invite.CompanyID)
			if defRole != "" {
				functionalRole = defRole
			} else {
				functionalRole = "corretor"
			}
		} else {
			functionalRole = "corretor"
		}
	}
	profile := "member"
	if functionalRole == pkgAuth.FunctionalRoleAdmin {
		profile = "admin"
	}

	appMeta := map[string]interface{}{
		// P2/M9: `account_id` é a identidade (conta local criada/reutilizada pelo
		// convite); `tenant_id` é a EMPRESA que convidou. Antes o cliente
		// convidado não tinha identidade própria e o middleware usava o
		// `tenant_id` como se fosse conta — `/me`, `/lgpd/me/account` e checagem
		// de vínculo passavam a operar sobre o id da empresa.
		"account_id":      userAcc.ID,
		"tenant_id":       invite.CompanyID,
		"permission":      profile,
		"functional_role": functionalRole,
		"type":            string(userAcc.Type),
	}
	if err := s.authService.supabaseAuth.UpdateAppMetadata(ctx, userID, appMeta); err != nil {
		return nil, err
	}

	refreshed, err := s.authService.supabaseAuth.RefreshToken(ctx, sess.RefreshToken)
	if err != nil {
		return nil, err
	}

	return &auth.TokenPair{
		AccessToken:  refreshed.AccessToken,
		RefreshToken: refreshed.RefreshToken,
	}, nil
}

func (s *InvitationService) AcceptInvitation(ctx context.Context, userID string, req *invitation.AcceptInvitationRequest) error {
	invite, err := s.invitationServiceRepo.GetByTokenService(ctx, req.Token)
	if err != nil {
		return err
	}
	if invite == nil {
		return errors.New("invalid invitation token")
	}
	if invite.Status != "pending" {
		return errors.New("invitation is not pending")
	}
	if time.Now().After(invite.ExpiresAt) {
		_ = s.invitationServiceRepo.UpdateStatusService(ctx, invite.CompanyID, invite.ID, "expired")
		return errors.New("invitation expired")
	}

	userAcc, err := s.accountServiceRepo.GetByIDService(ctx, userID)
	if err != nil {
		return err
	}
	if userAcc == nil {
		return errors.New("user not found")
	}
	userEmail := ""
	if userAcc.Individual != nil {
		userEmail = userAcc.Individual.Email
	}
	if strings.TrimSpace(strings.ToLower(userEmail)) != strings.TrimSpace(strings.ToLower(invite.Email)) {
		return errors.New("email mismatch")
	}

	compAcc, err := s.accountServiceRepo.GetByIDService(ctx, invite.CompanyID)
	if err != nil {
		return err
	}
	if compAcc == nil {
		return errors.New("company not found")
	}

	isMember := false
	for _, m := range compAcc.Company.Members {
		if m.AccountID == userID {
			isMember = true
			break
		}
	}

	if !isMember {
		functionalRole := invite.Role
		if !pkgAuth.IsAssignableRole(functionalRole) {
			// Fallback if invalid or empty
			if s.authService.tenantSettingsRepo != nil {
				defRole, _ := s.authService.tenantSettingsRepo.GetDefaultRole(ctx, invite.CompanyID)
				if defRole != "" {
					functionalRole = defRole
				} else {
					functionalRole = "corretor"
				}
			} else {
				functionalRole = "corretor"
			}
		}

		// Map profile based on functional role
		profile := "member"
		if functionalRole == pkgAuth.FunctionalRoleAdmin {
			profile = "admin"
		}

		compAcc.Company.Members = append(compAcc.Company.Members, company.CompanyMember{
			AccountID:      userID,
			Profile:        profile,
			FunctionalRole: functionalRole,
		})
		if err := s.accountServiceRepo.UpdateService(ctx, compAcc); err != nil {
			return err
		}
	}

	if err := s.invitationServiceRepo.UpdateStatusService(ctx, invite.CompanyID, invite.ID, "accepted"); err != nil {
		return err
	}

	if s.groupService != nil {
		if err := s.groupService.EnsureDefaultGroupForMember(ctx, invite.CompanyID, userID); err != nil {
			logger.Warn("Failed to ensure default group for member upon invitation acceptance", "companyID", invite.CompanyID, "userID", userID, "error", err)
		}
	}

	return nil
}
