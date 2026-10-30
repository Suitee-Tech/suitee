package services

import (
	"context"
	"fmt"

	"github.com/oginoino/toca-core/internal/domain/core/property"
	"github.com/oginoino/toca/pkg/events"
)

func NewDealEventHandler(propService PropertyService) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		// Payload is expected to be map[string]interface{} from JSON unmarshal
		payload, ok := event.Payload().(map[string]interface{})
		if !ok {
			// Try to handle if it's already the struct (in memory bus)
			return fmt.Errorf("invalid payload type: %T", event.Payload())
		}

		// Extract fields safely
		propID, _ := payload["property_id"].(string)
		newStatus, _ := payload["new_status"].(string)
		dealType, _ := payload["deal_type"].(string)

		if propID == "" || newStatus == "" {
			return nil // Skip invalid events
		}

		var propStatus property.PropertyStatus
		switch newStatus {
		case "NEGOTIATION":
			propStatus = property.PropertyStatusNegotiating
		case "PROPOSAL":
			propStatus = property.PropertyStatusProposal
		case "ANALYSIS":
			propStatus = property.PropertyStatusAnalysis
		case "CONTRACT":
			propStatus = property.PropertyStatusContract
		case "CONVERTED":
			if dealType == "RENT" || dealType == "rent" || dealType == "LOCACAO" || dealType == "locacao" {
				propStatus = property.PropertyStatusRented
			} else {
				propStatus = property.PropertyStatusSold
			}
		case "CANCELLED":
			// If cancelled, it goes back to... Published? Attending?
			// Ideally it should go back to the previous state, but we don't track it easily here.
			// Let's assume Published to be safe and visible again.
			propStatus = property.PropertyStatusPublished
		default:
			// No change for other statuses yet
			return nil
		}

		// Log the action
		fmt.Printf("Updating property %s status to %s based on deal status %s\n", propID, propStatus, newStatus)

		return propService.UpdateStatus(ctx, propID, propStatus)
	}
}
