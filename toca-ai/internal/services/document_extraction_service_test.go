package services

import (
	"testing"

	"github.com/oginoino/toca/toca-ai/internal/domain"
)

func TestExtValidateField_CPF(t *testing.T) {
	// Valid CPF must boost confidence and pass validation.
	base, status, _ := extValidateField("cpf_cnpj", "11144477735", 0.5)
	if status != domain.ValidationValid {
		t.Fatalf("valid CPF: status=%s", status)
	}
	if base < 0.95 {
		t.Fatalf("valid CPF should boost confidence, got %v", base)
	}

	// Repeated-digit CPF must be rejected (strict policy — never relax).
	low, status2, msg := extValidateField("cpf_cnpj", "11111111111", 0.9)
	if status2 != domain.ValidationInvalid {
		t.Fatalf("repeated-digit CPF must be invalid, got %s", status2)
	}
	if low > 0.30 {
		t.Fatalf("invalid CPF must cap confidence, got %v", low)
	}
	if msg == "" {
		t.Fatalf("invalid CPF should set a validation message")
	}
}

func TestExtValidateField_CNPJ(t *testing.T) {
	_, status, _ := extValidateField("cpf_cnpj", "11222333000181", 0.6)
	if status != domain.ValidationValid {
		t.Fatalf("valid CNPJ should pass, got %s", status)
	}
}

func TestExtNormalizeValue(t *testing.T) {
	if got := extNormalizeValue("date", "07/06/1990"); got != "1990-06-07" {
		t.Fatalf("date normalize: got %q", got)
	}
	if got := extNormalizeValue("cpf_cnpj", "111.444.777-35"); got != "11144477735" {
		t.Fatalf("cpf normalize: got %q", got)
	}
	if got := extNormalizeValue("uf", "sp"); got != "SP" {
		t.Fatalf("uf normalize: got %q", got)
	}
}

func TestExtTierThresholds(t *testing.T) {
	if extTier(0.9) != domain.TierHigh {
		t.Fatalf("0.9 should be high")
	}
	if extTier(0.7) != domain.TierMedium {
		t.Fatalf("0.7 should be medium")
	}
	if extTier(0.3) != domain.TierLow {
		t.Fatalf("0.3 should be low")
	}
}

func TestExtParseFields_StripsFences(t *testing.T) {
	in := "```json\n{\"fields\":[{\"name\":\"document\",\"value\":\"123\",\"confidence\":0.8}]}\n```"
	fields, err := extParseFields(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(fields) != 1 || fields[0].Name != "document" {
		t.Fatalf("unexpected fields: %+v", fields)
	}
}
