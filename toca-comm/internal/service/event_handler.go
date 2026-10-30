package service

import (
	"context"
	"fmt"
	"log"

	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
)

// CommEventHandler handles incoming domain events and automates communication tasks
type CommEventHandler struct {
	msgService   *MessageService
	inAppService *InAppNotificationService
}

func NewCommEventHandler(msgService *MessageService, inAppService *InAppNotificationService) *CommEventHandler {
	return &CommEventHandler{
		msgService:   msgService,
		inAppService: inAppService,
	}
}

// HandleDealCreated automatically creates a channel when a deal is created
func (h *CommEventHandler) HandleDealCreated(ctx context.Context, event events.Event) error {
	payload, ok := event.Payload().(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid payload format for DealCreated event")
	}

	dealID, _ := payload["deal_id"].(string)
	tenantID, _ := payload["tenant_id"].(string)
	buyerID, _ := payload["buyer_id"].(string)
	brokerID, _ := payload["broker_id"].(string)
	title, _ := payload["title"].(string)

	if dealID == "" || tenantID == "" {
		return fmt.Errorf("missing deal_id or tenant_id in DealCreated event")
	}

	// Set context for the service
	ctx = pkgcontext.WithTenantID(ctx, tenantID)
	// We need a system account ID to create the channel if it's automated.
	// For now, let's use the brokerID if available, or a system constant.
	systemID := "system"
	if brokerID != "" {
		systemID = brokerID
	}
	ctx = pkgcontext.WithAccountID(ctx, systemID)
	ctx = context.WithValue(ctx, CtxSenderName, "Toca Imóveis")

	channelName := fmt.Sprintf("Negócio: %s", title)
	if title == "" {
		channelName = fmt.Sprintf("Negócio #%s", dealID[:8])
	}

	participants := []string{}
	if buyerID != "" {
		participants = append(participants, buyerID)
	}
	if brokerID != "" {
		participants = append(participants, brokerID)
	}

	// Create Channel
	ch, err := h.msgService.CreateChannel(ctx, channelName, message.ChannelTypeDeal, participants, dealID, "deal")
	if err != nil {
		log.Printf("Failed to auto-create channel for deal %s: %v", dealID, err)
		return err
	}

	// Send an initial automated message
	content := "Canal de comunicação criado automaticamente para este negócio. Todos os participantes já foram adicionados."

	// Add content_type system
	ctx = context.WithValue(ctx, CtxContentType, "system")

	_, err = h.msgService.SendMessage(ctx, ch.ID, content)
	if err != nil {
		log.Printf("Failed to send welcome message for deal %s: %v", dealID, err)
	}

	log.Printf("Successfully created automated channel for deal %s", dealID)
	return nil
}

// HandleLeadCreated could automatically create a channel or send a welcome message
func (h *CommEventHandler) HandleLeadCreated(ctx context.Context, event events.Event) error {
	// Implementation similar to HandleDealCreated
	return nil
}

// HandleLeadAssigned é mantido apenas como no-op para preservar a
// assinatura pública até que callers externos sejam atualizados. A
// lógica real (idempotência + dispatch de canais + tipagem
// InAppTypeLeadAssigned para o NotificationBell) vive em
// EventConsumer.Start (event_consumer.go — queue comm_lead_assigned).
//
// Histórico: este handler causava double-notification quando rodava em
// paralelo com o consumer (#408). Como ambos consumiam a mesma routing
// key "lead.assigned" via filas distintas, cada evento gerava duas
// notificações in-app e dois broadcasts WS. main.go já não chama
// SubscribeWithQueue para este handler — deixar o método aqui evita
// erros de compilação em outros pacotes que ainda o referenciam.
//
// Deprecated: use o consumer em event_consumer.go (comm_lead_assigned).
func (h *CommEventHandler) HandleLeadAssigned(ctx context.Context, event events.Event) error {
	_ = ctx
	_ = event
	return nil
}

// HandleRouletteConfigUpdated repassa a notificação de mudança de
// configuração da roleta para todos os usuários conectados ao tenant.
// O FE invalida a query local e mostra um toast quando a mudança veio
// de outro admin. Fix da seam P1.6 do review de Roleta — antes não
// existia broadcast e dois admins se sobrescreviam silenciosamente.
func (h *CommEventHandler) HandleRouletteConfigUpdated(ctx context.Context, event events.Event) error {
	payload, ok := event.Payload().(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid payload format for RouletteConfigUpdated event")
	}

	tenantID, _ := payload["tenant_id"].(string)
	if tenantID == "" {
		return fmt.Errorf("missing tenant_id in RouletteConfigUpdated event")
	}

	broadcastPayload := map[string]interface{}{
		"tenant_id":      tenantID,
		"actor_id":       payload["actor_id"],
		"actor_email":    payload["actor_email"],
		"changed_fields": payload["changed_fields"],
		"updated_at":     payload["updated_at"],
	}

	if h.msgService != nil && h.msgService.Hub() != nil {
		hub := h.msgService.Hub()
		connectedUsers := hub.GetTenantUserIDs(tenantID)
		if len(connectedUsers) > 0 {
			hub.BroadcastEvent(tenantID, connectedUsers, "roulette_config_updated", broadcastPayload)
		}
	}

	log.Printf("Broadcasted roulette_config_updated for tenant %s by actor %v", tenantID, payload["actor_id"])
	return nil
}

// HandleBrokerChanged broadcasts a "broker_changed" event to all WebSocket clients of the
// affected tenant so they can re-fetch the broker list without waiting for revalidate-on-focus
// (closes the inconsistency window when admins edit/add/remove brokers from another tab).
//
// The event source is `broker.updated` (publicado pelo toca-crm em Create/Update/Delete) —
// renomeamos para `broker_changed` no payload WS para que o frontend trate todos os casos
// de mudança com um único subscriber.
func (h *CommEventHandler) HandleBrokerChanged(_ context.Context, event events.Event) error {
	payload, ok := event.Payload().(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid payload format for BrokerUpdated event")
	}

	tenantID, _ := payload["tenant_id"].(string)
	if tenantID == "" {
		return fmt.Errorf("missing tenant_id in BrokerUpdated event")
	}

	brokerID, _ := payload["broker_id"].(string)
	name, _ := payload["name"].(string)
	role, _ := payload["role"].(string)
	avatar, _ := payload["avatar"].(string)
	active, _ := payload["active"].(bool)
	paused, _ := payload["paused"].(bool)

	broadcastPayload := map[string]interface{}{
		"broker_id": brokerID,
		"tenant_id": tenantID,
		"name":      name,
		"role":      role,
		"avatar":    avatar,
		"active":    active,
		"paused":    paused,
	}

	// Broadcast a todos os usuários conectados deste tenant — o frontend reage chamando
	// refreshBrokers() para revalidar a lista local. Falha de broadcast é apenas logada
	// (o cliente ainda tem o revalidate-on-focus como fallback).
	if h.msgService != nil && h.msgService.Hub() != nil {
		hub := h.msgService.Hub()
		connectedUsers := hub.GetTenantUserIDs(tenantID)
		if len(connectedUsers) > 0 {
			hub.BroadcastEvent(tenantID, connectedUsers, "broker_changed", broadcastPayload)
		}
	}

	log.Printf("Broadcasted broker_changed for tenant %s: broker %s (active=%v)", tenantID, brokerID, active)
	return nil
}

// HandlePortalLeadReceived broadcasts a real-time notification when a lead is received from a portal
// HandleSiteLeadReceived broadcasts a real-time notification when a lead is
// captured by the public storefront (ADR 0006). The event is published by
// toca-sites after the local lead is persisted and the CRM lead is created.
func (h *CommEventHandler) HandleSiteLeadReceived(ctx context.Context, event events.Event) error {
	payload, ok := event.Payload().(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid payload format for SiteLeadReceived event")
	}

	tenantID, _ := payload["tenant_id"].(string)
	if tenantID == "" {
		return fmt.Errorf("missing tenant_id in SiteLeadReceived event")
	}

	broadcastPayload := map[string]interface{}{
		"lead_id":     payload["lead_id"],
		"crm_lead_id": payload["crm_lead_id"],
		"site_id":     payload["site_id"],
		"listing_id":  payload["listing_id"],
		"property_id": payload["property_id"],
		"name":        payload["name"],
		"email":       payload["email"],
		"phone":       payload["phone"],
		"origin_host": payload["origin_host"],
	}

	h.broadcastTenantEvent(tenantID, "site_lead_received", broadcastPayload)
	log.Printf("Broadcasted site_lead_received event for tenant %s", tenantID)
	return nil
}

// HandleSiteListingEvent broadcasts listing published/unpublished transitions
// so the panel can refresh the "Performance Web" card without polling.
func (h *CommEventHandler) HandleSiteListingEvent(ctx context.Context, event events.Event) error {
	payload, ok := event.Payload().(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid payload format for SiteListing event")
	}

	tenantID, _ := payload["tenant_id"].(string)
	if tenantID == "" {
		return fmt.Errorf("missing tenant_id in SiteListing event")
	}

	eventType := "site_listing_published"
	if event.Name() == "site.listing.unpublished" {
		eventType = "site_listing_unpublished"
	}
	broadcastPayload := map[string]interface{}{
		"site_id":     payload["site_id"],
		"listing_id":  payload["listing_id"],
		"property_id": payload["property_id"],
		"seo_slug":    payload["seo_slug"],
		"public_url":  payload["public_url"],
		"reason":      payload["reason"],
	}
	h.broadcastTenantEvent(tenantID, eventType, broadcastPayload)
	log.Printf("Broadcasted %s event for tenant %s", eventType, tenantID)
	return nil
}

func (h *CommEventHandler) broadcastTenantEvent(tenantID, eventType string, payload map[string]interface{}) {
	if h.msgService == nil || h.msgService.Hub() == nil {
		return
	}
	hub := h.msgService.Hub()
	connectedUsers := hub.GetTenantUserIDs(tenantID)
	if len(connectedUsers) == 0 {
		return
	}
	hub.BroadcastEvent(tenantID, connectedUsers, eventType, payload)
}

func (h *CommEventHandler) HandlePortalLeadReceived(ctx context.Context, event events.Event) error {
	payload, ok := event.Payload().(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid payload format for PortalLeadReceived event")
	}

	tenantID, _ := payload["tenant_id"].(string)
	if tenantID == "" {
		return fmt.Errorf("missing tenant_id in PortalLeadReceived event")
	}

	portalLeadID, _ := payload["portal_lead_id"].(string)
	crmLeadID, _ := payload["crm_lead_id"].(string)
	portal, _ := payload["portal"].(string)
	leadName, _ := payload["lead_name"].(string)
	leadEmail, _ := payload["lead_email"].(string)
	propertyID, _ := payload["property_id"].(string)

	// Broadcast to all connected users of this tenant
	broadcastPayload := map[string]interface{}{
		"portal_lead_id": portalLeadID,
		"crm_lead_id":    crmLeadID,
		"portal":         portal,
		"lead_name":      leadName,
		"lead_email":     leadEmail,
		"property_id":    propertyID,
	}

	// Broadcast to all connected users of this tenant via WebSocket
	if h.msgService != nil && h.msgService.Hub() != nil {
		hub := h.msgService.Hub()
		connectedUsers := hub.GetTenantUserIDs(tenantID)
		if len(connectedUsers) > 0 {
			hub.BroadcastEvent(tenantID, connectedUsers, "portal_lead_received", broadcastPayload)
		}
	}

	log.Printf("Broadcasted portal_lead_received event for tenant %s: lead %s from %s", tenantID, leadName, portal)
	return nil
}
