package withdrawal

import "testing"

// FIN-E2E-34: o settlement ref (PIX/TED end-to-end id) vive em SettlementRef e
// nunca sobrescreve o ExternalID — a chave idempotente imutável do saque.
func TestMarkSettledManually_DoesNotOverwriteExternalID(t *testing.T) {
	w, err := New("tenant-1", 1000, "saque", "requester")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	immutableExternalID := w.ExternalID

	if err := w.Approve("approver"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := w.MarkSettledManually("operator", "PIX-REF-123", "https://proof.example/comprovante.png"); err != nil {
		t.Fatalf("MarkSettledManually: %v", err)
	}

	if w.Status != StatusComplete {
		t.Fatalf("expected COMPLETE, got %s", w.Status)
	}
	if w.ExternalID != immutableExternalID {
		t.Fatalf("ExternalID must remain immutable: got %q want %q", w.ExternalID, immutableExternalID)
	}
	if w.SettlementRef != "PIX-REF-123" {
		t.Fatalf("SettlementRef should carry the operator reference, got %q", w.SettlementRef)
	}
	if w.ReceiptURL != "https://proof.example/comprovante.png" {
		t.Fatalf("ReceiptURL should carry the proof, got %q", w.ReceiptURL)
	}
}

// FIN-E2E-34: settlement ref fica vazio quando o operador não informa a
// referência externa, mantendo ExternalID intacto.
func TestMarkSettledManually_OptionalExternalRef(t *testing.T) {
	w, err := New("tenant-1", 1000, "saque", "requester")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	immutableExternalID := w.ExternalID

	if err := w.Approve("approver"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := w.MarkSettledManually("operator", "", ""); err != nil {
		t.Fatalf("MarkSettledManually: %v", err)
	}

	if w.ExternalID != immutableExternalID {
		t.Fatalf("ExternalID must remain immutable, got %q want %q", w.ExternalID, immutableExternalID)
	}
	if w.SettlementRef != "" {
		t.Fatalf("SettlementRef should stay empty when no ref is supplied, got %q", w.SettlementRef)
	}
}
