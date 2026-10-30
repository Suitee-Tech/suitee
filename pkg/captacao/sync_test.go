package captacao

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubPropertyLinker struct {
	calls []addOwnerCall
	err   error
}
type addOwnerCall struct{ tenantID, propertyID, leadID string; percentage float64 }

func (s *stubPropertyLinker) AddOwner(ctx context.Context, tenantID, propertyID, leadID string, percentage float64) error {
	s.calls = append(s.calls, addOwnerCall{tenantID, propertyID, leadID, percentage})
	return s.err
}

type stubLeadLinker struct {
	calls []setCaptCall
	err   error
}
type setCaptCall struct{ tenantID, leadID, propertyID string }

func (s *stubLeadLinker) SetCaptacaoProperty(ctx context.Context, tenantID, leadID, propertyID string) error {
	s.calls = append(s.calls, setCaptCall{tenantID, leadID, propertyID})
	return s.err
}

type capturingLogger struct{ msgs []string }

func (c *capturingLogger) Printf(format string, args ...interface{}) {
	c.msgs = append(c.msgs, format)
}

func TestFromLead_DefaultsPercentageTo100(t *testing.T) {
	prop := &stubPropertyLinker{}
	syncer := NewSyncer(prop, nil)

	syncer.FromLead(context.Background(), "tenant-A", "lead-1", "prop-1", 0)

	if len(prop.calls) != 1 {
		t.Fatalf("expected 1 AddOwner call, got %d", len(prop.calls))
	}
	if got := prop.calls[0].percentage; got != 100 {
		t.Errorf("expected percentage default 100, got %v", got)
	}
}

func TestFromLead_NoopOnEmptyArgs(t *testing.T) {
	prop := &stubPropertyLinker{}
	syncer := NewSyncer(prop, nil)

	syncer.FromLead(context.Background(), "", "lead-1", "prop-1", 50)
	syncer.FromLead(context.Background(), "tenant-A", "", "prop-1", 50)
	syncer.FromLead(context.Background(), "tenant-A", "lead-1", "", 50)

	if len(prop.calls) != 0 {
		t.Errorf("expected 0 AddOwner calls when any arg is empty, got %d", len(prop.calls))
	}
}

func TestFromLead_LogsAndContinuesOnError(t *testing.T) {
	prop := &stubPropertyLinker{err: errors.New("boom")}
	logger := &capturingLogger{}
	syncer := NewSyncer(prop, nil)
	syncer.Logger = logger

	syncer.FromLead(context.Background(), "tenant-A", "lead-1", "prop-1", 100)

	if len(logger.msgs) != 1 {
		t.Fatalf("expected 1 log message, got %d", len(logger.msgs))
	}
	if !strings.Contains(logger.msgs[0], "AddOwner") {
		t.Errorf("expected log to mention AddOwner, got %q", logger.msgs[0])
	}
}

func TestFromProperty_CallsLeadLinker(t *testing.T) {
	lead := &stubLeadLinker{}
	syncer := NewSyncer(nil, lead)

	syncer.FromProperty(context.Background(), "tenant-A", "prop-1", "lead-1")

	if len(lead.calls) != 1 {
		t.Fatalf("expected 1 SetCaptacaoProperty call, got %d", len(lead.calls))
	}
	c := lead.calls[0]
	if c.tenantID != "tenant-A" || c.leadID != "lead-1" || c.propertyID != "prop-1" {
		t.Errorf("unexpected args: %+v", c)
	}
}

func TestValidate(t *testing.T) {
	if err := (*Syncer)(nil).Validate(); err == nil {
		t.Error("expected nil-syncer Validate to fail")
	}
	if err := NewSyncer(nil, nil).Validate(); err == nil {
		t.Error("expected no-linkers Validate to fail")
	}
	if err := NewSyncer(&stubPropertyLinker{}, nil).Validate(); err != nil {
		t.Errorf("expected single-linker Validate to pass, got %v", err)
	}
	if err := NewSyncer(&stubPropertyLinker{}, &stubLeadLinker{}).Validate(); err != nil {
		t.Errorf("expected both-linkers Validate to pass, got %v", err)
	}
}
