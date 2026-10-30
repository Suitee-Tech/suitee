package services

import (
	"encoding/json"
	"strings"
	"unicode"
)

type suiteeCardItem struct {
	Key          string               `json:"key,omitempty"`
	Title        string               `json:"title"`
	Subtitle     string               `json:"subtitle,omitempty"`
	Badges       []string             `json:"badges,omitempty"`
	StatusColor  string               `json:"status_color,omitempty"`
	OpenPath     string               `json:"open_path,omitempty"`
	Copy         string               `json:"copy,omitempty"`
	ActionPrompt string               `json:"action_prompt,omitempty"`
	Metadata     []suiteeCardMetadata `json:"metadata,omitempty"`
	Details      []suiteeCardDetail   `json:"details,omitempty"`
}

type suiteeCardMetadata struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type suiteeCardDetail struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type suiteeCardsBlock struct {
	Title string           `json:"title,omitempty"`
	Items []suiteeCardItem `json:"items"`
}

func statusColorFromStatus(status string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	if s == "" {
		return ""
	}
	if strings.Contains(s, "cancel") || strings.Contains(s, "cancell") || strings.Contains(s, "perd") || strings.Contains(s, "lost") || strings.Contains(s, "reprov") || strings.Contains(s, "rejected") {
		return "red"
	}
	if strings.Contains(s, "sold") || strings.Contains(s, "vend") || strings.Contains(s, "rented") || strings.Contains(s, "alug") || strings.Contains(s, "inactive") || strings.Contains(s, "inativ") {
		return "gray"
	}
	if strings.Contains(s, "analysis") || strings.Contains(s, "analis") || strings.Contains(s, "proposal") || strings.Contains(s, "propos") || strings.Contains(s, "contract") || strings.Contains(s, "contrat") || strings.Contains(s, "negotiat") || strings.Contains(s, "negocia") {
		return "yellow"
	}
	if strings.Contains(s, "novo") || strings.Contains(s, "new") {
		return "yellow"
	}
	if strings.Contains(s, "available") || strings.Contains(s, "dispon") || strings.Contains(s, "published") || strings.Contains(s, "public") || strings.Contains(s, "contato") || strings.Contains(s, "agend") || strings.Contains(s, "convert") || strings.Contains(s, "won") {
		return "green"
	}
	return "gray"
}

func nestedString(m map[string]any, keys ...string) string {
	var cur any = m
	for _, k := range keys {
		obj, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = obj[k]
	}
	if s, ok := cur.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func nestedNumber(m map[string]any, keys ...string) float64 {
	var cur any = m
	for _, k := range keys {
		obj, ok := cur.(map[string]any)
		if !ok {
			return 0
		}
		cur = obj[k]
	}
	switch t := cur.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	default:
		return 0
	}
}

func mustSuiteeCards(block suiteeCardsBlock) string {
	b, err := json.Marshal(block)
	if err != nil {
		return "Não consegui montar o resultado visual. Tente novamente."
	}
	return "```suitee_cards\n" + string(b) + "\n```"
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func intFromAny(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case string:
		n := 0
		for _, r := range t {
			if unicode.IsDigit(r) {
				n = n*10 + int(r-'0')
				continue
			}
			break
		}
		return n
	default:
		return 0
	}
}
