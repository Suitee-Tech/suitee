// Package pagination provides cursor-based pagination primitives shared
// by every service's listing endpoints.
//
// Why cursor and not offset:
//   - offset/limit is O(N) at the DB level: SELECT * FROM big ORDER BY x
//     OFFSET 900000 LIMIT 50 still has to scan 900050 rows
//   - cursor pagination is O(1) per page when the cursor column is
//     indexed: SELECT * FROM big WHERE x > $1 ORDER BY x LIMIT 50
//   - cursors are stable across inserts; offsets shift items between
//     pages whenever a new row lands ahead of the cursor
//
// Pattern: encode the last seen sort-key + id into an opaque base64
// token; decoders ignore unknown fields so the cursor format can evolve
// without breaking existing clients.
package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DefaultPageSize is the requested page size when callers pass 0.
// Picked deliberately small so unbounded clients can't blow up a
// listing endpoint.
const DefaultPageSize = 25

// MaxPageSize caps client-supplied page sizes. Larger requests are
// silently clipped to this value — exposing the cap via metadata is
// safer than returning an error that breaks pagination tooling.
const MaxPageSize = 100

// Cursor is the opaque shape encoded into the page_token. Callers must
// not depend on its exact field set — only on the round-trip Encode/Decode.
type Cursor struct {
	// SortValue is the ORDER BY value of the LAST row in the previous
	// page. WHERE clause becomes `(sort_col, id) > (sort_value, id)`
	// for ascending order, reverse for descending. Stored as an
	// interface{} so it can hold time.Time, string, int64, etc.
	SortValue any `json:"v,omitempty"`
	// ID is the primary key of the LAST row, used as tiebreaker when
	// SortValue is non-unique (created_at can collide on bulk imports).
	ID string `json:"id,omitempty"`
	// Direction is "next" (forward pagination) or "prev" (backward).
	// "next" is the only well-tested path today; "prev" is reserved
	// for future use.
	Direction string `json:"dir,omitempty"`
}

// Page wraps a slice of items with the next-cursor metadata clients
// need to request the following page. NextToken is empty when no more
// rows exist.
type Page[T any] struct {
	Items     []T    `json:"items"`
	NextToken string `json:"next_token,omitempty"`
	PageSize  int    `json:"page_size"`
}

// Decode parses an opaque page_token back into a Cursor. Empty input
// returns the zero Cursor (start from the beginning). Invalid base64
// or JSON returns an error so callers can reject the request with 400
// instead of silently restarting.
func Decode(token string) (Cursor, error) {
	if token == "" {
		return Cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Cursor{}, fmt.Errorf("invalid cursor encoding: %w", err)
	}
	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return Cursor{}, fmt.Errorf("invalid cursor payload: %w", err)
	}
	return c, nil
}

// Encode turns a Cursor into its base64 representation. The output is
// URL-safe (no padding) so it can travel as a query string.
func Encode(c Cursor) (string, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// NormalizePageSize clips a client-supplied page size to [1, MaxPageSize]
// with DefaultPageSize as the fallback when zero is provided. Always
// returns a usable value.
func NormalizePageSize(requested int) int {
	if requested <= 0 {
		return DefaultPageSize
	}
	if requested > MaxPageSize {
		return MaxPageSize
	}
	return requested
}

// EncodeTime helps the common pattern where the cursor key is a
// timestamp + id. The time is rendered as RFC3339Nano so it survives
// JSON round-trip without precision loss.
func EncodeTime(t time.Time, id string) (string, error) {
	if id == "" {
		return "", errors.New("id is required")
	}
	return Encode(Cursor{SortValue: t.UTC().Format(time.RFC3339Nano), ID: id, Direction: "next"})
}

// DecodeTime is the inverse of EncodeTime. Returns the time + id +
// direction, or an error when the token is malformed.
func DecodeTime(token string) (time.Time, string, string, error) {
	c, err := Decode(token)
	if err != nil {
		return time.Time{}, "", "", err
	}
	if c.SortValue == nil {
		return time.Time{}, c.ID, c.Direction, nil
	}
	s, ok := c.SortValue.(string)
	if !ok {
		return time.Time{}, "", "", fmt.Errorf("cursor sort value is not a string: %T", c.SortValue)
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, "", "", fmt.Errorf("invalid cursor timestamp: %w", err)
	}
	return t, c.ID, c.Direction, nil
}

// SanityCheckDirection returns "next" when the input isn't a known
// direction value. Tightening the typing here without breaking older
// clients that omit the field.
func SanityCheckDirection(d string) string {
	switch strings.ToLower(d) {
	case "next", "prev":
		return strings.ToLower(d)
	default:
		return "next"
	}
}
