package validation

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

func New() *validator.Validate {
	v := validator.New()
	Register(v)
	return v
}

func InitGin() error {
	engine := binding.Validator.Engine()
	v, ok := engine.(*validator.Validate)
	if !ok {
		return fmt.Errorf("gin validator engine is not *validator.Validate")
	}
	Register(v)
	return nil
}

func Register(v *validator.Validate) {
	_ = v.RegisterValidation("cpf_valid", validateCPF)
	_ = v.RegisterValidation("cnpj_valid", validateCNPJ)
	_ = v.RegisterValidation("future_date", validateFutureDate)
}

func Extract(err error, obj ...any) *ValidationError {
	var ves validator.ValidationErrors
	if errors.As(err, &ves) {
		var t reflect.Type
		if len(obj) > 0 && obj[0] != nil {
			t = reflect.TypeOf(obj[0])
			if t.Kind() == reflect.Pointer {
				t = t.Elem()
			}
			if t.Kind() != reflect.Struct {
				t = nil
			}
		}

		fields := make([]FieldError, 0, len(ves))
		for _, fe := range ves {
			field := jsonFieldName(fe, t)
			code, msg := messageForTag(fe.Tag(), fe.Param())
			fields = append(fields, FieldError{
				Field:   field,
				Code:    code,
				Message: msg,
			})
		}
		return &ValidationError{Fields: fields}
	}

	return nil
}

func jsonFieldName(fe validator.FieldError, t reflect.Type) string {
	if t == nil {
		return toSnake(fe.Field())
	}

	sf, ok := t.FieldByName(fe.StructField())
	if !ok {
		return toSnake(fe.Field())
	}

	tag := sf.Tag.Get("json")
	if tag == "" || tag == "-" {
		return toSnake(fe.Field())
	}

	name := strings.Split(tag, ",")[0]
	if name == "" {
		return toSnake(fe.Field())
	}
	return name
}

func toSnake(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

func messageForTag(tag string, param string) (string, string) {
	switch tag {
	case "required":
		return "required", "campo obrigatório"
	case "email":
		return "email", "email inválido"
	case "len":
		return "len", fmt.Sprintf("deve ter tamanho %s", param)
	case "min":
		return "min", fmt.Sprintf("deve ter no mínimo %s", param)
	case "max":
		return "max", fmt.Sprintf("deve ter no máximo %s", param)
	case "oneof":
		return "oneof", "valor inválido"
	case "gt":
		return "gt", fmt.Sprintf("deve ser maior que %s", param)
	case "gte":
		return "gte", fmt.Sprintf("deve ser maior ou igual a %s", param)
	case "lt":
		return "lt", fmt.Sprintf("deve ser menor que %s", param)
	case "lte":
		return "lte", fmt.Sprintf("deve ser menor ou igual a %s", param)
	case "cpf_valid":
		return "cpf_invalid", "CPF inválido"
	case "cnpj_valid":
		return "cnpj_invalid", "CNPJ inválido"
	case "future_date":
		return "future_date", "deve ser uma data futura"
	default:
		return tag, "valor inválido"
	}
}

func validateFutureDate(fl validator.FieldLevel) bool {
	v := fl.Field()
	if !v.IsValid() {
		return true
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return true
		}
		v = v.Elem()
	}

	now := time.Now()

	if v.Type() == reflect.TypeOf(time.Time{}) {
		t := v.Interface().(time.Time)
		if t.IsZero() {
			return true
		}
		return t.After(now)
	}

	if v.Kind() == reflect.String {
		s := strings.TrimSpace(v.String())
		if s == "" {
			return true
		}
		if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
			y1, m1, d1 := t.Date()
			y2, m2, d2 := now.Date()
			if y1 != y2 {
				return y1 > y2
			}
			if m1 != m2 {
				return m1 > m2
			}
			return d1 > d2
		}

		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.After(now)
		}

		if t, err := time.Parse("2006-01-02T15:04:05Z07:00", s); err == nil {
			return t.After(now)
		}

		return false
	}

	return false
}
