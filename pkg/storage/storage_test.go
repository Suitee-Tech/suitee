package storage

import "testing"

func TestSanitizeFilenameBase(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"parentheses and space", "comprovante (1)", "comprovante_1"},
		{"accents inside parentheses", "documento (cópia)", "documento_cpia"},
		{"square brackets", "arquivo [final]", "arquivo_final"},
		{"already safe", "contrato_v2", "contrato_v2"},
		{"dotted base preserved", "foo.bar", "foo.bar"},
		{"only special chars", "@@@", "file"},
		{"emoji only", "📄🎉", "file"},
		{"empty", "", "file"},
		{"leading dot not hidden", ".env", "env"},
		{"trailing separators trimmed", "report_ ", "report"},
		{"collapses removed accent letters", "São Paulo", "So_Paulo"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SanitizeFilenameBase(c.in)
			if got != c.want {
				t.Errorf("SanitizeFilenameBase(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
