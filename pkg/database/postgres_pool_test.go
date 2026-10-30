package database

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestNormalizePoolLimits(t *testing.T) {
	tests := []struct {
		name             string
		max, min         int32
		wantMax, wantMin int32
	}{
		{
			name:    "single connection clamps legacy minimum",
			max:     1,
			min:     2,
			wantMax: 1,
			wantMin: 1,
		},
		{
			name:    "valid range remains unchanged",
			max:     5,
			min:     2,
			wantMax: 5,
			wantMin: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMax, gotMin := normalizePoolLimits(tt.max, tt.min)
			if gotMax != tt.wantMax || gotMin != tt.wantMin {
				t.Fatalf(
					"normalizePoolLimits(%d, %d) = (%d, %d), want (%d, %d)",
					tt.max,
					tt.min,
					gotMax,
					gotMin,
					tt.wantMax,
					tt.wantMin,
				)
			}
		})
	}
}

func TestCanHoldSessionLockDuringWork(t *testing.T) {
	tests := []struct {
		name     string
		poolMax  int32
		sqlMax   int
		expected bool
	}{
		{name: "one pgx slot", poolMax: 1, sqlMax: 2, expected: false},
		{name: "one database sql slot", poolMax: 2, sqlMax: 1, expected: false},
		{name: "two slots in both layers", poolMax: 2, sqlMax: 2, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := pgxpool.ParseConfig("postgres://test:test@127.0.0.1:1/test")
			if err != nil {
				t.Fatalf("parse pool config: %v", err)
			}
			cfg.MaxConns = tt.poolMax
			cfg.MinConns = 0

			pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
			if err != nil {
				t.Fatalf("create lazy pool: %v", err)
			}
			t.Cleanup(pool.Close)

			stdDB := stdlib.OpenDBFromPool(pool)
			stdDB.SetMaxOpenConns(tt.sqlMax)
			t.Cleanup(func() { _ = stdDB.Close() })

			db := &PostgresDB{
				DB:   sqlx.NewDb(stdDB, "pgx"),
				pool: pool,
			}
			if got := db.CanHoldSessionLockDuringWork(); got != tt.expected {
				t.Fatalf("CanHoldSessionLockDuringWork() = %v, want %v", got, tt.expected)
			}
		})
	}
}
