package collection

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AgreementStatus — ciclo de vida do acordo.
type AgreementStatus string

const (
	AgreementDraft     AgreementStatus = "DRAFT"
	AgreementActive    AgreementStatus = "ACTIVE"
	AgreementFulfilled AgreementStatus = "FULFILLED"
	AgreementBroken    AgreementStatus = "BROKEN"
	AgreementCancelled AgreementStatus = "CANCELLED"
)

// Erros de validação do acordo. O handler traduz todos em 400.
var (
	ErrInvalidAgreement      = errors.New("dados do acordo inválidos")
	ErrAgreementNotFound     = errors.New("acordo não encontrado")
	ErrAgreementNotActive    = errors.New("acordo não está ativo")
	ErrAgreementAlreadyEnded = errors.New("acordo já foi encerrado")
)

// Limites do acordo. Existem para impedir o que não é acordo: 0 parcela, 500
// parcelas, vencimento no passado. São deliberadamente folgados — a política
// fina (desconto máximo, alçada) é decisão do tenant, não do domínio.
const (
	MinInstallments = 1
	MaxInstallments = 36
	MinIntervalDays = 1
	MaxIntervalDays = 180
)

// Agreement é o acordo formalizado com o devedor (G2 do plano).
//
// Antes, "negociar" era gravar um valor e uma data no caso: nenhuma parcela
// existia, nenhuma cobrança era baixada e o acordo não tinha como ser cumprido,
// quebrado ou cobrado. Aqui o acordo é uma entidade com parcelas materializadas
// em cobranças de verdade.
//
// O acordo NÃO é dono da dívida: quem é são as cobranças (originais ou as
// parcelas). `OriginalChargeIDs` guarda o que foi substituído, para
// rastreabilidade e relatório — não para reabrir depois.
type Agreement struct {
	ID           string          `json:"id" bson:"_id"`
	CollectionID string          `json:"collection_id" bson:"collection_id"`
	TenantID     string          `json:"tenant_id" bson:"tenant_id"`
	ContractID   string          `json:"contract_id" bson:"contract_id"`
	Status       AgreementStatus `json:"status" bson:"status"`

	// OriginalTotal é a dívida do caso no momento do acordo (bruta, com encargos).
	OriginalTotal float64 `json:"original_total" bson:"original_total"`
	// TotalAmount é o valor efetivamente acordado (original - desconto).
	TotalAmount float64 `json:"total_amount" bson:"total_amount"`
	Discount    float64 `json:"discount" bson:"discount"`
	// DownPayment é a entrada paga na assinatura do acordo (0 = sem entrada).
	DownPayment float64 `json:"down_payment" bson:"down_payment"`
	// Installments é o número de parcelas, SEM contar a entrada.
	Installments int `json:"installments" bson:"installments"`
	// IntervalDays é o intervalo entre parcelas (30 = mensal).
	IntervalDays int       `json:"interval_days" bson:"interval_days"`
	FirstDueDate time.Time `json:"first_due_date" bson:"first_due_date"`

	// OriginalChargeIDs são as cobranças substituídas pelo acordo (canceladas).
	OriginalChargeIDs []string `json:"original_charge_ids" bson:"original_charge_ids"`
	// InstallmentChargeIDs são as cobranças criadas para as parcelas.
	InstallmentChargeIDs []string `json:"installment_charge_ids" bson:"installment_charge_ids"`

	Notes string `json:"notes,omitempty" bson:"notes,omitempty"`
	// ApprovalRequired marca acordo acima do limiar de alçada; ApprovedBy
	// guarda quem aprovou (segunda assinatura).
	ApprovalRequired bool   `json:"approval_required" bson:"approval_required"`
	ApprovedBy       string `json:"approved_by,omitempty" bson:"approved_by,omitempty"`

	CreatedBy string    `json:"created_by,omitempty" bson:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`

	BrokenAt    *time.Time `json:"broken_at,omitempty" bson:"broken_at,omitempty"`
	BreakReason string     `json:"break_reason,omitempty" bson:"break_reason,omitempty"`
}

// NewAgreementInput é a entrada validada de criação do acordo.
type NewAgreementInput struct {
	TotalAmount   float64
	Discount      float64
	DownPayment   float64
	Installments  int
	IntervalDays  int
	FirstDueDate  time.Time
	Notes         string
	OriginalTotal float64
	CreatedBy     string
}

// Validate confere o acordo antes de qualquer escrita.
//
// As regras são as que evitam um acordo impossível de cumprir ou um valor
// inventado: total positivo, desconto menor que o total, parcelas dentro do
// limite, vencimento não retroativo e entrada menor que o total (senão não
// sobra parcela).
func (in NewAgreementInput) Validate(now time.Time) error {
	if in.TotalAmount <= 0 {
		return fmt.Errorf("%w: valor do acordo deve ser maior que zero", ErrInvalidAgreement)
	}
	if in.Discount < 0 {
		return fmt.Errorf("%w: desconto não pode ser negativo", ErrInvalidAgreement)
	}
	if in.Discount >= in.TotalAmount {
		return fmt.Errorf("%w: desconto (%.2f) não pode ser igual ou maior que o valor do acordo (%.2f)",
			ErrInvalidAgreement, in.Discount, in.TotalAmount)
	}
	if in.DownPayment < 0 {
		return fmt.Errorf("%w: entrada não pode ser negativa", ErrInvalidAgreement)
	}
	if in.DownPayment >= in.TotalAmount {
		return fmt.Errorf("%w: entrada (%.2f) não pode ser igual ou maior que o valor do acordo (%.2f)",
			ErrInvalidAgreement, in.DownPayment, in.TotalAmount)
	}
	if in.Installments < MinInstallments || in.Installments > MaxInstallments {
		return fmt.Errorf("%w: número de parcelas deve estar entre %d e %d (recebido %d)",
			ErrInvalidAgreement, MinInstallments, MaxInstallments, in.Installments)
	}
	if in.IntervalDays < MinIntervalDays || in.IntervalDays > MaxIntervalDays {
		return fmt.Errorf("%w: intervalo entre parcelas deve estar entre %d e %d dias (recebido %d)",
			ErrInvalidAgreement, MinIntervalDays, MaxIntervalDays, in.IntervalDays)
	}
	if in.FirstDueDate.IsZero() {
		return fmt.Errorf("%w: informe o vencimento da primeira parcela", ErrInvalidAgreement)
	}
	// A primeira parcela pode vencer hoje (acordo fechado no dia), mas não no
	// passado: parcela que já nasce vencida vira inadimplência no mesmo tick.
	if in.FirstDueDate.Before(startOfDay(now)) {
		return fmt.Errorf("%w: vencimento da primeira parcela (%s) está no passado",
			ErrInvalidAgreement, in.FirstDueDate.Format("02/01/2006"))
	}
	if strings.TrimSpace(in.Notes) == "" && in.CreatedBy == "" {
		return fmt.Errorf("%w: informe o autor do acordo", ErrInvalidAgreement)
	}
	return nil
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// NewAgreement cria o acordo em estado ACTIVE a partir da entrada validada.
func NewAgreement(collectionID, tenantID, contractID string, in NewAgreementInput, now time.Time) *Agreement {
	return &Agreement{
		ID:            uuid.New().String(),
		CollectionID:  collectionID,
		TenantID:      tenantID,
		ContractID:    contractID,
		Status:        AgreementActive,
		OriginalTotal: in.OriginalTotal,
		TotalAmount:   in.TotalAmount,
		Discount:      in.Discount,
		DownPayment:   in.DownPayment,
		Installments:  in.Installments,
		IntervalDays:  in.IntervalDays,
		FirstDueDate:  in.FirstDueDate,
		Notes:         strings.TrimSpace(in.Notes),
		CreatedBy:     in.CreatedBy,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// Parcelas devolve o plano de vencimentos e valores: a entrada (quando houver,
// vencendo na data da primeira parcela) seguida das parcelas do acordo.
//
// O resto da divisão vai para a PRIMEIRA parcela, não para a última: é o que a
// imobiliária espera ao conferir a soma e evita uma última parcela com centavos
// que não fecham com o total.
type ParcelaAcordo struct {
	Numero     int
	Vencimento time.Time
	Valor      float64
	EhEntrada  bool
}

func (a *Agreement) Parcelas() []ParcelaAcordo {
	if a == nil {
		return nil
	}
	parcelas := make([]ParcelaAcordo, 0, a.Installments+1)
	restante := round2(a.TotalAmount - a.DownPayment)
	base := round2(restante / float64(a.Installments))

	numero := 1
	if a.DownPayment > 0 {
		parcelas = append(parcelas, ParcelaAcordo{
			Numero:     0,
			Vencimento: a.FirstDueDate,
			Valor:      round2(a.DownPayment),
			EhEntrada:  true,
		})
	}
	for i := 0; i < a.Installments; i++ {
		valor := base
		if i == 0 {
			// Ajuste do arredondamento na primeira parcela.
			valor = round2(restante - base*float64(a.Installments-1))
		}
		vencimento := a.FirstDueDate.AddDate(0, 0, a.IntervalDays*i)
		parcelas = append(parcelas, ParcelaAcordo{
			Numero:     numero,
			Vencimento: vencimento,
			Valor:      valor,
		})
		numero++
	}
	return parcelas
}

// PodeEncerrar reporta se o acordo ainda aceita quebra/cancelamento.
func (a *Agreement) PodeEncerrar() bool {
	return a != nil && (a.Status == AgreementActive || a.Status == AgreementDraft)
}

// Break marca o acordo como quebrado (parcela não paga).
func (a *Agreement) Break(reason string, now time.Time) error {
	if !a.PodeEncerrar() {
		return fmt.Errorf("%w: %s", ErrAgreementAlreadyEnded, a.Status)
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("%w: informe o motivo da quebra", ErrInvalidAgreement)
	}
	a.Status = AgreementBroken
	a.BreakReason = strings.TrimSpace(reason)
	a.BrokenAt = &now
	a.UpdatedAt = now
	return nil
}

// Cancel encerra o acordo sem quebra (erro de cadastro, desistência).
func (a *Agreement) Cancel(reason string, now time.Time) error {
	if !a.PodeEncerrar() {
		return fmt.Errorf("%w: %s", ErrAgreementAlreadyEnded, a.Status)
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("%w: informe o motivo do cancelamento", ErrInvalidAgreement)
	}
	a.Status = AgreementCancelled
	a.BreakReason = strings.TrimSpace(reason)
	a.UpdatedAt = now
	return nil
}

// Fulfill marca o acordo como cumprido (todas as parcelas pagas).
func (a *Agreement) Fulfill(now time.Time) {
	if a == nil {
		return
	}
	a.Status = AgreementFulfilled
	a.UpdatedAt = now
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

// AgreementRepository é a porta de persistência do acordo. Fica no domínio,
// como as outras do pacote, para o serviço depender de interface e não do
// Postgres.
type AgreementRepository interface {
	Create(ctx context.Context, a *Agreement) error
	GetByID(ctx context.Context, tenantID, id string) (*Agreement, error)
	GetActiveByCollection(ctx context.Context, tenantID, collectionID string) (*Agreement, error)
	Update(ctx context.Context, a *Agreement) error
}
