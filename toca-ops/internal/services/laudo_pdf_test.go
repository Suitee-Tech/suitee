package services

import (
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-ops/internal/models/inspection"
)

func sampleInspection() *inspection.Inspection {
	completed := time.Date(2026, 5, 10, 14, 0, 0, 0, time.UTC)
	return &inspection.Inspection{
		ID:            "insp-123",
		TenantID:      "tenant-1",
		PropertyID:    "prop-9",
		ContractID:    "contract-7",
		Type:          inspection.TypeExit,
		ScheduledDate: time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC),
		CompletedDate: &completed,
		InspectorName: "Ana Vistoriadora",
		Status:        inspection.StatusConcluida,
		Notes:         "Imóvel em bom estado geral.",
		Findings: []inspection.Finding{
			{
				ID:          "f1",
				Room:        "Cozinha",
				Description: "Pia e armários",
				Condition:   inspection.ConditionBom,
				Photos:      []string{"https://storage/photo1.jpg", "https://storage/photo2.jpg"},
			},
			{
				ID:          "f2",
				Room:        "Banheiro",
				Description: "Box e azulejos",
				Condition:   inspection.ConditionRuim,
				Notes:       "Azulejo trincado atrás da porta.",
				Photos:      []string{},
			},
		},
	}
}

func TestBuildLaudoHTML_IncludesCoreData(t *testing.T) {
	h := BuildLaudoHTML(sampleInspection())

	mustContain := []string{
		"Vistoria de Saída",
		"prop-9",
		"contract-7",
		"Ana Vistoriadora",
		"Imóvel em bom estado",
		"Cozinha",
		"Banheiro",
		"Pia e armários",
		"Azulejo trincado",
		"insp-123",
	}
	for _, s := range mustContain {
		if !strings.Contains(h, s) {
			t.Errorf("HTML should contain %q", s)
		}
	}
}

func TestBuildLaudoHTML_RendersPhotos(t *testing.T) {
	h := BuildLaudoHTML(sampleInspection())
	if !strings.Contains(h, `<img src="https://storage/photo1.jpg"`) {
		t.Error("photo 1 should be rendered as img tag")
	}
	if !strings.Contains(h, `<img src="https://storage/photo2.jpg"`) {
		t.Error("photo 2 should be rendered as img tag")
	}
	if !strings.Contains(h, "Sem fotos anexadas") {
		t.Error("finding without photos should show placeholder")
	}
}

func TestBuildLaudoHTML_ConditionBadgeColors(t *testing.T) {
	h := BuildLaudoHTML(sampleInspection())
	if !strings.Contains(h, "#16a34a") {
		t.Error("Condition Bom should use green badge")
	}
	if !strings.Contains(h, "#dc2626") {
		t.Error("Condition Ruim should use red badge")
	}
}

func TestBuildLaudoHTML_EscapesHTML(t *testing.T) {
	insp := sampleInspection()
	insp.Notes = `<script>alert('xss')</script>`
	insp.Findings[0].Room = `Sala & "varanda"`
	h := BuildLaudoHTML(insp)

	if strings.Contains(h, "<script>alert") {
		t.Error("script tag should be escaped, not raw")
	}
	if !strings.Contains(h, "&lt;script&gt;") {
		t.Error("expected escaped script tag")
	}
	if !strings.Contains(h, "Sala &amp; &#34;varanda&#34;") {
		t.Errorf("room name should be escaped")
	}
}

func TestBuildLaudoHTML_EmptyFindings(t *testing.T) {
	insp := sampleInspection()
	insp.Findings = nil
	h := BuildLaudoHTML(insp)
	if !strings.Contains(h, "Nenhum item registrado") {
		t.Error("empty findings should show empty-state message")
	}
	if !strings.Contains(h, "Itens vistoriados (0)") {
		t.Error("should show count 0")
	}
}

func TestBuildLaudoHTML_MissingFieldsUseEmDash(t *testing.T) {
	insp := &inspection.Inspection{
		ID:            "x",
		Type:          inspection.TypeEntry,
		Status:        inspection.StatusEmAndamento,
		ScheduledDate: time.Now(),
	}
	h := BuildLaudoHTML(insp)
	if !strings.Contains(h, "—") {
		t.Error("missing fields should render em-dash")
	}
	if !strings.Contains(h, "Vistoria de Entrada") {
		t.Error("ENTRY type label")
	}
}

func TestBuildLaudoHTML_TypeLabels(t *testing.T) {
	cases := map[string]string{
		inspection.TypeEntry:    "Vistoria de Entrada",
		inspection.TypeExit:     "Vistoria de Saída",
		inspection.TypePeriodic: "Vistoria Periódica",
		"":                      "Vistoria",
	}
	for typ, label := range cases {
		insp := sampleInspection()
		insp.Type = typ
		h := BuildLaudoHTML(insp)
		if !strings.Contains(h, label) {
			t.Errorf("type %q should render label %q", typ, label)
		}
	}
}

func TestBuildLaudoHTML_IsWellFormed(t *testing.T) {
	h := BuildLaudoHTML(sampleInspection())
	if !strings.HasPrefix(h, "<!DOCTYPE html>") {
		t.Error("should start with doctype")
	}
	if !strings.HasSuffix(h, "</html>") {
		t.Error("should end with closing html tag")
	}
	if strings.Count(h, "<body>") != 1 || strings.Count(h, "</body>") != 1 {
		t.Error("should have exactly one body open/close")
	}
}

func TestConditionBadgeColor(t *testing.T) {
	if conditionBadgeColor(inspection.ConditionBom) != "#16a34a" {
		t.Error("Bom should be green")
	}
	if conditionBadgeColor(inspection.ConditionRegular) != "#d97706" {
		t.Error("Regular should be amber")
	}
	if conditionBadgeColor(inspection.ConditionRuim) != "#dc2626" {
		t.Error("Ruim should be red")
	}
	if conditionBadgeColor("desconhecido") != "#64748b" {
		t.Error("unknown should be gray")
	}
}
