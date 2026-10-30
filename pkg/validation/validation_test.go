package validation

import (
	"testing"
	"time"
)

func TestIsValidCPF(t *testing.T) {
	t.Run("accepts valid CPFs", func(t *testing.T) {
		for _, v := range []string{"529.982.247-25", "52998224725", "111.444.777-35"} {
			if !IsValidCPF(v) {
				t.Errorf("expected valid: %q", v)
			}
		}
	})
	t.Run("rejects bad shape and same-digit", func(t *testing.T) {
		for _, v := range []string{"", "1234", "1234567890", "111.111.111-11", "00000000000", "529.982.247-26"} {
			if IsValidCPF(v) {
				t.Errorf("expected invalid: %q", v)
			}
		}
	})
}

func TestIsValidCNPJ(t *testing.T) {
	t.Run("accepts valid CNPJs", func(t *testing.T) {
		for _, v := range []string{"04.252.011/0001-10", "04252011000110", "11.222.333/0001-81"} {
			if !IsValidCNPJ(v) {
				t.Errorf("expected valid: %q", v)
			}
		}
	})
	t.Run("rejects bad shape and same-digit", func(t *testing.T) {
		for _, v := range []string{"", "12345678901234", "11111111111111", "00000000000000", "04.252.011/0001-11"} {
			if IsValidCNPJ(v) {
				t.Errorf("expected invalid: %q", v)
			}
		}
	})
}

func TestCPFAndCNPJ(t *testing.T) {
	v := New()

	type req struct {
		CPF  string `json:"cpf" validate:"required,cpf_valid"`
		CNPJ string `json:"cnpj" validate:"required,cnpj_valid"`
	}

	ok := req{
		CPF:  "529.982.247-25",
		CNPJ: "04.252.011/0001-10",
	}

	if err := v.Struct(ok); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}

	bad := req{
		CPF:  "111.111.111-11",
		CNPJ: "00.000.000/0000-00",
	}
	if err := v.Struct(bad); err == nil {
		t.Fatalf("expected error")
	}
}

func TestFutureDate(t *testing.T) {
	v := New()

	type req struct {
		Date string `json:"date" validate:"required,future_date"`
	}

	r := req{Date: time.Now().Add(24 * time.Hour).Format("2006-01-02")}
	if err := v.Struct(r); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}

	r2 := req{Date: time.Now().Add(-24 * time.Hour).Format("2006-01-02")}
	if err := v.Struct(r2); err == nil {
		t.Fatalf("expected error")
	}
}

func TestExtract(t *testing.T) {
	v := New()

	type req struct {
		Email string `json:"email" validate:"required,email"`
		TaxID string `json:"tax_id" validate:"required,cnpj_valid"`
	}

	err := v.Struct(req{})
	ve := Extract(err, req{})
	if ve == nil || len(ve.Fields) == 0 {
		t.Fatalf("expected structured errors")
	}

	if ve.Fields[0].Field == "" {
		t.Fatalf("expected field name")
	}
}
