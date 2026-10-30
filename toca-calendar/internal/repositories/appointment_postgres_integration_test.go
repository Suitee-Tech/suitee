package repositories

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-calendar/internal/models/appointment"
)

func TestAppointmentPostgresAtomicReservation(t *testing.T) {
	uri := os.Getenv("CALENDAR_REVIEW_TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("set CALENDAR_REVIEW_TEST_DATABASE_URL to a disposable toca_calendar_review_qa_* database")
	}
	db, err := sqlx.Connect("pgx", uri)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var name string
	if err := db.Get(&name, "SELECT current_database()"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "toca_calendar_review_qa_") {
		t.Fatal("refusing non-QA database")
	}
	if _, err := db.Exec(`CREATE SCHEMA calendar; CREATE TABLE calendar.appointments (
		id uuid PRIMARY KEY, tenant_id uuid NOT NULL, metadata jsonb NOT NULL,
		created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DROP SCHEMA calendar CASCADE"); err != nil {
			t.Errorf("cleanup QA schema: %v", err)
		}
	})
	migration, err := os.ReadFile("../../../supabase/migrations/20260916180000_calendar_agent_day_reservations.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatalf("apply reservation index migration: %v", err)
	}
	visibilityMigration, err := os.ReadFile("../../../supabase/migrations/20260925090000_calendar_visibility_indexes.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(visibilityMigration)); err != nil {
		t.Fatalf("apply visibility index migration: %v", err)
	}
	repo := NewAppointmentPostgresRepository(&database.PostgresDB{DB: db})
	tenant, agent := uuid.NewString(), uuid.NewString()
	makeAppointment := func(start, end string) *appointment.Appointment {
		a := appointment.New(tenant, "Visita", "2099-10-20", start, end, appointment.TypeVisita, appointment.PriorityMedia, "Cliente", "Corretor")
		a.AgentID = agent
		return a
	}
	ctx := context.Background()
	// Both callers start together; the repository check/write must coordinate
	// without relying on the service's earlier nontransactional preflight.
	start := make(chan struct{})
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() { <-start; results <- repo.Create(ctx, makeAppointment("09:00", "10:00")) }()
	}
	close(start)
	saved, conflicts := 0, 0
	for i := 0; i < 8; i++ {
		err := <-results
		if err == nil {
			saved++
		} else if errors.Is(err, appointment.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if saved != 1 || conflicts != 7 {
		t.Fatalf("overlapping creates: saved=%d conflicts=%d", saved, conflicts)
	}
	adjacent := makeAppointment("10:00", "11:00")
	if err := repo.Create(ctx, adjacent); err != nil {
		t.Fatalf("adjacent slot rejected: %v", err)
	}
	otherAgency := makeAppointment("09:00", "10:00")
	otherAgency.TenantID = uuid.NewString()
	if err := repo.Create(ctx, otherAgency); err != nil {
		t.Fatalf("different agency serialized as conflict: %v", err)
	}
	otherAgent := makeAppointment("09:00", "10:00")
	otherAgent.AgentID = uuid.NewString()
	if err := repo.Create(ctx, otherAgent); err != nil {
		t.Fatalf("different agent rejected: %v", err)
	}
	// Two previously free slots cannot both be moved into one occupied interval.
	first, second := makeAppointment("12:00", "13:00"), makeAppointment("13:00", "14:00")
	if err := repo.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatal(err)
	}
	first.Time, first.EndTime = "15:00", "16:00"
	second.Time, second.EndTime = "15:30", "16:30"
	start = make(chan struct{})
	for _, a := range []*appointment.Appointment{first, second} {
		go func(a *appointment.Appointment) { <-start; results <- repo.Update(ctx, a) }(a)
	}
	close(start)
	saved, conflicts = 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			saved++
		} else if errors.Is(err, appointment.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if saved != 1 || conflicts != 1 {
		t.Fatalf("overlapping updates: saved=%d conflicts=%d", saved, conflicts)
	}
	// Cancelling releases the interval, while reactivation checks it again.
	adjacent.Status = appointment.StatusCancelado
	if err := repo.Update(ctx, adjacent); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, makeAppointment("10:00", "11:00")); err != nil {
		t.Fatalf("cancelled slot did not release: %v", err)
	}
	adjacent.Status = appointment.StatusAgendado
	if err := repo.Update(ctx, adjacent); !errors.Is(err, appointment.ErrConflict) {
		t.Fatalf("reactivation accepted overlap: %v", err)
	}
	// Distinct edits of the same row cannot both overwrite the same revision.
	versioned := makeAppointment("18:00", "19:00")
	versioned.AgentID = ""
	if err := repo.Create(ctx, versioned); err != nil {
		t.Fatal(err)
	}
	left, right := *versioned, *versioned
	left.Title = "New title"
	right.Status = appointment.StatusConfirmado
	start = make(chan struct{})
	for _, a := range []*appointment.Appointment{&left, &right} {
		go func(a *appointment.Appointment) { <-start; results <- repo.Update(ctx, a) }(a)
	}
	close(start)
	saved, conflicts = 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			saved++
		} else if errors.Is(err, appointment.ErrConcurrentModification) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if saved != 1 || conflicts != 1 {
		t.Fatalf("same revision writes: saved=%d conflicts=%d", saved, conflicts)
	}
	current, err := repo.GetByID(ctx, versioned.ID)
	if err != nil {
		t.Fatal(err)
	}
	current.Description = "Next edit"
	if err := repo.Update(ctx, current); err != nil {
		t.Fatalf("fresh revision cannot update: %v", err)
	}

	// Access filters run in SQL before count/pagination: brokers only see their
	// own agenda; managers see professional team events, never private ones.
	accessTenant, owner, colleague := uuid.NewString(), uuid.NewString(), uuid.NewString()
	accessHour := 9
	addAccessAppointment := func(id, agentID string, visibility appointment.Visibility, professional bool) {
		startTime, endTime := fmt.Sprintf("%02d:00", accessHour), fmt.Sprintf("%02d:00", accessHour+1)
		accessHour++
		a := appointment.New(accessTenant, id, "2099-11-20", startTime, endTime, appointment.Type("Outro"), appointment.PriorityMedia, "", "Corretor")
		a.ID, a.AgentID, a.Visibility = id, agentID, visibility
		if professional {
			a.PropertyID = uuid.NewString()
		}
		if err := repo.Create(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	addAccessAppointment(uuid.NewString(), owner, appointment.VisibilityPrivate, false)
	teamID, privateID, legacyProfessionalID, legacyPrivateID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	addAccessAppointment(teamID, colleague, appointment.VisibilityTeam, false)
	addAccessAppointment(privateID, colleague, appointment.VisibilityPrivate, true)
	addAccessAppointment(legacyProfessionalID, colleague, "", true)
	addAccessAppointment(legacyPrivateID, colleague, "", false)
	assertVisible := func(teamScope bool, want int, forbidden ...string) {
		rows, total, err := repo.List(ctx, accessTenant, appointment.ListFilters{AccessAgentID: owner, TeamScope: teamScope, Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != want || total != int64(want) {
			t.Fatalf("team=%v rows=%d total=%d want=%d", teamScope, len(rows), total, want)
		}
		for _, row := range rows {
			for _, id := range forbidden {
				if row.ID == id {
					t.Fatalf("team=%v leaked private appointment %s", teamScope, id)
				}
			}
		}
	}
	assertVisible(false, 1, teamID, privateID, legacyProfessionalID, legacyPrivateID)
	assertVisible(true, 3, privateID, legacyPrivateID)

}
