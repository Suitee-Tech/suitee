package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca-auth/internal/domain/auth"
	"github.com/oginoino/toca-auth/internal/domain/company"
	"github.com/oginoino/toca-auth/internal/domain/group"
	"github.com/oginoino/toca/pkg/audit"
	pkgAuth "github.com/oginoino/toca/pkg/auth"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/logger"
)

type CompanyService struct {
	accountRepo    account.Repository
	credentialRepo auth.Repository
	groupRepo      group.Repository
	groupService   *GroupService
	// Superfícies de serviço (BYPASSRLS) para ler o detalhe de OUTROS membros:
	// `toca_auth.accounts`/`credentials` só permitem ao pool de aplicação
	// (toca_app) a própria linha (`id = auth.uid()`), então a lista de equipe
	// ficava vazia/sem nome/avatar para os colegas.
	accountServiceRepo    account.ServiceRepository
	credentialServiceRepo auth.ServiceRepository
	// membershipRevoker grava a marca de revogação do vínculo (P1/A15). Nil cai
	// para o Redis do ambiente.
	membershipRevoker pkgAuth.MembershipRevoker
}

// WithServiceRepos injeta as superfícies de serviço para leitura do detalhe de
// outros membros. Sem elas o serviço cai no pool de aplicação (testes).
func (s *CompanyService) WithServiceRepos(
	accountServiceRepo account.ServiceRepository,
	credentialServiceRepo auth.ServiceRepository,
) *CompanyService {
	s.accountServiceRepo = accountServiceRepo
	s.credentialServiceRepo = credentialServiceRepo
	return s
}

// memberAccount lê a conta de um membro da empresa pela pool de serviço.
func (s *CompanyService) memberAccount(ctx context.Context, accountID string) (*account.Account, error) {
	if s.accountServiceRepo != nil {
		return s.accountServiceRepo.GetByIDService(ctx, accountID)
	}
	return s.accountRepo.GetByID(ctx, accountID)
}

// memberCredential lê a credencial (e-mail) de um membro da empresa pela pool
// de serviço.
func (s *CompanyService) memberCredential(ctx context.Context, accountID string) (*auth.Credential, error) {
	if s.credentialServiceRepo != nil {
		return s.credentialServiceRepo.GetByAccountIDService(ctx, accountID)
	}
	return s.credentialRepo.GetByAccountID(ctx, accountID)
}

// WithMembershipRevoker injeta o gravador de revogação (Redis ou dublê).
func (s *CompanyService) WithMembershipRevoker(revoker pkgAuth.MembershipRevoker) *CompanyService {
	s.membershipRevoker = revoker
	return s
}

func NewCompanyService(
	accRepo account.Repository,
	credRepo auth.Repository,
	groupRepo group.Repository,
	groupService *GroupService,
) *CompanyService {
	return &CompanyService{
		accountRepo:    accRepo,
		credentialRepo: credRepo,
		groupRepo:      groupRepo,
		groupService:   groupService,
	}
}

func normalizeTaxID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	out := make([]byte, 0, len(value))
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if ch >= '0' && ch <= '9' {
			out = append(out, ch)
		}
	}
	return string(out)
}

// callerIsCompanyMember decide se o chamador pode ver a empresa.
//
// SECURITY (achado C1 da revisão de permissões de 2026-09-19): antes,
// `GET /api/v1/companies/:id` devolvia razão social, CNPJ, e-mail, telefone,
// endereço, owner_id e a lista de membros (com e-mails) de QUALQUER empresa
// para qualquer principal autenticado, bastando o UUID. Agora exige vínculo.
//
// Mesma regra de ListCompanyMembers: o tenant do token bate com a empresa, OU
// o chamador é membro registrado.
func (s *CompanyService) callerIsCompanyMember(ctx context.Context, requestorID string, compAcc *account.Account) bool {
	if compAcc == nil || compAcc.Company == nil {
		return false
	}
	if requestorID != "" && compAcc.Company.OwnerID == requestorID {
		return true
	}
	if tenantID, _ := pkgcontext.GetTenantID(ctx); tenantID != "" && tenantID == compAcc.ID {
		return true
	}
	for _, m := range compAcc.Company.Members {
		if requestorID != "" && m.AccountID == requestorID {
			return true
		}
	}
	return false
}

// callerIsCompanyAdmin decide se o chamador pode ALTERAR a empresa.
//
// SECURITY (achado C1): antes não havia checagem alguma — qualquer token
// reescrevia os dados cadastrais (CNPJ, e-mail de cobrança, endereço) de
// qualquer empresa. Exige proprietário ou perfil admin na própria empresa.
func (s *CompanyService) callerIsCompanyAdmin(requestorID string, compAcc *account.Account) bool {
	if compAcc == nil || compAcc.Company == nil || requestorID == "" {
		return false
	}
	if compAcc.Company.OwnerID == requestorID {
		return true
	}
	for _, m := range compAcc.Company.Members {
		if m.AccountID == requestorID && (m.Profile == "admin" || m.Profile == "owner") {
			return true
		}
	}
	return false
}

// GetCompany devolve a empresa somente para quem tem vínculo com ela.
func (s *CompanyService) GetCompany(ctx context.Context, requestorID, id string) (*account.Account, error) {
	acc, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if acc == nil || acc.Type != account.AccountTypeCompany {
		return nil, errors.New("company not found")
	}
	if !s.callerIsCompanyMember(ctx, requestorID, acc) {
		// "não encontrada" de propósito: não confirma a existência da empresa
		// para quem não tem vínculo.
		return nil, errors.New("company not found")
	}
	return acc, nil
}

// LookupCompanyByTaxID finds a company account by its CNPJ (tax_id). The input
// is normalized to digits-only so a masked CNPJ ("11.222.333/0001-81") and a
// raw one ("11222333000181") both resolve — the repository query also strips
// non-digits from the stored value, so matching is mask-insensitive on both
// sides. Returns ("company not found") when no company matches. Note: this
// does NOT relax CNPJ validation — callers (the handler) should still reject
// syntactically invalid CNPJs upstream if desired; here we only normalize for
// lookup.
func (s *CompanyService) LookupCompanyByTaxID(ctx context.Context, taxID string) (*account.Account, error) {
	normalized := normalizeTaxID(taxID)
	if normalized == "" {
		return nil, errors.New("tax_id is required")
	}
	acc, err := s.accountRepo.GetCompanyByTaxIDOrEmail(ctx, normalized, "")
	if err != nil {
		return nil, err
	}
	if acc == nil || acc.Type != account.AccountTypeCompany || acc.Company == nil {
		return nil, errors.New("company not found")
	}
	return acc, nil
}

func (s *CompanyService) CreateCompany(ctx context.Context, userID string, req *auth.CreateCompanyRequest) (*account.Account, error) {
	taxID := normalizeTaxID(req.TaxID)
	logger.Info("Attempting to create company", "user_id", userID, "company_name", req.Name, "tax_id", taxID)

	userAcc, err := s.accountRepo.GetByID(ctx, userID)
	if err != nil {
		logger.Error("Failed to get user", "error", err)
		return nil, err
	}
	if userAcc == nil || userAcc.Type != account.AccountTypeIndividual {
		logger.Warn("Invalid user type for company creation", "user_id", userID)
		return nil, errors.New("only individual accounts can create companies")
	}

	existingComp, err := s.accountRepo.GetCompanyByTaxIDOrEmail(ctx, taxID, "")
	if err != nil {
		logger.Error("Failed to check for duplicate company", "error", err)
		return nil, err
	}
	if existingComp != nil {
		existingTaxID := ""
		if existingComp.Company != nil {
			existingTaxID = existingComp.Company.TaxID
		}

		if normalizeTaxID(existingTaxID) == taxID && taxID != "" {
			logger.Warn("Duplicate company tax_id found", "tax_id", taxID)
			return nil, errors.New("company with this tax_id already exists")
		}
	}

	compAcc := &account.Account{
		ID:        uuid.New().String(),
		Type:      account.AccountTypeCompany,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Company: &company.Company{
			ID:        uuid.NewString(),
			Name:      req.Name,
			LegalName: req.LegalName,
			TaxID:     taxID,
			Email:     req.Email,
			Phone:     req.Phone,
			Address: company.Address{
				Street:     req.Address.Street,
				Number:     req.Address.Number,
				Complement: req.Address.Complement,
				District:   req.Address.District,
				City:       req.Address.City,
				State:      req.Address.State,
				ZipCode:    req.Address.ZipCode,
			},
			Type:     req.Type,
			IsActive: true,
			OwnerID:  userID,
			Members:  []company.CompanyMember{{AccountID: userID, Profile: "owner", FunctionalRole: "admin"}},
		},
	}

	if err := s.accountRepo.Create(ctx, compAcc); err != nil {
		logger.Error("Failed to persist company", "error", err)
		return nil, err
	}

	logger.Info("Company created successfully", "company_id", compAcc.ID, "user_id", userID)
	return compAcc, nil
}

// UpdateCompanyAccount altera os dados cadastrais da empresa.
//
// SECURITY (achado C1): exige que o chamador seja proprietário ou admin da
// empresa. Antes, o `companyID` vinha do path e o corpo era aplicado sem
// nenhuma verificação de vínculo ou autoridade.
func (s *CompanyService) UpdateCompanyAccount(ctx context.Context, requestorID, companyID string, req *company.Company) error {
	acc, err := s.accountRepo.GetByID(ctx, companyID)
	if err != nil {
		return err
	}
	if acc == nil || acc.Company == nil {
		return errors.New("company account not found")
	}
	if !s.callerIsCompanyAdmin(requestorID, acc) {
		return errors.New("unauthorized")
	}

	before := map[string]any{
		"name":  acc.Company.Name,
		"email": acc.Company.Email,
		"phone": acc.Company.Phone,
	}

	acc.Company.Name = req.Name
	acc.Company.LegalName = req.LegalName
	acc.Company.TaxID = normalizeTaxID(req.TaxID)
	acc.Company.Email = req.Email
	acc.Company.Phone = req.Phone
	acc.Company.Address = req.Address
	acc.UpdatedAt = time.Now()

	if err := s.accountRepo.Update(ctx, acc); err != nil {
		return err
	}

	// Achado M5 (P2): mudança de dados cadastrais da empresa é mutação de
	// configuração — auditada com antes/depois (sem PII: nome/telefone/e-mail da
	// própria empresa são dados de contato corporativo, não de titular).
	audit.Record(ctx, audit.Entry{
		Action:     "company.updated",
		EntityType: "company",
		EntityID:   companyID,
		Before:     before,
		After: map[string]any{
			"name":  acc.Company.Name,
			"email": acc.Company.Email,
			"phone": acc.Company.Phone,
		},
		Metadata: map[string]any{"actor_id": requestorID},
	})

	return nil
}

type CompanyMemberDetail struct {
	AccountID string `json:"account_id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Profile   string `json:"profile"`
	Role      string `json:"functional_role"`
	// IsActive — P2/M7: a lista de membros passa a distinguir vínculo suspenso.
	IsActive bool `json:"is_active"`
}

func (s *CompanyService) ListCompanyMembers(ctx context.Context, requestorID, companyID string) ([]CompanyMemberDetail, error) {
	compAcc, err := s.accountRepo.GetByID(ctx, companyID)
	if err != nil {
		return nil, err
	}
	if compAcc == nil || compAcc.Type != account.AccountTypeCompany {
		return nil, errors.New("company not found")
	}

	isMember := false
	tenantID, _ := pkgcontext.GetTenantID(ctx)
	if tenantID == companyID {
		isMember = true
	} else {
		for _, m := range compAcc.Company.Members {
			if m.AccountID == requestorID {
				isMember = true
				break
			}
		}
	}
	if !isMember {
		return nil, errors.New("unauthorized")
	}

	var details []CompanyMemberDetail
	for _, m := range compAcc.Company.Members {
		userAcc, err := s.memberAccount(ctx, m.AccountID)
		if err != nil || userAcc == nil || userAcc.Individual == nil {
			continue // Skip if corrupted
		}

		cred, err := s.memberCredential(ctx, m.AccountID)
		email := ""
		if err == nil && cred != nil {
			email = cred.Email
		}

		role := m.FunctionalRole
		if role == "" {
			role = "corretor"
		}

		details = append(details, CompanyMemberDetail{
			AccountID: m.AccountID,
			Email:     email,
			Name:      userAcc.Individual.FullName,
			AvatarURL: userAcc.Individual.AvatarURL,
			Profile:   m.Profile,
			Role:      role,
			IsActive:  m.Active(),
		})
	}
	return details, nil
}

func (s *CompanyService) ListCompanyMembersInternal(ctx context.Context, companyID string) ([]CompanyMemberDetail, error) {
	compAcc, err := s.accountRepo.GetByID(ctx, companyID)
	if err != nil {
		return nil, err
	}
	if compAcc == nil || compAcc.Type != account.AccountTypeCompany {
		return nil, errors.New("company not found")
	}

	var details []CompanyMemberDetail
	for _, m := range compAcc.Company.Members {
		userAcc, err := s.memberAccount(ctx, m.AccountID)
		if err != nil || userAcc == nil || userAcc.Individual == nil {
			continue
		}

		cred, err := s.memberCredential(ctx, m.AccountID)
		email := ""
		if err == nil && cred != nil {
			email = cred.Email
		}

		role := m.FunctionalRole
		if role == "" {
			role = "corretor"
		}

		details = append(details, CompanyMemberDetail{
			AccountID: m.AccountID,
			Email:     email,
			Name:      userAcc.Individual.FullName,
			AvatarURL: userAcc.Individual.AvatarURL,
			Profile:   m.Profile,
			Role:      role,
			IsActive:  m.Active(),
		})
	}
	return details, nil
}

func (s *CompanyService) RemoveMemberFromCompany(ctx context.Context, requestorID, companyID, memberID string) error {
	compAcc, err := s.accountRepo.GetByID(ctx, companyID)
	if err != nil {
		return err
	}
	if compAcc == nil || compAcc.Type != account.AccountTypeCompany {
		return errors.New("company not found")
	}

	isAdmin := false
	if compAcc.Company.OwnerID == requestorID {
		isAdmin = true
	} else {
		for _, m := range compAcc.Company.Members {
			if m.AccountID == requestorID && m.Profile == "admin" {
				isAdmin = true
				break
			}
		}
	}
	if !isAdmin {
		return errors.New("unauthorized")
	}

	if requestorID == memberID {
		return errors.New("cannot remove yourself")
	}

	var newMembers []company.CompanyMember
	found := false
	for _, m := range compAcc.Company.Members {
		if m.AccountID == memberID {
			found = true
			continue
		}
		newMembers = append(newMembers, m)
	}

	if !found {
		return errors.New("member not found in company")
	}

	compAcc.Company.Members = newMembers
	if err := s.accountRepo.Update(ctx, compAcc); err != nil {
		return err
	}

	// Remove member from all groups in the company
	if s.groupRepo != nil {
		if err := s.groupRepo.RemoveMemberFromAllGroups(ctx, companyID, memberID); err != nil {
			logger.Warn("Failed to remove member from groups", "companyID", companyID, "memberID", memberID, "error", err)
		}
	}

	// Achado M5 (P2): remoção de vínculo é evento de controle de acesso — sem
	// trilha não há como reconstituir quem tirou quem da empresa.
	audit.Record(ctx, audit.Entry{
		Action:     "company.member_removed",
		EntityType: "company_member",
		EntityID:   memberID,
		Metadata: map[string]any{
			"company_id": companyID,
			"actor_id":   requestorID,
		},
	})

	// P1/A15 — o access token do removido continua válido até expirar, então a
	// remoção precisa marcar o vínculo como revogado para que o middleware
	// (pkg/auth) recuse as próximas requisições dele nesta empresa. Falha aqui NÃO
	// desfaz a remoção (o vínculo já saiu da fonte de verdade): fica registrada e
	// o TTL da marca acompanha a vida do token.
	s.revokeMembership(ctx, companyID, memberID)

	return nil
}

// UpdateCompanyMember altera papel/perfil e ativa/desativa um vínculo existente
// (achados M6 e M7 da revisão de permissões de 2026-09-19).
//
// Antes deste endpoint não havia como mudar o papel de um membro já existente: o
// papel só mudava ao ADICIONAR o usuário (`AddUserToCompany.Role`), no convite
// ou como efeito colateral de grupo — e não havia como suspender alguém sem
// removê-lo da empresa (o `IsActive` do domínio nunca era lido).
//
// Regras (todas testadas):
//
//   - quem chama precisa ser dono ou admin da empresa (mesma autoridade de
//     `UpdateCompanyAccount`/`RemoveMemberFromCompany`);
//   - `functional_role` precisa EXISTIR no catálogo (pkg/auth) — o mesmo
//     conjunto que o middleware usa para autorizar; papel inventado deixaria o
//     membro com permissões vazias e a matriz do tenant inconsistente;
//   - `profile` segue a allowlist do C3: `admin` só pode ser concedido pelo dono;
//     `owner` não é atribuível (o dono é `Company.OwnerID`);
//   - ninguém altera o próprio vínculo (evita auto-elevação e auto-bloqueio);
//   - o vínculo do DONO não é alterável nem desativável (senão a empresa fica sem
//     administrador);
//   - desativar grava a marca de revogação do A15: o access token do suspenso
//     para de funcionar nesta empresa na requisição seguinte, e o login/
//     switch-context recusam emitir token novo (auth_service).
func (s *CompanyService) UpdateCompanyMember(ctx context.Context, requestorID, companyID, memberID string, req *auth.UpdateMemberRequest) (*CompanyMemberDetail, error) {
	if req == nil {
		req = &auth.UpdateMemberRequest{}
	}

	compAcc, err := s.accountRepo.GetByID(ctx, companyID)
	if err != nil {
		return nil, err
	}
	if compAcc == nil || compAcc.Type != account.AccountTypeCompany || compAcc.Company == nil {
		return nil, errors.New("company not found")
	}

	if !s.callerIsCompanyAdmin(requestorID, compAcc) {
		return nil, errors.New("unauthorized")
	}

	if memberID == requestorID {
		return nil, errors.New("cannot change your own membership")
	}
	if compAcc.Company.OwnerID == memberID {
		return nil, errors.New("cannot change the company owner")
	}

	index := -1
	for i, m := range compAcc.Company.Members {
		if m.AccountID == memberID {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, errors.New("member not found in company")
	}

	member := compAcc.Company.Members[index]

	// ── Papel funcional (catálogo) ────────────────────────────────────────────
	newRole := strings.ToLower(strings.TrimSpace(req.FunctionalRole))
	if newRole != "" {
		if !pkgAuth.IsAssignableRole(newRole) {
			return nil, fmt.Errorf("invalid functional role: %q", req.FunctionalRole)
		}
	}

	// ── Perfil de acesso na empresa ───────────────────────────────────────────
	newProfile := strings.ToLower(strings.TrimSpace(req.Profile))
	switch newProfile {
	case "":
		// não mexe
	case "member", "viewer":
		// perfis sem poder de administração: qualquer admin da empresa concede
	case "admin":
		if compAcc.Company.OwnerID != requestorID {
			return nil, errors.New("only the company owner can grant admin profile")
		}
	case "owner":
		// o dono é Company.OwnerID; promover a "owner" criaria dois donos e um
		// segundo bypass de root (achado C3).
		return nil, errors.New("owner profile cannot be assigned")
	default:
		return nil, fmt.Errorf("invalid profile: %q", req.Profile)
	}

	// ── Ativação (M7) ─────────────────────────────────────────────────────────
	deactivating := req.IsActive != nil && !*req.IsActive && member.Active()
	reactivating := req.IsActive != nil && *req.IsActive && !member.Active()

	beforeRole, beforeProfile, beforeActive := member.FunctionalRole, member.Profile, member.Active()

	if newRole != "" {
		compAcc.Company.Members[index].FunctionalRole = newRole
	}
	if newProfile != "" {
		compAcc.Company.Members[index].Profile = newProfile
	}
	if req.IsActive != nil {
		active := *req.IsActive
		compAcc.Company.Members[index].IsActive = &active
		if active {
			compAcc.Company.Members[index].DeactivatedAt = nil
		} else {
			now := time.Now()
			compAcc.Company.Members[index].DeactivatedAt = &now
		}
	}

	updated := compAcc.Company.Members[index]
	if beforeRole == updated.FunctionalRole && beforeProfile == updated.Profile && beforeActive == updated.Active() {
		// Nada mudou: não escreve, não audita ruído e não derruba o token de
		// quem não foi alterado.
		return s.memberDetail(ctx, companyID, updated), nil
	}

	if err := s.accountRepo.Update(ctx, compAcc); err != nil {
		return nil, err
	}

	if deactivating {
		audit.Record(ctx, audit.Entry{
			Action:     "company.member_deactivated",
			EntityType: "company_member",
			EntityID:   memberID,
			Before:     map[string]any{"is_active": true},
			After:      map[string]any{"is_active": false},
			Metadata:   map[string]any{"company_id": companyID, "actor_id": requestorID},
		})
		// Sem a marca o token vigente continuaria valendo até expirar; a marca de
		// SUSPENSÃO é durável (sem TTL) porque a decisão administrativa não
		// expira sozinha (P1/A15 + P2/M7).
		suspendMembershipMark(ctx, s.membershipRevoker, companyID, memberID)
		logger.Info("auth.membership_deactivated",
			"event", "auth.membership_deactivated",
			"tenant_id", companyID, "account_id", memberID)
	}

	if reactivating {
		// Reativação precisa LIMPAR a marca durável; sem isso o vínculo volta a
		// existir na fonte de verdade mas continua recusado para sempre.
		clearMembershipMark(ctx, s.revocationDeleter(), companyID, memberID)
		audit.Record(ctx, audit.Entry{
			Action:     "company.member_reactivated",
			EntityType: "company_member",
			EntityID:   memberID,
			Before:     map[string]any{"is_active": false},
			After:      map[string]any{"is_active": true},
			Metadata:   map[string]any{"company_id": companyID, "actor_id": requestorID},
		})
	}

	// Troca de papel/perfil: o token carrega `functional_role` e `permission`
	// (perfil) — os dois viram bypass de root ou autorização de catálogo no
	// middleware. Invalidar o vínculo é o que impede o portador de continuar com
	// o papel ANTIGO até o token expirar (mesma decisão de group_service).
	if beforeRole != updated.FunctionalRole || beforeProfile != updated.Profile {
		audit.Record(ctx, audit.Entry{
			Action:     "company.member_role_changed",
			EntityType: "company_member",
			EntityID:   memberID,
			Before:     map[string]any{"functional_role": beforeRole, "profile": beforeProfile},
			After:      map[string]any{"functional_role": updated.FunctionalRole, "profile": updated.Profile},
			Metadata:   map[string]any{"company_id": companyID, "actor_id": requestorID},
		})
		s.revokeMembership(ctx, companyID, memberID)
		logger.Info("auth.membership_revoked_on_role_change",
			"event", "auth.membership_revoked_on_role_change",
			"tenant_id", companyID, "account_id", memberID,
			"previous_role", beforeRole, "new_role", updated.FunctionalRole)
	}

	return s.memberDetail(ctx, companyID, updated), nil
}

// revocationDeleter devolve o limpador de marca injetado ou o Redis do ambiente.
func (s *CompanyService) revocationDeleter() pkgAuth.MembershipRevocationDeleter {
	if s.membershipRevoker == nil {
		return nil
	}
	if deleter, ok := s.membershipRevoker.(pkgAuth.MembershipRevocationDeleter); ok {
		return deleter
	}
	return nil
}

// memberDetail monta o detalhe público de um vínculo (nome/e-mail vêm da conta
// individual; ausência de conta não impede a resposta).
func (s *CompanyService) memberDetail(ctx context.Context, companyID string, member company.CompanyMember) *CompanyMemberDetail {
	detail := &CompanyMemberDetail{
		AccountID: member.AccountID,
		Profile:   member.Profile,
		Role:      member.FunctionalRole,
		IsActive:  member.Active(),
	}
	if acc, err := s.memberAccount(ctx, member.AccountID); err == nil && acc != nil && acc.Individual != nil {
		detail.Email = acc.Individual.Email
		detail.Name = acc.Individual.FullName
		detail.AvatarURL = acc.Individual.AvatarURL
	}
	if detail.Role == "" {
		detail.Role = "corretor"
	}
	_ = companyID
	return detail
}

// revokeMembership grava a marca de revogação do vínculo no Redis (quando
// configurado). Isolado para poder ser chamado de outros pontos que mudam o
// vínculo (ex.: troca de papel) sem repetir o tratamento de erro.
func (s *CompanyService) revokeMembership(ctx context.Context, tenantID, accountID string) {
	revokeMembershipMark(ctx, s.membershipRevoker, tenantID, accountID)
}

func (s *CompanyService) GetUserCompanies(ctx context.Context, userID string) ([]*account.Account, error) {
	return s.accountRepo.GetCompaniesByMemberID(ctx, userID)
}

func (s *CompanyService) AddUserToCompany(ctx context.Context, companyID string, userID string, req *auth.AddUserRequest) (*account.Account, error) {
	compAcc, err := s.accountRepo.GetByID(ctx, companyID)
	if err != nil {
		return nil, err
	}
	if compAcc == nil || compAcc.Type != account.AccountTypeCompany || compAcc.Company == nil {
		return nil, errors.New("company not found")
	}

	isAdmin := false
	if compAcc.Company.OwnerID == userID {
		isAdmin = true
	} else {
		for _, m := range compAcc.Company.Members {
			if m.AccountID == userID && (m.Profile == "admin" || m.Profile == "owner") {
				isAdmin = true
				break
			}
		}
	}
	if !isAdmin {
		return nil, errors.New("unauthorized")
	}

	// Ensure user exists
	userAcc, err := s.memberAccount(ctx, req.AccountID)
	if err != nil || userAcc == nil {
		return nil, errors.New("user not found")
	}

	for _, m := range compAcc.Company.Members {
		if m.AccountID == req.AccountID {
			return nil, errors.New("user is already a member")
		}
	}

	// SECURITY (achado C3): `profile` e `role` vinham crus do corpo e eram
	// gravados literalmente. `profile` é promovido a `app_metadata.permission`
	// no próximo switch-context, e `IsRootProfile` trata owner/admin como bypass
	// total do catálogo — ou seja, um admin de empresa fabricava novos
	// principais root (inclusive para conta de terceiro). Agora há allowlist e
	// exigência de autoridade para conceder admin/owner.
	profile := strings.ToLower(strings.TrimSpace(req.Profile))
	if profile == "" {
		profile = "member"
	}
	switch profile {
	case "member", "viewer":
		// perfis sem poder de administração: qualquer admin pode conceder
	case "admin", "owner":
		if compAcc.Company.OwnerID != userID {
			return nil, errors.New("only the company owner can grant admin or owner profile")
		}
	default:
		return nil, fmt.Errorf("invalid profile: %q", req.Profile)
	}

	functionalRole := strings.ToLower(strings.TrimSpace(req.Role))
	if functionalRole == "" {
		functionalRole = "corretor"
	}
	if !pkgAuth.IsAssignableRole(functionalRole) {
		// Mesma validação já aplicada no convite (invitation_service.go) — o
		// cadastro direto era o caminho inconsistente.
		return nil, fmt.Errorf("invalid functional role: %q", req.Role)
	}

	compAcc.Company.Members = append(compAcc.Company.Members, company.CompanyMember{
		AccountID:      req.AccountID,
		Profile:        profile,
		FunctionalRole: functionalRole,
	})

	if err := s.accountRepo.Update(ctx, compAcc); err != nil {
		return nil, err
	}

	// Add to default group
	if s.groupService != nil {
		if err := s.groupService.EnsureDefaultGroupForMember(ctx, companyID, req.AccountID); err != nil {
			logger.Warn("Failed to ensure default group for member", "companyID", companyID, "memberID", req.AccountID, "error", err)
		}
	}

	// Achado M5: mudanças de vínculo/papel não eram auditadas — não havia como
	// reconstituir quem elevou quem.
	audit.Record(ctx, audit.Entry{
		Action:     "company.member_added",
		EntityType: "company_member",
		EntityID:   req.AccountID,
		Metadata: map[string]any{
			"company_id":      companyID,
			"profile":         profile,
			"functional_role": functionalRole,
			"actor_id":        userID,
		},
	})

	return userAcc, nil
}
