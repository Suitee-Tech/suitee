package service

import (
	"strings"
	"testing"
)

func TestAppointmentTypeNoun(t *testing.T) {
	cases := map[string]string{
		"Visita":     "Visita",
		"Reuniao":    "Reunião",
		"Vistoria":   "Vistoria",
		"Assinatura": "Assinatura",
		"Entrega":    "Entrega",
		"Outro":      "",
		"":           "",
	}
	for in, want := range cases {
		if got := appointmentTypeNoun(in); got != want {
			t.Errorf("appointmentTypeNoun(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAppointmentDisplayName(t *testing.T) {
	if got := appointmentDisplayName("Visita na cobertura", "Visita"); got != "Visita na cobertura" {
		t.Errorf("title must win: %q", got)
	}
	if got := appointmentDisplayName("", "Reuniao"); got != "Reunião" {
		t.Errorf("type noun fallback: %q", got)
	}
	if got := appointmentDisplayName("", ""); got != "Compromisso" {
		t.Errorf("default fallback: %q", got)
	}
}

func TestAppointmentWhen(t *testing.T) {
	if got := appointmentWhen("2026-10-10", "14:00"); got != "2026-10-10 às 14:00" {
		t.Errorf("full when: %q", got)
	}
	if got := appointmentWhen("2026-10-10", ""); got != "2026-10-10" {
		t.Errorf("date only: %q", got)
	}
	if got := appointmentWhen("", "14:00"); got != "" {
		t.Errorf("no date: %q", got)
	}
}

func TestAppointmentStatusNeedsEmail(t *testing.T) {
	for _, status := range []string{"Confirmado", "Reagendado", "Cancelado"} {
		if !appointmentStatusNeedsEmail(status) {
			t.Errorf("status %q must produce email", status)
		}
	}
	for _, status := range []string{"Pendente", "Agendado", "Em Andamento", "Concluido", ""} {
		if appointmentStatusNeedsEmail(status) {
			t.Errorf("status %q must stay in-app only", status)
		}
	}
}

func TestAppointmentChangedCopy(t *testing.T) {
	client := appointmentClientSubjectChanged("Confirmado", "Visita na cobertura")
	if !strings.Contains(client, "confirmado") || !strings.Contains(client, "Visita na cobertura") {
		t.Errorf("client subject: %q", client)
	}
	client = appointmentClientSubjectChanged("Reagendado", "")
	if !strings.Contains(client, "reagendado") || !strings.Contains(client, "Compromisso") {
		t.Errorf("client subject (no title): %q", client)
	}
	client = appointmentClientSubjectChanged("Cancelado", "Vistoria")
	if !strings.Contains(client, "cancelado") {
		t.Errorf("client subject cancelled: %q", client)
	}

	body := appointmentClientBodyChanged("Cancelado", "Visita na cobertura", "2026-10-10 às 14:00")
	for _, want := range []string{"cancelado", "Visita na cobertura", "2026-10-10 às 14:00"} {
		if !strings.Contains(body, want) {
			t.Errorf("client body missing %q: %s", want, body)
		}
	}

	op := appointmentOperatorSubjectChanged("Reagendado", "Visita na cobertura")
	if !strings.Contains(op, "reagendado") {
		t.Errorf("operator subject: %q", op)
	}
	opBody := appointmentOperatorBodyChanged("Confirmado", "Visita na cobertura", "2026-10-10 às 14:00")
	for _, want := range []string{"confirmado", "Visita na cobertura", "2026-10-10 às 14:00"} {
		if !strings.Contains(opBody, want) {
			t.Errorf("operator body missing %q: %s", want, opBody)
		}
	}
}

func TestAppointmentCreatedCopy(t *testing.T) {
	subject := appointmentClientSubjectCreated("Visita na cobertura", "Visita", "Agendado")
	if !strings.Contains(subject, "agendado") || !strings.Contains(subject, "Visita na cobertura") {
		t.Errorf("created subject: %q", subject)
	}
	body := appointmentClientBodyCreated("Visita na cobertura", "Visita", "2026-10-10 às 14:00", "Agendado")
	for _, want := range []string{"Visita", "agendada", "2026-10-10 às 14:00", "Visita na cobertura"} {
		if !strings.Contains(body, want) {
			t.Errorf("created body missing %q: %s", want, body)
		}
	}
	// Tipo desconhecido cai no neutro.
	body = appointmentClientBodyCreated("", "", "2026-10-10 às 14:00", "Agendado")
	if !strings.Contains(body, "compromisso") || !strings.Contains(body, "2026-10-10 às 14:00") {
		t.Errorf("created body neutral fallback: %s", body)
	}
}

func TestAppointmentCreatedPendingCopyWaitsForTheBroker(t *testing.T) {
	// O pedido feito no portal ainda depende do corretor: a comunicação não
	// pode anunciar como agendado.
	subject := appointmentClientSubjectCreated("Visita na cobertura", "Visita", "Pendente")
	if !strings.Contains(subject, "Solicitação") || strings.Contains(subject, "agendado") {
		t.Errorf("pending subject: %q", subject)
	}
	body := appointmentClientBodyCreated("Visita na cobertura", "Visita", "2026-10-10 às 14:00", "Pendente")
	for _, want := range []string{"solicitação", "corretor confirma", "2026-10-10 às 14:00"} {
		if !strings.Contains(body, want) {
			t.Errorf("pending body missing %q: %s", want, body)
		}
	}
	for _, forbidden := range []string{"agendada", "agendado"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("pending body não pode dizer %q: %s", forbidden, body)
		}
	}
}

func TestAppointmentCalendarCTA(t *testing.T) {
	// Com arquivo: o download é o convite principal e o portal continua acessível.
	btnText, btnURL, secondaryText, secondaryURL := appointmentCalendarCTA("https://api.example/ics?token=abc", "/portal/deals/1")
	if btnText != "Adicionar ao calendário" || btnURL != "https://api.example/ics?token=abc" {
		t.Errorf("cta principal: %q -> %q", btnText, btnURL)
	}
	if secondaryText != "Ver no portal" || secondaryURL != "/portal/deals/1" {
		t.Errorf("cta secundário: %q -> %q", secondaryText, secondaryURL)
	}
	// Sem arquivo (pendente, cancelado ou assinador desligado) o portal é o botão.
	btnText, btnURL, secondaryText, secondaryURL = appointmentCalendarCTA("", "/portal/deals/1")
	if btnText != "Ver no portal" || btnURL != "/portal/deals/1" || secondaryText != "" || secondaryURL != "" {
		t.Errorf("cta sem calendário: %q -> %q (%q/%q)", btnText, btnURL, secondaryText, secondaryURL)
	}
}
