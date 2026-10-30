package storage

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ExtractUploadObjectKey converts a legacy /uploads/{bucket}/{key} reference
// back to the object key used by the configured storage provider. It accepts
// both relative paths and absolute URLs so records created by older local or
// production deployments can be migrated at read time.
func ExtractUploadObjectKey(bucket, rawURL string) (string, bool) {
	bucket = strings.Trim(bucket, "/")
	value := strings.TrimSpace(rawURL)
	if bucket == "" || value == "" {
		return "", false
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", false
	}

	prefix := "/uploads/" + bucket + "/"
	if !strings.HasPrefix(parsed.Path, prefix) {
		return "", false
	}

	key, err := url.PathUnescape(strings.TrimPrefix(parsed.Path, prefix))
	if err != nil {
		return "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false
	}
	return key, true
}

// LocalUploadPath converte uma referência pública `/uploads/<caminho>` no
// arquivo correspondente dentro do storage local (`STORAGE_DIR`, default
// `/uploads`).
//
// SECURITY (achado B8 da revisão de permissões de 2026-09-19): o toca-deal
// servia o diretório de uploads inteiro sem autenticação
// (`r.Static("/uploads", "./uploads")`), expondo contratos, documentos de
// identidade e comprovantes a quem soubesse a URL. O mount foi removido; os
// arquivos continuam acessíveis pelos endpoints autenticados, que resolvem o
// caminho local por esta função — a única implementação da regra (antes havia
// quatro cópias espalhadas entre handlers e serviços).
//
// A resolução é confinada ao diretório base: uma referência com `..` (ou com
// caminho absoluto) é recusada em vez de escapar do storage.
func LocalUploadPath(fileURL string) (string, bool) {
	value := strings.TrimSpace(fileURL)
	if value == "" {
		return "", false
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", false
	}
	path := parsed.Path
	if path == "" {
		path = value
	}

	const marker = "/uploads/"
	idx := strings.Index(path, marker)
	if idx < 0 {
		return "", false
	}
	relative := strings.TrimPrefix(path[idx+len(marker):], "/")
	if relative == "" {
		return "", false
	}

	base := strings.TrimSpace(os.Getenv("STORAGE_DIR"))
	if base == "" {
		base = "/uploads"
	}
	localPath := filepath.Join(base, filepath.FromSlash(relative))

	// Confinamento: o caminho resolvido precisa continuar dentro da base.
	cleanBase := filepath.Clean(base)
	cleanPath := filepath.Clean(localPath)
	if cleanPath != cleanBase && !strings.HasPrefix(cleanPath, cleanBase+string(os.PathSeparator)) {
		return "", false
	}
	return cleanPath, true
}

// LocalUploadFile devolve o caminho de um arquivo existente do storage local,
// ou ok=false quando a referência não é local ou o arquivo não existe.
func LocalUploadFile(fileURL string) (string, bool) {
	localPath, ok := LocalUploadPath(fileURL)
	if !ok {
		return "", false
	}
	info, err := os.Stat(localPath)
	if err != nil || info.IsDir() {
		return "", false
	}
	return localPath, true
}
