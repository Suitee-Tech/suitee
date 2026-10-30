package audit

import (
	"strings"
	"testing"
)

func TestBuildListQuery_Defaults(t *testing.T) {
	// Sem filtros → ORDER + LIMIT 100, sem WHERE.
	q, args := buildListQuery(ListFilters{})
	if !strings.Contains(q, "ORDER BY created_at DESC") {
		t.Errorf("missing ORDER BY: %s", q)
	}
	if strings.Contains(q, "WHERE") {
		t.Errorf("should not include WHERE when no filters: %s", q)
	}
	if !strings.Contains(q, "LIMIT $1") {
		t.Errorf("LIMIT placeholder wrong: %s", q)
	}
	if len(args) != 1 {
		t.Errorf("expected 1 arg (limit only), got %d", len(args))
	}
	if args[0] != 100 {
		t.Errorf("default limit should be 100, got %v", args[0])
	}
}

func TestBuildListQuery_LimitClamping(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{0, 100},     // default
		{-5, 100},    // negative falls back
		{50, 50},     // normal
		{500, 500},   // max
		{501, 100},   // out of range → default (defensive)
		{10000, 100}, // very large → default
	}
	for _, tc := range cases {
		_, args := buildListQuery(ListFilters{Limit: tc.in})
		got := args[len(args)-1]
		if got != tc.want {
			t.Errorf("Limit %d: want %d, got %v", tc.in, tc.want, got)
		}
	}
}

func TestBuildListQuery_TenantFilter(t *testing.T) {
	q, args := buildListQuery(ListFilters{TenantID: "t1"})
	if !strings.Contains(q, "tenant_id = $1::uuid") {
		t.Errorf("missing tenant filter: %s", q)
	}
	if args[0] != "t1" {
		t.Errorf("first arg should be tenant_id, got %v", args[0])
	}
}

func TestBuildListQuery_ActionExact(t *testing.T) {
	q, args := buildListQuery(ListFilters{Action: "auth.banking_updated"})
	if !strings.Contains(q, "action = $1") {
		t.Errorf("missing action filter: %s", q)
	}
	if args[0] != "auth.banking_updated" {
		t.Errorf("action arg wrong: %v", args[0])
	}
}

func TestBuildListQuery_ActionPrefix(t *testing.T) {
	q, args := buildListQuery(ListFilters{ActionPrefix: "auth."})
	if !strings.Contains(q, "action LIKE $1") {
		t.Errorf("missing prefix LIKE: %s", q)
	}
	if args[0] != "auth.%" {
		t.Errorf("expected 'auth.%%', got %v", args[0])
	}
}

func TestBuildListQuery_EntityFilter(t *testing.T) {
	q, args := buildListQuery(ListFilters{EntityType: "Deal", EntityID: "d-1"})
	if !strings.Contains(q, "entity_type = $1") {
		t.Errorf("missing entity_type: %s", q)
	}
	if !strings.Contains(q, "entity_id = $2") {
		t.Errorf("missing entity_id: %s", q)
	}
	if args[0] != "Deal" || args[1] != "d-1" {
		t.Errorf("entity args wrong: %v", args)
	}
}

func TestBuildListQuery_DateRange(t *testing.T) {
	q, _ := buildListQuery(ListFilters{From: "2026-05-01T00:00:00Z", To: "2026-05-31T23:59:59Z"})
	if !strings.Contains(q, "created_at >= $1::timestamptz") {
		t.Errorf("missing From: %s", q)
	}
	if !strings.Contains(q, "created_at <= $2::timestamptz") {
		t.Errorf("missing To: %s", q)
	}
}

func TestBuildListQuery_CursorPagination(t *testing.T) {
	q, args := buildListQuery(ListFilters{CreatedBefore: "2026-05-19T10:00:00Z", Limit: 50})
	if !strings.Contains(q, "created_at < $1::timestamptz") {
		t.Errorf("missing cursor: %s", q)
	}
	if args[0] != "2026-05-19T10:00:00Z" {
		t.Errorf("cursor arg wrong: %v", args[0])
	}
	if args[1] != 50 {
		t.Errorf("limit not 50: %v", args[1])
	}
}

func TestBuildListQuery_AllFiltersCombined(t *testing.T) {
	// Verifica que múltiplos filtros combinam com AND e que placeholders
	// estão na ordem correta.
	q, args := buildListQuery(ListFilters{
		TenantID:      "t-1",
		Action:        "auth.banking_updated",
		EntityType:    "User",
		ActorID:       "u-1",
		From:          "2026-01-01T00:00:00Z",
		CreatedBefore: "2026-05-19T10:00:00Z",
		Limit:         200,
	})
	if !strings.Contains(q, "WHERE tenant_id = $1::uuid AND action = $2 AND entity_type = $3 AND actor_id = $4::uuid AND created_at >= $5::timestamptz AND created_at < $6::timestamptz") {
		t.Errorf("AND chain wrong: %s", q)
	}
	if !strings.Contains(q, "LIMIT $7") {
		t.Errorf("LIMIT placeholder should be $7 (last after 6 filters): %s", q)
	}
	if len(args) != 7 {
		t.Errorf("expected 7 args, got %d", len(args))
	}
}

func TestBuildListQuery_ActorIDOnly(t *testing.T) {
	// Auditor procurando "tudo que o usuário X fez".
	q, _ := buildListQuery(ListFilters{ActorID: "u-malicious"})
	if !strings.Contains(q, "actor_id = $1::uuid") {
		t.Errorf("missing actor: %s", q)
	}
}

func TestBuildListQuery_OrderingPreserved(t *testing.T) {
	// Mesmo com vários filtros, ORDER BY e LIMIT vêm no final.
	q, _ := buildListQuery(ListFilters{TenantID: "t", Action: "x"})
	orderIdx := strings.Index(q, "ORDER BY")
	limitIdx := strings.Index(q, "LIMIT")
	if orderIdx == -1 || limitIdx == -1 || orderIdx >= limitIdx {
		t.Errorf("ORDER/LIMIT misplaced: %s", q)
	}
	whereIdx := strings.Index(q, "WHERE")
	if whereIdx == -1 || whereIdx >= orderIdx {
		t.Errorf("WHERE should come before ORDER: %s", q)
	}
}
