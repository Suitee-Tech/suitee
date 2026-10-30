package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/storage"
)

// SECURITY (achado B8 da revisão de permissões de 2026-09-19): o toca-deal
// servia `r.Static("/uploads", "./uploads")` — o diretório de arquivos sem
// autenticação. O mount foi removido; o download continua funcionando pelos
// endpoints autenticados (documento, checklist do operador e do portal), que
// chegam aqui depois das guardas de permissão/escopo.
//
// serveLocalUploadIfPresent serve o arquivo quando a URL aponta para o storage
// local e o arquivo existe, devolvendo true. Quando devolve false o chamador
// segue com o redirect para a URL pré-assinada (MinIO/S3/Supabase) — o
// comportamento anterior era exatamente esse, agora com a resolução do caminho
// em storage.LocalUploadPath (confinada ao STORAGE_DIR e sem path traversal).
func serveLocalUploadIfPresent(c *gin.Context, fileURL string) bool {
	localPath, ok := storage.LocalUploadFile(fileURL)
	if !ok {
		return false
	}
	c.File(localPath)
	return true
}
