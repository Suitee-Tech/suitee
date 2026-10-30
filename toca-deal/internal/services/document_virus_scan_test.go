package services_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/virusscan"
	"github.com/oginoino/toca/toca-deal/internal/domain/document"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// P1-13: o funil de uploads é fail-closed com estado auditável.

func uploadDoc() *document.Document {
	return &document.Document{
		ID:       "doc-scan-1",
		TenantID: "tenant-1",
		Category: document.CategoryContrato,
	}
}

func uploadFixture(scanner virusscan.Scanner) (*service.DocumentService, *fakeStorage) {
	stor := &fakeStorage{}
	svc := service.NewDocumentService(&fakeDocumentRepo{}, stor)
	if scanner != nil {
		svc.WithVirusScanner(scanner)
	}
	return svc, stor
}

func TestDocumentUpload_CleanPassesThrough(t *testing.T) {
	scanner := &fakeVirusScanner{clean: true}
	svc, stor := uploadFixture(scanner)

	err := svc.Upload(context.Background(), uploadDoc(), "contrato.pdf", "application/pdf", 4, strings.NewReader("dado"))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if stor.uploadCalls != 1 {
		t.Fatalf("storage.Upload calls = %d, want 1", stor.uploadCalls)
	}
	if string(stor.uploaded) != "dado" {
		t.Fatalf("bytes persistidos divergem do original: %q", stor.uploaded)
	}
	if len(scanner.scanned) != 1 || string(scanner.scanned[0]) != "dado" {
		t.Fatalf("scanner não recebeu os bytes exatos: %q", scanner.scanned)
	}
}

func TestDocumentUpload_VirusRejectedWithAudit(t *testing.T) {
	var got audit.Entry
	cleanup := audit.SetTestSink(func(e audit.Entry) { got = e })
	defer cleanup()

	scanner := &fakeVirusScanner{clean: false}
	svc, stor := uploadFixture(scanner)
	d := uploadDoc()

	err := svc.Upload(context.Background(), d, "malware.pdf", "application/pdf", 4, strings.NewReader("x"))
	if !errors.Is(err, virusscan.ErrVirusFound) {
		t.Fatalf("err = %v, want ErrVirusFound", err)
	}
	if stor.uploadCalls != 0 {
		t.Fatal("storage não pode receber um arquivo infectado")
	}
	if d.PersistenceStatus != document.PersistenceFailed || d.LastPersistError == "" {
		t.Fatalf("estado auditável não marcado: status=%q err=%q", d.PersistenceStatus, d.LastPersistError)
	}
	if got.Action != "document.virus_rejected" || got.EntityID != d.ID {
		t.Fatalf("audit ausente/divergente: %+v", got)
	}
}

func TestDocumentUpload_ScannerUnavailableFailsClosed(t *testing.T) {
	scanner := &fakeVirusScanner{err: fmt.Errorf("%w: dial refused", virusscan.ErrScannerUnavailable)}
	svc, stor := uploadFixture(scanner)

	err := svc.Upload(context.Background(), uploadDoc(), "f.pdf", "application/pdf", 4, strings.NewReader("x"))
	if !errors.Is(err, virusscan.ErrScannerUnavailable) {
		t.Fatalf("err = %v, want ErrScannerUnavailable", err)
	}
	if stor.uploadCalls != 0 {
		t.Fatal("scanner indisponível não pode deixar o upload passar")
	}
}

func TestDocumentUpload_NoScannerFailsClosed(t *testing.T) {
	svc, stor := uploadFixture(nil)

	err := svc.Upload(context.Background(), uploadDoc(), "f.pdf", "application/pdf", 4, strings.NewReader("x"))
	if !errors.Is(err, virusscan.ErrScannerUnavailable) {
		t.Fatalf("err = %v, want ErrScannerUnavailable (scanner ausente)", err)
	}
	if stor.uploadCalls != 0 {
		t.Fatal("sem scanner o upload deve ser recusado")
	}
}

// ensureReadAll fecha o contrato do spool: o serviço lê o stream inteiro
// (o scanner consome os MESMOS bytes que o storage recebe).
func TestDocumentUpload_ReadsWholeStream(t *testing.T) {
	scanner := &fakeVirusScanner{clean: true}
	svc, stor := uploadFixture(scanner)
	payload := strings.Repeat("a", 70000) // > um chunk do clamd

	if err := svc.Upload(context.Background(), uploadDoc(), "grande.pdf", "application/pdf", int64(len(payload)), strings.NewReader(payload)); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if string(stor.uploaded) != payload {
		t.Fatalf("bytes divergentes: %d vs %d", len(stor.uploaded), len(payload))
	}
	if len(scanner.scanned) != 1 || string(scanner.scanned[0]) != payload {
		t.Fatal("scanner não viu o stream inteiro")
	}
}

var _ = io.EOF
var _ = time.Now
