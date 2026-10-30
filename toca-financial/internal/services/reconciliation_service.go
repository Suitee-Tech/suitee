package services

import (
	"context"

	"github.com/oginoino/toca/toca-financial/internal/domain/report"
)

// ReconciliationService executa o relatório de reconciliação dry-run
// que detecta duplicatas, órfãos, excesso, subalocação e inconsistências
// de status cascade (FIN-COM-14).
type ReconciliationService struct {
	repo ReconciliationRepository
}

// ReconciliationRepository é a interface que o repositório de
// reconciliação implementa. Desacoplada para permitir testes.
type ReconciliationRepository interface {
	Run(ctx context.Context, tenantID string) (*report.ReconciliationReport, error)
}

func NewReconciliationService(repo ReconciliationRepository) *ReconciliationService {
	return &ReconciliationService{repo: repo}
}

// Run executa todas as verificações de reconciliação e retorna o
// relatório dry-run. Se tenantID for vazio, aplica-se ao tenant do
// contexto (handler injeta antes de chamar).
func (s *ReconciliationService) Run(ctx context.Context, tenantID string) (*report.ReconciliationReport, error) {
	return s.repo.Run(ctx, tenantID)
}
