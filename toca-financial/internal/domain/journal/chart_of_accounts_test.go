package journal

import (
	"reflect"
	"testing"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
)

func TestChartOfAccountsCoversLedgerAndProtectsCatalogue(t *testing.T) {
	chart := GetChartOfAccounts()
	if chart.Version != "1" {
		t.Fatalf("unexpected catalogue version: %q", chart.Version)
	}
	seenCodes := make(map[string]bool)
	seenAccounts := make(map[Account]bool)
	for _, account := range chart.Accounts {
		if seenCodes[account.Code] || account.Code == "" || account.Name == "" || account.Description == "" {
			t.Fatalf("duplicate or incomplete chart account: %+v", account)
		}
		seenCodes[account.Code] = true
		if !ValidAccounts[account.LedgerAccount] {
			t.Fatalf("chart references unknown ledger account: %q", account.LedgerAccount)
		}
		seenAccounts[account.LedgerAccount] = true
		if len(account.Flows) == 0 {
			t.Fatalf("account %s has no modality", account.Code)
		}
		for _, flow := range account.Flows {
			if flow != "sale" && flow != "rent" {
				t.Fatalf("invalid flow %q on %s", flow, account.Code)
			}
		}
	}
	for account := range ValidAccounts {
		if !seenAccounts[account] {
			t.Errorf("ledger account %q is missing from the catalogue", account)
		}
	}

	original := GetChartOfAccounts()
	chart.Accounts[0].Name = "altered"
	chart.Accounts[0].Flows[0] = "altered"
	if !reflect.DeepEqual(GetChartOfAccounts(), original) {
		t.Fatal("caller mutation changed the shared catalogue")
	}
}

func TestClassifyLegAccountPreservesEconomicOwnership(t *testing.T) {
	tests := []struct {
		name   string
		leg    Leg
		code   string
		nature AccountNature
	}{
		{"sale agency revenue", Leg{Account: AccountRevenue, Flow: "sale"}, CodeSaleIntermediation, NatureRevenue},
		{"rental administration", Leg{Account: AccountRevenue, Flow: "rent", AllocationPurpose: string(charge.AllocationPurposeAdminFee)}, CodeRentalAdministration, NatureRevenue},
		{"rental agency intermediation", Leg{Account: AccountRevenue, Flow: "rent", AllocationPurpose: string(charge.AllocationPurposeRentalIntermediationAgency)}, CodeRentalIntermediation, NatureRevenue},
		{"rental gross intermediation funds broker", Leg{Account: AccountRevenue, Flow: "rent", AllocationPurpose: string(charge.AllocationPurposeRentalIntermediationBroker)}, CodeRentalIntermediation, NatureRevenue},
		{"legacy gross intermediation funds broker", Leg{Account: AccountRevenue, AllocationPurpose: string(charge.AllocationPurposeRentalIntermediationBroker)}, CodeRentalIntermediation, NatureRevenue},
		{"rental purpose still pending", Leg{Account: AccountRevenue, Flow: "rent"}, CodeRentalRevenuePending, NatureRevenue},
		{"legacy revenue still pending", Leg{Account: AccountRevenue}, CodeRevenuePending, NatureRevenue},
		{"legacy recorded rental purpose", Leg{Account: AccountRevenue, AllocationPurpose: string(charge.AllocationPurposeAdminFee)}, CodeRentalAdministration, NatureRevenue},
		{"legacy recorded rental agency purpose", Leg{Account: AccountRevenue, AllocationPurpose: string(charge.AllocationPurposeRentalIntermediationAgency)}, CodeRentalIntermediation, NatureRevenue},
		{"explicit sale takes precedence", Leg{Account: AccountRevenue, Flow: "sale", AllocationPurpose: string(charge.AllocationPurposeAdminFee)}, CodeSaleIntermediation, NatureRevenue},
		{"unknown flow cannot infer rental", Leg{Account: AccountRevenue, Flow: "unknown", AllocationPurpose: string(charge.AllocationPurposeAdminFee)}, CodeRevenuePending, NatureRevenue},
		{"unknown purpose cannot infer rental", Leg{Account: AccountRevenue, AllocationPurpose: "unknown"}, CodeRevenuePending, NatureRevenue},
		{"broker funds remain payable", Leg{Account: AccountCommissionPayable, Flow: "rent", AllocationPurpose: string(charge.AllocationPurposeRentalIntermediationBroker)}, CodeCommissionPayable, NatureLiability},
		{"owner rent remains payable", Leg{Account: AccountOwnerPayable, Flow: "rent", AllocationPurpose: string(charge.AllocationPurposeOwnerRent)}, CodeOwnerPayable, NatureLiability},
		{"seller funds remain payable", Leg{Account: AccountOwnerPayable, Flow: "sale"}, CodeOwnerPayable, NatureLiability},
		{"agency commission expense", Leg{Account: AccountCommissionExpense, Flow: "sale"}, CodeCommissionExpense, NatureExpense},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := tt.leg
			got, ok := ClassifyLegAccount(tt.leg)
			if !ok || got.Code != tt.code || got.Nature != tt.nature || got.LedgerAccount != tt.leg.Account {
				t.Fatalf("classification = %+v, %v; want %s/%s", got, ok, tt.code, tt.nature)
			}
			if tt.leg != before {
				t.Fatal("classification mutated a historical journal leg")
			}
			// Reversals retain the same classification while changing direction.
			tt.leg.Direction = Debit
			reversal, ok := ClassifyLegAccount(tt.leg)
			if !ok || reversal.Code != got.Code {
				t.Fatalf("reversal changed the economic classification: %+v", reversal)
			}
		})
	}
	if _, ok := ClassifyLegAccount(Leg{Account: "unknown", Flow: "sale"}); ok {
		t.Fatal("unknown ledger account must not become a valid chart account")
	}
}
