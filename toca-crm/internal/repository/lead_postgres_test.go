package repository

import (
	"errors"
	"testing"
)

func TestIsMissingUpdatedSeq(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "unrelated error", err: errors.New("connection refused"), want: false},
		{
			name: "pgx column not found",
			err:  errors.New(`ERROR: column "updated_seq" does not exist (SQLSTATE 42703)`),
			want: true,
		},
		{
			name: "lowercase variant",
			err:  errors.New("column updated_seq does not exist"),
			want: true,
		},
		{
			name: "different column name",
			err:  errors.New(`ERROR: column "other_col" does not exist`),
			want: false,
		},
		{
			name: "mentions updated_seq but not missing",
			err:  errors.New("updated_seq value out of range"),
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isMissingUpdatedSeq(tc.err); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
