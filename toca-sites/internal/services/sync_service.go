package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-sites/internal/client/core"
	"github.com/oginoino/toca/toca-sites/internal/domain/site"
)

// SyncService materializa a regra de publicação na vitrine:
//
//	site.enabled AND property.status=published AND property.publication_status=published
//
// Qualquer outra combinação despublica a projeção. A regra fica num só lugar
// porque ela é a fronteira entre "estoque já capturado" e "anúncio público" —
// errar aqui expõe imóvel em rascunho ou tira do ar um anúncio ativo.
type SyncService struct {
	sites      site.SiteRepository
	listings   site.ListingRepository
	properties site.PropertySource
	lister     site.PropertyLister
	projection site.Projection
	bus        ReliablePublisher
	scheme     string
	hostSuffix string
	publicPort string
	now        func() time.Time
}

func NewSyncService(
	sites site.SiteRepository,
	listings site.ListingRepository,
	properties site.PropertySource,
	projection site.Projection,
	bus ReliablePublisher,
	hostSuffix, scheme string,
) *SyncService {
	if bus == nil {
		bus = nopPublisher{}
	}
	if projection == nil {
		projection = payloadNoop{}
	}
	return &SyncService{
		sites:      sites,
		listings:   listings,
		properties: properties,
		lister:     asLister(properties),
		projection: projection,
		bus:        bus,
		scheme:     scheme,
		hostSuffix: hostSuffix,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// HandlePropertyEvent é a porta do consumidor RabbitMQ. Eventos com ids
// inválidos são descartados (poison message não deve ser reentregue para
// sempre); falhas de infraestrutura sobem para o bus reenfileirar.
func (s *SyncService) HandlePropertyEvent(ctx context.Context, tenantID, propertyID string) error {
	if _, err := uuid.Parse(strings.TrimSpace(tenantID)); err != nil {
		log.Printf("[toca-sites] sync ignorado: tenant_id inválido %q", tenantID)
		return nil
	}
	if _, err := uuid.Parse(strings.TrimSpace(propertyID)); err != nil {
		log.Printf("[toca-sites] sync ignorado: property_id inválido %q", propertyID)
		return nil
	}
	return s.SyncProperty(ctx, tenantID, propertyID)
}

// SyncProperty relê o imóvel no toca-core e reconcilia a projeção. É
// idempotente: o mesmo imóvel pode ser sincronizado quantas vezes o outbox
// entregar.
func (s *SyncService) SyncProperty(ctx context.Context, tenantID, propertyID string) error {
	siteRec, err := s.sites.GetByTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, site.ErrNotFound) {
			// Sem vitrine cadastrada não existe projeção a desfazer.
			return nil
		}
		return wrap("carregar site", err)
	}

	if !siteRec.Enabled {
		return s.unpublish(ctx, siteRec, propertyID, "site_disabled")
	}

	property, err := s.properties.GetProperty(ctx, tenantID, propertyID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return s.unpublish(ctx, siteRec, propertyID, "property_missing")
		}
		return wrap("carregar imóvel no core", err)
	}

	if !property.IsPublishableOnSite() {
		return s.unpublish(ctx, siteRec, propertyID,
			fmt.Sprintf("property_not_public (status=%s publication_status=%s)", property.Status, property.PublicationStatus))
	}

	return s.publish(ctx, siteRec, property)
}

// ReconcileTenant projeta o estoque existente do tenant. Roda em background
// quando o site é ligado; é idempotente e não publica evento para anúncio que
// já estava publicado.
func (s *SyncService) ReconcileTenant(ctx context.Context, tenantID string) error {
	if s.lister == nil {
		log.Printf("[toca-sites] backfill ignorado para tenant %s: fonte sem listagem", tenantID)
		return nil
	}
	siteRec, err := s.sites.GetByTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, site.ErrNotFound) {
			return nil
		}
		return wrap("carregar site para backfill", err)
	}
	if !siteRec.Enabled {
		return nil
	}

	const pageSize = 100
	var processed int64
	for offset := 0; ; offset += pageSize {
		properties, total, err := s.lister.ListProperties(ctx, tenantID, pageSize, offset)
		if err != nil {
			return wrap("listar imóveis para backfill", err)
		}
		for _, property := range properties {
			if !property.IsPublishableOnSite() {
				if err := s.unpublish(ctx, siteRec, property.ID,
					fmt.Sprintf("property_not_public (status=%s publication_status=%s)", property.Status, property.PublicationStatus)); err != nil {
					return err
				}
				continue
			}
			if err := s.publish(ctx, siteRec, property); err != nil {
				return err
			}
		}
		processed += int64(len(properties))
		if len(properties) == 0 || processed >= total {
			break
		}
	}
	log.Printf("[toca-sites] backfill concluído tenant=%s imóveis=%d", tenantID, processed)
	return nil
}

func (s *SyncService) publish(ctx context.Context, siteRec *site.Site, property *site.PropertySnapshot) error {
	existing, err := s.listings.GetByProperty(ctx, siteRec.TenantID, property.ID)
	if err != nil && !errors.Is(err, site.ErrNotFound) {
		return wrap("carregar projeção local", err)
	}
	wasPublished := existing != nil && existing.Status == site.ListingPublished

	seoSlug := ""
	publishedAt := s.now()
	if existing != nil {
		seoSlug = existing.SeoSlug
		if wasPublished && existing.PublishedAt != nil {
			publishedAt = *existing.PublishedAt
		}
	}
	if seoSlug == "" {
		seoSlug, err = s.uniqueSeoSlug(ctx, siteRec.TenantID, property.ID, property.Title)
		if err != nil {
			return err
		}
	}

	var (
		projection *site.Listing
		saved      *site.Listing
		docID      string
	)
	// Corrida de slug: dois imóveis com o mesmo título podem escolher o mesmo
	// candidato entre o SELECT de disponibilidade e o INSERT. O índice único
	// do banco vence; recalculamos o sufixo e regravamos a projeção/Payload.
	for attempt := 1; attempt <= 5; attempt++ {
		projection = buildListing(siteRec, property, seoSlug, publishedAt)

		// Payload primeiro: se o CMS falhar, o retry do RabbitMQ refaz tudo sem
		// deixar o painel dizendo "publicado" para um anúncio que não existe lá.
		docID, err = s.projection.UpsertListing(ctx, siteRec, projection)
		if err != nil {
			return wrap("publicar listing no Payload", err)
		}

		saved, err = s.listings.Upsert(ctx, projection)
		if err == nil {
			break
		}
		if errors.Is(err, site.ErrSlugTaken) && (existing == nil || existing.SeoSlug == "") && attempt < 5 {
			seoSlug = ""
			seoSlug, err = s.uniqueSeoSlug(ctx, siteRec.TenantID, property.ID, property.Title)
			if err != nil {
				return err
			}
			continue
		}
		return wrap("persistir projeção local", err)
	}
	if saved == nil {
		return fmt.Errorf("não foi possível persistir projeção do imóvel %s após colisões de slug", property.ID)
	}
	if docID != "" {
		if err := s.listings.SetPayloadDocID(ctx, siteRec.TenantID, saved.ID, docID); err != nil {
			return wrap("gravar doc id do Payload", err)
		}
	}

	// Só transição entra no bus: cada re-sync de um anúncio já publicado não
	// pode gerar notificação duplicada para a imobiliária.
	if !wasPublished {
		event := contracts.SiteListingPublishedEvent{
			EventIDVal: uuid.New().String(),
			Data: contracts.SiteListingPublishedEventPayload{
				SiteID:     siteRec.ID,
				TenantID:   siteRec.TenantID,
				ListingID:  saved.ID,
				PropertyID: property.ID,
				SeoSlug:    seoSlug,
				PublicURL:  saved.PublicURL(s.publicBaseURL(siteRec)),
				Timestamp:  s.now().Format(time.RFC3339),
			},
		}
		if err := s.bus.PublishReliable(ctx, event); err != nil {
			return wrap("publicar site.listing.published", err)
		}
	}

	s.revalidate(siteRec.Slug, "/", "/imoveis", "/imoveis/"+seoSlug)
	return nil
}

func (s *SyncService) unpublish(ctx context.Context, siteRec *site.Site, propertyID, reason string) error {
	existing, err := s.listings.GetByProperty(ctx, siteRec.TenantID, propertyID)
	if err != nil {
		if errors.Is(err, site.ErrNotFound) {
			return nil
		}
		return wrap("carregar projeção local", err)
	}

	wasPublished := existing.Status == site.ListingPublished
	if existing.PayloadDocID != "" {
		if err := s.projection.UnpublishListing(ctx, siteRec, existing); err != nil {
			return wrap("despublicar listing no Payload", err)
		}
	}

	if _, err := s.listings.MarkUnpublished(ctx, siteRec.TenantID, propertyID); err != nil {
		if errors.Is(err, site.ErrNotFound) {
			return nil
		}
		return wrap("despublicar projeção local", err)
	}

	if wasPublished {
		event := contracts.SiteListingUnpublishedEvent{
			EventIDVal: uuid.New().String(),
			Data: contracts.SiteListingUnpublishedEventPayload{
				SiteID:     siteRec.ID,
				TenantID:   siteRec.TenantID,
				ListingID:  existing.ID,
				PropertyID: propertyID,
				Reason:     reason,
				Timestamp:  s.now().Format(time.RFC3339),
			},
		}
		if err := s.bus.PublishReliable(ctx, event); err != nil {
			return wrap("publicar site.listing.unpublished", err)
		}
	}

	if existing.SeoSlug != "" {
		s.revalidate(siteRec.Slug, "/", "/imoveis", "/imoveis/"+existing.SeoSlug)
	}
	return nil
}

func (s *SyncService) uniqueSeoSlug(ctx context.Context, tenantID, propertyID, title string) (string, error) {
	for attempt := 1; attempt <= 50; attempt++ {
		candidate := site.SeoSlug(title, propertyID, attempt)
		taken, err := s.listings.SeoSlugTaken(ctx, tenantID, candidate, propertyID)
		if err != nil {
			return "", wrap("verificar slug SEO", err)
		}
		if !taken {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("não foi possível gerar slug SEO único para o imóvel %s", propertyID)
}

// asLister aproveita a mesma implementação de core quando ela existe, sem
// obrigar PropertySource (e os fakes) a implementar listagem.
func asLister(source site.PropertySource) site.PropertyLister {
	lister, _ := source.(site.PropertyLister)
	return lister
}

// WithPublicPort informa a porta local do Next para URL/eventos.
func (s *SyncService) WithPublicPort(port string) *SyncService {
	s.publicPort = strings.TrimSpace(port)
	return s
}

func (s *SyncService) publicBaseURL(siteRec *site.Site) string {
	if siteRec.PublicBaseURL != "" {
		return siteRec.PublicBaseURL
	}
	return site.BuildPublicBaseURLWithPort(s.scheme, s.hostSuffix, siteRec.Slug, s.publicPort)
}

func (s *SyncService) revalidate(slug string, paths ...string) {
	if s.projection == nil || slug == "" {
		return
	}
	_ = s.projection.Revalidate(context.Background(), slug, paths)
}

func buildListing(siteRec *site.Site, p *site.PropertySnapshot, seoSlug string, publishedAt time.Time) *site.Listing {
	at := publishedAt
	// O Payload exige title/description. Um imóvel sem descrição no core não
	// pode fazer o sync falhar em loop nem renderizar um card vazio: gera um
	// texto mínimo a partir do tipo/localização.
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = "Imóvel" + locationSuffix(p)
	}
	description := strings.TrimSpace(p.Description)
	if description == "" {
		description = title + " com fotos, características e contato direto com a imobiliária."
	}
	return &site.Listing{
		TenantID:     siteRec.TenantID,
		SiteID:       siteRec.ID,
		PropertyID:   p.ID,
		SeoSlug:      seoSlug,
		Status:       site.ListingPublished,
		Title:        title,
		Description:  description,
		ListingType:  p.ListingType,
		PropertyType: p.PropertyType,
		SalePrice:    p.SalePrice,
		RentPrice:    p.RentPrice,
		CondoFee:     p.CondoFee,
		PropertyTax:  p.PropertyTax,
		Address: site.Address{
			// Rua/bairro/cidade bastam para o comprador se situar; número e
			// complemento ficam fora da projeção pública para não expor a
			// unidade exata antes do atendimento.
			Street:       p.Address.Street,
			Neighborhood: p.Address.Neighborhood,
			City:         p.Address.City,
			State:        p.Address.State,
			PostalCode:   p.Address.PostalCode,
		},
		Features: site.Features{
			Bedrooms:      p.Features.Bedrooms,
			Bathrooms:     p.Features.Bathrooms,
			Suites:        p.Features.Suites,
			ParkingSpaces: p.Features.ParkingSpaces,
			// Áreas não são "features" no domínio do core, mas o Payload as
			// agrupa junto dos demais atributos exibidos no card.
			AreaTotal: p.AreaTotal,
			AreaBuilt: p.AreaBuilt,
		},
		Amenities:       p.Amenities,
		Images:          p.Images,
		SourceUpdatedAt: p.UpdatedAt,
		PublishedAt:     &at,
	}
}

func locationSuffix(p *site.PropertySnapshot) string {
	if p == nil {
		return ""
	}
	if city := strings.TrimSpace(p.Address.City); city != "" {
		return " em " + city
	}
	return ""
}

// payloadNoop permite rodar o serviço sem Payload (dev/testes) mantendo a
// projeção local e o cadastro do site. Em produção PAYLOAD_API_URL é
// obrigatório para a vitrine render.
type payloadNoop struct{}

func (payloadNoop) EnsureTenant(context.Context, *site.Site) (string, error) { return "", nil }
func (payloadNoop) UpsertListing(context.Context, *site.Site, *site.Listing) (string, error) {
	return "", nil
}
func (payloadNoop) UnpublishListing(context.Context, *site.Site, *site.Listing) error { return nil }
func (payloadNoop) UpsertPage(context.Context, *site.Site, site.Page) error           { return nil }
func (payloadNoop) Revalidate(context.Context, string, []string) error                { return nil }
