package handlers

const (
	defaultPageLimit = 50
	maxPageLimit     = 200
)

// normalizePaging clamps user-supplied limit/offset to a safe range and
// returns the matching 1-based page index. It guards the page calculation
// against a divide-by-zero panic when limit <= 0 — a malformed `?limit=0`
// would otherwise reach `offset/limit` and crash the handler with a 500.
// limit, offset and page are returned together so pagination metadata stays
// coherent with the values actually used to query.
func normalizePaging(limit, offset int64) (int64, int64, int) {
	if limit <= 0 {
		limit = defaultPageLimit
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}
	if offset < 0 {
		offset = 0
	}
	page := int(offset/limit) + 1
	return limit, offset, page
}
