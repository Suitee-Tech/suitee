package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca-auth/internal/app"
	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca-auth/internal/domain/auth"
	"github.com/oginoino/toca-auth/internal/domain/company"
	domainevents "github.com/oginoino/toca-auth/internal/domain/events"
	"github.com/oginoino/toca-auth/internal/domain/user"
	supabaseauth "github.com/oginoino/toca-auth/internal/infrastructure/supabase"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/logger"
)

type RegistrationService struct {
	accountRepo  account.ServiceRepository
	supabaseAuth supabaseauth.Client
	eventBus     events.EventBus
	config       *app.Config
}

func NewRegistrationService(
	accRepo account.ServiceRepository,
	supabaseClient supabaseauth.Client,
	eventBus events.EventBus,
	cfg *app.Config,
) *RegistrationService {
	return &RegistrationService{
		accountRepo:  accRepo,
		supabaseAuth: supabaseClient,
		eventBus:     eventBus,
		config:       cfg,
	}
}

func (s *RegistrationService) Register(ctx context.Context, req *auth.RegisterRequest) (*auth.TokenPair, error) {
	if s.supabaseAuth == nil {
		return nil, errors.New("supabase auth not configured")
	}
	if err := auth.ValidatePasswordComplexity(req.Password); err != nil {
		return nil, err
	}

	// Pre-check duplicates in our database to fail fast with a predictable error
	// instead of relying on Supabase signup error parsing.
	emailLower := strings.ToLower(strings.TrimSpace(req.Email))
	if emailLower != "" {
		if existing, _ := s.accountRepo.GetByIndividualEmailService(ctx, emailLower); existing != nil {
			return nil, auth.ErrEmailAlreadyExists
		}
	}
	if req.Type == "individual" && strings.TrimSpace(req.Phone) != "" {
		if existing, _ := s.accountRepo.GetByIndividualPhoneService(ctx, req.Phone); existing != nil {
			return nil, auth.ErrPhoneAlreadyExists
		}
	}

	session, err := s.supabaseAuth.SignUp(ctx, req.Email, req.Password)
	if err != nil {
		return nil, err
	}

	userID := session.UserID
	now := time.Now()

	if existing, _ := s.accountRepo.GetByIDService(ctx, userID); existing != nil {
		return nil, auth.ErrEmailAlreadyExists
	}

	var tenantID string
	appPermission := ""
	appFunctionalRole := ""
	createdAccountIDs := []string{}

	rollback := func() {
		for _, id := range createdAccountIDs {
			_ = s.accountRepo.DeleteService(ctx, id)
		}
		_ = s.supabaseAuth.DeleteUser(ctx, userID)
	}

	userAcc := &account.Account{
		ID:        userID,
		Type:      account.AccountTypeIndividual,
		CreatedAt: now,
		UpdatedAt: now,
		Individual: &user.User{
			ID:       userID,
			Email:    req.Email,
			Role:     "user",
			IsActive: true,
			Phone:    req.Phone,
			Bio:      req.Bio,
		},
	}

	switch req.Type {
	case "individual":
		userAcc.Individual.FirstName = req.FirstName
		userAcc.Individual.LastName = req.LastName
		userAcc.Individual.FullName = strings.TrimSpace(req.FirstName + " " + req.LastName)
		tenantID = userID
	case "company":
		companyID := uuid.NewString()
		tenantID = companyID
		appPermission = "owner"
		appFunctionalRole = "admin"

		if req.CompanyName != "" {
			userAcc.Individual.FirstName = req.CompanyName
			userAcc.Individual.FullName = req.CompanyName
		}

		compAcc := &account.Account{
			ID:        companyID,
			Type:      account.AccountTypeCompany,
			CreatedAt: now,
			UpdatedAt: now,
			Company: &company.Company{
				ID:       uuid.NewString(),
				Name:     req.CompanyName,
				TaxID:    req.TaxID,
				Email:    req.Email,
				Type:     "franchisee",
				IsActive: true,
				OwnerID:  userID,
				Members:  []company.CompanyMember{{AccountID: userID, Profile: "owner", FunctionalRole: "admin"}},
			},
		}

		if err := s.accountRepo.CreateService(ctx, compAcc); err != nil {
			rollback()
			return nil, err
		}
		createdAccountIDs = append(createdAccountIDs, companyID)
	default:
		rollback()
		return nil, errors.New("invalid account type")
	}

	if err := s.accountRepo.CreateService(ctx, userAcc); err != nil {
		rollback()
		return nil, err
	}
	createdAccountIDs = append(createdAccountIDs, userID)

	appMeta := map[string]interface{}{
		// P2/M9: identidade em claim própria (`account_id`) e tenant no
		// `tenant_id` — a mesma separação vale para o cadastro novo.
		"account_id":      userID,
		"tenant_id":       tenantID,
		"permission":      appPermission,
		"functional_role": appFunctionalRole,
		"type":            req.Type,
	}
	if err := s.supabaseAuth.UpdateAppMetadata(ctx, userID, appMeta); err != nil {
		rollback()
		return nil, err
	}

	refreshed, err := s.supabaseAuth.RefreshToken(ctx, session.RefreshToken)
	if err != nil {
		refreshed, err = s.supabaseAuth.SignInWithPassword(ctx, req.Email, req.Password)
		if err != nil {
			rollback()
			return nil, err
		}
	}

	if s.eventBus != nil && userAcc.Individual != nil {
		eventPayload := domainevents.UserRegisteredEvent{
			Email:     req.Email,
			FirstName: userAcc.Individual.FirstName,
			TenantID:  tenantID,
			AppURL:    s.config.FrontendURL,
		}
		if err := s.eventBus.Publish(ctx, eventPayload); err != nil {
			logger.Error("Failed to publish user registered event", "error", err)
		}
	}

	return &auth.TokenPair{
		AccessToken:  refreshed.AccessToken,
		RefreshToken: refreshed.RefreshToken,
	}, nil
}
