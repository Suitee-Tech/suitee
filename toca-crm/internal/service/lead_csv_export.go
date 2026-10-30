package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strings"
	"time"

	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/exports"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

// LeadEntityType — identifier do generator no Registry. Issue #474.
//
// Mantemos como constante exportada para que o handler que cria o
// export job não acabe digitando "Lead" como string mágica.
const LeadEntityType exports.EntityType = "Lead"

// LeadCSVGenerator gera o CSV de leads. Implementa exports.Generator.
//
// Filtros suportados (Job.Filters): status, source, broker_id,
// created_from, created_to. Mais filtros caem direto da LeadFilter
// existente — bastando expor mais campos no handler. CSV mantém PII
// (nome/email/telefone) porque o usuário pediu explicitamente o
// export — diferente do audit log, que é retido 5y; CSVs vão pra
// storage com presigned URL de 7d.
type LeadCSVGenerator struct {
	repo lead.Repository
}

func NewLeadCSVGenerator(repo lead.Repository) *LeadCSVGenerator {
	return &LeadCSVGenerator{repo: repo}
}

// MaxLeadRowsPerExport — limite defensivo. Tenants com 100k leads
// rodariam o servidor pra lugar nenhum; melhor falhar rápido pedindo
// filtros mais estreitos. Override via env caso necessário no futuro.
const MaxLeadRowsPerExport = 50000

// Generate carrega leads e renderiza CSV. Implementação simples
// in-memory: pra 50k linhas dá ~5MB que cabe folgado. Streaming
// (chunked upload) fica pra refactor quando passarmos disso.
func (g *LeadCSVGenerator) Generate(ctx context.Context, job *exports.Job) ([]byte, string, int64, error) {
	// Hidrata tenant context para que o repo aplique RLS via service pool.
	ctx = pkgcontext.WithTenantID(ctx, job.TenantID)

	filter := lead.LeadFilter{
		Limit: int64(MaxLeadRowsPerExport),
	}
	if v, ok := job.Filters["status"].(string); ok && v != "" {
		filter.Status = v
	}
	if v, ok := job.Filters["source"].(string); ok && v != "" {
		filter.Source = v
	}
	if v, ok := job.Filters["assigned_broker_id"].(string); ok && v != "" {
		filter.AssignedBrokerID = v
	}
	if v, ok := job.Filters["created_from"].(string); ok && v != "" {
		t, perr := time.Parse(time.RFC3339, v)
		if perr == nil {
			filter.CreatedFrom = &t
		}
	}
	if v, ok := job.Filters["created_to"].(string); ok && v != "" {
		t, perr := time.Parse(time.RFC3339, v)
		if perr == nil {
			filter.CreatedTo = &t
		}
	}

	leads, total, err := g.repo.List(ctx, job.TenantID, filter)
	if err != nil {
		return nil, "", 0, fmt.Errorf("list leads: %w", err)
	}
	if total > MaxLeadRowsPerExport {
		return nil, "", 0, fmt.Errorf("export exceeds limit (%d > %d) — aplique filtros mais estreitos", total, MaxLeadRowsPerExport)
	}

	var buf bytes.Buffer
	// UTF-8 BOM para Excel (Windows) reconhecer acentos sem prompt
	// de encoding. Sem isso, "São Paulo" vira "S?o Paulo" no Excel.
	buf.WriteString("\uFEFF")

	w := csv.NewWriter(&buf)
	w.Comma = ';' // separador padrão BR (Excel BR usa ';' por default)

	// Header. Ordem fixa pra que callers consumindo o CSV
	// (planilhas de gestão) possam confiar na coluna sem name lookup.
	header := []string{
		"id", "display_id", "nome", "email", "telefone",
		"status", "source", "role",
		"assigned_broker_id", "is_client",
		"created_at", "assigned_at",
		"property_id", "contract_id",
	}
	if err := w.Write(header); err != nil {
		return nil, "", 0, err
	}

	for _, l := range leads {
		row := []string{
			l.ID,
			l.DisplayID,
			l.Name,
			l.Email,
			l.Phone,
			string(l.Status),
			string(l.Source),
			string(l.Role),
			l.AssignedBrokerID,
			boolStr(l.IsClient),
			l.CreatedAt.Format(time.RFC3339),
			timeOrEmpty(l.AssignedAt),
			firstPropertyID(l),
			l.ContractID,
		}
		if err := w.Write(row); err != nil {
			return nil, "", 0, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, "", 0, err
	}

	// Path inclui tenantID + jobID + timestamp para evitar colisão e
	// pra investigação ler "arquivo X é do tenant Y" sem abrir o blob.
	filePath := fmt.Sprintf("leads/%s/%s_leads_%s.csv",
		job.TenantID,
		time.Now().UTC().Format("20060102_150405"),
		job.ID[:8],
	)
	return buf.Bytes(), filePath, int64(len(leads)), nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func timeOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func firstPropertyID(l *lead.Lead) string {
	if len(l.PropertyIDs) > 0 {
		return l.PropertyIDs[0]
	}
	return ""
}

// CSVPayload — helper exportado para testes (impressão amigável).
func CSVPayload(content []byte) string {
	// Strip BOM para asserções de teste.
	return strings.TrimPrefix(string(content), "\uFEFF")
}
