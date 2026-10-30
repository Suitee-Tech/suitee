package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// B8 — a resolução do storage local ficou numa única função confinada ao
// diretório base. Ela substitui quatro cópias do mesmo `filepath.Join(STORAGE_DIR,
// parts[1])` que existiam nos handlers/serviços do toca-deal (o mount estático
// não autenticado foi removido; o download autenticado continua servindo o
// arquivo local por aqui).
func TestLocalUploadPath(t *testing.T) {
	base := t.TempDir()
	t.Setenv("STORAGE_DIR", base)

	tests := []struct {
		name     string
		fileURL  string
		wantRel  string
		wantOkay bool
	}{
		{name: "bucket e arquivo", fileURL: "/uploads/toca-documents/abc.pdf", wantRel: "toca-documents/abc.pdf", wantOkay: true},
		{name: "url absoluta", fileURL: "https://api.example.com/uploads/toca-documents/abc.pdf", wantRel: "toca-documents/abc.pdf", wantOkay: true},
		{name: "caminho aninhado", fileURL: "/uploads/toca-documents/2026/01/abc.pdf", wantRel: "toca-documents/2026/01/abc.pdf", wantOkay: true},
		{name: "fora do storage", fileURL: "https://bucket.s3.amazonaws.com/abc.pdf", wantOkay: false},
		{name: "sem arquivo", fileURL: "/uploads/", wantOkay: false},
		{name: "vazio", fileURL: "", wantOkay: false},
		{name: "traversal para fora da base", fileURL: "/uploads/../../etc/passwd", wantOkay: false},
		{name: "traversal no meio", fileURL: "/uploads/toca-documents/../../etc/passwd", wantOkay: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := LocalUploadPath(tt.fileURL)
			if ok != tt.wantOkay {
				t.Fatalf("LocalUploadPath(%q) ok=%v, want %v (path=%q)", tt.fileURL, ok, tt.wantOkay, got)
			}
			if !tt.wantOkay {
				return
			}
			want := filepath.Join(base, filepath.FromSlash(tt.wantRel))
			if got != want {
				t.Fatalf("LocalUploadPath(%q) = %q, want %q", tt.fileURL, got, want)
			}
		})
	}
}

// Sem STORAGE_DIR o default histórico continua sendo /uploads.
func TestLocalUploadPathDefaultBase(t *testing.T) {
	t.Setenv("STORAGE_DIR", "")
	got, ok := LocalUploadPath("/uploads/toca-documents/abc.pdf")
	if !ok || got != "/uploads/toca-documents/abc.pdf" {
		t.Fatalf("default = %q (ok=%v), want /uploads/toca-documents/abc.pdf", got, ok)
	}
}

func TestLocalUploadFileRequiresExistingFile(t *testing.T) {
	base := t.TempDir()
	t.Setenv("STORAGE_DIR", base)

	if _, ok := LocalUploadFile("/uploads/toca-documents/missing.pdf"); ok {
		t.Fatal("arquivo inexistente não pode resolver")
	}

	dir := filepath.Join(base, "toca-documents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, ok := LocalUploadFile("/uploads/toca-documents"); ok {
		t.Fatal("diretório não pode ser servido como arquivo")
	}

	path := filepath.Join(dir, "abc.pdf")
	if err := os.WriteFile(path, []byte("conteudo"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, ok := LocalUploadFile("/uploads/toca-documents/abc.pdf")
	if !ok || got != path {
		t.Fatalf("LocalUploadFile = %q (ok=%v), want %q", got, ok, path)
	}
}
