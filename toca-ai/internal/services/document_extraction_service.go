package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-ai/internal/domain"
)

// ExtractionService orquestra OCR (docling) -> LLM (DeepSeek JSON mode) ->
// validação determinística -> confiança composta. Nunca panica: falhas de OCR/LLM
// viram ExtractedDocument com Status "failed"/"empty" (fallback do plano).
type ExtractionService struct {
	ocr      *OCRClient
	deepseek *DeepseekClient
	prompts  *ExtractionPromptBuilder
	model    string
	maxTok   int
	timeout  time.Duration
}

func NewExtractionService(ocr *OCRClient, deepseek *DeepseekClient, prompts *ExtractionPromptBuilder, model string) *ExtractionService {
	return &ExtractionService{
		ocr:      ocr,
		deepseek: deepseek,
		prompts:  prompts,
		model:    model,
		maxTok:   2000,
		timeout:  100 * time.Second,
	}
}

type extLLMField struct {
	Name       string  `json:"name"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
	Evidence   string  `json:"evidence"`
}

// Extract roda o pipeline completo. Retorna sempre um *ExtractedDocument; o erro
// só é não-nil para falhas verdadeiramente inesperadas.
func (s *ExtractionService) Extract(ctx context.Context, req domain.ExtractDocumentRequest) (*domain.ExtractedDocument, error) {
	start := time.Now()
	result := &domain.ExtractedDocument{
		DocumentID:   req.DocumentID,
		DocumentType: req.DocumentType,
		Model:        s.model,
		OCRProvider:  "docling",
		CreatedAt:    time.Now().UTC(),
		Fields:       []domain.ExtractedField{},
	}
	finish := func(status, errMsg string) *domain.ExtractedDocument {
		result.Status = status
		result.Error = errMsg
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}

	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	// 1. OCR
	ocrRes, err := s.ocr.Parse(ctx, req.DocumentURL)
	if err != nil {
		return finish(domain.ExtractionStatusFailed, "ocr: "+err.Error()), nil
	}
	result.OCRConfidence = ocrRes.OCRConfidence

	source := strings.TrimSpace(ocrRes.Markdown)
	if source == "" {
		source = strings.TrimSpace(ocrRes.Text)
	}
	if source == "" {
		return finish(domain.ExtractionStatusEmpty, ""), nil
	}

	// 2. LLM (JSON mode)
	messages := s.prompts.Build(req.DocumentType, req.Context, req.TargetFields, source)
	msg, _, err := s.deepseek.ChatJSON(ctx, messages, s.maxTok)
	if err != nil {
		return finish(domain.ExtractionStatusFailed, "llm: "+err.Error()), nil
	}

	// 3. Parse defensivo
	raw, perr := extParseFields(msg.Content)
	if perr != nil {
		return finish(domain.ExtractionStatusFailed, "parse: "+perr.Error()), nil
	}
	rawByName := make(map[string]extLLMField, len(raw))
	for _, f := range raw {
		rawByName[strings.TrimSpace(f.Name)] = f
	}

	// 4. Normaliza + valida + confiança composta, campo a campo do catálogo.
	var confs []float64
	for _, spec := range req.TargetFields {
		ef := s.buildField(spec, rawByName[spec.Name], ocrRes.OCRConfidence)
		result.Fields = append(result.Fields, ef)
		if ef.ValidationStatus != domain.ValidationEmpty {
			confs = append(confs, ef.Confidence)
		}
	}
	result.OverallConfidence = extMean(confs)
	return finish(domain.ExtractionStatusCompleted, ""), nil
}

func (s *ExtractionService) buildField(spec domain.FieldSpec, lf extLLMField, ocrConf *float64) domain.ExtractedField {
	ef := domain.ExtractedField{
		Name:     spec.Name,
		Label:    spec.Label,
		Target:   spec.Target,
		Source:   domain.SourceOCR,
		RawValue: strings.TrimSpace(lf.Value),
		Evidence: strings.TrimSpace(lf.Evidence),
	}

	normalized := extNormalizeValue(spec.Type, lf.Value)
	ef.Value = normalized
	if normalized == "" {
		ef.Confidence = 0
		ef.Tier = domain.TierLow
		ef.ValidationStatus = domain.ValidationEmpty
		return ef
	}

	llmConf := extClamp01(lf.Confidence)
	base, status, vmsg := extValidateField(spec.Type, normalized, llmConf)
	ef.ValidationStatus = status
	ef.ValidationMessage = vmsg

	conf := base
	if ocrConf != nil {
		conf = base * (0.5 + 0.5*extClamp01(*ocrConf))
	}
	ef.Confidence = extClamp01(conf)
	ef.Tier = extTier(ef.Confidence)
	return ef
}

// --------------------------------------------------------------------------- //
// Parsing / normalização / validação
// --------------------------------------------------------------------------- //

var (
	extFenceRe  = regexp.MustCompile("```(?:json)?")
	extDigitsRe = regexp.MustCompile(`\D+`)
	extDMYRe    = regexp.MustCompile(`^(\d{2})[/-](\d{2})[/-](\d{4})`)
	extISORe    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)
	extEmailRe  = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

func extParseFields(content string) ([]extLLMField, error) {
	s := extFenceRe.ReplaceAllString(strings.TrimSpace(content), "")
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i {
		s = s[i : j+1]
	}
	var wrap struct {
		Fields []extLLMField `json:"fields"`
	}
	if err := json.Unmarshal([]byte(s), &wrap); err != nil {
		return nil, err
	}
	return wrap.Fields, nil
}

func extNormalizeValue(typ, v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	switch typ {
	case "cpf_cnpj", "cep", "phone":
		return extDigitsRe.ReplaceAllString(v, "")
	case "date":
		return extNormalizeDate(v)
	case "email":
		return strings.ToLower(v)
	case "uf":
		return strings.ToUpper(v)
	default:
		return strings.Join(strings.Fields(v), " ")
	}
}

func extNormalizeDate(v string) string {
	v = strings.TrimSpace(v)
	if m := extDMYRe.FindStringSubmatch(v); m != nil {
		return fmt.Sprintf("%s-%s-%s", m[3], m[2], m[1])
	}
	if extISORe.MatchString(v) {
		return v[:10]
	}
	return v
}

// extValidateField devolve (base, status, mensagem). base é a confiança após a
// checagem determinística, antes do fator de OCR.
func extValidateField(typ, normalized string, llmConf float64) (float64, string, string) {
	switch typ {
	case "cpf_cnpj":
		ok := false
		switch len(normalized) {
		case 11:
			ok = validation.IsValidCPF(normalized)
		case 14:
			ok = validation.IsValidCNPJ(normalized)
		}
		if ok {
			return max(llmConf, 0.95), domain.ValidationValid, ""
		}
		return min(llmConf, 0.30), domain.ValidationInvalid, "CPF/CNPJ inválido"
	case "cep":
		if len(normalized) == 8 {
			return max(llmConf, 0.90), domain.ValidationValid, ""
		}
		return min(llmConf, 0.30), domain.ValidationInvalid, "CEP inválido"
	case "date":
		if extIsValidISODate(normalized) {
			return max(llmConf, 0.90), domain.ValidationValid, ""
		}
		return min(llmConf, 0.30), domain.ValidationInvalid, "Data inválida"
	case "email":
		if extEmailRe.MatchString(normalized) {
			return max(llmConf, 0.90), domain.ValidationValid, ""
		}
		return min(llmConf, 0.30), domain.ValidationInvalid, "E-mail inválido"
	case "phone":
		if n := len(normalized); n >= 10 && n <= 13 {
			return max(llmConf, 0.85), domain.ValidationValid, ""
		}
		return min(llmConf, 0.40), domain.ValidationInvalid, "Telefone inválido"
	case "uf":
		if len(normalized) == 2 {
			return max(llmConf, 0.85), domain.ValidationValid, ""
		}
		return min(llmConf, 0.40), domain.ValidationInvalid, "UF inválida"
	default:
		return llmConf, domain.ValidationUnchecked, ""
	}
}

func extIsValidISODate(s string) bool {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return false
	}
	y := t.Year()
	return y >= 1900 && y <= time.Now().UTC().Year()
}

func extClamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

func extMean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func extTier(c float64) string {
	switch {
	case c >= 0.85:
		return domain.TierHigh
	case c >= 0.60:
		return domain.TierMedium
	default:
		return domain.TierLow
	}
}
