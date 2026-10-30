package service

import "testing"

func TestDealStatusLabel(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"NEGOTIATION", "Em Negociação"},
		{"PROPOSAL", "Proposta"},
		{"ANALYSIS", "Análise"},
		{"CONTRACT", "Contrato"},
		{"CONVERTED", "Concluído"},
		{"CANCELLED", "Perdido"},
		{"UNKNOWN_STATUS", "UNKNOWN_STATUS"},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := dealStatusLabel(c.in)
			if got != c.want {
				t.Errorf("dealStatusLabel(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestTicketStatusLabel(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"open", "Aberto"},
		{"assigned", "Atribuído"},
		{"in_progress", "Em andamento"},
		{"awaiting_client", "Aguardando cliente"},
		{"resolved", "Resolvido"},
		{"closed", "Fechado"},
		{"cancelled", "Cancelado"},
		{"weird_status", "weird_status"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := ticketStatusLabel(c.in)
			if got != c.want {
				t.Errorf("ticketStatusLabel(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestTicketTypeLabel(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"support", "Suporte"},
		{"maintenance", "Manutenção"},
		{"billing", "Financeiro"},
		{"documentation", "Documentação"},
		{"general", "Geral"},
		{"custom", "custom"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := ticketTypeLabel(c.in)
			if got != c.want {
				t.Errorf("ticketTypeLabel(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
