package exports

import (
	"context"
	"errors"
	"time"

	"github.com/oginoino/toca/pkg/logger"
)

// WorkerInterval — frequência do polling pra novos jobs QUEUED.
// Default 5s, exposto como var pra tests poderem reduzir.
var WorkerInterval = 5 * time.Second

// Worker — goroutine de background que processa export jobs. Cada
// service que usa pkg/exports pode rodar 1+ workers no boot via
// `go worker.Run(ctx)`. Múltiplos workers no mesmo serviço (ou em
// instâncias replicadas) são seguros: ClaimNextQueued usa
// UPDATE ... RETURNING pra atomicidade.
type Worker struct {
	service  *Service
	repo     Repository
	bucket   string
	uploader Uploader
}

func NewWorker(svc *Service, repo Repository, bucket string, uploader Uploader) *Worker {
	return &Worker{service: svc, repo: repo, bucket: bucket, uploader: uploader}
}

// Run dispara o loop principal. Encerra ao ctx.Done. Erros não-fatais
// loggados; loop continua. Worker que catastroficamente falha (panic)
// é reiniciado pelo runtime do go ou pelo orchestrator do serviço.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(WorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

// tick — uma rodada. Drena até MaxJobsPerTick jobs ou para no primeiro
// "nada pra fazer".
const MaxJobsPerTick = 5

func (w *Worker) tick(ctx context.Context) {
	for i := 0; i < MaxJobsPerTick; i++ {
		if ctx.Err() != nil {
			return
		}
		job, err := w.repo.ClaimNextQueued(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				logger.Warn("export worker: claim failed", "error", err.Error())
			}
			return
		}
		if job == nil {
			return // fila vazia, espera o próximo tick
		}
		// Encapsula process num func anônimo pra que panic em um job
		// não derrube o worker inteiro.
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("export worker: panic recovered",
						"job_id", job.ID, "panic", r)
					_ = job.MarkFailed("worker panic")
					_ = w.repo.Update(ctx, job)
				}
			}()
			if err := w.service.Process(ctx, job, w.bucket, w.uploader); err != nil {
				logger.Warn("export process error", "job_id", job.ID, "error", err.Error())
			}
		}()
	}
}
