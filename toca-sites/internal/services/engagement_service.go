package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-sites/internal/domain/site"
)

// RequestMeta é o contexto de borda do site público. IP/UA entram apenas como
// hash de deduplicação e nunca são persistidos crus.
type RequestMeta struct {
	IP        string
	UserAgent string
	Country   string
}

const (
	maxPathLen    = 512
	maxReferrer   = 512
	maxMessageLen = 2000
)

// EngagementService ingere pageviews e leads do site público. O tenant vem
// SEMPRE do Host: nenhum campo do corpo escolhe a imobiliária.
type EngagementService struct {
	sites      site.SiteRepository
	listings   site.ListingRepository
	engagement site.EngagementRepository
	crm        site.LeadSink
	bus        ReliablePublisher
	hostSuffix string
	now        func() time.Time
}

func NewEngagementService(
	sites site.SiteRepository,
	listings site.ListingRepository,
	engagement site.EngagementRepository,
	crm site.LeadSink,
	bus ReliablePublisher,
	hostSuffix string,
) *EngagementService {
	if bus == nil {
		bus = nopPublisher{}
	}
	return &EngagementService{
		sites:      sites,
		listings:   listings,
		engagement: engagement,
		crm:        crm,
		bus:        bus,
		hostSuffix: strings.Trim(strings.TrimSpace(hostSuffix), "."),
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// RecordView grava um pageview de um anúncio publicado do site resolvido pelo
// Host. Anúncio não publicado → 404 (também impede briga entre tenants).
func (s *EngagementService) RecordView(ctx context.Context, host string, in site.ViewInput, meta RequestMeta) error {
	siteRec, err := s.resolveSite(ctx, host)
	if err != nil {
		return err
	}
	propertyID, err := uuid.Parse(strings.TrimSpace(in.PropertyID))
	if err != nil {
		return invalid("property_id", "imóvel inválido")
	}
	listing, err := s.listings.GetPublishedForPublic(ctx, siteRec.TenantID, siteRec.ID, propertyID.String())
	if err != nil {
		if errors.Is(err, site.ErrNotFound) {
			return site.ErrNotFound
		}
		return err
	}

	view := &site.ListingView{
		TenantID:    siteRec.TenantID,
		SiteID:      siteRec.ID,
		ListingID:   listing.ListingID,
		PropertyID:  listing.PropertyID,
		SessionKey:  sessionKey(in.SessionID, meta, s.now()),
		Path:        truncate(in.Path, maxPathLen),
		Referrer:    truncate(in.Referrer, maxReferrer),
		UTMSource:   truncate(in.UTMSource, 120),
		UTMMedium:   truncate(in.UTMMedium, 120),
		UTMCampaign: truncate(in.UTMCampaign, 120),
		Country:     truncate(strings.ToUpper(meta.Country), 2),
	}
	return s.engagement.CreateView(ctx, view)
}

// CreateLead recebe o formulário de interesse. Ordem: valida → resolve site →
// valida anúncio → dedupe → grava local → CRM → evento.
func (s *EngagementService) CreateLead(ctx context.Context, host string, in site.LeadInput, meta RequestMeta) (*site.Lead, error) {
	if strings.TrimSpace(in.Honeypot) != "" {
		return nil, ErrDiscarded
	}
	if !in.AcceptedContact {
		return nil, invalid("accepted_contact", "é preciso aceitar ser contatado pela imobiliária")
	}

	name := validation.SanitizePersonName(in.Name)
	// O CRM usa a mesma regra do signatário (nome + sobrenome). Se o site
	// aceitasse um monônimo, gravaria o lead local e o CRM recusaria — o
	// comprador veria "enviado", mas o lead nunca chegaria ao painel.
	if err := validation.ValidateFullPersonName(name, "Nome"); err != nil {
		return nil, invalid("name", err.Error())
	}
	if utf8.RuneCountInString(name) > 120 {
		name = truncate(name, 120)
	}
	email := strings.TrimSpace(in.Email)
	phone := strings.TrimSpace(in.Phone)
	if email == "" && phone == "" {
		return nil, invalid("email", "informe e-mail ou telefone")
	}
	if email != "" && !validEmail(email) {
		return nil, invalid("email", "e-mail inválido")
	}
	phoneDigits := digitsOnly(phone)
	if phone != "" && (len(phoneDigits) < 8 || len(phoneDigits) > 15) {
		return nil, invalid("phone", "telefone inválido")
	}
	message := truncate(strings.TrimSpace(in.Message), maxMessageLen)

	siteRec, err := s.resolveSite(ctx, host)
	if err != nil {
		return nil, err
	}

	var listing *site.PublicListing
	propertyID := strings.TrimSpace(in.PropertyID)
	if propertyID != "" {
		parsed, err := uuid.Parse(propertyID)
		if err != nil {
			return nil, invalid("property_id", "imóvel inválido")
		}
		listing, err = s.listings.GetPublishedForPublic(ctx, siteRec.TenantID, siteRec.ID, parsed.String())
		if err != nil {
			if errors.Is(err, site.ErrNotFound) {
				return nil, site.ErrNotFound
			}
			return nil, err
		}
	}

	// Dedupe: submissão repetida (duplo clique, retry do browser) não vira
	// dois leads no CRM nem duas notificações. Janela curta para não engolir
	// um contato legítimo do mesmo comprador noutro dia.
	if email != "" || phoneDigits != "" {
		dup, err := s.engagement.FindDuplicateLead(ctx, siteRec.TenantID, propertyID, email, phoneDigits, s.now().Add(-10*time.Minute))
		if err != nil && !errors.Is(err, site.ErrNotFound) {
			return nil, wrap("verificar lead duplicado", err)
		}
		if dup != nil {
			dup.Status = site.LeadDuplicate
			return dup, nil
		}
	}

	lead := &site.Lead{
		ID:          uuid.New().String(),
		TenantID:    siteRec.TenantID,
		SiteID:      siteRec.ID,
		Name:        name,
		Email:       email,
		Phone:       phoneDigits,
		Message:     message,
		Status:      site.LeadReceived,
		OriginHost:  normalizeHost(host),
		UTMSource:   truncate(strings.TrimSpace(in.UTMSource), 120),
		UTMMedium:   truncate(strings.TrimSpace(in.UTMMedium), 120),
		UTMCampaign: truncate(strings.TrimSpace(in.UTMCampaign), 120),
	}
	if listing != nil {
		lead.ListingID = listing.ListingID
		lead.PropertyID = listing.PropertyID
		// O CRM não recebe o estoque: enviamos um snapshot do imóvel para que
		// o painel mostre o interesse e o portal consiga contextualizar a
		// sugestão sem uma chamada cross-tenant no login.
		if rec, recErr := s.listings.GetByProperty(ctx, siteRec.TenantID, listing.PropertyID); recErr == nil && rec != nil {
			lead.PropertyContext = propertyContextFromListing(rec)
		}
	}
	if err := s.engagement.CreateLead(ctx, lead); err != nil {
		return nil, wrap("gravar lead do site", err)
	}

	// O CRM é a fonte de verdade. Falha nele NÃO desfaz o lead local: o
	// comprador já viu "enviado" e o contato não pode ser perdido — o mesmo
	// desenho do webhook do toca-portal.
	if s.crm != nil {
		crmLeadID, err := s.crm.CreateLead(ctx, siteRec.TenantID, *lead)
		if err != nil {
			lead.Status = site.LeadFailed
			lead.Error = truncate(err.Error(), 500)
			if updateErr := s.engagement.UpdateLead(ctx, lead); updateErr != nil {
				log.Printf("[toca-sites] falha ao marcar lead %s como failed: %v", lead.ID, updateErr)
			}
			log.Printf("[toca-sites] CRM indisponível para o lead %s (tenant %s): %v", lead.ID, siteRec.TenantID, err)
			return lead, nil
		}
		lead.Status = site.LeadProcessed
		lead.CRMLeadID = crmLeadID
		if err := s.engagement.UpdateLead(ctx, lead); err != nil {
			return nil, wrap("atualizar lead processado", err)
		}
	}

	s.publishLeadEvent(ctx, siteRec, lead)
	return lead, nil
}

func (s *EngagementService) publishLeadEvent(ctx context.Context, siteRec *site.Site, lead *site.Lead) {
	event := contracts.SiteLeadReceivedEvent{
		EventIDVal: uuid.New().String(),
		Data: contracts.SiteLeadReceivedEventPayload{
			LeadID:     lead.ID,
			TenantID:   lead.TenantID,
			SiteID:     lead.SiteID,
			ListingID:  lead.ListingID,
			PropertyID: lead.PropertyID,
			Name:       lead.Name,
			Email:      lead.Email,
			Phone:      lead.Phone,
			Message:    lead.Message,
			CRMLeadID:  lead.CRMLeadID,
			OriginHost: lead.OriginHost,
			Timestamp:  s.now().Format(time.RFC3339),
		},
	}
	if err := s.bus.PublishReliable(ctx, event); err != nil {
		// O lead já está gravado e no CRM; perder o evento de notificação é
		// ruim, mas devolver 500 faria o comprador reenviar e duplicar.
		log.Printf("[toca-sites] falha ao publicar site.lead.received lead=%s site=%s: %v",
			lead.ID, siteRec.ID, err)
	}
}

func (s *EngagementService) resolveSite(ctx context.Context, host string) (*site.Site, error) {
	slug, err := site.SlugFromHost(host, s.hostSuffix)
	if err != nil {
		// Host fora da zona pública ou slug inválido: 404, sem distinguir.
		return nil, site.ErrNotFound
	}
	siteRec, err := s.sites.FindBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, site.ErrSiteDisabled) {
			return nil, site.ErrNotFound
		}
		return nil, err
	}
	return siteRec, nil
}

func sessionKey(sessionID string, meta RequestMeta, now time.Time) string {
	seed := strings.TrimSpace(sessionID)
	if seed == "" {
		seed = meta.IP + "|" + meta.UserAgent + "|" + now.UTC().Format("2006-01-02")
	}
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:16])
}

func validEmail(email string) bool {
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}
	return addr.Address == email && strings.Contains(addr.Address, "@")
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}

func normalizeHost(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	if i := strings.Index(h, "://"); i >= 0 {
		h = h[i+3:]
	}
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		h = h[:i]
	}
	if i := strings.LastIndex(h, ":"); i >= 0 {
		h = h[:i]
	}
	return strings.TrimSuffix(h, ".")
}

func propertyContextFromListing(l *site.Listing) *site.LeadPropertyContext {
	if l == nil || l.PropertyID == "" {
		return nil
	}
	return &site.LeadPropertyContext{
		PropertyID:    l.PropertyID,
		Title:         l.Title,
		PropertyType:  l.PropertyType,
		ListingType:   l.ListingType,
		Neighborhood:  l.Address.Neighborhood,
		City:          l.Address.City,
		State:         l.Address.State,
		SalePrice:     l.SalePrice,
		RentPrice:     l.RentPrice,
		Bedrooms:      l.Features.Bedrooms,
		Bathrooms:     l.Features.Bathrooms,
		Suites:        l.Features.Suites,
		ParkingSpaces: l.Features.ParkingSpaces,
		AreaTotal:     l.Features.AreaTotal,
		MediaImages:   imageURLs(l.Images),
	}
}

func imageURLs(images []site.Image) []string {
	out := make([]string, 0, len(images))
	for _, image := range images {
		if strings.TrimSpace(image.URL) != "" {
			out = append(out, image.URL)
		}
	}
	return out
}
