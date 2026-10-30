package broker

import (
	"testing"
	"time"
)

func TestIsCurrentlyPaused(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	future := now.Add(1 * time.Hour)
	past := now.Add(-1 * time.Hour)

	cases := []struct {
		name string
		b    Broker
		want bool
	}{
		{name: "nil-safe via zero value", b: Broker{}, want: false},
		{name: "paused false stays unpaused even with future PausedUntil", b: Broker{Paused: false, PausedUntil: &future}, want: false},
		{name: "paused indefinitely (no PausedUntil)", b: Broker{Paused: true}, want: true},
		{name: "paused with future PausedUntil — still paused", b: Broker{Paused: true, PausedUntil: &future}, want: true},
		{name: "paused with expired PausedUntil — auto-unpaused", b: Broker{Paused: true, PausedUntil: &past}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.b.IsCurrentlyPaused(now); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsCurrentlyPaused_NilReceiverSafe(t *testing.T) {
	var b *Broker
	if b.IsCurrentlyPaused(time.Now()) {
		t.Fatalf("nil broker should not be paused")
	}
}
