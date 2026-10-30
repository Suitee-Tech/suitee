package service

import (
	"context"
	"testing"
)

func TestEmailPrefsService_ListDefaultsAndLocks(t *testing.T) {
	prefs := &fakePrefsRepo{disabled: map[string]bool{}}
	svc := NewEmailPrefsService(prefs)

	list, err := svc.List(context.Background(), "user-1", "tenant-1", "operador")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 6 {
		t.Fatalf("categories = %d, want 6", len(list))
	}
	for _, c := range list {
		if !c.Enabled {
			t.Errorf("category %s must default to enabled", c.Category)
		}
		if c.Label == "" {
			t.Errorf("category %s missing label", c.Category)
		}
		switch c.Category {
		case "lead_assigned":
			if !c.Locked {
				t.Errorf("lead_assigned must be locked for operators")
			}
		case "appointments":
			if c.Locked {
				t.Errorf("appointments must NOT be locked for operators")
			}
		}
	}

	// Cliente do portal: appointments travado, lead_assigned livre.
	list, err = svc.List(context.Background(), "user-1", "tenant-1", "cliente")
	if err != nil {
		t.Fatalf("List (cliente): %v", err)
	}
	for _, c := range list {
		switch c.Category {
		case "appointments":
			if !c.Locked {
				t.Errorf("appointments must be locked for clientes")
			}
		case "lead_assigned":
			if c.Locked {
				t.Errorf("lead_assigned must NOT be locked for clientes")
			}
		}
	}
}

func TestEmailPrefsService_UpdateRespectsLocksAndPersists(t *testing.T) {
	prefs := &fakePrefsRepo{disabled: map[string]bool{}}
	svc := NewEmailPrefsService(prefs)
	ctx := context.Background()

	// Operador tenta desligar lead_assigned (travado) + proposals (livre).
	list, err := svc.Update(ctx, "user-1", "tenant-1", "operador", map[string]bool{
		"lead_assigned": false,
		"proposals":     false,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	for _, c := range list {
		switch c.Category {
		case "lead_assigned":
			if !c.Enabled {
				t.Errorf("locked category must stay enabled")
			}
		case "proposals":
			if c.Enabled {
				t.Errorf("proposals must be disabled after update")
			}
		}
	}
	// E o repo persistiu apenas o toggle livre.
	if !prefs.disabled["user-1|tenant-1|proposals"] {
		t.Error("proposals opt-out missing from repo")
	}
	if prefs.disabled["user-1|tenant-1|lead_assigned"] {
		t.Error("locked category must not be persisted")
	}
}

func TestEmailPrefsService_UnknownCategoryRejected(t *testing.T) {
	svc := NewEmailPrefsService(&fakePrefsRepo{disabled: map[string]bool{}})
	_, err := svc.Update(context.Background(), "user-1", "tenant-1", "operador", map[string]bool{
		"not_a_category": true,
	})
	if err == nil {
		t.Fatal("unknown category must be rejected")
	}
}

func TestIsEmailPrefCategoryLocked(t *testing.T) {
	if !IsEmailPrefCategoryLocked(CategoryLeadAssigned, "corretor") {
		t.Error("lead_assigned locked for corretor")
	}
	if IsEmailPrefCategoryLocked(CategoryLeadAssigned, "cliente") {
		t.Error("lead_assigned not locked for cliente")
	}
	if !IsEmailPrefCategoryLocked(CategoryAppointments, "cliente") {
		t.Error("appointments locked for cliente")
	}
	if IsEmailPrefCategoryLocked(CategoryProposals, "cliente") {
		t.Error("proposals not locked")
	}
}
