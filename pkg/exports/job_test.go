package exports

import (
	"errors"
	"testing"
)

func TestNewJob_ValidatesInput(t *testing.T) {
	cases := []struct {
		name    string
		tenant  string
		who     string
		entity  EntityType
		format  Format
		wantErr string
	}{
		{"ok", "t1", "u1", "Lead", FormatCSV, ""},
		{"sem tenant", "", "u1", "Lead", FormatCSV, "tenant_id"},
		{"sem requested_by", "t1", "", "Lead", FormatCSV, "requested_by"},
		{"sem entity", "t1", "u1", "", FormatCSV, "entity_type"},
		{"format inválido", "t1", "u1", "Lead", Format("xml"), "format"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewJob(tc.tenant, tc.who, tc.entity, tc.format, nil)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("expected ok, got %v", err)
				}
				return
			}
			if err == nil || !contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error contendo %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestNewJob_GeneratesUUIDAndStatus(t *testing.T) {
	j, err := NewJob("t1", "u1", "Lead", FormatCSV, map[string]interface{}{"status": "NEW"})
	if err != nil {
		t.Fatalf("NewJob: %v", err)
	}
	if j.ID == "" {
		t.Error("ID não gerado")
	}
	if j.Status != StatusQueued {
		t.Errorf("Status = %q, want QUEUED", j.Status)
	}
	if j.Filters["status"] != "NEW" {
		t.Errorf("Filters não preservados: %v", j.Filters)
	}
	if j.CreatedAt.IsZero() {
		t.Error("CreatedAt não setado")
	}
}

func TestStatus_Transitions(t *testing.T) {
	j, _ := NewJob("t1", "u1", "Lead", FormatCSV, nil)

	// QUEUED → RUNNING
	if err := j.MarkRunning(); err != nil {
		t.Fatalf("MarkRunning: %v", err)
	}
	if j.Status != StatusRunning || j.StartedAt == nil {
		t.Errorf("RUNNING state wrong: %+v", j)
	}

	// RUNNING → RUNNING (idempotente)
	if err := j.MarkRunning(); err != nil {
		t.Errorf("idempotent MarkRunning: %v", err)
	}

	// RUNNING → SUCCEEDED
	if err := j.MarkSucceeded("exports", "lead/x.csv", "https://signed", 42); err != nil {
		t.Fatalf("MarkSucceeded: %v", err)
	}
	if j.RowCount != 42 || j.ResultURL == "" || j.CompletedAt == nil {
		t.Errorf("SUCCEEDED state wrong: %+v", j)
	}

	// SUCCEEDED → SUCCEEDED (idempotente)
	if err := j.MarkSucceeded("exports", "lead/x.csv", "url", 1); err != nil {
		t.Errorf("idempotent MarkSucceeded: %v", err)
	}

	// SUCCEEDED → FAILED bloqueado
	if err := j.MarkFailed("retry"); err == nil {
		t.Error("expected ErrInvalidTransition de SUCCEEDED")
	}
}

func TestMarkSucceeded_FromQueued_Rejects(t *testing.T) {
	j, _ := NewJob("t1", "u1", "Lead", FormatCSV, nil)
	// Pula MarkRunning — gen errado tentando ir direto.
	err := j.MarkSucceeded("b", "p", "u", 1)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestMarkFailed_FromRunning_Ok(t *testing.T) {
	j, _ := NewJob("t1", "u1", "Lead", FormatCSV, nil)
	_ = j.MarkRunning()
	if err := j.MarkFailed("network down"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	if j.Status != StatusFailed || j.ErrorMessage != "network down" {
		t.Errorf("FAILED state wrong: %+v", j)
	}
}

func TestMarkFailed_FromQueued_Ok(t *testing.T) {
	// Caso: generator falha sem nem ter dado MarkRunning (rejeição
	// upfront). MarkFailed deve aceitar.
	j, _ := NewJob("t1", "u1", "Lead", FormatCSV, nil)
	if err := j.MarkFailed("validation"); err != nil {
		t.Errorf("MarkFailed from QUEUED: %v", err)
	}
}

func TestStatus_IsTerminal(t *testing.T) {
	if !StatusSucceeded.IsTerminal() || !StatusFailed.IsTerminal() {
		t.Error("succeeded/failed devem ser terminais")
	}
	if StatusQueued.IsTerminal() || StatusRunning.IsTerminal() {
		t.Error("queued/running NÃO devem ser terminais")
	}
}

func TestFormat_ContentType(t *testing.T) {
	if FormatCSV.ContentType() != "text/csv; charset=utf-8" {
		t.Errorf("CSV content-type: %s", FormatCSV.ContentType())
	}
	if FormatPDF.ContentType() != "application/pdf" {
		t.Errorf("PDF content-type: %s", FormatPDF.ContentType())
	}
	if Format("xml").ContentType() != "application/octet-stream" {
		t.Errorf("unknown format default: %s", Format("xml").ContentType())
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
