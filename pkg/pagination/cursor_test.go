package pagination

import (
	"testing"
	"time"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	in := Cursor{SortValue: "2026-05-15T12:00:00Z", ID: "abc", Direction: "next"}
	token, err := Encode(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if token == "" {
		t.Fatal("token empty")
	}
	out, err := Decode(token)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ID != in.ID || out.Direction != in.Direction || out.SortValue != in.SortValue {
		t.Fatalf("round trip: got %+v want %+v", out, in)
	}
}

func TestDecodeEmptyReturnsZero(t *testing.T) {
	c, err := Decode("")
	if err != nil {
		t.Fatalf("decode empty: %v", err)
	}
	if c.ID != "" || c.Direction != "" || c.SortValue != nil {
		t.Fatalf("expected zero cursor, got %+v", c)
	}
}

func TestDecodeInvalidBase64(t *testing.T) {
	if _, err := Decode("!!!not-base64!!!"); err == nil {
		t.Fatal("expected error on invalid base64")
	}
}

func TestNormalizePageSize(t *testing.T) {
	tests := []struct {
		in, want int
	}{
		{0, DefaultPageSize},
		{-3, DefaultPageSize},
		{10, 10},
		{MaxPageSize, MaxPageSize},
		{MaxPageSize + 50, MaxPageSize},
	}
	for _, tc := range tests {
		if got := NormalizePageSize(tc.in); got != tc.want {
			t.Errorf("NormalizePageSize(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestEncodeDecodeTime(t *testing.T) {
	now := time.Now().UTC().Round(time.Microsecond)
	tok, err := EncodeTime(now, "row-42")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, id, dir, err := DecodeTime(tok)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if id != "row-42" {
		t.Errorf("id: got %q want %q", id, "row-42")
	}
	if dir != "next" {
		t.Errorf("dir: got %q want %q", dir, "next")
	}
	if !got.Equal(now) {
		t.Errorf("time: got %v want %v", got, now)
	}
}

func TestEncodeTimeRequiresID(t *testing.T) {
	if _, err := EncodeTime(time.Now(), ""); err == nil {
		t.Fatal("expected error when id is empty")
	}
}
