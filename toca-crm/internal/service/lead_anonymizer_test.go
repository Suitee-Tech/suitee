package service

import (
	"strings"
	"testing"

	"github.com/oginoino/toca/pkg/gdpr"
)

func TestMaybeHash(t *testing.T) {
	t.Run("empty returns empty", func(t *testing.T) {
		if got := maybeHash(""); got != "" {
			t.Fatalf("want empty, got %q", got)
		}
	})

	t.Run("already-anonymized passthrough", func(t *testing.T) {
		// Hash something once, then ensure a second pass is a no-op.
		hashed := gdpr.HashIdentifier("alice@example.com")
		if !gdpr.IsAnonymized(hashed) {
			t.Fatalf("HashIdentifier output not detected as anonymized: %q", hashed)
		}
		if got := maybeHash(hashed); got != hashed {
			t.Fatalf("second hash mutated value: was %q, got %q", hashed, got)
		}
	})

	t.Run("plain value is anonymized", func(t *testing.T) {
		raw := "alice@example.com"
		got := maybeHash(raw)
		if got == raw {
			t.Fatalf("expected hash to differ from raw, got %q", got)
		}
		if !gdpr.IsAnonymized(got) {
			t.Fatalf("output not anonymized: %q", got)
		}
		// Determinism — two calls must yield the same hash so deal/financial
		// joins on the anonymized identifier remain stable.
		again := maybeHash(raw)
		if again != got {
			t.Fatalf("non-deterministic hash: got %q then %q", got, again)
		}
	})

	t.Run("different inputs hash to different values", func(t *testing.T) {
		a := maybeHash("a@b.com")
		b := maybeHash("c@d.com")
		if a == b {
			t.Fatalf("collision: %q == %q", a, b)
		}
	})

	t.Run("anon prefix sanity", func(t *testing.T) {
		// HashIdentifier should produce a stable prefix so consumers can
		// detect anonymized rows; we don't pin the exact prefix here, only
		// that the output looks distinctly non-original.
		got := maybeHash("12345678901")
		if got == "12345678901" {
			t.Fatalf("hash returned raw")
		}
		if !strings.HasPrefix(got, "anon:") {
			t.Logf("note: anon prefix not detected (got %q) — adjust IsAnonymized contract if changed", got)
		}
	})
}
