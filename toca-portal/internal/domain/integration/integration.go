package integration

import (
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/validation"
)

var validate = validation.New()

type PortalType string

const (
	PortalZAP      PortalType = "zap_imoveis"
	PortalOLX      PortalType = "olx_grupo_zap"
	PortalVivaReal PortalType = "vivareal"
)

type IntegrationStatus string

const (
	IntegrationActive       IntegrationStatus = "active"
	IntegrationDisconnected IntegrationStatus = "disconnected"
	IntegrationError        IntegrationStatus = "error"
)

type PortalIntegration struct {
	ID               string             `json:"id" bson:"_id" validate:"required"`
	TenantID         string             `json:"tenant_id" bson:"tenant_id" validate:"required"`
	Portal           PortalType         `json:"portal" bson:"portal" validate:"required,oneof=zap_imoveis olx_grupo_zap vivareal"`
	Status           IntegrationStatus  `json:"status" bson:"status" validate:"required"`
	FeedURL          string             `json:"feed_url" bson:"feed_url"`
	FeedToken        string             `json:"-" bson:"feed_token"`
	WebhookSecret    string             `json:"-" bson:"webhook_secret"`
	WebhookURL       string             `json:"webhook_url" bson:"webhook_url"`
	SyncFrequency    string             `json:"sync_frequency" bson:"sync_frequency" validate:"required,oneof=6h 12h 24h"`
	ContactName      string             `json:"contact_name" bson:"contact_name" validate:"required"`
	ContactEmail     string             `json:"contact_email" bson:"contact_email" validate:"required,email"`
	ContactPhone     string             `json:"contact_phone" bson:"contact_phone" validate:"required"`
	PublicationType  string             `json:"publication_type" bson:"publication_type" validate:"required,oneof=STANDARD PREMIUM SUPER_PREMIUM"`
	PropertyFilter   FeedPropertyFilter `json:"property_filter" bson:"property_filter"`
	CreatedAt        time.Time          `json:"created_at" bson:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at" bson:"updated_at"`
	LastSyncAt       *time.Time         `json:"last_sync_at" bson:"last_sync_at"`
	LastSyncStatus   string             `json:"last_sync_status,omitempty" bson:"last_sync_status,omitempty"`
	ErrorMessage     string             `json:"error_message,omitempty" bson:"error_message,omitempty"`
	ConsecutiveFails int                `json:"consecutive_fails" bson:"consecutive_fails"`
}

func (p *PortalIntegration) Validate() error {
	return validate.Struct(p)
}

func NewPortalIntegration(tenantID string, portal PortalType, contactName, contactEmail, contactPhone, syncFrequency, publicationType string, feedBaseURL string) (*PortalIntegration, error) {
	now := time.Now()
	feedToken := uuid.New().String()

	p := &PortalIntegration{
		ID:              uuid.New().String(),
		TenantID:        tenantID,
		Portal:          portal,
		Status:          IntegrationActive,
		FeedToken:       feedToken,
		FeedURL:         feedBaseURL + "/api/v1/portals/feed/" + feedToken,
		WebhookSecret:   uuid.New().String(),
		SyncFrequency:   syncFrequency,
		ContactName:     contactName,
		ContactEmail:    contactEmail,
		ContactPhone:    contactPhone,
		PublicationType: publicationType,
		PropertyFilter:  FeedPropertyFilter{},
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := p.Validate(); err != nil {
		return nil, err
	}

	return p, nil
}

// PortalDisplayName returns the human-readable name for a portal
func PortalDisplayName(portal PortalType) string {
	switch portal {
	case PortalZAP:
		return "ZAP Imoveis"
	case PortalOLX:
		return "OLX / Grupo ZAP"
	case PortalVivaReal:
		return "VivaReal"
	default:
		return string(portal)
	}
}
