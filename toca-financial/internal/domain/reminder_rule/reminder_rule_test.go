package reminder_rule

import (
	"testing"
	"time"
)

func TestNew_validation(t *testing.T) {
	cases := []struct {
		name        string
		tenant      string
		ruleName    string
		trigger     TriggerType
		offset      int
		channels    []Channel
		template    string
		expectError bool
	}{
		{"happy before_due", "tenant-1", "3d antes", TriggerBeforeDue, 3, []Channel{ChannelEmail}, "msg", false},
		{"on_due needs zero offset", "tenant-1", "no dia", TriggerOnDue, 0, []Channel{ChannelEmail}, "msg", false},
		{"on_due rejects nonzero offset", "tenant-1", "no dia", TriggerOnDue, 1, []Channel{ChannelEmail}, "msg", true},
		{"before_due rejects zero offset", "tenant-1", "antes", TriggerBeforeDue, 0, []Channel{ChannelEmail}, "msg", true},
		{"after_due rejects zero offset", "tenant-1", "depois", TriggerAfterDue, 0, []Channel{ChannelEmail}, "msg", true},
		{"unknown trigger rejected", "tenant-1", "x", "INVALID", 1, []Channel{ChannelEmail}, "msg", true},
		// `sms` passou a ser canal válido da régua (F1.1): o caso de "canal
		// desconhecido" usa agora um valor que realmente não existe.
		{"unknown channel rejected", "tenant-1", "x", TriggerBeforeDue, 1, []Channel{Channel("telegram")}, "msg", true},
		{"sms accepted", "tenant-1", "x", TriggerBeforeDue, 1, []Channel{ChannelSMS}, "msg", false},
		{"missing template rejected", "tenant-1", "x", TriggerBeforeDue, 1, []Channel{ChannelEmail}, "", true},
		{"missing name rejected", "tenant-1", "", TriggerBeforeDue, 1, []Channel{ChannelEmail}, "msg", true},
		{"missing tenant rejected", "", "x", TriggerBeforeDue, 1, []Channel{ChannelEmail}, "msg", true},
		{"empty channels rejected", "tenant-1", "x", TriggerBeforeDue, 1, nil, "msg", true},
		{"channels deduped", "tenant-1", "x", TriggerBeforeDue, 1, []Channel{ChannelEmail, ChannelEmail, ChannelInApp}, "msg", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := New(tc.tenant, tc.ruleName, tc.trigger, tc.offset, tc.channels, tc.template)
			gotErr := err != nil
			if gotErr != tc.expectError {
				t.Fatalf("expected error=%v, got %v (err=%v)", tc.expectError, gotErr, err)
			}
			if !tc.expectError && tc.name == "channels deduped" {
				if len(r.Channels) != 2 {
					t.Fatalf("expected channels deduped to 2, got %v", r.Channels)
				}
			}
		})
	}
}

func TestMatchesDueDate(t *testing.T) {
	due := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)

	beforeDue, _ := New("t", "3 dias antes", TriggerBeforeDue, 3, []Channel{ChannelEmail}, "msg")
	onDue, _ := New("t", "no dia", TriggerOnDue, 0, []Channel{ChannelEmail}, "msg")
	afterDue, _ := New("t", "7 dias depois", TriggerAfterDue, 7, []Channel{ChannelEmail}, "msg")

	if !beforeDue.MatchesDueDate(time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC), due) {
		t.Errorf("before_due should match 2026-05-17 for due 2026-05-20")
	}
	if beforeDue.MatchesDueDate(time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC), due) {
		t.Errorf("before_due should NOT match 2026-05-18 for due 2026-05-20")
	}
	if !onDue.MatchesDueDate(time.Date(2026, 5, 20, 12, 30, 0, 0, time.UTC), due) {
		t.Errorf("on_due should match same day regardless of time")
	}
	if !afterDue.MatchesDueDate(time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC), due) {
		t.Errorf("after_due 7 should match 2026-05-27 for due 2026-05-20")
	}
}
