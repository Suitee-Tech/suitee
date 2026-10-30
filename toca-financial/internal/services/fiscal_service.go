package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/toca-financial/internal/dimob"
	"github.com/oginoino/toca/toca-financial/internal/domain/fiscal"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	"github.com/oginoino/toca/toca-financial/internal/providers"
)

// transferLister é a fatia mínima do repositório de transfers que o
// FiscalService precisa para montar a DIMOB (#418) e os informes de
// rendimentos (#419) — só o List, que já existe na RepositoryInterface
// de transfer. Definir aqui evita acoplar o FiscalService à interface
// inteira e evita mexer em mocks.
type transferLister interface {
	List(ctx context.Context, tenantID, contractID, ownerID, status, referenceMonth string, limit, offset int64) ([]*transfer.OwnerTransfer, int64, error)
}

type FiscalRepository interface {
	CreateDimob(ctx context.Context, d *fiscal.DimobReport) error
	GetDimobByID(ctx context.Context, tenantID, id string) (*fiscal.DimobReport, error)
	ListDimob(ctx context.Context, tenantID string) ([]*fiscal.DimobReport, error)
	GetDimobByYear(ctx context.Context, tenantID string, year int) (*fiscal.DimobReport, error)
	UpdateDimob(ctx context.Context, d *fiscal.DimobReport) error
	SaveDimob(ctx context.Context, d *fiscal.DimobReport) error

	CreateNfse(ctx context.Context, n *fiscal.Nfse) error
	SaveNfse(ctx context.Context, n *fiscal.Nfse) error
	ListNfse(ctx context.Context, tenantID, status, referenceMonth string, limit, offset int64) ([]*fiscal.Nfse, int64, error)

	CreateInforme(ctx context.Context, i *fiscal.Informe) error
	UpdateInforme(ctx context.Context, i *fiscal.Informe) error
	SaveInforme(ctx context.Context, i *fiscal.Informe) error
	ListInformes(ctx context.Context, tenantID string, year int) ([]*fiscal.Informe, error)
	GetInformeByID(ctx context.Context, tenantID, id string) (*fiscal.Informe, error)

	ListDeadlines(ctx context.Context, tenantID string) ([]*fiscal.FiscalDeadline, error)
	GetCalendar(ctx context.Context, tenantID string, year int) ([]*fiscal.FiscalDeadline, error)

	AddReportHistory(ctx context.Context, h *fiscal.ReportHistory) error
	SaveReport(ctx context.Context, h *fiscal.ReportHistory) error
	ListReportHistory(ctx context.Context, tenantID string, filter fiscal.ReportHistoryFilter) ([]*fiscal.ReportHistory, int64, error)
	ArchiveReport(ctx context.Context, tenantID, id string) error
	GetReportByID(ctx context.Context, tenantID, id string) (*fiscal.ReportHistory, error)

	CountChargesByYear(ctx context.Context, tenantID string, year int, t string) (int, float64, error)
}

type FiscalService struct {
	repo FiscalRepository
	// transfers é a fonte dos repasses (OwnerTransfer) usada para
	// reconstruir as operações de locação da DIMOB (#418) e os totais
	// anuais por proprietário dos informes de rendimentos (#419).
	// Injetado via WithTransferLister — nil-safe.
	transfers transferLister
	// crmClient resolve nome e CPF/CNPJ de leads (locador/locatário da DIMOB
	// e proprietário dos informes). Opcional — sem ele, os campos de
	// documento/parte saem vazios e a validação orienta o operador.
	crmClient *clients.CrmClient
	// dealClient resolve o contrato (número, data, partes) de cada locação.
	// Opcional — sem ele, número/data do contrato saem vazios (advertência).
	dealClient *clients.DealClient
	// propertyClient resolve o endereço/CEP/UF do imóvel no toca-core.
	// Opcional — sem ele, a validação BLOQUEIA (a RFB não importa sem
	// endereço/CEP), com mensagem apontando cada contrato pendente.
	propertyClient *clients.PropertyClient
	// nfseProvider emite a NFS-e no provedor externo (P1-05). Nil mantém o
	// comportamento legado de emissão local (usado pelos testes antigos);
	// o router injeta o sandbox por padrão.
	nfseProvider providers.NfseProvider
}

func NewFiscalService(repo FiscalRepository) *FiscalService {
	return &FiscalService{repo: repo}
}

// WithNfseProvider injeta o provedor de NFS-e (P1-05). Encadeável.
func (s *FiscalService) WithNfseProvider(p providers.NfseProvider) *FiscalService {
	s.nfseProvider = p
	return s
}

// WithTransferLister injeta o repositório de transfers usado pelo
// BuildDimobFile para montar o TXT da DIMOB (#418). Encadeável, no mesmo
// padrão de TransferService.WithEventBus.
func (s *FiscalService) WithTransferLister(tl transferLister) *FiscalService {
	s.transfers = tl
	return s
}

// WithCrmClient injeta o cliente do CRM, usado pelo GenerateInformes
// para resolver o nome do proprietário (#419) e pela DIMOB para os
// documentos/nomes de locador e locatário. Encadeável.
func (s *FiscalService) WithCrmClient(c *clients.CrmClient) *FiscalService {
	s.crmClient = c
	return s
}

// WithDealClient injeta o cliente de contratos do toca-deal para enriquecer
// a DIMOB (número e data do contrato, partes). Encadeável.
func (s *FiscalService) WithDealClient(c *clients.DealClient) *FiscalService {
	s.dealClient = c
	return s
}

// WithPropertyClient injeta o cliente de imóveis do toca-core para enriquecer
// a DIMOB (endereço/CEP/UF — obrigatórios no leiaute). Encadeável.
func (s *FiscalService) WithPropertyClient(c *clients.PropertyClient) *FiscalService {
	s.propertyClient = c
	return s
}

// ErrDimobNotReady — a DIMOB do ano não pode ser baixada ainda: não foi
// gerada, foi gerada sem o CNPJ da declarante, ou os dados agregados não
// formam uma declaração válida. O handler trata como 400 — o usuário
// corrige (re)gerando a declaração.
var ErrDimobNotReady = errors.New("dimob: declaração não está pronta para download")

// ── Calendar ──

func (s *FiscalService) GetCalendar(ctx context.Context, tenantID string, year int) ([]*fiscal.FiscalDeadline, error) {
	deadlines, err := s.repo.GetCalendar(ctx, tenantID, year)
	if err != nil {
		return nil, err
	}

	// Se não houver prazos no banco, retorna array vazio em vez de gerar dados fictícios
	if len(deadlines) == 0 {
		return []*fiscal.FiscalDeadline{}, nil
	}

	return deadlines, nil
}

// Removed getDefaultDeadlines logic since the UI should only display real deadlines from the database.

// ── DIMOB ──

// dimobAggregates reúne as contagens e os valores de locação e venda do ano.
//
// As duas consultas são a única fonte dos totais da declaração. Uma falha aqui
// não pode ser tratada como "não houve operações": os contadores voltam zerados,
// e publicar esse zero como READY produz uma DIMOB plausível e vazia. Por isso
// o erro é propagado e interrompe a geração.
func (s *FiscalService) dimobAggregates(ctx context.Context, tenantID string, year int) (rentals, sales int, amount float64, err error) {
	rentalCount, rentalAmount, err := s.repo.CountChargesByYear(ctx, tenantID, year, "RENT")
	if err != nil {
		return 0, 0, 0, fmt.Errorf("contar cobranças de locação de %d: %w", year, err)
	}
	saleCount, saleAmount, err := s.repo.CountChargesByYear(ctx, tenantID, year, "SALE")
	if err != nil {
		return 0, 0, 0, fmt.Errorf("contar cobranças de venda de %d: %w", year, err)
	}
	return rentalCount, saleCount, rentalAmount + saleAmount, nil
}

func (s *FiscalService) GetDimobStatus(ctx context.Context, tenantID string, year int) (*fiscal.DimobReport, error) {
	report, err := s.repo.GetDimobByYear(ctx, tenantID, year)
	if err != nil {
		return nil, err
	}
	if report == nil {
		// Return a pending report with aggregated data
		report = fiscal.NewDimobReport(tenantID, year)

		rentals, sales, amount, err := s.dimobAggregates(ctx, tenantID, year)
		if err != nil {
			return nil, err
		}

		report.TotalRentals = rentals
		report.TotalSales = sales
		report.TotalAmount = amount
	}
	return report, nil
}

func (s *FiscalService) GenerateDimob(ctx context.Context, tenantID string, year int, companyCNPJ, companyName string) (*fiscal.DimobReport, error) {
	// Check if report already exists
	existing, err := s.repo.GetDimobByYear(ctx, tenantID, year)
	if err != nil {
		return nil, err
	}

	// Falha de agregação interrompe antes de qualquer escrita: nenhum status
	// READY é persistido com totais que não vieram de consulta bem-sucedida.
	rentals, sales, amount, err := s.dimobAggregates(ctx, tenantID, year)
	if err != nil {
		return nil, err
	}

	// Dados da declarante (a imobiliária) para o registro R01 do TXT.
	cnpj := strings.TrimSpace(companyCNPJ)
	name := strings.TrimSpace(companyName)

	if existing != nil {
		existing.TotalRentals = rentals
		existing.TotalSales = sales
		existing.TotalAmount = amount
		// Só sobrescreve a declarante quando o caller informa um valor —
		// regerar a DIMOB sem reenviar o CNPJ preserva o que já está salvo.
		if cnpj != "" {
			existing.DeclaranteCNPJ = cnpj
		}
		if name != "" {
			existing.DeclaranteNome = name
		}
		now := time.Now()
		existing.GeneratedAt = &now
		existing.Status = fiscal.DimobStatusReady
		if err := s.repo.UpdateDimob(ctx, existing); err != nil {
			return nil, err
		}

		// Save to report history — o arquivo é o download por streaming do
		// endpoint de arquivo (path relativo ao gateway).
		history := fiscal.NewReportHistory(tenantID, "DIMOB", fmt.Sprintf("DIMOB %d", year),
			fmt.Sprintf("/api/v1/fiscal/dimob/%d/file", year))
		_ = s.repo.SaveReport(ctx, history)

		return existing, nil
	}

	report := fiscal.NewDimobReport(tenantID, year)
	report.TotalRentals = rentals
	report.TotalSales = sales
	report.TotalAmount = amount
	report.DeclaranteCNPJ = cnpj
	report.DeclaranteNome = name
	now := time.Now()
	report.GeneratedAt = &now
	report.Status = fiscal.DimobStatusReady

	// CreateDimob (INSERT) e não SaveDimob: SaveDimob faz UPDATE e um
	// relatório novo ainda não tem linha — o UPDATE não afetaria nada,
	// e o BuildDimobFile nunca encontraria a DIMOB depois.
	if err := s.repo.CreateDimob(ctx, report); err != nil {
		return nil, err
	}

	// Save to report history — o arquivo é o download por streaming do
	// endpoint de arquivo (path relativo ao gateway).
	history := fiscal.NewReportHistory(tenantID, "DIMOB", fmt.Sprintf("DIMOB %d", year),
		fmt.Sprintf("/api/v1/fiscal/dimob/%d/file", year))
	_ = s.repo.SaveReport(ctx, history)

	return report, nil
}

// ── NFS-e ──

func (s *FiscalService) ListNfse(ctx context.Context, tenantID, referenceMonth, status string, limit, offset int64) ([]*fiscal.Nfse, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.repo.ListNfse(ctx, tenantID, status, referenceMonth, limit, offset)
}

// IssueNfse emite a NFS-e do mês. Com provedor (P1-05) o fluxo é: grava a
// nota como rascunho → chama o provedor → sucesso preenche número da
// autoridade e links (danfe/xml) e marca ISSUED; falha marca ERROR e devolve
// o erro tipado para o operador corrigir/reemitir. Sem provedor (legado),
// a nota é emitida localmente como antes.
func (s *FiscalService) IssueNfse(ctx context.Context, tenantID, referenceMonth, recipientID string, amount float64, serviceDescription string) (*fiscal.Nfse, error) {
	nfse := fiscal.NewNfse(tenantID, referenceMonth, recipientID, amount, serviceDescription)

	if s.nfseProvider == nil {
		// Legado: sem provedor a nota nasce emitida localmente (mesma
		// semântica de antes do P1-05 — um único write, sem SaveNfse).
		nfse.Status = fiscal.NfseStatusIssued
		if err := s.repo.CreateNfse(ctx, nfse); err != nil {
			return nil, err
		}
	} else {
		// P1-05: rascunho → provedor → recibo (ISSUED) ou ERROR auditável.
		if err := s.repo.CreateNfse(ctx, nfse); err != nil {
			return nil, err
		}
		receipt, err := s.nfseProvider.IssueNfse(ctx, providers.NfseIssueInput{
			TenantID:           tenantID,
			ReferenceMonth:     referenceMonth,
			RecipientName:      recipientID,
			AmountCents:        int64(math.Round(amount * 100)),
			ServiceDescription: serviceDescription,
			IdempotencyKey:     nfse.ID,
		})
		if err != nil {
			nfse.Status = fiscal.NfseStatusError
			nfse.UpdatedAt = time.Now()
			if saveErr := s.repo.SaveNfse(ctx, nfse); saveErr != nil {
				return nil, fmt.Errorf("nfse provider failed (%v) and error state could not be saved: %w", err, saveErr)
			}
			history := fiscal.NewReportHistory(tenantID, "NFSE", fmt.Sprintf("NFS-e %s/%s — falha de emissão no provedor", referenceMonth, recipientID), err.Error())
			_ = s.repo.SaveReport(ctx, history)
			return nfse, fmt.Errorf("nfse provider rejected/unavailable for %s/%s: %w", referenceMonth, recipientID, err)
		}
		nfse.Number = receipt.AuthorityNumber
		nfse.PDFURL = receipt.AuthorityURL
		nfse.XMLURL = receipt.XMLURL
		if !receipt.IssuedAt.IsZero() {
			nfse.IssueDate = receipt.IssuedAt
		}
		nfse.Status = fiscal.NfseStatusIssued
		nfse.UpdatedAt = time.Now()
		if err := s.repo.SaveNfse(ctx, nfse); err != nil {
			return nil, fmt.Errorf("persist issued nfse receipt: %w", err)
		}
	}

	// Save to report history — o recibo do provedor traz o link da DANFE.
	history := fiscal.NewReportHistory(tenantID, "NFSE", fmt.Sprintf("NFS-e #%s - %s", nfse.Number, referenceMonth), nfse.PDFURL)
	_ = s.repo.SaveReport(ctx, history)

	return nfse, nil
}

// ── Informes ──

// ListInformes devolve os informes do ano e preenche, na leitura, o nome do
// proprietário que ficou vazio na geração. Motivo: a leitura interna do lead
// pelo CRM passava pelo pool de aplicação sob RLS — sem claims de JWT a linha
// existia mas era invisível, e o nome saía vazio. Corrigido na origem (pool de
// serviço), este backfill conserta os informes JÁ gerados sem exigir que o
// operador regrave tudo.
func (s *FiscalService) ListInformes(ctx context.Context, tenantID string, year int) ([]*fiscal.Informe, error) {
	informes, err := s.repo.ListInformes(ctx, tenantID, year)
	if err != nil {
		return nil, err
	}
	for _, inf := range informes {
		if inf == nil || strings.TrimSpace(inf.OwnerName) != "" || inf.OwnerID == "" {
			continue
		}
		name := s.resolveOwnerName(ctx, tenantID, inf.OwnerID)
		if name == "" {
			continue
		}
		inf.OwnerName = name
		if err := s.repo.UpdateInforme(ctx, inf); err != nil {
			logger.Warn("informe.owner_name_backfill_failed", "event", "informe.owner_name_backfill_failed", "informe_id", inf.ID, "owner_id", inf.OwnerID, "error", err.Error())
		}
	}
	return informes, nil
}

// GetInformeByID busca um informe de rendimentos por ID dentro do tenant.
func (s *FiscalService) GetInformeByID(ctx context.Context, tenantID, id string) (*fiscal.Informe, error) {
	return s.repo.GetInformeByID(ctx, tenantID, id)
}

// GenerateInformes monta os Informes de Rendimentos do ano — um por
// proprietário (#419). Para cada owner, agrega os repasses (OwnerTransfer)
// dos 12 meses: aluguel bruto, taxa de administração e valor líquido
// repassado. Repasses cancelados são ignorados.
//
// Regerar é idempotente: se o owner já tem informe no ano, o registro é
// atualizado no lugar; senão, um novo é criado. Exige WithTransferLister.
func (s *FiscalService) GenerateInformes(ctx context.Context, tenantID string, year int) ([]*fiscal.Informe, error) {
	if s.transfers == nil {
		return nil, fmt.Errorf("fonte de repasses indisponível para gerar informes")
	}

	byOwner := map[string]*informeOwnerAgg{}
	for month := 1; month <= 12; month++ {
		referenceMonth := fmt.Sprintf("%d-%02d", year, month)
		monthTransfers, err := s.listTransfersForMonth(ctx, tenantID, referenceMonth)
		if err != nil {
			return nil, err
		}
		for _, tr := range monthTransfers {
			if tr.Status == transfer.StatusCancelled || tr.OwnerID == "" {
				continue
			}
			agg := byOwner[tr.OwnerID]
			if agg == nil {
				agg = &informeOwnerAgg{}
				byOwner[tr.OwnerID] = agg
			}
			agg.gross += tr.GrossRent
			agg.adminFee += tr.AdminFeeAmount
			agg.net += tr.NetAmount
			// T-05: a intermediação imobiliária retida no repasse (comissão
			// já descontada da base do proprietário) entra como linha própria
			// do informe anual — a mesma leitura do demonstrativo mensal.
			intermediation := 0.0
			saleRevenue := 0.0
			if tr.EconomicBreakdown != nil {
				intermediation = float64(tr.EconomicBreakdown.IntermediationRevenueCents) / 100
				// Em venda o campo de intermediação vem zerado: a comissão da
				// imobiliária é a receita da agência no negócio.
				saleRevenue = float64(tr.EconomicBreakdown.AgencyRevenueCents) / 100
			}
			agg.intermediation += intermediation

			if transferFlow(tr) == "sale" {
				agg.saleGross += tr.GrossRent
				agg.saleNet += tr.NetAmount
				if intermediation > 0 {
					agg.saleIntermed += intermediation
				} else {
					agg.saleIntermed += saleRevenue
				}
				agg.saleCount++
				continue
			}
			agg.rentalGross += tr.GrossRent
			agg.rentalAdminFee += tr.AdminFeeAmount
			agg.rentalIntermed += intermediation
			agg.rentalNet += tr.NetAmount
			agg.rentalCount++
		}
	}

	// Indexa os informes já existentes do ano por owner — regerar
	// atualiza o registro no lugar em vez de duplicar.
	existing, err := s.repo.ListInformes(ctx, tenantID, year)
	if err != nil {
		return nil, err
	}
	existingByOwner := make(map[string]*fiscal.Informe, len(existing))
	for _, inf := range existing {
		existingByOwner[inf.OwnerID] = inf
	}

	// Ordena os owners para uma saída determinística.
	ownerIDs := make([]string, 0, len(byOwner))
	for id := range byOwner {
		ownerIDs = append(ownerIDs, id)
	}
	sort.Strings(ownerIDs)

	now := time.Now()
	out := make([]*fiscal.Informe, 0, len(ownerIDs))
	for _, ownerID := range ownerIDs {
		agg := byOwner[ownerID]
		ownerName := s.resolveOwnerName(ctx, tenantID, ownerID)

		if inf := existingByOwner[ownerID]; inf != nil {
			// Só sobrescreve o nome quando o CRM respondeu — não apagar
			// um nome bom por uma falha transitória de rede.
			if ownerName != "" {
				inf.OwnerName = ownerName
			}
			inf.TotalGross = agg.gross
			inf.TotalAdminFee = agg.adminFee
			inf.TotalIntermediation = agg.intermediation
			inf.TotalNet = agg.net
			applyFlowBreakdown(inf, agg)
			inf.Status = fiscal.InformeStatusReady
			inf.GeneratedAt = &now
			if err := s.repo.UpdateInforme(ctx, inf); err != nil {
				return nil, err
			}
			out = append(out, inf)
			continue
		}

		inf := fiscal.NewInforme(tenantID, year, ownerID, ownerName, agg.gross, agg.adminFee, agg.intermediation, agg.net)
		applyFlowBreakdown(inf, agg)
		inf.Status = fiscal.InformeStatusReady
		inf.GeneratedAt = &now
		if err := s.repo.CreateInforme(ctx, inf); err != nil {
			return nil, err
		}
		out = append(out, inf)
	}

	// Registra a geração no histórico de relatórios fiscais.
	history := fiscal.NewReportHistory(tenantID, "INFORME", fmt.Sprintf("Informes de Rendimentos %d", year), "")
	_ = s.repo.SaveReport(ctx, history)

	return out, nil
}

// informeOwnerAgg agrega os repasses do ano de um proprietário, separando
// locação (renda tributável do proprietário) de venda (ganho de capital, com a
// comissão de intermediação retida pela imobiliária).
type informeOwnerAgg struct {
	gross          float64
	adminFee       float64
	intermediation float64
	net            float64
	rentalGross    float64
	rentalAdminFee float64
	rentalIntermed float64
	rentalNet      float64
	rentalCount    int
	saleGross      float64
	saleIntermed   float64
	saleNet        float64
	saleCount      int
}

// applyFlowBreakdown copia os totais por fluxo para o informe.
func applyFlowBreakdown(inf *fiscal.Informe, agg *informeOwnerAgg) {
	if inf == nil || agg == nil {
		return
	}
	inf.RentalGross = agg.rentalGross
	inf.RentalAdminFee = agg.rentalAdminFee
	inf.RentalIntermediation = agg.rentalIntermed
	inf.RentalNet = agg.rentalNet
	inf.RentalCount = agg.rentalCount
	inf.SaleGross = agg.saleGross
	inf.SaleIntermediation = agg.saleIntermed
	inf.SaleNet = agg.saleNet
	inf.SaleCount = agg.saleCount
}

// transferFlow classifica o repasse em locação ("rent") ou venda ("sale").
// O SettlementType é a fonte primária; o fluxo do detalhamento econômico
// cobre linhas antigas gravadas antes do campo.
func transferFlow(tr *transfer.OwnerTransfer) string {
	if tr == nil {
		return "rent"
	}
	if flow := strings.ToLower(strings.TrimSpace(tr.SettlementType)); flow != "" {
		return flow
	}
	if tr.EconomicBreakdown != nil {
		if flow := strings.ToLower(strings.TrimSpace(tr.EconomicBreakdown.Flow)); flow != "" {
			return flow
		}
	}
	return "rent"
}

// resolveOwnerName devolve o nome de exibição do proprietário via CRM.
// Best-effort: sem cliente CRM ou em caso de erro, devolve "" — o
// informe segue válido, apenas sem o nome resolvido.
func (s *FiscalService) resolveOwnerName(ctx context.Context, tenantID, ownerID string) string {
	if s.crmClient == nil || ownerID == "" {
		return ""
	}
	lead, err := s.crmClient.GetLead(ctx, tenantID, ownerID)
	if err != nil || lead == nil {
		return ""
	}
	return lead.Name
}

// ── Report History ──

func (s *FiscalService) ListReports(ctx context.Context, tenantID string, filter fiscal.ReportHistoryFilter) ([]*fiscal.ReportHistory, int64, error) {
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Type != "" {
		filter.Type = strings.ToUpper(strings.TrimSpace(filter.Type))
	}
	return s.repo.ListReportHistory(ctx, tenantID, filter)
}

// ArchiveReport remove um registro do histórico (soft delete). O arquivo
// deixa de aparecer na listagem; a linha fica para auditoria.
func (s *FiscalService) ArchiveReport(ctx context.Context, tenantID, id string) error {
	return s.repo.ArchiveReport(ctx, tenantID, id)
}

func (s *FiscalService) GetReportByID(ctx context.Context, tenantID, id string) (*fiscal.ReportHistory, error) {
	return s.repo.GetReportByID(ctx, tenantID, id)
}

// ── DIMOB — geração do arquivo TXT (layout RFB, #418) ──

// BuildDimobFile monta o arquivo de importação da DIMOB no leiaute oficial
// do programa da Receita Federal (registros de largura fixa: Header, R01,
// R02 e T9) e devolve o nome de arquivo sugerido + o conteúdo pronto para
// download.
//
// As locações (R02) são reconstruídas a partir dos repasses (OwnerTransfer)
// do ano-base, mês a mês (regime de caixa — o mês do recebimento é o mês da
// declaração): aluguel bruto por mês e taxa de administração como comissão.
// Repasses cancelados são ignorados. O enriquecimento via toca-deal (número
// e data do contrato), toca-crm (CPF/CNPJ e nome de locador e locatário) e
// toca-core (endereço/CEP/UF do imóvel) preenche os campos que o programa
// valida — endereço/CEP ausentes BLOQUEIAM a importação na RFB, então a
// geração falha com a lista de contratos pendentes em vez de entregar um
// arquivo que será rejeitado.
//
// Exige que a DIMOB do ano já tenha sido gerada (GenerateDimob): é de lá
// que vêm o CNPJ e a razão social da declarante (registro R01).
//
// As advertências não bloqueantes (contrato sem número/data) voltam em
// warnings — o handler as expõe no header X-Dimob-Warnings.
func (s *FiscalService) BuildDimobFile(ctx context.Context, tenantID string, year int) (filename string, content string, warnings []string, err error) {
	if s.transfers == nil {
		// Erro de configuração (router não injetou o transferLister), não
		// erro do usuário — não embrulha em ErrDimobNotReady.
		return "", "", nil, fmt.Errorf("dimob: fonte de repasses indisponível para montar o arquivo")
	}

	report, err := s.repo.GetDimobByYear(ctx, tenantID, year)
	if err != nil {
		return "", "", nil, err
	}
	if report == nil {
		return "", "", nil, fmt.Errorf("%w: DIMOB %d ainda não foi gerada", ErrDimobNotReady, year)
	}
	if strings.TrimSpace(report.DeclaranteCNPJ) == "" {
		return "", "", nil, fmt.Errorf("%w: DIMOB %d foi gerada sem o CNPJ da declarante — gere a declaração novamente informando CNPJ e razão social", ErrDimobNotReady, year)
	}

	byContract := map[string]*dimobRentalAgg{}

	for month := 1; month <= 12; month++ {
		referenceMonth := fmt.Sprintf("%d-%02d", year, month)
		monthTransfers, err := s.listTransfersForMonth(ctx, tenantID, referenceMonth)
		if err != nil {
			return "", "", nil, err
		}
		for _, tr := range monthTransfers {
			// Repasse cancelado não representa aluguel efetivamente
			// recebido — fora da declaração. Repasses de venda (settlement
			// de intermediação) entram em outra ficha (R04), ainda não
			// emitida — ver docs/domain/dimob.md.
			if tr.Status == transfer.StatusCancelled {
				continue
			}
			if tr.SettlementType != "" && tr.SettlementType != "rent" {
				continue
			}
			agg := byContract[tr.ContractID]
			if agg == nil {
				agg = &dimobRentalAgg{}
				byContract[tr.ContractID] = agg
			}
			agg.monthlyRent[month-1] += tr.GrossRent
			agg.monthlyCommission[month-1] += tr.AdminFeeAmount
			if agg.ownerID == "" {
				agg.ownerID = tr.OwnerID
			}
			if agg.propertyID == "" {
				agg.propertyID = tr.PropertyID
			}
		}
	}

	if len(byContract) == 0 {
		return "", "", nil, fmt.Errorf("%w: nenhuma operação de locação no ano-base %d — a RFB não aceita declaração sem movimento", ErrDimobNotReady, year)
	}

	// Ordena os contratos para um TXT determinístico: a mesma entrada
	// produz sempre o mesmo arquivo, o que facilita conferência e testes.
	contractIDs := make([]string, 0, len(byContract))
	for id := range byContract {
		contractIDs = append(contractIDs, id)
	}
	sort.Strings(contractIDs)

	rentals := make([]dimob.Rental, 0, len(contractIDs))
	var pending []string
	for _, id := range contractIDs {
		agg := byContract[id]
		r := dimob.Rental{
			MonthlyRent:       agg.monthlyRent,
			MonthlyCommission: agg.monthlyCommission,
		}
		label := s.enrichRental(ctx, tenantID, id, agg, &r)
		if strings.TrimSpace(r.Endereco) == "" || strings.TrimSpace(r.CEP) == "" {
			pending = append(pending, fmt.Sprintf("%s: endereço/CEP do imóvel pendentes", label))
		}
		rentals = append(rentals, r)
	}

	declaration := dimob.Declaration{
		Declarante: dimob.Declarante{
			CNPJ:    report.DeclaranteCNPJ,
			Nome:    report.DeclaranteNome,
			AnoBase: year,
		},
		Rentals: rentals,
	}

	if len(pending) > 0 {
		return "", "", nil, fmt.Errorf("%w: %s — a RFB bloqueia a importação sem endereço/CEP; complete o cadastro do imóvel e baixe novamente", ErrDimobNotReady, strings.Join(pending, "; "))
	}

	result, err := dimob.Render(declaration)
	if err != nil {
		// Declaração inconsistente (ex.: documento torto) — erro do usuário.
		if errors.Is(err, dimob.ErrInvalidDeclaration) {
			return "", "", nil, fmt.Errorf("%w: %v", ErrDimobNotReady, err)
		}
		return "", "", nil, err
	}

	return fmt.Sprintf("dimob-%d.txt", year), result.Content, result.Warnings, nil
}

// enrichRental preenche os campos cadastrais da locação a partir dos
// serviços de origem: contrato (toca-deal), locador e locatário (toca-crm)
// e imóvel (toca-core). Falhas de enriquecimento não derrubam a geração:
// os campos ficam vazios e a validação posterior orienta o operador
// (endereço/CEP são bloqueantes; contrato/data são advertências). Devolve
// um rótulo amigável (imóvel/contrato) para as mensagens de pendência.
func (s *FiscalService) enrichRental(ctx context.Context, tenantID, contractID string, agg *dimobRentalAgg, r *dimob.Rental) string {
	label := "contrato " + contractID
	var ctr *clients.ContractDTO
	if s.dealClient != nil {
		var ctrErr error
		ctr, ctrErr = s.dealClient.GetContract(ctx, tenantID, contractID)
		if ctrErr != nil {
			logger.Warn("dimob.enrichment_failed", "event", "dimob.enrichment_failed", "contract_id", contractID, "source", "deal", "error", ctrErr.Error())
		}
		if ctrErr == nil && ctr != nil {
			r.Contrato = onlyContractNumber(ctr.DisplayID)
			if strings.TrimSpace(ctr.DisplayID) != "" {
				label = "contrato " + ctr.DisplayID
			}
			if strings.TrimSpace(ctr.PropertyTitle) != "" {
				label = "contrato " + ctr.DisplayID + " (" + ctr.PropertyTitle + ")"
			}
			if !ctr.StartDate.IsZero() {
				// O leiaute da DIMOB escreve data em DDMMAAAA — não AAAAMMDD.
				r.DataContrato = ctr.StartDate.Format("02012006")
			}
			if agg.propertyID == "" {
				agg.propertyID = ctr.PropertyID
			}
			if ctr.DealID != "" && s.crmClient != nil {
				if deal, derr := s.dealClient.GetDeal(ctx, tenantID, ctr.DealID); derr == nil && deal != nil {
					if lead, lerr := s.crmClient.GetLead(ctx, tenantID, deal.CustomerID); lerr == nil && lead != nil {
						r.LocatarioDoc = lead.Documentation
						r.LocatarioNome = lead.Name
					}
				}
			}
		}
	}
	if s.crmClient != nil && agg.ownerID != "" {
		lead, leadErr := s.crmClient.GetLead(ctx, tenantID, agg.ownerID)
		if leadErr != nil {
			logger.Warn("dimob.enrichment_failed", "event", "dimob.enrichment_failed", "contract_id", contractID, "source", "crm-locador", "error", leadErr.Error())
		}
		if leadErr == nil && lead != nil {
			r.LocadorDoc = lead.Documentation
			r.LocadorNome = lead.Name
		}
	}
	// Fallback: signatários do contrato (locador/locatário) quando o CRM
	// não resolve o lead — garante ao menos o NOME das partes no R02.
	s.fillPartiesFromSignatories(ctr, r)
	if s.propertyClient != nil && agg.propertyID != "" {
		prop, propErr := s.propertyClient.GetProperty(ctx, tenantID, agg.propertyID)
		if propErr != nil {
			logger.Warn("dimob.enrichment_failed", "event", "dimob.enrichment_failed", "contract_id", contractID, "property_id", agg.propertyID, "source", "core-property", "error", propErr.Error())
		}
		if propErr == nil && prop != nil {
			r.Endereco = strings.TrimSpace(prop.Address.Street + " " + prop.Address.Number)
			r.CEP = prop.Address.PostalCode
			r.UF = prop.Address.StateCode
			if strings.TrimSpace(prop.Title) != "" {
				label = "imóvel " + prop.Title
			}
		}
	}
	return label
}

// dimobRentalAgg agrega os repasses do ano-base por contrato de locação:
// 12 valores mensais de aluguel bruto e de comissão (regime de caixa), mais
// o proprietário (locador) e o imóvel do primeiro repasse para enriquecer o
// registro R02.
type dimobRentalAgg struct {
	monthlyRent       [12]float64
	monthlyCommission [12]float64
	ownerID           string
	propertyID        string
}

// fillPartiesFromSignatories usa os signatários do contrato como fallback
// para locador/locatário quando o CRM não resolveu os leads: preenche nome e
// documento pelos papéis "locador"/"proprietario" e "locatario"/"inquilino".
func (s *FiscalService) fillPartiesFromSignatories(ctr *clients.ContractDTO, r *dimob.Rental) {
	if ctr == nil {
		return
	}
	for _, sig := range ctr.Signatories {
		role := strings.ToLower(strings.TrimSpace(sig.Role))
		switch {
		case r.LocadorNome == "" && (role == "locador" || role == "proprietario"):
			r.LocadorNome = sig.Name
			if r.LocadorDoc == "" {
				r.LocadorDoc = sig.Documentation
			}
		case r.LocatarioNome == "" && (role == "locatario" || role == "inquilino"):
			r.LocatarioNome = sig.Name
			if r.LocatarioDoc == "" {
				r.LocatarioDoc = sig.Documentation
			}
		}
	}
}

// onlyContractNumber extrai a porção numérica do número do contrato
// (o leiaute aceita até 6 dígitos — ex.: "CTR-0001" → "1").
func onlyContractNumber(displayID string) string {
	var b strings.Builder
	for _, r := range displayID {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// listTransfersForMonth devolve todos os repasses de um mês de
// referência, paginando internamente — a lista de um tenant pode passar
// do tamanho de uma página.
func (s *FiscalService) listTransfersForMonth(ctx context.Context, tenantID, referenceMonth string) ([]*transfer.OwnerTransfer, error) {
	const pageSize = 200
	var all []*transfer.OwnerTransfer
	var offset int64
	for {
		page, total, err := s.transfers.List(ctx, tenantID, "", "", "", referenceMonth, pageSize, offset)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		offset += int64(len(page))
		// Página curta = última página; offset alcançou o total = fim.
		if len(page) < pageSize || offset >= total {
			break
		}
	}
	return all, nil
}
