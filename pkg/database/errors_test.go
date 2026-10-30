package database

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsTimeout(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"deadline exceeded", context.DeadlineExceeded, true},
		{"canceled", context.Canceled, true},
		{"wrapped deadline", fmt.Errorf("query failed: %w", context.DeadlineExceeded), true},
		{"pg query canceled (57014)", &pgconn.PgError{Code: "57014"}, true},
		{"wrapped pg 57014", fmt.Errorf("scan rows: %w", &pgconn.PgError{Code: "57014"}), true},
		{"pg unique violation (23505)", &pgconn.PgError{Code: "23505"}, false},
		{"generic error", errors.New("boom"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsTimeout(tc.err); got != tc.want {
				t.Fatalf("IsTimeout(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
