package validation

import (
	"errors"
	"testing"
)

func TestNormalizeBRPhone(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr error
	}{
		{name: "mobile spaces", raw: "11 99999 9999", want: "+5511999999999"},
		{name: "mobile parens dash", raw: "(11) 99999-9999", want: "+5511999999999"},
		{name: "mobile dots", raw: "11.99999.9999", want: "+5511999999999"},
		{name: "mobile with country code", raw: "5511999999999", want: "+5511999999999"},
		{name: "mobile with +55", raw: "+55 11 99999-9999", want: "+5511999999999"},
		{name: "landline 10 digits", raw: "1133334444", want: "+551133334444"},
		{name: "landline with parens", raw: "(11) 3333-4444", want: "+551133334444"},
		{name: "landline with country code", raw: "551133334444", want: "+551133334444"},
		{name: "mobile rio 21", raw: "21 98765 4321", want: "+5521987654321"},

		{name: "empty rejected", raw: "", wantErr: ErrInvalidBRPhone},
		{name: "only spaces rejected", raw: "   ", wantErr: ErrInvalidBRPhone},
		{name: "garbage rejected", raw: "abc", wantErr: ErrInvalidBRPhone},
		{name: "too short", raw: "1199", wantErr: ErrInvalidBRPhone},
		{name: "too long", raw: "115512345678901", wantErr: ErrInvalidBRPhone},
		{name: "invalid DDD 00", raw: "00999999999", wantErr: ErrInvalidBRPhone},
		{name: "invalid DDD 10", raw: "10999999999", wantErr: ErrInvalidBRPhone},
		{name: "mobile missing leading 9", raw: "11888888888", wantErr: ErrInvalidBRPhone},
		{name: "landline starts with 1", raw: "1112345678", wantErr: ErrInvalidBRPhone},
		{name: "landline starts with 0", raw: "1102345678", wantErr: ErrInvalidBRPhone},
		{name: "landline starts with 6", raw: "1162345678", wantErr: ErrInvalidBRPhone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeBRPhone(tc.raw)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("got err %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
