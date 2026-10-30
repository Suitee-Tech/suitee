package handlers

import "testing"

// A vitrine do cliente sem negócio (`/client-portal/properties`) precisa ir para
// o toca-crm: é ele quem conhece os leads do cliente (e, por eles, as
// imobiliárias cujo portfólio pode ser mostrado). Escolher o serviço errado não
// dá erro de compilação — dá 404 na tela do cliente, então a decisão é testada.
func TestClientPortalServiceRouting(t *testing.T) {
	cases := []struct {
		path string
		want clientPortalServiceName
	}{
		// Vitrine e overview do lead → CRM.
		{"/properties", clientPortalCRM},
		{"/properties?listing_type=sale", clientPortalCRM},
		{"/overview", clientPortalCRM},
		{"/leads/lead-1/visit-request", clientPortalCRM},
		{"/leads/lead-1/visit", clientPortalCRM},
		{"/leads/lead-1/visit/calendar.ics", clientPortalCRM},
		{"/leads/lead-1/channel", clientPortalCRM},
		// Financeiro do cliente.
		{"/charges", clientPortalFinancial},
		{"/transfers", clientPortalFinancial},
		{"/rentals", clientPortalFinancial},
		{"/informes", clientPortalFinancial},
		{"/owner-statements", clientPortalFinancial},
		// Negócio (inclui a busca DEAL-scoped, que é outro caminho).
		{"/deals", clientPortalDeal},
		{"/deals/abc/properties", clientPortalDeal},
		{"/deals/abc/checklist", clientPortalDeal},
		{"/tickets/xyz", clientPortalDeal},
	}
	for _, tc := range cases {
		if got := clientPortalService(tc.path); got != tc.want {
			t.Fatalf("path %q → %q, want %q", tc.path, got, tc.want)
		}
	}
}
