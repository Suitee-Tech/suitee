package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca-auth/internal/domain/group"
	"github.com/oginoino/toca/pkg/audit"
	pkgAuth "github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/logger"
)

type GroupService struct {
	groupRepo   group.Repository
	accountRepo account.Repository
	// accountServiceRepo é a superfície de serviço (BYPASSRLS) usada para ler a
	// conta de OUTROS membros da empresa. O pool de aplicação (toca_app) é
	// RLS-restrito a `id = auth.uid()` em `toca_auth.accounts`, então ler um
	// colega pelo pool de aplicação devolve zero linhas — era por isso que o
	// nome/avatar dos demais membros sumia na tela de Grupos.
	accountServiceRepo account.ServiceRepository
	// membershipRevoker grava a marca de revogação quando o PAPEL do membro muda
	// (P1/A15). Nil cai para o Redis do ambiente.
	membershipRevoker pkgAuth.MembershipRevoker
}

func NewGroupService(groupRepo group.Repository, accountRepo account.Repository) *GroupService {
	return &GroupService{
		groupRepo:   groupRepo,
		accountRepo: accountRepo,
	}
}

// WithMembershipRevoker injeta o gravador de revogação (Redis ou dublê).
func (s *GroupService) WithMembershipRevoker(revoker pkgAuth.MembershipRevoker) *GroupService {
	s.membershipRevoker = revoker
	return s
}

// WithAccountServiceRepo injeta a superfície de serviço para ler o detalhe de
// outros membros. Sem ela o serviço cai no pool de aplicação (testes).
func (s *GroupService) WithAccountServiceRepo(repo account.ServiceRepository) *GroupService {
	s.accountServiceRepo = repo
	return s
}

// memberAccount lê a conta de um membro da empresa pela pool de serviço (RLS
// não permite ao pool de aplicação ler a conta de terceiros).
func (s *GroupService) memberAccount(ctx context.Context, accountID string) (*account.Account, error) {
	if s.accountServiceRepo != nil {
		return s.accountServiceRepo.GetByIDService(ctx, accountID)
	}
	return s.accountRepo.GetByID(ctx, accountID)
}

// defaultGroupFunctionalRole é o papel aplicado quando o cliente cria um grupo
// sem declarar o papel — o mesmo default de AddUserToCompany, o menos
// privilegiado dos papéis de operador (P2/M6).
const defaultGroupFunctionalRole = "corretor"

var defaultRoles = []struct {
	RoleID string
	Name   string
	Desc   string
}{
	{"admin", "Administrador", "Acesso total ao sistema"},
	{"gestor", "Gestor Comercial", "Gestão de equipe e relatórios"},
	{"corretor", "Corretor", "Acesso padrão para corretores"},
	{"operacoes", "Operações & Compliance", "Gestão de contratos e repasses"},
	{"financeiro", "Financeiro", "Gestão financeira e notas fiscais"},
	{"marketing", "Marketing", "Gestão de campanhas e leads"},
	{"auditor", "Auditor / Leitor", "Acesso somente leitura"},
	{"parceiro", "Parceiro / Stakeholder", "Acesso restrito para parceiros"},
	{"cliente", "Cliente Final", "Acesso para clientes da imobiliária"},
}

func (s *GroupService) EnsureDefaultGroups(ctx context.Context, companyID string) error {
	groups, err := s.groupRepo.ListByCompanyID(ctx, companyID)
	if err != nil {
		return err
	}

	existingRoles := make(map[string]bool)
	for _, g := range groups {
		if g.RoleID != "" {
			existingRoles[g.RoleID] = true
		}
	}

	for _, dr := range defaultRoles {
		if !existingRoles[dr.RoleID] {
			g := &group.Group{
				ID:          uuid.New().String(),
				CompanyID:   companyID,
				Name:        dr.Name,
				Description: dr.Desc,
				RoleID:      dr.RoleID,
				Members:     []group.GroupMember{},
				IsDefault:   true,
				CreatedBy:   "system",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			if err := s.groupRepo.Create(ctx, g); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *GroupService) syncCompanyMemberRole(ctx context.Context, companyID, accountID, newRoleID string) error {
	// P2/M6: o papel gravado no vínculo precisa EXISTIR — no catálogo OU como
	// PERFIL DE ACESSO do tenant (`cr_…`, criado em Configurações → Permissões).
	//
	// Sem esta guarda, um grupo legado (`RoleID = custom_<uuid>`, identificador
	// que não existe em catálogo nenhum) escrevia no membro um papel que a matriz
	// do tenant não conhece — o usuário ficava sem permissão nenhuma e fora de
	// todos os grupos reais. Fail-closed: recusa e explica.
	//
	// Aceitar o perfil aqui é o que UNIFICA as duas telas: o grupo de usuários é
	// quem atribui o perfil, e o que o membro recebe é o id do perfil — decidido
	// por `settings.RolePermissionsFor` no enforcement. Um id de perfil de OUTRA
	// imobiliária não casa com perfil nenhum da matriz deste tenant e, portanto,
	// não concede nada (o perfil é escopado por tenant).
	if !pkgAuth.IsAssignableRole(newRoleID) {
		return fmt.Errorf("group has no catalog functional role or access profile (%q): configure o papel do grupo antes de usá-lo", newRoleID)
	}

	compAcc, err := s.accountRepo.GetByID(ctx, companyID)
	if err != nil || compAcc == nil {
		return errors.New("company not found")
	}

	found := false
	previousRole := ""
	for i, m := range compAcc.Company.Members {
		if m.AccountID == accountID {
			previousRole = m.FunctionalRole
			compAcc.Company.Members[i].FunctionalRole = newRoleID
			found = true
			break
		}
	}

	if !found {
		return errors.New("member not found in company")
	}

	if err := s.accountRepo.Update(ctx, compAcc); err != nil {
		return err
	}

	// P1/A15 — o token carrega `functional_role`: mudar o papel de um membro sem
	// invalidar o token deixaria o papel ANTIGO valendo até expirar. A marca só é
	// gravada quando houve MUDANÇA de papel (e não na primeira atribuição, feita
	// no convite/entrada na empresa — marcar ali derrubaria o recém-chegado).
	if previousRole != "" && previousRole != newRoleID {
		// Achado M5 (P2): mudança de PAPEL é o evento de elevação de privilégio
		// por excelência — auditado com antes/depois para responder "quem elevou
		// quem, de qual papel para qual".
		audit.Record(ctx, audit.Entry{
			Action:     "company.member_role_changed",
			EntityType: "company_member",
			EntityID:   accountID,
			Before:     map[string]any{"functional_role": previousRole},
			After:      map[string]any{"functional_role": newRoleID},
			Metadata:   map[string]any{"company_id": companyID},
		})

		revokeMembershipMark(ctx, s.membershipRevoker, companyID, accountID)
		logger.Info("auth.membership_revoked_on_role_change",
			"event", "auth.membership_revoked_on_role_change",
			"tenant_id", companyID, "account_id", accountID,
			"previous_role", previousRole, "new_role", newRoleID)
	}

	return nil
}

func (s *GroupService) checkCompanyAdmin(ctx context.Context, requestorID, companyID string) error {
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
	return nil
}

func (s *GroupService) checkCompanyMember(ctx context.Context, requestorID, companyID string) error {
	compAcc, err := s.accountRepo.GetByID(ctx, companyID)
	if err != nil {
		return err
	}
	if compAcc == nil || compAcc.Type != account.AccountTypeCompany {
		return errors.New("company not found")
	}

	isMember := false
	for _, m := range compAcc.Company.Members {
		if m.AccountID == requestorID {
			isMember = true
			break
		}
	}
	if !isMember {
		return errors.New("unauthorized")
	}
	return nil
}

func (s *GroupService) CreateGroup(ctx context.Context, requestorID, companyID string, req *group.CreateGroupRequest) (*group.Group, error) {
	if err := s.checkCompanyAdmin(ctx, requestorID, companyID); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// O papel do grupo é o que será gravado como `functional_role` dos membros:
	// um papel do CATÁLOGO (mesma fonte do middleware) ou um PERFIL DE ACESSO do
	// tenant (`cr_…`, criado na tela de Permissões e Personalizados). O
	// identificador sintético `custom_<uuid>` continua não existindo: ele nunca
	// casava com a matriz do tenant.
	roleID := strings.ToLower(strings.TrimSpace(req.FunctionalRole))
	if roleID == "" {
		roleID = defaultGroupFunctionalRole
	}
	if !pkgAuth.IsAssignableRole(roleID) {
		return nil, fmt.Errorf("invalid functional role: %q", req.FunctionalRole)
	}

	newID := uuid.New().String()
	newGroup := &group.Group{
		ID:          newID,
		CompanyID:   companyID,
		Name:        req.Name,
		Description: req.Description,
		RoleID:      roleID,
		Members:     []group.GroupMember{},
		CreatedBy:   requestorID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.groupRepo.Create(ctx, newGroup); err != nil {
		logger.Error("Failed to create group", "error", err)
		return nil, err
	}

	// Achado M5 (P2): grupo define papel — criação é mutação de controle de acesso.
	audit.Record(ctx, audit.Entry{
		Action:     "company.group_created",
		EntityType: "company_group",
		EntityID:   newGroup.ID,
		After:      map[string]any{"name": newGroup.Name, "role_id": newGroup.RoleID},
		Metadata:   map[string]any{"company_id": companyID, "actor_id": requestorID},
	})

	return newGroup, nil
}

func (s *GroupService) UpdateGroup(ctx context.Context, requestorID, companyID, groupID string, req *group.UpdateGroupRequest) (*group.Group, error) {
	if err := s.checkCompanyAdmin(ctx, requestorID, companyID); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	g, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if g == nil || g.CompanyID != companyID {
		return nil, errors.New("group not found")
	}

	isDefault := g.IsSystemDefault()

	if !isDefault {
		g.Name = req.Name
	}
	g.Description = req.Description

	// P2/M6: reparo de grupo legado — um grupo com RoleID inválido
	// (`custom_<uuid>`) pode ser apontado para um papel do catálogo OU para um
	// perfil de acesso do tenant. Grupos default não trocam de papel por aqui (o
	// papel deles é a identidade do grupo).
	if newRole := strings.ToLower(strings.TrimSpace(req.FunctionalRole)); newRole != "" {
		if isDefault && newRole != g.RoleID {
			return nil, errors.New("cannot change the functional role of a default group")
		}
		if !pkgAuth.IsAssignableRole(newRole) {
			return nil, fmt.Errorf("invalid functional role: %q", req.FunctionalRole)
		}
		g.RoleID = newRole
	}

	g.UpdatedAt = time.Now()

	if err := s.groupRepo.Update(ctx, g); err != nil {
		return nil, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "company.group_updated",
		EntityType: "company_group",
		EntityID:   g.ID,
		After:      map[string]any{"name": g.Name, "description": g.Description, "role_id": g.RoleID},
		Metadata:   map[string]any{"company_id": companyID, "actor_id": requestorID},
	})

	return g, nil
}

func (s *GroupService) DeleteGroup(ctx context.Context, requestorID, companyID, groupID string) error {
	if err := s.checkCompanyAdmin(ctx, requestorID, companyID); err != nil {
		return err
	}

	g, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return err
	}
	if g == nil || g.CompanyID != companyID {
		return errors.New("group not found")
	}

	// Grupos de papel criados pelo sistema não são apagáveis (P2/M6: a
	// identificação é explícita — `RoleID` deixou de distinguir default de
	// custom, porque ambos apontam para papéis do catálogo).
	if g.IsSystemDefault() {
		return errors.New("cannot delete a default profile group")
	}

	// Reassign members to "corretor" role
	compAcc, err := s.accountRepo.GetByID(ctx, companyID)
	if err == nil && compAcc != nil {
		changed := false
		for i, m := range compAcc.Company.Members {
			if m.FunctionalRole == g.RoleID {
				compAcc.Company.Members[i].FunctionalRole = "corretor"
				changed = true
			}
		}
		if changed {
			_ = s.accountRepo.Update(ctx, compAcc)
		}
	}

	if err := s.groupRepo.Delete(ctx, companyID, groupID); err != nil {
		return err
	}

	// Achado M5 (P2): apagar grupo rebaixa os membros dele para "corretor" —
	// mutação de papel em massa, precisa de trilha.
	audit.Record(ctx, audit.Entry{
		Action:     "company.group_deleted",
		EntityType: "company_group",
		EntityID:   groupID,
		Before:     map[string]any{"name": g.Name, "role_id": g.RoleID},
		Metadata:   map[string]any{"company_id": companyID, "actor_id": requestorID},
	})

	return nil
}

func (s *GroupService) ListGroups(ctx context.Context, requestorID, companyID string) ([]*group.Group, error) {
	if err := s.checkCompanyMember(ctx, requestorID, companyID); err != nil {
		return nil, err
	}

	_ = s.EnsureDefaultGroups(ctx, companyID)

	groups, err := s.groupRepo.ListByCompanyID(ctx, companyID)
	if err != nil {
		return nil, err
	}

	compAcc, err := s.accountRepo.GetByID(ctx, companyID)
	if err == nil && compAcc != nil {
		roleMembers := make(map[string][]string)
		for _, m := range compAcc.Company.Members {
			role := m.FunctionalRole
			if role == "" {
				role = "corretor"
			}
			roleMembers[role] = append(roleMembers[role], m.AccountID)
		}

		// Fetch user details efficiently
		userDetails := make(map[string]struct {
			Name      string
			AvatarURL string
		})

		for _, m := range compAcc.Company.Members {
			if acc, err := s.memberAccount(ctx, m.AccountID); err == nil && acc != nil && acc.Individual != nil {
				userDetails[m.AccountID] = struct {
					Name      string
					AvatarURL string
				}{
					Name:      acc.Individual.FullName,
					AvatarURL: acc.Individual.AvatarURL,
				}
			}
		}

		for _, g := range groups {
			g.Members = []group.GroupMember{}
			for _, accID := range roleMembers[g.RoleID] {
				g.Members = append(g.Members, group.GroupMember{
					AccountID: accID,
					AddedAt:   time.Now(),
					Name:      userDetails[accID].Name,
					AvatarURL: userDetails[accID].AvatarURL,
				})
			}
		}
	}

	return groups, nil
}

type GroupMemberDetail struct {
	AccountID string    `json:"account_id"`
	AddedAt   time.Time `json:"added_at"`
	AddedBy   string    `json:"added_by"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	AvatarURL string    `json:"avatar_url"`
}

type GroupDetail struct {
	ID          string              `json:"id"`
	CompanyID   string              `json:"company_id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Members     []GroupMemberDetail `json:"members"`
	CreatedBy   string              `json:"created_by"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
	// IsDefault — a UI precisa saber para não oferecer "excluir" num grupo de
	// papel do sistema (P2/M6).
	IsDefault bool   `json:"is_default"`
	RoleID    string `json:"role_id"`
}

func (s *GroupService) GetGroup(ctx context.Context, requestorID, companyID, groupID string) (*GroupDetail, error) {
	if err := s.checkCompanyMember(ctx, requestorID, companyID); err != nil {
		return nil, err
	}

	g, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if g == nil || g.CompanyID != companyID {
		return nil, errors.New("group not found")
	}

	detail := &GroupDetail{
		ID:          g.ID,
		CompanyID:   g.CompanyID,
		Name:        g.Name,
		Description: g.Description,
		CreatedBy:   g.CreatedBy,
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
		IsDefault:   g.IsSystemDefault(),
		RoleID:      g.RoleID,
		Members:     []GroupMemberDetail{},
	}

	compAcc, err := s.accountRepo.GetByID(ctx, companyID)
	if err == nil && compAcc != nil {
		for _, m := range compAcc.Company.Members {
			role := m.FunctionalRole
			if role == "" {
				role = "corretor"
			}
			if role == g.RoleID {
				userAcc, err := s.memberAccount(ctx, m.AccountID)
				if err != nil || userAcc == nil || userAcc.Individual == nil {
					continue
				}
				detail.Members = append(detail.Members, GroupMemberDetail{
					AccountID: m.AccountID,
					AddedAt:   time.Now(),
					AddedBy:   "system",
					Name:      userAcc.Individual.FullName,
					Email:     userAcc.Individual.Email,
					AvatarURL: userAcc.Individual.AvatarURL,
				})
			}
		}
	}

	return detail, nil
}

// AddMemberToGroup coloca o membro no grupo — o efeito real é a troca de papel,
// auditada em `syncCompanyMemberRole` como `company.member_role_changed`.
func (s *GroupService) AddMemberToGroup(ctx context.Context, requestorID, companyID, groupID, memberAccountID string) error {
	if err := s.checkCompanyAdmin(ctx, requestorID, companyID); err != nil {
		return err
	}

	if err := s.checkCompanyMember(ctx, memberAccountID, companyID); err != nil {
		return errors.New("user is not a member of the company")
	}

	g, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return err
	}
	if g == nil || g.CompanyID != companyID {
		return errors.New("group not found")
	}

	return s.syncCompanyMemberRole(ctx, companyID, memberAccountID, g.RoleID)
}

// RemoveMemberFromGroup rebaixa o membro para o papel padrão — a mudança de
// papel é auditada em `syncCompanyMemberRole` (P2/M5).
func (s *GroupService) RemoveMemberFromGroup(ctx context.Context, requestorID, companyID, groupID, memberAccountID string) error {
	// Removing a member from a group just resets their role to the default "corretor"
	if err := s.checkCompanyAdmin(ctx, requestorID, companyID); err != nil {
		return err
	}

	return s.syncCompanyMemberRole(ctx, companyID, memberAccountID, "corretor")
}

func (s *GroupService) MoveMemberBetweenGroups(ctx context.Context, requestorID, companyID, sourceGroupID, targetGroupID, memberAccountID string) error {
	// This is identical to AddMemberToGroup since members can only have one RoleID
	return s.AddMemberToGroup(ctx, requestorID, companyID, targetGroupID, memberAccountID)
}

func (s *GroupService) EnsureDefaultGroupForMember(ctx context.Context, companyID, memberAccountID string) error {
	_ = s.EnsureDefaultGroups(ctx, companyID)

	compAcc, err := s.accountRepo.GetByID(ctx, companyID)
	if err != nil || compAcc == nil {
		return errors.New("company not found")
	}

	found := false
	changed := false
	for i, m := range compAcc.Company.Members {
		if m.AccountID == memberAccountID {
			found = true
			if m.FunctionalRole == "" {
				compAcc.Company.Members[i].FunctionalRole = "corretor"
				changed = true
			}
			break
		}
	}

	if !found {
		return errors.New("member not found in company")
	}

	if changed {
		return s.accountRepo.Update(ctx, compAcc)
	}

	return nil
}
