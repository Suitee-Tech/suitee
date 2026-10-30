// Package reminder_rule models the "Régua de Comunicação" — declarative
// rules that the financial service evaluates daily to auto-dispatch
// reminders for charges. Each rule says "N days before/after due,
// fire a reminder through channels X using template Y". The actual send
// flows through ChargeService.Notify so the audit trail and channel
// selection stay consistent with the manual UI path.
package reminder_rule

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type TriggerType string

const (
	// TriggerBeforeDue fires N days BEFORE the charge's due date.
	TriggerBeforeDue TriggerType = "BEFORE_DUE"
	// TriggerOnDue fires on the day of the due date.
	TriggerOnDue TriggerType = "ON_DUE"
	// TriggerAfterDue fires N days AFTER the due date (used for collections).
	TriggerAfterDue TriggerType = "AFTER_DUE"
)

type Channel string

const (
	ChannelEmail    Channel = "email"
	ChannelInApp    Channel = "in_app"
	ChannelWhatsApp Channel = "whatsapp"
	// ChannelSMS entrou na régua quando o canal passou a existir no toca-comm
	// (a régua só oferecia email/in_app/whatsapp e a imobiliária que opera por
	// SMS não tinha como configurar o aviso).
	ChannelSMS Channel = "sms"
)

// ReminderRule is one row of the Régua. OffsetDays is always >= 0 — the
// direction (before/on/after) is encoded by TriggerType. Storing them
// separately makes UI rendering and validation trivial; collapsing to a
// signed offset would be cute but harder to read.
type ReminderRule struct {
	ID          string      `json:"id"`
	TenantID    string      `json:"tenant_id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	TriggerType TriggerType `json:"trigger_type"`
	OffsetDays  int         `json:"offset_days"`
	Channels    []Channel   `json:"channels"`
	Template    string      `json:"template"`
	Enabled     bool        `json:"enabled"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// New constructs a rule with sensible defaults. Channels are deduped and
// validated; offset is clamped at 0 (negatives are nonsensical — direction
// is in the trigger type).
func New(tenantID, name string, trigger TriggerType, offsetDays int, channels []Channel, template string) (*ReminderRule, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.New("tenant_id is required")
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("name is required")
	}
	if err := ValidateTrigger(trigger); err != nil {
		return nil, err
	}
	if offsetDays < 0 {
		return nil, errors.New("offset_days cannot be negative; use trigger_type to choose direction")
	}
	if trigger == TriggerOnDue && offsetDays != 0 {
		return nil, errors.New("offset_days must be 0 when trigger_type is ON_DUE")
	}
	if (trigger == TriggerBeforeDue || trigger == TriggerAfterDue) && offsetDays == 0 {
		return nil, errors.New("offset_days must be >= 1 when trigger_type is BEFORE_DUE or AFTER_DUE")
	}
	deduped, err := normalizeChannels(channels)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(template) == "" {
		return nil, errors.New("template is required")
	}
	now := time.Now()
	return &ReminderRule{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		Name:        strings.TrimSpace(name),
		TriggerType: trigger,
		OffsetDays:  offsetDays,
		Channels:    deduped,
		Template:    template,
		Enabled:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func ValidateTrigger(t TriggerType) error {
	switch t {
	case TriggerBeforeDue, TriggerOnDue, TriggerAfterDue:
		return nil
	}
	return errors.New("invalid trigger_type")
}

func ValidateChannel(c Channel) error {
	switch c {
	case ChannelEmail, ChannelInApp, ChannelWhatsApp, ChannelSMS:
		return nil
	}
	return errors.New("invalid channel: " + string(c))
}

func normalizeChannels(channels []Channel) ([]Channel, error) {
	if len(channels) == 0 {
		return nil, errors.New("at least one channel is required")
	}
	seen := make(map[Channel]bool, len(channels))
	out := make([]Channel, 0, len(channels))
	for _, c := range channels {
		if err := ValidateChannel(c); err != nil {
			return nil, err
		}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out, nil
}

// MatchesDueDate returns true when a charge with the given due date should
// be hit by this rule on the reference day (typically "today"). The match
// is exact-day — the daily ticker is responsible for not double-firing
// (which is enforced by the reminder de-dup window in ChargeService).
func (r *ReminderRule) MatchesDueDate(referenceDay, dueDate time.Time) bool {
	rDay := toDay(referenceDay)
	target := r.TargetDay(dueDate)
	return rDay.Equal(target)
}

// TargetDay computes the day on which this rule should fire for a given
// charge due date. Public so tests + the engine can introspect it.
func (r *ReminderRule) TargetDay(dueDate time.Time) time.Time {
	d := toDay(dueDate)
	switch r.TriggerType {
	case TriggerBeforeDue:
		return d.AddDate(0, 0, -r.OffsetDays)
	case TriggerAfterDue:
		return d.AddDate(0, 0, r.OffsetDays)
	default:
		return d
	}
}

func toDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
