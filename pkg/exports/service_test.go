package exports

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// inMemoryRepo é o stub mínimo de Repository pros testes de Service /
// Worker. Suporta múltiplas escritas + queue draining.
type inMemoryRepo struct {
	mu   sync.Mutex
	jobs map[string]*Job
	// queue ordenada de IDs em status QUEUED (FIFO).
	queue []string
}

func newRepo() *inMemoryRepo {
	return &inMemoryRepo{jobs: map[string]*Job{}}
}

func (r *inMemoryRepo) Create(_ context.Context, j *Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *j
	r.jobs[j.ID] = &clone
	if j.Status == StatusQueued {
		r.queue = append(r.queue, j.ID)
	}
	return nil
}

func (r *inMemoryRepo) GetByID(_ context.Context, tenantID, id string) (*Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok || j.TenantID != tenantID {
		return nil, nil
	}
	clone := *j
	return &clone, nil
}

func (r *inMemoryRepo) Update(_ context.Context, j *Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *j
	r.jobs[j.ID] = &clone
	return nil
}

func (r *inMemoryRepo) ListByRequester(_ context.Context, tenantID, accountID string, limit, offset int) ([]*Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Job, 0)
	for _, j := range r.jobs {
		if j.TenantID == tenantID && j.RequestedBy == accountID {
			clone := *j
			out = append(out, &clone)
		}
	}
	return out, nil
}

func (r *inMemoryRepo) ClaimNextQueued(_ context.Context) (*Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for len(r.queue) > 0 {
		id := r.queue[0]
		r.queue = r.queue[1:]
		j, ok := r.jobs[id]
		if !ok || j.Status != StatusQueued {
			continue // foi cancelado/removido
		}
		if err := j.MarkRunning(); err != nil {
			continue
		}
		clone := *j
		return &clone, nil
	}
	return nil, nil
}

// stubGenerator devolve bytes/path fixos e contabiliza chamadas.
type stubGenerator struct {
	content  []byte
	path     string
	rowCount int64
	calls    int
	err      error
}

func (g *stubGenerator) Generate(_ context.Context, _ *Job) ([]byte, string, int64, error) {
	g.calls++
	return g.content, g.path, g.rowCount, g.err
}

func TestService_Submit_RejectsUnregisteredGenerator(t *testing.T) {
	ResetDefault()
	svc := NewService(newRepo())
	_, err := svc.Submit(context.Background(), SubmitInput{
		TenantID: "t1", RequestedBy: "u1", EntityType: "Ghost", Format: FormatCSV,
	})
	if !errors.Is(err, ErrGeneratorNotFound) {
		t.Errorf("expected ErrGeneratorNotFound, got %v", err)
	}
}

func TestService_Submit_QueuesValidJob(t *testing.T) {
	ResetDefault()
	Register("Lead", FormatCSV, &stubGenerator{content: []byte("ok"), path: "x.csv", rowCount: 1})
	repo := newRepo()
	svc := NewService(repo)

	j, err := svc.Submit(context.Background(), SubmitInput{
		TenantID: "t1", RequestedBy: "u1", EntityType: "Lead", Format: FormatCSV,
		Filters: map[string]interface{}{"status": "NEW"},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if j.Status != StatusQueued {
		t.Errorf("status = %q, want QUEUED", j.Status)
	}
	if len(repo.queue) != 1 {
		t.Errorf("queue should have 1, got %d", len(repo.queue))
	}
}

func TestService_Process_SuccessPath(t *testing.T) {
	ResetDefault()
	gen := &stubGenerator{
		content:  []byte("id,name\n1,Ana\n"),
		path:     "exports/t1/leads.csv",
		rowCount: 1,
	}
	Register("Lead", FormatCSV, gen)

	repo := newRepo()
	svc := NewService(repo)
	j, _ := svc.Submit(context.Background(), SubmitInput{
		TenantID: "t1", RequestedBy: "u1", EntityType: "Lead", Format: FormatCSV,
	})

	claimed, _ := repo.ClaimNextQueued(context.Background())
	if claimed == nil {
		t.Fatal("Claim devolveu nil")
	}

	uploaded := struct {
		bucket, path, ctype string
		size                int
	}{}
	uploader := func(_ context.Context, bucket, filename, ctype string, content []byte) (string, error) {
		uploaded.bucket, uploaded.path, uploaded.ctype = bucket, filename, ctype
		uploaded.size = len(content)
		return "https://signed/" + filename, nil
	}

	if err := svc.Process(context.Background(), claimed, "exports-bucket", uploader); err != nil {
		t.Fatalf("Process: %v", err)
	}

	got, _ := svc.GetByID(context.Background(), "t1", j.ID)
	if got.Status != StatusSucceeded {
		t.Errorf("status = %q, want SUCCEEDED", got.Status)
	}
	if got.RowCount != 1 {
		t.Errorf("RowCount = %d", got.RowCount)
	}
	if !strings.HasPrefix(got.ResultURL, "https://signed/") {
		t.Errorf("ResultURL wrong: %s", got.ResultURL)
	}
	if uploaded.bucket != "exports-bucket" || uploaded.path != gen.path {
		t.Errorf("upload args wrong: %+v", uploaded)
	}
	if uploaded.ctype != "text/csv; charset=utf-8" {
		t.Errorf("ctype = %s", uploaded.ctype)
	}
	if gen.calls != 1 {
		t.Errorf("generator chamado %d vezes, want 1", gen.calls)
	}
}

func TestService_Process_GeneratorErrorMarksFailed(t *testing.T) {
	ResetDefault()
	Register("Lead", FormatCSV, &stubGenerator{err: errors.New("query timeout")})

	repo := newRepo()
	svc := NewService(repo)
	j, _ := svc.Submit(context.Background(), SubmitInput{
		TenantID: "t1", RequestedBy: "u1", EntityType: "Lead", Format: FormatCSV,
	})
	claimed, _ := repo.ClaimNextQueued(context.Background())

	upload := func(context.Context, string, string, string, []byte) (string, error) {
		t.Fatal("uploader should not be called when generator fails")
		return "", nil
	}
	_ = svc.Process(context.Background(), claimed, "b", upload)

	got, _ := svc.GetByID(context.Background(), "t1", j.ID)
	if got.Status != StatusFailed {
		t.Errorf("status = %q, want FAILED", got.Status)
	}
	if !strings.Contains(got.ErrorMessage, "query timeout") {
		t.Errorf("ErrorMessage = %q", got.ErrorMessage)
	}
}

func TestService_Process_UploadErrorMarksFailed(t *testing.T) {
	ResetDefault()
	Register("Lead", FormatCSV, &stubGenerator{content: []byte("x"), path: "p.csv"})

	repo := newRepo()
	svc := NewService(repo)
	j, _ := svc.Submit(context.Background(), SubmitInput{
		TenantID: "t1", RequestedBy: "u1", EntityType: "Lead", Format: FormatCSV,
	})
	claimed, _ := repo.ClaimNextQueued(context.Background())

	uploadErr := func(context.Context, string, string, string, []byte) (string, error) {
		return "", errors.New("s3 down")
	}
	_ = svc.Process(context.Background(), claimed, "b", uploadErr)

	got, _ := svc.GetByID(context.Background(), "t1", j.ID)
	if got.Status != StatusFailed {
		t.Errorf("status = %q, want FAILED", got.Status)
	}
	if !strings.Contains(got.ErrorMessage, "s3 down") {
		t.Errorf("ErrorMessage = %q", got.ErrorMessage)
	}
}

func TestService_GetByID_TenantIsolation(t *testing.T) {
	ResetDefault()
	Register("Lead", FormatCSV, &stubGenerator{})
	repo := newRepo()
	svc := NewService(repo)
	j, _ := svc.Submit(context.Background(), SubmitInput{
		TenantID: "t-A", RequestedBy: "u1", EntityType: "Lead", Format: FormatCSV,
	})

	// Tenant B tentando ver o job do tenant A → not found.
	_, err := svc.GetByID(context.Background(), "t-B", j.ID)
	if !errors.Is(err, ErrJobNotFound) {
		t.Errorf("expected ErrJobNotFound, got %v", err)
	}
}

func TestRegistry_RegisterAndResolve(t *testing.T) {
	r := &Registry{generators: map[generatorKey]Generator{}}
	gen := &stubGenerator{}
	r.Register("Deal", FormatCSV, gen)

	got, err := r.Resolve("Deal", FormatCSV)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != gen {
		t.Error("returned different generator")
	}

	_, err = r.Resolve("Deal", FormatPDF)
	if !errors.Is(err, ErrGeneratorNotFound) {
		t.Errorf("expected ErrGeneratorNotFound, got %v", err)
	}
}
