// Package exports provides infrastructure for asynchronous CSV/PDF
// exports across services. Issue #474.
//
// Why async? CSVs/PDFs of long lists (10k+ leads, 20k+ charges) can
// take seconds to minutes to generate — blocking the HTTP request is
// fragile (timeout, retry duplication, browser closure aborts work).
// Pattern:
//
//   1. User clicks "Exportar CSV" → POST /exports retorna 202 + job ID
//   2. Worker pega da fila (Postgres-backed simple queue, sem RabbitMQ
//      pra evitar complicar wiring inicial — pode evoluir depois)
//   3. Worker chama Generator (registrado por entity type) → bytes
//   4. Upload pra pkg/storage → URL pré-assinada (7 dias)
//   5. Status → SUCCEEDED, ResultURL setada
//   6. (Opcional, follow-up) Evento export.completed → toca-comm
//      manda email com link
//   7. Frontend faz polling em GET /exports/:id ou abre a tela "Meus
//      Exports" e atualiza via WS
//
// Cada service expõe seus próprios endpoints (POST /leads/export,
// POST /charges/export etc.) que delegam pra exports.Service.Submit;
// o worker pode rodar em qualquer serviço — geralmente no mesmo que
// owna os dados pra não cruzar tenant boundaries de RLS.
package exports

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Status — lifecycle do export job.
type Status string

const (
	// StatusQueued — recém-criado, aguardando worker pegar.
	StatusQueued Status = "QUEUED"
	// StatusRunning — worker pegou e está processando. Esse estado
	// também serve como "soft lock" — outros workers veem RUNNING e
	// não pegam de novo (race protection a nível de DB via UPDATE
	// ... WHERE status = 'QUEUED' returning).
	StatusRunning Status = "RUNNING"
	// StatusSucceeded — terminou OK, ResultURL setada.
	StatusSucceeded Status = "SUCCEEDED"
	// StatusFailed — erro definitivo (não-recuperável). Error field
	// tem o detalhe. Caller pode tentar de novo criando job novo.
	StatusFailed Status = "FAILED"
)

func (s Status) IsTerminal() bool {
	return s == StatusSucceeded || s == StatusFailed
}

// Format do arquivo final. CSV é o caso default; PDF requer Generator
// específico (futuro: Gotenberg via toca-deal/contract style).
type Format string

const (
	FormatCSV Format = "csv"
	FormatPDF Format = "pdf"
)

func (f Format) IsValid() bool {
	return f == FormatCSV || f == FormatPDF
}

func (f Format) ContentType() string {
	switch f {
	case FormatCSV:
		return "text/csv; charset=utf-8"
	case FormatPDF:
		return "application/pdf"
	}
	return "application/octet-stream"
}

// EntityType — domínio sendo exportado. Generators são registrados
// por (EntityType, Format). Strings livres para que cada service
// adicione novos sem mexer no pkg/exports. Convenção: capitalizado +
// singular (Lead, Deal, Charge), igual ao audit.EntityType.
type EntityType string

// Job — registro persistido de uma operação de export. O blob inteiro
// vai em JSONB no DB; campos top-level pra status + entity ficam
// fora para queries eficientes.
type Job struct {
	ID         string                 `json:"id"`
	TenantID   string                 `json:"tenant_id"`
	RequestedBy string                `json:"requested_by"` // accountID
	EntityType EntityType             `json:"entity_type"`
	Format     Format                 `json:"format"`
	Filters    map[string]interface{} `json:"filters,omitempty"` // shape definido pelo Generator
	Status     Status                 `json:"status"`
	// ResultURL é o link pré-assinado quando SUCCEEDED. Vazio enquanto
	// QUEUED/RUNNING/FAILED.
	ResultURL    string     `json:"result_url,omitempty"`
	ResultBucket string     `json:"result_bucket,omitempty"`
	ResultPath   string     `json:"result_path,omitempty"`
	RowCount     int64      `json:"row_count,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	// Timestamps separados pro frontend distinguir "queued há 30s"
	// vs "started há 5min" vs "done há 10min".
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// NewJob constrói no estado QUEUED. UUID gerado aqui — caller não
// precisa providenciar. Validate roda dentro pra falhar rápido se o
// input estiver malformado.
func NewJob(tenantID, requestedBy string, entityType EntityType, format Format, filters map[string]interface{}) (*Job, error) {
	j := &Job{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		RequestedBy: requestedBy,
		EntityType:  entityType,
		Format:      format,
		Filters:     filters,
		Status:      StatusQueued,
		CreatedAt:   time.Now(),
	}
	if err := j.Validate(); err != nil {
		return nil, err
	}
	return j, nil
}

func (j *Job) Validate() error {
	if strings.TrimSpace(j.TenantID) == "" {
		return errors.New("tenant_id required")
	}
	if strings.TrimSpace(j.RequestedBy) == "" {
		return errors.New("requested_by required")
	}
	if strings.TrimSpace(string(j.EntityType)) == "" {
		return errors.New("entity_type required")
	}
	if !j.Format.IsValid() {
		return errors.New("format must be csv or pdf")
	}
	return nil
}

// MarkRunning transita QUEUED → RUNNING e marca StartedAt. Idempotente
// quando já RUNNING.
func (j *Job) MarkRunning() error {
	if j.Status == StatusRunning {
		return nil
	}
	if j.Status != StatusQueued {
		return ErrInvalidTransition
	}
	now := time.Now()
	j.Status = StatusRunning
	j.StartedAt = &now
	return nil
}

// MarkSucceeded fecha o job com URL + bytes processados.
func (j *Job) MarkSucceeded(bucket, path, url string, rowCount int64) error {
	if j.Status == StatusSucceeded {
		return nil
	}
	if j.Status != StatusRunning {
		return ErrInvalidTransition
	}
	now := time.Now()
	j.Status = StatusSucceeded
	j.ResultURL = url
	j.ResultBucket = bucket
	j.ResultPath = path
	j.RowCount = rowCount
	j.CompletedAt = &now
	return nil
}

// MarkFailed marca o job como erro definitivo. Pode rodar a partir de
// QUEUED ou RUNNING — falha de generator ou de upload são ambos
// terminais.
func (j *Job) MarkFailed(reason string) error {
	if j.Status == StatusFailed {
		return nil
	}
	if j.Status.IsTerminal() {
		return ErrInvalidTransition
	}
	now := time.Now()
	j.Status = StatusFailed
	j.ErrorMessage = reason
	j.CompletedAt = &now
	return nil
}

var (
	ErrInvalidTransition = errors.New("invalid export status transition")
	ErrJobNotFound       = errors.New("export job not found")
	ErrGeneratorNotFound = errors.New("no generator registered for entity_type/format")
)
