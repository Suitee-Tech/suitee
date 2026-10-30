package document

import "testing"

func TestSignedContractFilename(t *testing.T) {
	cases := []struct {
		name       string
		contractID string
		version    int
		want       string
	}{
		{"v1 default", "abc-123", 1, "contrato-assinado-v1-abc-123.pdf"},
		{"v2", "abc-123", 2, "contrato-assinado-v2-abc-123.pdf"},
		{"version zero rendered as v1", "abc-123", 0, "contrato-assinado-v1-abc-123.pdf"},
		{"negative version clamps to v1", "abc-123", -5, "contrato-assinado-v1-abc-123.pdf"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SignedContractFilename(tc.contractID, tc.version); got != tc.want {
				t.Errorf("SignedContractFilename(%q, %d) = %q; want %q",
					tc.contractID, tc.version, got, tc.want)
			}
		})
	}
}
