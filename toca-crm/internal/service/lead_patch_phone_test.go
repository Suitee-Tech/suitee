package service

import (
	"context"
	"strings"
	"testing"

	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

func TestPatch_NormalizesPhoneBeforeRepo(t *testing.T) {
	repo := &captureRepo{patchOk: true, patchResult: &lead.Lead{}}
	svc := &LeadService{repo: repo}

	updates := map[string]interface{}{
		"Phone": "(11) 99999-9999",
	}
	if _, ok, err := svc.Patch(context.Background(), "lead-1", "tenant-1", nil, updates); err != nil || !ok {
		t.Fatalf("patch failed: ok=%v err=%v", ok, err)
	}
	if len(repo.patchCalls) != 1 {
		t.Fatalf("expected 1 repo call, got %d", len(repo.patchCalls))
	}
	got := repo.patchCalls[0]["Phone"]
	if got != "+5511999999999" {
		t.Fatalf("phone not normalized in repo call: got %v", got)
	}
}

func TestPatch_InvalidPhoneRejectedBeforeRepo(t *testing.T) {
	repo := &captureRepo{patchOk: true, patchResult: &lead.Lead{}}
	svc := &LeadService{repo: repo}

	updates := map[string]interface{}{
		"Phone": "12345",
	}
	_, ok, err := svc.Patch(context.Background(), "lead-1", "tenant-1", nil, updates)
	if ok || err == nil {
		t.Fatalf("expected rejection, got ok=%v err=%v", ok, err)
	}
	if !strings.Contains(err.Error(), "invalid phone") {
		t.Fatalf("error should mention invalid phone, got %v", err)
	}
	if len(repo.patchCalls) != 0 {
		t.Fatalf("repo should not be called on invalid phone; got %d calls", len(repo.patchCalls))
	}
}

func TestPatch_EmptyPhoneSkipsNormalization(t *testing.T) {
	repo := &captureRepo{patchOk: true, patchResult: &lead.Lead{}}
	svc := &LeadService{repo: repo}

	updates := map[string]interface{}{
		"Phone": "",
		"Name":  "Alice Souza",
	}
	_, ok, err := svc.Patch(context.Background(), "lead-1", "tenant-1", nil, updates)
	if !ok || err != nil {
		t.Fatalf("expected success with empty phone, got ok=%v err=%v", ok, err)
	}
	if v := repo.patchCalls[0]["Phone"]; v != "" {
		t.Fatalf("empty phone should remain empty, got %v", v)
	}
}

// A edição também exige nome e sobrenome: sem isso o operador salvava o
// monônimo e só descobria ao iniciar o negócio na esteira.
func TestPatch_MononymRejectedBeforeRepo(t *testing.T) {
	repo := &captureRepo{patchOk: true, patchResult: &lead.Lead{}}
	svc := &LeadService{repo: repo}

	_, ok, err := svc.Patch(context.Background(), "lead-1", "tenant-1", nil, map[string]interface{}{"Name": "Alice"})
	if ok || err == nil {
		t.Fatalf("expected rejection, got ok=%v err=%v", ok, err)
	}
	if !strings.Contains(err.Error(), "precisa incluir sobrenome") {
		t.Fatalf("erro deveria citar o sobrenome, veio %v", err)
	}
	if len(repo.patchCalls) != 0 {
		t.Fatalf("repositório não pode ser chamado com nome inválido; chamadas: %d", len(repo.patchCalls))
	}
}

// O nome gravado sai normalizado, como o telefone.
func TestPatch_NormalizesNameBeforeRepo(t *testing.T) {
	repo := &captureRepo{patchOk: true, patchResult: &lead.Lead{}}
	svc := &LeadService{repo: repo}

	if _, ok, err := svc.Patch(context.Background(), "lead-1", "tenant-1", nil,
		map[string]interface{}{"Name": "  Alice   Souza "}); err != nil || !ok {
		t.Fatalf("patch falhou: ok=%v err=%v", ok, err)
	}
	if got := repo.patchCalls[0]["Name"]; got != "Alice Souza" {
		t.Fatalf("nome deveria sair normalizado, veio %v", got)
	}
}

// O cadastro (não só a atualização) precisa recusar o telefone inválido antes
// de escrever: o formulário aceitava DDD + 8 dígitos e o operador só descobria
// no erro do servidor. O repositório não pode ser chamado nesse caminho.
func TestCreate_InvalidPhoneRejectedBeforeRepo(t *testing.T) {
	repo := &captureRepo{}
	svc := &LeadService{repo: repo}

	err := svc.Create(context.Background(), &lead.Lead{
		Name:     "Maria Souza",
		Phone:    "8899999999", // DDD 88 + 8 dígitos: celular sem um dígito
		TenantID: "tenant-1",
	})
	if err == nil {
		t.Fatal("esperava recusa do telefone inválido")
	}
	if !strings.Contains(err.Error(), "invalid phone") {
		t.Fatalf("erro deve citar o telefone, veio %v", err)
	}
}

func TestCreate_NormalizesValidPhone(t *testing.T) {
	repo := &captureRepo{}
	svc := &LeadService{repo: repo}
	l := &lead.Lead{Name: "Maria Souza", Phone: "(88) 99999-9999", TenantID: "tenant-1"}

	// Sem repositório de roleta e sem barramento a criação segue até o repo;
	// o que importa aqui é o telefone já normalizado no agregado entregue.
	if err := svc.Create(context.Background(), l); err != nil {
		t.Fatalf("criação deveria passar: %v", err)
	}
	if l.Phone != "+5588999999999" {
		t.Fatalf("telefone deveria ser normalizado, veio %q", l.Phone)
	}
	if len(repo.created) != 1 || repo.created[0].Phone != "+5588999999999" {
		t.Fatalf("repositório recebeu %+v", repo.created)
	}
}
