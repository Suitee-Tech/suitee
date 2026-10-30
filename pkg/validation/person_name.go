package validation

import (
	"fmt"
	"strings"
)

// SanitizePersonName trims leading/trailing whitespace and collapses any run
// of internal whitespace (spaces, tabs, NBSP) to a single ASCII space. Use
// this at every input boundary (portal edit, operator form, import) so the
// downstream code can assume the field is already normalized.
//
// "  João \tSilva  " → "João Silva"
func SanitizePersonName(name string) string {
	return strings.Join(strings.Fields(name), " ")
}

// ValidatePersonName returns a user-facing error describing why `name` would
// be rejected by Clicksign (or any signer-facing system) BEFORE we burn a
// round-trip to validate it externally. Mirrors the rules Clicksign v3
// enforces when creating signers — the operator gets a precise message
// pointing at the field, instead of an opaque upstream 400.
//
// label is used in the error message (e.g. "Nome", "Sobrenome", "Razão social")
// so the same helper works for first-name / last-name / full-name fields.
//
// Rules:
//   - empty after sanitization → "<label> não pode estar vazio"
//   - shorter than 2 characters → "<label> muito curto"
//   - contains decimal digits → "<label> não pode conter números"
//
// The "must have a surname" check is NOT applied here because some callers
// validate first and last names separately. Use ValidateFullPersonName for
// the combined check.
func ValidatePersonName(name, label string) error {
	clean := SanitizePersonName(name)
	if clean == "" {
		return fmt.Errorf("%s não pode estar vazio", label)
	}
	if len([]rune(clean)) < 2 {
		return fmt.Errorf("%s muito curto (mínimo 2 caracteres)", label)
	}
	for _, r := range clean {
		if r >= '0' && r <= '9' {
			return fmt.Errorf("%s não pode conter números (recebido: %q)", label, clean)
		}
	}
	return nil
}

// ValidateFullPersonName adds the "must include a surname" rule on top of
// ValidatePersonName. Use this for fields representing a single concatenated
// name (e.g. Contract.Signatory.Name, where the Clicksign v3 API rejects
// mononyms with "name não está em um formato válido").
func ValidateFullPersonName(name, label string) error {
	if err := ValidatePersonName(name, label); err != nil {
		return err
	}
	if len(strings.Fields(name)) < 2 {
		return fmt.Errorf("%s precisa incluir sobrenome (ex: \"João Silva\")", label)
	}
	return nil
}

// IsPersonNameError informa se o erro veio de ValidatePersonName /
// ValidateFullPersonName. A camada de transporte usa para responder 400 com o
// motivo voltado ao operador ("Nome precisa incluir sobrenome") em vez de um
// 500 genérico — o texto dessas mensagens é contrato com o formulário.
func IsPersonNameError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, fragment := range []string{
		"precisa incluir sobrenome",
		"não pode estar vazio",
		"muito curto",
		"não pode conter números",
	} {
		if strings.Contains(msg, fragment) {
			return true
		}
	}
	return false
}
