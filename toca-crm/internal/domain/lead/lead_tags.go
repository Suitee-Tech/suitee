package lead

import (
	"strings"
)

func (l *Lead) DeriveTags() []string {
	if l == nil {
		return []string{}
	}

	tags := make([]string, 0, 8)
	seen := map[string]bool{}

	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		v = strings.ToUpper(v)
		if seen[v] {
			return
		}
		seen[v] = true
		tags = append(tags, v)
	}

	add(string(l.Source))
	add(string(l.Role))
	switch strings.ToLower(string(l.Role)) {
	case "buyer":
		add("COMPRA_VENDA")
		add("COMPRA E VENDA")
	case "tenant":
		add("LOCACAO")
		add("LOCAÇÃO")
	case "owner":
		add("CAPTACAO")
		add("CAPTAÇÃO")
	}

	if l.PropertyInterest != nil {
		add(string(l.PropertyInterest.Type))

		priceVal := 0.0
		if l.PropertyInterest.PriceRange.Max > 0 {
			priceVal = l.PropertyInterest.PriceRange.Max
		} else if l.PropertyInterest.PriceRange.Min > 0 {
			priceVal = l.PropertyInterest.PriceRange.Min
		}

		if priceVal > 0 {
			switch {
			case priceVal < 300_000:
				add("LOW_PRICE")
			case priceVal < 800_000:
				add("MID_PRICE")
			case priceVal < 2_000_000:
				add("HIGH_PRICE")
			default:
				add("PREMIUM_PRICE")
			}
		}

		loc := strings.TrimSpace(l.PropertyInterest.PreferredLocation)
		if loc != "" {
			parts := strings.FieldsFunc(loc, func(r rune) bool {
				switch r {
				case ',', '-', '/':
					return true
				default:
					return false
				}
			})
			for _, p := range parts {
				add(p)
			}
		}
	}

	for i := range tags {
		tags[i] = strings.TrimSpace(tags[i])
	}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		if t != "" {
			out = append(out, t)
		}
	}

	return out
}
