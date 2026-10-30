package valueobjects

import (
	"encoding/json"
	"math"
)

// Money represents a monetary value in cents to avoid floating point precision issues.
type Money struct {
	cents int64
}

// NewMoney creates a new Money instance from cents.
func NewMoney(cents int64) Money {
	return Money{cents: cents}
}

// NewMoneyFromFloat converts a float64 (e.g. 10.50) to Money (1050 cents).
func NewMoneyFromFloat(val float64) Money {
	return Money{cents: int64(math.Round(val * 100))}
}

// Cents returns the internal cents value.
func (m Money) Cents() int64 {
	return m.cents
}

// Float returns the value as a float64 (e.g. 10.50).
func (m Money) Float() float64 {
	return float64(m.cents) / 100.0
}

// Add adds another Money value to this one.
func (m Money) Add(other Money) Money {
	return Money{cents: m.cents + other.cents}
}

// Subtract subtracts another Money value from this one.
func (m Money) Subtract(other Money) (Money, error) {
	return Money{cents: m.cents - other.cents}, nil
}

// Percentage returns a new Money value that is the given percentage of this one.
// pct is expected to be a value between 0 and 100 (e.g. 10 for 10%).
func (m Money) Percentage(pct float64) Money {
	return Money{cents: int64(math.Round(float64(m.cents) * (pct / 100.0)))}
}

// IsZero returns true if the value is zero.
func (m Money) IsZero() bool {
	return m.cents == 0
}

// IsNegative returns true if the value is less than zero.
func (m Money) IsNegative() bool {
	return m.cents < 0
}

// MarshalJSON implements the json.Marshaler interface.
func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.Float())
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (m *Money) UnmarshalJSON(data []byte) error {
	var f float64
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	m.cents = int64(math.Round(f * 100))
	return nil
}
