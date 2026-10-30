package valueobjects

import (
	"errors"
	"regexp"
	"strings"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

type Email struct {
	value string
}

func NewEmail(email string) (Email, error) {
	val := strings.TrimSpace(strings.ToLower(email))
	if val == "" {
		return Email{}, errors.New("email cannot be empty")
	}
	if !emailRegex.MatchString(val) {
		return Email{}, errors.New("invalid email format")
	}
	return Email{value: val}, nil
}

func (e Email) String() string {
	return e.value
}
