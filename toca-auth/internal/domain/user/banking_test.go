package user

import (
	"errors"
	"testing"
)

func TestBankingInfo_IsEmpty(t *testing.T) {
	if !(BankingInfo{}).IsEmpty() {
		t.Error("zero-value should be empty")
	}
	if (BankingInfo{BankCode: "001"}).IsEmpty() {
		t.Error("BankCode set should not be empty")
	}
	if (BankingInfo{PixKey: "x"}).IsEmpty() {
		t.Error("PixKey set should not be empty")
	}
}

func TestBankingInfo_Validate_Empty(t *testing.T) {
	if err := (BankingInfo{}).Validate(); err != nil {
		t.Errorf("empty banking should be valid (opcional), got %v", err)
	}
}

func TestBankingInfo_Validate_PartialBankInfoFails(t *testing.T) {
	// BankCode sozinho sem Agency/Account → erro.
	cases := []struct {
		name string
		b    BankingInfo
		want string
	}{
		{
			name: "só bank_code",
			b:    BankingInfo{BankCode: "001"},
			want: "agency",
		},
		{
			name: "bank_code + agency, sem account",
			b:    BankingInfo{BankCode: "001", Agency: "1234"},
			want: "account",
		},
		{
			name: "só agency",
			b:    BankingInfo{Agency: "1234"},
			want: "bank_code",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.b.Validate()
			if err == nil {
				t.Fatal("expected error")
			}
			var bvErr *BankingValidationError
			if !errors.As(err, &bvErr) {
				t.Fatalf("expected BankingValidationError, got %T", err)
			}
			if bvErr.Field != tc.want {
				t.Errorf("expected field %q, got %q", tc.want, bvErr.Field)
			}
		})
	}
}

func TestBankingInfo_Validate_FullBankInfoOk(t *testing.T) {
	b := BankingInfo{
		BankCode:    "341",
		BankName:    "Itaú",
		Agency:      "1234",
		Account:     "56789",
		AccountType: BankAccountTypeChecking,
	}
	if err := b.Validate(); err != nil {
		t.Errorf("trio completo deveria ser válido, got %v", err)
	}
}

func TestBankingInfo_Validate_AccountType(t *testing.T) {
	b := BankingInfo{
		BankCode:    "341",
		Agency:      "1234",
		Account:     "56789",
		AccountType: "weird",
	}
	if err := b.Validate(); err == nil {
		t.Error("expected account_type inválido")
	}
}

func TestBankingInfo_Validate_PixCPF(t *testing.T) {
	// CPF gerado válido: 39053344705 (algoritmo modulo-11)
	good := BankingInfo{PixKeyType: PixKeyTypeCPF, PixKey: "390.533.447-05"}
	if err := good.Validate(); err != nil {
		t.Errorf("CPF válido com mask: %v", err)
	}
	bad := BankingInfo{PixKeyType: PixKeyTypeCPF, PixKey: "111.111.111-11"}
	if err := bad.Validate(); err == nil {
		t.Error("CPF repetido deve falhar")
	}
}

func TestBankingInfo_Validate_PixCNPJ(t *testing.T) {
	good := BankingInfo{PixKeyType: PixKeyTypeCNPJ, PixKey: "11.222.333/0001-81"}
	if err := good.Validate(); err != nil {
		t.Errorf("CNPJ válido com mask: %v", err)
	}
	bad := BankingInfo{PixKeyType: PixKeyTypeCNPJ, PixKey: "11.222.333/0001-00"}
	if err := bad.Validate(); err == nil {
		t.Error("CNPJ inválido deve falhar")
	}
}

func TestBankingInfo_Validate_PixEmail(t *testing.T) {
	good := BankingInfo{PixKeyType: PixKeyTypeEmail, PixKey: "ana@suitee.com.br"}
	if err := good.Validate(); err != nil {
		t.Errorf("email válido: %v", err)
	}
	bad := BankingInfo{PixKeyType: PixKeyTypeEmail, PixKey: "no-at-sign"}
	if err := bad.Validate(); err == nil {
		t.Error("email inválido deve falhar")
	}
}

func TestBankingInfo_Validate_PixPhone(t *testing.T) {
	good := BankingInfo{PixKeyType: PixKeyTypePhone, PixKey: "+5511999998888"}
	if err := good.Validate(); err != nil {
		t.Errorf("phone com + e DDI: %v", err)
	}
	good2 := BankingInfo{PixKeyType: PixKeyTypePhone, PixKey: "11999998888"}
	if err := good2.Validate(); err != nil {
		t.Errorf("phone sem +: %v", err)
	}
	bad := BankingInfo{PixKeyType: PixKeyTypePhone, PixKey: "abc"}
	if err := bad.Validate(); err == nil {
		t.Error("phone com letra deve falhar")
	}
}

func TestBankingInfo_Validate_PixEVP(t *testing.T) {
	good := BankingInfo{PixKeyType: PixKeyTypeEVP, PixKey: "550e8400-e29b-41d4-a716-446655440000"}
	if err := good.Validate(); err != nil {
		t.Errorf("UUID v4 válido: %v", err)
	}
	bad := BankingInfo{PixKeyType: PixKeyTypeEVP, PixKey: "not-a-uuid"}
	if err := bad.Validate(); err == nil {
		t.Error("não-UUID deve falhar")
	}
}

func TestBankingInfo_Validate_PixKeyWithoutType(t *testing.T) {
	b := BankingInfo{PixKey: "ana@suitee.com.br"}
	if err := b.Validate(); err == nil {
		t.Error("PixKey sem PixKeyType deve falhar")
	}
}

func TestBankingInfo_Validate_PixTypeWithoutKey(t *testing.T) {
	b := BankingInfo{PixKeyType: PixKeyTypeCPF}
	if err := b.Validate(); err == nil {
		t.Error("PixKeyType sem PixKey deve falhar")
	}
}

func TestBankingInfo_Validate_HolderDocument(t *testing.T) {
	// CPF válido
	if err := (BankingInfo{HolderDocument: "390.533.447-05"}).Validate(); err != nil {
		t.Errorf("HolderDocument CPF válido: %v", err)
	}
	// CNPJ válido
	if err := (BankingInfo{HolderDocument: "11.222.333/0001-81"}).Validate(); err != nil {
		t.Errorf("HolderDocument CNPJ válido: %v", err)
	}
	// Inválido
	if err := (BankingInfo{HolderDocument: "12345"}).Validate(); err == nil {
		t.Error("HolderDocument inválido deve falhar")
	}
}

func TestBankingInfo_Validate_OnlyPixOk(t *testing.T) {
	// Usuário pode preencher só PIX, sem trio bancário.
	b := BankingInfo{PixKeyType: PixKeyTypeEmail, PixKey: "x@y.com"}
	if err := b.Validate(); err != nil {
		t.Errorf("só PIX deveria ser válido, got %v", err)
	}
}

func TestBankingInfo_Validate_BothPixAndBankOk(t *testing.T) {
	// PIX + trio bancário tradicional juntos = OK (caller decide qual usar).
	b := BankingInfo{
		BankCode:    "341",
		Agency:      "1234",
		Account:     "56789",
		AccountType: BankAccountTypeChecking,
		PixKeyType:  PixKeyTypeEmail,
		PixKey:      "x@y.com",
	}
	if err := b.Validate(); err != nil {
		t.Errorf("PIX + trio: %v", err)
	}
}
