package services

import (
	"context"
	"errors"
	"fmt"
	access "github.com/oginoino/toca/pkg/partnershipaccess"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/pkg/virusscan"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/domain/document"
	"github.com/oginoino/toca/toca-deal/internal/domain/portal"
	repository "github.com/oginoino/toca/toca-deal/internal/repositories"
)

type LeadRepository interface {
	GetInterest(ctx context.Context, tenantID, id string) (interface{}, error)
	// Exists returns true when a lead with the given id exists in the tenant
	// and is not soft-deleted. Used by Create to prevent cross-tenant leak via
	// forged lead IDs.
	Exists(ctx context.Context, tenantID, id string) (bool, error)
	// ClientLeadTenants devolve as imobiliárias com que o cliente tem
	// ATENDIMENTO (lead) vinculado à conta dele.
	//
	// É a outra metade do vínculo cliente ↔ imobiliária: o portal não conhece a
	// imobiliária só por negócio — o corretor abre a conversa no lead, antes de
	// existir negócio. Sem esta fonte, `ListMyChannels` não tinha tenant nenhum
	// para consultar e o portal mostrava "Nenhuma conversa disponível" com a
	// mensagem do corretor já enviada (incidente 2026-09-21).
	ClientLeadTenants(ctx context.Context, accountID string) ([]string, error)
}

type PropertyRepository interface {
	GetSummaryByID(ctx context.Context, tenantID, id string, dealType deal.Type) (*portal.PropertySummary, error)
	ListRecommendations(ctx context.Context, tenantID string, dealType deal.Type, excludePropertyID string, interest interface{}, limit int64) ([]*portal.PropertySummary, error)
	Search(ctx context.Context, tenantID string, dealType deal.Type, params repository.SearchParams) ([]*portal.PropertySummary, error)
}

// PartyLeadHydrator fetches lead profile fields (name/email/phone/document/account_id)
// for parties that were created with only LeadID — used to backfill Party data
// captured at deal creation, when handlers don't always pass the owner's full
// profile. Production wires *clients.CrmClient; tests can inject a fake.
type PartyLeadHydrator interface {
	GetLead(ctx context.Context, tenantID, leadID string) (*clients.LeadDTO, error)
}

// DealSignerSyncer lets MoveDealStatus auto-heal when the deal.Signers
// projection is stale relative to the contract. ContractService implements
// this; wiring is optional so tests don't need to mock it.
type DealSignerSyncer interface {
	SyncDealSignersFromActiveContract(ctx context.Context, tenantID, dealID string) error
}

// Erros tipados retornados por UpdateStatus quando o deal não satisfaz
// as pré-condições de transição (#412, #416). Handlers HTTP usam
// errors.Is para mapear cada um para HTTP status corretos (409 Conflict,
// 422 Unprocessable, 403 Forbidden):
//
//   - ErrProposalNotApproved: tentar avançar para ANALYSIS/CONTRACT antes
//     de qualquer proposal ser aceita por todas as partes requeridas
//   - ErrPartiesPending: parties obrigatórias sem dados completos
//     (email/document vazio) — assinatura Clicksign falha downstream
//   - ErrSpouseMissing: principal casado sem cônjuge cadastrado
//     (e sem flag NoSpouseDeclared)
//   - ErrApprovalRequired: deal de alto valor exige aprovação por perfil
//     com permission `deals:approve` (admin/gestor/operações);
//     corretor não pode mover sozinho — protege contra fechamento
//     fraudulento ou erro de digitação sem revisão
//
// Mensagens em inglês para uso interno; frontend faz i18n.
var (
	ErrProposalNotApproved = errors.New("proposal must be accepted by all required parties before advancing")
	ErrPartiesPending      = errors.New("required parties have incomplete data (name, email or document missing)")
	ErrSpouseMissing       = errors.New("spouse required before advancing")
	ErrApprovalRequired    = errors.New("deal value exceeds approval threshold — requires deals:approve permission")

	// ErrChecklistNotApproved is returned when the tenant requires checklist
	// approval before moving to CONTRACT and required items are not APPROVED.
	ErrChecklistNotApproved = errors.New("required checklist items must be approved before moving to CONTRACT (tenant gate enabled)")

	// ErrSignersNotComplete is returned when attempting to CONVERT a deal
	// that still has unsigned required signers.
	ErrSignersNotComplete = errors.New("all required signers must sign before completing")

	// ErrCrossTenantResourceDenied is returned by Create/CreateWithParties
	// when a deal references a property or lead owned by ANOTHER tenant and
	// the co-brokering pre-conditions are not met: no active partnership, the
	// partnership does not grant the relevant shared scope, or its
	// SharingConfig.AllowCoBrokering is false. The handler maps it to 403.
	ErrCrossTenantResourceDenied = errors.New("cross-tenant resource not operable: no active co-brokering partnership grants access")

	// ErrSharedContentOperateDenied is returned when the cross-tenant branch
	// is taken but the caller lacks the `shared_content:operate` permission.
	// The handler maps it to 403.
	ErrSharedContentOperateDenied = errors.New("permission denied: shared_content:operate required to start a deal with a partner-shared resource")
)

// approvalThresholdBRL is retained as the explicit availability fallback for
// toca-auth failures and for legacy tests. Runtime tenant decisions resolve a
// versioned policy from toca_auth.tenant_settings through AuthClient first.
var approvalThresholdBRL = readApprovalThreshold()

const defaultApprovalThresholdBRL = 500_000.0

func readApprovalThreshold() float64 {
	raw := os.Getenv("TOCA_DEAL_APPROVAL_THRESHOLD_BRL")
	if raw == "" {
		return defaultApprovalThresholdBRL
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 {
		return defaultApprovalThresholdBRL
	}
	return v
}

type DealService struct {
	repo deal.Repository
	// portalRepo executa as operações do portal do cliente no pool de serviço
	// (cross-tenant por definição). Sem wiring explícito cai no repo comum.
	portalRepo      deal.Repository
	propertyRepo    PropertyRepository
	leadRepo        LeadRepository
	eventBus        events.EventBus
	storage         storage.Service
	financialClient *clients.FinancialClient
	opsClient       *clients.OpsClient
	calendarClient  *clients.CalendarClient
	commClient      *clients.CommClient
	leadHydrator    PartyLeadHydrator
	signerSyncer    DealSignerSyncer
	crossRepo       CrossTenantLookup
	approvalPolicy  ApprovalPolicyProvider
	auditRecorder   dealAuditRecorder
	// checklistDocs espelha no painel de documentos os arquivos anexados a itens
	// do checklist da esteira digital (o upload vive no metadata do negócio; o
	// painel lê deal.documents). Opcional — sem wiring o espelho é no-op.
	checklistDocs ChecklistDocumentSyncer
	// virusScanner varre os uploads do portal do cliente (P1-13).
	// Fail-closed, mesma semântica do DocumentService.
	virusScanner virusscan.Scanner
}

// ChecklistDocumentSyncer é o que o DealService precisa para materializar no
// painel de documentos um arquivo enviado pela esteira digital. Implementado por
// DocumentService; interface local para os testes injetarem um dublê.
type ChecklistDocumentSyncer interface {
	EnsureForChecklistItem(ctx context.Context, in ChecklistDocumentInput) (*document.Document, error)
	// PublishUploaded enfileira o documento para OCR+IA. O espelho do checklist
	// precisa publicar explicitamente: diferente do upload pela aba Documentos,
	// esse caminho não passa pelo PublishUploaded do DocumentService.
	PublishUploaded(ctx context.Context, d *document.Document)
}

// WithChecklistDocumentSync liga o espelho dos documentos da esteira no painel
// de documentos da imobiliária. Wired em routes.go a partir do DocumentService.
func (s *DealService) WithChecklistDocumentSync(syncer ChecklistDocumentSyncer) *DealService {
	s.checklistDocs = syncer
	return s
}

// WithVirusScanner injeta o antivírus do funil de uploads do portal (P1-13).
// Encadeável; testes usam um scanner fake.
func (s *DealService) WithVirusScanner(scanner virusscan.Scanner) *DealService {
	s.virusScanner = scanner
	return s
}

// mirrorChecklistDocument grava (ou atualiza) o documento no painel. Best-effort
// de propósito: o upload do cliente já está confirmado e o checklist é a fonte
// da verdade — derrubar a resposta por causa do espelho faria o cliente reenviar
// um arquivo que já subiu. A falha fica no log e o backfill
// (DealService.BackfillChecklistDocuments) reconcilia depois.
func (s *DealService) mirrorChecklistDocument(ctx context.Context, d *deal.Deal, item *deal.ChecklistItem, uploadedBy string) {
	if s.checklistDocs == nil || d == nil || item == nil {
		return
	}
	labels := make([]string, 0, len(item.RequestedFields))
	for _, f := range item.RequestedFields {
		if label := strings.TrimSpace(f.Label); label != "" {
			labels = append(labels, label)
			continue
		}
		if key := strings.TrimSpace(f.Key); key != "" {
			labels = append(labels, key)
		}
	}
	uploadedAt := time.Now()
	if item.UploadedAt != nil {
		uploadedAt = *item.UploadedAt
	}
	in := ChecklistDocumentInput{
		TenantID:             d.TenantID,
		DealID:               d.ID,
		PartyID:              item.PartyID,
		ItemID:               item.ID,
		ItemName:             item.Name,
		ItemCategory:         string(item.Category),
		RequestedFieldLabels: labels,
		FileName:             item.FileName,
		StorageKey:           item.StorageKey,
		FileSize:             item.FileSize,
		ContentType:          item.ContentType,
		UploadedBy:           uploadedBy,
		UploadedAt:           uploadedAt,
	}
	doc, err := s.checklistDocs.EnsureForChecklistItem(ctx, in)
	if err != nil {
		log.Printf("WARN: checklist document mirror failed for deal=%s item=%s: %v", d.ID, item.ID, err)
		return
	}
	if doc != nil {
		s.checklistDocs.PublishUploaded(ctx, doc)
	}
}

// CrossTenantLookup is the subset of repository.CrossTenantRepository the deal
// service needs to authorize a co-brokering deal against a partner-shared
// resource. Declared as an interface here so tests can inject a fake without
// importing the repository package.
type CrossTenantLookup interface {
	PropertyOwnerTenant(ctx context.Context, propertyID string) (string, error)
	LeadOwnerTenant(ctx context.Context, leadID string) (string, error)
	ActivePartnershipBetween(ctx context.Context, companyA, companyB string) (*repository.CrossPartnership, error)
	// BrokersInTenant valida os corretores nomeados na divisão de comissão
	// (N14) — ver a motivação na interface do repositório.
	BrokersInTenant(ctx context.Context, tenantID string, brokerIDs []string) (map[string]struct{}, error)
}

// WithCrossRepo wires the cross-schema lookup used to authorize co-brokering
// deals (a deal opened against a property/lead owned by a partner company).
// Optional — when nil, Create rejects any cross-tenant resource exactly as it
// did before this feature (the pre-existing "invalid X for tenant" path).
func (s *DealService) WithCrossRepo(c CrossTenantLookup) *DealService {
	s.crossRepo = c
	return s
}

func NewDealService(repo deal.Repository, propertyRepo PropertyRepository, leadRepo LeadRepository, eventBus events.EventBus, storageService storage.Service) *DealService {
	return &DealService{
		repo:            repo,
		portalRepo:      repo,
		propertyRepo:    propertyRepo,
		leadRepo:        leadRepo,
		eventBus:        eventBus,
		storage:         storageService,
		financialClient: clients.NewFinancialClient(),
		opsClient:       clients.NewOpsClient(),
		calendarClient:  clients.NewCalendarClient(),
		commClient:      clients.NewCommClient(),
		leadHydrator:    clients.NewCrmClient(),
		auditRecorder:   audit.Record,
	}
}

// WithLeadHydrator overrides the default CrmClient with a custom hydrator,
// primarily for tests.
func (s *DealService) WithLeadHydrator(h PartyLeadHydrator) *DealService {
	s.leadHydrator = h
	return s
}

// WithSignerSyncer wires the contract-driven re-sync used by MoveDealStatus
// to auto-heal a stale Signers projection before failing the "all required
// signers must sign" gate. Wired in routes.go from ContractService; tests
// that don't need this can leave it nil.
func (s *DealService) WithSignerSyncer(syncer DealSignerSyncer) *DealService {
	s.signerSyncer = syncer
	return s
}

// ReconcileSigners is the operator-triggered recovery for a single deal
// whose Deal.Signers projection drifted from the contract. Exposed as
// POST /api/v1/deals/:id/reconcile-signers so the Esteira "Reconciliar"
// button can force the sync without depending on event redelivery from
// toca-signature. Idempotent.
// ErrDealNotInScope é devolvido quando o negócio existe no tenant mas não está
// no escopo do chamador (P1/A25).
var ErrDealNotInScope = errors.New("deal not in caller scope")

// ReconcileSigners reprojeta os signatários do negócio.
//
// P1/A25: o handler só exigia `deals:write` (que o `corretor` tem) e o serviço
// não aplicava escopo — um corretor forçava ressincronização de signatários de
// negócio de OUTRO corretor. Agora o negócio é resolvido com o escopo do
// chamador antes de qualquer efeito.
func (s *DealService) ReconcileSigners(ctx context.Context, tenantID, dealID, brokerID string) error {
	if _, err := s.getDealByScope(ctx, tenantID, dealID, brokerID); err != nil {
		return ErrDealNotInScope
	}
	return s.reconcileSigners(ctx, tenantID, dealID)
}

func (s *DealService) reconcileSigners(ctx context.Context, tenantID, dealID string) error {
	if s == nil {
		return errors.New("deal service not configured")
	}
	if s.signerSyncer == nil {
		return errors.New("signer syncer not wired")
	}
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(dealID) == "" {
		return errors.New("tenant_id and deal_id are required")
	}
	if err := s.signerSyncer.SyncDealSignersFromActiveContract(ctx, tenantID, dealID); err != nil {
		return fmt.Errorf("reconcile signers for deal %s: %w", dealID, err)
	}
	return nil
}

// signersStaleAgainstContract decides whether MoveDealStatus(→CONVERTED)
// should attempt a contract-driven resync before validating. We only resync
// when at least one required signer is still PENDING — happy paths skip the
// extra repo round-trip entirely.
func (s *DealService) signersStaleAgainstContract(d *deal.Deal) bool {
	if s == nil || s.signerSyncer == nil || d == nil {
		return false
	}
	for i := range d.Signers {
		sig := &d.Signers[i]
		if !sig.IsSignerRequired() {
			continue
		}
		if sig.Status != deal.SignerStatusSigned {
			return true
		}
	}
	return false
}

// hydratePartiesFromLeads fills Name/Email/Phone/Document/AccountID for parties
// that were added with only a LeadID. This is the bridge between deal creation
// (which receives bare lead IDs for the owner from the property link, and for
// the customer when the handler doesn't pre-fetch the lead) and the parties
// panel UI, which renders "Cadastro pendente" when Name+Email are empty.
//
// Errors per-party are logged and skipped — a CRM outage must not block deal
// creation; the owner can still be edited manually in the UI later.
func (s *DealService) hydratePartiesFromLeads(ctx context.Context, d *deal.Deal) {
	if s == nil || s.leadHydrator == nil || d == nil {
		return
	}
	for i := range d.Parties {
		p := &d.Parties[i]
		if strings.TrimSpace(p.LeadID) == "" {
			continue
		}
		// Only hydrate parties that are missing the basic identifying fields;
		// fields the caller explicitly set must be preserved.
		hasName := strings.TrimSpace(p.Name) != ""
		hasEmail := strings.TrimSpace(p.Email) != ""
		hasPhone := strings.TrimSpace(p.Phone) != ""
		hasDoc := strings.TrimSpace(p.Document) != ""
		hasAccount := strings.TrimSpace(p.AccountID) != ""
		if hasName && hasEmail && hasPhone && hasDoc && hasAccount {
			continue
		}
		lead, err := s.leadHydrator.GetLead(ctx, d.TenantID, p.LeadID)
		if err != nil || lead == nil {
			continue
		}
		if !hasName && strings.TrimSpace(lead.Name) != "" {
			p.Name = lead.Name
		}
		if !hasEmail && strings.TrimSpace(lead.Email) != "" {
			p.Email = lead.Email
		}
		if !hasPhone && strings.TrimSpace(lead.Phone) != "" {
			p.Phone = lead.Phone
		}
		if !hasDoc && strings.TrimSpace(lead.Documentation) != "" {
			p.Document = lead.Documentation
		}
		if !hasAccount && strings.TrimSpace(lead.AccountID) != "" {
			p.AccountID = lead.AccountID
		}
	}
}

// PartyProfileHints bundles caller-supplied profile data so the service can
// populate Party.Name/Email/Phone/Document/AccountID up front instead of
// relying on the CRM hydration fallback (which can silently no-op if
// INTERNAL_API_KEY / CRM_SERVICE_URL aren't configured, or if the CRM call
// times out). Empty fields are ignored — they don't overwrite values supplied
// by other sources.
type PartyProfileHints struct {
	Name      string
	Email     string
	Phone     string
	Document  string
	AccountID string
}

// coBrokeringDecision is the outcome of the cross-tenant resource check.
type coBrokeringDecision struct {
	snapshot        *access.Snapshot
	coBrokering     bool
	sourceCompanyID string
}

// errCrossRepoUnavailable is an INTERNAL sentinel: the cross-schema repo is
// not wired (tests, or a deployment without the feature). Callers treat it as
// "cannot authorize cross-tenant" and fall back to the pre-existing
// "invalid X for tenant" rejection — preserving the original security gate.
var errCrossRepoUnavailable = errors.New("cross-tenant lookup not configured")

// authorizeCrossTenantProperty checks a property that does not belong to the
// caller tenant. It returns a decision with coBrokering=true when an active
// partnership between the two companies grants the shared_portfolio scope and
// AllowCoBrokering is on; otherwise it returns ErrCrossTenantResourceDenied.
//
// allowOperate carries the caller's `shared_content:operate` permission — when
// the cross-tenant branch is genuinely taken but allowOperate is false, the
// call is rejected with ErrSharedContentOperateDenied.
func (s *DealService) authorizeCrossTenantProperty(ctx context.Context, propertyID, callerTenantID string, allowOperate bool) (coBrokeringDecision, error) {
	if s.crossRepo == nil {
		return coBrokeringDecision{}, errCrossRepoUnavailable
	}
	ownerTenant, err := s.crossRepo.PropertyOwnerTenant(ctx, propertyID)
	if err != nil || ownerTenant == "" {
		// Property genuinely does not exist anywhere — fall back to the
		// legacy "invalid property_id" rejection.
		return coBrokeringDecision{}, errCrossRepoUnavailable
	}
	if ownerTenant == callerTenantID {
		// Same tenant after all (property repo just didn't surface it) —
		// not a cross-company deal.
		return coBrokeringDecision{}, nil
	}
	p, err := s.crossRepo.ActivePartnershipBetween(ctx, callerTenantID, ownerTenant)
	if err != nil {
		return coBrokeringDecision{}, err
	}
	if p == nil || !p.GrantFrom(ownerTenant).Allows("shared_portfolio") || !p.GrantFrom(ownerTenant).Sharing.AllowCoBrokering {
		return coBrokeringDecision{}, ErrCrossTenantResourceDenied
	}
	if !allowOperate {
		return coBrokeringDecision{}, ErrSharedContentOperateDenied
	}
	checker, ok := s.crossRepo.(interface {
		EligibleSharedResource(context.Context, string, string, string, access.Grant) (bool, error)
	})
	if !ok {
		return coBrokeringDecision{}, ErrCrossTenantResourceDenied
	}
	eligible, err := checker.EligibleSharedResource(ctx, "property", propertyID, ownerTenant, p.GrantFrom(ownerTenant))
	if err != nil {
		return coBrokeringDecision{}, err
	}
	if !eligible {
		return coBrokeringDecision{}, ErrCrossTenantResourceDenied
	}
	return coBrokeringDecision{coBrokering: true, sourceCompanyID: ownerTenant, snapshot: p.Snapshot()}, nil
}

// authorizeCrossTenantLead is the lead-shared counterpart of
// authorizeCrossTenantProperty: it requires the partnership to grant the
// `leads` scope (instead of shared_portfolio).
func (s *DealService) authorizeCrossTenantLead(ctx context.Context, leadID, callerTenantID string, allowOperate bool) (coBrokeringDecision, error) {
	if s.crossRepo == nil {
		return coBrokeringDecision{}, errCrossRepoUnavailable
	}
	ownerTenant, err := s.crossRepo.LeadOwnerTenant(ctx, leadID)
	if err != nil || ownerTenant == "" {
		return coBrokeringDecision{}, errCrossRepoUnavailable
	}
	if ownerTenant == callerTenantID {
		return coBrokeringDecision{}, nil
	}
	p, err := s.crossRepo.ActivePartnershipBetween(ctx, callerTenantID, ownerTenant)
	if err != nil {
		return coBrokeringDecision{}, err
	}
	if p == nil || !p.GrantFrom(ownerTenant).Allows("leads") || !p.GrantFrom(ownerTenant).Sharing.AllowCoBrokering {
		return coBrokeringDecision{}, ErrCrossTenantResourceDenied
	}
	if !allowOperate {
		return coBrokeringDecision{}, ErrSharedContentOperateDenied
	}
	checker, ok := s.crossRepo.(interface {
		EligibleSharedResource(context.Context, string, string, string, access.Grant) (bool, error)
	})
	if !ok {
		return coBrokeringDecision{}, ErrCrossTenantResourceDenied
	}
	eligible, err := checker.EligibleSharedResource(ctx, "lead", leadID, ownerTenant, p.GrantFrom(ownerTenant))
	if err != nil {
		return coBrokeringDecision{}, err
	}
	if !eligible {
		return coBrokeringDecision{}, ErrCrossTenantResourceDenied
	}
	return coBrokeringDecision{coBrokering: true, sourceCompanyID: ownerTenant, snapshot: p.Snapshot()}, nil
}

// CreateDealOption ajusta a criação sem quebrar os call sites existentes.
type CreateDealOption func(*createDealOptions)

type createDealOptions struct {
	idempotencyKey string
}

// WithIdempotencyKey identifica a INTENÇÃO de criação: o mesmo formulário
// reenviado (duplo clique, retry de rede) usa a mesma chave e recebe de volta
// o negócio já criado em vez de um duplicado.
func WithIdempotencyKey(key string) CreateDealOption {
	return func(o *createDealOptions) { o.idempotencyKey = strings.TrimSpace(key) }
}

func resolveCreateDealOptions(opts []CreateDealOption) createDealOptions {
	var o createDealOptions
	for _, apply := range opts {
		if apply != nil {
			apply(&o)
		}
	}
	return o
}

// duplicateWindow é a janela da guarda para clientes sem chave de idempotência:
// dois envios simultâneos (mesmo imóvel + cliente + tipo + valor) em menos de
// um minuto são o mesmo evento de negócio, não duas negociações.
const duplicateWindow = 60 * time.Second

// DuplicateDealError bloqueia uma criação duplicada e carrega o negócio
// existente para a borda HTTP devolver (409) e o painel apontar o cadastro.
type DuplicateDealError struct {
	ExistingID string
}

func (e *DuplicateDealError) Error() string {
	return "um negócio ativo com as mesmas partes, imóvel e valor já foi criado agora (existing_deal_id=" + e.ExistingID + ")"
}

// applyCreationDedup resolve a proteção de duplicação antes da criação.
// Retorna (nil, nil) quando a criação deve prosseguir; o negócio existente no
// replay idempotente; ou erro (chave reutilizada com payload diferente /
// duplicata na janela). Repositórios sem a guarda (testes) seguem sem proteção.
func (s *DealService) applyCreationDedup(
	ctx context.Context,
	o createDealOptions,
	tenantID, propertyID, customerID string,
	dealType deal.Type,
	value float64,
) (*deal.Deal, error) {
	guard, ok := s.repo.(deal.CreationGuardRepository)
	if !ok || guard == nil {
		return nil, nil
	}
	if o.idempotencyKey != "" {
		existing, err := guard.GetByIdempotencyKey(ctx, tenantID, o.idempotencyKey)
		if err != nil {
			return nil, fmt.Errorf("deal idempotency lookup: %w", err)
		}
		if existing == nil {
			return nil, nil
		}
		if err := validateCreationReplay(existing, dealType, propertyID, customerID, value); err != nil {
			return nil, err
		}
		audit.Record(ctx, audit.Entry{
			Action:     "deal.create.idempotent_replay",
			EntityType: "Deal",
			EntityID:   existing.ID,
			Metadata:   map[string]any{"tenant_id": tenantID, "property_id": propertyID, "customer_id": customerID},
		})
		return existing, nil
	}
	existing, err := guard.FindRecentDuplicate(ctx, tenantID, propertyID, customerID, dealType, value, time.Now().Add(-duplicateWindow))
	if err != nil {
		return nil, fmt.Errorf("deal duplicate window lookup: %w", err)
	}
	if existing != nil {
		return nil, &DuplicateDealError{ExistingID: existing.ID}
	}
	return nil, nil
}

// validateCreationReplay protege a chave de idempotência de reúso indevido: a
// mesma chave com payload diferente é conflito, nunca um replay silencioso.
func validateCreationReplay(existing *deal.Deal, dealType deal.Type, propertyID, customerID string, value float64) error {
	if existing == nil {
		return nil
	}
	if existing.Type != dealType || existing.PropertyID != propertyID ||
		existing.CustomerID != customerID || math.Abs(existing.Value-value) > 0.005 {
		return fmt.Errorf("idempotency key reused with a different payload (existing deal %s)", existing.ID)
	}
	return nil
}

// resolveIdempotentConflict reexecuta a leitura do vencedor quando a inserção
// perdeu a corrida para um envio simultâneo com a mesma chave.
func (s *DealService) resolveIdempotentConflict(
	ctx context.Context,
	o createDealOptions,
	tenantID, propertyID, customerID string,
	dealType deal.Type,
	value float64,
) (*deal.Deal, error) {
	if o.idempotencyKey == "" {
		return nil, nil
	}
	guard, ok := s.repo.(deal.CreationGuardRepository)
	if !ok || guard == nil {
		return nil, nil
	}
	winner, err := guard.GetByIdempotencyKey(ctx, tenantID, o.idempotencyKey)
	if err != nil {
		return nil, err
	}
	if winner == nil {
		return nil, nil
	}
	if err := validateCreationReplay(winner, dealType, propertyID, customerID, value); err != nil {
		return nil, err
	}
	return winner, nil
}

func (s *DealService) Create(
	ctx context.Context,
	dealType deal.Type,
	propertyID, ownerID, customerID, brokerID, leadID string,
	value float64,
	tenantID string,
	customerName, customerEmail, customerAccountID, customerRole string,
	ownerHints PartyProfileHints,
	allowCrossTenantOperate bool,
	opts ...CreateDealOption,
) (*deal.Deal, error) {
	o := resolveCreateDealOptions(opts)
	if replay, err := s.applyCreationDedup(ctx, o, tenantID, propertyID, customerID, dealType, value); err != nil || replay != nil {
		return replay, err
	}

	// Cross-tenant validation gate. Without this, a tenant could create a deal
	// referencing a property or lead that belongs to a different tenant by
	// passing a forged ID — a real risk because Deal stores foreign IDs as
	// plain strings (not FKs to a single shared schema).
	//
	// When a property/lead does NOT belong to the caller tenant the deal is
	// rejected UNLESS an active co-brokering partnership grants access — in
	// which case the deal is flagged CoBrokering and SourceCompanyID is set.
	//
	// We validate eagerly (before deal.New) so domain validation errors only
	// surface for genuinely well-formed requests. Repos may be nil in tests;
	// when nil we skip — production wiring always passes them.
	coBrokering := false
	sourceCompanyID := ""
	var partnershipSnapshot *access.Snapshot
	if s.propertyRepo != nil && strings.TrimSpace(propertyID) != "" {
		if _, err := s.propertyRepo.GetSummaryByID(ctx, tenantID, propertyID, dealType); err != nil {
			// Property is not in the caller tenant — allow only if a
			// co-brokering partnership authorizes it.
			decision, authErr := s.authorizeCrossTenantProperty(ctx, strings.TrimSpace(propertyID), tenantID, allowCrossTenantOperate)
			if authErr != nil {
				if errors.Is(authErr, ErrCrossTenantResourceDenied) || errors.Is(authErr, ErrSharedContentOperateDenied) {
					return nil, authErr
				}
				return nil, fmt.Errorf("invalid property_id for tenant: %w", err)
			}
			if decision.coBrokering {
				coBrokering = true
				sourceCompanyID = decision.sourceCompanyID
				partnershipSnapshot = decision.snapshot
			}
		}
	}
	if s.leadRepo != nil {
		// De-duplicate lookups: ownerID, customerID and leadID often coincide.
		seen := map[string]struct{}{}
		check := func(label, id string) error {
			id = strings.TrimSpace(id)
			if id == "" {
				return nil
			}
			if _, ok := seen[id]; ok {
				return nil
			}
			seen[id] = struct{}{}
			ok, err := s.leadRepo.Exists(ctx, tenantID, id)
			if err != nil {
				return fmt.Errorf("validating %s: %w", label, err)
			}
			if !ok {
				// Lead is not in the caller tenant. The lead-shared
				// co-brokering path applies only to the lead_id itself —
				// owner/customer leads are deal-local and must belong to
				// the caller. So only lead_id gets the partnership check.
				if label == "lead_id" {
					decision, authErr := s.authorizeCrossTenantLead(ctx, id, tenantID, allowCrossTenantOperate)
					if authErr != nil {
						if errors.Is(authErr, ErrCrossTenantResourceDenied) || errors.Is(authErr, ErrSharedContentOperateDenied) {
							return authErr
						}
						return fmt.Errorf("invalid %s for tenant: lead %s not found", label, id)
					}
					if decision.coBrokering {
						if partnershipSnapshot != nil && partnershipSnapshot.ID != decision.snapshot.ID {
							return ErrCrossTenantResourceDenied
						}
						coBrokering = true
						if sourceCompanyID == "" {
							sourceCompanyID = decision.sourceCompanyID
							partnershipSnapshot = decision.snapshot
						}
					}
					return nil
				}
				return fmt.Errorf("invalid %s for tenant: lead %s not found", label, id)
			}
			return nil
		}
		if err := check("owner_id", ownerID); err != nil {
			return nil, err
		}
		if err := check("customer_id", customerID); err != nil {
			return nil, err
		}
		if err := check("lead_id", leadID); err != nil {
			return nil, err
		}
	}

	d, err := deal.New(dealType, propertyID, ownerID, customerID, brokerID, value, tenantID)
	if err != nil {
		return nil, err
	}
	if coBrokering {
		d.CoBrokering = true
		d.SourceCompanyID = sourceCompanyID
		d.PartnershipSnapshot = partnershipSnapshot
	}

	// Override lead ID if explicitly provided (e.g., after lead/customer separation)
	if leadID != "" {
		d.LeadID = leadID
	}

	_ = d.AddParty(deal.Party{
		LeadID:    ownerID,
		Role:      deal.PartyRoleOwner,
		IsPrimary: true,
		Name:      ownerHints.Name,
		Email:     ownerHints.Email,
		Phone:     ownerHints.Phone,
		Document:  ownerHints.Document,
		AccountID: ownerHints.AccountID,
	})

	customerPartyRole := deal.PartyRoleTenant
	if dealType == deal.TypeSale {
		customerPartyRole = deal.PartyRoleBuyer
	}
	customerPartyLeadID := d.LeadID
	if customerPartyLeadID == "" {
		customerPartyLeadID = customerID
	}

	// Automatically add the customer as a signer if provided
	if customerName != "" && customerEmail != "" {
		role := deal.SignatoryRole(customerRole)
		if role == "" {
			if dealType == deal.TypeSale {
				role = deal.SignatoryRoleComprador
			} else {
				role = deal.SignatoryRoleLocatario
			}
		}
		if err := d.AddSigner(customerName, customerEmail, role); err == nil {
			for i := range d.Signers {
				if d.Signers[i].Email == customerEmail {
					d.Signers[i].AccountID = customerAccountID
					if partyIdx := findPartyIndexByID(d.Parties, d.Signers[i].ID); partyIdx != -1 {
						d.Parties[partyIdx].AccountID = customerAccountID
						d.Parties[partyIdx].LeadID = customerPartyLeadID
					}
					break
				}
			}
		}
	} else {
		_ = d.AddParty(deal.Party{
			LeadID:    customerPartyLeadID,
			Role:      customerPartyRole,
			IsPrimary: true,
		})
	}

	// Backfill Name/Email/Phone/Document/AccountID on parties that were added
	// with bare LeadIDs (owner pulled from property.owners, and the customer
	// when no name/email was supplied). Without this, the parties panel
	// renders "Cadastro pendente" even though the lead profile is known.
	s.hydratePartiesFromLeads(ctx, d)
	ensureSignersFromParties(d)

	d.IdempotencyKey = o.idempotencyKey
	if err := s.repo.Create(ctx, d); err != nil {
		if errors.Is(err, deal.ErrIdempotencyConflict) {
			if winner, rerr := s.resolveIdempotentConflict(ctx, o, tenantID, propertyID, customerID, dealType, value); rerr != nil || winner != nil {
				return winner, rerr
			}
		}
		return nil, err
	}

	// As partes já passam a receber atualizações com link do portal a partir
	// daqui: a conta de portal precisa existir antes do evento para que o
	// pedido de código encontre o contato (e o canal do cliente já nasça
	// vinculado à conta em vez de só ao e-mail).
	s.provisionPortalAccounts(ctx, d)

	// Publish DealCreated event
	if s.eventBus != nil {
		evt := NewDealCreatedEvent(d)
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue deal.created event for deal=%s: %v", d.ID, err)
		}
	}
	// Cross-company co-brokering deal — notify the resource owner company.
	s.publishPartnerDealInitiated(ctx, d, leadID)
	audit.Record(ctx, audit.Entry{
		Action:     "create",
		EntityType: "Deal",
		EntityID:   d.ID,
		After: map[string]any{
			"status":      string(d.Status),
			"type":        string(d.Type),
			"property_id": d.PropertyID,
			"broker_id":   d.BrokerID,
			"value":       d.Value,
		},
	})

	return d, nil
}

// publishPartnerDealInitiated emits partner.deal_initiated when a deal was
// opened against a partner-shared resource (CoBrokering=true). Best-effort —
// a publish failure is logged but never fails deal creation. resourceLeadID is
// the lead_id supplied to Create; when it differs from the deal's own
// owner/customer leads it identifies the shared lead the deal started from.
func (s *DealService) publishPartnerDealInitiated(ctx context.Context, d *deal.Deal, resourceLeadID string) {
	if s.eventBus == nil || d == nil || !d.CoBrokering {
		return
	}
	// The shared resource is the property when the deal carries one,
	// otherwise the shared lead.
	resourceType := "property"
	resourceID := d.PropertyID
	if strings.TrimSpace(resourceID) == "" {
		resourceType = "lead"
		resourceID = strings.TrimSpace(resourceLeadID)
		if resourceID == "" {
			resourceID = d.LeadID
		}
	}
	evt := contracts.PartnerDealInitiatedEvent{
		Data: contracts.PartnerDealInitiatedEventPayload{
			DealID:            d.ID,
			ResourceType:      resourceType,
			ResourceID:        resourceID,
			InitiatorTenantID: d.TenantID,
			OwnerTenantID:     d.SourceCompanyID,
			CreatedAt:         time.Now().UTC().Format(time.RFC3339),
		},
	}
	if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
		log.Printf("WARN: failed to enqueue partner.deal_initiated event for deal=%s: %v", d.ID, err)
	}
}

func findPartyIndexByID(parties []deal.Party, partyID string) int {
	for i := range parties {
		if parties[i].ID == partyID {
			return i
		}
	}
	return -1
}

func signatoryRoleFromPartyRole(dealType deal.Type, role deal.PartyRole) (deal.SignatoryRole, bool) {
	switch role {
	case deal.PartyRoleSpouse:
		return deal.SignatoryRoleConjuge, true
	case deal.PartyRoleAttorney:
		return deal.SignatoryRoleProcurador, true
	case deal.PartyRoleLegalRep:
		return deal.SignatoryRoleRepLegal, true
	case deal.PartyRoleGuardian:
		return deal.SignatoryRoleTutor, true
	case deal.PartyRoleCurator:
		return deal.SignatoryRoleCurador, true
	}
	switch dealType {
	case deal.TypeSale:
		switch role {
		case deal.PartyRoleBuyer:
			return deal.SignatoryRoleComprador, true
		case deal.PartyRoleOwner:
			return deal.SignatoryRoleVendedor, true
		case deal.PartyRoleGuarantor:
			return deal.SignatoryRoleAvalista, true
		case deal.PartyRoleWitness:
			return deal.SignatoryRoleTestemunha, true
		default:
			return "", false
		}
	case deal.TypeRent:
		switch role {
		case deal.PartyRoleTenant:
			return deal.SignatoryRoleLocatario, true
		case deal.PartyRoleOwner:
			return deal.SignatoryRoleLocador, true
		case deal.PartyRoleGuarantor:
			return deal.SignatoryRoleFiador, true
		case deal.PartyRoleWitness:
			return deal.SignatoryRoleTestemunha, true
		default:
			return "", false
		}
	default:
		return "", false
	}
}

func ensureSignersFromParties(d *deal.Deal) {
	if d == nil {
		return
	}
	for _, p := range d.Parties {
		if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Email) == "" {
			continue
		}
		if p.ID != "" {
			exists := false
			for i := range d.Signers {
				if d.Signers[i].ID == p.ID {
					exists = true
					break
				}
			}
			if exists {
				continue
			}
		}
		sr, ok := signatoryRoleFromPartyRole(d.Type, p.Role)
		if !ok {
			continue
		}
		_ = d.AddSigner(p.Name, p.Email, sr)
		if p.AccountID != "" {
			for i := range d.Signers {
				if d.Signers[i].Email == p.Email {
					d.Signers[i].AccountID = p.AccountID
					if partyIdx := findPartyIndexByID(d.Parties, d.Signers[i].ID); partyIdx != -1 {
						d.Parties[partyIdx].AccountID = p.AccountID
						if d.Parties[partyIdx].LeadID == "" && p.LeadID != "" {
							d.Parties[partyIdx].LeadID = p.LeadID
						}
					}
					break
				}
			}
		}
	}
}

func (s *DealService) CreateWithParties(
	ctx context.Context,
	dealType deal.Type,
	propertyID, ownerID, customerID, brokerID, leadID string,
	value float64,
	tenantID string,
	parties []deal.Party,
	ownerHints PartyProfileHints,
	allowCrossTenantOperate bool,
	opts ...CreateDealOption,
) (*deal.Deal, error) {
	o := resolveCreateDealOptions(opts)
	if replay, err := s.applyCreationDedup(ctx, o, tenantID, propertyID, customerID, dealType, value); err != nil || replay != nil {
		return replay, err
	}

	// Cross-company co-brokering check for the property the deal carries.
	// CreateWithParties historically skipped the cross-tenant property gate;
	// here we only ACT on it when the property is genuinely owned by another
	// tenant — same-tenant deals are unaffected. crossRepo nil → skip (tests).
	coBrokering := false
	sourceCompanyID := ""
	var partnershipSnapshot *access.Snapshot
	if s.crossRepo != nil && strings.TrimSpace(propertyID) != "" {
		ownerTenant, lookupErr := s.crossRepo.PropertyOwnerTenant(ctx, strings.TrimSpace(propertyID))
		if lookupErr != nil || ownerTenant == "" {
			return nil, ErrCrossTenantResourceDenied
		}
		if ownerTenant != tenantID {
			decision, authErr := s.authorizeCrossTenantProperty(ctx, strings.TrimSpace(propertyID), tenantID, allowCrossTenantOperate)
			if authErr != nil {
				return nil, authErr
			}
			if decision.coBrokering {
				coBrokering = true
				sourceCompanyID = decision.sourceCompanyID
				partnershipSnapshot = decision.snapshot
			}
		}
	}

	if s.crossRepo != nil && strings.TrimSpace(leadID) != "" {
		owner, err := s.crossRepo.LeadOwnerTenant(ctx, leadID)
		if err != nil || owner == "" {
			return nil, ErrCrossTenantResourceDenied
		}
		if owner != tenantID {
			decision, err := s.authorizeCrossTenantLead(ctx, leadID, tenantID, allowCrossTenantOperate)
			if err != nil {
				return nil, err
			}
			if partnershipSnapshot != nil && partnershipSnapshot.ID != decision.snapshot.ID {
				return nil, ErrCrossTenantResourceDenied
			}
			coBrokering = true
			sourceCompanyID = owner
			partnershipSnapshot = decision.snapshot
		}
	}

	d, err := deal.New(dealType, propertyID, ownerID, customerID, brokerID, value, tenantID)
	if err != nil {
		return nil, err
	}
	d.IdempotencyKey = o.idempotencyKey
	if coBrokering {
		d.CoBrokering = true
		d.SourceCompanyID = sourceCompanyID
		d.PartnershipSnapshot = partnershipSnapshot
	}

	if leadID != "" {
		d.LeadID = leadID
	}

	for _, p := range parties {
		if err := d.AddParty(p); err != nil {
			return nil, err
		}
	}

	if d.PrimaryByRole(deal.PartyRoleOwner) == nil && ownerID != "" {
		_ = d.AddParty(deal.Party{
			LeadID:    ownerID,
			Role:      deal.PartyRoleOwner,
			IsPrimary: true,
			Name:      ownerHints.Name,
			Email:     ownerHints.Email,
			Phone:     ownerHints.Phone,
			Document:  ownerHints.Document,
			AccountID: ownerHints.AccountID,
		})
	}

	customerPartyRole := deal.PartyRoleTenant
	if dealType == deal.TypeSale {
		customerPartyRole = deal.PartyRoleBuyer
	}
	customerPartyLeadID := d.LeadID
	if customerPartyLeadID == "" {
		customerPartyLeadID = customerID
	}
	if d.PrimaryByRole(customerPartyRole) == nil && customerPartyLeadID != "" {
		_ = d.AddParty(deal.Party{LeadID: customerPartyLeadID, Role: customerPartyRole, IsPrimary: true})
	}

	// Hydrate any party that was supplied with only a LeadID so the parties
	// panel renders the lead profile instead of "Cadastro pendente".
	s.hydratePartiesFromLeads(ctx, d)

	for _, p := range d.Parties {
		if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Email) == "" {
			continue
		}
		sr, ok := signatoryRoleFromPartyRole(dealType, p.Role)
		if !ok {
			continue
		}
		if err := d.AddSigner(p.Name, p.Email, sr); err != nil {
			return nil, err
		}
		if p.AccountID != "" {
			for i := range d.Signers {
				if d.Signers[i].Email == p.Email {
					d.Signers[i].AccountID = p.AccountID
					if partyIdx := findPartyIndexByID(d.Parties, d.Signers[i].ID); partyIdx != -1 {
						d.Parties[partyIdx].AccountID = p.AccountID
						if d.Parties[partyIdx].LeadID == "" && p.LeadID != "" {
							d.Parties[partyIdx].LeadID = p.LeadID
						}
					}
					break
				}
			}
		}
	}

	d.SyncLegacyIDsFromParties()

	if err := s.repo.Create(ctx, d); err != nil {
		if errors.Is(err, deal.ErrIdempotencyConflict) {
			if winner, rerr := s.resolveIdempotentConflict(ctx, o, tenantID, propertyID, customerID, dealType, value); rerr != nil || winner != nil {
				return winner, rerr
			}
		}
		return nil, err
	}

	// Mesmo provisionamento do Create: as duas portas de entrada da esteira
	// precisam deixar as partes aptas a pedir o código do portal.
	s.provisionPortalAccounts(ctx, d)

	if s.eventBus != nil {
		evt := NewDealCreatedEvent(d)
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue deal.created event for deal=%s: %v", d.ID, err)
		}
	}
	// Cross-company co-brokering deal — notify the resource owner company.
	s.publishPartnerDealInitiated(ctx, d, leadID)
	audit.Record(ctx, audit.Entry{
		Action:     "create",
		EntityType: "Deal",
		EntityID:   d.ID,
		After: map[string]any{
			"status":      string(d.Status),
			"type":        string(d.Type),
			"property_id": d.PropertyID,
			"broker_id":   d.BrokerID,
			"value":       d.Value,
		},
		Metadata: map[string]any{"path": "CreateWithParties"},
	})

	return d, nil
}

func (s *DealService) getDealByScope(ctx context.Context, tenantID, id, brokerID string) (*deal.Deal, error) {
	if brokerID != "" {
		return s.repo.GetByIDScoped(ctx, tenantID, id, brokerID)
	}
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *DealService) GetByID(ctx context.Context, tenantID, id, brokerID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, id, brokerID)
	if err != nil {
		return nil, err
	}

	d.EnsurePartiesFromLegacy()
	ensureSignersFromParties(d)

	// Add missing presigned URLs for checklist items
	s.populateChecklistURLs(ctx, d)

	return d, nil
}

// populateChecklistURLs dynamically populates FileURL with short-lived presigned URLs.
func (s *DealService) populateChecklistURLs(ctx context.Context, d *deal.Deal) {
	if s.storage == nil || d == nil {
		return
	}
	for i := range d.Checklist {
		item := &d.Checklist[i]
		if item.StorageKey != "" {
			url, err := s.storage.GetPresignedURL(ctx, "toca-documents", item.StorageKey, presignedURLTTL)
			if err == nil && url != "" {
				item.FileURL = url
			}
		}
	}
}

func (s *DealService) List(ctx context.Context, tenantID string, limit, offset int64, brokerID string) ([]*deal.Deal, int64, error) {
	if brokerID != "" {
		return s.repo.ListScoped(ctx, tenantID, brokerID, limit, offset)
	}
	return s.repo.List(ctx, tenantID, limit, offset)
}

func (s *DealService) ListByCustomerIDs(ctx context.Context, tenantID string, customerIDs []string, limit, offset int64, brokerID string) ([]*deal.Deal, int64, error) {
	if brokerID != "" {
		return s.repo.ListByCustomerIDsScoped(ctx, tenantID, brokerID, customerIDs, limit, offset)
	}
	return s.repo.ListByCustomerIDs(ctx, tenantID, customerIDs, limit, offset)
}

func (s *DealService) Search(ctx context.Context, tenantID string, filter deal.DealFilter, brokerScopeID string) ([]*deal.Deal, int64, error) {
	filter.BrokerScopeID = brokerScopeID
	return s.repo.Search(ctx, tenantID, filter)
}

func (s *DealService) GetStats(ctx context.Context, tenantID, brokerID string) (int64, float64, error) {
	if brokerID != "" {
		return s.repo.GetStatsScoped(ctx, tenantID, brokerID)
	}
	return s.repo.GetStats(ctx, tenantID)
}

func (s *DealService) AddDocument(ctx context.Context, tenantID, dealID string, docType deal.DocumentType, description string) error {
	d, err := s.repo.GetByID(ctx, tenantID, dealID)
	if err != nil {
		return err
	}

	d.AddDocument(docType, description)
	return s.repo.Update(ctx, d)
}

func (s *DealService) UpdateDocumentURL(ctx context.Context, tenantID, dealID, docID, url, userID string) error {
	d, err := s.repo.GetByID(ctx, tenantID, dealID)
	if err != nil {
		return err
	}

	d.UpdateDocument(docID, url, userID)
	return s.repo.Update(ctx, d)
}

func (s *DealService) AddProposal(ctx context.Context, tenantID, dealID, brokerID string, value float64, paymentMethod string, installments int, downPayment float64, createdBy, notes string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}
	d.AddProposal(value, paymentMethod, installments, downPayment, createdBy, notes)
	if d.Status == deal.StatusNegotiation {
		d.Status = deal.StatusProposal
		d.AddHistory(deal.StatusProposal, createdBy, "Deal advanced to Proposal stage")
	}
	d.AddHistory(d.Status, createdBy, "Proposal added")
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}

	if s.eventBus != nil && len(d.Proposals) > 0 {
		customerAccountIDs := SignerAccountIDs(d)

		proposalID := d.Proposals[len(d.Proposals)-1].ID
		evt := contracts.ProposalSentEvent{
			Data: contracts.ProposalSentEventPayload{
				DealID:             d.ID,
				TenantID:           d.TenantID,
				ProposalID:         proposalID,
				Value:              value,
				CustomerAccountIDs: customerAccountIDs,
				CustomerEmails:     CustomerEmailsWithoutAccount(d),
			},
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue proposal.sent event for deal=%s proposal=%s: %v", d.ID, proposalID, err)
		}
	}

	return d, nil
}

// WithPortalRepository troca o repositório usado pelas rotas do portal do
// cliente (pool de serviço). As rotas de operador seguem no repositório comum.
func (s *DealService) WithPortalRepository(repo deal.Repository) *DealService {
	if repo != nil {
		s.portalRepo = repo
	}
	return s
}

// portal devolve o repositório do portal do cliente. Em testes e em serviços
// montados sem wiring explícito cai no repositório comum (mesmo comportamento
// de antes) — nunca fica nulo.
func (s *DealService) portal() deal.Repository {
	if s.portalRepo != nil {
		return s.portalRepo
	}
	return s.repo
}

// ChecklistDocumentBackfillStats resume o backfill dos documentos da esteira.
type ChecklistDocumentBackfillStats struct {
	DealsScanned int `json:"deals_scanned"`
	ItemsScanned int `json:"items_scanned"`
	Documents    int `json:"documents"`
}

// BackfillChecklistDocuments varre os negócios do tenant e materializa no painel
// de documentos os arquivos que os clientes já tinham enviado pela esteira antes
// deste espelho existir (ou cujo espelho falhou). Idempotente: o ID do documento
// é derivado do item, então rodar de novo atualiza em vez de duplicar.
func (s *DealService) BackfillChecklistDocuments(ctx context.Context, tenantID string) (ChecklistDocumentBackfillStats, error) {
	stats := ChecklistDocumentBackfillStats{}
	if s.checklistDocs == nil {
		return stats, errors.New("checklist document sync is not wired")
	}
	const pageSize int64 = 200
	for offset := int64(0); ; offset += pageSize {
		deals, total, err := s.repo.List(ctx, tenantID, pageSize, offset)
		if err != nil {
			return stats, err
		}
		for _, d := range deals {
			stats.DealsScanned++
			if d == nil {
				continue
			}
			for i := range d.Checklist {
				item := &d.Checklist[i]
				if item.Status != deal.ChecklistUploaded || strings.TrimSpace(item.StorageKey) == "" {
					continue
				}
				stats.ItemsScanned++
				labels := make([]string, 0, len(item.RequestedFields))
				for _, f := range item.RequestedFields {
					if label := strings.TrimSpace(f.Label); label != "" {
						labels = append(labels, label)
					} else if key := strings.TrimSpace(f.Key); key != "" {
						labels = append(labels, key)
					}
				}
				uploadedAt := time.Now()
				if item.UploadedAt != nil {
					uploadedAt = *item.UploadedAt
				}
				in := ChecklistDocumentInput{
					TenantID:             tenantID,
					DealID:               d.ID,
					PartyID:              item.PartyID,
					ItemID:               item.ID,
					ItemName:             item.Name,
					ItemCategory:         string(item.Category),
					RequestedFieldLabels: labels,
					FileName:             item.FileName,
					StorageKey:           item.StorageKey,
					FileSize:             item.FileSize,
					ContentType:          item.ContentType,
					UploadedBy:           item.UploadedBy,
					UploadedAt:           uploadedAt,
				}
				if _, err := s.checklistDocs.EnsureForChecklistItem(ctx, in); err != nil {
					return stats, fmt.Errorf("backfill checklist document for deal %s item %s: %w", d.ID, item.ID, err)
				}
				stats.Documents++
			}
		}
		if int64(len(deals)) == 0 || offset+pageSize >= total {
			break
		}
	}
	return stats, nil
}
