package service

import (
	"context"

	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-comm/internal/domain/notification"
	"github.com/oginoino/toca/toca-comm/internal/repository"
)

type InAppNotificationService struct {
	repo repository.InAppNotificationRepository
	hub  *Hub
}

func NewInAppNotificationService(repo repository.InAppNotificationRepository, hub *Hub) *InAppNotificationService {
	return &InAppNotificationService{
		repo: repo,
		hub:  hub,
	}
}

// withTenantContext propaga o tenant para o contexto da conexão. O pool de
// aplicação semeia `request.jwt.claims` a partir do ctx no checkout
// (applyJWTClaimsHook), e a RLS de comm.inapp_notifications valida por
// auth.jwt() — consumidores de eventos rodam SEM JWT e vinham gravando com
// claims vazias (SQLSTATE 42501, lead.assigned indo para DLQ). O tenant já é
// parâmetro explícito validado pelo chamador; propagar é correto e idempotente
// para o caminho HTTP (mesmo valor do middleware).
func withTenantContext(ctx context.Context, tenantID string) context.Context {
	if tenantID == "" {
		return ctx
	}
	return pkgcontext.WithTenantID(ctx, tenantID)
}

func (s *InAppNotificationService) Create(ctx context.Context, tenantID, userID string, notifType notification.InAppType, priority notification.Priority, title, content, link string, data map[string]interface{}) error {
	n := notification.NewInApp(tenantID, userID, notifType, priority, title, content, link, data)
	err := s.repo.Create(withTenantContext(ctx, tenantID), n)
	if err != nil {
		return err
	}

	if s.hub != nil {
		s.hub.BroadcastEvent(tenantID, []string{userID}, "inapp_notification", n)
	}

	return nil
}

// CreateIdempotent inserts an in-app notification using a caller-supplied
// deterministic id (typically derived via events.DeriveNotificationID from
// the source event). When the id already exists, the insert is a no-op and
// the hub broadcast is skipped — so handler retries / re-emissions of the
// same logical event produce a single notification and a single websocket
// push instead of duplicates.
//
// Returns true when a new row was inserted, false when the id was already
// present (the caller can use this signal to skip downstream side effects
// like external messaging).
func (s *InAppNotificationService) CreateIdempotent(ctx context.Context, id, tenantID, userID string, notifType notification.InAppType, priority notification.Priority, title, content, link string, data map[string]interface{}) (bool, error) {
	n := notification.NewInApp(tenantID, userID, notifType, priority, title, content, link, data)
	if id != "" {
		n.ID = id
	}
	created, err := s.repo.CreateIfNotExists(withTenantContext(ctx, tenantID), n)
	if err != nil {
		return false, err
	}
	if !created {
		return false, nil
	}
	if s.hub != nil {
		s.hub.BroadcastEvent(tenantID, []string{userID}, "inapp_notification", n)
	}
	return true, nil
}

func (s *InAppNotificationService) List(ctx context.Context, tenantID, userID string, limit, offset int) ([]*notification.InAppNotification, int, error) {
	return s.repo.List(ctx, tenantID, userID, limit, offset)
}

func (s *InAppNotificationService) GetUnreadCount(ctx context.Context, tenantID, userID string) (int, error) {
	return s.repo.GetUnreadCount(ctx, tenantID, userID)
}

func (s *InAppNotificationService) MarkAsRead(ctx context.Context, id, tenantID, userID string) error {
	return s.repo.MarkAsRead(ctx, id, tenantID, userID)
}

func (s *InAppNotificationService) MarkAllAsRead(ctx context.Context, tenantID, userID string) error {
	return s.repo.MarkAllAsRead(ctx, tenantID, userID)
}
