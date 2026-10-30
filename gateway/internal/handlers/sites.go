package handlers

import (
	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	"github.com/oginoino/toca/gateway/internal/services/proxy"

	"github.com/gin-gonic/gin"
)

// RegisterSites expõe o toca-sites na borda autenticada (`/api/v1/sites*`).
//
// O site público NÃO passa por aqui: `{marca}.imoveis.suitee.com.br` é roteado
// pelo Traefik direto ao toca-web (Next), e o Next chama o toca-sites pela rede
// interna. As rotas públicas de view/lead existem atrás deste gateway apenas
// para o caso de o frontend público chamar a API diretamente.
func RegisterSites(r *gin.Engine, cfg gwconfig.Config, pxy *proxy.Service) {
	r.Any("/api/v1/sites", pxy.ReverseProxy(cfg.SitesServiceURL))
	r.Any("/api/v1/sites/*path", pxy.ReverseProxy(cfg.SitesServiceURL))
}
