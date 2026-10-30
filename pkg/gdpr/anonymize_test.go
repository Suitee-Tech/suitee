package gdpr

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestHashIdentifierIsDeterministicAndPrefixed(t *testing.T) {
	a := HashIdentifier("maria@cutia.tech")
	b := HashIdentifier("maria@cutia.tech")
	if a != b {
		t.Fatalf("not deterministic: %q != %q", a, b)
	}
	if !strings.HasPrefix(a, AnonPrefix) {
		t.Fatalf("missing prefix: %q", a)
	}
	if !IsAnonymized(a) {
		t.Fatalf("IsAnonymized returned false for %q", a)
	}
}

func TestHashIdentifierDifferentInputsDiffer(t *testing.T) {
	if HashIdentifier("a") == HashIdentifier("b") {
		t.Fatal("expected different hashes for different inputs")
	}
}

type stubAnonymizer struct {
	domain  string
	touched int
	err     error
	called  int
	mu      sync.Mutex
}

func (s *stubAnonymizer) Domain() string { return s.domain }
func (s *stubAnonymizer) Anonymize(_ context.Context, _ string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.called++
	return s.touched, s.err
}

func TestRegisterAndAnonymizeAccount(t *testing.T) {
	// Reset registry for isolation.
	registry = nil

	ok := &stubAnonymizer{domain: "test.ok", touched: 7}
	fail := &stubAnonymizer{domain: "test.fail", err: errors.New("boom")}

	Register(ok)
	Register(fail)
	Register(ok) // duplicate — should be ignored

	got := AnonymizeAccount(context.Background(), "acct-1")
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0].Domain != "test.ok" || got[0].RowsTouched != 7 || got[0].Err != nil {
		t.Errorf("unexpected result[0]: %+v", got[0])
	}
	if got[1].Domain != "test.fail" || got[1].Err == nil {
		t.Errorf("unexpected result[1]: %+v", got[1])
	}
	if ok.called != 1 || fail.called != 1 {
		t.Errorf("call counts wrong: ok=%d fail=%d", ok.called, fail.called)
	}

	summary := SummaryString(got)
	if !strings.Contains(summary, "test.ok=7 rows") || !strings.Contains(summary, "test.fail=error") {
		t.Errorf("summary missing expected content: %s", summary)
	}
}

func TestAnonymizeAccountEmptyIDIsNoOp(t *testing.T) {
	registry = []Anonymizer{&stubAnonymizer{domain: "x"}}
	if r := AnonymizeAccount(context.Background(), ""); r != nil {
		t.Fatalf("expected nil for empty account id, got %+v", r)
	}
}
