package valueobjects

import (
	"errors"
	"regexp"
)

var numericRegex = regexp.MustCompile(`\D`)

type Phone struct {
	value string
}

// NewPhone normalizes Brazilian phone formats (removes masks, checks length).
func NewPhone(phone string) (Phone, error) {
	val := numericRegex.ReplaceAllString(phone, "")
	if val == "" {
		return Phone{}, errors.New("phone cannot be empty")
	}

	// Basic Brazilian number validation:
	// Usually 10 or 11 digits (with DDD), occasionally 12 or 13 if country code is included.
	if len(val) < 10 || len(val) > 14 {
		return Phone{}, errors.New("invalid phone length")
	}

	return Phone{value: val}, nil
}

func (p Phone) String() string {
	return p.value
}
