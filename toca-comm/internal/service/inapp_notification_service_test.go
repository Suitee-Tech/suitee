package service

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca/toca-comm/internal/domain/notification"
)

// mockInAppRepo is an in-memory implementation of repository.InAppNotificationRepository
// used to verify InAppNotificationService logic without touching Postgres.
type mockInAppRepo struct {
	notifications []*notification.InAppNotification
	createErr     error
	listErr       error
	unreadErr     error
	markErr       error
	markAllErr    error

	listLimit  int
	listOffset int
}

func (m *mockInAppRepo) Create(ctx context.Context, n *notification.InAppNotification) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.notifications = append(m.notifications, n)
	return nil
}

func (m *mockInAppRepo) CreateIfNotExists(ctx context.Context, n *notification.InAppNotification) (bool, error) {
	if m.createErr != nil {
		return false, m.createErr
	}
	for _, existing := range m.notifications {
		if existing.ID == n.ID {
			return false, nil
		}
	}
	m.notifications = append(m.notifications, n)
	return true, nil
}

func (m *mockInAppRepo) List(ctx context.Context, tenantID, userID string, limit, offset int) ([]*notification.InAppNotification, int, error) {
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	m.listLimit = limit
	m.listOffset = offset
	out := make([]*notification.InAppNotification, 0, len(m.notifications))
	for _, n := range m.notifications {
		if n.TenantID == tenantID && n.UserID == userID {
			out = append(out, n)
		}
	}
	return out, len(out), nil
}

func (m *mockInAppRepo) GetUnreadCount(ctx context.Context, tenantID, userID string) (int, error) {
	if m.unreadErr != nil {
		return 0, m.unreadErr
	}
	count := 0
	for _, n := range m.notifications {
		if n.TenantID == tenantID && n.UserID == userID && !n.IsRead {
			count++
		}
	}
	return count, nil
}

func (m *mockInAppRepo) MarkAsRead(ctx context.Context, id, tenantID, userID string) error {
	if m.markErr != nil {
		return m.markErr
	}
	for _, n := range m.notifications {
		if n.ID == id && n.TenantID == tenantID && n.UserID == userID {
			n.IsRead = true
			return nil
		}
	}
	return nil
}

func (m *mockInAppRepo) MarkAllAsRead(ctx context.Context, tenantID, userID string) error {
	if m.markAllErr != nil {
		return m.markAllErr
	}
	for _, n := range m.notifications {
		if n.TenantID == tenantID && n.UserID == userID {
			n.IsRead = true
		}
	}
	return nil
}

func TestInAppNotificationService_Create_PersistsAndBroadcasts(t *testing.T) {
	repo := &mockInAppRepo{}
	// Hub with no registered clients — BroadcastEvent should be a no-op but
	// must not panic. This is the critical assertion: the prod event consumer
	// always calls Create with a hub attached, so we exercise that path.
	svc := NewInAppNotificationService(repo, NewHub())

	err := svc.Create(context.Background(), "tenant-1", "user-1",
		notification.InAppTypeSystem, notification.PriorityImportant,
		"Nova Proposta", "Proposta recebida", "/portal/deals/123",
		map[string]interface{}{"deal_id": "123"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if len(repo.notifications) != 1 {
		t.Fatalf("expected 1 notification persisted, got %d", len(repo.notifications))
	}
	got := repo.notifications[0]
	if got.TenantID != "tenant-1" || got.UserID != "user-1" {
		t.Errorf("tenant/user mismatch: %q/%q", got.TenantID, got.UserID)
	}
	if got.Type != notification.InAppTypeSystem {
		t.Errorf("type = %v, want %v", got.Type, notification.InAppTypeSystem)
	}
	if got.Priority != notification.PriorityImportant {
		t.Errorf("priority = %v, want %v", got.Priority, notification.PriorityImportant)
	}
	if got.IsRead {
		t.Error("new notification should be unread")
	}
	if got.ID == "" {
		t.Error("ID should be populated by NewInApp")
	}
}

func TestInAppNotificationService_Create_NoHub(t *testing.T) {
	repo := &mockInAppRepo{}
	svc := NewInAppNotificationService(repo, nil)

	// hub nil — must still persist without panicking
	if err := svc.Create(context.Background(), "t1", "u1",
		notification.InAppTypeSystem, notification.PriorityNormal,
		"Title", "Body", "", nil); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if len(repo.notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(repo.notifications))
	}
}

func TestInAppNotificationService_Create_PropagatesRepoError(t *testing.T) {
	wantErr := errors.New("db down")
	repo := &mockInAppRepo{createErr: wantErr}
	svc := NewInAppNotificationService(repo, NewHub())

	err := svc.Create(context.Background(), "t1", "u1",
		notification.InAppTypeSystem, notification.PriorityNormal,
		"x", "y", "", nil)
	if !errors.Is(err, wantErr) {
		t.Errorf("expected %v, got %v", wantErr, err)
	}
}

func TestInAppNotificationService_List_FiltersByTenantAndUser(t *testing.T) {
	repo := &mockInAppRepo{
		notifications: []*notification.InAppNotification{
			{ID: "1", TenantID: "t1", UserID: "u1"},
			{ID: "2", TenantID: "t1", UserID: "u2"},
			{ID: "3", TenantID: "t2", UserID: "u1"},
			{ID: "4", TenantID: "t1", UserID: "u1"},
		},
	}
	svc := NewInAppNotificationService(repo, nil)

	items, total, err := svc.List(context.Background(), "t1", "u1", 10, 0)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Errorf("expected 2 results for t1/u1, got total=%d len=%d", total, len(items))
	}
	if repo.listLimit != 10 || repo.listOffset != 0 {
		t.Errorf("limit/offset not forwarded: limit=%d offset=%d", repo.listLimit, repo.listOffset)
	}
}

func TestInAppNotificationService_GetUnreadCount(t *testing.T) {
	repo := &mockInAppRepo{
		notifications: []*notification.InAppNotification{
			{TenantID: "t1", UserID: "u1", IsRead: false},
			{TenantID: "t1", UserID: "u1", IsRead: true},
			{TenantID: "t1", UserID: "u1", IsRead: false},
			{TenantID: "t1", UserID: "u2", IsRead: false},
		},
	}
	svc := NewInAppNotificationService(repo, nil)

	count, err := svc.GetUnreadCount(context.Background(), "t1", "u1")
	if err != nil {
		t.Fatalf("GetUnreadCount error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 unread for t1/u1, got %d", count)
	}
}

func TestInAppNotificationService_MarkAsRead(t *testing.T) {
	target := &notification.InAppNotification{ID: "n-1", TenantID: "t1", UserID: "u1", IsRead: false}
	repo := &mockInAppRepo{notifications: []*notification.InAppNotification{target}}
	svc := NewInAppNotificationService(repo, nil)

	if err := svc.MarkAsRead(context.Background(), "n-1", "t1", "u1"); err != nil {
		t.Fatalf("MarkAsRead error: %v", err)
	}
	if !target.IsRead {
		t.Error("notification should be marked as read")
	}
}

func TestInAppNotificationService_MarkAllAsRead(t *testing.T) {
	a := &notification.InAppNotification{TenantID: "t1", UserID: "u1", IsRead: false}
	b := &notification.InAppNotification{TenantID: "t1", UserID: "u1", IsRead: false}
	other := &notification.InAppNotification{TenantID: "t1", UserID: "u2", IsRead: false}
	repo := &mockInAppRepo{notifications: []*notification.InAppNotification{a, b, other}}
	svc := NewInAppNotificationService(repo, nil)

	if err := svc.MarkAllAsRead(context.Background(), "t1", "u1"); err != nil {
		t.Fatalf("MarkAllAsRead error: %v", err)
	}
	if !a.IsRead || !b.IsRead {
		t.Error("u1 notifications should be marked as read")
	}
	if other.IsRead {
		t.Error("u2 notification should NOT be marked as read")
	}
}
