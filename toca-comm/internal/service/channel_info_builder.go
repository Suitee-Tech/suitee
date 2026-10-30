package service

import (
	"fmt"

	"github.com/oginoino/toca/pkg/money"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
)

// DealPayload is the subset of DealCreatedEvent fields needed to build
// channel info. Keeping it decoupled from the event bus allows unit testing
// without importing the full event infrastructure.
type DealPayload struct {
	DealID     string
	TenantID   string
	PropertyID string
	Title      string
	DealType   string
	DealValue  float64
	BrokerID   string
	BrokerName string
	Parties    []PartyPayload
}

// PartyPayload mirrors PartyInfo from the deal event.
type PartyPayload struct {
	Name   string
	Role   string
	Type   string
	LeadID string
}

// ErrMissingPropertyID is returned when the deal has no PropertyID set.
var ErrMissingPropertyID = fmt.Errorf("deal payload is missing property_id")

// BuildChannelInfo constructs the initial ChannelInfo sections from deal
// context. It validates required fields and returns rich, labelled sections:
//
//   - "Negócio" – deal type, value, link to deal page
//   - "Imóvel"    – property reference with link
//   - "Partes"    – parties with names and roles
//
// Links point to the relative frontend routes so they work in any
// environment (localhost, production) without base-URL configuration.
//
// Returns ErrMissingPropertyID when PropertyID is empty.
// Missing optional fields (deal type, parties, broker name) are gracefully
// skipped — they produce empty or omitted sections rather than errors.
func BuildChannelInfo(p DealPayload) (*message.ChannelInfo, error) {
	if p.PropertyID == "" {
		return nil, ErrMissingPropertyID
	}

	sections := make([]message.ChannelInfoSection, 0, 3)

	// 1. Negócio (deal metadata + link to deal page)
	dealItems := []message.ChannelInfoItem{
		{Label: "ID", Value: p.Title, Link: "/esteira/deals/" + p.DealID},
	}
	if p.DealType != "" {
		dealItems = append(dealItems, message.ChannelInfoItem{
			Label: "Tipo", Value: translateDealType(p.DealType),
		})
	}
	if p.DealValue > 0 {
		dealItems = append(dealItems, message.ChannelInfoItem{
			Label: "Valor", Value: money.BRL(p.DealValue),
		})
	}
	sections = append(sections, message.ChannelInfoSection{
		Label: "Negócio",
		Items: dealItems,
	})

	// 2. Imóvel (property reference with link)
	propItems := []message.ChannelInfoItem{
		{Label: "Imóvel", Value: p.PropertyID, Link: "/imoveis/" + p.PropertyID},
	}
	sections = append(sections, message.ChannelInfoSection{
		Label: "Imóvel",
		Items: propItems,
	})

	// 3. Partes (parties involved)
	partyItems := make([]message.ChannelInfoItem, 0, len(p.Parties))
	for _, party := range p.Parties {
		if party.Name == "" {
			continue
		}
		desc := translatePartyRole(party.Role)
		if party.Type != "" {
			t := translatePartyType(party.Type)
			if desc != "" {
				desc = desc + " · " + t
			} else {
				desc = t
			}
		}
		item := message.ChannelInfoItem{
			Label: party.Name,
			Value: desc,
		}
		if party.LeadID != "" {
			item.Link = "/leads/" + party.LeadID
		}
		partyItems = append(partyItems, item)
	}
	if len(partyItems) > 0 {
		sections = append(sections, message.ChannelInfoSection{
			Label: "Partes",
			Items: partyItems,
		})
	}

	return &message.ChannelInfo{Sections: sections}, nil
}

// translateDealType maps internal deal type constants to Portuguese labels.
func translateDealType(t string) string {
	switch t {
	case "SALE":
		return "Venda"
	case "RENT":
		return "Locação"
	default:
		return t
	}
}

// translatePartyRole maps internal party role constants to Portuguese labels.
func translatePartyRole(r string) string {
	switch r {
	case "buyer":
		return "Comprador"
	case "seller":
		return "Vendedor"
	case "owner":
		return "Proprietário"
	case "tenant":
		return "Locatário"
	case "landlord":
		return "Locador"
	case "guarantor":
		return "Fiador"
	case "witness":
		return "Testemunha"
	case "broker":
		return "Corretor"
	default:
		return r
	}
}

// translatePartyType maps internal party kind constants to Portuguese labels.
func translatePartyType(k string) string {
	switch k {
	case "cliente":
		return "Cliente"
	case "proprietario":
		return "Proprietário"
	case "locador":
		return "Locador"
	case "locatario":
		return "Locatário"
	case "fiador":
		return "Fiador"
	case "parceiro":
		return "Parceiro"
	case "corretor":
		return "Corretor"
	default:
		return k
	}
}
