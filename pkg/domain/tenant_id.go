// Package domain hosts cross-service value types shared by every toca-*
// service. TenantID is the first such type — historically every service
// passed tenant identifiers as bare strings, which made it trivial to
// swap a tenant_id with an account_id at a call site and only learn
// about it from a Postgres "invalid input syntax for type uuid" later
// (or, worse, an RLS leak).
//
// The wire format is preserved: JSON encodes/decodes to a lowercase UUID
// string, matching what every service already speaks. The difference is
// at compile time — a *clients.EnsureChannelRequest can no longer accept
// a CustomerAccountID where it expects a TenantID.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ErrInvalidTenantID is returned when a string cannot be parsed as a
// UUID. Callers should wrap this with their own context (which DTO,
// which field) before surfacing it.
var ErrInvalidTenantID = errors.New("invalid tenant_id")

// TenantID identifies a tenant. The zero value (uuid.Nil) means
// "unset" and IsZero reports it as such — it should never be persisted
// or sent over the wire.
type TenantID uuid.UUID

// NewTenantID parses a string into a TenantID. Whitespace is trimmed.
// Empty input is rejected — use the zero value (TenantID{}) if you
// intentionally want "unset".
func NewTenantID(s string) (TenantID, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return TenantID{}, fmt.Errorf("%w: empty string", ErrInvalidTenantID)
	}
	u, err := uuid.Parse(trimmed)
	if err != nil {
		return TenantID{}, fmt.Errorf("%w: %v", ErrInvalidTenantID, err)
	}
	return TenantID(u), nil
}

// MustTenantID is the panic-on-error variant of NewTenantID, intended
// for tests and for places where the input is known-good (e.g. a value
// loaded from a column typed as UUID). Production code paths that
// accept user-provided input must use NewTenantID instead.
func MustTenantID(s string) TenantID {
	t, err := NewTenantID(s)
	if err != nil {
		panic(err)
	}
	return t
}

// TenantIDFromUUID wraps a uuid.UUID without re-parsing.
func TenantIDFromUUID(u uuid.UUID) TenantID {
	return TenantID(u)
}

// UUID returns the underlying uuid.UUID.
func (t TenantID) UUID() uuid.UUID {
	return uuid.UUID(t)
}

// String returns the canonical lowercase UUID representation.
func (t TenantID) String() string {
	return uuid.UUID(t).String()
}

// IsZero reports whether t is the zero value (uuid.Nil).
func (t TenantID) IsZero() bool {
	return uuid.UUID(t) == uuid.Nil
}

// MarshalJSON encodes the TenantID as a JSON string. Zero values
// encode as the literal null so unset fields don't accidentally serialize
// as "00000000-0000-0000-0000-000000000000".
func (t TenantID) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.String())
}

// UnmarshalJSON accepts a JSON string holding a UUID, or null/empty
// (which become the zero value).
func (t *TenantID) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*t = TenantID{}
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("%w: tenant_id must be a JSON string", ErrInvalidTenantID)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		*t = TenantID{}
		return nil
	}
	u, err := uuid.Parse(s)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTenantID, err)
	}
	*t = TenantID(u)
	return nil
}
