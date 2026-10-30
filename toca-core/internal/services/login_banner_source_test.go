package services

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca-core/internal/domain/core/settings"
)

// Troca entre a imagem enviada e os presets do sistema.
//
// Antes deste contrato, `loginBannerUrl` preenchida vencia o preset e o único
// jeito de voltar para um preset era o botão "Voltar para as opções", que
// gravava `loginBannerUrl: ""` — a imagem enviada era perdida do registro e a
// imobiliária precisava subir o arquivo de novo para usá-la outra vez.
//
// Agora as duas imagens ficam gravadas e `loginBannerSource` diz qual está
// ativa. A regra de compatibilidade (`EffectiveLoginBannerSource`) mantém o
// comportamento dos registros gravados antes do campo existir.

func TestEffectiveLoginBannerSource(t *testing.T) {
	cases := []struct {
		name   string
		source string
		url    string
		want   string
	}{
		{"fonte custom explicita", "custom", "https://cdn.exemplo.com/a.png", "custom"},
		{"fonte preset explicita com imagem guardada", "preset", "https://cdn.exemplo.com/a.png", "preset"},
		{"fonte preset sem imagem", "preset", "", "preset"},
		// Compatibilidade: registro anterior ao campo, com imagem enviada.
		{"vazio com imagem cai em custom", "", "https://cdn.exemplo.com/a.png", "custom"},
		{"vazio sem imagem cai em preset", "", "", "preset"},
		{"espacos contam como vazio", "", "   ", "preset"},
		{"imagem com espacos em volta ainda conta", "", "  https://cdn.exemplo.com/a.png  ", "custom"},
		// Valor desconhecido não pode deixar o painel sem imagem.
		{"valor desconhecido usa a regra antiga", "qualquer", "https://cdn.exemplo.com/a.png", "custom"},
		{"valor desconhecido sem imagem", "qualquer", "", "preset"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EffectiveLoginBannerSource(tc.source, tc.url); got != tc.want {
				t.Fatalf("EffectiveLoginBannerSource(%q, %q) = %q, want %q", tc.source, tc.url, got, tc.want)
			}
		})
	}
}

// fakeAppearanceRepo é o repositório em memória da aparência: guarda o que foi
// gravado para os testes conferirem o registro final, não só a resposta.
type fakeAppearanceRepo struct {
	row      *settings.AppearanceSettings
	upserts  int
	upserted *settings.AppearanceSettings
}

func (f *fakeAppearanceRepo) GetByTenantID(context.Context, string) (*settings.AppearanceSettings, error) {
	if f.row == nil {
		return nil, nil
	}
	copied := *f.row
	return &copied, nil
}

func (f *fakeAppearanceRepo) Upsert(_ context.Context, s *settings.AppearanceSettings) error {
	copied := *s
	f.upserted = &copied
	f.row = &copied
	f.upserts++
	return nil
}

// Sem registro no banco, a aba precisa de uma fonte coerente: preset, não vazio.
func TestGetAppearance_DefaultsToPresetSource(t *testing.T) {
	svc := &SettingsService{appearanceRepo: &fakeAppearanceRepo{}}

	got, err := svc.GetAppearance(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got.LoginBannerSource != LoginBannerSourcePreset {
		t.Fatalf("fonte = %q, want %q", got.LoginBannerSource, LoginBannerSourcePreset)
	}
	if got.LoginBannerPreset != DefaultLoginBannerPreset {
		t.Fatalf("preset = %q, want %q", got.LoginBannerPreset, DefaultLoginBannerPreset)
	}
}

// Registro antigo (sem o campo) com imagem enviada: a leitura tem de devolver
// `custom`, senão a aba marcaria o preset errado e a tela de login manteria a
// imagem — as duas discordariam.
func TestGetAppearance_NormalizesLegacyRowWithUploadedImage(t *testing.T) {
	repo := &fakeAppearanceRepo{row: &settings.AppearanceSettings{
		TenantID:          "tenant-1",
		LoginBannerPreset: "trm",
		LoginBannerURL:    "https://cdn.exemplo.com/toca-logos/a.webp",
	}}
	svc := &SettingsService{appearanceRepo: repo}

	got, err := svc.GetAppearance(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got.LoginBannerSource != LoginBannerSourceCustom {
		t.Fatalf("fonte = %q, want %q", got.LoginBannerSource, LoginBannerSourceCustom)
	}
	// O preset escolhido é preservado: é ele que a troca rápida oferece.
	if got.LoginBannerPreset != "trm" {
		t.Fatalf("preset = %q, want trm (nao pode ser sobrescrito pelo padrao)", got.LoginBannerPreset)
	}
}

// Com a fonte em `preset` e a imagem guardada, o preset escolhido NÃO pode ser
// substituído pelo padrão — era o que a regra antiga (`bannerURL == ""`) faria.
func TestGetAppearance_KeepsPresetWhileImageIsStored(t *testing.T) {
	repo := &fakeAppearanceRepo{row: &settings.AppearanceSettings{
		TenantID:          "tenant-1",
		LoginBannerPreset: "",
		LoginBannerURL:    "https://cdn.exemplo.com/toca-logos/a.webp",
		LoginBannerSource: LoginBannerSourcePreset,
	}}
	svc := &SettingsService{appearanceRepo: repo}

	got, err := svc.GetAppearance(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got.LoginBannerSource != LoginBannerSourcePreset {
		t.Fatalf("fonte = %q, want %q", got.LoginBannerSource, LoginBannerSourcePreset)
	}
	if got.LoginBannerPreset != DefaultLoginBannerPreset {
		t.Fatalf("preset vazio deveria cair no padrao, veio %q", got.LoginBannerPreset)
	}
	// A imagem continua gravada: é o que permite voltar para ela.
	if got.LoginBannerURL == "" {
		t.Fatal("a imagem enviada nao pode ser apagada ao escolher um preset")
	}
}

// Enviar uma imagem a ativa (é o que o usuário acabou de pedir) e preserva o
// preset escolhido antes.
func TestUploadLoginBanner_ActivatesCustomAndKeepsPreset(t *testing.T) {
	repo := &fakeAppearanceRepo{row: &settings.AppearanceSettings{
		TenantID:          "tenant-1",
		LoginBannerPreset: "cliente",
		LoginBannerSource: LoginBannerSourcePreset,
	}}
	svc := &SettingsService{appearanceRepo: repo, storageService: &fakeStorageService{
		publicBase: "http://localhost:54321/storage/v1",
	}}

	url, err := svc.UploadLoginBanner(context.Background(), "tenant-1", "a.webp", "image/webp", bytes.NewReader([]byte("x")))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if url == "" {
		t.Fatal("upload deveria devolver a URL gravada")
	}
	if repo.upserted.LoginBannerSource != LoginBannerSourceCustom {
		t.Fatalf("fonte = %q, want %q", repo.upserted.LoginBannerSource, LoginBannerSourceCustom)
	}
	if repo.upserted.LoginBannerPreset != "cliente" {
		t.Fatalf("preset = %q, want cliente (guardado para a troca rapida)", repo.upserted.LoginBannerPreset)
	}
}

func TestValidateAppearanceFields_LoginBannerSource(t *testing.T) {
	clearAppearanceAssetEnv(t)
	svc := &SettingsService{}

	for _, valid := range []string{"", LoginBannerSourceCustom, LoginBannerSourcePreset} {
		if err := svc.validateAppearanceFields(&settings.AppearanceSettings{LoginBannerSource: valid}); err != nil {
			t.Fatalf("fonte %q deveria ser aceita: %v", valid, err)
		}
	}
	for _, invalid := range []string{"upload", "CUSTOM", "sim", "custom "} {
		err := svc.validateAppearanceFields(&settings.AppearanceSettings{LoginBannerSource: invalid})
		if invalid == "custom " { // espacos em volta sao normalizados antes da comparacao
			if err != nil {
				t.Fatalf("fonte %q deveria ser aceita (trim): %v", invalid, err)
			}
			continue
		}
		if !errors.Is(err, ErrInvalidAppearance) {
			t.Fatalf("fonte %q deveria ser recusada, err = %v", invalid, err)
		}
	}
}

// A URL guardada é validada mesmo com o preset ativo: ela volta a ser carregada
// numa página pública no primeiro clique da troca rápida.
func TestValidateAppearanceFields_ValidatesStoredImageEvenWhenInactive(t *testing.T) {
	clearAppearanceAssetEnv(t)
	svc := &SettingsService{}

	err := svc.validateAppearanceFields(&settings.AppearanceSettings{
		LoginBannerSource: LoginBannerSourcePreset,
		LoginBannerURL:    "https://evil.example.com/banner.png",
	})
	if !errors.Is(err, ErrInvalidAppearance) {
		t.Fatalf("imagem guardada de terceiro deveria ser recusada, err = %v", err)
	}
}
