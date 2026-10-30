package site

import (
	"context"
	"time"
)

// PropertySnapshot é o recorte do imóvel do toca-core necessário para montar a
// projeção pública. O sync SEMPRE relê o imóvel: o evento `property.updated`
// carrega só o suficiente para localizar o agregado, e uma projeção montada a
// partir do evento ficaria com preço/status velhos.
type PropertySnapshot struct {
	ID          string
	Title       string
	Description string
	Status      string

	PublicationStatus string
	PropertyType      string
	ListingType       string

	Address   Address
	Features  Features
	Amenities []string
	Images    []Image
	AreaTotal float64
	AreaBuilt float64

	SalePrice   float64
	RentPrice   float64
	CondoFee    float64
	PropertyTax float64

	UpdatedAt time.Time
}

// IsPublishableOnSite aplica a regra única de publicação na vitrine:
// status=published E publication_status=published. O site enabled é checado
// separadamente, porque o imóvel pode estar pronto enquanto o site está
// desligado.
func (p PropertySnapshot) IsPublishableOnSite() bool {
	return p.Status == "published" && p.PublicationStatus == "published"
}

// PropertySource lê o imóvel do toca-core por HTTP interno.
type PropertySource interface {
	GetProperty(ctx context.Context, tenantID, propertyID string) (*PropertySnapshot, error)
}

// PropertyLister pagina o estoque do tenant. Usado no backfill quando o site
// é ligado: os eventos cobrem mudanças futuras, não o estoque já existente.
type PropertyLister interface {
	ListProperties(ctx context.Context, tenantID string, limit, offset int) ([]*PropertySnapshot, int64, error)
}

// Projection é a escrita no Payload CMS. O Go é dono da orquestração;
// o Payload é uma projeção, nunca source of truth.
type Projection interface {
	// EnsureTenant cria/atualiza o tenant Payload da imobiliária e devolve o
	// doc id (string) usado nas próximas chamadas.
	EnsureTenant(ctx context.Context, s *Site) (string, error)
	// UpsertListing cria ou atualiza a listing Payload e devolve o doc id.
	UpsertListing(ctx context.Context, s *Site, l *Listing) (string, error)
	UnpublishListing(ctx context.Context, s *Site, l *Listing) error
	// UpsertPage publica uma das três páginas institucionais no CMS.
	UpsertPage(ctx context.Context, s *Site, p Page) error
	// Revalidate invalida o cache ISR dos caminhos afetados no Next.
	Revalidate(ctx context.Context, tenantSlug string, paths []string) error
}

// LeadSink cria o lead no toca-crm via HTTP interno. A fonte de verdade do
// lead continua sendo o CRM; o toca-sites guarda o espelho para métricas e
// para não perder contato quando o CRM estiver indisponível.
type LeadSink interface {
	CreateLead(ctx context.Context, tenantID string, l Lead) (string, error)
}
