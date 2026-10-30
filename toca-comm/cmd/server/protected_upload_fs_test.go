package main

import (
	"os"
	"path/filepath"
	"testing"
)

// N7 (reauditoria de 2026-09-20): a superfície legada `/uploads` é pública e o
// diretório é compartilhado por cinco serviços no compose local. A proteção era
// uma DENYLIST de um bucket só — qualquer bucket novo, ou qualquer arquivo que
// outro serviço gravasse ali, ficava legível por padrão. Agora é ALLOWLIST.

func writeUpload(t *testing.T, root, rel string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("conteudo"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestProtectedUploadFSBlocksFinancialProofs(t *testing.T) {
	root := t.TempDir()
	writeUpload(t, root, "financial-proofs/tenant-a/proof.pdf")

	if _, err := newProtectedUploadFS(root).Open("/financial-proofs/tenant-a/proof.pdf"); err == nil {
		t.Fatal("comprovantes financeiros não podem ser servidos pela superfície pública")
	}
}

// O caso que a denylist deixava passar: bucket que não está na allowlist.
func TestProtectedUploadFSBlocksBucketsOutsideAllowlist(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"toca-message-attachments/tenant-a/chat/segredo.png",
		"bucket-futuro/tenant-a/arquivo.pdf", // bucket que ainda não existe
	} {
		writeUpload(t, root, rel)
	}

	fs := newProtectedUploadFS(root)
	for _, path := range []string{
		"/toca-message-attachments/tenant-a/chat/segredo.png",
		"/bucket-futuro/tenant-a/arquivo.pdf",
	} {
		if _, err := fs.Open(path); err == nil {
			t.Fatalf("%s não pode ser servido: bucket fora da allowlist tem de nascer privado", path)
		}
	}
}

// A raiz não é servida — sem isto um diretório poderia listar todos os buckets.
func TestProtectedUploadFSDoesNotServeRoot(t *testing.T) {
	root := t.TempDir()
	fs := newProtectedUploadFS(root)

	for _, path := range []string{"/", "", "/.", "/../"} {
		if _, err := fs.Open(path); err == nil {
			t.Fatalf("raiz %q não pode ser servida", path)
		}
	}
}

// Os buckets de mídia pública legada continuam servidos: o frontend resolve
// `/uploads/...` (`shared/media.ts`, `app/lib/documents.ts`) e a tela de login
// pública usa logos.
func TestProtectedUploadFSKeepsLegacyPublicBucketsAvailable(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		"avatars/avatar.jpg",
		"toca-avatars/tenant-a/avatar.webp",
		"toca-logos/tenant-a/banner.webp",
		"toca-property-images/tenant-a/foto.jpg",
		"toca-documents/tenant-a/ticket-1/attachment.png",
	}
	for _, rel := range paths {
		writeUpload(t, root, rel)
	}

	fs := newProtectedUploadFS(root)
	for _, path := range paths {
		file, err := fs.Open("/" + path)
		if err != nil {
			t.Fatalf("mídia pública legada %s deveria continuar disponível: %v", path, err)
		}
		_ = file.Close()
	}
}
