package service

import (
	"context"
	"testing"

	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-comm/internal/domain/notification"
)

// ctxCapturingRepo registra o contexto recebido para verificar a propagação
// de tenant — o pool de aplicação semeia request.jwt.claims a partir do ctx
// e a RLS de comm.inapp_notifications exige o tenant do JWT (fix do SQLSTATE
// 42501 em consumidores de eventos).
type ctxCapturingRepo struct {
	lastCtx context.Context
}

func (r *ctxCapturingRepo) Create(ctx context.Context, n *notification.InAppNotification) error {
	r.lastCtx = ctx
	return nil
}

func (r *ctxCapturingRepo) CreateIfNotExists(ctx context.Context, n *notification.InAppNotification) (bool, error) {
	r.lastCtx = ctx
	return true, nil
}

func (r *ctxCapturingRepo) List(context.Context, string, string, int, int) ([]*notification.InAppNotification, int, error) {
	return nil, 0, nil
}

func (r *ctxCapturingRepo) GetUnreadCount(context.Context, string, string) (int, error) {
	return 0, nil
}

func (r *ctxCapturingRepo) MarkAsRead(context.Context, string, string, string) error {
	return nil
}

func (r *ctxCapturingRepo) MarkAllAsRead(context.Context, string, string) error {
	return nil
}

func TestInAppNotificationService_PropagatesTenantToRepoContext(t *testing.T) {
	repo := &ctxCapturingRepo{}
	svc := NewInAppNotificationService(repo, nil)

	// Chamador sem tenant no ctx (consumidor de evento): o serviço injeta.
	background := context.Background()
	if err := svc.Create(background, "t-1", "u-1", notification.InAppTypeSystem, notification.PriorityNormal, "título", "corpo", "", nil); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got, _ := pkgcontext.GetTenantID(repo.lastCtx); got != "t-1" {
		t.Fatalf("repo ctx tenant = %q, want t-1 (RLS precisa das claims)", got)
	}

	// Caminho idempotente idem.
	if _, err := svc.CreateIdempotent(background, "id-1", "t-2", "u-2", notification.InAppTypeSystem, notification.PriorityNormal, "título", "corpo", "", nil); err != nil {
		t.Fatalf("CreateIdempotent: %v", err)
	}
	if got, _ := pkgcontext.GetTenantID(repo.lastCtx); got != "t-2" {
		t.Fatalf("repo ctx tenant = %q, want t-2", got)
	}

	// Chamador que JÁ traz tenant (HTTP): preserva o mesmo valor.
	httpCtx := pkgcontext.WithTenantID(context.Background(), "t-3")
	if err := svc.Create(httpCtx, "t-3", "u-3", notification.InAppTypeSystem, notification.PriorityNormal, "título", "corpo", "", nil); err != nil {
		t.Fatalf("Create (http ctx): %v", err)
	}
	if got, _ := pkgcontext.GetTenantID(repo.lastCtx); got != "t-3" {
		t.Fatalf("repo ctx tenant = %q, want t-3", got)
	}
}
