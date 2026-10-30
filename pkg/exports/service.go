package exports

import (
	"context"
	"errors"
	"fmt"
)

// Service — fachada para o ciclo de vida do export job. Handlers
// HTTP chamam Submit, GetByID, ListByRequester. Worker chama Process
// internamente após pegar via Repo.ClaimNextQueued.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// SubmitInput — params do Submit em DTO próprio para o handler não
// vazar dependência direta com a struct Job.
type SubmitInput struct {
	TenantID    string
	RequestedBy string
	EntityType  EntityType
	Format      Format
	Filters     map[string]interface{}
}

// Submit cria um job em QUEUED. Valida que (entity, format) tem
// generator registrado — falha rápido em 400 em vez de criar job
// fadado a falhar no worker.
func (s *Service) Submit(ctx context.Context, in SubmitInput) (*Job, error) {
	if !IsRegistered(in.EntityType, in.Format) {
		return nil, fmt.Errorf("%w: %s/%s", ErrGeneratorNotFound, in.EntityType, in.Format)
	}
	j, err := NewJob(in.TenantID, in.RequestedBy, in.EntityType, in.Format, in.Filters)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, j); err != nil {
		return nil, err
	}
	return j, nil
}

// GetByID — pra polling do frontend ("já ficou pronto?").
func (s *Service) GetByID(ctx context.Context, tenantID, id string) (*Job, error) {
	j, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if j == nil {
		return nil, ErrJobNotFound
	}
	return j, nil
}

// ListByRequester — "Meus exports" do usuário corrente.
func (s *Service) ListByRequester(ctx context.Context, tenantID, accountID string, limit, offset int) ([]*Job, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListByRequester(ctx, tenantID, accountID, limit, offset)
}

// Process — invocado pelo worker depois do Repo.ClaimNextQueued
// retornar um Job (já em RUNNING). Resolve generator, gera bytes,
// uploada via uploader, marca como SUCCEEDED ou FAILED.
//
// Uploader é passado como callback (não como interface no Service)
// para evitar circular import entre pkg/exports e pkg/storage —
// também permite testes injetarem fake uploader sem montar o
// storage.Service inteiro.
type Uploader func(ctx context.Context, bucket, filename, contentType string, content []byte) (presignedURL string, err error)

// Process leva o job de RUNNING → SUCCEEDED|FAILED. Caller (worker)
// passa o bucket onde os arquivos vão (geralmente "exports" ou
// equivalente configurável).
func (s *Service) Process(ctx context.Context, job *Job, bucket string, upload Uploader) error {
	gen, err := ResolveDefault(job.EntityType, job.Format)
	if err != nil {
		// Generator desapareceu entre Submit e Process — improvável (a
		// validação inicial bloqueia) mas defensivo.
		_ = job.MarkFailed(fmt.Sprintf("generator unavailable: %v", err))
		return s.repo.Update(ctx, job)
	}

	content, filePath, rowCount, err := gen.Generate(ctx, job)
	if err != nil {
		_ = job.MarkFailed(fmt.Sprintf("generator error: %v", err))
		return s.repo.Update(ctx, job)
	}

	url, err := upload(ctx, bucket, filePath, job.Format.ContentType(), content)
	if err != nil {
		_ = job.MarkFailed(fmt.Sprintf("upload error: %v", err))
		return s.repo.Update(ctx, job)
	}

	if err := job.MarkSucceeded(bucket, filePath, url, rowCount); err != nil {
		// Estado inválido — não deve acontecer no caminho feliz.
		return errors.New("MarkSucceeded failed: " + err.Error())
	}
	return s.repo.Update(ctx, job)
}
