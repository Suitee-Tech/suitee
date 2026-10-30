package validation

import (
	"regexp"

	"github.com/go-playground/validator/v10"
)

var nonDigits = regexp.MustCompile(`\D+`)

// IsValidCPF reports whether the given string parses as a valid Brazilian CPF
// according to the modulus-11 algorithm. Punctuation is stripped before the
// check. Repeated-digit CPFs (e.g. "11111111111") are rejected to match the
// stricter `cnpj_valid` policy in pkg/validation/documents.go and the FE
// `isValidCnpj` in toka/utils/masks.ts — never relax this.
func IsValidCPF(s string) bool {
	digits := nonDigits.ReplaceAllString(s, "")
	if len(digits) != 11 {
		return false
	}
	if allSameDigits(digits) {
		return false
	}
	d1 := cpfDigit(digits[:9], 10)
	d2 := cpfDigit(digits[:9]+string([]byte{byte('0' + d1)}), 11)
	return digits[9] == byte('0'+d1) && digits[10] == byte('0'+d2)
}

// IsValidCNPJ reports whether the given string parses as a valid Brazilian
// CNPJ. Punctuation is stripped before the check. Repeated-digit CNPJs are
// rejected — never relax this (see memory `feedback_cnpj_validation`).
func IsValidCNPJ(s string) bool {
	digits := nonDigits.ReplaceAllString(s, "")
	if len(digits) != 14 {
		return false
	}
	if allSameDigits(digits) {
		return false
	}
	d1 := cnpjDigit(digits[:12], []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	d2 := cnpjDigit(digits[:12]+string([]byte{byte('0' + d1)}), []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	return digits[12] == byte('0'+d1) && digits[13] == byte('0'+d2)
}

func validateCPF(fl validator.FieldLevel) bool {
	s := nonDigits.ReplaceAllString(fl.Field().String(), "")
	if len(s) == 0 {
		return true
	}
	if len(s) != 11 {
		return false
	}
	if allSameDigits(s) {
		return false
	}

	d1 := cpfDigit(s[:9], 10)
	d2 := cpfDigit(s[:9]+string([]byte{byte('0' + d1)}), 11)
	return s[9] == byte('0'+d1) && s[10] == byte('0'+d2)
}

func cpfDigit(base string, weightStart int) int {
	sum := 0
	w := weightStart
	for i := 0; i < len(base); i++ {
		sum += int(base[i]-'0') * w
		w--
	}
	r := (sum * 10) % 11
	if r == 10 {
		return 0
	}
	return r
}

func validateCNPJ(fl validator.FieldLevel) bool {
	s := nonDigits.ReplaceAllString(fl.Field().String(), "")
	if len(s) == 0 {
		return true
	}
	if len(s) != 14 {
		return false
	}
	if allSameDigits(s) {
		return false
	}

	d1 := cnpjDigit(s[:12], []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	d2 := cnpjDigit(s[:12]+string([]byte{byte('0' + d1)}), []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	return s[12] == byte('0'+d1) && s[13] == byte('0'+d2)
}

func cnpjDigit(base string, weights []int) int {
	sum := 0
	for i := 0; i < len(base); i++ {
		sum += int(base[i]-'0') * weights[i]
	}
	r := sum % 11
	if r < 2 {
		return 0
	}
	return 11 - r
}

func allSameDigits(s string) bool {
	if s == "" {
		return true
	}
	first := s[0]
	for i := 1; i < len(s); i++ {
		if s[i] != first {
			return false
		}
	}
	return true
}
