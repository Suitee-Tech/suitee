package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/oginoino/toca/toca-sites/internal/domain/site"
)

// SiteService é o caso de uso do registro da vitrine e das páginas
// institucionais. Ele monta `public_base_url` — o frontend NUNCA concatena o
// host comercial — e mantém o tenant Payload em sincronia.
type SiteService struct {
	sites      site.SiteRepository
	listings   site.ListingRepository
	engagement site.EngagementRepository
	projection site.Projection
	reconciler TenantReconciler

	hostSuffix string
	scheme     string
	publicPort string
	now        func() time.Time
}

func NewSiteService(
	sites site.SiteRepository,
	listings site.ListingRepository,
	engagement site.EngagementRepository,
	projection site.Projection,
	hostSuffix, scheme string,
) *SiteService {
	return &SiteService{
		sites:      sites,
		listings:   listings,
		engagement: engagement,
		projection: projection,
		hostSuffix: strings.Trim(strings.TrimSpace(hostSuffix), "."),
		scheme:     scheme,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// TenantReconciler projeta o estoque existente quando a vitrine é ligada.
// Interface estreita para o SiteService não depender do SyncService inteiro.
type TenantReconciler interface {
	ReconcileTenant(ctx context.Context, tenantID string) error
}

// WithPublicPort informa a porta do Next no host público (apenas local; em
// produção é vazia e o scheme/https resolve a 443).
func (s *SiteService) WithPublicPort(port string) *SiteService {
	s.publicPort = strings.TrimSpace(port)
	return s
}

// WithReconciler injeta o backfill. Sem ele (testes/dev) o save continua
// funcionando; só o primeiro enable não publica o estoque pré-existente.
func (s *SiteService) WithReconciler(r TenantReconciler) *SiteService {
	s.reconciler = r
	return s
}

// SaveSiteInput é o corpo do PUT /api/v1/sites/me.
type SaveSiteInput struct {
	Name     string        `json:"name"`
	Slug     string        `json:"slug"`
	Enabled  *bool         `json:"enabled,omitempty"`
	Branding site.Branding `json:"branding"`
	Contact  site.Contact  `json:"contact"`
	Pages    []site.Page   `json:"pages,omitempty"`
}

// SiteOverview é a resposta do GET /api/v1/sites/me: registro, páginas e
// agregados em uma chamada só (o painel não precisa orquestrar 3 endpoints).
type SiteOverview struct {
	Site  *site.Site      `json:"site"`
	Pages []site.Page     `json:"pages"`
	Stats *site.SiteStats `json:"stats"`
	// HostSuffix/PublicScheme permitem ao painel mostrar o preview
	// `{marca}.{sufixo}` ANTES do primeiro save. A URL canônica continua
	// vindo pronta em site.public_base_url.
	HostSuffix   string `json:"host_suffix"`
	PublicScheme string `json:"public_scheme"`
	// PublicPort é vazio em produção (https implícito) e 3002 no local.
	PublicPort string `json:"public_port,omitempty"`
}

// canonicalPublicURL é a URL montada com o sufixo ATUAL do ambiente. O valor
// persistido pode ter sido gravado antes de uma troca de domínio (beta →
// produção); o painel e a readiness nunca devem usar um host antigo.
func (s *SiteService) canonicalPublicURL(siteRec *site.Site) string {
	if siteRec == nil || strings.TrimSpace(siteRec.Slug) == "" {
		return ""
	}
	return site.BuildPublicBaseURLWithPort(s.scheme, s.hostSuffix, siteRec.Slug, s.publicPort)
}

// GetOverview devolve o estado atual da vitrine. Tenant sem site recebe um
// registro desligado (200, não 404): o painel precisa renderizar o formulário
// de ativação.
func (s *SiteService) GetOverview(ctx context.Context, tenantID string) (*SiteOverview, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, site.ErrNotFound
	}
	siteRec, err := s.sites.GetByTenant(ctx, tenantID)
	if err != nil {
		if !errors.Is(err, site.ErrNotFound) {
			return nil, err
		}
		siteRec = &site.Site{TenantID: tenantID}
	}

	if canonical := s.canonicalPublicURL(siteRec); canonical != "" && siteRec.PublicBaseURL != canonical {
		siteRec.PublicBaseURL = canonical
		if siteRec.ID != "" {
			if err := s.sites.Update(ctx, siteRec); err != nil {
				log.Printf("[toca-sites] falha ao atualizar URL pública do site %s: %v", siteRec.ID, err)
			}
		}
	}

	pages := defaultPages(siteRec.Name)
	if siteRec.ID != "" {
		loaded, err := s.sites.ListPages(ctx, tenantID, siteRec.ID)
		if err != nil {
			return nil, err
		}
		pages = mergePages(pages, loaded)
	}

	stats := &site.SiteStats{}
	if siteRec.ID != "" {
		stats, err = s.engagement.StatsForSite(ctx, tenantID, s.now())
		if err != nil {
			return nil, err
		}
	}

	return &SiteOverview{
		Site: siteRec, Pages: pages, Stats: stats,
		HostSuffix: s.hostSuffix, PublicScheme: s.scheme, PublicPort: s.publicPort,
	}, nil
}

// SiteReadiness é o estado exibido no painel antes de liberar o link público.
// `ready` só é true quando o host responde 200 com certificado TLS validado
// pelo cliente HTTP do servidor; enquanto isso o painel não abre a vitrine.
type SiteReadiness struct {
	Ready     bool      `json:"ready"`
	Status    string    `json:"status"`
	PublicURL string    `json:"public_url,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// Readiness consulta o próprio host público. Não usa um segredo nem dados do
// tenant: a URL é a canônica persistida e a validação é feita pelo cliente HTTP
// padrão (cadeia TLS, hostname e status). Erros de rede/certificado mantêm o
// painel em "publicando", em vez de expor um link que o Chrome bloquearia.
func (s *SiteService) Readiness(ctx context.Context, tenantID string) (*SiteReadiness, error) {
	out := &SiteReadiness{CheckedAt: time.Now().UTC()}
	siteRec, err := s.sites.GetByTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, site.ErrNotFound) {
			out.Status = "not_configured"
			return out, nil
		}
		return nil, err
	}
	if canonical := s.canonicalPublicURL(siteRec); canonical != "" {
		out.PublicURL = canonical
	} else {
		out.PublicURL = siteRec.PublicBaseURL
	}
	if !siteRec.Enabled {
		out.Status = "disabled"
		return out, nil
	}
	if out.PublicURL == "" {
		out.Status = "not_configured"
		return out, nil
	}

	parsed, err := url.Parse(out.PublicURL)
	if err != nil || parsed.Hostname() == "" {
		out.Status = "awaiting_certificate"
		return out, nil
	}
	// Em desenvolvimento local a vitrine é HTTP; o link funciona sem ACME.
	if parsed.Scheme != "https" {
		out.Ready = true
		out.Status = "ready"
		return out, nil
	}
	if suffix := strings.Trim(strings.ToLower(s.hostSuffix), "."); suffix != "" {
		host := strings.ToLower(parsed.Hostname())
		if host != suffix && !strings.HasSuffix(host, "."+suffix) {
			out.Status = "awaiting_certificate"
			return out, nil
		}
	}

	reqCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, out.PublicURL, nil)
	if err != nil {
		out.Status = "awaiting_publication"
		return out, nil
	}
	resp, err := (&http.Client{Timeout: 6 * time.Second}).Do(req)
	if err != nil {
		out.Status = "awaiting_certificate"
		return out, nil
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
	if resp.StatusCode == http.StatusOK {
		out.Ready = true
		out.Status = "ready"
		return out, nil
	}
	out.Status = "awaiting_publication"
	return out, nil
}

// Save cria/atualiza a vitrine e — quando ligada — provisiona o tenant no
// Payload. O slug é único GLOBAL porque o host é o discriminador do tenant.
func (s *SiteService) Save(ctx context.Context, tenantID string, in SaveSiteInput) (*SiteOverview, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, site.ErrNotFound
	}
	if _, err := uuid.Parse(tenantID); err != nil {
		return nil, invalid("tenant_id", "tenant inválido")
	}

	existing, err := s.sites.GetByTenant(ctx, tenantID)
	if err != nil && !errors.Is(err, site.ErrNotFound) {
		return nil, err
	}
	creating := existing == nil

	name := strings.TrimSpace(in.Name)
	if name == "" {
		if creating {
			return nil, invalid("name", "informe o nome da imobiliária")
		}
		name = existing.Name
	}

	slug, err := s.resolveSlug(ctx, tenantID, name, strings.TrimSpace(in.Slug), existing)
	if err != nil {
		return nil, err
	}

	wasEnabled := existing != nil && existing.Enabled
	enabled := false
	if in.Enabled != nil {
		enabled = *in.Enabled
	} else if !creating {
		enabled = existing.Enabled
	}

	record := &site.Site{
		TenantID: tenantID,
		Slug:     slug,
		Name:     name,
		Enabled:  enabled,
		Branding: in.Branding,
		Contact:  in.Contact,
	}
	if !creating {
		record.ID = existing.ID
		record.CreatedAt = existing.CreatedAt
		record.PayloadTenantID = existing.PayloadTenantID
		// Branding/contato omitidos no PUT não devem apagar o que já existe.
		if in.Branding == (site.Branding{}) {
			record.Branding = existing.Branding
		}
		if in.Contact == (site.Contact{}) {
			record.Contact = existing.Contact
		}
	}
	branding, err := normalizeBranding(record.Branding, existing)
	if err != nil {
		return nil, err
	}
	record.Branding = branding

	record.PublicBaseURL = site.BuildPublicBaseURLWithPort(s.scheme, s.hostSuffix, slug, s.publicPort)

	// Provisiona o Payload ANTES de persistir o enable: se o CMS estiver
	// indisponível, o painel recebe o erro e não fica com um site "ligado"
	// que não renderiza.
	//
	// DESLIGAR também passa pelo CMS: o Next resolve o tenant lá, então só
	// gravar enabled=false no Postgres deixaria a vitrine no ar. Só pulamos
	// quando o site nunca foi provisionado (nunca teve payload_tenant_id).
	shouldProvision := s.projection != nil &&
		(enabled || (existing != nil && existing.PayloadTenantID != ""))
	if shouldProvision {
		payloadTenantID, err := s.projection.EnsureTenant(ctx, record)
		if err != nil {
			return nil, wrap("provisionar tenant no Payload", err)
		}
		record.PayloadTenantID = payloadTenantID
	}

	if creating {
		if err := s.sites.Create(ctx, record); err != nil {
			return nil, err
		}
	} else {
		if err := s.sites.Update(ctx, record); err != nil {
			return nil, err
		}
	}
	if record.PayloadTenantID != "" {
		if err := s.sites.SetPayloadTenantID(ctx, tenantID, record.ID, record.PayloadTenantID); err != nil {
			return nil, err
		}
	}

	if len(in.Pages) > 0 {
		if err := s.savePages(ctx, tenantID, record.ID, in.Pages); err != nil {
			return nil, err
		}
	}
	// Replica TODAS as páginas persistidas, não só as do payload: ligar o site
	// depois de já ter conteúdo local não pode deixar o CMS vazio.
	if stored, err := s.sites.ListPages(ctx, tenantID, record.ID); err == nil && len(stored) > 0 {
		if err := s.publishPages(ctx, record, stored); err != nil {
			return nil, err
		}
	}
	s.revalidate(ctx, slug, "/", "/sobre", "/contato", "/imoveis")

	// Primeiro enable: o estoque já cadastrado não gerou evento enquanto o
	// site estava desligado. Backfill em background para o painel não esperar
	// centenas de upserts no Payload.
	if enabled && !wasEnabled && s.reconciler != nil {
		go func() {
			backfillCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			if err := s.reconciler.ReconcileTenant(backfillCtx, tenantID); err != nil {
				log.Printf("[toca-sites] backfill do site falhou tenant=%s: %v", tenantID, err)
			}
		}()
	}

	return s.GetOverview(ctx, tenantID)
}

// Reconcile dispara em background a reprojeção do estoque no Payload. É o
// botão "Sincronizar vitrine" do painel: útil quando um evento foi perdido ou
// quando o CMS foi restaurado. Idempotente (upsert por property_id).
func (s *SiteService) Reconcile(ctx context.Context, tenantID string) error {
	if strings.TrimSpace(tenantID) == "" {
		return site.ErrNotFound
	}
	siteRec, err := s.sites.GetByTenant(ctx, tenantID)
	if err != nil {
		return err
	}
	if !siteRec.Enabled {
		return invalid("enabled", "ligue o site antes de sincronizar a vitrine")
	}
	if s.reconciler == nil {
		return invalid("projection", "projeção do site indisponível neste ambiente")
	}
	go func() {
		backfillCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		if err := s.reconciler.ReconcileTenant(backfillCtx, tenantID); err != nil {
			log.Printf("[toca-sites] sincronização manual falhou tenant=%s: %v", tenantID, err)
		}
	}()
	return nil
}

// PortalSite devolve o mínimo para o portal do cliente exibir a imobiliária
// do deal: nome, URL da home e tokens de marca. Nunca expõe leads/métricas.
type PortalSite struct {
	Name         string        `json:"name"`
	Slug         string        `json:"slug"`
	PublicURL    string        `json:"public_url"`
	Branding     site.Branding `json:"branding"`
	ContactPhone string        `json:"contact_phone,omitempty"`
}

func (s *SiteService) PortalSite(ctx context.Context, tenantID string) (*PortalSite, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, site.ErrNotFound
	}
	siteRec, err := s.sites.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if !siteRec.Enabled || siteRec.Slug == "" {
		return nil, site.ErrNotFound
	}
	base := s.canonicalPublicURL(siteRec)
	if base == "" {
		base = siteRec.PublicBaseURL
	}
	return &PortalSite{
		Name:         siteRec.Name,
		Slug:         siteRec.Slug,
		PublicURL:    base,
		Branding:     siteRec.Branding,
		ContactPhone: siteRec.Contact.WhatsApp,
	}, nil
}

// SavePage atualiza uma página específica (PUT /api/v1/sites/pages/:key).
func (s *SiteService) SavePage(ctx context.Context, tenantID, key string, p site.Page) (*site.Page, error) {
	siteRec, err := s.sites.GetByTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, site.ErrNotFound) {
			return nil, site.ErrNotFound
		}
		return nil, err
	}
	pageKey := site.PageKey(key)
	if !site.ValidPageKey(pageKey) {
		return nil, invalid("key", "página inválida (use home, about ou contact)")
	}
	p.Key = pageKey
	if err := s.savePages(ctx, tenantID, siteRec.ID, []site.Page{p}); err != nil {
		return nil, err
	}
	if err := s.publishPages(ctx, siteRec, []site.Page{p}); err != nil {
		return nil, err
	}
	out, err := s.sites.GetPage(ctx, tenantID, siteRec.ID, pageKey)
	if err != nil {
		return nil, err
	}
	s.revalidate(ctx, siteRec.Slug, pagePath(pageKey))
	return out, nil
}

// publishPages espelha as páginas no Payload. Site desligado não provisiona
// tenant — nesse caso a edição fica local e sobe no próximo Save com enabled.
func (s *SiteService) publishPages(ctx context.Context, siteRec *site.Site, pages []site.Page) error {
	if s.projection == nil || siteRec == nil || !siteRec.Enabled {
		return nil
	}
	for _, p := range pages {
		if err := s.projection.UpsertPage(ctx, siteRec, p); err != nil {
			return wrap("publicar página no Payload", err)
		}
	}
	return nil
}

func (s *SiteService) savePages(ctx context.Context, tenantID, siteID string, pages []site.Page) error {
	clean := make([]site.Page, 0, len(pages))
	for _, p := range pages {
		if !site.ValidPageKey(p.Key) {
			return invalid("pages.key", fmt.Sprintf("página inválida: %q", p.Key))
		}
		// Limites espelham a collection Pages do Payload (maxLength): validar
		// aqui dá 400 no painel em vez de 500 vindo do CMS.
		if len([]rune(p.Title)) > 140 {
			return invalid("pages.title", "título não pode passar de 140 caracteres")
		}
		if len([]rune(p.Subtitle)) > 320 {
			return invalid("pages.subtitle", "subtítulo não pode passar de 320 caracteres")
		}
		clean = append(clean, p)
	}
	return s.sites.UpsertPages(ctx, tenantID, siteID, clean)
}

var hexColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func normalizeBranding(in site.Branding, existing *site.Site) (site.Branding, error) {
	out := in
	out.LogoURL = strings.TrimSpace(out.LogoURL)
	out.CoverURL = strings.TrimSpace(out.CoverURL)
	out.Tagline = strings.TrimSpace(out.Tagline)
	out.PrimaryColor = strings.ToLower(strings.TrimSpace(out.PrimaryColor))
	out.AccentColor = strings.ToLower(strings.TrimSpace(out.AccentColor))
	out.Theme = strings.ToLower(strings.TrimSpace(out.Theme))

	if out.PrimaryColor == "" {
		if existing != nil && existing.Branding.PrimaryColor != "" {
			out.PrimaryColor = strings.ToLower(strings.TrimSpace(existing.Branding.PrimaryColor))
		} else {
			out.PrimaryColor = "#0f172a"
		}
	}
	if out.AccentColor == "" {
		if existing != nil && existing.Branding.AccentColor != "" {
			out.AccentColor = strings.ToLower(strings.TrimSpace(existing.Branding.AccentColor))
		} else {
			out.AccentColor = "#f59e0b"
		}
	}
	if !hexColorPattern.MatchString(out.PrimaryColor) {
		return out, ValidationError{Field: "branding.primary_color", Message: "cor primária inválida; use #RRGGBB"}
	}
	if !hexColorPattern.MatchString(out.AccentColor) {
		return out, ValidationError{Field: "branding.accent_color", Message: "cor de destaque inválida; use #RRGGBB"}
	}
	if out.Theme == "" {
		out.Theme = "light"
	}
	if out.Theme != "light" && out.Theme != "dark" {
		return out, ValidationError{Field: "branding.theme", Message: "tema deve ser light ou dark"}
	}
	if utf8.RuneCountInString(out.Tagline) > 160 {
		return out, ValidationError{Field: "branding.tagline", Message: "frase de destaque não pode passar de 160 caracteres"}
	}
	return out, nil
}

func (s *SiteService) resolveSlug(ctx context.Context, tenantID, name, requested string, existing *site.Site) (string, error) {
	if requested != "" {
		if err := site.ValidateSlug(requested); err != nil {
			return "", ValidationError{Field: "slug", Message: err.Error()}
		}
		if existing != nil && existing.Slug == requested {
			return requested, nil
		}
		taken, err := s.sites.SlugTaken(ctx, tenantID, requested)
		if err != nil {
			return "", err
		}
		if taken {
			return "", site.ErrSlugTaken
		}
		return requested, nil
	}
	// Slug omitido: mantém o atual (estabilidade do canonical) ou deriva do
	// nome comercial no primeiro save.
	if existing != nil && existing.Slug != "" {
		return existing.Slug, nil
	}
	candidate := site.Slugify(name)
	tenantSuffix := strings.ReplaceAll(tenantID, "-", "")
	if len(tenantSuffix) > 6 {
		tenantSuffix = tenantSuffix[:6]
	}
	// Um nome puramente numérico/simbólico não vira host semântico. O sufixo
	// do tenant mantém o slug determinístico e único mesmo sem nome útil.
	if candidate == "" || !site.HasLetter(candidate) || len(candidate) < site.MinSlugLength {
		prefix := candidate
		if !site.HasLetter(prefix) {
			prefix = ""
		}
		candidate = strings.Trim(strings.Trim(prefix+"-imobiliaria-"+tenantSuffix, "-"), "-")
	}
	if len(candidate) > site.MaxSlugLength {
		candidate = strings.Trim(candidate[:site.MaxSlugLength], "-")
	}
	if _, reserved := site.ReservedSlugs[candidate]; reserved {
		if candidate == "imobiliaria" {
			candidate = "imobiliaria-" + tenantSuffix
		} else {
			candidate = "imobiliaria-" + candidate
		}
	}

	for attempt := 1; attempt <= 50; attempt++ {
		trial := candidate
		if attempt > 1 {
			suffix := fmt.Sprintf("-%d", attempt)
			base := candidate
			if len(base)+len(suffix) > site.MaxSlugLength {
				base = strings.Trim(base[:site.MaxSlugLength-len(suffix)], "-")
			}
			trial = base + suffix
		}
		if err := site.ValidateSlug(trial); err != nil {
			continue
		}
		taken, err := s.sites.SlugTaken(ctx, tenantID, trial)
		if err != nil {
			return "", err
		}
		if !taken {
			return trial, nil
		}
	}
	return "", site.ErrSlugTaken
}

// ListingURL devolve a URL canônica do anúncio APENAS quando o site está
// ligado e a listing está publicada. O portal do cliente usa esta resposta
// para decidir se mostra "Ver anúncio no site".
type ListingURL struct {
	Published bool   `json:"published"`
	PublicURL string `json:"public_url,omitempty"`
	SeoSlug   string `json:"seo_slug,omitempty"`
	Status    string `json:"status,omitempty"`
}

func (s *SiteService) ListingURL(ctx context.Context, tenantID, propertyID string) (*ListingURL, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(propertyID) == "" {
		return nil, site.ErrNotFound
	}
	if _, err := uuid.Parse(propertyID); err != nil {
		return nil, invalid("property_id", "imóvel inválido")
	}
	siteRec, err := s.sites.GetByTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, site.ErrNotFound) {
			return &ListingURL{Published: false, Status: string(site.ListingUnpublished)}, nil
		}
		return nil, err
	}
	listing, err := s.listings.GetByProperty(ctx, tenantID, propertyID)
	if err != nil {
		if errors.Is(err, site.ErrNotFound) {
			return &ListingURL{Published: false, Status: string(site.ListingUnpublished)}, nil
		}
		return nil, err
	}
	published := siteRec.Enabled && listing.Status == site.ListingPublished
	base := s.canonicalPublicURL(siteRec)
	if base == "" {
		base = siteRec.PublicBaseURL
	}
	out := &ListingURL{
		Published: published,
		SeoSlug:   listing.SeoSlug,
		Status:    string(listing.Status),
	}
	if published {
		out.PublicURL = listing.PublicURL(base)
	}
	return out, nil
}

// ListingStats alimenta o card "Performance Web" do detalhe do imóvel.
func (s *SiteService) ListingStats(ctx context.Context, tenantID, propertyID string) (*site.ListingStats, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(propertyID) == "" {
		return nil, site.ErrNotFound
	}
	if _, err := uuid.Parse(propertyID); err != nil {
		return nil, invalid("property_id", "imóvel inválido")
	}
	siteRec, err := s.sites.GetByTenant(ctx, tenantID)
	if err != nil && !errors.Is(err, site.ErrNotFound) {
		return nil, err
	}
	listing, err := s.listings.GetByProperty(ctx, tenantID, propertyID)
	if err != nil && !errors.Is(err, site.ErrNotFound) {
		return nil, err
	}
	stats, err := s.engagement.StatsForListing(ctx, tenantID, propertyID, s.now())
	if err != nil {
		return nil, err
	}
	if siteRec != nil {
		if listing != nil {
			stats.ListingID = listing.ID
			stats.SeoSlug = listing.SeoSlug
			stats.Status = string(listing.Status)
			if listing.PublishedAt != nil {
				stats.PublishedAt = listing.PublishedAt.UTC().Format(time.RFC3339)
			}
			if siteRec.Enabled && listing.Status == site.ListingPublished {
				base := siteRec.PublicBaseURL
				if base == "" {
					base = site.BuildPublicBaseURLWithPort(s.scheme, s.hostSuffix, siteRec.Slug, s.publicPort)
				}
				stats.PublicURL = listing.PublicURL(base)
			}
		} else {
			stats.Status = string(site.ListingUnpublished)
		}
	}
	return stats, nil
}

// SiteStats devolve os agregados da vitrine para o painel /site.
func (s *SiteService) SiteStats(ctx context.Context, tenantID string) (*site.SiteStats, error) {
	return s.engagement.StatsForSite(ctx, tenantID, s.now())
}

// ListLeads é a leitura paginada dos leads do site para o painel.
func (s *SiteService) ListLeads(ctx context.Context, tenantID string, limit, offset int64) ([]site.Lead, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	return s.engagement.ListLeads(ctx, tenantID, limit, offset)
}

func (s *SiteService) revalidate(ctx context.Context, slug string, paths ...string) {
	if s.projection == nil || slug == "" {
		return
	}
	// Revalidação é best-effort: o ISR expira sozinho e uma falha no Next não
	// pode desfazer um save que já foi persistido.
	_ = s.projection.Revalidate(ctx, slug, paths)
}

func defaultPages(siteName string) []site.Page {
	homeTitle := strings.TrimSpace(siteName)
	if homeTitle == "" {
		homeTitle = "Encontre seu novo imóvel"
	}
	return []site.Page{
		{Key: site.PageHome, Title: homeTitle},
		{Key: site.PageAbout, Title: "Sobre nós"},
		{Key: site.PageContact, Title: "Fale conosco"},
	}
}

func mergePages(defaults, loaded []site.Page) []site.Page {
	byKey := make(map[site.PageKey]site.Page, len(loaded))
	for _, p := range loaded {
		byKey[p.Key] = p
	}
	out := make([]site.Page, 0, len(defaults))
	for _, d := range defaults {
		if p, ok := byKey[d.Key]; ok {
			out = append(out, p)
			continue
		}
		out = append(out, d)
	}
	return out
}

func pagePath(key site.PageKey) string {
	switch key {
	case site.PageAbout:
		return "/sobre"
	case site.PageContact:
		return "/contato"
	default:
		return "/"
	}
}
