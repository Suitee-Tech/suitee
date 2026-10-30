package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/invoice"
)

var (
	ErrInvoiceChargeLinkRequired            = errors.New("invoice is not linked to a charge")
	ErrInvoicePaymentApplicationUnavailable = errors.New("invoice payment application is not configured")
	ErrInvoiceChargeContractMismatch        = errors.New("invoice reference does not match charge contract")
	ErrInvoiceChargePayerMismatch           = errors.New("invoice payer does not match charge payer")
	ErrInvoiceAmountExceedsChargeBalance    = errors.New("invoice amount exceeds charge outstanding balance")
	ErrInvoiceIdempotencyConflict           = errors.New("invoice idempotency key was reused with a different command")
	ErrInvoiceEventBusRequired              = errors.New("event bus is required for an atomic invoice payment")
	ErrInvoiceTransactionalOutboxRequired   = errors.New("event bus does not support transactional outbox")
	ErrInvoiceAtomicIssuanceRequired        = errors.New("linked invoice issuance requires a transaction")
	ErrInvoiceChargeLockRequired            = errors.New("payment application does not support charge locking")
)

type invoiceChargeLookup interface {
	GetByID(ctx context.Context, tenantID, chargeID string) (*charge.Charge, error)
}

type invoicePaymentApplication interface {
	invoiceChargeLookup
	MarkInvoicePaidIdempotent(ctx context.Context, tenantID, chargeID, invoiceID, paymentMethod string, paidAmount float64, paymentID string, paidAt time.Time) (*charge.Charge, error)
}

type invoiceChargeReservationLocker interface {
	GetByIDForUpdate(ctx context.Context, tenantID, chargeID string) (*charge.Charge, error)
}

type invoicePaymentLockingRepository interface {
	GetByIDForUpdate(ctx context.Context, tenantID, id string) (*invoice.Invoice, error)
}

type invoiceTransactionalOutbox interface {
	EnqueueOutboxInTx(ctx context.Context, tx *sqlx.Tx, event events.Event) (string, error)
}

type invoiceTxRunner func(ctx context.Context, work func(context.Context) error) error

type InvoiceService struct {
	repo               invoice.Repository
	paymentApplication invoicePaymentApplication
	eventBus           events.EventBus
	db                 *database.PostgresDB
	creationTx         invoiceTxRunner
	paymentTx          invoiceTxRunner
}

// NewInvoiceService publishes invoice.paid only after the real invoice row is
// settled. ChargeService independently owns charge.paid for full receivable
// settlement; keeping both producers separate prevents synthetic invoice IDs.
func NewInvoiceService(repo invoice.Repository, eventBus events.EventBus) *InvoiceService {
	return &InvoiceService{repo: repo, eventBus: eventBus}
}

func (s *InvoiceService) WithPaymentApplication(application invoicePaymentApplication) *InvoiceService {
	s.paymentApplication = application
	return s
}

// WithDB makes both linked issuance (Charge lock + reservation + Invoice row)
// and payment (canonical Charge cash + Invoice projection + outbox) atomic.
// ChargeService detects the carried transaction and reuses it.
func (s *InvoiceService) WithDB(db *database.PostgresDB) *InvoiceService {
	s.db = db
	if db == nil {
		s.creationTx = nil
		s.paymentTx = nil
		return s
	}
	runner := invoiceTxRunner(func(ctx context.Context, work func(context.Context) error) error {
		return db.RunInTx(ctx, func(tx *sqlx.Tx) error {
			return work(database.WithTx(ctx, tx))
		})
	})
	s.creationTx = runner
	s.paymentTx = runner
	return s
}

func (s *InvoiceService) Create(ctx context.Context, referenceID, payerID string, amount float64, dueDate time.Time, invType invoice.Type, tenantID string) (*invoice.Invoice, error) {
	return s.CreateWithSource(ctx, referenceID, payerID, amount, dueDate, invType, tenantID, "")
}

func (s *InvoiceService) CreateWithSource(ctx context.Context, referenceID, payerID string, amount float64, dueDate time.Time, invType invoice.Type, tenantID, sourceEventID string) (*invoice.Invoice, error) {
	return s.CreateWithSourceAndCharge(ctx, referenceID, payerID, amount, dueDate, invType, tenantID, sourceEventID, "")
}

// CreateWithSourceAndCharge links the invoice projection to the canonical
// receivable that will receive every payment allocation.
func (s *InvoiceService) CreateWithSourceAndCharge(ctx context.Context, referenceID, payerID string, amount float64, dueDate time.Time, invType invoice.Type, tenantID, sourceEventID, chargeID string) (*invoice.Invoice, error) {
	inv, err := invoice.New(referenceID, payerID, amount, dueDate, invType, tenantID)
	if err != nil {
		logger.FromContext(ctx).With("operation", "invoice.create", "tenantID", tenantID).
			Error("failed to create invoice entity", "error", err)
		return nil, err
	}

	inv.Status = invoice.StatusIssued // Auto issue for now
	inv.SourceEventID = strings.TrimSpace(sourceEventID)
	inv.ChargeID = strings.TrimSpace(chargeID)
	if inv.ChargeID != "" && s.creationTx != nil && database.TxFromCtx(ctx) == nil {
		var created *invoice.Invoice
		err := s.creationTx(ctx, func(txCtx context.Context) error {
			var createErr error
			created, createErr = s.createWithChargeReservation(txCtx, inv)
			return createErr
		})
		return created, err
	}
	return s.createWithChargeReservation(ctx, inv)
}

func (s *InvoiceService) createWithChargeReservation(ctx context.Context, inv *invoice.Invoice) (*invoice.Invoice, error) {
	log := logger.FromContext(ctx).With("operation", "invoice.create", "tenantID", inv.TenantID)

	// The idempotency key identifies the command, not a hash of its mutable
	// payload. Resolve an existing command before consulting the current charge
	// balance so a later retry remains stable after other payments occurred.
	if existing, found, err := s.existingInvoiceCreateCommand(ctx, inv); err != nil || found {
		return existing, err
	}

	// A linked invoice is a slice of one canonical receivable. Reject an
	// unrelated or over-sized allocation before persisting the projection.
	// The Charge row is the serialization point shared by every invoice create
	// command for this receivable. Once locked, re-check the idempotency key
	// (another waiter may have committed it), then calculate the amount already
	// reserved by all non-terminal linked invoices in the same transaction.
	if inv.ChargeID != "" {
		if s.paymentApplication == nil {
			return nil, ErrInvoicePaymentApplicationUnavailable
		}
		var canonicalCharge *charge.Charge
		var getErr error
		if database.TxFromCtx(ctx) != nil {
			locker, ok := s.paymentApplication.(invoiceChargeReservationLocker)
			if !ok {
				return nil, ErrInvoiceChargeLockRequired
			}
			canonicalCharge, getErr = locker.GetByIDForUpdate(ctx, inv.TenantID, inv.ChargeID)
		} else {
			if s.db != nil {
				return nil, ErrInvoiceAtomicIssuanceRequired
			}
			// Lightweight in-memory callers have no SQL transaction. They still
			// exercise the reservation invariant, while production WithDB paths
			// always take the locked branch above.
			canonicalCharge, getErr = s.paymentApplication.GetByID(ctx, inv.TenantID, inv.ChargeID)
		}
		if getErr != nil {
			return nil, fmt.Errorf("load canonical charge %s for invoice creation: %w", inv.ChargeID, getErr)
		}
		if database.TxFromCtx(ctx) != nil {
			if existing, found, err := s.existingInvoiceCreateCommand(ctx, inv); err != nil || found {
				return existing, err
			}
		}
		if validationErr := validateInvoiceChargeAllocation(inv, canonicalCharge); validationErr != nil {
			return nil, validationErr
		}
		reservedCents, reservationErr := s.repo.SumOpenAmountCentsByChargeID(ctx, inv.TenantID, inv.ChargeID, "")
		if reservationErr != nil {
			return nil, reservationErr
		}
		if validationErr := validateInvoiceChargeReservation(inv, canonicalCharge, reservedCents); validationErr != nil {
			return nil, validationErr
		}
	}

	if err := s.repo.Create(ctx, inv); err != nil {
		if errors.Is(err, invoice.ErrInvoiceAlreadyExists) && inv.SourceEventID != "" {
			existing, gerr := s.repo.GetBySourceEventID(ctx, inv.TenantID, inv.SourceEventID)
			if gerr != nil {
				log.Error("failed to fetch existing invoice on idempotency conflict", "error", gerr)
				return nil, gerr
			}
			if existing != nil {
				if !sameInvoiceCreateCommand(existing, inv) {
					return nil, ErrInvoiceIdempotencyConflict
				}
				log.Info("invoice idempotency: returning existing invoice", "invoiceID", existing.ID)
				return existing, nil
			}
		}
		log.Error("failed to save invoice", "error", err)
		return nil, err
	}

	log.Info("invoice created successfully", "invoiceID", inv.ID)
	return inv, nil
}

func (s *InvoiceService) existingInvoiceCreateCommand(ctx context.Context, expected *invoice.Invoice) (*invoice.Invoice, bool, error) {
	if expected == nil || expected.SourceEventID == "" {
		return nil, false, nil
	}
	existing, err := s.repo.GetBySourceEventID(ctx, expected.TenantID, expected.SourceEventID)
	if err != nil {
		return nil, false, err
	}
	if existing == nil {
		return nil, false, nil
	}
	if !sameInvoiceCreateCommand(existing, expected) {
		return nil, true, ErrInvoiceIdempotencyConflict
	}
	return existing, true, nil
}

func sameInvoiceCreateCommand(canonical, expected *invoice.Invoice) bool {
	if canonical == nil || expected == nil {
		return false
	}
	return canonical.TenantID == expected.TenantID &&
		canonical.SourceEventID == expected.SourceEventID &&
		strings.TrimSpace(canonical.ReferenceID) == strings.TrimSpace(expected.ReferenceID) &&
		strings.TrimSpace(canonical.ChargeID) == strings.TrimSpace(expected.ChargeID) &&
		strings.TrimSpace(canonical.PayerID) == strings.TrimSpace(expected.PayerID) &&
		valueobjects.NewMoneyFromFloat(canonical.Amount).Cents() == valueobjects.NewMoneyFromFloat(expected.Amount).Cents() &&
		canonical.DueDate.Equal(expected.DueDate) &&
		canonical.Type == expected.Type
}

func (s *InvoiceService) GetByID(ctx context.Context, tenantID, id string) (*invoice.Invoice, error) {
	log := logger.FromContext(ctx).With("operation", "invoice.getByID", "tenantID", tenantID, "invoiceID", id)

	inv, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		log.Error("failed to fetch invoice", "error", err)
		return nil, err
	}
	if inv == nil {
		return nil, invoice.ErrInvoiceNotFound
	}
	if inv.TenantID != tenantID {
		return nil, invoice.ErrInvoiceNotFound
	}
	return inv, nil
}

func (s *InvoiceService) GetByReferenceID(ctx context.Context, tenantID, referenceID string) ([]*invoice.Invoice, error) {
	log := logger.FromContext(ctx).With("operation", "invoice.getByReferenceID", "tenantID", tenantID, "referenceID", referenceID)

	invoices, err := s.repo.GetByReferenceID(ctx, tenantID, referenceID)
	if err != nil {
		log.Error("failed to fetch invoices by reference", "error", err)
		return nil, err
	}
	return invoices, nil
}

func (s *InvoiceService) List(ctx context.Context, tenantID string, status *invoice.Status, referenceID *string) ([]*invoice.Invoice, error) {
	log := logger.FromContext(ctx).With("operation", "invoice.list", "tenantID", tenantID)

	invoices, err := s.repo.List(ctx, tenantID, status, referenceID)
	if err != nil {
		log.Error("failed to list invoices", "error", err)
		return nil, err
	}
	return invoices, nil
}

func (s *InvoiceService) Search(ctx context.Context, tenantID string, filter invoice.SearchFilter) ([]*invoice.Invoice, int64, error) {
	return s.repo.Search(ctx, tenantID, filter)
}

func (s *InvoiceService) Pay(ctx context.Context, tenantID, id string) (*invoice.Invoice, error) {
	return s.PayIdempotent(ctx, tenantID, id, uuid.NewString(), "INVOICE", time.Now())
}

// PayIdempotent allocates invoice cash through the same ChargeService used by
// /charges/:id/pay, manual reconciliation and future webhook adapters. The
// invoice row is only a projection and advances after the canonical,
// append-only payment succeeds.
func (s *InvoiceService) PayIdempotent(ctx context.Context, tenantID, id, paymentID, paymentMethod string, paidAt time.Time) (*invoice.Invoice, error) {
	if strings.TrimSpace(paymentID) == "" {
		return nil, errors.New("payment id is required")
	}
	if strings.TrimSpace(paymentMethod) == "" {
		return nil, errors.New("payment method is required")
	}
	if paidAt.IsZero() {
		paidAt = time.Now()
	}
	if s.paymentTx != nil && database.TxFromCtx(ctx) == nil {
		var paid *invoice.Invoice
		err := s.paymentTx(ctx, func(txCtx context.Context) error {
			var err error
			paid, err = s.payIdempotent(txCtx, tenantID, id, paymentID, paymentMethod, paidAt)
			return err
		})
		return paid, err
	}
	return s.payIdempotent(ctx, tenantID, id, paymentID, paymentMethod, paidAt)
}

func (s *InvoiceService) payIdempotent(ctx context.Context, tenantID, id, paymentID, paymentMethod string, paidAt time.Time) (*invoice.Invoice, error) {
	log := logger.FromContext(ctx).With("operation", "invoice.pay_idempotent", "tenantID", tenantID, "invoiceID", id)
	var inv *invoice.Invoice
	var err error
	loadedWithLock := false
	if database.TxFromCtx(ctx) != nil {
		lockingRepo, ok := s.repo.(invoicePaymentLockingRepository)
		if !ok && s.db != nil {
			return nil, errors.New("invoice repository does not support payment locking")
		}
		if ok {
			loadedWithLock = true
			inv, err = lockingRepo.GetByIDForUpdate(ctx, tenantID, id)
		}
	}
	if !loadedWithLock {
		inv, err = s.repo.GetByID(ctx, tenantID, id)
	}
	if err != nil {
		return nil, err
	}
	if inv == nil {
		return nil, invoice.ErrInvoiceNotFound
	}
	if inv.TenantID != tenantID {
		return nil, invoice.ErrInvoiceNotFound
	}
	if inv.Status == invoice.StatusPaid {
		return inv, nil
	}
	if strings.TrimSpace(inv.ChargeID) == "" {
		return nil, ErrInvoiceChargeLinkRequired
	}
	if s.paymentApplication == nil {
		return nil, ErrInvoicePaymentApplicationUnavailable
	}
	canonicalCharge, err := s.paymentApplication.GetByID(ctx, tenantID, inv.ChargeID)
	if err != nil {
		return nil, fmt.Errorf("load canonical charge %s for invoice: %w", inv.ChargeID, err)
	}
	if err := validateInvoiceChargeAllocation(inv, canonicalCharge); err != nil {
		return nil, err
	}
	if _, err := s.paymentApplication.MarkInvoicePaidIdempotent(ctx, tenantID, inv.ChargeID, inv.ID, paymentMethod, inv.Amount, paymentID, paidAt); err != nil {
		return nil, fmt.Errorf("apply invoice payment to charge %s: %w", inv.ChargeID, err)
	}
	if err := inv.PayAt(paidAt); err != nil {
		log.Error("failed to pay invoice", "error", err)
		return nil, err
	}
	if err := s.repo.Update(ctx, inv); err != nil {
		log.Error("failed to update invoice after payment", "error", err)
		return nil, err
	}
	if err := s.publishInvoicePaid(ctx, inv); err != nil {
		return nil, err
	}
	log.Info("invoice paid successfully")
	recordMetric := func() { metrics.IncInvoicePaid(string(inv.Type)) }
	if !database.DeferUntilCommit(ctx, recordMetric) {
		recordMetric()
	}
	return inv, nil
}

func (s *InvoiceService) publishInvoicePaid(ctx context.Context, inv *invoice.Invoice) error {
	if inv == nil {
		return errors.New("paid invoice is required")
	}
	if s.eventBus == nil {
		if s.db != nil {
			return ErrInvoiceEventBusRequired
		}
		// Lightweight callers without a database retain their historical
		// no-broker behavior. Production WithDB paths fail closed above.
		return nil
	}

	paidAt := time.Now().UTC()
	if inv.PaidAt != nil {
		paidAt = inv.PaidAt.UTC()
	}
	evt := contracts.InvoicePaidEvent{
		EventIDVal: events.DeriveNotificationID("invoice.paid", inv.TenantID, inv.ID),
		Data: contracts.InvoicePaidEventPayload{
			SchemaVersion: 2,
			InvoiceID:     inv.ID, ChargeID: inv.ChargeID, TenantID: inv.TenantID,
			ReferenceID: inv.ReferenceID, Type: string(inv.Type), Amount: inv.Amount,
			Currency: inv.Currency, PaidAt: paidAt.Format(time.RFC3339Nano),
		},
	}
	if tx := database.TxFromCtx(ctx); tx != nil {
		outbox, ok := s.eventBus.(invoiceTransactionalOutbox)
		if !ok {
			if s.db != nil {
				return ErrInvoiceTransactionalOutboxRequired
			}
			return s.eventBus.PublishReliable(ctx, evt)
		}
		if _, err := outbox.EnqueueOutboxInTx(ctx, tx, evt); err != nil {
			return fmt.Errorf("enqueue invoice.paid atomically: %w", err)
		}
		return nil
	}
	if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
		return fmt.Errorf("publish invoice.paid: %w", err)
	}
	return nil
}

// validateInvoiceChargeAllocation prevents a tenant-scoped but unrelated
// charge ID from receiving invoice cash. Amounts are compared in cents and an
// invoice may represent any positive slice up to the outstanding balance.
func validateInvoiceChargeAllocation(inv *invoice.Invoice, canonical *charge.Charge) error {
	if canonical == nil {
		return charge.ErrNotFound
	}
	if canonical.TenantID != inv.TenantID {
		return charge.ErrNotFound
	}
	if strings.TrimSpace(canonical.ContractID) != strings.TrimSpace(inv.ReferenceID) {
		return fmt.Errorf("%w: invoice reference=%q charge contract=%q", ErrInvoiceChargeContractMismatch, inv.ReferenceID, canonical.ContractID)
	}
	if strings.TrimSpace(canonical.TenantPartyID) != strings.TrimSpace(inv.PayerID) {
		return fmt.Errorf("%w: invoice payer=%q charge payer=%q", ErrInvoiceChargePayerMismatch, inv.PayerID, canonical.TenantPartyID)
	}

	invoiceCents := valueobjects.NewMoneyFromFloat(inv.Amount).Cents()
	paidCents := int64(0)
	if canonical.PaidAmount != nil {
		paidCents = canonical.PaidAmount.Cents()
	}
	remainingCents := canonical.TotalAmount.Cents() - paidCents
	if invoiceCents <= 0 || invoiceCents > remainingCents {
		return fmt.Errorf("%w: invoice=%d cents remaining=%d cents", ErrInvoiceAmountExceedsChargeBalance, invoiceCents, remainingCents)
	}
	return nil
}

// validateInvoiceChargeReservation accounts for cash already received on the
// Charge and for invoices that are still open. PAID invoices are excluded by
// the repository because their amount already reduced the Charge balance;
// CANCELLED invoices are excluded because they released their reservation.
func validateInvoiceChargeReservation(inv *invoice.Invoice, canonical *charge.Charge, reservedCents int64) error {
	if inv == nil || canonical == nil {
		return charge.ErrNotFound
	}
	invoiceCents := valueobjects.NewMoneyFromFloat(inv.Amount).Cents()
	paidCents := int64(0)
	if canonical.PaidAmount != nil {
		paidCents = canonical.PaidAmount.Cents()
	}
	outstandingCents := canonical.TotalAmount.Cents() - paidCents
	availableCents := outstandingCents - reservedCents
	if reservedCents < 0 || availableCents < 0 || invoiceCents <= 0 || invoiceCents > availableCents {
		return fmt.Errorf(
			"%w: invoice=%d cents outstanding=%d cents open_invoices=%d cents available=%d cents",
			ErrInvoiceAmountExceedsChargeBalance, invoiceCents, outstandingCents, reservedCents, availableCents,
		)
	}
	return nil
}
