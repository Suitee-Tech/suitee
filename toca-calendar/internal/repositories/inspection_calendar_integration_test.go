package repositories_test

import (
	"context"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-calendar/internal/models/appointment"
	"github.com/oginoino/toca/toca-calendar/internal/repositories"
	"github.com/oginoino/toca/toca-calendar/internal/services"
	"os"
	"strings"
	"testing"
	"time"
)

func TestInspectionProjectionPostgres(t *testing.T) {
	uri := os.Getenv("INSPECTION_REVIEW_TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("requires disposable toca_inspection_review_qa_* database")
	}
	db, err := sqlx.Connect("pgx", uri)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err := db.Get(&name, "SELECT current_database()"); err != nil || !strings.HasPrefix(name, "toca_inspection_review_qa_") {
		t.Fatal("refusing non-QA database")
	}
	if _, err := db.Exec(`CREATE SCHEMA calendar;CREATE TABLE calendar.appointments(id uuid PRIMARY KEY, tenant_id uuid NOT NULL, metadata jsonb NOT NULL, created_at timestamptz DEFAULT now(), updated_at timestamptz DEFAULT now(), deleted_at timestamptz)`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA calendar CASCADE")
	repo := repositories.NewAppointmentPostgresRepository(&database.PostgresDB{DB: db})
	handler := services.NewInspectionCalendarHandler(repo)
	stamp := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	p := contracts.InspectionCalendarPayload{InspectionID: uuid.NewString(), TenantID: uuid.NewString(), ContractID: uuid.NewString(), PropertyID: uuid.NewString(), InspectorID: uuid.NewString(), InspectorName: "Ana", ScheduledBy: uuid.NewString(), Status: "Agendada", Type: "ENTRY", ScheduledDate: stamp, UpdatedAt: stamp, TimeZone: "America/Sao_Paulo"}
	deliver := func(p contracts.InspectionCalendarPayload) {
		t.Helper()
		if err := handler(context.Background(), contracts.InspectionCalendarEvent{ID: uuid.NewString(), Data: p}); err != nil {
			t.Fatal(err)
		}
	}
	deliver(p)
	deliver(p)
	newer := p
	newer.ScheduledDate = stamp.Add(time.Hour)
	newer.UpdatedAt = stamp.Add(time.Second)
	deliver(newer)
	deliver(p)
	rows, total, err := repo.List(context.Background(), p.TenantID, appointment.ListFilters{AccessAgentID: p.ScheduledBy})
	if err != nil || total != 1 || rows[0].Time != "10:00" {
		t.Fatalf("schedule duplicated or stale: %+v %d %v", rows, total, err)
	}
	newer.Status = "Cancelada"
	newer.UpdatedAt = stamp.Add(2 * time.Second)
	deliver(newer)
	deliver(p)
	rows, total, err = repo.List(context.Background(), p.TenantID, appointment.ListFilters{AccessAgentID: p.ScheduledBy, TeamScope: true})
	if err != nil || total != 1 || rows[0].Status != appointment.StatusCancelado {
		t.Fatalf("cancel not projected: %+v %d %v", rows, total, err)
	}
	_, total, err = repo.List(context.Background(), uuid.NewString(), appointment.ListFilters{})
	if err != nil || total != 0 {
		t.Fatal("foreign tenant can list projection", err)
	}
}
