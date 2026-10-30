package database

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestServiceTimeoutHookNeverSeedsJWTClaims(t *testing.T) {
	if strings.Contains(serviceTimeoutConfigSQL, "request.jwt.claims") {
		t.Fatal("service-role timeout hook must not seed tenant JWT claims")
	}
	for _, setting := range []string{"statement_timeout", "lock_timeout"} {
		if !strings.Contains(serviceTimeoutConfigSQL, setting) {
			t.Fatalf("service-role timeout hook missing %s", setting)
		}
	}
}

func TestServiceTimeoutValuesUseBackstopsWithoutDeadline(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	statement, lock := serviceTimeoutValues(context.Background(), now)
	if statement != "60000" || lock != "10000" {
		t.Fatalf("serviceTimeoutValues() = (%s, %s), want (60000, 10000)", statement, lock)
	}
}

func TestServiceTimeoutValuesHonorDeliveryDeadline(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithDeadline(context.Background(), now.Add(7*time.Second))
	defer cancel()

	statement, lock := serviceTimeoutValues(ctx, now)
	if statement != "5500" || lock != "5500" {
		t.Fatalf("serviceTimeoutValues() = (%s, %s), want deadline-bounded (5500, 5500)", statement, lock)
	}
}

func TestServiceTimeoutValuesKeepMinimumForNearDeadline(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithDeadline(context.Background(), now.Add(500*time.Millisecond))
	defer cancel()

	statement, lock := serviceTimeoutValues(ctx, now)
	if statement != "1000" || lock != "1000" {
		t.Fatalf("serviceTimeoutValues() = (%s, %s), want minimum (1000, 1000)", statement, lock)
	}
}
