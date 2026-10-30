package services

import (
	"testing"

	"github.com/oginoino/toca/pkg/clients"
)

// TestFilterPublicTicketComments garante que notas internas nunca vazam
// para o portal cliente. Regressão direta de #449 (J06 privacy).
func TestFilterPublicTicketComments(t *testing.T) {
	t.Run("strips internal comments", func(t *testing.T) {
		input := []clients.TicketComment{
			{ID: "1", Visibility: "public", Content: "Olá, sua solicitação foi recebida"},
			{ID: "2", Visibility: "internal", Content: "Cliente já reclamou disso antes, ver histórico"},
			{ID: "3", Visibility: "public", Content: "Resolvido — fechando ticket"},
		}
		got := filterPublicTicketComments(input)
		if len(got) != 2 {
			t.Fatalf("expected 2 public comments, got %d", len(got))
		}
		for _, c := range got {
			if c.ID == "2" {
				t.Fatal("internal comment id=2 leaked into output — PRIVACY ISSUE")
			}
		}
	})

	t.Run("empty visibility treated as public (legacy backward-compat)", func(t *testing.T) {
		input := []clients.TicketComment{
			{ID: "1", Visibility: "", Content: "Comment antigo sem visibility"},
		}
		got := filterPublicTicketComments(input)
		if len(got) != 1 {
			t.Errorf("legacy comment should be returned (treated as public), got %d", len(got))
		}
	})

	t.Run("unknown visibility values are stripped (safe by default)", func(t *testing.T) {
		input := []clients.TicketComment{
			{ID: "1", Visibility: "weird", Content: "Tipo desconhecido"},
			{ID: "2", Visibility: "public", Content: "OK"},
		}
		got := filterPublicTicketComments(input)
		if len(got) != 1 {
			t.Errorf("unknown visibility values should be stripped, got %d items", len(got))
		}
		if got[0].ID != "2" {
			t.Errorf("expected only id=2 to pass, got %q", got[0].ID)
		}
	})

	t.Run("nil or empty input returns empty slice", func(t *testing.T) {
		if got := filterPublicTicketComments(nil); len(got) != 0 {
			t.Errorf("nil input should return empty, got len=%d", len(got))
		}
		if got := filterPublicTicketComments([]clients.TicketComment{}); len(got) != 0 {
			t.Errorf("empty input should return empty, got len=%d", len(got))
		}
	})

	t.Run("all internal returns empty", func(t *testing.T) {
		input := []clients.TicketComment{
			{ID: "1", Visibility: "internal"},
			{ID: "2", Visibility: "internal"},
		}
		got := filterPublicTicketComments(input)
		if len(got) != 0 {
			t.Errorf("all-internal input should return empty, got %d", len(got))
		}
	})
}
