package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/clients"

	"github.com/oginoino/toca/toca-financial/internal/dimob"
	"github.com/oginoino/toca/toca-financial/internal/domain/fiscal"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	"github.com/oginoino/toca/toca-financial/internal/providers"
)

// fiscalRepoStub é um FiscalRepository de teste. GetDimobByYear devolve um
// report configurável; SaveDimob/UpdateDimob capturam o último argumento
// para asserção. O resto é no-op — BuildDimobFile e GenerateDimob não
// exercitam os demais métodos.
type fiscalRepoStub struct {
	dimobByYear  *fiscal.DimobReport
	createdDimob *fiscal.DimobReport
	savedDimob   *fiscal.DimobReport
	updatedDimob *fiscal.DimobReport

	createdNfse        *fiscal.Nfse
	savedNfse          *fiscal.Nfse
	listNfseStatus     string
	listNfseMonth      string
	reportByID         *fiscal.ReportHistory
	reportHistory      []*fiscal.ReportHistory
	listReportsFilter  fiscal.ReportHistoryFilter
	archivedReportID   string
	savedReport        *fiscal.ReportHistory
	reportLookupTenant string
	reportLookupID     string

	existingInformes []*fiscal.Informe
	createdInformes  []*fiscal.Informe
	updatedInformes  []*fiscal.Informe

	// countChargesErr injeta falha de agregação por tipo ("RENT"/"SALE").
	// countChargesByType devolve contagem/valor por tipo.
	countChargesErr    map[string]error
	countChargesByType map[string]struct {
		count  int
		amount float64
	}
}

func (f *fiscalRepoStub) CreateDimob(ctx context.Context, d *fiscal.DimobReport) error {
	f.createdDimob = d
	return nil
}
func (f *fiscalRepoStub) GetDimobByID(ctx context.Context, tenantID, id string) (*fiscal.DimobReport, error) {
	return nil, nil
}
func (f *fiscalRepoStub) ListDimob(ctx context.Context, tenantID string) ([]*fiscal.DimobReport, error) {
	return nil, nil
}
func (f *fiscalRepoStub) GetDimobByYear(ctx context.Context, tenantID string, year int) (*fiscal.DimobReport, error) {
	return f.dimobByYear, nil
}
func (f *fiscalRepoStub) UpdateDimob(ctx context.Context, d *fiscal.DimobReport) error {
	f.updatedDimob = d
	return nil
}
func (f *fiscalRepoStub) SaveDimob(ctx context.Context, d *fiscal.DimobReport) error {
	f.savedDimob = d
	return nil
}
func (f *fiscalRepoStub) CreateNfse(ctx context.Context, n *fiscal.Nfse) error {
	f.createdNfse = n
	return nil
}
func (f *fiscalRepoStub) SaveNfse(ctx context.Context, n *fiscal.Nfse) error {
	f.savedNfse = n
	return nil
}
func (f *fiscalRepoStub) ListNfse(ctx context.Context, tenantID, status, referenceMonth string, limit, offset int64) ([]*fiscal.Nfse, int64, error) {
	f.listNfseStatus = status
	f.listNfseMonth = referenceMonth
	return nil, 0, nil
}
func (f *fiscalRepoStub) CreateInforme(ctx context.Context, i *fiscal.Informe) error {
	f.createdInformes = append(f.createdInformes, i)
	return nil
}
func (f *fiscalRepoStub) UpdateInforme(ctx context.Context, i *fiscal.Informe) error {
	f.updatedInformes = append(f.updatedInformes, i)
	return nil
}
func (f *fiscalRepoStub) SaveInforme(ctx context.Context, i *fiscal.Informe) error { return nil }
func (f *fiscalRepoStub) ListInformes(ctx context.Context, tenantID string, year int) ([]*fiscal.Informe, error) {
	return f.existingInformes, nil
}
func (f *fiscalRepoStub) GetInformeByID(ctx context.Context, tenantID, id string) (*fiscal.Informe, error) {
	for _, inf := range f.existingInformes {
		if inf.ID == id {
			return inf, nil
		}
	}
	return nil, nil
}
func (f *fiscalRepoStub) ListDeadlines(ctx context.Context, tenantID string) ([]*fiscal.FiscalDeadline, error) {
	return nil, nil
}
func (f *fiscalRepoStub) GetCalendar(ctx context.Context, tenantID string, year int) ([]*fiscal.FiscalDeadline, error) {
	return nil, nil
}
func (f *fiscalRepoStub) AddReportHistory(ctx context.Context, h *fiscal.ReportHistory) error {
	return nil
}
func (f *fiscalRepoStub) SaveReport(ctx context.Context, h *fiscal.ReportHistory) error {
	f.savedReport = h
	return nil
}
func (f *fiscalRepoStub) ListReportHistory(ctx context.Context, tenantID string, filter fiscal.ReportHistoryFilter) ([]*fiscal.ReportHistory, int64, error) {
	f.listReportsFilter = filter
	return f.reportHistory, int64(len(f.reportHistory)), nil
}
func (f *fiscalRepoStub) ArchiveReport(ctx context.Context, tenantID, id string) error {
	f.archivedReportID = id
	return nil
}
func (f *fiscalRepoStub) GetReportByID(ctx context.Context, tenantID, id string) (*fiscal.ReportHistory, error) {
	f.reportLookupTenant = tenantID
	f.reportLookupID = id
	return f.reportByID, nil
}
func (f *fiscalRepoStub) CountChargesByYear(ctx context.Context, tenantID string, year int, t string) (int, float64, error) {
	// Injeção de falha por tipo: sem isso o caminho de erro de
	// GetDimobStatus/GenerateDimob é inalcançável em teste.
	if err, ok := f.countChargesErr[t]; ok && err != nil {
		return 0, 0, err
	}
	if agg, ok := f.countChargesByType[t]; ok {
		return agg.count, agg.amount, nil
	}
	return 0, 0, nil
}

// transferListerStub devolve repasses pré-carregados por mês de referência
// ("YYYY-MM"), respeitando limit/offset para exercitar a paginação de
// listTransfersForMonth.
type transferListerStub struct {
	byMonth map[string][]*transfer.OwnerTransfer
}

func (s *transferListerStub) List(ctx context.Context, tenantID, contractID, ownerID, status, referenceMonth string, limit, offset int64) ([]*transfer.OwnerTransfer, int64, error) {
	all := s.byMonth[referenceMonth]
	total := int64(len(all))
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func newDimobReportWithDeclarante(year int) *fiscal.DimobReport {
	r := fiscal.NewDimobReport("tenant-1", year)
	r.DeclaranteCNPJ = "11.222.333/0001-81"
	r.DeclaranteNome = "Imobiliária Modelo Ltda"
	r.Status = fiscal.DimobStatusReady
	return r
}

func TestIssueNfse_PersistsNewDocumentViaCreate(t *testing.T) {
	repo := &fiscalRepoStub{}
	svc := NewFiscalService(repo)

	got, err := svc.IssueNfse(
		context.Background(),
		"tenant-1",
		"2026-07",
		"recipient-1",
		1250.50,
		"Intermediação imobiliária",
	)
	if err != nil {
		t.Fatalf("IssueNfse: %v", err)
	}
	if repo.createdNfse == nil {
		t.Fatal("new NFS-e must be persisted with CreateNfse")
	}
	if repo.savedNfse != nil {
		t.Fatal("new NFS-e must not use the update-only SaveNfse path")
	}
	if got != repo.createdNfse {
		t.Fatal("service must return the persisted NFS-e instance")
	}
	if got.Status != fiscal.NfseStatusIssued {
		t.Fatalf("status = %q, want %q", got.Status, fiscal.NfseStatusIssued)
	}
}

func TestListNfse_MapsStatusAndReferenceMonthToRepository(t *testing.T) {
	repo := &fiscalRepoStub{}
	svc := NewFiscalService(repo)

	if _, _, err := svc.ListNfse(context.Background(), "tenant-1", "2026-07", "ISSUED", 20, 0); err != nil {
		t.Fatalf("ListNfse: %v", err)
	}
	if repo.listNfseStatus != "ISSUED" {
		t.Fatalf("repository status = %q, want ISSUED", repo.listNfseStatus)
	}
	if repo.listNfseMonth != "2026-07" {
		t.Fatalf("repository reference month = %q, want 2026-07", repo.listNfseMonth)
	}
}

func TestGetReportByID_PropagatesTenantScope(t *testing.T) {
	report := fiscal.NewReportHistory("tenant-1", "DIMOB", "DIMOB 2025", "")
	repo := &fiscalRepoStub{reportByID: report}
	svc := NewFiscalService(repo)

	got, err := svc.GetReportByID(context.Background(), "tenant-1", report.ID)
	if err != nil {
		t.Fatalf("GetReportByID: %v", err)
	}
	if got != report {
		t.Fatal("service must return the repository report")
	}
	if repo.reportLookupTenant != "tenant-1" {
		t.Fatalf("repository tenant = %q, want tenant-1", repo.reportLookupTenant)
	}
	if repo.reportLookupID != report.ID {
		t.Fatalf("repository report id = %q, want %q", repo.reportLookupID, report.ID)
	}
}

// dimobEnv é um ambiente de enriquecimento completo (toca-deal, toca-crm e
// toca-core falsos) para os testes do BuildDimobFile.
type dimobEnv struct {
	dealClient     *clients.DealClient
	crmClient      *clients.CrmClient
	propertyClient *clients.PropertyClient
	dealServer     *httptest.Server
	crmServer      *httptest.Server
	coreServer     *httptest.Server
}

// newDimobEnv sobe três servidores falsos e devolve os clientes apontando
// para eles. Um id de contrato/conhecido devolve dados completos; qualquer
// outro id devolve 404 (permite testar pendências).
func newDimobEnv(t *testing.T) *dimobEnv {
	t.Helper()
	t.Setenv("INTERNAL_API_KEY", "test-key")

	dealServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/deal/internal/contracts/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/deal/internal/contracts/")
			if id == "missing" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": clients.ContractDTO{
					ID:        id,
					DisplayID: "CTR-0001",
					DealID:    "deal-" + id,
					StartDate: time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC),
					Type:      "locacao",
					Signatories: []clients.ContractSignatoryDTO{
						{ID: "sig-1", Name: "Ayla Mizuke", Role: "locador", Documentation: "111.222.333-44"},
						{ID: "sig-2", Name: "Luana do Crato", Role: "locatario"},
					},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/api/v1/deal/internal/deals/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/deal/internal/deals/")
			customerID := "lead-customer"
			if id == "deal-ctr-X" {
				customerID = "lead-ausente"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": clients.DealDTO{
					ID:         id,
					CustomerID: customerID,
					OwnerID:    "lead-owner",
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	crmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/crm/internal/leads/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/crm/internal/leads/")
		if id == "lead-owner" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": clients.LeadDTO{ID: id, Name: "Fábio Ramos", Documentation: "123.456.789-09"},
			})
			return
		}
		if id == "lead-customer" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": clients.LeadDTO{ID: id, Name: "Carla Menezes", Documentation: "987.654.321-00"},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	coreServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/core/internal/properties/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Replica a exigência do middleware global do toca-core: sem
		// X-Tenant-Id a chamada interna responde 401 (regressão do download
		// 400 da DIMOB no beta — o cliente não enviava o header).
		if r.Header.Get("X-Tenant-Id") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"missing X-Tenant-Id"}`))
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/core/internal/properties/")
		if id == "missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": clients.PropertyAddressDTO{
				ID:    id,
				Title: "Apartamento Vila Mariana",
				Address: struct {
					Street       string `json:"street"`
					Number       string `json:"number"`
					Complement   string `json:"complement"`
					Neighborhood string `json:"neighborhood"`
					City         string `json:"city"`
					StateCode    string `json:"stateCode"`
					PostalCode   string `json:"postalCode"`
					Formatted    string `json:"formatted_address"`
				}{Street: "Rua Vitória Régia", Number: "152", City: "São Paulo", StateCode: "SP", PostalCode: "14032-000"},
			},
		})
	}))

	return &dimobEnv{
		dealClient:     &clients.DealClient{BaseClient: clients.NewBaseClient(dealServer.URL)},
		crmClient:      &clients.CrmClient{BaseClient: clients.NewBaseClient(crmServer.URL)},
		propertyClient: &clients.PropertyClient{BaseClient: clients.NewBaseClient(coreServer.URL)},
		dealServer:     dealServer,
		crmServer:      crmServer,
		coreServer:     coreServer,
	}
}

func (e *dimobEnv) Close() {
	e.dealServer.Close()
	e.crmServer.Close()
	e.coreServer.Close()
}

func (e *dimobEnv) service(repo *fiscalRepoStub, transfers *transferListerStub) *FiscalService {
	return NewFiscalService(repo).
		WithTransferLister(transfers).
		WithCrmClient(e.crmClient).
		WithDealClient(e.dealClient).
		WithPropertyClient(e.propertyClient)
}

func TestBuildDimobFile_Success(t *testing.T) {
	env := newDimobEnv(t)
	defer env.Close()
	repo := &fiscalRepoStub{dimobByYear: newDimobReportWithDeclarante(2025)}
	transfers := &transferListerStub{byMonth: map[string][]*transfer.OwnerTransfer{
		"2025-01": {{ContractID: "ctr-1", OwnerID: "lead-owner", PropertyID: "prop-1", GrossRent: 1500, AdminFeeAmount: 150, Status: transfer.StatusProcessed}},
		"2025-02": {{ContractID: "ctr-1", OwnerID: "lead-owner", PropertyID: "prop-1", GrossRent: 1500, AdminFeeAmount: 150, Status: transfer.StatusProcessed}},
	}}
	svc := env.service(repo, transfers)

	filename, content, warnings, err := svc.BuildDimobFile(context.Background(), "tenant-1", 2025)
	if err != nil {
		t.Fatalf("BuildDimobFile: %v", err)
	}
	if filename != "dimob-2025.txt" {
		t.Errorf("filename = %q, want dimob-2025.txt", filename)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings inesperados: %v", warnings)
	}

	lines := strings.Split(strings.TrimSuffix(content, "\r\n"), "\r\n")
	if len(lines) != 4 { // header + R01 + 1×R02 + T9
		t.Fatalf("esperava 4 linhas, veio %d", len(lines))
	}
	// Header: "DIMOB" e o resto reservado em branco (largura oficial de 374).
	if lines[0] != "DIMOB"+strings.Repeat(" ", 369) {
		t.Errorf("header fora do leiaute: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "R01") || len(lines[1]) != 270 {
		t.Errorf("linha 1 deveria ser o R01 de 270 posições: %q", lines[1][:10])
	}
	// R02 no leiaute: 797 posições, sequencial de 5 e data em DDMMAAAA.
	if len(lines[2]) != 797 {
		t.Errorf("R02 com %d posições, esperado 797", len(lines[2]))
	}
	if !strings.HasPrefix(lines[2], "R02") {
		t.Errorf("linha 2 deveria ser R02: %q", lines[2][:10])
	}
	if lines[2][21:26] != "00001" {
		t.Errorf("sequencial da locação = %q", lines[2][21:26])
	}
	if strings.TrimSpace(lines[2][40:100]) != "FABIO RAMOS" {
		t.Errorf("locador = %q", lines[2][40:100])
	}
	if strings.TrimSpace(lines[2][114:174]) != "CARLA MENEZES" {
		t.Errorf("locatária = %q", lines[2][114:174])
	}
	if lines[2][174:180] != "000001" {
		t.Errorf("número do contrato = %q", lines[2][174:180])
	}
	if lines[2][180:188] != "10012025" {
		t.Errorf("data do contrato (DDMMAAAA) = %q", lines[2][180:188])
	}
	if lines[2][188:202] != "00000000150000" {
		t.Errorf("aluguel de janeiro = %q", lines[2][188:202])
	}
	if lines[2][202:216] != "00000000015000" {
		t.Errorf("comissão de janeiro = %q", lines[2][202:216])
	}
	if lines[2][753:761] != "14032000" {
		t.Errorf("CEP = %q", lines[2][753:761])
	}
	// T9: "T9" + 100 posições reservadas — sem CNPJ e sem contagem de linhas.
	if lines[3] != "T9"+strings.Repeat(" ", 100) {
		t.Errorf("T9 fora do leiaute: %q", lines[3])
	}

	// O arquivo entregue ao operador passa pela mesma conferência posicional
	// que o programa da RFB aplica na importação.
	report := dimob.ValidateFile(content)
	if report.HasErrors() {
		t.Errorf("arquivo gerado deveria ser conforme, veio: %v", report.Errors())
	}
}

func TestBuildDimobFile_NoReport(t *testing.T) {
	env := newDimobEnv(t)
	defer env.Close()
	repo := &fiscalRepoStub{dimobByYear: nil}
	svc := env.service(repo, &transferListerStub{})
	if _, _, _, err := svc.BuildDimobFile(context.Background(), "tenant-1", 2025); !errors.Is(err, ErrDimobNotReady) {
		t.Errorf("esperava ErrDimobNotReady para DIMOB não gerada, veio %v", err)
	}
}

func TestBuildDimobFile_MissingCNPJ(t *testing.T) {
	env := newDimobEnv(t)
	defer env.Close()
	r := fiscal.NewDimobReport("tenant-1", 2025)
	r.DeclaranteNome = "Imobiliária Modelo Ltda" // sem CNPJ
	repo := &fiscalRepoStub{dimobByYear: r}
	svc := env.service(repo, &transferListerStub{})
	if _, _, _, err := svc.BuildDimobFile(context.Background(), "tenant-1", 2025); !errors.Is(err, ErrDimobNotReady) {
		t.Errorf("esperava ErrDimobNotReady para DIMOB sem CNPJ, veio %v", err)
	}
}

func TestBuildDimobFile_NoTransferLister(t *testing.T) {
	repo := &fiscalRepoStub{dimobByYear: newDimobReportWithDeclarante(2025)}
	svc := NewFiscalService(repo) // sem WithTransferLister
	_, _, _, err := svc.BuildDimobFile(context.Background(), "tenant-1", 2025)
	if err == nil {
		t.Fatal("esperava erro quando o transferLister é nil")
	}
	if errors.Is(err, ErrDimobNotReady) {
		t.Errorf("transferLister nil é erro de configuração (500), não ErrDimobNotReady: %v", err)
	}
}

func TestBuildDimobFile_SkipsCancelledAndSales(t *testing.T) {
	env := newDimobEnv(t)
	defer env.Close()
	repo := &fiscalRepoStub{dimobByYear: newDimobReportWithDeclarante(2025)}
	transfers := &transferListerStub{byMonth: map[string][]*transfer.OwnerTransfer{
		"2025-03": {
			{ContractID: "ctr-A", OwnerID: "lead-owner", PropertyID: "prop-1", GrossRent: 2000, AdminFeeAmount: 200, Status: transfer.StatusProcessed},
			{ContractID: "ctr-B", OwnerID: "lead-owner", PropertyID: "prop-1", GrossRent: 999, AdminFeeAmount: 99, Status: transfer.StatusCancelled},
			// Repasse de venda (intermediação) vai para a ficha R04 — ainda não emitida.
			{ContractID: "ctr-C", OwnerID: "lead-owner", PropertyID: "prop-1", SettlementType: "sale", GrossRent: 9000, AdminFeeAmount: 900, Status: transfer.StatusProcessed},
		},
	}}
	svc := env.service(repo, transfers)
	_, content, _, err := svc.BuildDimobFile(context.Background(), "tenant-1", 2025)
	if err != nil {
		t.Fatalf("BuildDimobFile: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(content, "\r\n"), "\r\n")
	if len(lines) != 4 { // só ctr-A: header + R01 + 1×R02 + T9
		t.Fatalf("cancelado/venda deveriam ser ignorados, linhas = %d", len(lines))
	}
	if strings.Contains(content, "00000000099900") {
		t.Error("repasse cancelado não deveria aparecer no arquivo")
	}
}

func TestBuildDimobFile_EmptyYearBlocks(t *testing.T) {
	env := newDimobEnv(t)
	defer env.Close()
	repo := &fiscalRepoStub{dimobByYear: newDimobReportWithDeclarante(2025)}
	svc := env.service(repo, &transferListerStub{byMonth: map[string][]*transfer.OwnerTransfer{}})
	_, _, _, err := svc.BuildDimobFile(context.Background(), "tenant-1", 2025)
	if !errors.Is(err, ErrDimobNotReady) {
		t.Fatalf("esperava ErrDimobNotReady para ano sem movimento, veio %v", err)
	}
	if !strings.Contains(err.Error(), "sem movimento") {
		t.Errorf("mensagem deveria citar sem movimento: %v", err)
	}
}

func TestBuildDimobFile_MissingPropertyBlocksWithContract(t *testing.T) {
	env := newDimobEnv(t)
	defer env.Close()
	repo := &fiscalRepoStub{dimobByYear: newDimobReportWithDeclarante(2025)}
	transfers := &transferListerStub{byMonth: map[string][]*transfer.OwnerTransfer{
		"2025-04": {{ContractID: "ctr-X", OwnerID: "lead-owner", PropertyID: "missing", GrossRent: 1000, AdminFeeAmount: 100, Status: transfer.StatusProcessed}},
	}}
	svc := env.service(repo, transfers)
	_, _, _, err := svc.BuildDimobFile(context.Background(), "tenant-1", 2025)
	if !errors.Is(err, ErrDimobNotReady) {
		t.Fatalf("esperava ErrDimobNotReady para endereço/CEP pendentes, veio %v", err)
	}
	if !strings.Contains(err.Error(), "CTR-0001") {
		t.Errorf("mensagem deveria citar o contrato pendente: %v", err)
	}
}

func TestBuildDimobFile_SignatoryFallbackForMissingLead(t *testing.T) {
	env := newDimobEnv(t)
	defer env.Close()
	repo := &fiscalRepoStub{dimobByYear: newDimobReportWithDeclarante(2025)}
	// ownerID desconhecido no CRM fake -> o nome/documento do locador vêm
	// dos signatários do contrato (papel "locador").
	transfers := &transferListerStub{byMonth: map[string][]*transfer.OwnerTransfer{
		"2025-05": {{ContractID: "ctr-X", OwnerID: "lead-ausente", PropertyID: "prop-1", GrossRent: 1000, AdminFeeAmount: 100, Status: transfer.StatusProcessed}},
	}}
	svc := env.service(repo, transfers)
	_, content, _, err := svc.BuildDimobFile(context.Background(), "tenant-1", 2025)
	if err != nil {
		t.Fatalf("BuildDimobFile: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(content, "\r\n"), "\r\n")
	if !strings.Contains(lines[2], "AYLA MIZUKE") {
		t.Errorf("locador deveria vir do signatário: %q", lines[2][42:102])
	}
	if !strings.Contains(lines[2], "LUANA DO CRATO") {
		t.Errorf("locatário deveria vir do signatário: %q", lines[2][116:176])
	}
	if !strings.Contains(lines[2], "11122233344") {
		t.Errorf("documento do locador deveria vir do signatário: %q", lines[2][28:42])
	}
}

func TestBuildDimobFile_WarningsForMissingContractInfo(t *testing.T) {
	env := newDimobEnv(t)
	defer env.Close()
	repo := &fiscalRepoStub{dimobByYear: newDimobReportWithDeclarante(2025)}
	// Contrato "missing" no toca-deal → sem número/data do contrato (advertência).
	transfers := &transferListerStub{byMonth: map[string][]*transfer.OwnerTransfer{
		"2025-05": {{ContractID: "missing", OwnerID: "lead-owner", PropertyID: "prop-1", GrossRent: 1000, AdminFeeAmount: 100, Status: transfer.StatusProcessed}},
	}}
	svc := env.service(repo, transfers)
	_, content, warnings, err := svc.BuildDimobFile(context.Background(), "tenant-1", 2025)
	if err != nil {
		t.Fatalf("BuildDimobFile: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("esperava advertências para contrato sem número/data")
	}
	if !strings.Contains(strings.Join(warnings, " "), "número de contrato") {
		t.Errorf("advertências deveriam citar número do contrato: %v", warnings)
	}
	if strings.Split(strings.TrimSuffix(content, "\r\n"), "\r\n")[2][176:182] != "000000" {
		t.Error("contrato ausente deveria renderizar zeros")
	}
}

// Regressão do informe sem proprietário: informes gravados com nome vazio
// (leitura interna do lead devolvia 404 sob RLS) são preenchidos na leitura
// pelo CRM e persistidos.
// T-06: a geração separa locação de venda e expõe a comissão de
// intermediação de venda (que vem da receita da agência no negócio, já que o
// campo de intermediação do detalhamento é zerado em vendas).
func TestGenerateInformes_SeparatesRentalAndSale(t *testing.T) {
	env := newDimobEnv(t)
	defer env.Close()
	repo := &fiscalRepoStub{}
	transfers := &transferListerStub{byMonth: map[string][]*transfer.OwnerTransfer{
		"2025-03": {
			{
				ContractID: "ctr-rent", OwnerID: "lead-customer", PropertyID: "prop-1",
				SettlementType: "rent", GrossRent: 2800, AdminFeeAmount: 280, NetAmount: 2158.72,
				EconomicBreakdown: &transfer.EconomicBreakdown{Flow: "rent", IntermediationRevenueCents: 36128},
			},
			{
				ContractID: "ctr-sale", OwnerID: "lead-customer", PropertyID: "prop-1",
				SettlementType: "sale", GrossRent: 1400000, NetAmount: 1316000,
				EconomicBreakdown: &transfer.EconomicBreakdown{Flow: "sale", AgencyRevenueCents: 8400000},
			},
		},
	}}
	svc := env.service(repo, transfers)

	informes, err := svc.GenerateInformes(context.Background(), "tenant-1", 2025)
	if err != nil {
		t.Fatalf("GenerateInformes: %v", err)
	}
	if len(informes) != 1 {
		t.Fatalf("esperava 1 informe, veio %d", len(informes))
	}
	inf := informes[0]
	if inf.RentalGross != 2800 || inf.RentalAdminFee != 280 || inf.RentalNet != 2158.72 || inf.RentalCount != 1 {
		t.Errorf("bloco de locação incorreto: %+v", inf)
	}
	if inf.RentalIntermediation != 361.28 {
		t.Errorf("intermediação de locação = %v, esperado 361.28", inf.RentalIntermediation)
	}
	if inf.SaleGross != 1400000 || inf.SaleNet != 1316000 || inf.SaleCount != 1 {
		t.Errorf("bloco de venda incorreto: %+v", inf)
	}
	if inf.SaleIntermediation != 84000 {
		t.Errorf("intermediação de venda = %v, esperado 84000", inf.SaleIntermediation)
	}
	// Totais continuam sendo a soma dos fluxos.
	if inf.TotalGross != 1402800 || inf.TotalNet != 1318158.72 {
		t.Errorf("totais = %v/%v", inf.TotalGross, inf.TotalNet)
	}
}

func TestListInformes_BackfillsMissingOwnerName(t *testing.T) {
	env := newDimobEnv(t)
	defer env.Close()

	informe := fiscal.NewInforme("tenant-1", 2025, "lead-customer", "", 100, 10, 0, 90)
	informe.OwnerName = "" // gerado quando o nome não resolvia
	repo := &fiscalRepoStub{existingInformes: []*fiscal.Informe{informe}}
	svc := env.service(repo, &transferListerStub{})

	informes, err := svc.ListInformes(context.Background(), "tenant-1", 2025)
	if err != nil {
		t.Fatalf("ListInformes: %v", err)
	}
	if len(informes) != 1 {
		t.Fatalf("esperava 1 informe, veio %d", len(informes))
	}
	if informes[0].OwnerName != "Carla Menezes" {
		t.Errorf("nome do proprietário = %q, esperado Carla Menezes", informes[0].OwnerName)
	}
	if len(repo.updatedInformes) != 1 {
		t.Errorf("o nome deveria ser persistido (UpdateInforme), atualizações = %d", len(repo.updatedInformes))
	}
}

// Quando o CRM não tem o lead, o informe permanece sem nome (não inventa).
func TestListInformes_KeepsEmptyNameWhenLeadMissing(t *testing.T) {
	env := newDimobEnv(t)
	defer env.Close()

	informe := fiscal.NewInforme("tenant-1", 2025, "lead-ausente", "", 100, 10, 0, 90)
	repo := &fiscalRepoStub{existingInformes: []*fiscal.Informe{informe}}
	svc := env.service(repo, &transferListerStub{})

	informes, err := svc.ListInformes(context.Background(), "tenant-1", 2025)
	if err != nil {
		t.Fatalf("ListInformes: %v", err)
	}
	if informes[0].OwnerName != "" {
		t.Errorf("nome deveria continuar vazio, veio %q", informes[0].OwnerName)
	}
	if len(repo.updatedInformes) != 0 {
		t.Error("não deveria persistir nada sem nome resolvido")
	}
}

func TestListReports_PassesFilterAndNormalizesType(t *testing.T) {
	repo := &fiscalRepoStub{reportHistory: []*fiscal.ReportHistory{
		fiscal.NewReportHistory("tenant-1", "DIMOB", "DIMOB 2025", ""),
	}}
	svc := NewFiscalService(repo)
	reports, total, err := svc.ListReports(context.Background(), "tenant-1", fiscal.ReportHistoryFilter{Type: " dimob ", Year: 2025, Limit: 5, Offset: 10})
	if err != nil {
		t.Fatalf("ListReports: %v", err)
	}
	if len(reports) != 1 || total != 1 {
		t.Fatalf("reports = %d, total = %d", len(reports), total)
	}
	if repo.listReportsFilter.Type != "DIMOB" {
		t.Errorf("tipo normalizado = %q, esperado DIMOB", repo.listReportsFilter.Type)
	}
	if repo.listReportsFilter.Year != 2025 || repo.listReportsFilter.Limit != 5 || repo.listReportsFilter.Offset != 10 {
		t.Errorf("filtro repassado incorreto: %+v", repo.listReportsFilter)
	}
}

func TestArchiveReport_DelegatesToRepository(t *testing.T) {
	repo := &fiscalRepoStub{}
	svc := NewFiscalService(repo)
	if err := svc.ArchiveReport(context.Background(), "tenant-1", "report-1"); err != nil {
		t.Fatalf("ArchiveReport: %v", err)
	}
	if repo.archivedReportID != "report-1" {
		t.Errorf("id arquivado = %q", repo.archivedReportID)
	}
}

func TestGenerateDimob_StoresDeclarante(t *testing.T) {
	repo := &fiscalRepoStub{} // sem report existente → cria um novo
	svc := NewFiscalService(repo)
	report, err := svc.GenerateDimob(context.Background(), "tenant-1", 2025, "11.222.333/0001-81", "Imobiliária Modelo Ltda")
	if err != nil {
		t.Fatalf("GenerateDimob: %v", err)
	}
	if report.DeclaranteCNPJ != "11.222.333/0001-81" {
		t.Errorf("DeclaranteCNPJ = %q", report.DeclaranteCNPJ)
	}
	if repo.createdDimob == nil || repo.createdDimob.DeclaranteNome != "Imobiliária Modelo Ltda" {
		t.Error("declarante não foi persistida via CreateDimob")
	}
}

func TestGenerateDimob_RegenPreservesDeclarante(t *testing.T) {
	existing := newDimobReportWithDeclarante(2025) // já tem CNPJ + nome
	repo := &fiscalRepoStub{dimobByYear: existing}
	svc := NewFiscalService(repo)
	report, err := svc.GenerateDimob(context.Background(), "tenant-1", 2025, "", "")
	if err != nil {
		t.Fatalf("GenerateDimob: %v", err)
	}
	if report.DeclaranteCNPJ != "11.222.333/0001-81" {
		t.Errorf("regerar sem CNPJ deveria preservar o existente, veio %q", report.DeclaranteCNPJ)
	}
}

func TestGenerateDimob_NewReportPersistsViaCreate(t *testing.T) {
	// Regressão: relatório novo precisa ser INSERT (CreateDimob). Antes
	// o caminho usava SaveDimob — que é UPDATE e não persiste linha nova.
	repo := &fiscalRepoStub{} // sem report existente
	svc := NewFiscalService(repo)
	if _, err := svc.GenerateDimob(context.Background(), "tenant-1", 2025, "11.222.333/0001-81", "Imob"); err != nil {
		t.Fatalf("GenerateDimob: %v", err)
	}
	if repo.createdDimob == nil {
		t.Error("relatório novo deveria ser persistido via CreateDimob")
	}
	if repo.updatedDimob != nil {
		t.Error("relatório novo não deveria chamar UpdateDimob")
	}
}

// ── FIN-02: falha de agregação não pode virar DIMOB vazia publicada como pronta ──

func TestGenerateDimob_RentalAggregationFailureAborts(t *testing.T) {
	repo := &fiscalRepoStub{
		countChargesErr: map[string]error{"RENT": errors.New("postgres indisponível")},
	}
	svc := NewFiscalService(repo)

	report, err := svc.GenerateDimob(context.Background(), "tenant-1", 2025, "11.222.333/0001-81", "Imob")
	if err == nil {
		t.Fatal("falha ao contar locação deveria interromper a geração")
	}
	if report != nil {
		t.Errorf("nenhum relatório deveria ser devolvido, veio %+v", report)
	}
	if repo.createdDimob != nil {
		t.Error("nada pode ser persistido quando a agregação falha")
	}
	if repo.updatedDimob != nil {
		t.Error("nada pode ser atualizado quando a agregação falha")
	}
}

func TestGenerateDimob_SaleAggregationFailureAborts(t *testing.T) {
	repo := &fiscalRepoStub{
		countChargesErr: map[string]error{"SALE": errors.New("timeout")},
	}
	svc := NewFiscalService(repo)

	if _, err := svc.GenerateDimob(context.Background(), "tenant-1", 2025, "11.222.333/0001-81", "Imob"); err == nil {
		t.Fatal("falha ao contar venda deveria interromper a geração")
	}
	if repo.createdDimob != nil {
		t.Error("nada pode ser persistido quando a agregação falha")
	}
}

func TestGenerateDimob_AggregationFailureDoesNotOverwriteExisting(t *testing.T) {
	// Regressão do defeito: a regeração sobrescrevia um relatório já pronto
	// com totais zerados e status READY quando a consulta falhava.
	existing := newDimobReportWithDeclarante(2025)
	existing.Status = fiscal.DimobStatusReady
	existing.TotalAmount = 987654.32
	repo := &fiscalRepoStub{
		dimobByYear:     existing,
		countChargesErr: map[string]error{"RENT": errors.New("postgres indisponível")},
	}
	svc := NewFiscalService(repo)

	if _, err := svc.GenerateDimob(context.Background(), "tenant-1", 2025, "", ""); err == nil {
		t.Fatal("falha de agregação deveria interromper a regeração")
	}
	if repo.updatedDimob != nil {
		t.Error("relatório existente não pode ser sobrescrito com totais incompletos")
	}
	if existing.TotalAmount != 987654.32 {
		t.Errorf("total existente foi corrompido: %v", existing.TotalAmount)
	}
}

func TestGetDimobStatus_AggregationFailureReturnsError(t *testing.T) {
	// Sem relatório persistido o status é sintetizado das agregações. Falha
	// ali devolvia 200 com total zerado, indistinguível de "sem operações".
	repo := &fiscalRepoStub{
		countChargesErr: map[string]error{"SALE": errors.New("postgres indisponível")},
	}
	svc := NewFiscalService(repo)

	report, err := svc.GetDimobStatus(context.Background(), "tenant-1", 2025)
	if err == nil {
		t.Fatal("falha de agregação deveria ser propagada, não devolver total zerado")
	}
	if report != nil {
		t.Errorf("nenhum relatório deveria ser devolvido, veio %+v", report)
	}
}

func TestDimobAggregates_SumsRentalsAndSales(t *testing.T) {
	repo := &fiscalRepoStub{
		countChargesByType: map[string]struct {
			count  int
			amount float64
		}{
			"RENT": {count: 3, amount: 1000.50},
			"SALE": {count: 1, amount: 250000},
		},
	}
	svc := NewFiscalService(repo)

	report, err := svc.GetDimobStatus(context.Background(), "tenant-1", 2025)
	if err != nil {
		t.Fatalf("GetDimobStatus: %v", err)
	}
	if report.TotalRentals != 3 || report.TotalSales != 1 {
		t.Errorf("contagens = %d/%d, want 3/1", report.TotalRentals, report.TotalSales)
	}
	if report.TotalAmount != 251000.50 {
		t.Errorf("TotalAmount = %v, want 251000.50", report.TotalAmount)
	}
}

func TestGenerateInformes_AggregatesByOwner(t *testing.T) {
	repo := &fiscalRepoStub{}
	transfers := &transferListerStub{byMonth: map[string][]*transfer.OwnerTransfer{
		"2025-01": {
			{OwnerID: "owner-1", GrossRent: 1000, AdminFeeAmount: 100, NetAmount: 900, Status: transfer.StatusProcessed},
			{OwnerID: "owner-2", GrossRent: 2000, AdminFeeAmount: 200, NetAmount: 1800, Status: transfer.StatusProcessed},
		},
		"2025-02": {
			{OwnerID: "owner-1", GrossRent: 1000, AdminFeeAmount: 100, NetAmount: 900, Status: transfer.StatusApproved},
		},
	}}
	svc := NewFiscalService(repo).WithTransferLister(transfers)

	informes, err := svc.GenerateInformes(context.Background(), "tenant-1", 2025)
	if err != nil {
		t.Fatalf("GenerateInformes: %v", err)
	}
	if len(informes) != 2 {
		t.Fatalf("esperava 2 informes (um por owner), veio %d", len(informes))
	}
	// Saída ordenada por owner ID: owner-1 vem antes de owner-2.
	o1 := informes[0]
	if o1.OwnerID != "owner-1" {
		t.Fatalf("informe[0] owner = %q, want owner-1", o1.OwnerID)
	}
	if o1.TotalGross != 2000 || o1.TotalAdminFee != 200 || o1.TotalNet != 1800 {
		t.Errorf("owner-1 agregado errado: gross=%v fee=%v net=%v", o1.TotalGross, o1.TotalAdminFee, o1.TotalNet)
	}
	if informes[1].OwnerID != "owner-2" || informes[1].TotalGross != 2000 {
		t.Errorf("owner-2 agregado errado: owner=%q gross=%v", informes[1].OwnerID, informes[1].TotalGross)
	}
	if len(repo.createdInformes) != 2 {
		t.Errorf("esperava 2 informes criados via CreateInforme, veio %d", len(repo.createdInformes))
	}
	for _, inf := range informes {
		if inf.Status != fiscal.InformeStatusReady {
			t.Errorf("informe %s status = %q, want READY", inf.OwnerID, inf.Status)
		}
	}
}

func TestGenerateInformes_SkipsCancelled(t *testing.T) {
	repo := &fiscalRepoStub{}
	transfers := &transferListerStub{byMonth: map[string][]*transfer.OwnerTransfer{
		"2025-05": {
			{OwnerID: "owner-1", GrossRent: 1000, AdminFeeAmount: 100, NetAmount: 900, Status: transfer.StatusProcessed},
			{OwnerID: "owner-1", GrossRent: 5000, AdminFeeAmount: 500, NetAmount: 4500, Status: transfer.StatusCancelled},
		},
	}}
	svc := NewFiscalService(repo).WithTransferLister(transfers)

	informes, err := svc.GenerateInformes(context.Background(), "tenant-1", 2025)
	if err != nil {
		t.Fatalf("GenerateInformes: %v", err)
	}
	if len(informes) != 1 {
		t.Fatalf("esperava 1 informe, veio %d", len(informes))
	}
	if informes[0].TotalGross != 1000 {
		t.Errorf("repasse cancelado deveria ser ignorado, gross = %v want 1000", informes[0].TotalGross)
	}
}

func TestGenerateInformes_RegenUpdatesExisting(t *testing.T) {
	// owner-1 já tem informe no ano — regerar atualiza, não duplica.
	prior := fiscal.NewInforme("tenant-1", 2025, "owner-1", "Maria", 1, 1, 1, 1)
	repo := &fiscalRepoStub{existingInformes: []*fiscal.Informe{prior}}
	transfers := &transferListerStub{byMonth: map[string][]*transfer.OwnerTransfer{
		"2025-07": {
			{OwnerID: "owner-1", GrossRent: 3000, AdminFeeAmount: 300, NetAmount: 2700, Status: transfer.StatusProcessed},
		},
	}}
	svc := NewFiscalService(repo).WithTransferLister(transfers)

	informes, err := svc.GenerateInformes(context.Background(), "tenant-1", 2025)
	if err != nil {
		t.Fatalf("GenerateInformes: %v", err)
	}
	if len(informes) != 1 {
		t.Fatalf("esperava 1 informe, veio %d", len(informes))
	}
	if informes[0].ID != prior.ID {
		t.Errorf("regerar deveria reaproveitar o informe existente (id %s), veio %s", prior.ID, informes[0].ID)
	}
	if informes[0].TotalGross != 3000 {
		t.Errorf("informe existente não foi reagregado: gross = %v", informes[0].TotalGross)
	}
	if len(repo.updatedInformes) != 1 {
		t.Errorf("esperava 1 UpdateInforme, veio %d", len(repo.updatedInformes))
	}
	if len(repo.createdInformes) != 0 {
		t.Errorf("regerar não deveria criar informe novo, veio %d", len(repo.createdInformes))
	}
}

func TestGenerateInformes_NoTransferLister(t *testing.T) {
	svc := NewFiscalService(&fiscalRepoStub{}) // sem WithTransferLister
	if _, err := svc.GenerateInformes(context.Background(), "tenant-1", 2025); err == nil {
		t.Error("esperava erro quando o transferLister é nil")
	}
}

func TestGenerateInformes_NoTransfers(t *testing.T) {
	repo := &fiscalRepoStub{}
	svc := NewFiscalService(repo).WithTransferLister(&transferListerStub{})
	informes, err := svc.GenerateInformes(context.Background(), "tenant-1", 2025)
	if err != nil {
		t.Fatalf("GenerateInformes: %v", err)
	}
	if len(informes) != 0 {
		t.Errorf("sem repasses não deveria gerar informes, veio %d", len(informes))
	}
}

func TestGenerateInformes_AggregatesIntermediation(t *testing.T) {
	// T-05: a linha de intermediação imobiliária do informe anual soma a
	// comissão retida do EconomicBreakdown (centavos) dos repasses do ano.
	repo := &fiscalRepoStub{}
	transfers := &transferListerStub{byMonth: map[string][]*transfer.OwnerTransfer{
		"2025-03": {
			{
				OwnerID: "owner-1", GrossRent: 3000, AdminFeeAmount: 300, NetAmount: 2200,
				Status: transfer.StatusProcessed,
				EconomicBreakdown: &transfer.EconomicBreakdown{
					IntermediationRevenueCents: 50000, // R$ 500,00
				},
			},
		},
		"2025-08": {
			{
				OwnerID: "owner-1", GrossRent: 2000, AdminFeeAmount: 200, NetAmount: 1750,
				Status: transfer.StatusProcessed,
				EconomicBreakdown: &transfer.EconomicBreakdown{
					IntermediationRevenueCents: 5000, // R$ 50,00
				},
			},
		},
	}}
	svc := NewFiscalService(repo).WithTransferLister(transfers)

	informes, err := svc.GenerateInformes(context.Background(), "tenant-1", 2025)
	if err != nil {
		t.Fatalf("GenerateInformes: %v", err)
	}
	if len(informes) != 1 {
		t.Fatalf("esperava 1 informe, veio %d", len(informes))
	}
	if informes[0].TotalIntermediation != 550 {
		t.Errorf("intermediação agregada = %v, want 550", informes[0].TotalIntermediation)
	}
	if informes[0].TotalNet != 3950 {
		t.Errorf("net = %v, want 3950", informes[0].TotalNet)
	}
}

func TestGenerateInformes_IntermediationWithoutBreakdownIsZero(t *testing.T) {
	// Repasses legados sem EconomicBreakdown não inventam intermediação.
	repo := &fiscalRepoStub{}
	transfers := &transferListerStub{byMonth: map[string][]*transfer.OwnerTransfer{
		"2025-05": {
			{OwnerID: "owner-1", GrossRent: 1000, AdminFeeAmount: 100, NetAmount: 900, Status: transfer.StatusProcessed},
		},
	}}
	svc := NewFiscalService(repo).WithTransferLister(transfers)

	informes, err := svc.GenerateInformes(context.Background(), "tenant-1", 2025)
	if err != nil {
		t.Fatalf("GenerateInformes: %v", err)
	}
	if len(informes) != 1 {
		t.Fatalf("esperava 1 informe, veio %d", len(informes))
	}
	if informes[0].TotalIntermediation != 0 {
		t.Errorf("sem breakdown a intermediação deveria ser 0, veio %v", informes[0].TotalIntermediation)
	}
}

func TestIssueNfse_WithSandboxProviderFillsAuthorityReceipt(t *testing.T) {
	repo := &fiscalRepoStub{}
	svc := NewFiscalService(repo).WithNfseProvider(providers.NewSandboxNfseProvider())

	got, err := svc.IssueNfse(
		context.Background(), "tenant-1", "2026-07", "recipient-1", 1250.50,
		"Intermediação imobiliária",
	)
	if err != nil {
		t.Fatalf("IssueNfse: %v", err)
	}
	if repo.createdNfse == nil || repo.savedNfse == nil {
		t.Fatal("fluxo com provedor deve criar (rascunho) e salvar (recibo)")
	}
	if got.Status != fiscal.NfseStatusIssued {
		t.Fatalf("status = %q, want ISSUED", got.Status)
	}
	if got.Number == "" || got.PDFURL == "" || got.XMLURL == "" {
		t.Fatalf("recibo de autoridade não preenchido: %+v", got)
	}
}

func TestIssueNfse_ProviderFailureMarksErrorAndWrapsTypedError(t *testing.T) {
	repo := &fiscalRepoStub{}
	sandbox := providers.NewSandboxNfseProvider()
	sandbox.FailWith = providers.ErrNfseProviderUnavailable
	svc := NewFiscalService(repo).WithNfseProvider(sandbox)

	got, err := svc.IssueNfse(
		context.Background(), "tenant-1", "2026-07", "recipient-1", 100,
		"Intermediação",
	)
	if err == nil {
		t.Fatal("esperava erro do provedor")
	}
	if !errors.Is(err, providers.ErrNfseProviderUnavailable) {
		t.Fatalf("err = %v, want wrap de ErrNfseProviderUnavailable", err)
	}
	if got == nil || got.Status != fiscal.NfseStatusError {
		t.Fatalf("nota deveria ficar ERROR, veio %+v", got)
	}
	if repo.savedNfse == nil || repo.savedNfse.Status != fiscal.NfseStatusError {
		t.Fatal("estado ERROR não foi persistido")
	}
}

func TestGenerateDimob_HistoryCarriesFileURL(t *testing.T) {
	repo := &fiscalRepoStub{}
	svc := NewFiscalService(repo).WithTransferLister(&transferListerStub{})

	if _, err := svc.GenerateDimob(context.Background(), "tenant-1", 2025, "12345678000199", "Imobiliária Teste"); err != nil {
		t.Fatalf("GenerateDimob: %v", err)
	}
	if repo.savedReport == nil {
		t.Fatal("histórico do relatório não foi salvo")
	}
	if repo.savedReport.FileURL != "/api/v1/fiscal/dimob/2025/file" {
		t.Fatalf("file_url = %q, want /api/v1/fiscal/dimob/2025/file", repo.savedReport.FileURL)
	}
}

func TestIssueNfse_HistoryCarriesDanfeURL(t *testing.T) {
	repo := &fiscalRepoStub{}
	svc := NewFiscalService(repo).WithNfseProvider(providers.NewSandboxNfseProvider())

	got, err := svc.IssueNfse(context.Background(), "tenant-1", "2026-07", "recipient-1", 100, "Intermediação")
	if err != nil {
		t.Fatalf("IssueNfse: %v", err)
	}
	if repo.savedReport == nil {
		t.Fatal("histórico da NFS-e não foi salvo")
	}
	if repo.savedReport.FileURL != got.PDFURL || repo.savedReport.FileURL == "" {
		t.Fatalf("file_url do histórico = %q, danfe da nota = %q", repo.savedReport.FileURL, got.PDFURL)
	}
}
