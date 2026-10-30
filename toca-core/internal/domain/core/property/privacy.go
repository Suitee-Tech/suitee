package property

import (
	"strconv"

	"github.com/oginoino/toca-core/internal/domain/core/address"
)

// ProjectForPartner returns a privacy-projected COPY of a property suitable
// for exposing to a PARTNER company through the shared portfolio. The original
// is never mutated.
//
// The projection always:
//   - clears interested-leads, the intermediation contract ref, and the
//     capturing brokers (all PII / internal data of the owner company);
//   - coarsens the address — neighborhood/city/state are kept, but the exact
//     street, number, complement, postal code, reference point and formatted
//     address are dropped, and the lat/long are rounded to ~2 decimals
//     (≈ neighborhood precision) so the partner cannot pinpoint the property.
//
// When shareOwnerContact is false the owner identity (legacy Owner + Owners)
// is also cleared. When true the owner data is kept — but the address is
// STILL coarsened, because location privacy is independent of owner-contact
// sharing.
func ProjectForPartner(p *Property, shareOwnerContact bool) *Property {
	if p == nil {
		return nil
	}

	// Shallow copy. Slices/structs are reassigned explicitly below, so the
	// caller's value is never mutated through the returned pointer.
	cp := *p

	// PII / internal data — always stripped.
	cp.InterestedLeads = nil
	cp.IntermediationContractID = ""
	cp.CapturingBrokers = nil

	if !shareOwnerContact {
		cp.Owner = OwnerRef{}
		cp.Owners = nil
	}

	cp.Address = coarsenAddress(p.Address)

	return &cp
}

// coarsenAddress drops the precise location fields and rounds the
// coordinates, keeping only neighborhood-level granularity.
func coarsenAddress(a address.Address) address.Address {
	a.Street = ""
	a.Number = ""
	a.Complement = ""
	a.PostalCode = ""
	a.FormattedAddress = ""
	a.ReferencePoint = ""
	a.Latitude = coarsenCoordinate(a.Latitude)
	a.Longitude = coarsenCoordinate(a.Longitude)
	return a
}

// coarsenCoordinate rounds a decimal coordinate string to 2 decimal places
// (~1.1 km precision). Non-numeric or empty values are dropped entirely.
func coarsenCoordinate(v string) string {
	if v == "" {
		return ""
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return ""
	}
	return strconv.FormatFloat(roundTo(f, 2), 'f', 2, 64)
}

func roundTo(v float64, places int) float64 {
	pow := 1.0
	for i := 0; i < places; i++ {
		pow *= 10
	}
	if v >= 0 {
		return float64(int64(v*pow+0.5)) / pow
	}
	return float64(int64(v*pow-0.5)) / pow
}
