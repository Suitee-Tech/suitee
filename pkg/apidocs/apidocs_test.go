package apidocs

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// M16 — o Swagger não pode existir em release. O teste usa o router real do
// pacote (mesma função que os serviços chamam).
func TestRegisterIsGatedByEnvironment(t *testing.T) {
	tests := []struct {
		name    string
		appEnv  string
		appMode string
		want    int
	}{
		{name: "produção", appEnv: "production", appMode: "release", want: http.StatusNotFound},
		{name: "prod", appEnv: "prod", appMode: "", want: http.StatusNotFound},
		{name: "release explícito", appEnv: "beta", appMode: "release", want: http.StatusNotFound},
		{name: "desenvolvimento", appEnv: "development", appMode: "debug", want: http.StatusOK},
		{name: "teste", appEnv: "test", appMode: "", want: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_ENV", tt.appEnv)
			t.Setenv("APP_MODE", tt.appMode)

			gin.SetMode(gin.TestMode)
			engine := gin.New()
			v1 := engine.Group("/api/v1")
			registered := Register(v1, "/swagger/*any", func(c *gin.Context) {
				c.String(http.StatusOK, "swagger-ui")
			})

			if wantRegistered := tt.want == http.StatusOK; registered != wantRegistered {
				t.Fatalf("Register devolveu %v, want %v", registered, wantRegistered)
			}

			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/swagger/index.html", nil))
			if recorder.Code != tt.want {
				t.Fatalf("GET /api/v1/swagger/index.html = %d, want %d", recorder.Code, tt.want)
			}
		})
	}
}

func TestEnabledMirrorsReleaseEnvironments(t *testing.T) {
	for _, env := range []string{"production", "prod"} {
		t.Setenv("APP_ENV", env)
		t.Setenv("APP_MODE", "")
		if Enabled() {
			t.Fatalf("APP_ENV=%s não pode habilitar a documentação", env)
		}
	}
	t.Setenv("APP_ENV", "development")
	t.Setenv("APP_MODE", "release")
	if Enabled() {
		t.Fatal("APP_MODE=release não pode habilitar a documentação")
	}
	t.Setenv("APP_MODE", "debug")
	if !Enabled() {
		t.Fatal("ambiente de desenvolvimento deveria manter a documentação")
	}
}
