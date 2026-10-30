package postgres

import (
	"errors"
	"time"

	"github.com/oginoino/toca/toca-sites/internal/domain/site"
)

// Fixtures mínimos para exercitar o fail-closed sem Postgres. Os métodos
// precisam recusar ANTES de tocar no banco (db nil), então os campos não
// importam além do tenant vazio.

func siteListingFixture() site.Listing {
	return site.Listing{}
}

func siteLeadFixture() site.Lead {
	return site.Lead{}
}

func siteViewFixture() site.ListingView {
	return site.ListingView{}
}

func nowFixture() time.Time { return time.Unix(1700000000, 0).UTC() }

func errText(s string) error { return errors.New(s) }
