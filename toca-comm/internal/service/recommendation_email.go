package service

import (
	"fmt"

	"github.com/oginoino/toca/pkg/money"
)

// Copy dos e-mails transacionais de recomendação do corretor
// (beta F3 — "Sugestão do seu corretor").

func recommendationSubject(title string) string {
	if title == "" {
		return "Sugestão do seu corretor"
	}
	return "Sugestão do seu corretor: " + title
}

// recommendationInAppContent é o corpo curto da notificação in-app (sino do
// portal): título do imóvel + localização.
func recommendationInAppContent(title, neighborhood, city string) string {
	displayTitle := title
	if displayTitle == "" {
		displayTitle = "Um imóvel foi recomendado"
	}
	if loc := recommendationLocationLine(neighborhood, city, ""); loc != "" {
		return displayTitle + " · " + loc
	}
	return displayTitle
}

// recommendationPriceLine formata o preço exibido: locação > venda quando os
// dois existem; "sob consulta" quando nenhum.
func recommendationPriceLine(salePrice, rentPrice float64) string {
	if rentPrice > 0 {
		return fmt.Sprintf("Aluguel: <strong>%s</strong>", money.BRL(rentPrice))
	}
	if salePrice > 0 {
		return fmt.Sprintf("Valor: <strong>%s</strong>", money.BRL(salePrice))
	}
	return "Valor: sob consulta"
}

func recommendationLocationLine(neighborhood, city, state string) string {
	parts := []string{}
	if neighborhood != "" {
		parts = append(parts, neighborhood)
	}
	if city != "" {
		parts = append(parts, city)
	}
	if state != "" {
		parts = append(parts, state)
	}
	if len(parts) == 0 {
		return ""
	}
	line := parts[0]
	for _, p := range parts[1:] {
		line += " · " + p
	}
	return line
}

func recommendationClientBody(title, neighborhood, city, state string, salePrice, rentPrice float64) string {
	displayTitle := title
	if displayTitle == "" {
		displayTitle = "um imóvel"
	}
	lines := fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Seu corretor indicou <strong>%s</strong> para você.</p>", displayTitle)
	if loc := recommendationLocationLine(neighborhood, city, state); loc != "" {
		lines += fmt.Sprintf("<p style='margin: 0 0 8px 0; font-size: 14px; color: #6b7280;'>%s</p>", loc)
	}
	lines += fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 14px; color: #6b7280;'>%s</p>", recommendationPriceLine(salePrice, rentPrice))
	return lines
}
