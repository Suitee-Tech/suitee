package lead

import (
	"context"
	"time"
)

// LeadExpectedVersion is the optional optimistic-locking token sent by clients
// via the If-Match header. UpdatedSeq is preferred when present (Wave 1.6
// onwards) and is sub-nanosecond safe. UpdatedAt is accepted for backward
// compatibility with clients written before the seq column existed.
type LeadExpectedVersion struct {
	UpdatedAt  *time.Time
	UpdatedSeq *int64
}

// HasCheck reports whether the caller wants the repository to enforce a
// concurrency check. Nil or all-zero fields mean "no check".
func (v *LeadExpectedVersion) HasCheck() bool {
	return v != nil && (v.UpdatedAt != nil || v.UpdatedSeq != nil)
}

type LeadFilter struct {
	Limit  int64
	Offset int64

	Search string

	Status   string
	StatusIn []string

	DisplayID  string
	Email      string
	Phone      string
	AccountID  string
	ContractID string

	Source   string
	SourceIn []string

	Tag       string
	Qualified *bool

	Role     string
	IsClient *bool

	AssignedBrokerID string
	Unassigned       *bool
	BrokerScopeID    string

	AssignedFrom *time.Time
	AssignedTo   *time.Time

	ExpiresBefore *time.Time
	ExpiresAfter  *time.Time

	EscalationLevelMin *int
	RouletteRetriesMin *int
	TierID             string

	CreatedFrom *time.Time
	CreatedTo   *time.Time
	UpdatedFrom *time.Time
	UpdatedTo   *time.Time

	LastInteractionFrom *time.Time
	LastInteractionTo   *time.Time
	StaleDays           *int

	PropertyID         string
	CaptacaoPropertyID string
	OwnerLeadID        string
	IsCaptacao         *bool

	InterestType         string
	InterestLocation     string
	InterestMinPrice     *float64
	InterestMaxPrice     *float64
	InterestBedroomsMin  *int
	InterestBathroomsMin *int
	InterestParkingMin   *int
	InterestAreaMin      *float64
	InterestAreaMax      *float64

	NoteSearch string
}

type Repository interface {
	Create(ctx context.Context, l *Lead) error
	// GetByID resolve o lead **dentro do tenant informado**.
	//
	// SECURITY: o tenant é parâmetro obrigatório de propósito. Antes ele era
	// omitido e a query filtrava só por `id`, o que permitia a um usuário de um
	// tenant ler o lead de outro conhecendo o UUID (achado C9 da revisão de
	// permissões de 2026-09-19). Não reintroduza um caminho sem tenant: fluxos
	// legitimamente cross-tenant (workers/eventos) devem passar o tenant do
	// payload — nunca "".
	GetByID(ctx context.Context, tenantID, id string) (*Lead, error)
	// GetByIDForTenantService resolve o lead de um tenant EXPLÍCITO pelo pool
	// de SERVIÇO (BYPASSRLS).
	//
	// SECURITY: caminho das rotas INTERNAS (chave de serviço), onde não há
	// claims de JWT para a política de RLS — o pool de aplicação devolveria
	// "não encontrado" para uma linha que existe. O escopo continua provado
	// pelo `tenant_id` explícito no WHERE, resolvido da credencial
	// (auth.ResolveTenantForInternalRequest), nunca do query string cru.
	GetByIDForTenantService(ctx context.Context, tenantID, id string) (*Lead, error)
	// GetByIDAnyTenant resolve o lead **sem** filtro de tenant.
	//
	// SECURITY: use apenas quando a autorização for provada por outro meio,
	// imediatamente após a leitura. Casos legítimos hoje:
	//
	//   - portal do cliente (visitLead com client=true): valida a identidade do
	//     titular (account id ou e-mail) logo depois, e o tenant do token de um
	//     cliente OTP é a própria conta local dele — pode divergir do tenant da
	//     empresa dona do lead;
	//
	// Nunca exponha este caminho a uma rota de operador, nem o use como atalho
	// quando o tenant do chamador está disponível.
	GetByIDAnyTenant(ctx context.Context, id string) (*Lead, error)
	GetByEmail(ctx context.Context, tenantID, email string) (*Lead, error)
	GetByPhone(ctx context.Context, tenantID, phone string) (*Lead, error)
	GetByDisplayID(ctx context.Context, tenantID, displayID string) (*Lead, error)
	ListByAccountID(ctx context.Context, tenantID, accountID string, limit int64) ([]*Lead, error)
	FindByAccountID(ctx context.Context, accountID string, limit int64) ([]*Lead, error)
	// FindByEmailAnyTenant resolve leads pelo e-mail SEM filtro de tenant.
	//
	// SECURITY (achado A19): use apenas quando a autorização vier da posse do
	// e-mail (portal do cliente, cujo token traz o e-mail verificado e cujo
	// tenant é a própria conta local — pode divergir do tenant da empresa).
	// Nunca exponha este caminho a uma rota de operador.
	FindByEmailAnyTenant(ctx context.Context, email string, limit int64) ([]*Lead, error)
	// Update grava o agregado no tenant do próprio lead (`l.TenantID`).
	// SECURITY: a query inclui `AND tenant_id = $` — um agregado carregado
	// fora do tenant não pode ser gravado por engano (defesa em profundidade
	// do achado C9).
	Update(ctx context.Context, l *Lead) error
	Patch(ctx context.Context, leadID, tenantID string, expected *LeadExpectedVersion, updates map[string]interface{}) (*Lead, bool, error)
	AssignBrokerIfUnassigned(ctx context.Context, leadID, tenantID, brokerID, details string) (bool, error)
	// Delete remove (soft delete) o lead **dentro do tenant informado**.
	// SECURITY: mesmo contrato de GetByID — ver comentário acima.
	Delete(ctx context.Context, tenantID, id string) error
	List(ctx context.Context, tenantID string, filter LeadFilter) ([]*Lead, int64, error)
	ListUnassigned(ctx context.Context, tenantID string, limit int64) ([]*Lead, error)
	CountUnassigned(ctx context.Context, tenantID string) (int64, error)
	GetStats(ctx context.Context, tenantID string, brokerID string) (int64, map[string]int64, float64, error)
	GetActiveLeadCountsByBroker(ctx context.Context, tenantID string, brokerIDs []string) (map[string]int, error)
	// ListForSLARedirect returns leads in the given statuses whose AssignedAt
	// is older than cutoff and whose RouletteRetries is still under maxRetries.
	// The caller (SLA service) processes one tier at a time, mutating the
	// lead status as it goes — so two tiers with overlapping statuses don't
	// double-process a single lead.
	//
	// A `tierID` parameter used to live here but was never actually filtered
	// in the WHERE clause; the SLA loop already provides the tier-specific
	// cutoff and maxRetries, which is all the query needs. Removed in
	// Wave P0.2 to stop suggesting a filter that didn't exist.
	ListForSLARedirect(ctx context.Context, tenantID string, statuses []string, maxRetries int, cutoff time.Time, limit int64) ([]*Lead, error)
	ReassignBrokerIfCurrent(ctx context.Context, leadID, tenantID, expectedBrokerID, newBrokerID, tierID, details string) (bool, error)
	// ListFirstContactSLABreaches retorna leads que cruzaram o SLA de
	// primeiro contato: atribuídos antes de `cutoff`, sem
	// FirstContactAt, sem FirstContactSLAViolatedAt já setado
	// (notificação ainda não disparada — idempotência por linha).
	// Status terminal (CONVERTED, LOST) excluído. Issue #427.
	ListFirstContactSLABreaches(ctx context.Context, tenantID string, cutoff time.Time, limit int64) ([]*Lead, error)

	// ListSharedForPartner lists every lead owned by ownerTenantID that has
	// opted into the partner shared-content feature (PartnershipShared=true).
	//
	// SECURITY: this query intentionally bypasses the usual tenant scoping
	// (it filters by ownerTenantID, NOT by the JWT's tenant). It must only be
	// reached through a permission-checked partnership service method, never
	// from a generic tenant-scoped route.
	ListSharedForPartner(ctx context.Context, ownerTenantID string) ([]*Lead, error)
}

type RouletteRepository interface {
	GetConfig(ctx context.Context, tenantID string) (*RouletteConfig, error)
	UpdateConfig(ctx context.Context, cfg *RouletteConfig) error
	SetIndexCAS(ctx context.Context, tenantID, stage string, expected, newIdx int) (bool, error)
	ListEnabledTenantIDs(ctx context.Context) ([]string, error)
}

type RouletteHistoryRepository interface {
	Create(ctx context.Context, h *RouletteHistory) error
	List(ctx context.Context, tenantID string, limit, offset int64) ([]*RouletteHistory, int64, error)
}
