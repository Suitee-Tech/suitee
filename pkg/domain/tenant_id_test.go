package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

const sampleUUID = "11111111-2222-3333-4444-555555555555"

func TestNewTenantID(t *testing.T) {
	t.Run("valid uuid", func(t *testing.T) {
		got, err := NewTenantID(sampleUUID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.String() != sampleUUID {
			t.Fatalf("expected %q, got %q", sampleUUID, got.String())
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		got, err := NewTenantID("  " + sampleUUID + "\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.String() != sampleUUID {
			t.Fatalf("expected %q, got %q", sampleUUID, got.String())
		}
	})

	t.Run("empty rejected", func(t *testing.T) {
		_, err := NewTenantID("")
		if !errors.Is(err, ErrInvalidTenantID) {
			t.Fatalf("expected ErrInvalidTenantID, got %v", err)
		}
	})

	t.Run("garbage rejected", func(t *testing.T) {
		_, err := NewTenantID("not-a-uuid")
		if !errors.Is(err, ErrInvalidTenantID) {
			t.Fatalf("expected ErrInvalidTenantID, got %v", err)
		}
	})
}

func TestMustTenantID_PanicsOnInvalid(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = MustTenantID("nope")
}

func TestTenantID_JSONRoundTrip(t *testing.T) {
	type wrapper struct {
		TenantID TenantID `json:"tenant_id"`
	}

	in := wrapper{TenantID: MustTenantID(sampleUUID)}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	expected := `{"tenant_id":"` + sampleUUID + `"}`
	if string(raw) != expected {
		t.Fatalf("expected %q, got %q", expected, string(raw))
	}

	var out wrapper
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.TenantID != in.TenantID {
		t.Fatalf("roundtrip mismatch: %v vs %v", in.TenantID, out.TenantID)
	}
}

func TestTenantID_MarshalZeroAsNull(t *testing.T) {
	var zero TenantID
	raw, err := json.Marshal(zero)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != "null" {
		t.Fatalf("zero TenantID should marshal as null, got %q", string(raw))
	}
}

func TestTenantID_UnmarshalNullAndEmpty(t *testing.T) {
	for _, payload := range []string{`null`, `""`, `"   "`} {
		var got TenantID = MustTenantID(sampleUUID)
		if err := json.Unmarshal([]byte(payload), &got); err != nil {
			t.Fatalf("unmarshal %q: %v", payload, err)
		}
		if !got.IsZero() {
			t.Fatalf("payload %q should yield zero, got %v", payload, got)
		}
	}
}

func TestTenantID_UnmarshalRejectsBadShape(t *testing.T) {
	for _, payload := range []string{`123`, `true`, `{"id":"x"}`, `"not-a-uuid"`} {
		var got TenantID
		err := json.Unmarshal([]byte(payload), &got)
		if err == nil {
			t.Fatalf("payload %q should have failed", payload)
		}
		if !errors.Is(err, ErrInvalidTenantID) {
			t.Fatalf("payload %q: expected ErrInvalidTenantID, got %v", payload, err)
		}
	}
}

func TestTenantID_FromUUID(t *testing.T) {
	u := uuid.MustParse(sampleUUID)
	tid := TenantIDFromUUID(u)
	if tid.UUID() != u {
		t.Fatalf("expected %v, got %v", u, tid.UUID())
	}
	if !strings.EqualFold(tid.String(), sampleUUID) {
		t.Fatalf("expected %s, got %s", sampleUUID, tid.String())
	}
}

func TestTenantID_IsZero(t *testing.T) {
	var z TenantID
	if !z.IsZero() {
		t.Fatal("zero value should be IsZero")
	}
	nz := MustTenantID(sampleUUID)
	if nz.IsZero() {
		t.Fatal("non-zero value should not be IsZero")
	}
}
