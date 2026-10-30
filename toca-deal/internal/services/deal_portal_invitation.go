package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// Portal invitation orchestration calls the auth service and persists the
// account projection without changing the DealService public contract.

// ensureClientAccount garante no toca-auth a conta de portal do contato
// (idempotente por e-mail) e devolve o account_id. É o passo que faz o pedido
// de código funcionar: sem conta, o toca-auth responde sucesso genérico ao OTP
// e não envia nada — o contato é desconhecido para o portal.
func (s *DealService) ensureClientAccount(ctx context.Context, tenantID, email, name, phone string) (string, error) {
	authBaseURL := os.Getenv("AUTH_SERVICE_URL")
	internalKey := os.Getenv("INTERNAL_API_KEY")
	if authBaseURL == "" {
		return "", errors.New("AUTH_SERVICE_URL not configured")
	}
	if internalKey == "" {
		return "", errors.New("INTERNAL_API_KEY not configured")
	}
	authURL := authBaseURL + "/api/v1/auth/internal/accounts/ensure-client"

	payload := map[string]string{
		"email":     email,
		"name":      name,
		"phone":     phone,
		"tenant_id": tenantID,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", internalKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call auth service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if len(body) > 0 {
			return "", fmt.Errorf("auth service returned status: %d: %s", resp.StatusCode, string(body))
		}
		return "", fmt.Errorf("auth service returned status: %d", resp.StatusCode)
	}

	var authResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			AccountID string `json:"account_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return "", fmt.Errorf("failed to decode auth response: %w", err)
	}
	if !authResp.Success {
		if authResp.Message != "" {
			return "", fmt.Errorf("failed to ensure client account: %s", authResp.Message)
		}
		return "", fmt.Errorf("failed to ensure client account")
	}
	if authResp.Data.AccountID == "" {
		return "", fmt.Errorf("auth service did not return an account_id")
	}
	return authResp.Data.AccountID, nil
}

// PortalContact é um contato do negócio (parte ou signatário) que precisa de
// conta de portal.
type PortalContact struct {
	Email string
	Name  string
	Phone string
}

// PortalContactsWithoutAccount devolve, deduplicado por e-mail, quem participa
// do negócio (parte ou signatário) e ainda não tem conta de portal em nenhum
// registro. É a lista que o provisionamento cria e a que o comando de reparo
// relata em modo leitura.
func PortalContactsWithoutAccount(d *deal.Deal) []PortalContact {
	if d == nil {
		return nil
	}
	withAccount := map[string]bool{}
	for _, p := range d.Parties {
		if strings.TrimSpace(p.AccountID) != "" {
			withAccount[strings.ToLower(strings.TrimSpace(p.Email))] = true
		}
	}
	for _, sgn := range d.Signers {
		if strings.TrimSpace(sgn.AccountID) != "" {
			withAccount[strings.ToLower(strings.TrimSpace(sgn.Email))] = true
		}
	}

	out := []PortalContact{}
	seen := map[string]bool{}
	add := func(email, name, phone string) {
		email = strings.TrimSpace(email)
		if email == "" {
			return
		}
		key := strings.ToLower(email)
		if seen[key] || withAccount[key] {
			return
		}
		seen[key] = true
		out = append(out, PortalContact{Email: email, Name: name, Phone: phone})
	}
	for _, p := range d.Parties {
		add(p.Email, p.Name, p.Phone)
	}
	for _, sgn := range d.Signers {
		add(sgn.Email, sgn.Name, "")
	}
	return out
}

// assignPortalAccount replica o account_id em toda parte/signatário com o
// mesmo e-mail (a mesma pessoa pode aparecer em mais de um papel) e informa se
// algum registro mudou. Não chama o serviço de identidade.
func assignPortalAccount(d *deal.Deal, email, accountID string) bool {
	if d == nil || accountID == "" {
		return false
	}
	changed := false
	for i := range d.Parties {
		if strings.EqualFold(strings.TrimSpace(d.Parties[i].Email), email) && d.Parties[i].AccountID != accountID {
			d.Parties[i].AccountID = accountID
			changed = true
		}
	}
	for i := range d.Signers {
		if strings.EqualFold(strings.TrimSpace(d.Signers[i].Email), email) && d.Signers[i].AccountID != accountID {
			d.Signers[i].AccountID = accountID
			changed = true
		}
	}
	return changed
}

// ProvisionPortalAccounts garante a conta de portal de TODA parte e
// signatário com e-mail que ainda não tem conta e devolve true quando o
// agregado mudou.
//
// Antes só comprador/locatário eram provisionados: vendedor/locador (role
// owner) e as demais partes do negócio ficavam sem identidade, então pediam o
// código no portal e o toca-auth descartava o pedido em silêncio (200 genérico,
// nenhum e-mail) — o "código de acesso nunca recebido". O portal concede acesso
// a QUALQUER parte ou signatário com `account_id` no negócio
// (FindByPartyAccountID), então a conta é o que faltava para essas pessoas.
//
// Nunca derruba a mutação do negócio: falha do serviço de identidade vira
// advertência no log e o convite manual ("Convidar para o portal") continua
// disponível como reparo.
func (s *DealService) ProvisionPortalAccounts(ctx context.Context, d *deal.Deal) bool {
	if s == nil || d == nil {
		return false
	}

	// Quem já tem conta em um registro e não no outro é a mesma pessoa: replica
	// o account_id conhecido antes de pedir qualquer coisa ao toca-auth.
	known := map[string]string{}
	for _, p := range d.Parties {
		if id := strings.TrimSpace(p.AccountID); id != "" {
			known[strings.ToLower(strings.TrimSpace(p.Email))] = id
		}
	}
	for _, sgn := range d.Signers {
		if id := strings.TrimSpace(sgn.AccountID); id != "" {
			known[strings.ToLower(strings.TrimSpace(sgn.Email))] = id
		}
	}
	changed := false
	for email, accountID := range known {
		if assignPortalAccount(d, email, accountID) {
			changed = true
		}
	}

	for _, contact := range PortalContactsWithoutAccount(d) {
		accountID, err := s.ensureClientAccount(ctx, d.TenantID, contact.Email, contact.Name, contact.Phone)
		if err != nil {
			log.Printf("WARN: failed to provision portal account for deal=%s email=%s: %v", d.ID, contact.Email, err)
			continue
		}
		if assignPortalAccount(d, contact.Email, accountID) {
			changed = true
		}
	}
	return changed
}

// provisionPortalAccounts aplica ProvisionPortalAccounts e persiste o negócio
// quando algo mudou. É o que roda nas mutações que introduzem e-mail no
// agregado (criação, nova parte, novo signatário, edição de contato).
func (s *DealService) provisionPortalAccounts(ctx context.Context, d *deal.Deal) {
	if s == nil || s.repo == nil || d == nil {
		return
	}
	if !s.ProvisionPortalAccounts(ctx, d) {
		return
	}
	if err := s.repo.Update(ctx, d); err != nil {
		log.Printf("WARN: failed to persist portal accounts for deal=%s: %v", d.ID, err)
	}
}

// InviteSigner garante a conta de portal de um signatário e publica o convite —
// é o caminho manual ("Convidar para o portal"). A criação do negócio já
// provisiona as partes sozinha (provisionPortalAccounts); este caminho
// continua valendo para convidar alguém depois.
func (s *DealService) InviteSigner(ctx context.Context, tenantID, dealID, brokerID, signerID string) error {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return err
	}
	if d == nil {
		return errors.New("deal not found")
	}

	var targetSigner *deal.Signer
	for i := range d.Signers {
		if d.Signers[i].ID == signerID {
			targetSigner = &d.Signers[i]
			break
		}
	}

	if targetSigner == nil {
		return errors.New("signer not found")
	}

	if targetSigner.Email == "" {
		return errors.New("signer must have an email to be invited")
	}

	accountID, err := s.ensureClientAccount(ctx, tenantID, targetSigner.Email, targetSigner.Name, "")
	if err != nil {
		return err
	}

	targetSigner.AccountID = accountID
	if partyIdx := findPartyIndexByID(d.Parties, targetSigner.ID); partyIdx != -1 {
		d.Parties[partyIdx].AccountID = accountID
	}
	if err := s.repo.Update(ctx, d); err != nil {
		return fmt.Errorf("failed to update deal with account_id: %w", err)
	}

	if s.eventBus != nil {
		evt := ClientInvitedEvent{
			Email:      targetSigner.Email,
			ClientName: targetSigner.Name,
			Phone:      "",
			AccountID:  targetSigner.AccountID,
			TenantID:   tenantID,
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue client.invited event for account=%s: %v", targetSigner.AccountID, err)
		}
	}

	return nil
}
