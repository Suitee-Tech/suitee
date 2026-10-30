package services

import (
	"os"
	"testing"
	"time"
)

// White-box test inside the `service` package (not `service_test`) so it
// can poke `readPresignedTTL` without exporting it. The env var
// `TOCA_PRESIGNED_URL_TTL` is read at process startup into
// `presignedURLTTL`, but the helper itself is pure and reads each call —
// safe to exercise here.

func TestReadPresignedTTL_UnsetReturnsDefault(t *testing.T) {
	t.Setenv("TOCA_PRESIGNED_URL_TTL", "")
	if got := readPresignedTTL(); got != presignedURLTTLDefault {
		t.Errorf("got %v, want default %v", got, presignedURLTTLDefault)
	}
}

func TestReadPresignedTTL_ValidWithinBounds(t *testing.T) {
	t.Setenv("TOCA_PRESIGNED_URL_TTL", "5m")
	if got := readPresignedTTL(); got != 5*time.Minute {
		t.Errorf("got %v, want 5m", got)
	}
}

func TestReadPresignedTTL_BelowMinFallsBackToDefault(t *testing.T) {
	t.Setenv("TOCA_PRESIGNED_URL_TTL", "10s")
	if got := readPresignedTTL(); got != presignedURLTTLDefault {
		t.Errorf("got %v, want default (10s is below 1m floor)", got)
	}
}

func TestReadPresignedTTL_AboveMaxFallsBackToDefault(t *testing.T) {
	t.Setenv("TOCA_PRESIGNED_URL_TTL", "2h")
	if got := readPresignedTTL(); got != presignedURLTTLDefault {
		t.Errorf("got %v, want default (2h exceeds 1h ceiling)", got)
	}
}

func TestReadPresignedTTL_GarbageFallsBackToDefault(t *testing.T) {
	t.Setenv("TOCA_PRESIGNED_URL_TTL", "definitely-not-a-duration")
	if got := readPresignedTTL(); got != presignedURLTTLDefault {
		t.Errorf("got %v, want default for unparseable input", got)
	}
}

// Sanity guard so a future refactor doesn't silently lower the default.
func TestPresignedTTLDefault_IsFifteenMinutes(t *testing.T) {
	if presignedURLTTLDefault != 15*time.Minute {
		t.Errorf("default = %v, expected 15m (matches the historical hardcode)", presignedURLTTLDefault)
	}
	_ = os.Getenv("TOCA_PRESIGNED_URL_TTL") // touch to avoid unused-import on `os`
}
