package service

import (
	"context"
	"errors"
	"reflect"

	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

var ErrVisitConflict = errors.New("Os dados desta visita foram alterados. Atualize a página antes de tentar novamente.")

func visitSnapshot(l *lead.Lead) *lead.Lead {
	copy := *l
	if l.VisitRequest != nil {
		visit := *l.VisitRequest
		copy.VisitRequest = &visit
	}
	return &copy
}

// persistVisit never writes a stale whole-lead snapshot after an upstream
// request. Retry unrelated lead edits, but do not overwrite a changed visit
// or its customer/broker assignment. Patch uses seq and timestamp guards,
// including the timestamp-only fallback on older databases.
func (s *LeadService) persistVisit(ctx context.Context, before *lead.Lead, next *lead.VisitRequest) error {
	current := before
	for attempt := 0; attempt < 3; attempt++ {
		seq, updatedAt := current.UpdatedSeq, current.UpdatedAt
		expected := &lead.LeadExpectedVersion{UpdatedSeq: &seq, UpdatedAt: &updatedAt}
		_, saved, err := s.repo.Patch(ctx, before.ID, before.TenantID, expected, map[string]interface{}{"visit_request": next})
		if err != nil {
			return err
		}
		if saved {
			return nil
		}
		current, err = s.repo.GetByID(ctx, before.TenantID, before.ID)
		if err != nil {
			return err
		}
		if current == nil {
			return ErrVisitNotFound
		}
		if current.TenantID != before.TenantID || current.AccountID != before.AccountID || current.AssignedBrokerID != before.AssignedBrokerID {
			return ErrVisitConflict
		}
		if reflect.DeepEqual(current.VisitRequest, next) {
			return nil
		}
		if !reflect.DeepEqual(current.VisitRequest, before.VisitRequest) {
			return ErrVisitConflict
		}
	}
	return ErrVisitConflict
}
