package handlers_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	appCtx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-ops/internal/handlers"
	"github.com/oginoino/toca/toca-ops/internal/models/inspection"
	"github.com/oginoino/toca/toca-ops/internal/services"
	"github.com/stretchr/testify/assert"
)

// tenantScopedInspectionRepo reproduz a semântica do repositório Postgres
// depois da correção (auditoria C8): o SELECT é filtrado por `tenant_id` do
// chamador, tenant vazio falha fechado, e toda escrita passa por Update — que
// os testes contam para provar que nada foi gravado no registro alheio.
type tenantScopedInspectionRepo struct {
	mu         sync.Mutex
	records    map[string]*inspection.Inspection
	updates    []string
	lastTenant string
}

func newTenantScopedInspectionRepo(seed ...*inspection.Inspection) *tenantScopedInspectionRepo {
	r := &tenantScopedInspectionRepo{records: map[string]*inspection.Inspection{}}
	for _, s := range seed {
		r.records[s.ID] = s
	}
	return r
}

func (r *tenantScopedInspectionRepo) Create(_ context.Context, i *inspection.Inspection) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *i
	r.records[i.ID] = &cp
	return nil
}

func (r *tenantScopedInspectionRepo) GetByID(_ context.Context, tenantID, id string) (*inspection.Inspection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastTenant = tenantID
	if strings.TrimSpace(tenantID) == "" {
		return nil, fmt.Errorf("inspection not found: tenant context required")
	}
	rec, ok := r.records[id]
	if !ok || rec.TenantID != tenantID {
		// Mesmo erro do Postgres para "não existe" e "é de outro tenant".
		return nil, fmt.Errorf("inspection not found: sql: no rows in result set")
	}
	cp := *rec
	return &cp, nil
}

func (r *tenantScopedInspectionRepo) Update(_ context.Context, i *inspection.Inspection) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates = append(r.updates, i.ID)
	cp := *i
	r.records[i.ID] = &cp
	return nil
}

func (r *tenantScopedInspectionRepo) Delete(_ context.Context, tenantID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.records, id)
	return nil
}

func (r *tenantScopedInspectionRepo) List(_ context.Context, f inspection.ListFilter) ([]*inspection.Inspection, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*inspection.Inspection, 0)
	for _, rec := range r.records {
		if f.TenantID != "" && rec.TenantID != f.TenantID {
			continue
		}
		cp := *rec
		out = append(out, &cp)
	}
	return out, int64(len(out)), nil
}

func (r *tenantScopedInspectionRepo) updateCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.updates)
}

func (r *tenantScopedInspectionRepo) stored(id string) *inspection.Inspection {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.records[id]
	if !ok {
		return nil
	}
	cp := *rec
	return &cp
}

type stubPDFRenderer struct{}

func (stubPDFRenderer) RenderHTML(context.Context, string, string) ([]byte, error) {
	return []byte("%PDF-1.4 stub"), nil
}
func (stubPDFRenderer) Available() bool { return true }

type stubStorage struct{}

func (stubStorage) Upload(context.Context, string, string, string, io.Reader) (string, error) {
	return "https://storage.example/toca-inspections/laudo.pdf?signed=1", nil
}
func (stubStorage) Delete(context.Context, string, string) error { return nil }
func (stubStorage) GetURL(string, string) string                 { return "" }
func (stubStorage) GetPresignedURL(context.Context, string, string, time.Duration) (string, error) {
	return "", nil
}
func (stubStorage) GetPresignedPutURL(context.Context, string, string, time.Duration) (string, error) {
	return "", nil
}
func (stubStorage) IsPrivate(string) bool { return true }

// newInspectionRouter monta SÓ as rotas de vistoria, com um middleware que
// injeta o tenant do chamador no request context — o mesmo valor que
// auth.Middleware/InternalOrMiddleware colocam em produção. Isso isola o
// comportamento handler→service→repositório, sem JWT nem banco.
func newInspectionRouter(repo *tenantScopedInspectionRepo, tenantID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := services.NewInspectionService(repo, nil).
		WithPDFRenderer(stubPDFRenderer{}).
		WithStorage(stubStorage{})
	h := handlers.NewInspectionHandler(svc)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		if tenantID != "" {
			c.Request = c.Request.WithContext(appCtx.WithTenantID(c.Request.Context(), tenantID))
		}
		c.Next()
	})
	r.GET("/api/v1/inspections/:id", h.GetByID)
	r.POST("/api/v1/inspections/:id/generate-laudo", h.GenerateLaudo)
	r.PUT("/api/v1/inspections/:id", h.Update)
	r.POST("/api/v1/inspections/:id/deductions", h.RecordDeductions)
	r.DELETE("/api/v1/inspections/:id", h.Delete)
	return r
}

func seedInspection(tenantID string) *inspection.Inspection {
	return &inspection.Inspection{
		ID:            "insp-" + tenantID,
		TenantID:      tenantID,
		PropertyID:    "prop-" + tenantID,
		ContractID:    "contract-" + tenantID,
		Type:          inspection.TypeExit,
		Status:        inspection.StatusConcluida,
		InspectorID:   "ins-1",
		InspectorName: "Maria",
		ReportURL:     "https://docs.example/" + tenantID + "/laudo.pdf",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

func doRequest(r *gin.Engine, method, path string, body string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// C8: leitura cross-tenant. Antes da correção o handler chamava
// service.GetByID(ctx, id) e o repositório filtrava apenas por id — a vistoria
// do tenant B era devolvida com 200 para um chamador do tenant A.
func TestInspectionHandler_GetByID_CrossTenant_IsNotFound(t *testing.T) {
	victim := seedInspection("tenant-B")
	repo := newTenantScopedInspectionRepo(victim)
	router := newInspectionRouter(repo, "tenant-A")

	w := doRequest(router, http.MethodGet, "/api/v1/inspections/"+victim.ID, "")

	assert.Equal(t, http.StatusNotFound, w.Code, "vistoria de outro tenant precisa falhar fechado (404)")
	assert.NotContains(t, w.Body.String(), "prop-tenant-B", "a resposta não pode vazar dados do tenant B")
	assert.NotContains(t, w.Body.String(), victim.ReportURL)
	assert.Equal(t, "tenant-A", repo.lastTenant, "o tenant consultado deve ser o do chamador, não o do registro")
}

func TestInspectionHandler_GetByID_OwnTenant_StillWorks(t *testing.T) {
	own := seedInspection("tenant-A")
	repo := newTenantScopedInspectionRepo(own)
	router := newInspectionRouter(repo, "tenant-A")

	w := doRequest(router, http.MethodGet, "/api/v1/inspections/"+own.ID, "")

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "prop-tenant-A")
}

func TestInspectionHandler_GetByID_EmptyTenant_Forbidden(t *testing.T) {
	victim := seedInspection("tenant-B")
	repo := newTenantScopedInspectionRepo(victim)
	router := newInspectionRouter(repo, "")

	w := doRequest(router, http.MethodGet, "/api/v1/inspections/"+victim.ID, "")

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Zero(t, repo.updateCount())
}

// C8: o fluxo de escrita. Antes da correção o handler chamava
// GenerateAndAttachLaudo(ctx, id); o UPDATE usava o tenant DO REGISTRO
// carregado por id, então o laudo do tenant B era sobrescrito pelo tenant A.
func TestInspectionHandler_GenerateLaudo_CrossTenant_DoesNotWrite(t *testing.T) {
	victim := seedInspection("tenant-B")
	originalURL := victim.ReportURL
	repo := newTenantScopedInspectionRepo(victim)
	router := newInspectionRouter(repo, "tenant-A")

	w := doRequest(router, http.MethodPost, "/api/v1/inspections/"+victim.ID+"/generate-laudo", "")

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Zero(t, repo.updateCount(), "nenhuma escrita pode acontecer em vistoria de outro tenant")
	if stored := repo.stored(victim.ID); assert.NotNil(t, stored) {
		assert.Equal(t, originalURL, stored.ReportURL, "o laudo do tenant B não pode ser sobrescrito")
	}
}

func TestInspectionHandler_GenerateLaudo_OwnTenant_Writes(t *testing.T) {
	own := seedInspection("tenant-A")
	repo := newTenantScopedInspectionRepo(own)
	router := newInspectionRouter(repo, "tenant-A")

	w := doRequest(router, http.MethodPost, "/api/v1/inspections/"+own.ID+"/generate-laudo", "")

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, repo.updateCount())
	if stored := repo.stored(own.ID); assert.NotNil(t, stored) {
		assert.Contains(t, stored.ReportURL, "storage.example")
	}
}

func TestInspectionHandler_Update_CrossTenant_IsNotFound(t *testing.T) {
	victim := seedInspection("tenant-B")
	originalURL := victim.ReportURL
	repo := newTenantScopedInspectionRepo(victim)
	router := newInspectionRouter(repo, "tenant-A")

	w := doRequest(router, http.MethodPut, "/api/v1/inspections/"+victim.ID,
		`{"report_url":"https://evil.example/laudo.pdf"}`)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Zero(t, repo.updateCount())
	if stored := repo.stored(victim.ID); assert.NotNil(t, stored) {
		assert.Equal(t, originalURL, stored.ReportURL)
	}
}

func TestInspectionHandler_Delete_CrossTenant_IsNotFound(t *testing.T) {
	victim := seedInspection("tenant-B")
	repo := newTenantScopedInspectionRepo(victim)
	router := newInspectionRouter(repo, "tenant-A")

	w := doRequest(router, http.MethodDelete, "/api/v1/inspections/"+victim.ID, "")

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.NotNil(t, repo.stored(victim.ID), "vistoria de outro tenant não pode ser apagada")
}

func TestInspectionHandler_RecordDeductions_CrossTenant_IsNotFound(t *testing.T) {
	victim := seedInspection("tenant-B")
	repo := newTenantScopedInspectionRepo(victim)
	router := newInspectionRouter(repo, "tenant-A")

	w := doRequest(router, http.MethodPost, "/api/v1/inspections/"+victim.ID+"/deductions",
		`{"deductions":[{"room":"Sala","description":"Pintura","amount":100}]}`)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Zero(t, repo.updateCount())
	if stored := repo.stored(victim.ID); assert.NotNil(t, stored) {
		assert.Empty(t, stored.Deductions)
	}
}
