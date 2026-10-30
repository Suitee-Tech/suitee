package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/oginoino/toca/pkg/events"
)

// propertySyncer é o recorte usado pelo consumidor (facilita teste sem
// RabbitMQ/Postgres).
type propertySyncer interface {
	HandlePropertyEvent(ctx context.Context, tenantID, propertyID string) error
}

// PropertyConsumer assina os eventos de imóvel no barramento. A fila é
// durável e específica do serviço: o toca-sites NÃO pode competir com
// toca-deal/toca-portal pelo mesmo consumer group.
type PropertyConsumer struct {
	bus  events.EventBus
	sync propertySyncer
}

func NewPropertyConsumer(bus events.EventBus, sync propertySyncer) *PropertyConsumer {
	return &PropertyConsumer{bus: bus, sync: sync}
}

// Queues consumidas pelo serviço. Exportadas para o teste e para o README.
const (
	QueuePropertyUpdated  = "toca-sites.property.updated"
	QueuePropertyCaptured = "toca-sites.property.captured"
)

// Start registra as assinaturas. Pode ser chamado uma vez no boot.
func (c *PropertyConsumer) Start() error {
	if c.bus == nil {
		return nil
	}
	// Strings literais de propósito: o inventário gerado
	// (docs/architecture/generated/routes-events.md) indexa os vínculos de
	// evento por literal e a fila precisa aparecer lá.
	if err := c.bus.SubscribeWithQueue("toca-sites.property.updated", "property.updated", c.handleUploadedOrUpdated); err != nil {
		return fmt.Errorf("assinar property.updated: %w", err)
	}
	if err := c.bus.SubscribeWithQueue("toca-sites.property.captured", "property.captured", c.handleUploadedOrUpdated); err != nil {
		return fmt.Errorf("assinar property.captured: %w", err)
	}
	return nil
}

func (c *PropertyConsumer) handleUploadedOrUpdated(ctx context.Context, event events.Event) error {
	tenantID, propertyID := propertyRefFromPayload(event.Payload())
	if tenantID == "" || propertyID == "" {
		// Evento sem ids não é acionável; devolver nil evita poison message.
		return nil
	}
	return c.sync.HandlePropertyEvent(ctx, tenantID, propertyID)
}

// propertyRefFromPayload aceita tanto o struct tipado (publicação em processo,
// testes) quanto o map[string]interface{} que o RabbitMQ produz ao fazer
// json.Unmarshal do corpo. Marshal/unmarshal normaliza os dois caminhos.
func propertyRefFromPayload(payload interface{}) (tenantID, propertyID string) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", ""
	}
	var p struct {
		TenantID   string `json:"tenant_id"`
		PropertyID string `json:"property_id"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", ""
	}
	return p.TenantID, p.PropertyID
}
