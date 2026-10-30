package validation

import (
	"errors"
	"strings"
)

// ErrInvalidBRPhone is returned by NormalizeBRPhone when the input cannot be
// coerced into a valid Brazilian phone number (mobile or landline).
var ErrInvalidBRPhone = errors.New("invalid Brazilian phone number")

// brAreaCodes is the set of valid Brazilian DDDs (area codes). Anything outside
// this set is rejected so we don't pollute the database with typos.
var brAreaCodes = map[string]struct{}{
	"11": {}, "12": {}, "13": {}, "14": {}, "15": {}, "16": {}, "17": {}, "18": {}, "19": {},
	"21": {}, "22": {}, "24": {}, "27": {}, "28": {},
	"31": {}, "32": {}, "33": {}, "34": {}, "35": {}, "37": {}, "38": {},
	"41": {}, "42": {}, "43": {}, "44": {}, "45": {}, "46": {}, "47": {}, "48": {}, "49": {},
	"51": {}, "53": {}, "54": {}, "55": {},
	"61": {}, "62": {}, "63": {}, "64": {}, "65": {}, "66": {}, "67": {}, "68": {}, "69": {},
	"71": {}, "73": {}, "74": {}, "75": {}, "77": {}, "79": {},
	"81": {}, "82": {}, "83": {}, "84": {}, "85": {}, "86": {}, "87": {}, "88": {}, "89": {},
	"91": {}, "92": {}, "93": {}, "94": {}, "95": {}, "96": {}, "97": {}, "98": {}, "99": {},
}

// NormalizeBRPhone coerces a wide range of human-formatted Brazilian phone
// numbers to E.164 (+55DDDNNNNNNNN).
//
// Accepted shapes (any whitespace, parentheses, dashes or dots are stripped):
//   - 11 99999 9999            → +5511999999999
//   - (11) 99999-9999          → +5511999999999
//   - 1133334444               → +551133334444   (landline, 10 digits)
//   - 5511999999999            → +5511999999999
//   - +55 (11) 99999-9999      → +5511999999999
//
// Returns ErrInvalidBRPhone for unparseable inputs, invalid DDDs, or numbers
// of the wrong length. Empty input also returns the error so callers can rely
// on the result being a valid token.
func NormalizeBRPhone(raw string) (string, error) {
	s := raw
	// Keep a leading '+' only; we don't allow it elsewhere.
	hasPlus := strings.HasPrefix(strings.TrimSpace(s), "+")
	digits := nonDigits.ReplaceAllString(s, "")
	if digits == "" {
		return "", ErrInvalidBRPhone
	}

	// Strip country code if present.
	switch {
	case hasPlus && strings.HasPrefix(digits, "55"):
		digits = digits[2:]
	case len(digits) > 11 && strings.HasPrefix(digits, "55"):
		// Length-based heuristic: numbers over 11 digits starting with "55"
		// are assumed to carry the country code.
		digits = digits[2:]
	}

	if len(digits) != 10 && len(digits) != 11 {
		return "", ErrInvalidBRPhone
	}
	ddd := digits[:2]
	if _, ok := brAreaCodes[ddd]; !ok {
		return "", ErrInvalidBRPhone
	}
	// Mobile numbers (11 digits) must have a leading 9 in the subscriber part.
	if len(digits) == 11 && digits[2] != '9' {
		return "", ErrInvalidBRPhone
	}
	// Landline (10 digits) cannot start with 0 or 1 in the subscriber part —
	// those ranges are reserved for special services (0800, 1056, etc.).
	if len(digits) == 10 {
		first := digits[2]
		if first < '2' || first > '5' {
			return "", ErrInvalidBRPhone
		}
	}
	return "+55" + digits, nil
}
