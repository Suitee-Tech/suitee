package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/oginoino/toca-auth/internal/app"
	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca-auth/internal/domain/auth"
	"github.com/oginoino/toca-auth/internal/domain/user"
	supabaseauth "github.com/oginoino/toca-auth/internal/infrastructure/supabase"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/events"
)

type fakeCredentialRepo struct {
	byEmail map[string]*auth.Credential
}

func (r *fakeCredentialRepo) Create(ctx context.Context, cred *auth.Credential) error {
	panic("unexpected Create")
}

func (r *fakeCredentialRepo) GetByEmail(ctx context.Context, email string) (*auth.Credential, error) {
	if r.byEmail == nil {
		return nil, nil
	}
	return r.byEmail[email], nil
}

func (r *fakeCredentialRepo) GetByAccountID(ctx context.Context, accountID string) (*auth.Credential, error) {
	return nil, nil
}

func (r *fakeCredentialRepo) UpdatePassword(ctx context.Context, email, passwordHash string) error {
	return nil
}

func (r *fakeCredentialRepo) VerifyEmail(ctx context.Context, email string) error {
	return nil
}

// Service-role (pre-auth) variants mirror the tenant-scoped methods above.
func (r *fakeCredentialRepo) CreateService(ctx context.Context, cred *auth.Credential) error {
	return r.Create(ctx, cred)
}

func (r *fakeCredentialRepo) GetByEmailService(ctx context.Context, email string) (*auth.Credential, error) {
	return r.GetByEmail(ctx, email)
}

func (r *fakeCredentialRepo) GetByAccountIDService(ctx context.Context, accountID string) (*auth.Credential, error) {
	return r.GetByAccountID(ctx, accountID)
}

type fakeAccountRepo struct {
	mu          sync.Mutex
	byID        map[string]*account.Account
	byEmail     map[string]*account.Account
	byPhoneNorm map[string]*account.Account
}

func (r *fakeAccountRepo) Create(ctx context.Context, acc *account.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byID == nil {
		r.byID = map[string]*account.Account{}
	}
	r.byID[acc.ID] = acc
	return nil
}

func (r *fakeAccountRepo) GetByID(ctx context.Context, id string) (*account.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byID == nil {
		return nil, nil
	}
	return r.byID[id], nil
}

func (r *fakeAccountRepo) GetByIndividualEmail(ctx context.Context, email string) (*account.Account, error) {
	if r.byEmail == nil {
		return nil, nil
	}
	return r.byEmail[strings.ToLower(email)], nil
}

func (r *fakeAccountRepo) GetByIndividualPhone(ctx context.Context, phone string) (*account.Account, error) {
	if r.byPhoneNorm == nil {
		return nil, nil
	}
	return r.byPhoneNorm[normalizePhone(phone)], nil
}

func (r *fakeAccountRepo) Update(ctx context.Context, acc *account.Account) error {
	return nil
}

func (r *fakeAccountRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byID == nil {
		return nil
	}
	delete(r.byID, id)
	return nil
}

func (r *fakeAccountRepo) GetCompaniesByMemberID(ctx context.Context, memberID string) ([]*account.Account, error) {
	return nil, nil
}

func (r *fakeAccountRepo) GetCompanyByTaxIDOrEmail(ctx context.Context, taxID, email string) (*account.Account, error) {
	return nil, nil
}

// Service-role (pre-auth) variants mirror the tenant-scoped methods above.
func (r *fakeAccountRepo) CreateService(ctx context.Context, acc *account.Account) error {
	return r.Create(ctx, acc)
}

func (r *fakeAccountRepo) GetByIDService(ctx context.Context, id string) (*account.Account, error) {
	return r.GetByID(ctx, id)
}

func (r *fakeAccountRepo) GetByIndividualEmailService(ctx context.Context, email string) (*account.Account, error) {
	return r.GetByIndividualEmail(ctx, email)
}

func (r *fakeAccountRepo) GetByIndividualPhoneService(ctx context.Context, phone string) (*account.Account, error) {
	return r.GetByIndividualPhone(ctx, phone)
}

func (r *fakeAccountRepo) UpdateService(ctx context.Context, acc *account.Account) error {
	return r.Update(ctx, acc)
}

func (r *fakeAccountRepo) DeleteService(ctx context.Context, id string) error {
	return r.Delete(ctx, id)
}

type fakeSupabase struct {
	mu           sync.Mutex
	recoveryLink string
	sent         []struct {
		email, phone, channel string
		createUser            bool
	}
	generated    []struct{ email string }
	emailOTP     string
	verifyErr    error
	verifyEmail  string
	verifyPhone  string
	verifyType   string
	verifyToken  string
	updateUserID string
	updateMeta   map[string]interface{}
	// P1/A14: captura a troca de senha e a revogação de sessões.
	passwordUpdates []struct{ userID, password string }
	logoutAllTokens []string
	signInErr       error
	updatePwdErr    error
	logoutAllErr    error
	refreshIn       string
	// Reset de senha mediado pelo backend: dono devolvido por GetUser e erros
	// injetáveis para cobrir os caminhos de recusa.
	getUserTokens []string
	getUserID     string
	getUserErr    error
	sessionUserID string
	refreshErr    error
}

func (f *fakeSupabase) SignUp(ctx context.Context, email, password string) (*supabaseauth.Session, error) {
	return nil, errors.New("unexpected SignUp")
}

func (f *fakeSupabase) SignInWithPassword(ctx context.Context, email, password string) (*supabaseauth.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.signInErr != nil {
		return nil, f.signInErr
	}
	return &supabaseauth.Session{AccessToken: "access", RefreshToken: "refresh", UserID: "acc1", Email: email}, nil
}

func (f *fakeSupabase) GetUser(ctx context.Context, accessToken string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getUserTokens = append(f.getUserTokens, accessToken)
	if f.getUserErr != nil {
		return "", f.getUserErr
	}
	if f.getUserID != "" {
		return f.getUserID, nil
	}
	if f.sessionUserID != "" {
		return f.sessionUserID, nil
	}
	return "acc1", nil
}

func (f *fakeSupabase) UpdateUserPassword(ctx context.Context, userID, password string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updatePwdErr != nil {
		return f.updatePwdErr
	}
	f.passwordUpdates = append(f.passwordUpdates, struct{ userID, password string }{userID, password})
	return nil
}

func (f *fakeSupabase) RefreshToken(ctx context.Context, refreshToken string) (*supabaseauth.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshIn = refreshToken
	if f.refreshErr != nil {
		return nil, f.refreshErr
	}
	userID := f.sessionUserID
	if userID == "" {
		userID = "acc1"
	}
	return &supabaseauth.Session{
		AccessToken:  "access-refreshed",
		RefreshToken: "refresh-refreshed",
		UserID:       userID,
		Email:        "client@example.com",
	}, nil
}

func (f *fakeSupabase) Logout(ctx context.Context, accessToken string) error {
	return errors.New("unexpected Logout")
}

func (f *fakeSupabase) LogoutAll(ctx context.Context, accessToken string) error {
	f.mu.Lock()
	f.logoutAllTokens = append(f.logoutAllTokens, accessToken)
	err := f.logoutAllErr
	f.mu.Unlock()
	return err
}

func (f *fakeSupabase) GenerateEmailOTP(ctx context.Context, email string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.generated = append(f.generated, struct{ email string }{email: email})
	if f.emailOTP == "" {
		f.emailOTP = "275123"
	}
	return f.emailOTP, nil
}

func (f *fakeSupabase) GenerateRecoveryLink(ctx context.Context, email string, redirectTo string) (string, error) {
	f.mu.Lock()
	f.recoveryLink = redirectTo + "#access_token=access&refresh_token=refresh&type=recovery"
	link := f.recoveryLink
	f.mu.Unlock()
	return link, nil
}

func (f *fakeSupabase) GenerateRecoveryTokenHash(ctx context.Context, email string) (string, error) {
	return "valid-recovery-hash", nil
}

func (f *fakeSupabase) VerifyRecoveryTokenHash(ctx context.Context, tokenHash string) (*supabaseauth.Session, error) {
	if tokenHash != "valid-recovery-hash" {
		return nil, errors.New("invalid recovery token")
	}
	return &supabaseauth.Session{AccessToken: "access-from-hash", RefreshToken: "refresh-from-hash", UserID: "acc1", Email: "u@example.com"}, nil
}

func (f *fakeSupabase) SendOTP(ctx context.Context, email, phone, channel string, createUser bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, struct {
		email, phone, channel string
		createUser            bool
	}{email: email, phone: phone, channel: channel, createUser: createUser})
	return nil
}

func (f *fakeSupabase) VerifyOTP(ctx context.Context, typ, email, phone, token string) (*supabaseauth.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verifyType = typ
	f.verifyEmail = email
	f.verifyPhone = phone
	f.verifyToken = token
	if f.verifyErr != nil {
		return nil, f.verifyErr
	}
	return &supabaseauth.Session{
		AccessToken:  "access",
		RefreshToken: "refresh",
		UserID:       "acc1",
		Email:        "client@example.com",
	}, nil
}

func (f *fakeSupabase) UpdateAppMetadata(ctx context.Context, userID string, appMetadata map[string]interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateUserID = userID
	f.updateMeta = appMetadata
	return nil
}

func (f *fakeSupabase) DeleteUser(ctx context.Context, userID string) error {
	return nil
}

type fakeEventBus struct {
	mu     sync.Mutex
	events []events.Event
}

func (b *fakeEventBus) Publish(ctx context.Context, event events.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, event)
	return nil
}

func (b *fakeEventBus) PublishReliable(ctx context.Context, event events.Event) error {
	return b.Publish(ctx, event)
}

func (b *fakeEventBus) Subscribe(eventName string, handler events.EventHandler) error { return nil }
func (b *fakeEventBus) SubscribeWithQueue(queueName string, eventName string, handler events.EventHandler) error {
	return nil
}

func newTestAuthService(accRepo account.Repository, credRepo auth.Repository, sb supabaseauth.Client) *AuthService {
	svc := &AuthService{
		accountRepo:    accRepo,
		credentialRepo: credRepo,
		supabaseAuth:   sb,
		config:         &app.Config{},
	}
	// Wire the service-role (pre-auth) surfaces when the fakes implement them,
	// so login/OTP/e-mail availability tests exercise the same code paths as prod.
	if ar, ok := accRepo.(account.ServiceRepository); ok {
		svc.accountServiceRepo = ar
	}
	if cr, ok := credRepo.(auth.ServiceRepository); ok {
		svc.credentialServiceRepo = cr
	}
	return svc
}

func TestRequestOTP_Email_GeneratesOTPAndPublishesEvent(t *testing.T) {
	ctx := context.Background()
	sb := &fakeSupabase{}
	accRepo := &fakeAccountRepo{byID: map[string]*account.Account{"acc1": {ID: "acc1"}}}
	credRepo := &fakeCredentialRepo{byEmail: map[string]*auth.Credential{"client@example.com": {AccountID: "acc1", Email: "client@example.com"}}}
	svc := newTestAuthService(accRepo, credRepo, sb)
	bus := &fakeEventBus{}
	svc.eventBus = bus

	if err := svc.RequestOTP(ctx, "client@example.com", "email"); err != nil {
		t.Fatalf("RequestOTP error: %v", err)
	}

	if len(sb.generated) != 1 || sb.generated[0].email != "client@example.com" {
		t.Fatalf("expected GenerateEmailOTP to be called")
	}
	if len(bus.events) != 1 || bus.events[0].Name() != "auth.otp.requested" {
		t.Fatalf("expected auth.otp.requested event to be published")
	}
}

// Contato desconhecido no ramo e-mail: a resposta continua genérica de sucesso
// (não confirma existência de conta), mas o descarte precisa deixar rastro —
// era o único caminho do fluxo que devolvia 200 sem log nem audit, e por isso
// "código de acesso nunca recebido" ficava invisível para o suporte.
func TestRequestOTP_Email_UnknownContactSkipsSilentlyButLeavesTrace(t *testing.T) {
	ctx := context.Background()

	var entries []audit.Entry
	cleanup := audit.SetTestSink(func(e audit.Entry) { entries = append(entries, e) })
	t.Cleanup(cleanup)

	sb := &fakeSupabase{}
	svc := newTestAuthService(&fakeAccountRepo{}, &fakeCredentialRepo{}, sb)
	bus := &fakeEventBus{}
	svc.eventBus = bus
	svc.otpLimiter = NewOTPContactLimiter(nil)

	if err := svc.RequestOTP(ctx, "desconhecido@example.com", "email"); err != nil {
		t.Fatalf("RequestOTP de contato desconhecido deve responder sucesso: %v", err)
	}
	if len(sb.generated) != 0 {
		t.Fatalf("contato desconhecido não pode gerar código: %+v", sb.generated)
	}
	if len(bus.events) != 0 {
		t.Fatalf("contato desconhecido não pode publicar auth.otp.requested: %+v", bus.events)
	}

	found := false
	for _, e := range entries {
		if e.Action != "auth.otp_skipped_unknown_contact" {
			continue
		}
		found = true
		if e.Metadata["method"] != "email" || e.Metadata["reason"] != "unknown_contact" {
			t.Fatalf("trilha sem o motivo do descarte: %+v", e.Metadata)
		}
		if e.Metadata["contact_hash"] == "" || e.Metadata["contact_hash"] == "desconhecido@example.com" {
			t.Fatalf("trilha deveria registrar o hash do contato, não o contato: %+v", e.Metadata)
		}
	}
	if !found {
		t.Fatal("descarte de contato desconhecido sem trilha de auditoria")
	}
}

// M14 — o ramo WhatsApp só envia para contato que JÁ é cliente, e sem
// `create_user` (a conta existe; o Supabase não deve criar uma segunda
// identidade para o mesmo telefone).
func TestRequestOTP_WhatsApp_OnlyForKnownContact(t *testing.T) {
	ctx := context.Background()
	contact := "(11) 98765-4321"
	phone := normalizePhone(contact)

	// Contato conhecido: envia.
	sb := &fakeSupabase{}
	repo := &fakeAccountRepo{byPhoneNorm: map[string]*account.Account{
		phone: {ID: "acc-client", Type: account.AccountTypeIndividual},
	}}
	svc := newTestAuthService(repo, &fakeCredentialRepo{}, sb)
	if err := svc.RequestOTP(ctx, contact, "whatsapp"); err != nil {
		t.Fatalf("RequestOTP error: %v", err)
	}
	if len(sb.sent) != 1 || sb.sent[0].phone != phone || sb.sent[0].channel != "whatsapp" {
		t.Fatalf("expected whatsapp OTP to be sent with normalized phone")
	}
	if sb.sent[0].createUser {
		t.Fatal("SendOTP pediu create_user=true para contato existente — cria identidade duplicada")
	}
}

// Contato desconhecido: resposta genérica de sucesso, nada enviado, nada criado.
func TestRequestOTP_WhatsApp_SkipsUnknownContact(t *testing.T) {
	ctx := context.Background()
	sb := &fakeSupabase{}
	svc := newTestAuthService(&fakeAccountRepo{}, &fakeCredentialRepo{}, sb)

	if err := svc.RequestOTP(ctx, "(11) 90000-0000", "whatsapp"); err != nil {
		t.Fatalf("contato desconhecido deveria devolver sucesso genérico, veio: %v", err)
	}
	if len(sb.sent) != 0 {
		t.Fatalf("OTP foi enviado para contato desconhecido: %+v", sb.sent)
	}
}

func TestVerifyOTP_Email_ReturnsRefreshedTokensAndSetsMetadata(t *testing.T) {
	ctx := context.Background()

	sb := &fakeSupabase{}
	accRepo := &fakeAccountRepo{}
	svc := newTestAuthService(accRepo, &fakeCredentialRepo{}, sb)

	tokens, err := svc.VerifyOTP(ctx, "client@example.com", "123456")
	if err != nil {
		t.Fatalf("VerifyOTP error: %v", err)
	}
	if tokens.AccessToken != "access-refreshed" || tokens.RefreshToken != "refresh-refreshed" {
		t.Fatalf("unexpected token pair")
	}

	if sb.updateUserID != "acc1" {
		t.Fatalf("expected UpdateAppMetadata to be called")
	}
	if sb.updateMeta["tenant_id"] != "acc1" || sb.updateMeta["functional_role"] != "cliente" {
		t.Fatalf("unexpected app metadata: %#v", sb.updateMeta)
	}

	acc, _ := accRepo.GetByID(ctx, "acc1")
	if acc == nil || acc.Individual == nil || acc.Individual.Email != "client@example.com" {
		t.Fatalf("expected account to be created")
	}
}

func TestVerifyOTP_InvalidCode_ReturnsNeutralError(t *testing.T) {
	ctx := context.Background()
	sb := &fakeSupabase{verifyErr: errors.New("invalid")}
	svc := newTestAuthService(&fakeAccountRepo{}, &fakeCredentialRepo{}, sb)

	_, err := svc.VerifyOTP(ctx, "client@example.com", "000000")
	if err == nil || err.Error() != "invalid or expired code" {
		t.Fatalf("expected neutral error, got %v", err)
	}
}

func TestNormalizePhone_Idempotent(t *testing.T) {
	if normalizePhone("(11) 98765-4321") != normalizePhone("11987654321") {
		t.Fatalf("expected normalized phones to match")
	}
}

func TestRegistration_PreChecksDuplicates(t *testing.T) {
	ctx := context.Background()
	cfg := &app.Config{}

	t.Run("email already in accounts → rejects before Supabase", func(t *testing.T) {
		accRepo := &fakeAccountRepo{byEmail: map[string]*account.Account{
			"taken@example.com": {ID: "acc-existing"},
		}}
		// fakeSupabase's SignUp panics with "unexpected SignUp" if reached, so this verifies the pre-check.
		svc := NewRegistrationService(accRepo, &fakeSupabase{}, nil, cfg)
		_, err := svc.Register(ctx, &auth.RegisterRequest{
			Email:           "Taken@Example.com",
			Password:        "ValidPass1!",
			ConfirmPassword: "ValidPass1!",
			Type:            "individual",
			FirstName:       "A",
			LastName:        "B",
		})
		if !errors.Is(err, auth.ErrEmailAlreadyExists) {
			t.Fatalf("expected ErrEmailAlreadyExists, got %v", err)
		}
	})

	t.Run("phone already in accounts → rejects before Supabase", func(t *testing.T) {
		accRepo := &fakeAccountRepo{byPhoneNorm: map[string]*account.Account{
			"11999998888": {ID: "acc-existing"},
		}}
		svc := NewRegistrationService(accRepo, &fakeSupabase{}, nil, cfg)
		_, err := svc.Register(ctx, &auth.RegisterRequest{
			Email:           "fresh@example.com",
			Password:        "ValidPass1!",
			ConfirmPassword: "ValidPass1!",
			Type:            "individual",
			FirstName:       "A",
			LastName:        "B",
			Phone:           "(11) 99999-8888",
		})
		if !errors.Is(err, auth.ErrPhoneAlreadyExists) {
			t.Fatalf("expected ErrPhoneAlreadyExists, got %v", err)
		}
	})
}

// ─────────────────── #463 UpdateUserInfo banking ───────────────────

// capturingAccountRepo persiste updates de fato (override do no-op da
// fakeAccountRepo) para que os testes de banking possam ler de volta
// o estado pós-Update.
type capturingAccountRepo struct {
	fakeAccountRepo
	updateCalled int
}

func (r *capturingAccountRepo) Update(ctx context.Context, acc *account.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byID == nil {
		r.byID = map[string]*account.Account{}
	}
	r.byID[acc.ID] = acc
	r.updateCalled++
	return nil
}

func TestUpdateUserInfo_PersistsBanking(t *testing.T) {
	ctx := context.Background()
	accRepo := &capturingAccountRepo{}
	accRepo.byID = map[string]*account.Account{
		"acc1": {
			ID:         "acc1",
			Individual: &user.User{ID: "acc1", FirstName: "Ana", LastName: "Silva", Email: "ana@x.com"},
		},
	}
	svc := newTestAuthService(accRepo, &fakeCredentialRepo{}, &fakeSupabase{})

	banking := &user.BankingInfo{
		BankCode:    "341",
		BankName:    "Itaú",
		Agency:      "1234",
		Account:     "56789",
		AccountType: user.BankAccountTypeChecking,
		PixKeyType:  user.PixKeyTypeEmail,
		PixKey:      "ana@x.com",
	}

	err := svc.UpdateUserInfo(ctx, "acc1", map[string]interface{}{
		"individual.banking": banking,
	})
	if err != nil {
		t.Fatalf("UpdateUserInfo: %v", err)
	}
	if accRepo.updateCalled != 1 {
		t.Errorf("expected 1 Update call, got %d", accRepo.updateCalled)
	}
	got := accRepo.byID["acc1"].Individual.Banking
	if got.BankCode != "341" || got.PixKey != "ana@x.com" {
		t.Errorf("banking not persisted: %+v", got)
	}
}

func TestUpdateUserInfo_RejectsInvalidBanking(t *testing.T) {
	ctx := context.Background()
	accRepo := &capturingAccountRepo{}
	accRepo.byID = map[string]*account.Account{
		"acc1": {ID: "acc1", Individual: &user.User{ID: "acc1", FirstName: "Ana", LastName: "Silva", Email: "ana@x.com"}},
	}
	svc := newTestAuthService(accRepo, &fakeCredentialRepo{}, &fakeSupabase{})

	// PIX type CPF + chave inválida → service rejeita mesmo se caller
	// bypassar o handler (defesa em profundidade).
	banking := &user.BankingInfo{
		PixKeyType: user.PixKeyTypeCPF,
		PixKey:     "111.111.111-11",
	}
	err := svc.UpdateUserInfo(ctx, "acc1", map[string]interface{}{
		"individual.banking": banking,
	})
	if err == nil {
		t.Fatal("expected validation error from service")
	}
	if accRepo.updateCalled != 0 {
		t.Errorf("Update should NOT be called on invalid banking; got %d", accRepo.updateCalled)
	}
}

func TestUpdateUserInfo_AllowsPartialUpdateWithoutBanking(t *testing.T) {
	// UpdateUserInfo continua atualizando firstname etc. quando
	// banking não está no updates map (compat com chamadas legacy).
	ctx := context.Background()
	accRepo := &capturingAccountRepo{}
	accRepo.byID = map[string]*account.Account{
		"acc1": {ID: "acc1", Individual: &user.User{ID: "acc1", FirstName: "Ana", LastName: "Silva", Email: "ana@x.com"}},
	}
	svc := newTestAuthService(accRepo, &fakeCredentialRepo{}, &fakeSupabase{})

	err := svc.UpdateUserInfo(ctx, "acc1", map[string]interface{}{
		"individual.firstname": "Mariana",
	})
	if err != nil {
		t.Fatalf("UpdateUserInfo: %v", err)
	}
	got := accRepo.byID["acc1"].Individual
	if got.FirstName != "Mariana" {
		t.Errorf("expected firstname updated, got %q", got.FirstName)
	}
	// Banking nem foi mexido.
	if !got.Banking.IsEmpty() {
		t.Errorf("banking should remain empty, got %+v", got.Banking)
	}
}
