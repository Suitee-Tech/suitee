package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

// B8 — o mount estático `/uploads` foi removido do router; o arquivo local
// continua sendo entregue pelo endpoint autenticado, que agora usa o helper
// único `serveLocalUploadIfPresent`. O teste prova as duas metades do
// contrato: arquivo local existente é servido; URL fora do storage local
// devolve false para o chamador seguir com o redirect pré-assinado.
func TestServeLocalUploadIfPresent(t *testing.T) {
	base := t.TempDir()
	t.Setenv("STORAGE_DIR", base)

	dir := filepath.Join(base, "toca-documents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := []byte("%PDF-1.4 contrato")
	if err := os.WriteFile(filepath.Join(dir, "contrato.pdf"), content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/download", func(c *gin.Context) {
		if serveLocalUploadIfPresent(c, "/uploads/toca-documents/contrato.pdf") {
			return
		}
		c.String(http.StatusFound, "redirect-to-presigned-url")
	})
	engine.GET("/download-remote", func(c *gin.Context) {
		if serveLocalUploadIfPresent(c, "https://bucket.s3.amazonaws.com/toca-documents/contrato.pdf") {
			return
		}
		c.String(http.StatusFound, "redirect-to-presigned-url")
	})

	local := httptest.NewRecorder()
	engine.ServeHTTP(local, httptest.NewRequest(http.MethodGet, "/download", nil))
	if local.Code != http.StatusOK {
		t.Fatalf("arquivo local: status %d, want 200 (corpo=%q)", local.Code, local.Body.String())
	}
	if local.Body.String() != string(content) {
		t.Fatalf("arquivo local: corpo %q, want %q", local.Body.String(), content)
	}

	remote := httptest.NewRecorder()
	engine.ServeHTTP(remote, httptest.NewRequest(http.MethodGet, "/download-remote", nil))
	if remote.Code != http.StatusFound || remote.Body.String() != "redirect-to-presigned-url" {
		t.Fatalf("URL remota: status %d corpo %q, want o redirect pré-assinado", remote.Code, remote.Body.String())
	}
}

// Um caminho que tenta escapar do STORAGE_DIR nunca é servido — antes havia
// quatro cópias do `filepath.Join` sem essa verificação.
func TestServeLocalUploadIfPresentRejectsTraversal(t *testing.T) {
	base := t.TempDir()
	t.Setenv("STORAGE_DIR", base)

	secret := filepath.Join(filepath.Dir(base), "segredo.txt")
	if err := os.WriteFile(secret, []byte("nao-pode-vazar"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	defer func() { _ = os.Remove(secret) }()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/download", func(c *gin.Context) {
		if serveLocalUploadIfPresent(c, "/uploads/../segredo.txt") {
			return
		}
		c.String(http.StatusFound, "redirect-to-presigned-url")
	})

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/download", nil))
	if w.Code != http.StatusFound || w.Body.String() == "nao-pode-vazar" {
		t.Fatalf("traversal serviu conteúdo proibido: status %d corpo %q", w.Code, w.Body.String())
	}
}
