package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/pdf"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/toca-ops/internal/models/inspection"
	"github.com/oginoino/toca/toca-ops/internal/models/supplier"
)

const InspectionBucket = "toca-inspections"

var ErrFindingNotFound = errors.New("finding not found in inspection")
var ErrTooManyPhotos = errors.New("max 5 photos per upload request")

// ErrInspectionNotFound é a resposta "fecha o portão" (fail closed) para uma
// vistoria que não existe *para o tenant do chamador*. Id inexistente, tenant
// estrangeiro e tenant vazio colapsam no mesmo erro para que a resposta HTTP
// não sirva de oráculo de existência entre tenants (auditoria C8).
var ErrInspectionNotFound = errors.New("inspection not found")

type InspectionService struct {
	suppliers   supplier.Repository
	repo        inspection.Repository
	eventBus    events.EventBus
	storage     storage.Service
	pdfRenderer pdf.Renderer
}

func NewInspectionService(repo inspection.Repository, eventBus events.EventBus) *InspectionService {
	return &InspectionService{repo: repo, eventBus: eventBus}
}

// WithSuppliers validates selected professionals within the authenticated company.
func (s *InspectionService) WithSuppliers(repo supplier.Repository) *InspectionService {
	s.suppliers = repo
	return s
}

var ErrInvalidInspector = errors.New("selecione um profissional ativo do tipo Vistoria ou Vistoriador")

func (s *InspectionService) validateInspector(ctx context.Context, insp *inspection.Inspection) error {
	if s.suppliers == nil {
		return nil
	}
	v, err := s.suppliers.GetByID(ctx, insp.TenantID, insp.InspectorID)
	if err != nil || v == nil || !v.Active {
		return ErrInvalidInspector
	}
	switch strings.ToLower(strings.TrimSpace(v.ServiceType)) {
	case "vistoria", "vistorias", "vistoriador", "vistoriadora":
	default:
		return ErrInvalidInspector
	}
	insp.InspectorName = v.Name
	return nil
}

// Public creation requires a registered supplier. Internal contract automation
// may still create unassigned inspections through CreateScheduled.
func (s *InspectionService) CreateAssigned(ctx context.Context, insp *inspection.Inspection) error {
	if err := s.validateInspector(ctx, insp); err != nil {
		return err
	}
	return s.Create(ctx, insp)
}

func (s *InspectionService) WithStorage(st storage.Service) *InspectionService {
	s.storage = st
	return s
}

func (s *InspectionService) WithPDFRenderer(r pdf.Renderer) *InspectionService {
	s.pdfRenderer = r
	return s
}

func (s *InspectionService) Create(ctx context.Context, insp *inspection.Inspection) error {
	if insp.PropertyID == "" {
		return fmt.Errorf("property_id is required")
	}
	if err := inspection.ValidateType(insp.Type); err != nil {
		return err
	}
	insp.Type = inspection.NormalizeType(insp.Type)
	if err := s.repo.Create(ctx, insp); err != nil {
		return err
	}
	s.publishScheduled(ctx, insp)
	return nil
}

// loadTenantInspection é o único caminho de leitura de uma vistoria por id. O
// tenant vem do chamador autenticado (nunca do registro) e qualquer falha —
// inclusive erro de banco — vira ErrInspectionNotFound: o detalhe fica no log,
// não na resposta, para não distinguir "não existe" de "é de outro tenant".
func (s *InspectionService) loadTenantInspection(ctx context.Context, tenantID, id string) (*inspection.Inspection, error) {
	insp, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		log.Printf("[inspection] load failed (tenant=%q id=%q): %v", tenantID, id, err)
		return nil, ErrInspectionNotFound
	}
	if insp == nil {
		return nil, ErrInspectionNotFound
	}
	return insp, nil
}

func (s *InspectionService) CreateScheduled(ctx context.Context, id string, insp *inspection.Inspection) error {
	if id != "" {
		// Caminho interno (consumidor de deal.contract.signed): não há tenant de
		// request, mas o payload do evento traz o tenant do contrato, então a
		// checagem de idempotência é escopada por ele — não é preciso um
		// GetByIDAnyTenant.
		existing, err := s.repo.GetByID(ctx, insp.TenantID, id)
		if err == nil && existing != nil {
			return nil
		}
		insp.ID = id
	}
	return s.Create(ctx, insp)
}

func (s *InspectionService) GetByID(ctx context.Context, tenantID, id string) (*inspection.Inspection, error) {
	return s.loadTenantInspection(ctx, tenantID, id)
}

func (s *InspectionService) Update(ctx context.Context, insp *inspection.Inspection) error {
	existing, err := s.loadTenantInspection(ctx, insp.TenantID, insp.ID)
	if err != nil {
		return err
	}
	if insp.Type != "" {
		if err := inspection.ValidateType(insp.Type); err != nil {
			return err
		}
		insp.Type = inspection.NormalizeType(insp.Type)
	}
	if insp.InspectorID != existing.InspectorID || insp.InspectorName != existing.InspectorName || !insp.ScheduledDate.Equal(existing.ScheduledDate) {
		if err := s.validateInspector(ctx, insp); err != nil {
			return err
		}
	}
	insp.ScheduledBy = existing.ScheduledBy
	if insp.TimeZone == "" {
		insp.TimeZone = existing.TimeZone
	}
	insp.TenantID = existing.TenantID
	insp.CreatedAt = existing.CreatedAt
	if err := s.repo.Update(ctx, insp); err != nil {
		return err
	}
	if existing.Status != inspection.StatusConcluida && insp.Status == inspection.StatusConcluida && insp.ReportURL != "" {
		if insp.CompletedDate == nil {
			now := time.Now()
			insp.CompletedDate = &now
		}
		s.publishCompleted(ctx, insp)
	}
	if existing.Status != inspection.StatusCancelada && insp.Status == inspection.StatusCancelada {
		s.publishCancelled(ctx, insp)
	}
	return nil
}

func (s *InspectionService) Delete(ctx context.Context, tenantID, id string) error {
	// P2/item 36: o tenant desce até a escrita (não basta o GetByID escopado que
	// o handler fazia antes de chamar).
	return s.repo.Delete(ctx, tenantID, id)
}

func (s *InspectionService) List(ctx context.Context, f inspection.ListFilter) ([]*inspection.Inspection, int64, error) {
	return s.repo.List(ctx, f)
}

func (s *InspectionService) RecordDeductions(ctx context.Context, tenantID, id string, deductions []inspection.Deduction) (*inspection.Inspection, error) {
	if len(deductions) == 0 {
		return nil, fmt.Errorf("at least one deduction is required")
	}
	insp, err := s.loadTenantInspection(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if insp.Type != inspection.TypeExit {
		return nil, fmt.Errorf("deductions are only valid on EXIT inspections (got %s)", insp.Type)
	}
	if insp.Status != inspection.StatusConcluida {
		return nil, fmt.Errorf("deductions require the inspection to be Concluída first")
	}
	insp.Deductions = append(insp.Deductions, deductions...)
	insp.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, insp); err != nil {
		return nil, err
	}

	if s.eventBus != nil {
		total := 0.0
		lines := make([]contracts.DeductionLine, 0, len(deductions))
		for _, d := range deductions {
			total += d.Amount
			lines = append(lines, contracts.DeductionLine{
				Room:        d.Room,
				Description: d.Description,
				Amount:      d.Amount,
			})
		}
		evt := contracts.InspectionDeductionsRecordedEvent{
			EventIDVal: uuid.New().String(),
			Data: contracts.InspectionDeductionsRecordedEventPayload{
				InspectionID: insp.ID,
				TenantID:     insp.TenantID,
				ContractID:   insp.ContractID,
				Deductions:   lines,
				TotalAmount:  total,
				Timestamp:    time.Now().UTC().Format(time.RFC3339),
			},
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to publish inspection.deductions_recorded for id=%s: %v", insp.ID, err)
		}
	}

	return insp, nil
}

type FindingPhotoUpload struct {
	FileName    string
	ContentType string
	RawBytes    []byte
}

type UploadFindingPhotosResult struct {
	URLs       []string
	Thumbnails []string
}

func (s *InspectionService) UploadFindingPhotos(ctx context.Context, tenantID, inspectionID, findingID, actorID, actorName string, uploads []FindingPhotoUpload) (*UploadFindingPhotosResult, error) {
	if s.storage == nil {
		return nil, errors.New("storage service not configured")
	}
	if len(uploads) == 0 {
		return nil, errors.New("no photos in request")
	}
	if len(uploads) > 5 {
		return nil, ErrTooManyPhotos
	}

	insp, err := s.loadTenantInspection(ctx, tenantID, inspectionID)
	if err != nil {
		// ErrInspectionNotFound é propagado como está (sem wrapper): o handler
		// o reconhece via errors.Is e responde 404 sem distinguir "não existe"
		// de "é de outro tenant".
		return nil, err
	}

	insp.Findings = inspection.EnsureIDs(insp.Findings)
	idx := inspection.FindFindingIndex(insp.Findings, findingID)
	if idx < 0 {
		return nil, ErrFindingNotFound
	}

	urls := make([]string, 0, len(uploads))
	thumbURLs := make([]string, 0, len(uploads))
	for i, up := range uploads {
		if !isAcceptedImageContentType(up.ContentType) {
			log.Printf("[inspection upload] skip photo #%d for finding %s: unsupported content type %q", i, findingID, up.ContentType)
			continue
		}
		processed, _, _, perr := storage.ProcessImage(up.RawBytes, storage.DefaultPhotoOptions)
		if perr != nil {
			log.Printf("[inspection upload] skip photo #%d for finding %s: process error %v", i, findingID, perr)
			continue
		}

		photoUUID := uuid.New().String()
		objectName := path.Join(
			"tenants", sanitize(tenantID),
			"inspections", sanitize(inspectionID),
			sanitize(findingID),
			photoUUID+".jpg",
		)
		url, uerr := s.storage.Upload(ctx, InspectionBucket, objectName, "image/jpeg", bytes.NewReader(processed))
		if uerr != nil {
			log.Printf("[inspection upload] storage upload failed for finding %s: %v", findingID, uerr)
			continue
		}

		thumbProcessed, _, _, terr := storage.ProcessImage(up.RawBytes, storage.DefaultThumbnailOptions)
		var thumbURL string
		if terr != nil {
			log.Printf("[inspection upload] thumbnail process failed for finding %s photo %s: %v (continuing without thumb)", findingID, photoUUID, terr)
		} else {
			thumbObjectName := path.Join(
				"tenants", sanitize(tenantID),
				"inspections", sanitize(inspectionID),
				sanitize(findingID),
				photoUUID+"_thumb.jpg",
			)
			tURL, terr2 := s.storage.Upload(ctx, InspectionBucket, thumbObjectName, "image/jpeg", bytes.NewReader(thumbProcessed))
			if terr2 != nil {
				log.Printf("[inspection upload] thumbnail upload failed for finding %s photo %s: %v (continuing without thumb)", findingID, photoUUID, terr2)
			} else {
				thumbURL = tURL
			}
		}

		urls = append(urls, url)
		thumbURLs = append(thumbURLs, thumbURL)
	}

	if len(urls) == 0 {
		return nil, errors.New("no photos were processed successfully")
	}

	insp.Findings[idx].Photos = append(insp.Findings[idx].Photos, urls...)
	insp.Findings[idx].Thumbnails = append(insp.Findings[idx].Thumbnails, thumbURLs...)
	insp.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, insp); err != nil {
		return &UploadFindingPhotosResult{URLs: urls, Thumbnails: thumbURLs}, fmt.Errorf("persist photos: %w", err)
	}

	thumbsOK := 0
	for _, t := range thumbURLs {
		if t != "" {
			thumbsOK++
		}
	}
	audit.Record(ctx, audit.Entry{
		Action:     "inspection.photos.uploaded",
		EntityType: "Inspection",
		EntityID:   insp.ID,
		Before:     map[string]any{"photo_count": len(insp.Findings[idx].Photos) - len(urls)},
		After:      map[string]any{"photo_count": len(insp.Findings[idx].Photos)},
		Metadata: map[string]any{
			"actor_id":         actorID,
			"actor_name":       actorName,
			"tenant_id":        tenantID,
			"inspection_id":    insp.ID,
			"finding_id":       findingID,
			"new_urls":         urls,
			"new_thumb_urls":   thumbURLs,
			"thumbs_persisted": thumbsOK,
			"requested":        len(uploads),
			"persisted":        len(urls),
		},
	})

	return &UploadFindingPhotosResult{URLs: urls, Thumbnails: thumbURLs}, nil
}

func isAcceptedImageContentType(ct string) bool {
	switch strings.ToLower(strings.TrimSpace(ct)) {
	case "image/jpeg", "image/jpg", "image/png", "image/webp":
		return true
	}
	return false
}

func sanitize(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '-' || c == '_':
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return "unknown"
	}
	return string(out)
}

func (s *InspectionService) HasCompletedExit(ctx context.Context, tenantID, contractID string) (bool, error) {
	if tenantID == "" || contractID == "" {
		return false, nil
	}
	list, _, err := s.repo.List(ctx, inspection.ListFilter{
		TenantID:   tenantID,
		ContractID: contractID,
		Type:       inspection.TypeExit,
		Limit:      50,
	})
	if err != nil {
		return false, err
	}
	for _, i := range list {
		if i == nil {
			continue
		}
		if i.Status == inspection.StatusConcluida && i.ReportURL != "" {
			return true, nil
		}
	}
	return false, nil
}

func (s *InspectionService) publishScheduled(ctx context.Context, i *inspection.Inspection) {
	if s.eventBus == nil {
		return
	}
	evt := contracts.InspectionScheduledEvent{
		EventIDVal: uuid.New().String(),
		Data: contracts.InspectionScheduledEventPayload{
			InspectionID:  i.ID,
			TenantID:      i.TenantID,
			PropertyID:    i.PropertyID,
			ContractID:    i.ContractID,
			Type:          i.Type,
			ScheduledDate: i.ScheduledDate.UTC().Format(time.RFC3339),
			InspectorID:   i.InspectorID,
			Timestamp:     time.Now().UTC().Format(time.RFC3339),
		},
	}
	if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
		log.Printf("WARN: failed to publish inspection.scheduled for id=%s: %v", i.ID, err)
	}
}

func (s *InspectionService) publishCompleted(ctx context.Context, i *inspection.Inspection) {
	if s.eventBus == nil {
		return
	}
	hasDamage := false
	for _, f := range i.Findings {
		if f.Condition != "" && f.Condition != inspection.ConditionBom {
			hasDamage = true
			break
		}
	}
	completedAt := ""
	if i.CompletedDate != nil {
		completedAt = i.CompletedDate.UTC().Format(time.RFC3339)
	}
	evt := contracts.InspectionCompletedEvent{
		EventIDVal: uuid.New().String(),
		Data: contracts.InspectionCompletedEventPayload{
			InspectionID:  i.ID,
			TenantID:      i.TenantID,
			PropertyID:    i.PropertyID,
			ContractID:    i.ContractID,
			Type:          i.Type,
			CompletedAt:   completedAt,
			ReportURL:     i.ReportURL,
			FindingsCount: len(i.Findings),
			HasDamage:     hasDamage,
			Timestamp:     time.Now().UTC().Format(time.RFC3339),
		},
	}
	if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
		log.Printf("WARN: failed to publish inspection.completed for id=%s: %v", i.ID, err)
	}
}

func (s *InspectionService) publishCancelled(ctx context.Context, i *inspection.Inspection) {
	if s.eventBus == nil {
		return
	}
	cancelledAt := time.Now().UTC().Format(time.RFC3339)
	evt := contracts.InspectionCancelledEvent{
		EventIDVal: uuid.New().String(),
		Data: contracts.InspectionCancelledEventPayload{
			InspectionID: i.ID,
			TenantID:     i.TenantID,
			PropertyID:   i.PropertyID,
			ContractID:   i.ContractID,
			Type:         i.Type,
			Reason:       i.Notes,
			CancelledAt:  cancelledAt,
			Timestamp:    cancelledAt,
		},
	}
	if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
		log.Printf("WARN: failed to publish inspection.cancelled for id=%s: %v", i.ID, err)
	}
}
