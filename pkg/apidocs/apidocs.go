// Package apidocs concentra o gate da documentação interativa (Swagger UI).
//
// SECURITY (achado M16 da revisão de permissões de 2026-09-19): o Swagger estava
// público em auth, crm, comm, portal, signature e core. Em alguns serviços ele
// era registrado antes do `Use(...)` do grupo, então a autenticação só valia
// "por acaso da ordem de registro" — e no toca-portal a rota nem estava sob o
// grupo autenticado. O mapa completo da API (paths, DTOs, nomes de campos) é
// insumo de atacante; em release ele não deve existir.
//
// Regra única: fora de release a documentação é montada; em release
// (`APP_MODE=release` ou `APP_ENV=production|prod`) a rota NÃO é registrada —
// nem para usuário autenticado. Por isso `Register` centraliza a decisão: um
// serviço novo não tem como "esquecer" o gate, e o CI
// (scripts/check-route-guards) exige que todo `ginSwagger.WrapHandler` esteja
// dentro de uma chamada a este pacote.
package apidocs

import (
	"os"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/logger"
)

// Enabled informa se a documentação interativa pode ser montada neste processo.
func Enabled() bool {
	return !config.IsReleaseEnv(os.Getenv("APP_ENV"), os.Getenv("APP_MODE"))
}

// Register monta o handler de documentação no grupo informado quando o ambiente
// permite. Devolve true quando a rota foi registrada.
func Register(group *gin.RouterGroup, path string, handlers ...gin.HandlerFunc) bool {
	if !Enabled() {
		logger.Info("apidocs.disabled",
			"path", path,
			"reason", "ambiente de release: Swagger UI não é montado")
		return false
	}
	group.GET(path, handlers...)
	return true
}
