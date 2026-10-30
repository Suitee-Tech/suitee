package handlers

import (
	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	"github.com/oginoino/toca/gateway/internal/services/proxy"

	"github.com/gin-gonic/gin"
)

func RegisterCalendar(r *gin.Engine, cfg gwconfig.Config, pxy *proxy.Service) {
	r.Any("/api/v1/appointments", pxy.ReverseProxy(cfg.CalendarServiceURL))
	r.Any("/api/v1/appointments/*path", pxy.ReverseProxy(cfg.CalendarServiceURL))
	// Download assinado do .ics enviado por e-mail: a autorização é o token da
	// própria URL, então a borda só encaminha (o serviço valida tenant e prazo).
	r.GET("/api/v1/calendar/public/appointments/:id/calendar.ics", pxy.ReverseProxy(cfg.CalendarServiceURL))
}
