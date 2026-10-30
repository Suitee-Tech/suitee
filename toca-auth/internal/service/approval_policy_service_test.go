package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca-auth/internal/domain/tenantsettings"
)

type fakeApprovalPolicyRepository struct {
	policy       tenantsettings.ApprovalPolicy
	found        bool
	getErr       error
	upsertErr    error
	upsertCalls  int
	gotEnabled   bool
	gotThreshold float64
	gotUpdatedBy string
}

func (f *fakeApprovalPolicyRepository) GetApprovalPolicy(context.Context, string) (tenantsettings.ApprovalPolicy, bool, error) {
	return f.policy, f.found, f.getErr
}

func (f *fakeApprovalPolicyRepository) UpsertApprovalPolicy(_ context.Context, _ string, enabled bool, threshold float64, updatedBy string) (tenantsettings.ApprovalPolicy, error) {
	f.upsertCalls++
	f.gotEnabled = enabled
	f.gotThreshold = threshold
	f.gotUpdatedBy = updatedBy
	if f.upsertErr != nil {
		return tenantsettings.ApprovalPolicy{}, f.upsertErr
	}
	updated := tenantsettings.DefaultApprovalPolicy()
	updated.Revision = 2
	updated.Enabled = enabled
	updated.ThresholdBRL = threshold
	updated.Source = "tenant_settings"
	updated.UpdatedAt = time.Now().UTC()
	updated.UpdatedBy = updatedBy
	return updated, nil
}

func TestApprovalPolicyServiceGetUsesPlatformDefaultWhenTenantHasNoPolicy(t *testing.T) {
	svc := NewApprovalPolicyService(&fakeApprovalPolicyRepository{})
	policy, err := svc.Get(context.Background(), "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if policy.Source != "platform_default" || policy.ThresholdBRL != 500_000 || policy.Revision != 0 {
		t.Fatalf("unexpected default: %+v", policy)
	}
}

func TestApprovalPolicyServiceGetPropagatesRepositoryFailure(t *testing.T) {
	wantErr := errors.New("database unavailable")
	svc := NewApprovalPolicyService(&fakeApprovalPolicyRepository{getErr: wantErr})
	_, err := svc.Get(context.Background(), "11111111-1111-1111-1111-111111111111")
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestApprovalPolicyServiceUpdateValidatesAndPersistsVersionedPolicy(t *testing.T) {
	repo := &fakeApprovalPolicyRepository{}
	svc := NewApprovalPolicyService(repo)

	updated, err := svc.Update(context.Background(), "22222222-2222-2222-2222-222222222222", UpdateApprovalPolicyInput{
		Enabled:      true,
		ThresholdBRL: 900_000,
		UpdatedBy:    "  user-42  ",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if repo.upsertCalls != 1 || !repo.gotEnabled || repo.gotThreshold != 900_000 || repo.gotUpdatedBy != "user-42" {
		t.Fatalf("unexpected upsert: %+v", repo)
	}
	if updated.SchemaVersion != 1 || updated.Revision != 2 || updated.RequiredPermission != "deals:approve" {
		t.Fatalf("unexpected updated policy: %+v", updated)
	}
}

func TestApprovalPolicyServiceUpdateRejectsUnsafePolicy(t *testing.T) {
	repo := &fakeApprovalPolicyRepository{}
	svc := NewApprovalPolicyService(repo)
	_, err := svc.Update(context.Background(), "33333333-3333-3333-3333-333333333333", UpdateApprovalPolicyInput{
		Enabled:      true,
		ThresholdBRL: 0,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if repo.upsertCalls != 0 {
		t.Fatalf("upsert calls = %d, want 0", repo.upsertCalls)
	}
}
