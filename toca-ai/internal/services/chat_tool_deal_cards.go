package services

import (
	"fmt"
	"strings"

	"github.com/oginoino/toca/pkg/money"
)

func buildDealCards(deal map[string]any) suiteeCardsBlock {
	dealID := firstString(deal, "id")
	displayID := firstString(deal, "display_id", "displayId")
	status := firstString(deal, "status", "stage")
	dealType := firstString(deal, "type")
	value := nestedNumber(deal, "value")
	customerName := firstString(deal, "customer_name", "customerName")
	propertyID := firstString(deal, "property_id")
	ownerID := firstString(deal, "owner_id")
	customerID := firstString(deal, "customer_id")
	updatedAt := firstString(deal, "updated_at")

	title := strings.TrimSpace(displayID)
	if title == "" {
		title = strings.TrimSpace(dealID)
	}
	if title == "" {
		title = "Deal"
	}
	title = "Deal " + title

	sub := strings.TrimSpace(customerName)
	if sub == "" {
		sub = strings.TrimSpace(customerID)
	}
	if value > 0 {
		if sub == "" {
			sub = money.BRLWhole(value)
		} else {
			sub = sub + " · " + money.BRLWhole(value)
		}
	}

	badges := make([]string, 0, 4)
	if dealType != "" {
		badges = append(badges, humanizeDealType(dealType))
	}
	if status != "" {
		badges = append(badges, humanizeDealStage(status))
	}

	details := make([]suiteeCardDetail, 0, 6)
	if propertyID != "" {
		details = append(details, suiteeCardDetail{Label: "Imóvel", Value: propertyID})
	}
	if customerID != "" {
		details = append(details, suiteeCardDetail{Label: "Cliente", Value: customerID})
	}
	if ownerID != "" {
		details = append(details, suiteeCardDetail{Label: "Proprietário", Value: ownerID})
	}

	meta := make([]suiteeCardMetadata, 0, 2)
	if value > 0 {
		meta = append(meta, suiteeCardMetadata{Kind: "currency", Value: formatCurrencyBRLFromFloat(value)})
	}
	if updatedAt != "" {
		meta = append(meta, suiteeCardMetadata{Kind: "date", Value: updatedAt})
	}

	items := make([]suiteeCardItem, 0, 10)
	if dealID != "" {
		items = append(items, suiteeCardItem{
			Key:          dealID,
			Title:        title,
			Subtitle:     sub,
			Badges:       badges,
			StatusColor:  statusColorFromStatus(status),
			OpenPath:     "/esteira/deals/" + dealID,
			Copy:         title,
			ActionPrompt: "Sugira os próximos passos imediatos para avançar este deal, considerando status e checklist.",
			Metadata:     meta,
			Details:      details,
		})
	}

	checklist, _ := deal["checklist"].([]any)
	if len(checklist) > 0 && dealID != "" {
		pending := 0
		approved := 0
		rejected := 0
		uploaded := 0
		for _, it := range checklist {
			m, _ := it.(map[string]any)
			switch strings.ToUpper(firstString(m, "status")) {
			case "APPROVED":
				approved++
			case "REJECTED":
				rejected++
			case "UPLOADED":
				uploaded++
			default:
				pending++
			}
		}
		if len(items) > 0 {
			if pending > 0 {
				items[0].Badges = append(items[0].Badges, fmt.Sprintf("%d pendente(s)", pending))
			}
			if rejected > 0 {
				items[0].Badges = append(items[0].Badges, fmt.Sprintf("%d reprovado(s)", rejected))
			}
			if approved > 0 {
				items[0].Badges = append(items[0].Badges, fmt.Sprintf("%d aprovado(s)", approved))
			}
			if uploaded > 0 {
				items[0].Badges = append(items[0].Badges, fmt.Sprintf("%d enviado(s)", uploaded))
			}
		}

		for i, it := range checklist {
			if len(items) >= 10 {
				break
			}
			m, _ := it.(map[string]any)
			itemID := firstString(m, "id")
			name := firstString(m, "name")
			category := firstString(m, "category")
			st := firstString(m, "status")
			required := boolFromAny(m["required"])
			solicited := boolFromAny(m["solicited"])
			fileName := firstString(m, "file_name")
			reason := firstString(m, "rejection_reason")
			uploadedAt := firstString(m, "uploaded_at")
			solicitedAt := firstString(m, "solicited_at")

			b := make([]string, 0, 4)
			if category != "" {
				b = append(b, humanizeChecklistCategory(category))
			}
			if st != "" {
				b = append(b, humanizeChecklistStatus(st))
			}
			if required {
				b = append(b, "Obrigatório")
			}
			if solicited {
				b = append(b, "Solicitado")
			}

			meta := make([]suiteeCardMetadata, 0, 1)
			if uploadedAt != "" {
				meta = append(meta, suiteeCardMetadata{Kind: "date", Value: uploadedAt})
			} else if solicitedAt != "" {
				meta = append(meta, suiteeCardMetadata{Kind: "date", Value: solicitedAt})
			}

			det := make([]suiteeCardDetail, 0, 3)
			if fileName != "" {
				det = append(det, suiteeCardDetail{Label: "Arquivo", Value: fileName})
			}
			if reason != "" {
				det = append(det, suiteeCardDetail{Label: "Motivo", Value: reason})
			}

			title := strings.TrimSpace(name)
			if title == "" {
				title = "Item " + fmt.Sprintf("%d", i+1)
			}
			items = append(items, suiteeCardItem{
				Key:          itemID,
				Title:        title,
				Subtitle:     strings.TrimSpace(humanizeChecklistCategory(category)),
				Badges:       b,
				StatusColor:  checklistStatusColor(st),
				OpenPath:     "/esteira/deals/" + dealID,
				Copy:         title + " — " + strings.TrimSpace(humanizeChecklistStatus(st)),
				ActionPrompt: "Sugira o próximo passo objetivo para este item do checklist (" + title + ") considerando o status atual.",
				Metadata:     meta,
				Details:      det,
			})
		}
	}

	titleBlock := "Deal"
	if len(checklist) > 0 {
		titleBlock = "Deal e checklist"
	}
	if len(items) == 0 {
		items = append(items, suiteeCardItem{
			Title: "Deal encontrado",
			Badges: []string{
				strings.TrimSpace(status),
			},
		})
	}
	return suiteeCardsBlock{Title: titleBlock, Items: items}
}
