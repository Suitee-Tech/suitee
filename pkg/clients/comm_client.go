package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/oginoino/toca/pkg/domain"
)

type CommClient struct {
	BaseClient *BaseClient
}

func NewCommClient() *CommClient {
	return &CommClient{
		BaseClient: NewBaseClient(os.Getenv("COMM_SERVICE_URL")),
	}
}

type EnsureChannelRequest struct {
	TenantID     domain.TenantID `json:"tenant_id"`
	DealID       string          `json:"deal_id"`
	Name         string          `json:"name"`
	Participants []string        `json:"participants"`
}

type ParticipantProfile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

type ChannelResponse struct {
	ParticipantProfiles []ParticipantProfile `json:"participant_profiles,omitempty"`
	ID                  string               `json:"id"`
	Name                string               `json:"name,omitempty"`
	Description         string               `json:"description,omitempty"`
	Type                string               `json:"type,omitempty"`
	Status              string               `json:"status,omitempty"`
	Participants        []string             `json:"participants"`
	EntityID            string               `json:"entity_id,omitempty"`
	EntityType          string               `json:"entity_type,omitempty"`
	PinnedBy            []string             `json:"pinned_by,omitempty"`
	MutedBy             []string             `json:"muted_by,omitempty"`
	UpdatedAt           string               `json:"updated_at,omitempty"`
	LastMessage         *MessageParticipant  `json:"last_message,omitempty"`
}

// MessageParticipant é um resumo da última mensagem de um canal.
type MessageParticipant struct {
	ID              string `json:"id,omitempty"`
	Content         string `json:"content,omitempty"`
	SenderID        string `json:"sender_id,omitempty"`
	SenderName      string `json:"sender_name,omitempty"`
	SenderAvatarURL string `json:"sender_avatar_url,omitempty"`
	SentAt          string `json:"sent_at,omitempty"`
	ChannelID       string `json:"channel_id,omitempty"`
}

// EnsureDirectChannel acha ou cria um canal direto (1-a-1) entre participantes.
func (c *CommClient) EnsureDirectChannel(ctx context.Context, req EnsureChannelRequest) (*ChannelResponse, error) {
	path := "/api/v1/comm/internal/channels/direct"
	resp, err := c.BaseClient.Do(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("comm service returned status: %d", resp.StatusCode)
	}
	var response struct {
		Data *ChannelResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

// EnsureChannelForDeal creates or gets a channel for a deal
func (c *CommClient) EnsureChannelForDeal(ctx context.Context, req EnsureChannelRequest) (*ChannelResponse, error) {
	path := "/api/v1/comm/internal/channels/ensure"
	resp, err := c.BaseClient.Do(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("comm service returned status: %d", resp.StatusCode)
	}

	var response struct {
		Data *ChannelResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Data, nil
}

// ListChannelsByParticipant lista os canais de um participante DENTRO de um
// tenant.
//
// P1/A3: antes a rota era `?participant=` sem tenant ("across all tenants") e o
// toca-comm devolvia canais de qualquer tenant para quem tivesse a chave
// interna. A credencial de serviço agora precisa dizer o tenant; quem chama em
// nome de um usuário lista os PRÓPRIOS canais pela rota de usuário do toca-comm
// (`GET /api/v1/channels`), onde a autorização é a participação.
func (c *CommClient) ListChannelsByParticipant(ctx context.Context, tenantID, accountID string) ([]*ChannelResponse, error) {
	path := fmt.Sprintf("/api/v1/comm/internal/channels?participant=%s&tenant_id=%s",
		url.QueryEscape(accountID), url.QueryEscape(tenantID))
	resp, err := c.BaseClient.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("comm service returned status: %d", resp.StatusCode)
	}

	var response struct {
		Data []*ChannelResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Data, nil
}

type PushNotificationRequest struct {
	AccountID string      `json:"account_id"`
	Title     string      `json:"title"`
	Message   string      `json:"message"`
	Type      string      `json:"type"`
	Link      string      `json:"link,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

// PushInAppNotification pushes an in-app notification
func (c *CommClient) PushInAppNotification(ctx context.Context, accountID string, req PushNotificationRequest) error {
	path := fmt.Sprintf("/api/v1/comm/internal/notifications/%s", accountID)
	resp, err := c.BaseClient.Do(ctx, http.MethodPost, path, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("comm service returned status: %d", resp.StatusCode)
	}

	return nil
}

type SendNotificationRequest struct {
	Type      string `json:"type"`
	Recipient string `json:"recipient"`
	Subject   string `json:"subject,omitempty"`
	Content   string `json:"content"`
	// TenantID atribui a mensagem ao tenant de origem (P1/A3). Opcional: envios
	// de sistema — e-mail de recuperação de senha, pedido por quem ainda não tem
	// tenant — vão sem ele e ficam registrados como SISTEMA no toca-comm.
	TenantID string `json:"tenant_id,omitempty"`
}

func (c *CommClient) SendNotification(ctx context.Context, req SendNotificationRequest) error {
	// P1/A3: rota interna (`/comm/internal/...`), bloqueada na borda pela
	// denylist de P0/C10. O caminho antigo (`/api/v1/notifications/send`) era
	// alcançável pela internet com a chave interna compartilhada.
	path := "/api/v1/comm/internal/notifications/send"
	resp, err := c.BaseClient.Do(ctx, http.MethodPost, path, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("comm service returned status: %d", resp.StatusCode)
	}
	return nil
}
