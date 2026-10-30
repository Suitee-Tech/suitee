package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// IsTimeout reports whether err is a request/query timeout rather than a
// genuine server failure: a Go context deadline/cancellation, or a Postgres
// query-canceled error (SQLSTATE 57014 — what statement_timeout raises).
//
// Handlers use it to surface HTTP 504 instead of 500 so "the request ran out
// of time" is distinguishable from "the server broke". Matches the
// *pgconn.PgError / .Code pattern already used in the repositories
// (e.g. transfer_postgres.go).
func IsTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "57014"
	}
	return false
}
