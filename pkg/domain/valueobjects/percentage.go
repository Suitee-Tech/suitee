package valueobjects

import (
	"errors"
)

type Percentage struct {
	value float64
}

// NewPercentage creates and validates a percentage between 0 and 100.
func NewPercentage(val float64) (Percentage, error) {
	if val < 0 || val > 100 {
		return Percentage{}, errors.New("percentage must be between 0 and 100")
	}
	return Percentage{value: val}, nil
}

func (p Percentage) Float() float64 {
	return p.value
}
