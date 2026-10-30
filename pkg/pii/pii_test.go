package pii

import (
	"strings"
	"testing"
)

func TestMaskString(t *testing.T) {
	SetProductionMode(true)
	defer SetProductionMode(false)

	tests := []struct {
		name string
		kind Kind
		in   string
		want string
	}{
		{"email_normal", KindEmail, "maria@cutia.tech", "g***o@cutia.tech"},
		{"email_short_local", KindEmail, "ab@x.io", "a***@x.io"},
		{"email_invalid", KindEmail, "notanemail", "<redacted>"},
		{"cpf_formatted", KindCPF, "123.456.789-00", "***8900"},
		{"cpf_raw", KindCPF, "12345678900", "***8900"},
		{"phone_formatted", KindPhone, "+55 (11) 99999-1234", "***1234"},
		{"phone_short", KindPhone, "123", "***"},
		{"name_single", KindName, "Maria", "M***"},
		{"name_two", KindName, "Maria Silva", "M*** S***"},
		{"address_redacts_fully", KindAddress, "Rua Foo 123", "<address-redacted>"},
		{"empty_stays_empty", KindEmail, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MaskString(tc.kind, tc.in)
			if got != tc.want {
				t.Errorf("MaskString(%s, %q) = %q, want %q", tc.kind, tc.in, got, tc.want)
			}
		})
	}
}

type fixtureLead struct {
	ID    string `json:"id"`
	Name  string `json:"name"  pii:"name"`
	Email string `json:"email" pii:"email"`
	Phone string `json:"phone" pii:"phone"`
	Note  string `json:"note"` // no tag — should pass through
}

func TestMaskForLog_StructTag(t *testing.T) {
	SetProductionMode(true)
	defer SetProductionMode(false)

	in := fixtureLead{
		ID:    "lead-1",
		Name:  "Maria Silva",
		Email: "maria@cutia.tech",
		Phone: "+55 11 99999-1234",
		Note:  "high priority",
	}
	out := MaskForLog(in).(fixtureLead)
	if out.ID != "lead-1" {
		t.Errorf("ID changed: %q", out.ID)
	}
	if !strings.Contains(out.Name, "***") {
		t.Errorf("Name not masked: %q", out.Name)
	}
	if !strings.Contains(out.Email, "cutia.tech") {
		t.Errorf("Email domain stripped: %q", out.Email)
	}
	if !strings.Contains(out.Phone, "1234") {
		t.Errorf("Phone last digits lost: %q", out.Phone)
	}
	if out.Note != "high priority" {
		t.Errorf("untagged Note changed: %q", out.Note)
	}
}

func TestMaskForLog_NonProductionIsNoOp(t *testing.T) {
	SetProductionMode(false)
	defer SetProductionMode(false)

	in := fixtureLead{Name: "Maria Silva", Email: "maria@cutia.tech"}
	out := MaskForLog(in).(fixtureLead)
	if out.Name != in.Name || out.Email != in.Email {
		t.Errorf("dev-mode masking changed values: %+v", out)
	}
}
