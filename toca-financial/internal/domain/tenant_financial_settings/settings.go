// Package tenant_financial_settings models the per-tenant defaults that
// drive the financial pipeline when callers omit explicit values:
//
//   - Late fee % e juros mensais usados pelo delinquency_service quando o
//     contrato não traz overrides.
//   - Dia de vencimento padrão consumido pelo rental_charge_config_service.
//   - % de comissão default (RENT / SALE) consumido pelo
//     event_handler contract.signed quando o deal não carrega commission_pct.
//   - "payout_proof_required" controla o guard da UI de payouts
//     (/financeiro/payouts) — quando true, marcar-pago exige upload de
//     comprovante via /proofs.
//
// Defaults aplicados em runtime quando a row é ausente (o service Get
// devolve um Settings populado sempre). Não há seed na migration —
// tenants não são enumeráveis dali.
package tenant_financial_settings

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	DefaultLateFeePct        = 2.0
	DefaultMonthlyInterest   = 1.0
	DefaultDueDay            = 10
	DefaultCommissionPctRent = 100.0
	DefaultCommissionPctSale = 6.0
	DefaultCurrency          = "BRL"
	DefaultPayoutProofReqd   = true
	// DefaultAdminFeePct alinha o default ao mercado brasileiro (10%
	// retido pela imobiliária no aluguel). Consumido pelo
	// TransferChargeHook.EnsureMonthlyForChargePaid para descontar a
	// taxa de administração do gross_rent antes do net_amount
	// repassado ao proprietário. Substitui a confusão histórica com
	// contract.commission_pct (que segue exclusivo de comissão de
	// broker — vide event_handler.go).
	DefaultAdminFeePct = 10.0
	// DefaultLeaseTermMonths é o prazo padrão do contrato de locação em
	// meses. É a sugestão que o plano pré-contrato usa para calcular o
	// término (e, por consequência, o residual do último aluguel); o
	// prazo efetivo continua sendo o do contrato gerado. Espelha
	// `DefaultLeaseTermMonths` do toca-deal, que segue como fallback
	// quando o tenant não configurou nada.
	DefaultLeaseTermMonths = 30
	// MinLeaseTermMonths/MaxLeaseTermMonths limitam o prazo aceito na
	// configuração (1 mês a 10 anos).
	MinLeaseTermMonths = 1
	MaxLeaseTermMonths = 120
)

// Settings represents one tenant's financial defaults. Carry-all struct;
// adding a field here means: (a) update repo scanner, (b) update handler
// PUT body, (c) update FE settings page form.
type Settings struct {
	TenantID                  string  `json:"tenant_id"`
	DefaultLateFeePct         float64 `json:"default_late_fee_pct"`
	DefaultMonthlyInterestPct float64 `json:"default_monthly_interest_pct"`
	DefaultDueDay             int     `json:"default_due_day"`
	DefaultCommissionPctRent  float64 `json:"default_commission_pct_rent"`
	DefaultCommissionPctSale  float64 `json:"default_commission_pct_sale"`
	DefaultCurrency           string  `json:"default_currency"`
	PayoutProofRequired       bool    `json:"payout_proof_required"`
	// Alíquotas padrão das retenções sobre comissão de corretor. 0 = não retém
	// (o operador liga quando a imobiliária passou a reter). INSS de autônomo é
	// tipicamente 20%; o ISS varia por município (2% a 5%) — por isso é campo, e
	// não constante.
	DefaultINSSPct float64 `json:"default_inss_pct"`
	DefaultISSPct  float64 `json:"default_iss_pct"`
	// AdminFeePct é o % do aluguel bruto retido pela imobiliária como
	// taxa de administração antes do repasse ao proprietário.
	// Consumido pelo TransferChargeHook.EnsureMonthlyForChargePaid
	// (RENT e SALE). Default DefaultAdminFeePct (10%).
	AdminFeePct float64 `json:"admin_fee_pct"`
	// DefaultLeaseTermMonths é o prazo padrão (meses) sugerido para novos
	// contratos de locação. Consumido pelo plano pré-contrato da esteira.
	DefaultLeaseTermMonths int `json:"default_lease_term_months"`
	// DelinquencyAutoCaseAfterDays é o limiar (em dias de atraso) para o motor
	// abrir o caso de cobrança automaticamente. 0 = usa o padrão da plataforma
	// (`DefaultDelinquencyConfig.AutoCreateCollectionAfterDays`, hoje 5).
	//
	// Era constante de código: imobiliária que cobra com 3 dias ou que só quer o
	// caso depois de 10 não tinha como ajustar, e a tela repetia o número 5
	// (espelhado no frontend, sujeito a mentir se o default mudasse).
	DelinquencyAutoCaseAfterDays int `json:"delinquency_auto_case_after_days"`
	// HoldTransfersOnDelinquency é a política de retenção de repasse por
	// inadimplência. DESLIGADO por padrão e **ainda não consumido por nenhum
	// gate**: o repasse nasce do pagamento da cobrança (sem pagamento não há
	// repasse), então reter por inadimplência exige decidir o que fazer quando o
	// caso é resolvido e como liberar o valor retido — decisão de produto
	// registrada em docs/features/financeiro/inadimplencia-diagnostico-e-plano.md.
	//
	// O campo existe para a política poder ser configurada e auditada, mas o
	// comentário é explícito de propósito: campo que promete efeito sem ter gate
	// é pior que campo ausente.
	HoldTransfersOnDelinquency bool `json:"hold_transfers_on_delinquency"`
	// DelinquencyHoldMinSharePct é a MATERIALIDADE mínima da dívida em relação ao
	// repasse para a retenção acontecer, em %. Padrão 100 = só retém quando a
	// dívida em aberto cobre o repasse inteiro (reter R$ 3.000 de repasse por
	// R$ 200 de dívida é desproporcional, e essa é a régua que evita isso).
	// Abaixo de 100 a imobiliária fica mais conservadora, ciente de que um repasse
	// grande pode ficar retido por uma dívida menor.
	DelinquencyHoldMinSharePct int       `json:"delinquency_hold_min_share_pct"`
	UpdatedBy                  string    `json:"updated_by,omitempty"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

// New builds a Settings instance populated com os defaults globais. Used
// by the repo Get() fallback (tenant sem row) e por testes.
func New(tenantID string) *Settings {
	return &Settings{
		TenantID:                  tenantID,
		DefaultLateFeePct:         DefaultLateFeePct,
		DefaultMonthlyInterestPct: DefaultMonthlyInterest,
		DefaultDueDay:             DefaultDueDay,
		DefaultCommissionPctRent:  DefaultCommissionPctRent,
		DefaultCommissionPctSale:  DefaultCommissionPctSale,
		DefaultCurrency:           DefaultCurrency,
		PayoutProofRequired:       DefaultPayoutProofReqd,
		AdminFeePct:               DefaultAdminFeePct,
		DefaultLeaseTermMonths:    DefaultLeaseTermMonths,
		UpdatedAt:                 time.Now().UTC(),
	}
}

// Validate ensures invariantes que os handlers checam antes de persistir
// um payload vindo do PUT.
func (s *Settings) Validate() error {
	if s == nil {
		return errors.New("settings is nil")
	}
	if strings.TrimSpace(s.TenantID) == "" {
		return errors.New("tenant_id is required")
	}
	if s.DefaultLateFeePct < 0 || s.DefaultLateFeePct > 100 {
		return errors.New("default_late_fee_pct must be between 0 and 100")
	}
	if s.DefaultMonthlyInterestPct < 0 || s.DefaultMonthlyInterestPct > 100 {
		return errors.New("default_monthly_interest_pct must be between 0 and 100")
	}
	// Alíquotas de retenção: 0 desliga; acima de 100 não existe tributo.
	if s.DefaultINSSPct < 0 || s.DefaultINSSPct > 100 {
		return errors.New("default_inss_pct must be between 0 and 100")
	}
	if s.DefaultISSPct < 0 || s.DefaultISSPct > 100 {
		return errors.New("default_iss_pct must be between 0 and 100")
	}
	if s.DefaultDueDay < 1 || s.DefaultDueDay > 28 {
		// Limite 1-28 cobre o mês mais curto (fevereiro), evitando
		// vencimentos que "escorregam" em meses sem o dia escolhido.
		return errors.New("default_due_day must be between 1 and 28")
	}
	if s.DefaultCommissionPctRent < 0 || s.DefaultCommissionPctRent > 100 {
		return errors.New("default_commission_pct_rent must be between 0 and 100")
	}
	if s.DefaultCommissionPctSale < 0 || s.DefaultCommissionPctSale > 100 {
		return errors.New("default_commission_pct_sale must be between 0 and 100")
	}
	// Limiar de abertura de caso: 0 = padrão da plataforma; acima de 365 dias não
	// é política de cobrança, é dado errado.
	if s.DelinquencyAutoCaseAfterDays < 0 || s.DelinquencyAutoCaseAfterDays > 365 {
		return errors.New("delinquency_auto_case_after_days must be between 0 and 365")
	}
	// 0 = padrão (100%). Acima de 100 não existe materialidade a exigir.
	if s.DelinquencyHoldMinSharePct < 0 || s.DelinquencyHoldMinSharePct > 100 {
		return errors.New("delinquency_hold_min_share_pct must be between 0 and 100")
	}
	if s.AdminFeePct < 0 || s.AdminFeePct > 100 {
		return errors.New("admin_fee_pct must be between 0 and 100")
	}
	if s.DefaultLeaseTermMonths < MinLeaseTermMonths || s.DefaultLeaseTermMonths > MaxLeaseTermMonths {
		return fmt.Errorf("default_lease_term_months must be between %d and %d", MinLeaseTermMonths, MaxLeaseTermMonths)
	}
	if strings.TrimSpace(s.DefaultCurrency) == "" {
		return errors.New("default_currency is required")
	}
	return nil
}
