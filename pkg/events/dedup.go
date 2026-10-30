package events

import (
	"strings"

	"github.com/google/uuid"
)

// notificationNamespace is the deterministic UUID v5 namespace for
// notifications/messages derived from events. Fixed so the same input parts
// always produce the same output ID across services and process restarts.
//
// Generated once with `uuid.NewSHA1(uuid.NameSpaceURL, []byte("toca/events/notification"))`.
var notificationNamespace = uuid.MustParse("d7b8e6e6-7e16-5d3b-9c5b-1f87a5e8cd6e")

// DeriveNotificationID returns a deterministic UUID v5 for handler-side
// idempotency. Handlers that produce side effects from events (in-app
// notifications, external messaging, etc.) can use this to derive a stable
// row ID from a natural key (event ID + target user + action), so repeated
// deliveries — or different events that map to the same logical side
// effect — collapse to a single row via ON CONFLICT DO NOTHING.
//
// Parts are joined with "|" (a separator unlikely to appear in IDs) so
// "a|b" and "ab|" produce different IDs. Empty parts are skipped.
//
// Example: DeriveNotificationID(evt.EventID(), userID, "ticket_created")
func DeriveNotificationID(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		cleaned = append(cleaned, p)
	}
	if len(cleaned) == 0 {
		return uuid.New().String()
	}
	key := strings.Join(cleaned, "|")
	return uuid.NewSHA1(notificationNamespace, []byte(key)).String()
}
