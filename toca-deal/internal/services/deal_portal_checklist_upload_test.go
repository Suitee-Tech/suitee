package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/virusscan"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/stretchr/testify/require"
)

// O cliente do portal roda sem claims de tenant: as leituras do negócio passam
// pelo repositório do portal (pool de serviço). O repositório do pool de
// aplicação devolve "nada" para ele — é o que se simula aqui para garantir que
// nenhum caminho do portal volte a ler pelo pool errado (issue "erro ao enviar
// documento da parte": /upload-url respondia 500 por causa disso).
type portalTestRepo struct {
	*inMemoryDealRepo
	portalDeals []*deal.Deal
}

func (r *portalTestRepo) FindBySignerIdentity(_ context.Context, _, _ string) ([]*deal.Deal, error) {
	return r.portalDeals, nil
}

// appPoolRepo esconde tudo: qualquer leitura por ele devolve nil (RLS sem
// claims), como acontecia no beta.
type appPoolRepo struct{ *inMemoryDealRepo }

func (r *appPoolRepo) GetByID(context.Context, string, string) (*deal.Deal, error) { return nil, nil }

// portalScannerStub registra o que foi varrido e devolve o veredito combinado.
type portalScannerStub struct {
	clean   bool
	err     error
	scanned [][]byte
}

func (s *portalScannerStub) Scan(_ context.Context, r io.Reader) (virusscan.Result, error) {
	body, _ := io.ReadAll(r)
	s.scanned = append(s.scanned, body)
	if s.err != nil {
		return virusscan.Result{}, s.err
	}
	if !s.clean {
		return virusscan.Result{Signature: "Eicar-Test-Signature"}, fmt.Errorf("%w: Eicar-Test-Signature", virusscan.ErrVirusFound)
	}
	return virusscan.Result{Clean: true}, nil
}

type downloadableStorage struct {
	objects      map[string][]byte
	uploaded     []byte
	uploadCalls  int
	deleted      []string
	presignedURL string
}

func (s *downloadableStorage) Upload(_ context.Context, _, filename, _ string, content io.Reader) (string, error) {
	s.uploadCalls++
	body, err := io.ReadAll(content)
	if err != nil {
		return "", err
	}
	s.uploaded = body
	return "http://fake-storage/" + filename, nil
}

func (s *downloadableStorage) GetURL(bucket, filename string) string {
	return "http://fake-storage/" + bucket + "/" + filename
}

func (s *downloadableStorage) GetPresignedURL(context.Context, string, string, time.Duration) (string, error) {
	return "https://storage.example/get", nil
}

func (s *downloadableStorage) GetPresignedPutURL(context.Context, string, string, time.Duration) (string, error) {
	if s.presignedURL == "" {
		return "", errors.New("presigned PUT not supported for local storage")
	}
	return s.presignedURL, nil
}

func (s *downloadableStorage) IsPrivate(string) bool { return true }

func (s *downloadableStorage) Download(_ context.Context, bucket, filename string) (io.ReadCloser, string, int64, error) {
	body, ok := s.objects[bucket+"/"+filename]
	if !ok {
		return nil, "", 0, errors.New("object not found")
	}
	return io.NopCloser(strings.NewReader(string(body))), "application/pdf", int64(len(body)), nil
}

func (s *downloadableStorage) Delete(_ context.Context, bucket, filename string) error {
	s.deleted = append(s.deleted, bucket+"/"+filename)
	return nil
}

func portalUploadFixture(t *testing.T, scanner virusscan.Scanner, objects map[string][]byte) (*DealService, *appPoolRepo, *downloadableStorage) {
	t.Helper()
	stored := &deal.Deal{
		ID:       "deal-1",
		TenantID: "tenant-1",
		Status:   deal.StatusAnalysis,
		Checklist: []deal.ChecklistItem{
			{ID: "item-1", Name: "Documento de Identidade (RG/CNH)", Status: deal.ChecklistPending},
		},
	}
	appRepo := &appPoolRepo{inMemoryDealRepo: newInMemoryDealRepo(stored)}
	portalRepo := &portalTestRepo{inMemoryDealRepo: appRepo.inMemoryDealRepo, portalDeals: []*deal.Deal{stored}}
	storage := &downloadableStorage{objects: objects}

	svc := NewDealService(appRepo, nil, nil, nil, storage)
	svc.WithPortalRepository(portalRepo)
	if scanner != nil {
		svc.WithVirusScanner(scanner)
	}
	return svc, appRepo, storage
}

// A leitura do negócio no /upload-url precisa do repositório do portal.
func TestPortalChecklistUploadURLReadsThroughThePortalRepository(t *testing.T) {
	svc, _, storage := portalUploadFixture(t, &portalScannerStub{clean: true}, nil)
	storage.presignedURL = "https://storage.example/signed"

	out, err := svc.GetMyChecklistItemUploadURL(context.Background(), "acc-1", "cliente@example.com", "deal-1", "item-1", "Documento.pdf", "application/pdf", 1024)
	require.NoError(t, err)
	require.Equal(t, "https://storage.example/signed", out.UploadURL)
	require.Contains(t, out.StorageKey, "tenant-1/checklist/deal-1/")
}

// Arquivo acima do teto é recusado com ErrFileTooLarge antes de ir para a
// memória ou para o scanner (o usuário recebe 413 com o limite, não 500).
func TestPortalChecklistUploadRejectsOversizedFileBeforeScanning(t *testing.T) {
	scanner := &portalScannerStub{clean: true}
	svc, _, storage := portalUploadFixture(t, scanner, nil)

	_, err := svc.UploadMyChecklistItem(context.Background(), "acc-1", "cliente@example.com", "Cliente",
		"deal-1", "item-1", "grande.pdf", "application/pdf", virusscan.MaxUploadBytes+1, strings.NewReader("x"))

	require.ErrorIs(t, err, virusscan.ErrFileTooLarge)
	require.Empty(t, scanner.scanned, "arquivo grande não deve sequer chegar ao scanner")
	require.Zero(t, storage.uploadCalls, "arquivo grande não pode ser persistido")
}

// Upload com vírus: nada é persistido e o veredito é tipado (422 no transporte).
func TestPortalChecklistUploadRejectsVirus(t *testing.T) {
	svc, _, storage := portalUploadFixture(t, &portalScannerStub{clean: false}, nil)

	_, err := svc.UploadMyChecklistItem(context.Background(), "acc-1", "cliente@example.com", "Cliente",
		"deal-1", "item-1", "infectado.pdf", "application/pdf", 10, strings.NewReader("EICAR"))

	require.ErrorIs(t, err, virusscan.ErrVirusFound)
	require.Zero(t, storage.uploadCalls)
}

// O fluxo presigned (PUT direto no storage) também passa pelo antivírus na
// conclusão: objeto infectado é removido e o item NÃO fica marcado como enviado.
func TestPortalChecklistCompleteScansPresignedUploadAndDeletesInfected(t *testing.T) {
	svc, appRepo, storage := portalUploadFixture(t, &portalScannerStub{clean: false},
		map[string][]byte{"toca-documents/tenant-1/checklist/deal-1/abc_doc.pdf": []byte("EICAR")})

	_, err := svc.CompleteMyChecklistItemUpload(context.Background(), "acc-1", "cliente@example.com", "Cliente",
		"deal-1", "item-1", "tenant-1/checklist/deal-1/abc_doc.pdf", "doc.pdf", "application/pdf", 10)

	require.ErrorIs(t, err, virusscan.ErrVirusFound)
	require.Contains(t, storage.deleted, "toca-documents/tenant-1/checklist/deal-1/abc_doc.pdf")
	stored, _ := appRepo.inMemoryDealRepo.GetByID(context.Background(), "tenant-1", "deal-1")
	require.Equal(t, deal.ChecklistPending, stored.Checklist[0].Status, "item não pode ser marcado como enviado")
}

// Scanner fora: conclusão recusada (fail-closed) e item segue pendente.
func TestPortalChecklistCompleteFailsClosedWithoutScanner(t *testing.T) {
	svc, appRepo, _ := portalUploadFixture(t, &portalScannerStub{err: virusscan.ErrScannerUnavailable},
		map[string][]byte{"toca-documents/tenant-1/checklist/deal-1/abc_doc.pdf": []byte("conteudo")})

	_, err := svc.CompleteMyChecklistItemUpload(context.Background(), "acc-1", "cliente@example.com", "Cliente",
		"deal-1", "item-1", "tenant-1/checklist/deal-1/abc_doc.pdf", "doc.pdf", "application/pdf", 8)

	require.ErrorIs(t, err, virusscan.ErrScannerUnavailable)
	stored, _ := appRepo.inMemoryDealRepo.GetByID(context.Background(), "tenant-1", "deal-1")
	require.Equal(t, deal.ChecklistPending, stored.Checklist[0].Status)
}

// A chave de storage vem do cliente: apontar para o arquivo de outro negócio é
// recusado antes de qualquer varredura.
func TestPortalChecklistCompleteRejectsForeignStorageKey(t *testing.T) {
	scanner := &portalScannerStub{clean: true}
	svc, _, _ := portalUploadFixture(t, scanner, map[string][]byte{"toca-documents/outro/checklist/x/y.pdf": []byte("conteudo")})

	_, err := svc.CompleteMyChecklistItemUpload(context.Background(), "acc-1", "cliente@example.com", "Cliente",
		"deal-1", "item-1", "outro/checklist/x/y.pdf", "doc.pdf", "application/pdf", 8)

	require.ErrorIs(t, err, ErrPortalAccessDenied)
	require.Empty(t, scanner.scanned)
}

// Caminho feliz presigned: objeto limpo conclui o item com a chave informada.
func TestPortalChecklistCompleteAcceptsCleanPresignedUpload(t *testing.T) {
	svc, appRepo, _ := portalUploadFixture(t, &portalScannerStub{clean: true},
		map[string][]byte{"toca-documents/tenant-1/checklist/deal-1/abc_doc.pdf": []byte("conteudo limpo")})

	d, err := svc.CompleteMyChecklistItemUpload(context.Background(), "acc-1", "cliente@example.com", "Cliente",
		"deal-1", "item-1", "tenant-1/checklist/deal-1/abc_doc.pdf", "doc.pdf", "application/pdf", 14)

	require.NoError(t, err)
	require.Equal(t, deal.ChecklistUploaded, d.Checklist[0].Status)
	require.Equal(t, "tenant-1/checklist/deal-1/abc_doc.pdf", d.Checklist[0].StorageKey)

	stored, _ := appRepo.inMemoryDealRepo.GetByID(context.Background(), "tenant-1", "deal-1")
	require.Equal(t, deal.ChecklistUploaded, stored.Checklist[0].Status)
}
