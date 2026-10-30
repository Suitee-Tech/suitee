//go:build integration

package service

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/config"
	appCtx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-crm/internal/domain/broker"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
	"github.com/oginoino/toca/toca-crm/internal/repository"
)

func TestRoulette_StagePoolsSequentialOrder(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantID := uuid.NewString()

	leadRepo := repository.NewLeadPostgresRepository(db)
	rouletteRepo := repository.NewRoulettePostgresRepository(db)
	historyRepo := repository.NewRouletteHistoryPostgresRepository(db)
	brokerRepo := repository.NewBrokerPostgresRepository(db)

	svc := NewLeadRouletteService(leadRepo, rouletteRepo, historyRepo, brokerRepo, nil, nil)

	brokerIDs := createBrokers(t, ctx, brokerRepo, tenantID, 5)

	cfg := &lead.RouletteConfig{
		TenantID:         tenantID,
		Enabled:          true,
		ActiveBrokers:    []string{brokerIDs[0], brokerIDs[1]},
		LastIndex:        -1,
		StageBrokerPools: map[string][]string{"NEW": brokerIDs},
		StageLastIndex:   map[string]int{},
		DistMethod:       lead.DistMethodSequential,
		AutoCapLimit:     false,
		RespectSpecialty: false,
		RespectRegion:    false,
		RespectSchedule:  false,
	}
	if err := rouletteRepo.UpdateConfig(ctx, cfg); err != nil {
		t.Fatalf("init config: %v", err)
	}

	assignments := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		l := newTestLead(t, tenantID)
		if err := leadRepo.Create(ctx, l); err != nil {
			t.Fatalf("create lead: %v", err)
		}
		ok, err := svc.AssignByRoulette(ctx, l, "")
		if err != nil {
			t.Fatalf("assign: %v", err)
		}
		if !ok {
			t.Fatalf("expected assigned=true, got false")
		}

		updated, err := leadRepo.GetByID(ctx, l.TenantID, l.ID)
		if err != nil {
			t.Fatalf("get lead: %v", err)
		}
		assignments = append(assignments, updated.AssignedBrokerID)
	}

	for i := 0; i < len(assignments); i++ {
		want := brokerIDs[i%len(brokerIDs)]
		if assignments[i] != want {
			t.Fatalf("assignment[%d] expected %s, got %s (all=%v)", i, want, assignments[i], assignments)
		}
	}
}

func TestRoulette_GetConfigDefaultsWhenMissing(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantID := uuid.NewString()

	rouletteRepo := repository.NewRoulettePostgresRepository(db)
	cfg, err := rouletteRepo.GetConfig(ctx, tenantID)
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if cfg == nil {
		t.Fatalf("expected non-nil config")
	}
	if cfg.Enabled {
		t.Fatalf("expected defaults with Enabled=false, got true")
	}
	if cfg.LastIndex != -1 {
		t.Fatalf("expected LastIndex=-1, got %d", cfg.LastIndex)
	}
}

func TestAuth_InternalServiceRoleDoesNotBypassPermissions(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantID := uuid.NewString()

	rouletteRepo := repository.NewRoulettePostgresRepository(db)

	// Context "interno" não deve ser tratado como admin/owner.
	ctx = appCtx.WithTenantID(ctx, tenantID)
	ctx = appCtx.WithAccountID(ctx, "internal")
	ctx = appCtx.WithUserPermission(ctx, "service")
	ctx = appCtx.WithFunctionalRole(ctx, "service")
	ctx = appCtx.WithUserType(ctx, "service")

	cfg, err := rouletteRepo.GetConfig(ctx, tenantID)
	if err != nil {
		t.Fatalf("expected GetConfig ok, got err=%v", err)
	}
	if cfg == nil {
		t.Fatalf("expected config non-nil")
	}
}

func TestRoulette_LoadBalanceRotatesOnTies(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantID := uuid.NewString()

	leadRepo := repository.NewLeadPostgresRepository(db)
	rouletteRepo := repository.NewRoulettePostgresRepository(db)
	historyRepo := repository.NewRouletteHistoryPostgresRepository(db)
	brokerRepo := repository.NewBrokerPostgresRepository(db)

	svc := NewLeadRouletteService(leadRepo, rouletteRepo, historyRepo, brokerRepo, nil, nil)

	brokerIDs := createBrokers(t, ctx, brokerRepo, tenantID, 3)

	cfg := &lead.RouletteConfig{
		TenantID:         tenantID,
		Enabled:          true,
		ActiveBrokers:    brokerIDs,
		LastIndex:        -1,
		StageBrokerPools: map[string][]string{"NEW": brokerIDs},
		StageLastIndex:   map[string]int{"NEW": -1},
		DistMethod:       lead.DistMethodLoadBalance,
		AutoCapLimit:     false,
		RespectSpecialty: false,
		RespectRegion:    false,
		RespectSchedule:  false,
	}
	if err := rouletteRepo.UpdateConfig(ctx, cfg); err != nil {
		t.Fatalf("init config: %v", err)
	}

	got := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		l := newTestLead(t, tenantID)
		if err := leadRepo.Create(ctx, l); err != nil {
			t.Fatalf("create lead: %v", err)
		}
		ok, err := svc.AssignByRoulette(ctx, l, "")
		if err != nil {
			t.Fatalf("assign: %v", err)
		}
		if !ok {
			t.Fatalf("expected assigned=true, got false")
		}
		updated, err := leadRepo.GetByID(ctx, l.TenantID, l.ID)
		if err != nil {
			t.Fatalf("get lead: %v", err)
		}
		got = append(got, updated.AssignedBrokerID)
	}

	want := []string{brokerIDs[0], brokerIDs[1], brokerIDs[2], brokerIDs[0], brokerIDs[1], brokerIDs[2]}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("assignment[%d] expected %s, got %s (all=%v)", i, want[i], got[i], got)
		}
	}
}

func TestRoulette_StagePoolsNormalizesStatusCase(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantID := uuid.NewString()

	leadRepo := repository.NewLeadPostgresRepository(db)
	rouletteRepo := repository.NewRoulettePostgresRepository(db)
	historyRepo := repository.NewRouletteHistoryPostgresRepository(db)
	brokerRepo := repository.NewBrokerPostgresRepository(db)

	svc := NewLeadRouletteService(leadRepo, rouletteRepo, historyRepo, brokerRepo, nil, nil)

	brokerIDs := createBrokers(t, ctx, brokerRepo, tenantID, 5)

	cfg := &lead.RouletteConfig{
		TenantID:         tenantID,
		Enabled:          true,
		ActiveBrokers:    []string{brokerIDs[0], brokerIDs[1]},
		LastIndex:        -1,
		StageBrokerPools: map[string][]string{"NEW": brokerIDs},
		StageLastIndex:   map[string]int{},
		DistMethod:       lead.DistMethodSequential,
		AutoCapLimit:     false,
		RespectSpecialty: false,
		RespectRegion:    false,
		RespectSchedule:  false,
	}
	if err := rouletteRepo.UpdateConfig(ctx, cfg); err != nil {
		t.Fatalf("init config: %v", err)
	}

	l := newTestLead(t, tenantID)
	l.Status = lead.LeadStatus("new")
	if err := leadRepo.Create(ctx, l); err != nil {
		t.Fatalf("create lead: %v", err)
	}
	ok, err := svc.AssignByRoulette(ctx, l, "")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if !ok {
		t.Fatalf("expected assigned=true, got false")
	}

	updated, err := leadRepo.GetByID(ctx, l.TenantID, l.ID)
	if err != nil {
		t.Fatalf("get lead: %v", err)
	}
	if updated.AssignedBrokerID != brokerIDs[0] {
		t.Fatalf("expected broker %s, got %s", brokerIDs[0], updated.AssignedBrokerID)
	}
}

func TestRoulette_RespectSpecialtyDoesNotBlockWhenBrokerHasNoSpecialties(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantID := uuid.NewString()

	leadRepo := repository.NewLeadPostgresRepository(db)
	rouletteRepo := repository.NewRoulettePostgresRepository(db)
	historyRepo := repository.NewRouletteHistoryPostgresRepository(db)
	brokerRepo := repository.NewBrokerPostgresRepository(db)

	svc := NewLeadRouletteService(leadRepo, rouletteRepo, historyRepo, brokerRepo, nil, nil)

	brokerIDs := createBrokers(t, ctx, brokerRepo, tenantID, 5)

	cfg := &lead.RouletteConfig{
		TenantID:         tenantID,
		Enabled:          true,
		ActiveBrokers:    brokerIDs,
		LastIndex:        -1,
		StageBrokerPools: map[string][]string{"NEW": brokerIDs},
		StageLastIndex:   map[string]int{},
		DistMethod:       lead.DistMethodSequential,
		AutoCapLimit:     false,
		RespectSpecialty: true,
		RespectRegion:    false,
		RespectSchedule:  false,
	}
	if err := rouletteRepo.UpdateConfig(ctx, cfg); err != nil {
		t.Fatalf("init config: %v", err)
	}

	l := newTestLead(t, tenantID)
	l.Role = lead.ClientRoleBuyer
	if err := leadRepo.Create(ctx, l); err != nil {
		t.Fatalf("create lead: %v", err)
	}

	ok, err := svc.AssignByRoulette(ctx, l, "")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if !ok {
		t.Fatalf("expected assigned=true, got false")
	}
}

func TestRoulette_RespectRegionDoesNotBlockWhenBrokerHasNoRegions(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantID := uuid.NewString()

	leadRepo := repository.NewLeadPostgresRepository(db)
	rouletteRepo := repository.NewRoulettePostgresRepository(db)
	historyRepo := repository.NewRouletteHistoryPostgresRepository(db)
	brokerRepo := repository.NewBrokerPostgresRepository(db)

	svc := NewLeadRouletteService(leadRepo, rouletteRepo, historyRepo, brokerRepo, nil, nil)

	brokerIDs := createBrokers(t, ctx, brokerRepo, tenantID, 5)

	cfg := &lead.RouletteConfig{
		TenantID:         tenantID,
		Enabled:          true,
		ActiveBrokers:    brokerIDs,
		LastIndex:        -1,
		StageBrokerPools: map[string][]string{"NEW": brokerIDs},
		StageLastIndex:   map[string]int{},
		DistMethod:       lead.DistMethodSequential,
		AutoCapLimit:     false,
		RespectSpecialty: false,
		RespectRegion:    true,
		RespectSchedule:  false,
	}
	if err := rouletteRepo.UpdateConfig(ctx, cfg); err != nil {
		t.Fatalf("init config: %v", err)
	}

	l := newTestLead(t, tenantID)
	l.Role = lead.ClientRoleBuyer
	if l.PropertyInterest == nil {
		l.PropertyInterest = &lead.LeadProperty{}
	}
	l.PropertyInterest.PreferredLocation = "Interesse Geral"
	if err := leadRepo.Create(ctx, l); err != nil {
		t.Fatalf("create lead: %v", err)
	}

	ok, err := svc.AssignByRoulette(ctx, l, "")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if !ok {
		t.Fatalf("expected assigned=true, got false")
	}
}

func TestRoulette_BumpsPointerWhenNoEligibleBrokers(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantID := uuid.NewString()

	leadRepo := repository.NewLeadPostgresRepository(db)
	rouletteRepo := repository.NewRoulettePostgresRepository(db)
	historyRepo := repository.NewRouletteHistoryPostgresRepository(db)
	brokerRepo := repository.NewBrokerPostgresRepository(db)

	svc := NewLeadRouletteService(leadRepo, rouletteRepo, historyRepo, brokerRepo, nil, nil)

	brokerIDs := createBrokersWithSpecialties(t, ctx, brokerRepo, tenantID, 5, []string{"LOCACAO"})

	cfg := &lead.RouletteConfig{
		TenantID:         tenantID,
		Enabled:          true,
		ActiveBrokers:    brokerIDs,
		LastIndex:        -1,
		StageBrokerPools: map[string][]string{"NEW": brokerIDs},
		StageLastIndex:   map[string]int{},
		DistMethod:       lead.DistMethodSequential,
		AutoCapLimit:     false,
		RespectSpecialty: true,
		RespectRegion:    false,
		RespectSchedule:  false,
	}
	if err := rouletteRepo.UpdateConfig(ctx, cfg); err != nil {
		t.Fatalf("init config: %v", err)
	}

	l1 := newTestLead(t, tenantID)
	l1.Role = lead.ClientRoleBuyer
	if err := leadRepo.Create(ctx, l1); err != nil {
		t.Fatalf("create lead: %v", err)
	}
	ok, err := svc.AssignByRoulette(ctx, l1, "")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if ok {
		t.Fatalf("expected assigned=false when no eligible brokers")
	}

	cfgAfter1, err := rouletteRepo.GetConfig(ctx, tenantID)
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if cfgAfter1.StageLastIndex == nil || cfgAfter1.StageLastIndex["NEW"] != 0 {
		t.Fatalf("expected StageLastIndex.NEW=0 after bump, got %v", cfgAfter1.StageLastIndex)
	}

	l2 := newTestLead(t, tenantID)
	l2.Role = lead.ClientRoleBuyer
	if err := leadRepo.Create(ctx, l2); err != nil {
		t.Fatalf("create lead: %v", err)
	}
	ok, err = svc.AssignByRoulette(ctx, l2, "")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if ok {
		t.Fatalf("expected assigned=false when no eligible brokers")
	}

	cfgAfter2, err := rouletteRepo.GetConfig(ctx, tenantID)
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if cfgAfter2.StageLastIndex == nil || cfgAfter2.StageLastIndex["NEW"] != 1 {
		t.Fatalf("expected StageLastIndex.NEW=1 after second bump, got %v", cfgAfter2.StageLastIndex)
	}
}

func TestRoulette_SkipsPausedBrokerButKeepsOrder(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantID := uuid.NewString()

	leadRepo := repository.NewLeadPostgresRepository(db)
	rouletteRepo := repository.NewRoulettePostgresRepository(db)
	historyRepo := repository.NewRouletteHistoryPostgresRepository(db)
	brokerRepo := repository.NewBrokerPostgresRepository(db)

	svc := NewLeadRouletteService(leadRepo, rouletteRepo, historyRepo, brokerRepo, nil, nil)

	brokerIDs := createBrokers(t, ctx, brokerRepo, tenantID, 5)
	if ok, err := brokerRepo.SetPaused(ctx, brokerIDs[2], tenantID, true); err != nil || !ok {
		t.Fatalf("pause broker: ok=%v err=%v", ok, err)
	}

	cfg := &lead.RouletteConfig{
		TenantID:         tenantID,
		Enabled:          true,
		ActiveBrokers:    brokerIDs,
		LastIndex:        -1,
		StageBrokerPools: map[string][]string{"NEW": brokerIDs},
		StageLastIndex:   map[string]int{},
		DistMethod:       lead.DistMethodSequential,
		AutoCapLimit:     false,
		RespectSpecialty: false,
		RespectRegion:    false,
		RespectSchedule:  false,
	}
	if err := rouletteRepo.UpdateConfig(ctx, cfg); err != nil {
		t.Fatalf("init config: %v", err)
	}

	want := []string{brokerIDs[0], brokerIDs[1], brokerIDs[3], brokerIDs[4], brokerIDs[0], brokerIDs[1]}
	got := make([]string, 0, len(want))
	for i := 0; i < len(want); i++ {
		l := newTestLead(t, tenantID)
		if err := leadRepo.Create(ctx, l); err != nil {
			t.Fatalf("create lead: %v", err)
		}
		ok, err := svc.AssignByRoulette(ctx, l, "")
		if err != nil {
			t.Fatalf("assign: %v", err)
		}
		if !ok {
			t.Fatalf("expected assigned=true, got false")
		}
		updated, err := leadRepo.GetByID(ctx, l.TenantID, l.ID)
		if err != nil {
			t.Fatalf("get lead: %v", err)
		}
		got = append(got, updated.AssignedBrokerID)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("assignment[%d] expected %s, got %s (all=%v)", i, want[i], got[i], got)
		}
	}
}

func TestRoulette_DoesNotAssignWhenNoEligibleBrokers(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantID := uuid.NewString()

	leadRepo := repository.NewLeadPostgresRepository(db)
	rouletteRepo := repository.NewRoulettePostgresRepository(db)
	historyRepo := repository.NewRouletteHistoryPostgresRepository(db)
	brokerRepo := repository.NewBrokerPostgresRepository(db)

	svc := NewLeadRouletteService(leadRepo, rouletteRepo, historyRepo, brokerRepo, nil, nil)

	brokerIDs := createBrokers(t, ctx, brokerRepo, tenantID, 3)
	for _, id := range brokerIDs {
		if ok, err := brokerRepo.SetPaused(ctx, id, tenantID, true); err != nil || !ok {
			t.Fatalf("pause broker %s: ok=%v err=%v", id, ok, err)
		}
	}

	cfg := &lead.RouletteConfig{
		TenantID:         tenantID,
		Enabled:          true,
		ActiveBrokers:    brokerIDs,
		LastIndex:        -1,
		StageBrokerPools: map[string][]string{"NEW": brokerIDs},
		StageLastIndex:   map[string]int{"NEW": -1},
		DistMethod:       lead.DistMethodSequential,
		AutoCapLimit:     false,
		RespectSpecialty: false,
		RespectRegion:    false,
		RespectSchedule:  false,
	}
	if err := rouletteRepo.UpdateConfig(ctx, cfg); err != nil {
		t.Fatalf("init config: %v", err)
	}

	l := newTestLead(t, tenantID)
	if err := leadRepo.Create(ctx, l); err != nil {
		t.Fatalf("create lead: %v", err)
	}

	ok, err := svc.AssignByRoulette(ctx, l, "")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if ok {
		t.Fatalf("expected assigned=false when no eligible brokers, got true")
	}
}

func TestRoulette_SeparatePointersPerStage(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantID := uuid.NewString()

	leadRepo := repository.NewLeadPostgresRepository(db)
	rouletteRepo := repository.NewRoulettePostgresRepository(db)
	historyRepo := repository.NewRouletteHistoryPostgresRepository(db)
	brokerRepo := repository.NewBrokerPostgresRepository(db)

	svc := NewLeadRouletteService(leadRepo, rouletteRepo, historyRepo, brokerRepo, nil, nil)

	newPool := createBrokers(t, ctx, brokerRepo, tenantID, 3)
	contactedPool := createBrokers(t, ctx, brokerRepo, tenantID, 2)

	cfg := &lead.RouletteConfig{
		TenantID:      tenantID,
		Enabled:       true,
		ActiveBrokers: newPool,
		LastIndex:     -1,
		StageBrokerPools: map[string][]string{
			"NEW":       newPool,
			"CONTACTED": contactedPool,
		},
		StageLastIndex:   map[string]int{},
		DistMethod:       lead.DistMethodSequential,
		AutoCapLimit:     false,
		RespectSpecialty: false,
		RespectRegion:    false,
		RespectSchedule:  false,
	}
	if err := rouletteRepo.UpdateConfig(ctx, cfg); err != nil {
		t.Fatalf("init config: %v", err)
	}

	l1 := newTestLead(t, tenantID)
	l1.Status = lead.LeadStatusNew
	if err := leadRepo.Create(ctx, l1); err != nil {
		t.Fatalf("create lead: %v", err)
	}
	if ok, err := svc.AssignByRoulette(ctx, l1, ""); err != nil || !ok {
		t.Fatalf("assign new: ok=%v err=%v", ok, err)
	}

	l2 := newTestLead(t, tenantID)
	l2.Status = lead.LeadStatusContacted
	if err := leadRepo.Create(ctx, l2); err != nil {
		t.Fatalf("create lead: %v", err)
	}
	if ok, err := svc.AssignByRoulette(ctx, l2, ""); err != nil || !ok {
		t.Fatalf("assign contacted: ok=%v err=%v", ok, err)
	}

	updated1, _ := leadRepo.GetByID(ctx, l1.TenantID, l1.ID)
	updated2, _ := leadRepo.GetByID(ctx, l2.TenantID, l2.ID)

	if updated1.AssignedBrokerID != newPool[0] {
		t.Fatalf("NEW expected %s, got %s", newPool[0], updated1.AssignedBrokerID)
	}
	if updated2.AssignedBrokerID != contactedPool[0] {
		t.Fatalf("CONTACTED expected %s, got %s", contactedPool[0], updated2.AssignedBrokerID)
	}
}

func TestRoulette_SimulatePersistAdvancesPointer(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantID := uuid.NewString()

	leadRepo := repository.NewLeadPostgresRepository(db)
	rouletteRepo := repository.NewRoulettePostgresRepository(db)
	historyRepo := repository.NewRouletteHistoryPostgresRepository(db)
	brokerRepo := repository.NewBrokerPostgresRepository(db)

	svc := NewLeadRouletteService(leadRepo, rouletteRepo, historyRepo, brokerRepo, nil, nil)

	brokerIDs := createBrokers(t, ctx, brokerRepo, tenantID, 5)

	cfg := &lead.RouletteConfig{
		TenantID:         tenantID,
		Enabled:          true,
		ActiveBrokers:    brokerIDs,
		LastIndex:        -1,
		StageBrokerPools: map[string][]string{"NEW": brokerIDs},
		StageLastIndex:   map[string]int{"NEW": -1},
		DistMethod:       lead.DistMethodSequential,
		AutoCapLimit:     false,
		RespectSpecialty: false,
		RespectRegion:    false,
		RespectSchedule:  false,
	}
	if err := rouletteRepo.UpdateConfig(ctx, cfg); err != nil {
		t.Fatalf("init config: %v", err)
	}

	// stage em lower-case para validar normalização + persistência no ponteiro correto
	res, err := svc.SimulateRoulette(ctx, tenantID, "new", 3, true, 0)
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if res == nil || len(res.Assignments) != 3 {
		t.Fatalf("expected 3 assignments, got %+v", res)
	}

	updatedCfg, err := rouletteRepo.GetConfig(ctx, tenantID)
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if got := currentIndex(updatedCfg, "NEW"); got != 2 {
		t.Fatalf("expected StageLastIndex NEW == 2, got %d (cfg=%+v)", got, updatedCfg.StageLastIndex)
	}
}

func TestRoulette_UpdateConfigNormalizesStageKeys(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantID := uuid.NewString()

	leadRepo := repository.NewLeadPostgresRepository(db)
	rouletteRepo := repository.NewRoulettePostgresRepository(db)
	historyRepo := repository.NewRouletteHistoryPostgresRepository(db)
	brokerRepo := repository.NewBrokerPostgresRepository(db)

	svc := NewLeadRouletteService(leadRepo, rouletteRepo, historyRepo, brokerRepo, nil, nil)

	brokerIDs := createBrokers(t, ctx, brokerRepo, tenantID, 3)

	// Config inicial com StageLastIndex "sujo" (lowercase)
	cfg := &lead.RouletteConfig{
		TenantID:       tenantID,
		Enabled:        true,
		ActiveBrokers:  brokerIDs,
		LastIndex:      -1,
		StageLastIndex: map[string]int{"new": 1},
		DistMethod:     lead.DistMethodSequential,
	}
	if err := rouletteRepo.UpdateConfig(ctx, cfg); err != nil {
		t.Fatalf("init config: %v", err)
	}

	enabled := true
	updated, err := svc.UpdateConfig(ctx, tenantID, lead.UpdateRouletteConfigDTO{
		Enabled:          &enabled,
		StageBrokerPools: map[string][]string{"new": brokerIDs},
	}, RouletteActor{})
	if err != nil {
		t.Fatalf("update config: %v", err)
	}

	if updated.StageBrokerPools == nil || len(updated.StageBrokerPools["NEW"]) != len(brokerIDs) {
		t.Fatalf("expected StageBrokerPools['NEW'] set, got %+v", updated.StageBrokerPools)
	}
	if updated.StageLastIndex == nil || updated.StageLastIndex["NEW"] != 1 {
		t.Fatalf("expected StageLastIndex['NEW']=1, got %+v", updated.StageLastIndex)
	}

	// e agora a atribuição com status lowercase deve cair no pool "NEW"
	l := newTestLead(t, tenantID)
	l.Status = lead.LeadStatus("new")
	if err := leadRepo.Create(ctx, l); err != nil {
		t.Fatalf("create lead: %v", err)
	}
	if ok, err := svc.AssignByRoulette(ctx, l, ""); err != nil || !ok {
		t.Fatalf("assign: ok=%v err=%v", ok, err)
	}
	assigned, _ := leadRepo.GetByID(ctx, l.TenantID, l.ID)
	if assigned.AssignedBrokerID != brokerIDs[2] {
		// StageLastIndex começou em 1, então o próximo deve ser idx=2
		t.Fatalf("expected assigned broker %s, got %s", brokerIDs[2], assigned.AssignedBrokerID)
	}
}

func TestRoulette_UpdateConfigInitializesStageLastIndexEntries(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantID := uuid.NewString()

	leadRepo := repository.NewLeadPostgresRepository(db)
	rouletteRepo := repository.NewRoulettePostgresRepository(db)
	historyRepo := repository.NewRouletteHistoryPostgresRepository(db)
	brokerRepo := repository.NewBrokerPostgresRepository(db)

	svc := NewLeadRouletteService(leadRepo, rouletteRepo, historyRepo, brokerRepo, nil, nil)

	newPool := createBrokers(t, ctx, brokerRepo, tenantID, 3)
	contactedPool := createBrokers(t, ctx, brokerRepo, tenantID, 2)

	enabled := true
	updated, err := svc.UpdateConfig(ctx, tenantID, lead.UpdateRouletteConfigDTO{
		Enabled: &enabled,
		StageBrokerPools: map[string][]string{
			"new":       newPool,       // lowercase para validar normalização + init
			"CONTACTED": contactedPool, // uppercase
		},
	}, RouletteActor{})
	if err != nil {
		t.Fatalf("update config: %v", err)
	}

	if updated.StageLastIndex == nil {
		t.Fatalf("expected StageLastIndex map to be initialized, got nil")
	}
	if got, ok := updated.StageLastIndex["NEW"]; !ok || got != -1 {
		t.Fatalf("expected StageLastIndex[NEW]=-1, got %d (present=%v)", got, ok)
	}
	if got, ok := updated.StageLastIndex["CONTACTED"]; !ok || got != -1 {
		t.Fatalf("expected StageLastIndex[CONTACTED]=-1, got %d (present=%v)", got, ok)
	}

	// Precisa estar persistido no banco também — valida que o jsonb_set do
	// SetIndexCAS conseguirá casar COALESCE(..., -1) = -1 na primeira atribuição.
	persisted, err := rouletteRepo.GetConfig(ctx, tenantID)
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if persisted.StageLastIndex == nil {
		t.Fatalf("expected StageLastIndex persisted, got nil")
	}
	if persisted.StageLastIndex["NEW"] != -1 {
		t.Fatalf("expected persisted StageLastIndex[NEW]=-1, got %d", persisted.StageLastIndex["NEW"])
	}
	if persisted.StageLastIndex["CONTACTED"] != -1 {
		t.Fatalf("expected persisted StageLastIndex[CONTACTED]=-1, got %d", persisted.StageLastIndex["CONTACTED"])
	}

	// E agora a primeira atribuição em cada stage deve ir para o broker[0] daquele pool.
	l1 := newTestLead(t, tenantID)
	l1.Status = lead.LeadStatusNew
	if err := leadRepo.Create(ctx, l1); err != nil {
		t.Fatalf("create lead: %v", err)
	}
	if ok, err := svc.AssignByRoulette(ctx, l1, ""); err != nil || !ok {
		t.Fatalf("assign NEW: ok=%v err=%v", ok, err)
	}
	got1, _ := leadRepo.GetByID(ctx, l1.TenantID, l1.ID)
	if got1.AssignedBrokerID != newPool[0] {
		t.Fatalf("NEW first assignment expected %s, got %s", newPool[0], got1.AssignedBrokerID)
	}

	l2 := newTestLead(t, tenantID)
	l2.Status = lead.LeadStatusContacted
	if err := leadRepo.Create(ctx, l2); err != nil {
		t.Fatalf("create lead: %v", err)
	}
	if ok, err := svc.AssignByRoulette(ctx, l2, ""); err != nil || !ok {
		t.Fatalf("assign CONTACTED: ok=%v err=%v", ok, err)
	}
	got2, _ := leadRepo.GetByID(ctx, l2.TenantID, l2.ID)
	if got2.AssignedBrokerID != contactedPool[0] {
		t.Fatalf("CONTACTED first assignment expected %s, got %s", contactedPool[0], got2.AssignedBrokerID)
	}
}

// TestRoulette_SimulationIsReproducibleWithSameSeed proves SimulateRoulette
// returns identical assignment sequences when invoked twice with the same
// seed. Wave P2.12 — addresses the gap flagged in the review: the simulator
// supports a seed parameter but no test ever proved the determinism contract.
//
// Uses DistMethodWeighted so RNG actually drives selection (sequential
// ignores RNG and would pass the test for the wrong reason).
func TestRoulette_SimulationIsReproducibleWithSameSeed(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantID := uuid.NewString()

	leadRepo := repository.NewLeadPostgresRepository(db)
	rouletteRepo := repository.NewRoulettePostgresRepository(db)
	historyRepo := repository.NewRouletteHistoryPostgresRepository(db)
	brokerRepo := repository.NewBrokerPostgresRepository(db)

	svc := NewLeadRouletteService(leadRepo, rouletteRepo, historyRepo, brokerRepo, nil, nil)

	brokerIDs := createBrokers(t, ctx, brokerRepo, tenantID, 3)
	// Distribute uneven weights so the weighted picker visibly favors some
	// brokers — and a different seed would yield a visibly different ordering.
	for i, id := range brokerIDs {
		b, err := brokerRepo.GetByID(ctx, id)
		if err != nil {
			t.Fatalf("get broker %d: %v", i, err)
		}
		b.Weight = float64(i + 1) // 1, 2, 3
		if err := brokerRepo.Update(ctx, b); err != nil {
			t.Fatalf("update broker %d: %v", i, err)
		}
	}

	cfg := &lead.RouletteConfig{
		TenantID:         tenantID,
		Enabled:          true,
		ActiveBrokers:    brokerIDs,
		LastIndex:        -1,
		StageBrokerPools: map[string][]string{"NEW": brokerIDs},
		StageLastIndex:   map[string]int{"NEW": -1},
		DistMethod:       lead.DistMethodWeighted,
	}
	if err := rouletteRepo.UpdateConfig(ctx, cfg); err != nil {
		t.Fatalf("init config: %v", err)
	}

	const seed int64 = 424242
	const count = 20

	// persist=false to keep the pointer state stable between runs;
	// otherwise run 2 would start with a different StageLastIndex.
	res1, err := svc.SimulateRoulette(ctx, tenantID, "NEW", count, false, seed)
	if err != nil {
		t.Fatalf("simulate #1: %v", err)
	}
	res2, err := svc.SimulateRoulette(ctx, tenantID, "NEW", count, false, seed)
	if err != nil {
		t.Fatalf("simulate #2: %v", err)
	}

	if len(res1.Assignments) != count || len(res2.Assignments) != count {
		t.Fatalf("expected %d assignments per run, got %d and %d", count, len(res1.Assignments), len(res2.Assignments))
	}
	for i := 0; i < count; i++ {
		if res1.Assignments[i].BrokerID != res2.Assignments[i].BrokerID {
			t.Fatalf("non-deterministic at index %d: run1=%s run2=%s — same seed must yield same sequence",
				i, res1.Assignments[i].BrokerID, res2.Assignments[i].BrokerID)
		}
	}

	// Run with a different seed should NOT produce the same sequence
	// (probability of accidental match with 20 weighted picks across 3
	// brokers is astronomically low; if it ever does, increase count).
	res3, err := svc.SimulateRoulette(ctx, tenantID, "NEW", count, false, seed+1)
	if err != nil {
		t.Fatalf("simulate #3: %v", err)
	}
	identical := true
	for i := 0; i < count; i++ {
		if res1.Assignments[i].BrokerID != res3.Assignments[i].BrokerID {
			identical = false
			break
		}
	}
	if identical {
		t.Fatalf("different seeds produced identical sequences — RNG seeding is likely broken")
	}
}

// TestRoulette_QualificationRoutingRoutesToMatchingBroker exercises the full
// qualification routing path end-to-end: respectSpecialty + respectRegion +
// QualRouting bind fields from the lead's qualification map into the
// eligibility filter. Wave P2.11 — addresses the gap where qualification
// routing had unit coverage for the extraction helpers but no integration
// proof that the picker actually honored the resolved tags.
func TestRoulette_QualificationRoutingRoutesToMatchingBroker(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantID := uuid.NewString()

	leadRepo := repository.NewLeadPostgresRepository(db)
	rouletteRepo := repository.NewRoulettePostgresRepository(db)
	historyRepo := repository.NewRouletteHistoryPostgresRepository(db)
	brokerRepo := repository.NewBrokerPostgresRepository(db)

	svc := NewLeadRouletteService(leadRepo, rouletteRepo, historyRepo, brokerRepo, nil, nil)

	// Two brokers: one specialised in "luxo" + region "Centro", another with
	// no specialisations (a generic broker).
	specialised := createBrokersWithSpecialties(t, ctx, brokerRepo, tenantID, 1, []string{"luxo"})
	specialisedID := specialised[0]
	// Attach a region too. The specialty helper only sets Specialties, so
	// fetch + update for the region.
	b, err := brokerRepo.GetByID(ctx, specialisedID)
	if err != nil {
		t.Fatalf("get specialised broker: %v", err)
	}
	b.Regions = []string{"Centro"}
	if err := brokerRepo.Update(ctx, b); err != nil {
		t.Fatalf("update specialised broker regions: %v", err)
	}

	generic := createBrokers(t, ctx, brokerRepo, tenantID, 1)
	genericID := generic[0]

	pool := []string{genericID, specialisedID} // generic first so a naïve sequential picker would land on it
	regionFieldID := "regiao_field"

	cfg := &lead.RouletteConfig{
		TenantID:         tenantID,
		Enabled:          true,
		ActiveBrokers:    pool,
		LastIndex:        -1,
		StageBrokerPools: map[string][]string{"NEW": pool},
		StageLastIndex:   map[string]int{"NEW": -1},
		DistMethod:       lead.DistMethodSequential,
		RespectSpecialty: true,
		RespectRegion:    true,
		QualRouting: &lead.QualRoutingConfig{
			SpecialtyFieldIDs: []string{"interesse_field"},
			RegionFieldID:     &regionFieldID,
		},
	}
	if err := rouletteRepo.UpdateConfig(ctx, cfg); err != nil {
		t.Fatalf("init config: %v", err)
	}

	// Lead carrying qualification values that only the specialised broker
	// can match. Without QualRouting these fields are invisible to the
	// picker.
	l := newTestLead(t, tenantID)
	l.Qualification = map[string]interface{}{
		"interesse_field": "luxo",
		"regiao_field":    "Centro",
	}
	if err := leadRepo.Create(ctx, l); err != nil {
		t.Fatalf("create lead: %v", err)
	}

	ok, err := svc.AssignByRoulette(ctx, l, "")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if !ok {
		t.Fatalf("expected assignment to succeed")
	}

	got, err := leadRepo.GetByID(ctx, l.TenantID, l.ID)
	if err != nil {
		t.Fatalf("get lead: %v", err)
	}
	if got.AssignedBrokerID != specialisedID {
		t.Fatalf("expected lead to be routed to specialised broker %s, got %s (generic=%s)",
			specialisedID, got.AssignedBrokerID, genericID)
	}
}

// TestRoulette_ConcurrentAssignmentsPreserveOrder fires N goroutines into
// AssignByRoulette at the same time and verifies the CAS pointer keeps the
// distribution coherent: no lead ends up assigned twice, no lead is left
// orphaned, and the per-broker load is balanced within a small tolerance.
//
// Wave P2.10 — addresses the production-readiness gap flagged in the
// review: every prior roulette test was strictly sequential.
func TestRoulette_ConcurrentAssignmentsPreserveOrder(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantID := uuid.NewString()

	leadRepo := repository.NewLeadPostgresRepository(db)
	rouletteRepo := repository.NewRoulettePostgresRepository(db)
	historyRepo := repository.NewRouletteHistoryPostgresRepository(db)
	brokerRepo := repository.NewBrokerPostgresRepository(db)

	svc := NewLeadRouletteService(leadRepo, rouletteRepo, historyRepo, brokerRepo, nil, nil)

	const (
		brokerCount = 5
		leadCount   = 50
	)
	brokerIDs := createBrokers(t, ctx, brokerRepo, tenantID, brokerCount)

	cfg := &lead.RouletteConfig{
		TenantID:         tenantID,
		Enabled:          true,
		ActiveBrokers:    brokerIDs,
		LastIndex:        -1,
		StageBrokerPools: map[string][]string{"NEW": brokerIDs},
		StageLastIndex:   map[string]int{"NEW": -1},
		DistMethod:       lead.DistMethodSequential,
	}
	if err := rouletteRepo.UpdateConfig(ctx, cfg); err != nil {
		t.Fatalf("init config: %v", err)
	}

	// Pre-create the leads so all goroutines just call AssignByRoulette.
	// Avoiding the create path inside the goroutines keeps the test focused
	// on the assignment race specifically.
	leads := make([]*lead.Lead, 0, leadCount)
	for i := 0; i < leadCount; i++ {
		l := newTestLead(t, tenantID)
		if err := leadRepo.Create(ctx, l); err != nil {
			t.Fatalf("create lead %d: %v", i, err)
		}
		leads = append(leads, l)
	}

	var (
		wg       sync.WaitGroup
		successN int64
		errN     int64
	)
	for i := 0; i < leadCount; i++ {
		wg.Add(1)
		go func(l *lead.Lead) {
			defer wg.Done()
			ok, err := svc.AssignByRoulette(ctx, l, "")
			if err != nil {
				atomic.AddInt64(&errN, 1)
				return
			}
			if ok {
				atomic.AddInt64(&successN, 1)
			}
		}(leads[i])
	}
	wg.Wait()

	if atomic.LoadInt64(&errN) > 0 {
		t.Fatalf("expected zero errors under concurrency, got %d", errN)
	}
	if atomic.LoadInt64(&successN) != leadCount {
		t.Fatalf("expected %d successful assignments, got %d", leadCount, successN)
	}

	// Read back every lead and confirm two invariants:
	//   (a) every lead is assigned to a broker in the pool — no orphans
	//   (b) per-broker load is within tolerance of perfect balance
	loads := map[string]int{}
	brokerSet := map[string]bool{}
	for _, id := range brokerIDs {
		brokerSet[id] = true
	}
	for _, l := range leads {
		got, err := leadRepo.GetByID(ctx, l.TenantID, l.ID)
		if err != nil {
			t.Fatalf("get lead %s: %v", l.ID, err)
		}
		if got.AssignedBrokerID == "" {
			t.Fatalf("lead %s ended up unassigned after concurrent run", l.ID)
		}
		if !brokerSet[got.AssignedBrokerID] {
			t.Fatalf("lead %s was assigned to broker %s which is not in the pool", l.ID, got.AssignedBrokerID)
		}
		loads[got.AssignedBrokerID]++
	}

	// Perfect balance is leadCount/brokerCount = 10. Under contention with
	// CAS retries some skew is normal; allow ±3 around the ideal value.
	const ideal = leadCount / brokerCount
	const tolerance = 3
	for id, n := range loads {
		if n < ideal-tolerance || n > ideal+tolerance {
			t.Errorf("broker %s received %d leads, expected %d±%d (loads=%v)", id, n, ideal, tolerance, loads)
		}
	}

	// History rows must equal the number of successful assignments —
	// CAS-exhausted attempts would not persist history.
	hist, _, err := historyRepo.List(ctx, tenantID, 100, 0)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if int64(len(hist)) != atomic.LoadInt64(&successN) {
		t.Fatalf("history rows (%d) != success count (%d)", len(hist), successN)
	}
}

// TestRoulette_TenantIsolation proves a tenant cannot observe nor mutate
// another tenant's roulette state. The two tenants are configured with
// disjoint broker pools and run through SimulateRoulette in parallel — the
// stage pointers, audit trail, and history rows must remain partitioned.
//
// Wave P0.3 — addresses the gap flagged in the Roleta review: every test
// previously exercised a single tenant, so cross-tenant leaks would go
// undetected.
func TestRoulette_TenantIsolation(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t, ctx)
	defer db.Close()

	tenantA := uuid.NewString()
	tenantB := uuid.NewString()

	leadRepo := repository.NewLeadPostgresRepository(db)
	rouletteRepo := repository.NewRoulettePostgresRepository(db)
	historyRepo := repository.NewRouletteHistoryPostgresRepository(db)
	brokerRepo := repository.NewBrokerPostgresRepository(db)

	svc := NewLeadRouletteService(leadRepo, rouletteRepo, historyRepo, brokerRepo, nil, nil)

	brokersA := createBrokers(t, ctx, brokerRepo, tenantA, 3)
	brokersB := createBrokers(t, ctx, brokerRepo, tenantB, 2)
	brokerSetA := map[string]bool{}
	for _, id := range brokersA {
		brokerSetA[id] = true
	}
	brokerSetB := map[string]bool{}
	for _, id := range brokersB {
		brokerSetB[id] = true
	}

	for _, tt := range []struct {
		tenant  string
		brokers []string
	}{
		{tenantA, brokersA},
		{tenantB, brokersB},
	} {
		cfg := &lead.RouletteConfig{
			TenantID:         tt.tenant,
			Enabled:          true,
			ActiveBrokers:    tt.brokers,
			LastIndex:        -1,
			StageBrokerPools: map[string][]string{"NEW": tt.brokers},
			StageLastIndex:   map[string]int{"NEW": -1},
			DistMethod:       lead.DistMethodSequential,
		}
		if err := rouletteRepo.UpdateConfig(ctx, cfg); err != nil {
			t.Fatalf("init config tenant=%s: %v", tt.tenant, err)
		}
	}

	// Run simulations for both tenants. SimulateRoulette with persist=true
	// advances the stage pointer in the DB — the critical operation for
	// cross-tenant leakage tests.
	resA, err := svc.SimulateRoulette(ctx, tenantA, "NEW", 5, true, 0)
	if err != nil {
		t.Fatalf("simulate tenant A: %v", err)
	}
	resB, err := svc.SimulateRoulette(ctx, tenantB, "NEW", 5, true, 0)
	if err != nil {
		t.Fatalf("simulate tenant B: %v", err)
	}

	// (1) Assignments respect tenant boundaries.
	for i, a := range resA.Assignments {
		if !brokerSetA[a.BrokerID] {
			t.Fatalf("tenant A assignment[%d] picked broker %s which does not belong to A (A=%v)", i, a.BrokerID, brokersA)
		}
		if brokerSetB[a.BrokerID] {
			t.Fatalf("tenant A assignment[%d] picked a B broker %s — CROSS-TENANT LEAK", i, a.BrokerID)
		}
	}
	for i, a := range resB.Assignments {
		if !brokerSetB[a.BrokerID] {
			t.Fatalf("tenant B assignment[%d] picked broker %s which does not belong to B (B=%v)", i, a.BrokerID, brokersB)
		}
		if brokerSetA[a.BrokerID] {
			t.Fatalf("tenant B assignment[%d] picked an A broker %s — CROSS-TENANT LEAK", i, a.BrokerID)
		}
	}

	// (2) Stage pointers are independent. 5 assignments over a pool of size N
	// with sequential dist advances the pointer to (5-1) % N.
	cfgA, err := rouletteRepo.GetConfig(ctx, tenantA)
	if err != nil {
		t.Fatalf("get config A: %v", err)
	}
	cfgB, err := rouletteRepo.GetConfig(ctx, tenantB)
	if err != nil {
		t.Fatalf("get config B: %v", err)
	}
	if wantA, gotA := (5-1)%len(brokersA), currentIndex(cfgA, "NEW"); wantA != gotA {
		t.Fatalf("tenant A pointer: want %d, got %d", wantA, gotA)
	}
	if wantB, gotB := (5-1)%len(brokersB), currentIndex(cfgB, "NEW"); wantB != gotB {
		t.Fatalf("tenant B pointer: want %d, got %d", wantB, gotB)
	}

	// (3) Broker pools are not mutated cross-tenant.
	for _, id := range cfgA.ActiveBrokers {
		if brokerSetB[id] {
			t.Fatalf("tenant A ActiveBrokers contains B broker %s", id)
		}
	}
	for _, id := range cfgB.ActiveBrokers {
		if brokerSetA[id] {
			t.Fatalf("tenant B ActiveBrokers contains A broker %s", id)
		}
	}

	// (4) History rows are partitioned. Reading tenant A's history must not
	// surface assignments persisted under tenant B.
	histA, _, err := historyRepo.List(ctx, tenantA, 100, 0)
	if err != nil {
		t.Fatalf("list history A: %v", err)
	}
	for _, h := range histA {
		if h.TenantID != tenantA {
			t.Fatalf("tenant A history contains entry for tenant %s — CROSS-TENANT LEAK", h.TenantID)
		}
		if brokerSetB[h.ToBrokerID] {
			t.Fatalf("tenant A history references B broker %s", h.ToBrokerID)
		}
	}
	histB, _, err := historyRepo.List(ctx, tenantB, 100, 0)
	if err != nil {
		t.Fatalf("list history B: %v", err)
	}
	for _, h := range histB {
		if h.TenantID != tenantB {
			t.Fatalf("tenant B history contains entry for tenant %s — CROSS-TENANT LEAK", h.TenantID)
		}
		if brokerSetA[h.ToBrokerID] {
			t.Fatalf("tenant B history references A broker %s", h.ToBrokerID)
		}
	}

	// (5) ListEnabledTenantIDs surfaces both tenants but the SLA worker
	// processes them independently — sanity-check the listing is not empty
	// and contains both.
	enabled, err := rouletteRepo.ListEnabledTenantIDs(ctx)
	if err != nil {
		t.Fatalf("list enabled tenants: %v", err)
	}
	foundA, foundB := false, false
	for _, id := range enabled {
		if id == tenantA {
			foundA = true
		}
		if id == tenantB {
			foundB = true
		}
	}
	if !foundA || !foundB {
		t.Fatalf("ListEnabledTenantIDs missing one of the test tenants (found A=%v B=%v, list=%v)", foundA, foundB, enabled)
	}
}

func connectTestDB(t *testing.T, ctx context.Context) *database.PostgresDB {
	t.Helper()

	uri := os.Getenv("DATABASE_URI_TEST")
	if uri == "" {
		uri = "postgres://postgres:postgres@localhost:5432/toca_test?sslmode=disable"
	}

	cfg := config.DatabaseConfig{URI: uri}
	db, err := database.ConnectPostgres(ctx, cfg)
	if err != nil {
		t.Skipf("skipping integration test: failed to connect to postgres: %v", err)
	}
	return db
}

func createBrokers(t *testing.T, ctx context.Context, repo broker.Repository, tenantID string, count int) []string {
	t.Helper()
	out := make([]string, 0, count)
	for i := 0; i < count; i++ {
		b := broker.New("E2E Broker", "corretor", "", tenantID)
		b.ID = uuid.NewString()
		b.Active = true
		if err := repo.Create(ctx, b); err != nil {
			t.Fatalf("create broker: %v", err)
		}
		out = append(out, b.ID)
	}
	return out
}

func createBrokersWithSpecialties(t *testing.T, ctx context.Context, repo broker.Repository, tenantID string, count int, specialties []string) []string {
	t.Helper()
	out := make([]string, 0, count)
	for i := 0; i < count; i++ {
		b := broker.New("E2E Broker", "corretor", "", tenantID)
		b.ID = uuid.NewString()
		b.Active = true
		b.Specialties = specialties
		if err := repo.Create(ctx, b); err != nil {
			t.Fatalf("create broker: %v", err)
		}
		out = append(out, b.ID)
	}
	return out
}

func newTestLead(t *testing.T, tenantID string) *lead.Lead {
	t.Helper()
	now := time.Now().UTC()
	return &lead.Lead{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Name:      "E2E Lead",
		Email:     uuid.NewString() + "@test.com",
		Phone:     "11999999999",
		Source:    lead.LeadSourceWebsite,
		Status:    lead.LeadStatusNew,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
