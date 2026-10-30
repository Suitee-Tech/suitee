package journal

import "github.com/oginoino/toca/toca-financial/internal/domain/charge"

// AccountNature distinguishes assets and obligations from the agency's result.
type AccountNature string

const (
	NatureAsset     AccountNature = "asset"
	NatureLiability AccountNature = "liability"
	NatureEquity    AccountNature = "equity"
	NatureRevenue   AccountNature = "revenue"
	NatureExpense   AccountNature = "expense"
)

// ChartAccount is a stable reporting classification of an existing ledger
// account. Several revenue classifications share AccountRevenue; the economic
// dimensions on each leg distinguish them without rewriting posted entries.
type ChartAccount struct {
	Code          string        `json:"code"`
	Name          string        `json:"name"`
	Nature        AccountNature `json:"nature"`
	LedgerAccount Account       `json:"ledger_account"`
	Flows         []string      `json:"flows"`
	Description   string        `json:"description"`
}

type ChartOfAccounts struct {
	Version  string         `json:"version"`
	Accounts []ChartAccount `json:"accounts"`
}

// Account codes are stable identifiers, independent of translated names.
const (
	CodeCash                 = "1.1.01"
	CodeAccountsReceivable   = "1.1.02"
	CodeOwnerPayable         = "2.1.01"
	CodeCommissionPayable    = "2.1.02"
	CodePartnerPayable       = "2.1.03"
	CodeRoyaltyPayable       = "2.1.04"
	CodeTaxWithheld          = "2.1.05"
	CodeOtherPayable         = "2.1.06"
	CodeWithdrawalPayable    = "2.1.07"
	CodeSaleIntermediation   = "3.1.01"
	CodeRentalAdministration = "3.1.02"
	CodeRentalIntermediation = "3.1.03"
	CodeRentalRevenuePending = "3.1.04"
	CodeRevenuePending       = "3.1.99"
	CodeCommissionExpense    = "4.1.01"
	CodeExpense              = "4.1.02"
	CodeEquity               = "5.1.01"
)

var chartAccounts = []ChartAccount{
	{CodeCash, "Caixa e bancos", NatureAsset, AccountCash, []string{"sale", "rent"}, "Entradas e saídas efetivas de caixa. O saldo inclui valores de terceiros ainda a repassar."},
	{CodeAccountsReceivable, "Contas a receber", NatureAsset, AccountAccountsReceivable, []string{"sale", "rent"}, "Direitos a receber de compradores e inquilinos registrados no razão."},
	{CodeOwnerPayable, "Repasses a proprietários", NatureLiability, AccountOwnerPayable, []string{"sale", "rent"}, "Valores pertencentes ao proprietário ou vendedor, mantidos como obrigação até a liquidação do repasse."},
	{CodeCommissionPayable, "Comissões a pagar", NatureLiability, AccountCommissionPayable, []string{"sale", "rent"}, "Comissões devidas aos corretores. A obrigação é baixada quando o pagamento é confirmado."},
	{CodePartnerPayable, "Repasses a parceiros", NatureLiability, AccountPartnerPayable, []string{"sale", "rent"}, "Valores destinados a empresas parceiras, separados da receita própria da imobiliária."},
	{CodeRoyaltyPayable, "Royalties a pagar", NatureLiability, AccountRoyaltyPayable, []string{"sale", "rent"}, "Royalties devidos à franqueadora até sua liquidação."},
	{CodeTaxWithheld, "Tributos retidos a recolher", NatureLiability, AccountTaxWithheld, []string{"sale", "rent"}, "Tributos retidos que permanecem comprometidos até o recolhimento."},
	{CodeOtherPayable, "Outras retenções a destinar", NatureLiability, AccountOtherPayable, []string{"sale", "rent"}, "Outros valores retidos com destinação pendente, sem reconhecimento como receita própria."},
	{CodeWithdrawalPayable, "Saques a pagar", NatureLiability, AccountWithdrawalPayable, []string{"sale", "rent"}, "Valores já comprometidos em solicitações de saque e ainda não liquidados."},
	{CodeSaleIntermediation, "Intermediação de vendas", NatureRevenue, AccountRevenue, []string{"sale"}, "Parcela da intermediação de venda reconhecida como receita própria da imobiliária."},
	{CodeRentalAdministration, "Administração de locações", NatureRevenue, AccountRevenue, []string{"rent"}, "Taxa de administração identificada na destinação do recebimento de aluguel."},
	{CodeRentalIntermediation, "Intermediação de locações", NatureRevenue, AccountRevenue, []string{"rent"}, "Parcela da intermediação de locação pertencente à imobiliária, identificada na destinação do recebimento."},
	{CodeRentalRevenuePending, "Receitas de locação a classificar", NatureRevenue, AccountRevenue, []string{"rent"}, "Receita identificada como locação cuja finalidade ainda precisa ser apurada."},
	{CodeRevenuePending, "Receitas com modalidade a apurar", NatureRevenue, AccountRevenue, []string{"sale", "rent"}, "Receita sem modalidade confiável no lançamento; preserva a classificação pendente sem inferir venda ou locação."},
	{CodeCommissionExpense, "Despesas com comissões", NatureExpense, AccountCommissionExpense, []string{"sale", "rent"}, "Comissões suportadas pela imobiliária registradas como despesa própria."},
	{CodeExpense, "Outras despesas da imobiliária", NatureExpense, AccountExpense, []string{"sale", "rent"}, "Despesas de responsabilidade econômica da imobiliária, inclusive encargos por ela assumidos."},
	{CodeEquity, "Patrimônio líquido", NatureEquity, AccountEquity, []string{"sale", "rent"}, "Movimentações patrimoniais da imobiliária; o caixa disponível também depende das obrigações pendentes."},
}

// GetChartOfAccounts returns an independent copy of the read-only catalogue.
func GetChartOfAccounts() ChartOfAccounts {
	accounts := make([]ChartAccount, len(chartAccounts))
	for i, account := range chartAccounts {
		accounts[i] = copyChartAccount(account)
	}
	return ChartOfAccounts{Version: "1", Accounts: accounts}
}

// ClassifyLegAccount resolves a reporting account only from recorded economic
// dimensions. Missing revenue dimensions stay explicit rather than guessing
// from descriptions, amounts or the presence of a contract. Unknown ledger
// accounts return false. Classification never changes a journal leg.
func ClassifyLegAccount(leg Leg) (ChartAccount, bool) {
	if leg.Account != AccountRevenue {
		for _, account := range chartAccounts {
			if account.LedgerAccount == leg.Account {
				return copyChartAccount(account), true
			}
		}
		return ChartAccount{}, false
	}

	code := CodeRevenuePending
	switch leg.Flow {
	case "sale":
		code = CodeSaleIntermediation
	case "rent":
		code = rentalRevenueCode(leg.AllocationPurpose)
	case "":
		// Purpose is a recorded allocation dimension. It can identify rental
		// revenue on older legs that predate the explicit modality field.
		if leg.AllocationPurpose == string(charge.AllocationPurposeAdminFee) ||
			leg.AllocationPurpose == string(charge.AllocationPurposeRentalIntermediationAgency) ||
			leg.AllocationPurpose == string(charge.AllocationPurposeRentalIntermediationBroker) {
			code = rentalRevenueCode(leg.AllocationPurpose)
		}
	}
	for _, account := range chartAccounts {
		if account.Code == code {
			return copyChartAccount(account), true
		}
	}
	return ChartAccount{}, false
}

func rentalRevenueCode(purpose string) string {
	switch charge.AllocationPurpose(purpose) {
	case charge.AllocationPurposeAdminFee:
		return CodeRentalAdministration
	case charge.AllocationPurposeRentalIntermediationAgency, charge.AllocationPurposeRentalIntermediationBroker:
		return CodeRentalIntermediation
	default:
		return CodeRentalRevenuePending
	}
}

func copyChartAccount(account ChartAccount) ChartAccount {
	account.Flows = append([]string(nil), account.Flows...)
	return account
}
