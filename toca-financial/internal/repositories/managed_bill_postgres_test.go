package repositories

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/managed_bill"
)

func TestManagedBillPostgresMissingLookupContracts(t *testing.T) {
	sqlDB := sql.OpenDB(managedBillNoRowsConnector{})
	t.Cleanup(func() { _ = sqlDB.Close() })
	repo := NewManagedBillPostgresRepository(&database.PostgresDB{
		DB: sqlx.NewDb(sqlDB, "managed-bill-no-rows"),
	})

	bySource, err := repo.GetBySourceKey(context.Background(), "tenant-id", "source-key")
	if err != nil || bySource != nil {
		t.Fatalf("missing source-key lookup = (%+v, %v), want (nil, nil)", bySource, err)
	}

	byID, err := repo.GetByID(context.Background(), "tenant-id", "bill-id")
	if byID != nil || !errors.Is(err, managed_bill.ErrNotFound) {
		t.Fatalf("missing id lookup = (%+v, %v), want (nil, ErrNotFound)", byID, err)
	}
}

func TestManagedBillListPredicateScopesAndEscapesSearch(t *testing.T) {
	where, args := managedBillListPredicate("tenant-id", managed_bill.Filter{
		ContractID: "contract-id",
		Search:     `Condominio_10%\\A`,
	})

	for _, fragment := range []string{
		"tenant_id = $1",
		"contract_id = $2",
		"component ILIKE $3 ESCAPE '\\'",
		"creditor ILIKE $3 ESCAPE '\\'",
		"description ILIKE $3 ESCAPE '\\'",
	} {
		if !strings.Contains(where, fragment) {
			t.Fatalf("predicate %q missing %q", where, fragment)
		}
	}
	if len(args) != 3 || args[0] != "tenant-id" || args[1] != "contract-id" || args[2] != `%Condominio\_10\%\\\\A%` {
		t.Fatalf("unexpected managed bill search args: %#v", args)
	}
}

func TestEncodeManagedBillDocumentsNormalizesAndSerializesPaymentAttempts(t *testing.T) {
	_, _, paymentAttempts, err := encodeManagedBillDocuments(&managed_bill.ManagedBill{})
	if err != nil {
		t.Fatalf("encode nil payment attempts: %v", err)
	}
	if string(paymentAttempts) != "[]" {
		t.Fatalf("nil payment attempts encoded as %s, want []", paymentAttempts)
	}

	reconciledAt := time.Date(2026, time.August, 21, 13, 0, 0, 0, time.UTC)
	bill := &managed_bill.ManagedBill{PaymentAttempts: []managed_bill.PaymentAttempt{{
		ID:                      "managed-bill-payment-source-1",
		PaymentReference:        "pix-payment-1",
		PaymentJournalSourceID:  "managed-bill-payment-source-1",
		PaidBy:                  "payer-1",
		PaidAt:                  reconciledAt.Add(-time.Hour),
		ReconciledBy:            "reconciler-1",
		ReconciledAt:            &reconciledAt,
		ReconciliationReference: "bank-line-1",
		ReversalReference:       "refund-1",
		ReversalJournalSourceID: "managed-bill-reversal-source-1",
		ReversalProofReference:  "proof://refund-1",
		ReversedBy:              "reverser-1",
		ReversedAt:              reconciledAt.Add(time.Hour),
		ReversalReason:          "payment rejected",
	}}}
	_, _, paymentAttempts, err = encodeManagedBillDocuments(bill)
	if err != nil {
		t.Fatalf("encode payment attempt: %v", err)
	}
	var payload []map[string]any
	if err := json.Unmarshal(paymentAttempts, &payload); err != nil {
		t.Fatalf("decode encoded payment attempts: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("encoded payment attempts length = %d, want 1", len(payload))
	}
	for _, key := range []string{
		"id", "payment_reference", "payment_journal_source_id", "paid_by", "paid_at",
		"reconciled_by", "reconciled_at", "reconciliation_reference", "reversal_reference",
		"reversal_journal_source_id", "reversal_proof_reference", "reversed_by", "reversed_at", "reversal_reason",
	} {
		if _, ok := payload[0][key]; !ok {
			t.Errorf("encoded payment attempt missing snake_case key %q: %s", key, paymentAttempts)
		}
	}
	if _, found := payload[0]["PaymentJournalSourceID"]; found {
		t.Fatalf("encoded payment attempt leaked Go field names: %s", paymentAttempts)
	}
}

func TestScanManagedBillDecodesAndNormalizesPaymentAttempts(t *testing.T) {
	bill, err := scanManagedBill(managedBillDocumentRow{paymentAttempts: []byte(`[{"id":"payment-source-1"}]`)})
	if err != nil {
		t.Fatalf("scan payment attempts: %v", err)
	}
	if len(bill.PaymentAttempts) != 1 || bill.PaymentAttempts[0].ID != "payment-source-1" {
		t.Fatalf("decoded payment attempts = %+v, want payment-source-1", bill.PaymentAttempts)
	}

	bill, err = scanManagedBill(managedBillDocumentRow{paymentAttempts: []byte("null")})
	if err != nil {
		t.Fatalf("scan null payment attempts: %v", err)
	}
	if bill.PaymentAttempts == nil || len(bill.PaymentAttempts) != 0 {
		t.Fatalf("null payment attempts normalized as %#v, want non-nil empty slice", bill.PaymentAttempts)
	}
	if !strings.Contains(managedBillSelect, "\tpayment_attempts,") {
		t.Fatal("managed bill SELECT does not include payment_attempts")
	}
}

// managedBillNoRowsConnector is a deliberately tiny database/sql test double.
// It exercises the real repository QueryRow/Scan path without adding sqlmock to
// the module or requiring TOCA_TEST_DATABASE_URI.
type managedBillNoRowsConnector struct{}

func (managedBillNoRowsConnector) Connect(context.Context) (driver.Conn, error) {
	return managedBillNoRowsConn{}, nil
}

func (managedBillNoRowsConnector) Driver() driver.Driver { return managedBillNoRowsDriver{} }

type managedBillNoRowsDriver struct{}

func (managedBillNoRowsDriver) Open(string) (driver.Conn, error) {
	return managedBillNoRowsConn{}, nil
}

type managedBillNoRowsConn struct{}

func (managedBillNoRowsConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("managed bill no-rows driver does not prepare statements")
}

func (managedBillNoRowsConn) Close() error { return nil }

func (managedBillNoRowsConn) Begin() (driver.Tx, error) {
	return nil, errors.New("managed bill no-rows driver does not begin transactions")
}

func (managedBillNoRowsConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return managedBillNoRows{}, nil
}

type managedBillNoRows struct{}

func (managedBillNoRows) Columns() []string { return nil }
func (managedBillNoRows) Close() error      { return nil }
func (managedBillNoRows) Next([]driver.Value) error {
	return io.EOF
}

type managedBillDocumentRow struct {
	paymentAttempts []byte
}

func (r managedBillDocumentRow) Scan(dest ...any) error {
	if len(dest) != 36 {
		return errors.New("unexpected managed bill scan destination count")
	}
	*dest[15].(*string) = string(managed_bill.StatusApproved)
	*dest[31].(*[]byte) = []byte("[]")
	*dest[32].(*[]byte) = []byte("[]")
	*dest[33].(*[]byte) = r.paymentAttempts
	return nil
}
