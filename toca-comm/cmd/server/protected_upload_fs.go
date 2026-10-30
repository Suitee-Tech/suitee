package main

import (
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// publicUploadBuckets é a ALLOWLIST da superfície legada `/uploads`.
//
// N7 (reauditoria de 2026-09-20): esta superfície é pública e sem autenticação
// (`r.StaticFS("/uploads", ...)` na raiz da engine, alcançável pela borda), e o
// diretório é COMPARTILHADO — no compose local, `uploads_data` é montado por
// cinco serviços (auth, comm, core, deal, financial), todos com
// `STORAGE_DIR=/uploads`. A proteção era uma DENYLIST com um único bucket
// (comprovantes financeiros): tudo que fosse novo, ou que qualquer serviço
// gravasse ali, ficava publicamente legível por padrão.
//
// Invertida para allowlist: só os buckets que o produto trata como mídia
// pública legada são servidos; qualquer bucket novo nasce PRIVADO e precisa ser
// adicionado aqui de propósito. Cobre `avatars` (caminho anterior aos buckets
// `toca-*`) e os que o frontend resolve por `/uploads/...`
// (`shared/media.ts`, `app/lib/documents.ts`) e que a tela de login pública usa.
//
// Em produção isso é inerte de qualquer forma: o compose de prod não monta o
// volume (os serviços usam `STORAGE_PROVIDER=supabase`).
var publicUploadBuckets = map[string]struct{}{
	"avatars":              {},
	"toca-avatars":         {},
	"toca-logos":           {},
	"toca-property-images": {},
	"toca-documents":       {},
}

// protectedUploadFS mantém a superfície legada `/uploads` apenas para os buckets
// de mídia pública. Tudo o mais — comprovantes financeiros, anexos de mensagem
// e qualquer bucket futuro — é recusado aqui; os comprovantes são servidos pelo
// toca-financial depois de autenticar.
type protectedUploadFS struct {
	http.FileSystem
}

func newProtectedUploadFS(root string) http.FileSystem {
	return protectedUploadFS{FileSystem: gin.Dir(root, false)}
}

// legacyUploadBucket devolve o bucket que a superfície legada serviria e se ele
// está na allowlist. A normalização é a mesma que o `http.FileSystem` aplica
// depois, então `..`, barras invertidas e duplicadas não escapam.
func legacyUploadBucket(name string) (string, bool) {
	normalized := strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(name, "\\", "/")), "/")
	bucket := normalized
	if idx := strings.Index(normalized, "/"); idx >= 0 {
		bucket = normalized[:idx]
	}
	// Bucket vazio (a própria raiz `/uploads`) não é servido: sem isso um
	// diretório poderia listar o conteúdo de todos os buckets.
	if bucket == "" || bucket == "." {
		return "", false
	}
	_, allowed := publicUploadBuckets[bucket]
	return bucket, allowed
}

func (fs protectedUploadFS) Open(name string) (http.File, error) {
	if _, allowed := legacyUploadBucket(name); !allowed {
		return nil, os.ErrPermission
	}
	return fs.FileSystem.Open(name)
}
