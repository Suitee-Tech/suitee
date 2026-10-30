package clicksign

import "testing"

func TestVerifyHMAC(t *testing.T) {
	body := []byte(`{"event":{"name":"sign"}}`)
	secret := "shh"
	good := SignBody(body, secret)

	t.Run("valid", func(t *testing.T) {
		if err := VerifyHMAC(body, good, secret); err != nil {
			t.Fatalf("expected valid, got %v", err)
		}
	})

	t.Run("tampered body", func(t *testing.T) {
		if err := VerifyHMAC([]byte(`{"event":{"name":"refusal"}}`), good, secret); err == nil {
			t.Fatal("expected mismatch error")
		}
	})

	t.Run("wrong secret", func(t *testing.T) {
		if err := VerifyHMAC(body, good, "other"); err == nil {
			t.Fatal("expected mismatch error")
		}
	})

	t.Run("missing header", func(t *testing.T) {
		if err := VerifyHMAC(body, "", secret); err == nil {
			t.Fatal("expected error for missing header")
		}
	})

	t.Run("missing secret", func(t *testing.T) {
		if err := VerifyHMAC(body, good, ""); err == nil {
			t.Fatal("expected error for missing secret")
		}
	})
}
