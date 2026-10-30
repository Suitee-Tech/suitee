package fiscal

import (
	"time"

	"github.com/google/uuid"
)

// NFS-e (Nota Fiscal de Serviço Eletrônica)

type NfseStatus string

const (
	NfseStatusDraft     NfseStatus = "DRAFT"
	NfseStatusIssued    NfseStatus = "ISSUED"
	NfseStatusCancelled NfseStatus = "CANCELLED"
	NfseStatusError     NfseStatus = "ERROR"
)

type Nfse struct {
	ID                 string     `json:"id" bson:"_id"`
	TenantID           string     `json:"tenant_id" bson:"tenant_id"`
	Number             string     `json:"number" bson:"number"`
	ReferenceMonth     string     `json:"reference_month" bson:"reference_month"`
	IssueDate          time.Time  `json:"issue_date" bson:"issue_date"`
	Amount             float64    `json:"amount" bson:"amount"`
	ServiceDescription string     `json:"service_description" bson:"service_description"`
	RecipientName      string     `json:"recipient_name" bson:"recipient_name"`
	RecipientDocument  string     `json:"recipient_document" bson:"recipient_document"`
	Status             NfseStatus `json:"status" bson:"status"`
	PDFURL             string     `json:"pdf_url,omitempty" bson:"pdf_url,omitempty"`
	XMLURL             string     `json:"xml_url,omitempty" bson:"xml_url,omitempty"`
	CreatedAt          time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at" bson:"updated_at"`
}

func NewNfse(tenantID, referenceMonth, recipientID string, amount float64, serviceDescription string) *Nfse {
	now := time.Now()
	return &Nfse{
		ID:                 uuid.New().String(),
		TenantID:           tenantID,
		ReferenceMonth:     referenceMonth,
		IssueDate:          now,
		Amount:             amount,
		ServiceDescription: serviceDescription,
		RecipientName:      recipientID,
		RecipientDocument:  "",
		Number:             uuid.New().String()[:8],
		Status:             NfseStatusDraft,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// DIMOB (Declaração de Informações sobre Atividades Imobiliárias)

type DimobStatus string

const (
	DimobStatusPending    DimobStatus = "PENDING"
	DimobStatusGenerating DimobStatus = "GENERATING"
	DimobStatusReady      DimobStatus = "READY"
	DimobStatusSubmitted  DimobStatus = "SUBMITTED"
	DimobStatusError      DimobStatus = "ERROR"
)

type DimobReport struct {
	ID           string      `json:"id" bson:"_id"`
	TenantID     string      `json:"tenant_id" bson:"tenant_id"`
	Year         int         `json:"year" bson:"year"`
	Status       DimobStatus `json:"status" bson:"status"`
	TotalRentals int         `json:"total_rentals" bson:"total_rentals"`
	TotalSales   int         `json:"total_sales" bson:"total_sales"`
	TotalAmount  float64     `json:"total_amount" bson:"total_amount"`
	FileURL      string      `json:"file_url,omitempty" bson:"file_url,omitempty"`
	// DeclaranteCNPJ / DeclaranteNome — dados da pessoa jurídica que
	// entrega a DIMOB (a imobiliária). Capturados no GenerateDimob e
	// usados no download do TXT (registro R01 do layout RFB). Issue #418.
	DeclaranteCNPJ string     `json:"declarante_cnpj,omitempty" bson:"declarante_cnpj,omitempty"`
	DeclaranteNome string     `json:"declarante_nome,omitempty" bson:"declarante_nome,omitempty"`
	GeneratedAt    *time.Time `json:"generated_at,omitempty" bson:"generated_at,omitempty"`
	SubmittedAt    *time.Time `json:"submitted_at,omitempty" bson:"submitted_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at" bson:"created_at"`
}

func NewDimobReport(tenantID string, year int) *DimobReport {
	return &DimobReport{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		Year:      year,
		Status:    DimobStatusPending,
		CreatedAt: time.Now(),
	}
}

// Informe de Rendimentos

type InformeStatus string

const (
	InformeStatusPending    InformeStatus = "PENDING"
	InformeStatusGenerating InformeStatus = "GENERATING"
	InformeStatusReady      InformeStatus = "READY"
	InformeStatusError      InformeStatus = "ERROR"
)

type Informe struct {
	ID            string  `json:"id" bson:"_id"`
	TenantID      string  `json:"tenant_id" bson:"tenant_id"`
	Year          int     `json:"year" bson:"year"`
	OwnerID       string  `json:"owner_id" bson:"owner_id"`
	OwnerName     string  `json:"owner_name" bson:"owner_name"`
	TotalGross    float64 `json:"total_gross" bson:"total_gross"`
	TotalAdminFee float64 `json:"total_admin_fee" bson:"total_admin_fee"`
	// TotalIntermediation soma a intermediação imobiliária retida dos
	// repasses do ano (T-05): é a linha que explica o repasse menor do que o
	// bruto, junto com a taxa de administração.
	TotalIntermediation float64 `json:"total_intermediation" bson:"total_intermediation"`
	TotalNet            float64 `json:"total_net" bson:"total_net"`

	// Separação por fluxo (T-06). Locação gera renda tributável do
	// proprietário (carnê-leão/ajuste anual); venda gera ganho de capital
	// (GCAP) e a comissão de intermediação retida da imobiliária. Somar os
	// dois num único "rendimento bruto" superestima a renda — ex.: um repasse
	// de venda de R$ 1,4 mi aparecia como rendimento de locação.
	RentalGross          float64 `json:"rental_gross" bson:"rental_gross"`
	RentalAdminFee       float64 `json:"rental_admin_fee" bson:"rental_admin_fee"`
	RentalIntermediation float64 `json:"rental_intermediation" bson:"rental_intermediation"`
	RentalNet            float64 `json:"rental_net" bson:"rental_net"`
	RentalCount          int     `json:"rental_count" bson:"rental_count"`

	// SaleGross é o valor das vendas do ano; SaleIntermediation é a comissão
	// de intermediação de venda retida (receita da imobiliária, não renda do
	// proprietário); SaleNet é o repasse líquido das vendas.
	SaleGross          float64       `json:"sale_gross" bson:"sale_gross"`
	SaleIntermediation float64       `json:"sale_intermediation" bson:"sale_intermediation"`
	SaleNet            float64       `json:"sale_net" bson:"sale_net"`
	SaleCount          int           `json:"sale_count" bson:"sale_count"`
	Status             InformeStatus `json:"status" bson:"status"`
	PDFURL             string        `json:"pdf_url,omitempty" bson:"pdf_url,omitempty"`
	GeneratedAt        *time.Time    `json:"generated_at,omitempty" bson:"generated_at,omitempty"`
	CreatedAt          time.Time     `json:"created_at" bson:"created_at"`
}

func NewInforme(tenantID string, year int, ownerID, ownerName string, totalGross, totalAdminFee, totalIntermediation, totalNet float64) *Informe {
	return &Informe{
		ID:                  uuid.New().String(),
		TenantID:            tenantID,
		Year:                year,
		OwnerID:             ownerID,
		OwnerName:           ownerName,
		TotalGross:          totalGross,
		TotalAdminFee:       totalAdminFee,
		TotalIntermediation: totalIntermediation,
		TotalNet:            totalNet,
		Status:              InformeStatusPending,
		CreatedAt:           time.Now(),
	}
}

// Fiscal Calendar

type DeadlineStatus string

const (
	DeadlineStatusUpcoming  DeadlineStatus = "UPCOMING"
	DeadlineStatusDueSoon   DeadlineStatus = "DUE_SOON"
	DeadlineStatusOverdue   DeadlineStatus = "OVERDUE"
	DeadlineStatusCompleted DeadlineStatus = "COMPLETED"
)

type FiscalDeadline struct {
	ID          string         `json:"id" bson:"_id"`
	TenantID    string         `json:"tenant_id" bson:"tenant_id"`
	Type        string         `json:"type" bson:"type"` // DIMOB, NFSE, INFORME
	Title       string         `json:"title" bson:"title"`
	Description string         `json:"description" bson:"description"`
	DueDate     time.Time      `json:"due_date" bson:"due_date"`
	Status      DeadlineStatus `json:"status" bson:"status"`
}

// Report History

type ReportHistoryStatus string

const (
	ReportHistoryStatusReady ReportHistoryStatus = "READY"
	ReportHistoryStatusError ReportHistoryStatus = "ERROR"
)

// ReportHistoryFilter filtra a listagem do histórico de relatórios:
// tipo (DIMOB/NFSE/INFORME), ano-calendário do relatório e paginação.
type ReportHistoryFilter struct {
	Type   string
	Year   int
	Limit  int64
	Offset int64
}

type ReportHistory struct {
	ID          string              `json:"id" bson:"_id"`
	TenantID    string              `json:"tenant_id" bson:"tenant_id"`
	Type        string              `json:"type" bson:"type"` // DIMOB, NFSE, INFORME
	Name        string              `json:"name" bson:"name"`
	FileURL     string              `json:"file_url,omitempty" bson:"file_url,omitempty"`
	GeneratedAt time.Time           `json:"generated_at" bson:"generated_at"`
	Status      ReportHistoryStatus `json:"status" bson:"status"`
}

func NewReportHistory(tenantID, reportType, name, fileURL string) *ReportHistory {
	return &ReportHistory{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		Type:        reportType,
		Name:        name,
		FileURL:     fileURL,
		GeneratedAt: time.Now(),
		Status:      ReportHistoryStatusReady,
	}
}
