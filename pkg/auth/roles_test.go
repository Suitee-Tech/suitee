package auth

import "testing"

// TestIsRootProfile pins the set of profiles that grant blanket access via
// the middleware bypass + provider bypass. Tightening this list (or
// loosening it) is a security-critical change — the test exists so a
// silent edit shows up in review.
func TestIsRootProfile(t *testing.T) {
	cases := []struct {
		name    string
		profile string
		want    bool
	}{
		{"owner lowercase", "owner", true},
		{"admin lowercase", "admin", true},
		{"owner uppercase", "OWNER", true},
		{"admin mixed case", "Admin", true},
		{"owner with whitespace", "  owner  ", true},
		{"corretor is not root", "corretor", false},
		{"gestor is not root", "gestor", false},
		{"empty is not root", "", false},
		{"superadmin is not root (typo guard)", "superadmin", false},
		{"random is not root", "random", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsRootProfile(tc.profile); got != tc.want {
				t.Errorf("IsRootProfile(%q) = %v, want %v", tc.profile, got, tc.want)
			}
		})
	}
}
