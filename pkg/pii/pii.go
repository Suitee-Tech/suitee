// Package pii provides PII (Personally Identifiable Information) masking
// helpers and annotations for LGPD-aware logging.
//
// Tag your domain structs with the `pii:"<kind>"` struct tag where
// <kind> is one of: email, cpf, cnpj, phone, name, address, document.
// Then call MaskForLog(value) before passing to logger/audit/etc.
//
// Example:
//
//	type Lead struct {
//	    ID    string `json:"id"`
//	    Name  string `json:"name"  pii:"name"`
//	    Email string `json:"email" pii:"email"`
//	    Phone string `json:"phone" pii:"phone"`
//	}
//
//	logger.InfoContext(ctx, "lead.created", "lead", pii.MaskForLog(lead))
//
// In production (APP_ENV=production) MaskForLog masks every tagged field;
// in dev/test it logs the raw value to keep debugging ergonomic.
package pii

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"sync"
)

// Kind enumerates the supported PII categories. The masking strategy
// varies by kind so the masked output is still useful for debugging
// (e.g. email keeps the domain so we know it's the right Gmail account).
type Kind string

const (
	KindEmail    Kind = "email"
	KindCPF      Kind = "cpf"
	KindCNPJ     Kind = "cnpj"
	KindPhone    Kind = "phone"
	KindName     Kind = "name"
	KindAddress  Kind = "address"
	KindDocument Kind = "document"
)

var (
	// productionMode is cached because env lookup on every log call would
	// be wasteful. Reset via SetProductionMode in tests if needed.
	productionMode     bool
	productionModeOnce sync.Once
)

func isProduction() bool {
	productionModeOnce.Do(func() {
		env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
		mode := strings.ToLower(strings.TrimSpace(os.Getenv("APP_MODE")))
		productionMode = env == "production" || env == "prod" || mode == "release"
	})
	return productionMode
}

// SetProductionMode forces the env-based decision (mostly for tests that
// want to assert masking on/off behavior).
func SetProductionMode(on bool) {
	productionMode = on
	productionModeOnce.Do(func() {}) // mark as resolved so subsequent calls don't re-read env
}

// MaskForLog returns a deep copy of v with every `pii:"<kind>"` tagged
// field replaced by its masked representation. Non-struct inputs are
// returned as-is. Safe to call with nil — returns nil.
//
// In dev (non-production) MaskForLog is a no-op so logs remain useful
// for tracing real values during testing.
func MaskForLog(v any) any {
	if v == nil {
		return nil
	}
	if !isProduction() {
		return v
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return v
	}
	// Operate on a deep copy via JSON to avoid mutating caller state.
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	clone := reflect.New(rv.Type()).Interface()
	if err := json.Unmarshal(raw, clone); err != nil {
		return v
	}
	maskFields(reflect.ValueOf(clone).Elem())
	return reflect.ValueOf(clone).Elem().Interface()
}

func maskFields(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			maskFields(v.Elem())
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			tag := field.Tag.Get("pii")
			fv := v.Field(i)
			if tag != "" {
				maskValue(Kind(tag), fv)
				continue
			}
			maskFields(fv)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			maskFields(v.Index(i))
		}
	case reflect.Map:
		// Don't descend into maps — too dynamic. Callers should tag
		// the keys/values explicitly if needed.
	}
}

func maskValue(k Kind, v reflect.Value) {
	if !v.CanSet() {
		return
	}
	if v.Kind() != reflect.String {
		// For now only string fields are masked. Numeric PII (rare —
		// usually phone is stored as string anyway) can be added later.
		return
	}
	raw := v.String()
	v.SetString(MaskString(k, raw))
}

// MaskString returns a masked rendering of one value. Exported so
// callers can mask non-struct values too (e.g. emails inside maps).
//
// The strategy keeps just enough signal for debugging: first/last char
// of the local part of an email, last 4 digits of phone/document, etc.
// Empty input always returns "" (no need to mask absence).
func MaskString(k Kind, s string) string {
	if s == "" {
		return ""
	}
	switch k {
	case KindEmail:
		return maskEmail(s)
	case KindCPF, KindCNPJ, KindDocument:
		return maskDocument(s)
	case KindPhone:
		return maskPhone(s)
	case KindName:
		return maskName(s)
	case KindAddress:
		return "<address-redacted>"
	default:
		// Unknown kind — fail-closed by redacting fully.
		return "<redacted>"
	}
}

func maskEmail(s string) string {
	at := strings.Index(s, "@")
	if at < 1 {
		return "<redacted>"
	}
	local := s[:at]
	domain := s[at:]
	if len(local) <= 2 {
		return string(local[0]) + "***" + domain
	}
	return string(local[0]) + "***" + string(local[len(local)-1]) + domain
}

func maskDocument(s string) string {
	// Keep last 4 digits, redact the rest. Works for CPF (11 chars) and
	// CNPJ (14 chars) when input has dashes/dots or not.
	digits := stripNonDigits(s)
	if len(digits) <= 4 {
		return "***"
	}
	tail := digits[len(digits)-4:]
	return "***" + tail
}

func maskPhone(s string) string {
	digits := stripNonDigits(s)
	if len(digits) <= 4 {
		return "***"
	}
	return "***" + digits[len(digits)-4:]
}

func maskName(s string) string {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		if len(parts[0]) <= 1 {
			return parts[0]
		}
		return string(parts[0][0]) + "***"
	}
	first := parts[0]
	last := parts[len(parts)-1]
	if len(last) > 0 {
		return string(first[0]) + "*** " + string(last[0]) + "***"
	}
	return string(first[0]) + "***"
}

func stripNonDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
