package valueobjects

import (
	"encoding/json"
	"testing"
)

type TestStruct struct {
	Amount Money `json:"amount"`
}

func TestMoney_MarshalJSON(t *testing.T) {
	m := NewMoneyFromFloat(10.50)
	ts := TestStruct{Amount: m}

	data, err := json.Marshal(ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := `{"amount":10.5}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func TestMoney_UnmarshalJSON(t *testing.T) {
	jsonData := []byte(`{"amount": 10.50}`)
	var ts TestStruct

	err := json.Unmarshal(jsonData, &ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ts.Amount.Cents() != 1050 {
		t.Errorf("expected cents 1050, got %d", ts.Amount.Cents())
	}
	if ts.Amount.Float() != 10.50 {
		t.Errorf("expected float 10.50, got %f", ts.Amount.Float())
	}
}

func TestMoney_RoundTripJSON(t *testing.T) {
	original := NewMoneyFromFloat(123.45)
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var unmarshaled Money
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if original.Cents() != unmarshaled.Cents() {
		t.Errorf("expected %d, got %d", original.Cents(), unmarshaled.Cents())
	}
}
