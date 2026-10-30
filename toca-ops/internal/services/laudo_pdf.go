package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/pdf"
	"github.com/oginoino/toca/toca-ops/internal/models/inspection"
)

func inspectionTypeLabel(t string) string {
	switch inspection.NormalizeType(t) {
	case inspection.TypeEntry:
		return "Vistoria de Entrada"
	case inspection.TypeExit:
		return "Vistoria de Saída"
	case inspection.TypePeriodic:
		return "Vistoria Periódica"
	}
	return "Vistoria"
}

func conditionBadgeColor(condition string) string {
	switch condition {
	case inspection.ConditionBom:
		return "#16a34a"
	case inspection.ConditionRegular:
		return "#d97706"
	case inspection.ConditionRuim:
		return "#dc2626"
	}
	return "#64748b"
}

func BuildLaudoHTML(insp *inspection.Inspection) string {
	esc := html.EscapeString
	var b strings.Builder

	b.WriteString(`<!DOCTYPE html><html lang="pt-BR"><head><meta charset="utf-8">`)
	b.WriteString(`<style>`)
	b.WriteString(`
		* { box-sizing: border-box; }
		body { font-family: -apple-system, "Segoe UI", Roboto, sans-serif; color: #0f172a; margin: 0; padding: 32px; }
		.header { border-bottom: 3px solid #0f172a; padding-bottom: 16px; margin-bottom: 24px; }
		.header h1 { font-size: 22px; margin: 0 0 4px; }
		.header .meta { font-size: 12px; color: #64748b; }
		.summary { background: #f8fafc; border: 1px solid #e2e8f0; border-radius: 8px; padding: 16px; margin-bottom: 24px; }
		.summary dl { margin: 0; display: grid; grid-template-columns: 1fr 1fr; gap: 8px 24px; }
		.summary dt { font-size: 10px; text-transform: uppercase; letter-spacing: 0.5px; color: #94a3b8; }
		.summary dd { margin: 0 0 8px; font-size: 13px; font-weight: 600; }
		.finding { border: 1px solid #e2e8f0; border-radius: 8px; padding: 14px; margin-bottom: 14px; page-break-inside: avoid; }
		.finding-head { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 6px; }
		.finding-room { font-size: 14px; font-weight: 700; }
		.finding-desc { font-size: 12px; color: #475569; }
		.badge { display: inline-block; padding: 3px 10px; border-radius: 999px; font-size: 10px; font-weight: 700; color: #fff; }
		.notes { font-size: 11px; color: #64748b; font-style: italic; margin-top: 6px; }
		.photos { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; }
		.photos img { width: 150px; height: 110px; object-fit: cover; border-radius: 6px; border: 1px solid #e2e8f0; }
		.no-photos { font-size: 10px; color: #cbd5e1; }
		.footer { margin-top: 32px; padding-top: 16px; border-top: 1px solid #e2e8f0; font-size: 10px; color: #94a3b8; text-align: center; }
		.empty { font-size: 13px; color: #94a3b8; font-style: italic; }
	`)
	b.WriteString(`</style></head><body>`)

	b.WriteString(`<div class="header">`)
	b.WriteString(`<h1>` + esc(inspectionTypeLabel(insp.Type)) + `</h1>`)
	b.WriteString(`<div class="meta">Laudo gerado em ` +
		esc(time.Now().Format("02/01/2006 15:04")) + ` · Suitee</div>`)
	b.WriteString(`</div>`)

	b.WriteString(`<div class="summary"><dl>`)
	writeSummaryItem(&b, esc, "Imóvel", insp.PropertyID)
	writeSummaryItem(&b, esc, "Contrato", insp.ContractID)
	writeSummaryItem(&b, esc, "Vistoriador", insp.InspectorName)
	writeSummaryItem(&b, esc, "Data agendada", insp.ScheduledDate.Format("02/01/2006"))
	if insp.CompletedDate != nil {
		writeSummaryItem(&b, esc, "Data de conclusão", insp.CompletedDate.Format("02/01/2006"))
	}
	writeSummaryItem(&b, esc, "Status", insp.Status)
	b.WriteString(`</dl></div>`)

	if strings.TrimSpace(insp.Notes) != "" {
		b.WriteString(`<div class="summary"><dt>Observações gerais</dt>`)
		b.WriteString(`<dd style="font-weight:400;white-space:pre-wrap;">` + esc(insp.Notes) + `</dd></div>`)
	}

	b.WriteString(`<h2 style="font-size:16px;margin:24px 0 12px;">Itens vistoriados (` +
		fmt.Sprintf("%d", len(insp.Findings)) + `)</h2>`)
	if len(insp.Findings) == 0 {
		b.WriteString(`<p class="empty">Nenhum item registrado nesta vistoria.</p>`)
	}
	for _, f := range insp.Findings {
		b.WriteString(`<div class="finding">`)
		b.WriteString(`<div class="finding-head">`)
		b.WriteString(`<div>`)
		b.WriteString(`<div class="finding-room">` + esc(orEmDash(f.Room)) + `</div>`)
		if strings.TrimSpace(f.Description) != "" {
			b.WriteString(`<div class="finding-desc">` + esc(f.Description) + `</div>`)
		}
		b.WriteString(`</div>`)
		cond := strings.TrimSpace(f.Condition)
		if cond == "" {
			cond = "Não informado"
		}
		b.WriteString(`<span class="badge" style="background:` + conditionBadgeColor(f.Condition) + `;">` +
			esc(cond) + `</span>`)
		b.WriteString(`</div>`)

		if strings.TrimSpace(f.Notes) != "" {
			b.WriteString(`<div class="notes">` + esc(f.Notes) + `</div>`)
		}

		if len(f.Photos) > 0 {
			b.WriteString(`<div class="photos">`)
			for _, p := range f.Photos {
				if strings.TrimSpace(p) == "" {
					continue
				}
				b.WriteString(`<img src="` + html.EscapeString(p) + `" alt="foto">`)
			}
			b.WriteString(`</div>`)
		} else {
			b.WriteString(`<div class="no-photos">Sem fotos anexadas.</div>`)
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`<div class="footer">`)
	b.WriteString(`Documento gerado automaticamente pela plataforma Suitee. `)
	b.WriteString(`ID da vistoria: ` + esc(insp.ID))
	b.WriteString(`</div>`)

	b.WriteString(`</body></html>`)
	return b.String()
}

func writeSummaryItem(b *strings.Builder, esc func(string) string, label, value string) {
	b.WriteString(`<dt>` + esc(label) + `</dt>`)
	b.WriteString(`<dd>` + esc(orEmDash(value)) + `</dd>`)
}

func orEmDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

var ErrPDFRendererUnavailable = errors.New("pdf renderer not available (GOTENBERG_URL unset)")

// GenerateLaudoPDF carrega a vistoria *antes* de checar renderer/storage: o
// portão de tenant não pode depender da configuração do ambiente, e um id de
// outro tenant precisa responder 404 mesmo com o renderer desligado.
func (s *InspectionService) GenerateLaudoPDF(ctx context.Context, tenantID, inspectionID string) (string, error) {
	insp, err := s.loadTenantInspection(ctx, tenantID, inspectionID)
	if err != nil {
		return "", err
	}
	if s.pdfRenderer == nil || !s.pdfRenderer.Available() {
		return "", ErrPDFRendererUnavailable
	}
	if s.storage == nil {
		return "", errors.New("storage service not configured")
	}

	htmlDoc := BuildLaudoHTML(insp)
	pdfBytes, err := s.pdfRenderer.RenderHTML(ctx, "laudo-"+insp.ID, htmlDoc)
	if err != nil {
		return "", fmt.Errorf("render laudo pdf: %w", err)
	}

	objectName := fmt.Sprintf("laudos/%s/laudo-%s.pdf", insp.TenantID, insp.ID)
	url, err := s.storage.Upload(ctx, InspectionBucket, objectName, "application/pdf", bytes.NewReader(pdfBytes))
	if err != nil {
		return "", fmt.Errorf("upload laudo pdf: %w", err)
	}
	return url, nil
}

// GenerateAndAttachLaudo gera o PDF e grava o ReportURL. A releitura que
// antecede a escrita também é escopada pelo tenant do chamador: é exatamente
// aqui que a versão anterior sobrescrevia o laudo de outro tenant (auditoria
// C8), porque o UPDATE usava o tenant do próprio registro carregado por id.
func (s *InspectionService) GenerateAndAttachLaudo(ctx context.Context, tenantID, inspectionID string) (*inspection.Inspection, error) {
	url, err := s.GenerateLaudoPDF(ctx, tenantID, inspectionID)
	if err != nil {
		return nil, err
	}
	insp, err := s.loadTenantInspection(ctx, tenantID, inspectionID)
	if err != nil {
		return nil, err
	}
	insp.ReportURL = url
	insp.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, insp); err != nil {
		return nil, err
	}
	return insp, nil
}

var _ pdf.Renderer = pdf.NoopRenderer{}
