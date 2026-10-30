package services

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-ops/internal/models/ticket"
)

type ticketAttachmentStorage struct{}

func (ticketAttachmentStorage) Upload(context.Context, string, string, string, io.Reader) (string, error) {
	return "", nil
}

func (ticketAttachmentStorage) Delete(context.Context, string, string) error { return nil }
func (ticketAttachmentStorage) GetURL(string, string) string                 { return "" }
func (ticketAttachmentStorage) GetPresignedURL(_ context.Context, bucket, key string, _ time.Duration) (string, error) {
	return "https://storage.example/" + bucket + "/" + key + "?signed=1", nil
}
func (ticketAttachmentStorage) GetPresignedPutURL(context.Context, string, string, time.Duration) (string, error) {
	return "", nil
}
func (ticketAttachmentStorage) IsPrivate(string) bool { return true }

// fakeTicketRepo is an in-memory ticket.Repository for unit-testing TicketService.
type fakeTicketRepo struct {
	mu      sync.Mutex
	tickets map[string]*ticket.Ticket
	failOn  string // operation name to fail on, for error-path tests
}

func newFakeTicketRepo() *fakeTicketRepo {
	return &fakeTicketRepo{tickets: map[string]*ticket.Ticket{}}
}

func (f *fakeTicketRepo) Create(ctx context.Context, t *ticket.Ticket) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failOn == "Create" {
		return errors.New("create failed")
	}
	f.tickets[t.ID] = t
	return nil
}

func (f *fakeTicketRepo) GetByID(ctx context.Context, tenantID, id string) (*ticket.Ticket, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failOn == "GetByID" {
		return nil, errors.New("get failed")
	}
	t, ok := f.tickets[id]
	if !ok || t.TenantID != tenantID {
		return nil, nil
	}
	return t, nil
}

func (f *fakeTicketRepo) Update(ctx context.Context, t *ticket.Ticket) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failOn == "Update" {
		return errors.New("update failed")
	}
	f.tickets[t.ID] = t
	return nil
}

func (f *fakeTicketRepo) List(ctx context.Context, tenantID string, filters ticket.ListFilters) ([]*ticket.Ticket, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*ticket.Ticket{}
	for _, t := range f.tickets {
		if t.TenantID == tenantID {
			out = append(out, t)
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeTicketRepo) ListByRequester(ctx context.Context, tenantID, accountID string) ([]*ticket.Ticket, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*ticket.Ticket{}
	for _, t := range f.tickets {
		if t.TenantID == tenantID && t.RequesterAccountID == accountID {
			out = append(out, t)
		}
	}
	return out, nil
}

func TestTicketService_Create_RequiresTenantID(t *testing.T) {
	svc := NewTicketService(newFakeTicketRepo(), nil)
	_, err := svc.Create(context.Background(), CreateTicketRequest{
		Subject:            "x",
		RequesterAccountID: "u1",
	})
	if err == nil || !strings.Contains(err.Error(), "tenant_id") {
		t.Errorf("expected tenant_id error, got %v", err)
	}
}

func TestTicketService_Create_RequiresRequester(t *testing.T) {
	svc := NewTicketService(newFakeTicketRepo(), nil)
	_, err := svc.Create(context.Background(), CreateTicketRequest{
		TenantID: "t1",
		Subject:  "x",
	})
	if err == nil || !strings.Contains(err.Error(), "requester_account_id") {
		t.Errorf("expected requester_account_id error, got %v", err)
	}
}

func TestTicketService_Create_RequiresSubject(t *testing.T) {
	svc := NewTicketService(newFakeTicketRepo(), nil)
	_, err := svc.Create(context.Background(), CreateTicketRequest{
		TenantID:           "t1",
		RequesterAccountID: "u1",
		Subject:            "   ",
	})
	if err == nil || !strings.Contains(err.Error(), "subject") {
		t.Errorf("expected subject error, got %v", err)
	}
}

func TestTicketService_Create_AppliesDefaults(t *testing.T) {
	repo := newFakeTicketRepo()
	svc := NewTicketService(repo, nil)
	got, err := svc.Create(context.Background(), CreateTicketRequest{
		TenantID:           "t1",
		RequesterAccountID: "u1",
		Subject:            "Air conditioner broken",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if got.Type != ticket.TicketTypeGeneral {
		t.Errorf("default Type = %q, want %q", got.Type, ticket.TicketTypeGeneral)
	}
	if got.Priority != ticket.TicketPriorityMedium {
		t.Errorf("default Priority = %q, want %q", got.Priority, ticket.TicketPriorityMedium)
	}
	if got.Category != "general" {
		t.Errorf("default Category = %q, want %q", got.Category, "general")
	}
	if got.Status != ticket.TicketStatusOpen {
		t.Errorf("initial Status = %q, want %q", got.Status, ticket.TicketStatusOpen)
	}
}

func TestTicketService_Create_TrimsStringFields(t *testing.T) {
	repo := newFakeTicketRepo()
	svc := NewTicketService(repo, nil)
	got, err := svc.Create(context.Background(), CreateTicketRequest{
		TenantID:           "t1",
		RequesterAccountID: "u1",
		Subject:            "  Air conditioner broken  ",
		Description:        "  needs urgent repair  ",
		RequesterName:      "  Maria  ",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if got.Subject != "Air conditioner broken" {
		t.Errorf("Subject not trimmed: %q", got.Subject)
	}
	if got.Description != "needs urgent repair" {
		t.Errorf("Description not trimmed: %q", got.Description)
	}
	if got.RequesterName != "Maria" {
		t.Errorf("RequesterName not trimmed: %q", got.RequesterName)
	}
}

func TestTicketService_GetByIDForOperator_PresignsAttachmentsWithoutPersistingURLs(t *testing.T) {
	repo := newFakeTicketRepo()
	svc := NewTicketService(repo, nil).WithStorage(ticketAttachmentStorage{})
	created, err := svc.Create(context.Background(), CreateTicketRequest{
		TenantID:           "tenant-1",
		RequesterAccountID: "account-1",
		Subject:            "Arquivo do cliente",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	created.Comments = []ticket.Comment{
		ticket.NewComment("account-1", "Cliente", "client", "Segue o documento", []string{"tenant-1/tickets/ticket-1/document.png"}),
	}
	if err := repo.Update(context.Background(), created); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	view, err := svc.GetByIDForOperator(context.Background(), "tenant-1", created.ID)
	if err != nil {
		t.Fatalf("GetByIDForOperator returned error: %v", err)
	}
	if got := view.Comments[0].Attachments[0]; got != "https://storage.example/toca-documents/tenant-1/tickets/ticket-1/document.png?signed=1" {
		t.Fatalf("signed attachment = %q", got)
	}

	raw, err := svc.GetByID(context.Background(), "tenant-1", created.ID)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if got := raw.Comments[0].Attachments[0]; got != "tenant-1/tickets/ticket-1/document.png" {
		t.Fatalf("stored attachment was mutated to %q", got)
	}
}

func TestTicketService_GetByIDForOperator_RecoversLegacyUploadURL(t *testing.T) {
	repo := newFakeTicketRepo()
	svc := NewTicketService(repo, nil).WithStorage(ticketAttachmentStorage{})
	created, err := svc.Create(context.Background(), CreateTicketRequest{
		TenantID:           "tenant-1",
		RequesterAccountID: "account-1",
		Subject:            "Arquivo legado do cliente",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	created.Comments = []ticket.Comment{
		ticket.NewComment(
			"account-1",
			"Cliente",
			"client",
			"Segue o documento",
			[]string{"/uploads/toca-documents/tenant-1/tickets/ticket-1/Fundo.png"},
		),
	}
	if err := repo.Update(context.Background(), created); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	view, err := svc.GetByIDForOperator(context.Background(), "tenant-1", created.ID)
	if err != nil {
		t.Fatalf("GetByIDForOperator returned error: %v", err)
	}
	want := "https://storage.example/toca-documents/tenant-1/tickets/ticket-1/Fundo.png?signed=1"
	if got := view.Comments[0].Attachments[0]; got != want {
		t.Fatalf("legacy attachment = %q, want %q", got, want)
	}
}

func TestTicketService_UpdateStatus_SetsResolvedAt(t *testing.T) {
	repo := newFakeTicketRepo()
	svc := NewTicketService(repo, nil)
	created, _ := svc.Create(context.Background(), CreateTicketRequest{
		TenantID:           "t1",
		RequesterAccountID: "u1",
		Subject:            "x",
	})

	updated, err := svc.UpdateStatus(context.Background(), "t1", created.ID, ticket.TicketStatusResolved)
	if err != nil {
		t.Fatalf("UpdateStatus error: %v", err)
	}
	if updated.Status != ticket.TicketStatusResolved {
		t.Errorf("status = %q, want %q", updated.Status, ticket.TicketStatusResolved)
	}
	if updated.ResolvedAt == nil {
		t.Error("ResolvedAt should be set when status becomes resolved")
	}
}

func TestTicketService_UpdateStatus_SetsClosedAt(t *testing.T) {
	repo := newFakeTicketRepo()
	svc := NewTicketService(repo, nil)
	created, _ := svc.Create(context.Background(), CreateTicketRequest{
		TenantID:           "t1",
		RequesterAccountID: "u1",
		Subject:            "x",
	})

	updated, err := svc.UpdateStatus(context.Background(), "t1", created.ID, ticket.TicketStatusClosed)
	if err != nil {
		t.Fatalf("UpdateStatus error: %v", err)
	}
	if updated.ClosedAt == nil {
		t.Error("ClosedAt should be set when status becomes closed")
	}
}

func TestTicketService_UpdateStatus_NotFoundError(t *testing.T) {
	svc := NewTicketService(newFakeTicketRepo(), nil)
	_, err := svc.UpdateStatus(context.Background(), "t1", "nonexistent", ticket.TicketStatusClosed)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got %v", err)
	}
}

func TestTicketService_Assign_MovesOpenToAssigned(t *testing.T) {
	repo := newFakeTicketRepo()
	svc := NewTicketService(repo, nil)
	created, _ := svc.Create(context.Background(), CreateTicketRequest{
		TenantID:           "t1",
		RequesterAccountID: "u1",
		Subject:            "x",
	})

	got, err := svc.Assign(context.Background(), "t1", created.ID, "agent-1", "Agent One")
	if err != nil {
		t.Fatalf("Assign error: %v", err)
	}
	if got.AssignedToID != "agent-1" || got.AssignedToName != "Agent One" {
		t.Errorf("assignment fields mismatch: id=%q name=%q", got.AssignedToID, got.AssignedToName)
	}
	if got.Status != ticket.TicketStatusAssigned {
		t.Errorf("status should auto-transition to assigned, got %q", got.Status)
	}
}

func TestTicketService_Assign_KeepsExistingStatus(t *testing.T) {
	repo := newFakeTicketRepo()
	svc := NewTicketService(repo, nil)
	created, _ := svc.Create(context.Background(), CreateTicketRequest{
		TenantID:           "t1",
		RequesterAccountID: "u1",
		Subject:            "x",
	})
	created.Status = ticket.TicketStatusInProgress
	_ = repo.Update(context.Background(), created)

	got, err := svc.Assign(context.Background(), "t1", created.ID, "agent-1", "Agent One")
	if err != nil {
		t.Fatalf("Assign error: %v", err)
	}
	if got.Status != ticket.TicketStatusInProgress {
		t.Errorf("status should NOT regress; got %q", got.Status)
	}
}

func TestTicketService_AddComment_RequiresContent(t *testing.T) {
	repo := newFakeTicketRepo()
	svc := NewTicketService(repo, nil)
	created, _ := svc.Create(context.Background(), CreateTicketRequest{
		TenantID:           "t1",
		RequesterAccountID: "u1",
		Subject:            "x",
	})

	_, err := svc.AddComment(context.Background(), "t1", created.ID, AddCommentRequest{
		AuthorID:   "u1",
		AuthorType: "client",
		Content:    "  ",
	})
	if err == nil || !strings.Contains(err.Error(), "content") {
		t.Errorf("expected content error, got %v", err)
	}
}

func TestTicketService_AddComment_DefaultsAuthorType(t *testing.T) {
	repo := newFakeTicketRepo()
	svc := NewTicketService(repo, nil)
	created, _ := svc.Create(context.Background(), CreateTicketRequest{
		TenantID:           "t1",
		RequesterAccountID: "u1",
		Subject:            "x",
	})

	c, err := svc.AddComment(context.Background(), "t1", created.ID, AddCommentRequest{
		AuthorID: "u1",
		Content:  "First reply",
	})
	if err != nil {
		t.Fatalf("AddComment error: %v", err)
	}
	if c.AuthorType != "operator" {
		t.Errorf("default AuthorType = %q, want %q", c.AuthorType, "operator")
	}
}

func TestTicketService_AddComment_AppendsToTicket(t *testing.T) {
	repo := newFakeTicketRepo()
	svc := NewTicketService(repo, nil)
	created, _ := svc.Create(context.Background(), CreateTicketRequest{
		TenantID:           "t1",
		RequesterAccountID: "u1",
		Subject:            "x",
	})

	_, err := svc.AddComment(context.Background(), "t1", created.ID, AddCommentRequest{
		AuthorID:   "u1",
		AuthorType: "client",
		Content:    "Reply 1",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddComment(context.Background(), "t1", created.ID, AddCommentRequest{
		AuthorID:   "agent-1",
		AuthorType: "operator",
		Content:    "Reply 2",
	})
	if err != nil {
		t.Fatal(err)
	}

	reloaded, _ := svc.GetByID(context.Background(), "t1", created.ID)
	if len(reloaded.Comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(reloaded.Comments))
	}
}

func TestTicketService_AddComment_IdempotentWithSameClientRequestID(t *testing.T) {
	repo := newFakeTicketRepo()
	svc := NewTicketService(repo, nil)
	created, _ := svc.Create(context.Background(), CreateTicketRequest{
		TenantID:           "t1",
		RequesterAccountID: "u1",
		Subject:            "x",
	})

	crid := "client-req-abc-123"

	first, err := svc.AddComment(context.Background(), "t1", created.ID, AddCommentRequest{
		AuthorID:        "u1",
		AuthorType:      "client",
		Content:         "Olá, alguma novidade?",
		ClientRequestID: crid,
	})
	if err != nil {
		t.Fatalf("first AddComment: %v", err)
	}
	if first.ClientRequestID != crid {
		t.Errorf("expected crid persisted, got %q", first.ClientRequestID)
	}

	second, err := svc.AddComment(context.Background(), "t1", created.ID, AddCommentRequest{
		AuthorID:        "u1",
		AuthorType:      "client",
		Content:         "Olá, alguma novidade?",
		ClientRequestID: crid,
	})
	if err != nil {
		t.Fatalf("retry AddComment: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("expected same comment ID on retry, got first=%q second=%q", first.ID, second.ID)
	}

	reloaded, _ := svc.GetByID(context.Background(), "t1", created.ID)
	if len(reloaded.Comments) != 1 {
		t.Errorf("expected 1 comment after retry, got %d", len(reloaded.Comments))
	}
}

func TestTicketService_AddComment_DifferentClientRequestIDs_CreateBoth(t *testing.T) {
	repo := newFakeTicketRepo()
	svc := NewTicketService(repo, nil)
	created, _ := svc.Create(context.Background(), CreateTicketRequest{
		TenantID:           "t1",
		RequesterAccountID: "u1",
		Subject:            "x",
	})

	_, err := svc.AddComment(context.Background(), "t1", created.ID, AddCommentRequest{
		AuthorID:        "u1",
		AuthorType:      "client",
		Content:         "Mensagem A",
		ClientRequestID: "req-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddComment(context.Background(), "t1", created.ID, AddCommentRequest{
		AuthorID:        "u1",
		AuthorType:      "client",
		Content:         "Mensagem A",
		ClientRequestID: "req-2",
	})
	if err != nil {
		t.Fatal(err)
	}

	reloaded, _ := svc.GetByID(context.Background(), "t1", created.ID)
	if len(reloaded.Comments) != 2 {
		t.Errorf("expected 2 distinct comments (different crids), got %d", len(reloaded.Comments))
	}
}

func TestTicketService_AddComment_EmptyClientRequestID_DoesNotDedup(t *testing.T) {
	repo := newFakeTicketRepo()
	svc := NewTicketService(repo, nil)
	created, _ := svc.Create(context.Background(), CreateTicketRequest{
		TenantID:           "t1",
		RequesterAccountID: "u1",
		Subject:            "x",
	})

	_, err := svc.AddComment(context.Background(), "t1", created.ID, AddCommentRequest{
		AuthorID: "u1", AuthorType: "client", Content: "same",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddComment(context.Background(), "t1", created.ID, AddCommentRequest{
		AuthorID: "u1", AuthorType: "client", Content: "same",
	})
	if err != nil {
		t.Fatal(err)
	}
	reloaded, _ := svc.GetByID(context.Background(), "t1", created.ID)
	if len(reloaded.Comments) != 2 {
		t.Errorf("expected 2 comments when crid is empty (legacy), got %d", len(reloaded.Comments))
	}
}

func TestTicketService_AddComment_WhitespaceCRID_TreatedAsEmpty(t *testing.T) {
	repo := newFakeTicketRepo()
	svc := NewTicketService(repo, nil)
	created, _ := svc.Create(context.Background(), CreateTicketRequest{
		TenantID: "t1", RequesterAccountID: "u1", Subject: "x",
	})

	_, err := svc.AddComment(context.Background(), "t1", created.ID, AddCommentRequest{
		AuthorID:        "u1",
		AuthorType:      "client",
		Content:         "a",
		ClientRequestID: "   ",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddComment(context.Background(), "t1", created.ID, AddCommentRequest{
		AuthorID:        "u1",
		AuthorType:      "client",
		Content:         "b",
		ClientRequestID: "   ",
	})
	if err != nil {
		t.Fatal(err)
	}
	reloaded, _ := svc.GetByID(context.Background(), "t1", created.ID)
	if len(reloaded.Comments) != 2 {
		t.Errorf("whitespace crid should not trigger dedup, got %d comments", len(reloaded.Comments))
	}
}

func TestTicketService_List_RequiresTenantID(t *testing.T) {
	svc := NewTicketService(newFakeTicketRepo(), nil)
	_, _, err := svc.List(context.Background(), "", ticket.ListFilters{})
	if err == nil || !strings.Contains(err.Error(), "tenant_id") {
		t.Errorf("expected tenant_id error, got %v", err)
	}
}

func TestTicketService_ListByRequester_EmptyAccountID(t *testing.T) {
	svc := NewTicketService(newFakeTicketRepo(), nil)
	out, err := svc.ListByRequester(context.Background(), "t1", "  ")
	if err != nil {
		t.Fatalf("ListByRequester error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty result for empty accountID, got %d tickets", len(out))
	}
}
