package postgres

import (
	"context"
	"testing"

	appCtx "github.com/oginoino/toca/pkg/context"
)

func TestWithTenantSeedsClaimForRLS(t *testing.T) {
	ctx := withTenant(context.Background(), "tenant-1")
	got, err := appCtx.GetTenantID(ctx)
	if err != nil || got != "tenant-1" {
		t.Fatalf("claim=%q err=%v", got, err)
	}
}

func TestWithTenantDoesNotOverrideCallerClaim(t *testing.T) {
	ctx := appCtx.WithTenantID(context.Background(), "caller")
	got, _ := appCtx.GetTenantID(withTenant(ctx, "other"))
	if got != "caller" {
		t.Fatalf("claim sobrescrito: %q", got)
	}
}

func TestWithTenantNoopForEmptyTenant(t *testing.T) {
	if _, err := appCtx.GetTenantID(withTenant(context.Background(), "  ")); err == nil {
		t.Fatal("tenant vazio não pode semear claim")
	}
}
