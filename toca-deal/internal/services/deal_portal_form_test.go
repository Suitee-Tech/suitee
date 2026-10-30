package services

import (
	"reflect"
	"testing"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

func TestFormSubmissionValuesKeepsOnlyRequestedFields(t *testing.T) {
	requested := []deal.ChecklistRequestedField{
		{Key: "full_name", Label: "Nome completo"},
		{Key: "cpf", Label: "CPF"},
		{Key: "profissao", Label: "Profissão"},
	}
	got := formSubmissionValues(
		requested,
		"  Fernando Pessoa ",
		"123.456.789-00",
		"",
		"",
		map[string]string{"profissao": " Engenheiro ", "nao_pedido": "x"},
	)
	want := map[string]string{
		"full_name": "Fernando Pessoa",
		"cpf":       "123.456.789-00",
		"profissao": "Engenheiro",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("formSubmissionValues() = %#v, want %#v", got, want)
	}
}

func TestFormSubmissionValuesIncludesEmptyRequestedField(t *testing.T) {
	got := formSubmissionValues(
		[]deal.ChecklistRequestedField{{Key: "phone"}},
		"", "", "", "",
		nil,
	)
	if value, ok := got["phone"]; !ok || value != "" {
		t.Fatalf("expected phone present and empty, got %#v", got)
	}
}
