package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-financial/internal/domain/collection"
	"github.com/oginoino/toca/toca-financial/internal/services"
)

// collectionRepoMemoria é o mínimo para exercitar o handler de update: guarda um
// caso, aceita Update e registra o que foi persistido.
type collectionRepoMemoria struct {
	caso       *collection.Collection
	persistido *collection.Collection
}

func (r *collectionRepoMemoria) Create(context.Context, *collection.Collection) error { return nil }
func (r *collectionRepoMemoria) CreateIdempotent(context.Context, *collection.Collection) error {
	return nil
}
func (r *collectionRepoMemoria) GetByID(_ context.Context, _, _ string) (*collection.Collection, error) {
	return r.caso, nil
}
func (r *collectionRepoMemoria) Update(_ context.Context, c *collection.Collection) error {
	r.persistido = c
	r.caso = c
	return nil
}
func (r *collectionRepoMemoria) List(context.Context, string, string, int64, int64) ([]*collection.Collection, int64, error) {
	return nil, 0, nil
}
func (r *collectionRepoMemoria) AddAction(context.Context, string, string, collection.Action) error {
	return nil
}
func (r *collectionRepoMemoria) FindByChargeID(context.Context, string, string) (*collection.Collection, error) {
	return nil, nil
}
func (r *collectionRepoMemoria) FindByContract(context.Context, string, string) ([]*collection.Collection, error) {
	return nil, nil
}
func (r *collectionRepoMemoria) FindActiveByDebtor(context.Context, string, string) (*collection.Collection, error) {
	return nil, nil
}
func (r *collectionRepoMemoria) GetStats(context.Context, string) (*collection.CollectionStats, error) {
	return &collection.CollectionStats{}, nil
}

// Regressão: uma mudança de status sem os campos de negociação apagava o acordo
// já registrado, porque o handler atribuía `negotiated_amount`/`negotiated_date`
// incondicionalmente. O corpo típico desta rota é justamente uma transição
// (NEGOTIATED → GUARANTEE_ACTIVATED → JUDICIAL) sem esses campos.
func TestUpdateCollectionNaoApagaNegociacaoQuandoCampoAusente(t *testing.T) {
	gin.SetMode(gin.TestMode)

	valorNegociado := 2500.0
	dataNegociacao := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	repo := &collectionRepoMemoria{caso: &collection.Collection{
		ID:               "coll-1",
		TenantID:         "t-1",
		ContractID:       "c-1",
		Status:           collection.StatusNegotiated,
		NegotiatedAmount: &valorNegociado,
		NegotiatedDate:   &dataNegociacao,
		UpdatedAt:        time.Now(),
	}}
	svc := services.NewCollectionService(repo)
	handler := NewCollectionHandler(svc)

	// Corpo com transição de status e SEM os campos de negociação.
	corpo, _ := json.Marshal(map[string]any{"status": "GUARANTEE_ACTIVATED"})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/collections/coll-1", bytes.NewReader(corpo))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "coll-1"}}
	ctx := pkgctx.WithTenantID(c.Request.Context(), "t-1")
	c.Request = c.Request.WithContext(ctx)

	handler.Update(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, corpo = %s", rec.Code, rec.Body.String())
	}
	if repo.persistido == nil {
		t.Fatalf("nada foi persistido")
	}
	if repo.persistido.NegotiatedAmount == nil || *repo.persistido.NegotiatedAmount != valorNegociado {
		t.Fatalf("negotiated_amount foi apagado: %v", repo.persistido.NegotiatedAmount)
	}
	if repo.persistido.NegotiatedDate == nil || !repo.persistido.NegotiatedDate.Equal(dataNegociacao) {
		t.Fatalf("negotiated_date foi apagado: %v", repo.persistido.NegotiatedDate)
	}
	if repo.persistido.Status != collection.StatusGuaranteeActivated {
		t.Fatalf("status = %s, esperado GUARANTEE_ACTIVATED", repo.persistido.Status)
	}
}

// O envio explícito continua atualizando o acordo.
func TestUpdateCollectionAtualizaNegociacaoQuandoCampoEnviado(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &collectionRepoMemoria{caso: &collection.Collection{
		ID:        "coll-1",
		TenantID:  "t-1",
		Status:    collection.StatusActive,
		UpdatedAt: time.Now(),
	}}
	handler := NewCollectionHandler(services.NewCollectionService(repo))

	corpo, _ := json.Marshal(map[string]any{
		"status":            "NEGOTIATED",
		"negotiated_amount": 1800.5,
		"negotiated_date":   "2026-10-01T00:00:00Z",
	})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/collections/coll-1", bytes.NewReader(corpo))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "coll-1"}}
	c.Request = c.Request.WithContext(pkgctx.WithTenantID(c.Request.Context(), "t-1"))

	handler.Update(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, corpo = %s", rec.Code, rec.Body.String())
	}
	if repo.persistido.NegotiatedAmount == nil || *repo.persistido.NegotiatedAmount != 1800.5 {
		t.Fatalf("negotiated_amount não foi atualizado: %v", repo.persistido.NegotiatedAmount)
	}
	if repo.persistido.Status != collection.StatusNegotiated {
		t.Fatalf("status = %s, esperado NEGOTIATED", repo.persistido.Status)
	}
}
