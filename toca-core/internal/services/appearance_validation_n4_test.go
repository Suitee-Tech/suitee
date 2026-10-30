package services

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/oginoino/toca-core/internal/domain/core/settings"
	"github.com/oginoino/toca/pkg/storage"
)

// N4 (reauditoria de 2026-09-20): `loginBannerUrl` e `customDomain` eram
// gravados sem validação. A tela de login é PÚBLICA e carrega a imagem para
// todos os usuários do tenant — URL arbitrária é requisição a terceiros numa
// página não autenticada, e `customDomain` sem forma de domínio não é domínio.

// clearAppearanceAssetEnv zera TODAS as envs que alimentam a allowlist de hosts.
// Se uma env nova entrar em `appearanceAssetHosts`, ela precisa entrar aqui —
// caso contrário o caso "falha fechado" passa a aceitar host herdado do ambiente
// de quem roda o teste.
func clearAppearanceAssetEnv(t *testing.T) {
	t.Helper()
	for _, envKey := range []string{
		"PUBLIC_UPLOADS_URL", "PUBLIC_GATEWAY_URL", "PUBLIC_API_URL",
		"SUPABASE_PUBLIC_URL", "SUPABASE_URL", "S3_PUBLIC_ENDPOINT",
	} {
		t.Setenv(envKey, "")
	}
}

// fakeStorageService devolve a URL pública exatamente como o provedor real a
// monta — é a fonte primária da allowlist e o que `Upload` devolve.
type fakeStorageService struct{ publicBase string }

func (f *fakeStorageService) Upload(_ context.Context, bucket, filename, _ string, _ io.Reader) (string, error) {
	return f.GetURL(bucket, filename), nil
}
func (f *fakeStorageService) Delete(context.Context, string, string) error { return nil }
func (f *fakeStorageService) GetURL(bucket, filename string) string {
	return f.publicBase + "/" + bucket + "/" + filename
}
func (f *fakeStorageService) GetPresignedURL(context.Context, string, string, time.Duration) (string, error) {
	return "", nil
}
func (f *fakeStorageService) GetPresignedPutURL(context.Context, string, string, time.Duration) (string, error) {
	return "", nil
}
func (f *fakeStorageService) IsPrivate(bucket string) bool { return false }

var _ storage.Service = (*fakeStorageService)(nil)

func TestValidateAppearanceFields_CustomDomain(t *testing.T) {
	svc := &SettingsService{}
	cases := map[string]bool{
		"":                               true,
		"crm.suaimobiliaria.com.br":      true,
		"crm.suaimobiliaria.com.br:8443": true,
		"localhost:3000":                 true,
		"http://evil.example.com":        false, // esquema não é domínio
		"evil.example.com/caminho":       false, // caminho não é domínio
		"javascript:alert(1)":            false,
		"com espaço.com":                 false,
		"-comeca-com-hifen.com":          false,
	}
	for domain, wantOK := range cases {
		t.Run(domain, func(t *testing.T) {
			err := svc.validateAppearanceFields(&settings.AppearanceSettings{CustomDomain: domain})
			if wantOK && err != nil {
				t.Fatalf("domínio %q deveria ser aceito: %v", domain, err)
			}
			if !wantOK && !errors.Is(err, ErrInvalidAppearance) {
				t.Fatalf("domínio %q deveria ser recusado, err = %v", domain, err)
			}
		})
	}
}

func TestValidateAppearanceFields_LoginBannerURL(t *testing.T) {
	// Host da plataforma: o mesmo que pkg/storage usa para montar a URL pública.
	t.Setenv("PUBLIC_UPLOADS_URL", "http://localhost:8080/uploads")
	t.Setenv("SUPABASE_URL", "http://localhost:54321")

	svc := &SettingsService{}
	cases := map[string]bool{
		"":                        true,
		"/toca-logos/banner.webp": true, // caminho relativo (mesma origem)
		"http://localhost:8080/toca-logos/a.webp":                           true, // host da plataforma
		"http://localhost:54321/storage/v1/object/public/toca-logos/a.webp": true,
		// Protocol-relative: resolvido contra a origem, mas aponta para fora.
		"//evil.example.com/banner.png":       false,
		"https://evil.example.com/banner.png": false,
		"javascript:alert(1)":                 false,
		"data:image/png;base64,AAAA":          false,
		"file:///etc/passwd":                  false,
	}
	for raw, wantOK := range cases {
		t.Run(raw, func(t *testing.T) {
			err := svc.validateAppearanceFields(&settings.AppearanceSettings{LoginBannerURL: raw})
			if wantOK && err != nil {
				t.Fatalf("URL %q deveria ser aceita: %v", raw, err)
			}
			if !wantOK && !errors.Is(err, ErrInvalidAppearance) {
				t.Fatalf("URL %q deveria ser recusada, err = %v", raw, err)
			}
		})
	}
}

// Sem host de plataforma configurado, URL absoluta falha FECHADO — só caminho
// relativo passa.
func TestValidateAppearanceFields_FailsClosedWithoutPlatformHost(t *testing.T) {
	clearAppearanceAssetEnv(t)

	svc := &SettingsService{}
	if err := svc.validateAppearanceFields(&settings.AppearanceSettings{LoginBannerURL: "/toca-logos/a.webp"}); err != nil {
		t.Fatalf("caminho relativo deveria passar: %v", err)
	}
	if err := svc.validateAppearanceFields(&settings.AppearanceSettings{LoginBannerURL: "https://qualquer.example.com/a.webp"}); !errors.Is(err, ErrInvalidAppearance) {
		t.Fatalf("URL absoluta sem host configurado deveria falhar fechado, err = %v", err)
	}
}

// REGRESSÃO do bug real (400 ao salvar a aparência depois do upload do banner).
//
// Com `STORAGE_PROVIDER=supabase` a URL do banner é gravada com o host PÚBLICO
// (aquele que o NAVEGADOR resolve: `localhost:54321` ou `SUPABASE_PUBLIC_URL`),
// enquanto `SUPABASE_URL` — que era a única env consultada — aponta para o
// host INTERNO do container (`host.docker.internal:54321`). A validação
// recusava a URL que o próprio upload acabara de produzir.
//
// A allowlist agora deriva de `storage.Service.GetURL`, a MESMA função que
// gravou a URL. Com o código antigo (só envs) este teste falha; com o novo,
// passa.
func TestValidateAppearanceFields_AcceptsHostFromStorageService(t *testing.T) {
	clearAppearanceAssetEnv(t)
	// Divisão interna/pública exata do stack local via Docker.
	t.Setenv("SUPABASE_URL", "http://host.docker.internal:54321")

	svc := &SettingsService{storageService: &fakeStorageService{
		publicBase: "http://localhost:54321/storage/v1",
	}}

	uploaded := svc.storageService.GetURL("toca-logos", "tenant-1/abc.webp")
	if err := svc.validateAppearanceFields(&settings.AppearanceSettings{LoginBannerURL: uploaded}); err != nil {
		t.Fatalf("a URL produzida pelo próprio upload deveria ser aceita (%s): %v", uploaded, err)
	}

	// O host interno continua aceito (a env segue na allowlist como reserva)...
	if err := svc.validateAppearanceFields(&settings.AppearanceSettings{
		LoginBannerURL: "http://host.docker.internal:54321/storage/v1/object/public/toca-logos/a.webp",
	}); err != nil {
		t.Fatalf("host interno configurado deveria seguir aceito: %v", err)
	}
	// ...mas terceiros continuam recusados.
	if err := svc.validateAppearanceFields(&settings.AppearanceSettings{
		LoginBannerURL: "https://evil.example.com/banner.png",
	}); !errors.Is(err, ErrInvalidAppearance) {
		t.Fatalf("host de terceiro deveria continuar recusado, err = %v", err)
	}
}

// Mesmo sem nenhuma env, o host do provedor injetado basta: é a fonte primária.
func TestValidateAppearanceFields_StorageServiceAloneIsEnough(t *testing.T) {
	clearAppearanceAssetEnv(t)

	svc := &SettingsService{storageService: &fakeStorageService{publicBase: "https://cdn.suaimobiliaria.com.br"}}
	if err := svc.validateAppearanceFields(&settings.AppearanceSettings{
		LoginBannerURL: "https://cdn.suaimobiliaria.com.br/toca-logos/a.webp",
	}); err != nil {
		t.Fatalf("host do provedor injetado deveria ser aceito: %v", err)
	}
}

func TestValidateAppearanceFields_FontAndIconAllowlists(t *testing.T) {
	svc := &SettingsService{}
	for _, font := range []string{"Inter", "DM Sans", "Poppins", "Roboto", "Nunito", "Open Sans"} {
		if err := svc.validateAppearanceFields(&settings.AppearanceSettings{FontFamily: font}); err != nil {
			t.Fatalf("fonte %q deveria ser aceita: %v", font, err)
		}
	}
	for _, style := range []string{"regular", "light", "bold", "sharp"} {
		if err := svc.validateAppearanceFields(&settings.AppearanceSettings{IconStyle: style}); err != nil {
			t.Fatalf("estilo %q deveria ser aceito: %v", style, err)
		}
	}
	if err := svc.validateAppearanceFields(&settings.AppearanceSettings{FontFamily: "Comic Sans"}); !errors.Is(err, ErrInvalidAppearance) {
		t.Fatalf("fonte desconhecida deveria ser recusada, veio %v", err)
	}
	if err := svc.validateAppearanceFields(&settings.AppearanceSettings{IconStyle: "neon"}); !errors.Is(err, ErrInvalidAppearance) {
		t.Fatalf("estilo desconhecido deveria ser recusado, veio %v", err)
	}
}

// A FIACÃO também é contrato: `UpdateAppearance` precisa recusar ANTES de tocar o
// repositório. Sem este caso, remover a chamada da validação não quebraria nenhum
// teste — foi exatamente o que a mutação revelou.
func TestUpdateAppearance_RejectsInvalidFieldsBeforeRepository(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("validação não rodou antes do repositório (panic: %v)", r)
		}
	}()

	// Serviço sem repositório: qualquer caminho que chegue ao I/O estoura.
	svc := &SettingsService{}

	cases := map[string]*settings.AppearanceSettings{
		"customDomain com esquema":         {CustomDomain: "http://evil.example.com"},
		"customDomain com caminho":         {CustomDomain: "evil.example.com/caminho"},
		"loginBannerUrl de terceiro":       {LoginBannerURL: "https://evil.example.com/banner.png"},
		"loginBannerUrl protocol-relativa": {LoginBannerURL: "//evil.example.com/banner.png"},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			err := svc.UpdateAppearance(context.Background(), "tenant-1", req)
			if !errors.Is(err, ErrInvalidAppearance) {
				t.Fatalf("esperava ErrInvalidAppearance antes do repositório, veio %v", err)
			}
		})
	}
}
