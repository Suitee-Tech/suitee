package services

import (
	"context"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/domain/document"
)

// autoApplyConfidenceThreshold — campos com confiança >= isso são auto-aplicados
// na party (origem = IA). Abaixo, ficam só na extração para revisão do operador.
const autoApplyConfidenceThreshold = 0.85

var extractionDigits = regexp.MustCompile(`\D+`)

// NewDocumentExtractionHandler retorna o handler de `document.uploaded`: roda
// OCR+IA no documento, persiste o resultado e auto-aplica campos de alta
// confiança na party-alvo (quando resolvível). Best-effort + idempotente por
// document_id (a fila própria + DLQ cuidam de retry).
func NewDocumentExtractionHandler(docs *DocumentService, deals *DealService, ai *clients.AIClient) events.EventHandler {
	return func(ctx context.Context, evt events.Event) error {
		payload, ok := evt.Payload().(map[string]interface{})
		if !ok {
			log.Printf("document.uploaded: unexpected payload type %T", evt.Payload())
			return nil
		}
		documentID := stringField(payload, "document_id")
		tenantID := stringField(payload, "tenant_id")
		if documentID == "" || tenantID == "" {
			log.Printf("document.uploaded: missing required fields, payload=%v", payload)
			return nil
		}
		category := stringField(payload, "category")
		entityType := stringField(payload, "entity_type")
		entityID := stringField(payload, "entity_id")

		ctx = pkgctx.WithTenantID(ctx, tenantID)

		if ai == nil {
			log.Printf("document.uploaded: AI client not configured, skipping doc=%s", documentID)
			return nil
		}

		// Idempotência: não reprocessa documentos já em estado terminal.
		// O tenant veio do payload e já está no ctx (linha acima) — o
		// documento é resolvido dentro dele (achado C11).
		if existing, err := docs.GetByID(ctx, tenantID, documentID); err == nil && existing != nil {
			switch existing.ExtractionStatus {
			case document.ExtractionCompleted, document.ExtractionEmpty:
				return nil
			}
		}

		_ = docs.SetExtractionStatus(ctx, documentID, document.ExtractionProcessing, "")

		url, err := docs.GetDownloadURL(ctx, documentID, tenantID)
		if err != nil || strings.TrimSpace(url) == "" {
			msg := "presigned url indisponível"
			if err != nil {
				msg = err.Error()
			}
			_ = docs.SetExtraction(ctx, documentID, failedExtraction(msg), document.ExtractionFailed, msg)
			return nil
		}

		// Resolve a party-alvo (best-effort) para conhecer o tipo (pessoa/empresa)
		// e, depois, auto-aplicar. Só tratamos documentos ligados a um deal no v1.
		dealID, parties := loadDealParties(ctx, deals, tenantID, entityType, entityID)
		partyKind := deal.PartyKindPerson
		if len(parties) == 1 && parties[0].Kind != "" {
			partyKind = parties[0].Kind
		}

		res, err := ai.ExtractDocument(ctx, clients.ExtractDocumentRequest{
			DocumentID:   documentID,
			DocumentURL:  url,
			DocumentType: category,
			TenantID:     tenantID,
			Context: clients.AIDocumentContext{
				Category:   category,
				EntityType: entityType,
				EntityID:   entityID,
				PartyKind:  string(partyKind),
			},
			TargetFields: PartyFieldCatalog(partyKind),
		})
		if err != nil {
			msg := err.Error()
			_ = docs.SetExtraction(ctx, documentID, failedExtraction(msg), document.ExtractionFailed, msg)
			return nil
		}

		ext := mapExtraction(res)
		status := terminalStatus(res.Status)

		// Auto-aplica campos de alta confiança quando há party resolvível.
		if status == document.ExtractionCompleted {
			if partyID := pickTargetParty(parties, res); partyID != "" && dealID != "" {
				if applyFields := highConfidenceFields(res); len(applyFields) > 0 {
					if _, aerr := deals.ApplyExtractedFields(ctx, tenantID, dealID, "", partyID, applyFields, deal.ProvenanceAI, documentID, "system"); aerr != nil {
						log.Printf("document.uploaded: auto-apply failed doc=%s party=%s: %v", documentID, partyID, aerr)
					} else {
						ext.AppliedPartyID = partyID
						markApplied(ext, applyFields)
					}
				}
			}
		}

		_ = docs.SetExtraction(ctx, documentID, ext, status, ext.Error)
		return nil
	}
}

// loadDealParties carrega o deal e suas parties em escopo de sistema. Retorna
// dealID vazio quando o documento não está ligado a um deal.
func loadDealParties(ctx context.Context, deals *DealService, tenantID, entityType, entityID string) (string, []deal.Party) {
	if deals == nil || entityType != string(document.EntityDeal) || entityID == "" {
		return "", nil
	}
	d, err := deals.GetByID(ctx, tenantID, entityID, "")
	if err != nil || d == nil {
		return "", nil
	}
	return d.ID, d.Parties
}

// pickTargetParty escolhe a party que deve receber os dados: prioriza o match do
// CPF/CNPJ extraído com o de uma party; senão, a única party do deal; senão, vazio.
func pickTargetParty(parties []deal.Party, res *clients.AIExtractedDocument) string {
	if len(parties) == 0 || res == nil {
		return ""
	}
	for _, f := range res.Fields {
		if f.Name != "document" || f.ValidationStatus != "valid" {
			continue
		}
		want := extractionDigits.ReplaceAllString(f.Value, "")
		if want == "" {
			continue
		}
		for i := range parties {
			if extractionDigits.ReplaceAllString(parties[i].Document, "") == want {
				return parties[i].ID
			}
		}
	}
	if len(parties) == 1 {
		return parties[0].ID
	}
	return ""
}

// highConfidenceFields seleciona os campos auto-aplicáveis (alta confiança, não
// inválidos, com valor).
func highConfidenceFields(res *clients.AIExtractedDocument) []ExtractedFieldApply {
	out := make([]ExtractedFieldApply, 0, len(res.Fields))
	for _, f := range res.Fields {
		if f.Confidence < autoApplyConfidenceThreshold || f.ValidationStatus == "invalid" {
			continue
		}
		if strings.TrimSpace(f.Value) == "" {
			continue
		}
		out = append(out, ExtractedFieldApply{
			Name:       f.Name,
			Target:     f.Target,
			Value:      f.Value,
			Confidence: f.Confidence,
		})
	}
	return out
}

func mapExtraction(res *clients.AIExtractedDocument) *document.DocumentExtraction {
	ext := &document.DocumentExtraction{
		Status:            res.Status,
		Error:             res.Error,
		OverallConfidence: res.OverallConfidence,
		Model:             res.Model,
		OCRProvider:       res.OCRProvider,
		OCRConfidence:     res.OCRConfidence,
		ExtractedAt:       time.Now().UTC(),
		Fields:            make([]document.ExtractedField, 0, len(res.Fields)),
	}
	for _, f := range res.Fields {
		ext.Fields = append(ext.Fields, document.ExtractedField{
			Name:              f.Name,
			Label:             f.Label,
			Target:            f.Target,
			Value:             f.Value,
			RawValue:          f.RawValue,
			Evidence:          f.Evidence,
			Confidence:        f.Confidence,
			Tier:              f.Tier,
			Source:            f.Source,
			ValidationStatus:  f.ValidationStatus,
			ValidationMessage: f.ValidationMessage,
		})
	}
	return ext
}

// markApplied marca como aplicados os campos auto-gravados na party.
func markApplied(ext *document.DocumentExtraction, applied []ExtractedFieldApply) {
	names := make(map[string]struct{}, len(applied))
	for _, a := range applied {
		names[a.Name] = struct{}{}
	}
	for i := range ext.Fields {
		if _, ok := names[ext.Fields[i].Name]; ok {
			ext.Fields[i].Applied = true
		}
	}
}

func failedExtraction(msg string) *document.DocumentExtraction {
	return &document.DocumentExtraction{
		Status:      string(document.ExtractionFailed),
		Error:       msg,
		Fields:      []document.ExtractedField{},
		ExtractedAt: time.Now().UTC(),
	}
}

func terminalStatus(aiStatus string) document.ExtractionStatus {
	switch aiStatus {
	case "failed":
		return document.ExtractionFailed
	case "empty":
		return document.ExtractionEmpty
	default:
		return document.ExtractionCompleted
	}
}
