package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/audit"
	pkgevents "github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
)

// ErrExitInspectionRequired é retornado por TerminateContract quando o tenant
// configurou a regra "vistoria de saída obrigatória" e nenhuma vistoria EXIT
// concluída/com laudo foi encontrada para o contrato.
var ErrExitInspectionRequired = errors.New("exit inspection (vistoria de saída) is required before termination")

// ExitInspectionChecker é uma porta opcional para consultar a existência de
// vistoria de saída concluída no toca-ops. Implementada como interface para
// manter o ContractService livre de import direto do cliente HTTP — wire em
// routes.go quando a feature flag estiver ligada.
type ExitInspectionChecker interface {
	HasCompletedExitInspection(ctx context.Context, tenantID, contractID string) (bool, error)
}

// TerminateContractRequest é o input do encerramento (camada de serviço).
// EarlyTerminationFee, SecurityDepositReturn e SecurityDepositDeductions são
// opcionais (default 0). RequireExitInspection liga a checagem do checker
// — sem checker wireado a flag é ignorada.
type TerminateContractRequest struct {
	Reason                    string
	EarlyTerminationFee       float64
	SecurityDepositReturn     float64
	SecurityDepositDeductions float64
	ExitInspectionID          string
	RequireExitInspection     bool
}

// WithExitInspectionChecker injeta o cliente que consulta toca-ops para
// validar vistoria de saída antes do encerramento. Opcional — sem ele,
// RequireExitInspection é ignorado e o encerramento prossegue.
func (s *ContractService) WithExitInspectionChecker(checker ExitInspectionChecker) *ContractService {
	s.exitInspectionChecker = checker
	return s
}

// TerminateContract encerra um contrato ativo: marca status=TERMINATED,
// preenche c.Termination, persiste e publica contract.terminated para o
// toca-financial encerrar a recorrência e processar acerto de caução/multa.
func (s *ContractService) TerminateContract(ctx context.Context, tenantID, contractID string, req TerminateContractRequest) (*contract.Contract, error) {
	c, err := s.contractRepo.GetByID(ctx, tenantID, contractID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("contract not found")
	}

	if req.RequireExitInspection && s.exitInspectionChecker != nil {
		ok, err := s.exitInspectionChecker.HasCompletedExitInspection(ctx, tenantID, contractID)
		if err != nil {
			return nil, fmt.Errorf("exit inspection check failed: %w", err)
		}
		if !ok {
			return nil, ErrExitInspectionRequired
		}
	}

	triggeredBy := actorFromContext(ctx)
	if err := c.Terminate(req.Reason, triggeredBy, req.EarlyTerminationFee, req.SecurityDepositReturn, req.SecurityDepositDeductions, req.ExitInspectionID); err != nil {
		return nil, err
	}

	// Hidrata payload do evento com referências derivadas do Deal para que o
	// consumer não precise consultar deal repo de novo. Best-effort: falha de
	// lookup só perde os campos opcionais.
	d, _ := s.dealRepo.GetByID(ctx, tenantID, c.DealID)
	evt := pkgevents.ContractTerminatedEvent{
		Data: pkgevents.ContractTerminatedEventPayload{
			TenantID:                  tenantID,
			ContractID:                c.ID,
			DealID:                    c.DealID,
			PropertyID:                c.PropertyID,
			TerminationDate:           c.Termination.TerminatedAt.UTC().Format(time.RFC3339),
			Reason:                    c.Termination.Reason,
			EarlyTerminationFee:       c.Termination.EarlyTerminationFee,
			SecurityDeposit:           c.Termination.SecurityDepositReturn,
			SecurityDepositDeductions: c.Termination.SecurityDepositDeductions,
			TriggeredBy:               triggeredBy,
			Timestamp:                 pkgevents.NowRFC3339(),
		},
	}
	if d != nil {
		evt.Data.OwnerID = d.OwnerID
		evt.Data.CustomerID = d.CustomerID
		if evt.Data.PropertyID == "" {
			evt.Data.PropertyID = d.PropertyID
		}
	}

	if err := s.saveContractWithEvents(ctx, c, false, evt); err != nil {
		return nil, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "contract.terminate",
		EntityType: "Contract",
		EntityID:   c.ID,
		After: map[string]any{
			"status":                      string(c.Status),
			"reason":                      req.Reason,
			"early_termination_fee":       req.EarlyTerminationFee,
			"security_deposit_return":     req.SecurityDepositReturn,
			"security_deposit_deductions": req.SecurityDepositDeductions,
			"exit_inspection_id":          req.ExitInspectionID,
		},
		Metadata: map[string]any{"path": "TerminateContract"},
	})

	return c, nil
}

// ReadjustContractRequest é o input do reajuste (camada de serviço).
// NewValue tem precedência. Se NewValue == 0 e IndexValue > 0, calcula
// newValue = currentValue * (1 + indexValue/100). Mode = "manual" ou "auto".
type ReadjustContractRequest struct {
	NewValue       float64
	IndexName      string
	IndexValue     float64 // pct, e.g. 4.5 for 4.5%
	EffectiveFrom  time.Time
	NextAdjustment string // ISO date or descriptor (e.g. "2026-05-01")
	Mode           string
	Note           string
}

// ReadjustContract aplica reajuste no contrato e publica contract.readjusted
// para o toca-financial atualizar o RentalChargeConfig.MonthlyAmount.
func (s *ContractService) ReadjustContract(ctx context.Context, tenantID, contractID string, req ReadjustContractRequest) (*contract.Contract, error) {
	c, err := s.contractRepo.GetByID(ctx, tenantID, contractID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("contract not found")
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode != "auto" {
		mode = "manual"
	}

	newValue := req.NewValue
	if newValue <= 0 && req.IndexValue > 0 {
		newValue = c.Value * (1 + req.IndexValue/100)
	}
	if newValue <= 0 {
		return nil, errors.New("readjust requires new_value or index_value")
	}

	triggeredBy := actorFromContext(ctx)
	if mode == "auto" && triggeredBy == "system" {
		triggeredBy = "scheduler"
	}

	prev := c.Value
	entry, err := c.ApplyAdjustment(newValue, req.IndexValue, req.IndexName, mode, triggeredBy, req.Note, req.EffectiveFrom)
	if err != nil {
		return nil, err
	}
	if req.NextAdjustment != "" {
		c.NextAdjustment = req.NextAdjustment
	}

	evt := pkgevents.ContractReadjustedEvent{
		Data: pkgevents.ContractReadjustedEventPayload{
			TenantID:       tenantID,
			ContractID:     c.ID,
			DealID:         c.DealID,
			PreviousValue:  prev,
			NewValue:       newValue,
			IndexName:      req.IndexName,
			IndexValue:     req.IndexValue,
			EffectiveFrom:  entry.EffectiveFrom.UTC().Format(time.RFC3339),
			NextAdjustment: c.NextAdjustment,
			TriggeredBy:    triggeredBy,
			Timestamp:      pkgevents.NowRFC3339(),
		},
	}

	if err := s.saveContractWithEvents(ctx, c, false, evt); err != nil {
		return nil, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "contract.readjust",
		EntityType: "Contract",
		EntityID:   c.ID,
		After: map[string]any{
			"previous_value": prev,
			"new_value":      newValue,
			"index_name":     req.IndexName,
			"index_value":    req.IndexValue,
			"mode":           mode,
		},
		Metadata: map[string]any{"path": "ReadjustContract"},
	})

	return c, nil
}

// AmendContractRequest é o input para emitir um aditivo. Os campos vazios
// herdam do contrato original — apenas o que muda precisa ser informado.
// Reason é obrigatório (vai pro audit/event).
type AmendContractRequest struct {
	Reason string

	// Campos opcionais — quando vazios herdam do contrato original.
	StartDate          *time.Time
	EndDate            *time.Time
	Value              float64
	LateFeePct         float64
	MonthlyInterestPct float64
	AdjustIndex        string
	AutoAdjust         *bool
	NextAdjustment     string
	SecurityDeposit    float64
	// Content é o corpo do novo contrato (DOCX base64). Quando vazio, copia
	// do original — útil para aditivos que só ajustam valor/prazo. Quando
	// preenchido, o aditivo já vira para Clicksign com o texto reescrito.
	Content string
	// Signatories opcional: substitui a lista do original. Vazio = herda.
	Signatories []contract.Signatory
}

// AmendContract emite um aditivo: cria um novo contrato em DRAFT vinculado ao
// original via AmendsContractID, herda os campos não informados, registra o
// aditivo no histórico do original e publica contract.amended. NÃO altera o
// contrato original (ele continua ACTIVE até o aditivo ser assinado — o
// ciclo de signature.envelope_finished do novo contrato dispara o
// supersedeOriginalOnAmendmentSigned hook abaixo, que move o original para
// EXPIRED e marca SupersededAt).
//
// Retorna o NOVO contrato (DRAFT) — o caller (handler) tipicamente já redireciona
// o operador para a tela de envio de assinatura do aditivo.
func (s *ContractService) AmendContract(ctx context.Context, tenantID, contractID string, req AmendContractRequest) (*contract.Contract, error) {
	if strings.TrimSpace(req.Reason) == "" {
		return nil, errors.New("amendment reason is required")
	}
	original, err := s.contractRepo.GetByID(ctx, tenantID, contractID)
	if err != nil {
		return nil, err
	}
	if original == nil {
		return nil, errors.New("contract not found")
	}
	if !original.IsActive() {
		return nil, errors.New("can only amend an active (SIGNED/ACTIVE) contract")
	}
	if original.AmendsContractID != "" {
		return nil, errors.New("cannot amend an amendment — start from the original contract")
	}

	// Compose new contract by inheriting from the original. We can't reuse
	// contract.New() because we want to bypass auto end-date derivation and
	// signatories validation may differ (operator may add/remove fiador).
	newID := uuid.New().String()
	now := time.Now()
	startDate := original.StartDate
	if req.StartDate != nil {
		startDate = *req.StartDate
	}
	endDate := original.EndDate
	if req.EndDate != nil {
		endDate = req.EndDate
	}
	value := original.Value
	if req.Value > 0 {
		value = req.Value
	}
	lateFee := original.LateFeePct
	if req.LateFeePct > 0 {
		lateFee = req.LateFeePct
	}
	interest := original.MonthlyInterestPct
	if req.MonthlyInterestPct > 0 {
		interest = req.MonthlyInterestPct
	}
	adjustIndex := original.AdjustIndex
	if strings.TrimSpace(req.AdjustIndex) != "" {
		adjustIndex = req.AdjustIndex
	}
	autoAdjust := original.AutoAdjust
	if req.AutoAdjust != nil {
		autoAdjust = *req.AutoAdjust
	}
	nextAdjustment := original.NextAdjustment
	if strings.TrimSpace(req.NextAdjustment) != "" {
		nextAdjustment = req.NextAdjustment
	}
	securityDeposit := original.SecurityDeposit
	if req.SecurityDeposit > 0 {
		securityDeposit = req.SecurityDeposit
	}
	content := original.Content
	if strings.TrimSpace(req.Content) != "" {
		content = req.Content
	}
	signatories := req.Signatories
	if signatories == nil {
		// Deep-clone the original signatories so signature flow on the
		// amendment doesn't accidentally mutate the source rows. Reset
		// per-signer status fields — the amendment must be re-signed from
		// scratch.
		signatories = make([]contract.Signatory, 0, len(original.Signatories))
		for _, s := range original.Signatories {
			signatories = append(signatories, contract.Signatory{
				ID:            uuid.New().String(),
				Name:          s.Name,
				Email:         s.Email,
				Role:          s.Role,
				PhoneNumber:   s.PhoneNumber,
				Documentation: s.Documentation,
				Birthday:      s.Birthday,
				Group:         s.Group,
			})
		}
	}
	if err := contract.ValidateSignatories(original.Type, signatories); err != nil {
		return nil, err
	}

	amendment := &contract.Contract{
		ID:                 newID,
		DealID:             original.DealID,
		Status:             contract.StatusDraft,
		Content:            content,
		Signatories:        signatories,
		StartDate:          startDate,
		EndDate:            endDate,
		Type:               original.Type,
		PropertyID:         original.PropertyID,
		PropertyTitle:      original.PropertyTitle,
		Client:             original.Client,
		Value:              value,
		CommissionPct:      original.CommissionPct,
		LateFeePct:         lateFee,
		MonthlyInterestPct: interest,
		AdjustIndex:        adjustIndex,
		AutoAdjust:         autoAdjust,
		NextAdjustment:     nextAdjustment,
		SecurityDeposit:    securityDeposit,
		AmendsContractID:   original.ID,
		CreatedAt:          now,
		UpdatedAt:          now,
		TenantID:           tenantID,
	}
	if err := s.contractRepo.Create(ctx, amendment); err != nil {
		return nil, err
	}

	// Record the amendment on the original — preserves the chain for
	// reporting (the rentals tab shows "este contrato gerou 2 aditivos").
	triggeredBy := actorFromContext(ctx)
	original.Amendments = append(original.Amendments, contract.AmendmentEntry{
		NewContractID: amendment.ID,
		CreatedAt:     now,
		Reason:        req.Reason,
		TriggeredBy:   triggeredBy,
	})
	original.UpdatedAt = now

	evt := pkgevents.ContractAmendedEvent{
		Data: pkgevents.ContractAmendedEventPayload{
			TenantID:           tenantID,
			OriginalContractID: original.ID,
			NewContractID:      amendment.ID,
			DealID:             original.DealID,
			Reason:             req.Reason,
			TriggeredBy:        triggeredBy,
			Timestamp:          pkgevents.NowRFC3339(),
		},
	}
	if err := s.saveContractWithEvents(ctx, original, false, evt); err != nil {
		return nil, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "contract.amend",
		EntityType: "Contract",
		EntityID:   original.ID,
		After: map[string]any{
			"new_contract_id": amendment.ID,
			"reason":          req.Reason,
		},
		Metadata: map[string]any{"path": "AmendContract"},
	})

	return amendment, nil
}

// IsAmendment reports whether the given contract was emitted as an amendment
// of another contract — i.e. it carries a non-empty AmendsContractID. Used
// by the signed-PDF persist subscriber to reclassify the resulting Document
// as "aditivo" instead of "contrato".
func (s *ContractService) IsAmendment(ctx context.Context, tenantID, contractID string) (bool, error) {
	if tenantID == "" || contractID == "" {
		return false, nil
	}
	c, err := s.contractRepo.GetByID(ctx, tenantID, contractID)
	if err != nil {
		return false, err
	}
	if c == nil {
		return false, nil
	}
	return strings.TrimSpace(c.AmendsContractID) != "", nil
}

// RecordInspectionCompleted is called by the inspection.completed subscriber
// after a vistoria moves to Concluída with a laudo URL. For ENTRY we pin
// the inspection id on the contract — symmetry with Termination.ExitInspectionID
// already maintained by TerminateContract. For EXIT we only audit; the
// actual termination flow still happens through TerminateContract gated by
// HasCompletedExitInspection.
//
// Idempotent: re-running with the same (tenant, contract, inspection)
// no-ops once EntryInspectionID is already set.
func (s *ContractService) RecordInspectionCompleted(ctx context.Context, tenantID, contractID, inspectionType, inspectionID string) error {
	if tenantID == "" || contractID == "" || inspectionID == "" {
		return errors.New("tenant, contract and inspection ids are required")
	}
	c, err := s.contractRepo.GetByID(ctx, tenantID, contractID)
	if err != nil {
		return err
	}
	if c == nil {
		return nil // contract may have been deleted — ack and move on
	}

	switch strings.ToUpper(strings.TrimSpace(inspectionType)) {
	case "ENTRY":
		if c.EntryInspectionID == inspectionID {
			return nil // idempotent
		}
		c.EntryInspectionID = inspectionID
		c.UpdatedAt = time.Now()
		if err := s.contractRepo.Update(ctx, c); err != nil {
			return err
		}
		audit.Record(ctx, audit.Entry{
			Action:     "contract.entry_inspection_recorded",
			EntityType: "Contract",
			EntityID:   c.ID,
			After:      map[string]any{"entry_inspection_id": inspectionID},
			Metadata:   map[string]any{"path": "RecordInspectionCompleted"},
		})
	case "EXIT":
		// Don't auto-terminate. Just audit so the operator's later
		// TerminateContract call has the timeline trail. ExitInspectionID
		// itself is captured on the Termination block by TerminateContract.
		audit.Record(ctx, audit.Entry{
			Action:     "contract.exit_inspection_recorded",
			EntityType: "Contract",
			EntityID:   c.ID,
			After:      map[string]any{"exit_inspection_id": inspectionID},
			Metadata:   map[string]any{"path": "RecordInspectionCompleted"},
		})
	default:
		// PERIODIC or unknown — log and ack.
		log.Printf("inspection.completed: contract=%s type=%s — no contract-side mutation", contractID, inspectionType)
	}
	return nil
}

// SupersedeOriginalOnAmendmentSigned é chamado pelo signature.envelope_finished
// handler quando o contrato recém-assinado é um aditivo (carrega
// AmendsContractID). Marca o original como EXPIRED e preenche SupersededAt no
// AmendmentEntry correspondente. No-op se o signed.AmendsContractID for vazio.
func (s *ContractService) SupersedeOriginalOnAmendmentSigned(ctx context.Context, signed *contract.Contract) error {
	if signed == nil || signed.AmendsContractID == "" {
		return nil
	}
	original, err := s.contractRepo.GetByID(ctx, signed.TenantID, signed.AmendsContractID)
	if err != nil {
		return err
	}
	if original == nil {
		return nil
	}
	if original.Status == contract.StatusExpired || original.Status == contract.StatusTerminated {
		return nil // already moved
	}
	now := time.Now()
	original.Status = contract.StatusExpired
	for i := range original.Amendments {
		if original.Amendments[i].NewContractID == signed.ID && original.Amendments[i].SupersededAt == nil {
			ts := now
			original.Amendments[i].SupersededAt = &ts
			break
		}
	}
	original.UpdatedAt = now
	return s.contractRepo.Update(ctx, original)
}

// RunAutoReadjustLoop schedules a daily sweep of auto-readjustments on a
// background goroutine. Returns immediately; the loop runs until ctx is
// cancelled (usually on graceful shutdown).
//
// interval defaults to 24h when <= 0. defaultIndexValue is the % applied to
// every due contract that doesn't carry its own override — operators pass a
// per-environment value (typically the latest IGP-M/IPCA quote loaded from
// a separate ETL job). When defaultIndexValue <= 0 the loop still runs but
// per-contract readjustments are skipped (logged), so wiring this on
// without a real index value is a safe no-op rather than a silent bug.
func (s *ContractService) RunAutoReadjustLoop(ctx context.Context, interval time.Duration, defaultIndexValue float64) {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	go func() {
		// Brief startup delay so we don't fire at the same instant as boot
		// reconciliation work — these two sweeps both list contracts cross-
		// tenant and we'd rather stagger them.
		first := time.NewTimer(5 * time.Minute)
		defer first.Stop()
		select {
		case <-ctx.Done():
			return
		case <-first.C:
		}

		s.runAutoReadjustTick(ctx, defaultIndexValue)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runAutoReadjustTick(ctx, defaultIndexValue)
			}
		}
	}()
}

func (s *ContractService) runAutoReadjustTick(ctx context.Context, defaultIndexValue float64) {
	today := time.Now()
	count, err := s.RunAutoReadjustments(ctx, today, defaultIndexValue)
	if err != nil {
		log.Printf("AutoReadjust: sweep failed: %v", err)
		return
	}
	if count > 0 {
		log.Printf("AutoReadjust: applied %d readjustment(s) on %s", count, today.Format("2006-01-02"))
	}
}

// RunAutoReadjustments scans contracts whose AutoAdjust=true and NextAdjustment
// is due (<= today, ISO format YYYY-MM-DD), applies the index-based readjustment
// and rolls NextAdjustment forward by 12 months. Returns the number of contracts
// readjusted. Errors per-contract are logged but never abort the batch.
//
// Intended to be wired into a daily ticker by main.go. Idempotent within a
// single day — once applied, NextAdjustment moves out of the due window.
//
// defaultIndexValue is the % to apply when no per-contract override exists. A
// future iteration can fetch the live IGPM/IPCA quote from an external source;
// for now the operator configures the default via env or scheduler call site.
func (s *ContractService) RunAutoReadjustments(ctx context.Context, today time.Time, defaultIndexValue float64) (int, error) {
	all, err := s.contractRepo.ListAutoAdjustDue(ctx, today)
	if err != nil {
		return 0, fmt.Errorf("list auto-adjust due: %w", err)
	}
	count := 0
	for _, c := range all {
		if c == nil || !c.AutoAdjust || !c.IsActive() {
			continue
		}
		next, err := time.Parse("2006-01-02", strings.TrimSpace(c.NextAdjustment))
		if err != nil {
			log.Printf("auto-readjust: contract %s has unparseable next_adjustment=%q — skipping", c.ID, c.NextAdjustment)
			continue
		}
		indexValue := defaultIndexValue
		if indexValue <= 0 {
			log.Printf("auto-readjust: skipping contract %s — no default index value configured", c.ID)
			continue
		}
		nextAdj := next.AddDate(1, 0, 0).Format("2006-01-02")
		if _, err := s.ReadjustContract(ctx, c.TenantID, c.ID, ReadjustContractRequest{
			IndexName:      c.AdjustIndex,
			IndexValue:     indexValue,
			EffectiveFrom:  today,
			NextAdjustment: nextAdj,
			Mode:           "auto",
			Note:           "auto-readjust scheduler",
		}); err != nil {
			log.Printf("auto-readjust failed for contract %s: %v", c.ID, err)
			continue
		}
		count++
	}
	return count, nil
}
