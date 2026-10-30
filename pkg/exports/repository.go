package exports

import "context"

// Repository — port para persistência dos export jobs. Cada service
// que usa o pkg/exports providencia uma implementação (Postgres no
// caso comum). Mantém pkg/exports agnóstico de DB.
type Repository interface {
	Create(ctx context.Context, j *Job) error
	GetByID(ctx context.Context, tenantID, id string) (*Job, error)
	Update(ctx context.Context, j *Job) error

	// ListByRequester — usado pelo painel "Meus exports" do frontend.
	// Newest first. Limit/offset para paginação (default 50 caso 0).
	ListByRequester(ctx context.Context, tenantID, accountID string, limit, offset int) ([]*Job, error)

	// ClaimNextQueued — atomicamente pega o próximo job QUEUED e
	// marca como RUNNING. Retorna nil quando não há fila. Worker chama
	// no loop pra evitar dois workers pegarem o mesmo job (race
	// proteção via UPDATE ... RETURNING).
	ClaimNextQueued(ctx context.Context) (*Job, error)
}
