package validation

import (
	"fmt"
)

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

type ValidationError struct {
	Fields []FieldError `json:"fields"`
}

func (e ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return "validation error"
	}
	return fmt.Sprintf("validation error: %s", e.Fields[0].Field)
}
