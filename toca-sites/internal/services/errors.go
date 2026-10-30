package services

import (
	"errors"
	"fmt"
)

// ErrValidation marca erro de entrada do usuário (400). Os handlers usam
// errors.Is para distinguir de falha de infraestrutura.
var ErrValidation = errors.New("validation error")

// ErrDiscarded é usado quando a requisição pública parece bot (honeypot). O
// handler responde como sucesso para não ensinar o robô, mas nada é gravado.
var ErrDiscarded = errors.New("request discarded")

// ErrSiteDisabled indica site existente porém desligado. Na zona pública é
// traduzido para 404 — o comprador não deve saber que existe uma vitrine
// desligada naquele host.
var ErrSiteDisabled = errors.New("site is disabled")

// ValidationError carrega o campo para a UI destacar o input.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string { return e.Message }
func (e ValidationError) Unwrap() error { return ErrValidation }

func invalid(field, message string) error {
	return ValidationError{Field: field, Message: message}
}

func wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", op, err)
}
