package lead

import "time"

// zeroTime is the zero value used to blank out time.Time fields during the
// partner projection (AssignedAt / ExpiresAt are values, not pointers).
var zeroTime time.Time

// ProjectLeadForPartner returns a privacy-projected COPY of a lead suitable
// for exposing to a PARTNER company through the shared-leads feature. The
// original is never mutated.
//
// LGPD: a lead is personal data. The projection ALWAYS keeps the
// non-identifying commercial signal a partner needs to decide whether to
// co-broker — status, the (derived) stage, and the property interest. It
// ALWAYS strips the owner company's internal operational data: the assigned
// broker, the roulette/SLA state, escalation counters, qualification notes
// and the full interaction history (free-text notes routinely contain PII).
//
// When shareContact is false the direct identifiers (name, e-mail, phone,
// avatar) and the auth metadata are also cleared, so the partner sees an
// anonymous opportunity. When true those identifiers are kept — but the
// internal fields above are STILL stripped, because operational privacy is
// independent of contact sharing.
func ProjectLeadForPartner(l *Lead, shareContact bool) *Lead {
	if l == nil {
		return nil
	}

	// Shallow copy. Pointers/slices/maps are reassigned explicitly below so
	// the caller's value is never mutated through the returned pointer.
	cp := *l

	// Internal / operational data of the owner company — ALWAYS stripped.
	cp.AssignedBrokerID = ""
	cp.History = nil
	cp.Qualification = nil
	cp.SLARetries = nil
	cp.RouletteRetries = 0
	cp.EscalationLevel = 0
	cp.AssignedAt = zeroTime
	cp.ExpiresAt = zeroTime
	cp.FirstContactAt = nil
	cp.FirstContactSLAViolatedAt = nil
	cp.LastInteraction = nil
	// Auth metadata is never partner-facing regardless of contact sharing.
	cp.AuthMetadata = AuthMetadata{}
	cp.AccountID = ""
	cp.ContractID = ""
	cp.CaptacaoPropertyID = ""
	cp.OwnerLeadID = ""
	cp.PropertyIDs = nil

	if !shareContact {
		// Direct identifiers — PII. Cleared when the agreement does not
		// share owner contact.
		cp.Name = ""
		cp.Email = ""
		cp.Phone = ""
		cp.Avatar = ""
	}

	// Status, Role and PropertyInterest are intentionally kept — they are the
	// commercial signal the partner needs and carry no direct identifier.

	return &cp
}
