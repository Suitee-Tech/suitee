package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
	"github.com/oginoino/toca/toca-comm/internal/domain/notification"
)

type InAppNotificationRepository interface {
	Create(ctx context.Context, n *notification.InAppNotification) error
	// CreateIfNotExists inserts the notification only if no row with the same
	// id exists. Returns true if a new row was inserted, false if a row was
	// already present (idempotent no-op). Used by event-driven handlers that
	// derive deterministic ids from the source event so retries / re-emissions
	// don't generate duplicate notifications.
	CreateIfNotExists(ctx context.Context, n *notification.InAppNotification) (bool, error)
	List(ctx context.Context, tenantID, userID string, limit, offset int) ([]*notification.InAppNotification, int, error)
	GetUnreadCount(ctx context.Context, tenantID, userID string) (int, error)
	MarkAsRead(ctx context.Context, id, tenantID, userID string) error
	MarkAllAsRead(ctx context.Context, tenantID, userID string) error
}

type MessageRepository interface {
	Create(ctx context.Context, msg *message.Message) error
	GetMessage(ctx context.Context, id, tenantID, accountID string) (*message.Message, error)
	GetMessages(ctx context.Context, channelID, tenantID string, limit, offset int) ([]*message.Message, int, error)
	SearchMessages(ctx context.Context, tenantID, accountID string, channelIDs []string, query string, hashtag string, mention string, limit, offset int) ([]*message.Message, int, error)
	UpdateMessageContent(ctx context.Context, id, tenantID, accountID, content string) error
	UpdateMessagePinStatus(ctx context.Context, id, tenantID, accountID string, pinned bool) error
	UpdateMessageReactions(ctx context.Context, id, tenantID, accountID string, reactions []message.Reaction) error
	DeleteMessage(ctx context.Context, id, tenantID, accountID string) error
	SaveMessage(ctx context.Context, msg *message.Message) error

	GetChannel(ctx context.Context, id, tenantID string) (*message.Channel, error)
	// GetChannelForParticipant busca por id SEM tenant, mas com a participação
	// no próprio WHERE — é o único caminho legítimo para quem tem tenant
	// divergente do canal (cliente do portal). Substitui GetChannelByID (busca
	// crua por id), que devolvia o canal para qualquer chamador.
	//
	// Recebe o CONJUNTO de identidades do chamador (conta + leads vinculados,
	// ver LeadIDsForAccount): um canal criado pelo painel antes do convite nomeia
	// o lead, e o portal autentica pela conta — a igualdade exata de id negava o
	// canal ao próprio dono.
	GetChannelForParticipant(ctx context.Context, id string, participantIDs []string) (*message.Channel, error)
	FindDirectChannel(ctx context.Context, tenantID string, participants []string) (*message.Channel, error)
	GetChannelByReference(ctx context.Context, tenantID, refID, refType string) (*message.Channel, error)
	ListChannels(ctx context.Context, tenantID, accountID, role string) ([]*message.Channel, error)
	// ListChannelsByParticipant lista os canais em que alguma identidade do
	// chamador está na lista de participantes. tenantID vazio significa "sem
	// recorte de tenant" e só é aceito quando a autorização É a própria
	// participação (chamador com JWT pedindo os próprios canais); credencial de
	// serviço passa o tenant.
	ListChannelsByParticipant(ctx context.Context, tenantID string, participantIDs []string) ([]*message.Channel, error)
	CreateChannel(ctx context.Context, c *message.Channel) error
	UpdateChannel(ctx context.Context, id, tenantID string, updates map[string]interface{}) (*message.Channel, error)
	UpdateChannelLastMessage(ctx context.Context, tenantID, channelID string, msg *message.Message) error
	UpdateChannelStatus(ctx context.Context, channelID, tenantID string, status message.ChannelStatus) error
	UpdateChannelParticipants(ctx context.Context, channelID, tenantID string, participants []string) error
	UpdateChannelPinned(ctx context.Context, channelID, tenantID, accountID string, pinned bool) error
	UpdateChannelMuted(ctx context.Context, channelID, tenantID, accountID string, muted bool) error

	// AccountBelongsToTenant informa se o account id é conhecido no tenant —
	// lead (por id ou AccountID) ou corretor. Usado para validar quem pode ser
	// adicionado a um canal (achado A9).
	AccountBelongsToTenant(ctx context.Context, tenantID, accountID string) (bool, error)

	// ClientAccountForLead devolve a conta do portal (lead.AccountID) vinculada
	// a um lead do tenant, ou "" quando o cliente ainda não foi convidado.
	//
	// Invariante do canal: participante é a IDENTIDADE COM QUE O PRINCIPAL
	// AUTENTICA — a conta. O lead só entra como participante enquanto não existe
	// conta; assim que ela existe, o canal nomeia a conta (senão o portal lista
	// vazio, não autoriza envio e não entrega em tempo real).
	ClientAccountForLead(ctx context.Context, tenantID, leadID string) (string, error)

	// LeadIDsForAccount devolve os leads do tenant vinculados à conta do portal
	// — as OUTRAS identidades do mesmo principal. É o que permite reconhecer (e
	// curar) canais antigos que ainda nomeiam o lead.
	LeadIDsForAccount(ctx context.Context, tenantID, accountID string) ([]string, error)

	// ReplaceChannelParticipant troca `fromID` por `toID` na lista de
	// participantes do canal (dedupe, atômico no SQL). Usado na cura de canais
	// criados antes do convite, quando o lead passa a ter conta.
	ReplaceChannelParticipant(ctx context.Context, channelID, tenantID, fromID, toID string) error

	GetMetrics(ctx context.Context, tenantID string) (*message.Metrics, error)
	MarkMessagesAsRead(ctx context.Context, channelID, tenantID, participantID string) error

	GetTemplateByID(ctx context.Context, tenantID, id string) (*message.Template, error)
	GetTemplateByName(ctx context.Context, tenantID, name string) (*message.Template, error)
	ListTemplates(ctx context.Context, tenantID, category string) ([]*message.Template, error)
	CreateTemplate(ctx context.Context, t *message.Template) error
	UpdateTemplate(ctx context.Context, t *message.Template) error
	DeleteTemplate(ctx context.Context, tenantID, id string) error
}

type inAppNotificationPostgresRepository struct {
	db *database.PostgresDB
}

func NewInAppNotificationPostgresRepository(db *database.PostgresDB) InAppNotificationRepository {
	return &inAppNotificationPostgresRepository{db: db}
}

func (r *inAppNotificationPostgresRepository) Create(ctx context.Context, notif *notification.InAppNotification) error {
	notif.CreatedAt = time.Now()
	metadata, _ := json.Marshal(notif)
	query := `INSERT INTO comm.inapp_notifications (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query, notif.ID, notif.TenantID, string(metadata)).Scan(&notif.CreatedAt)
}

// CreateIfNotExists honours the InAppNotificationRepository contract by
// inserting the row with ON CONFLICT (id) DO NOTHING. Returns true when a
// new row was created, false when the id was already present.
func (r *inAppNotificationPostgresRepository) CreateIfNotExists(ctx context.Context, notif *notification.InAppNotification) (bool, error) {
	notif.CreatedAt = time.Now()
	metadata, _ := json.Marshal(notif)
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO comm.inapp_notifications (id, tenant_id, metadata) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING`,
		notif.ID, notif.TenantID, string(metadata),
	)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

func (r *inAppNotificationPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*notification.InAppNotification, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM comm.inapp_notifications WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("inapp_notification not found: %w", err)
	}
	var n notification.InAppNotification
	_ = json.Unmarshal(metadata, &n)
	return &n, nil
}

func (r *inAppNotificationPostgresRepository) Update(ctx context.Context, notif *notification.InAppNotification) error {
	metadata, _ := json.Marshal(notif)
	_, err := r.db.ExecContext(ctx, "UPDATE comm.inapp_notifications SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), notif.ID, notif.TenantID)
	return err
}

func (r *inAppNotificationPostgresRepository) List(ctx context.Context, tenantID, accountID string, limit, offset int) ([]*notification.InAppNotification, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM comm.inapp_notifications
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'user_id') = $2
	`, tenantID, accountID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM comm.inapp_notifications
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'user_id') = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, tenantID, accountID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*notification.InAppNotification, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var n notification.InAppNotification
		if err := json.Unmarshal(metadata, &n); err != nil {
			return nil, 0, err
		}
		out = append(out, &n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *inAppNotificationPostgresRepository) GetUnreadCount(ctx context.Context, tenantID, accountID string) (int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM comm.inapp_notifications
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'user_id') = $2 AND COALESCE(metadata->>'is_read','false') = 'false'
	`, tenantID, accountID).Scan(&total)
	return total, err
}

func (r *inAppNotificationPostgresRepository) MarkAllAsRead(ctx context.Context, tenantID, accountID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE comm.inapp_notifications
		SET metadata = jsonb_set(
			jsonb_set(metadata, '{is_read}', 'true'::jsonb, true),
			'{read_at}',
			to_jsonb(NOW()),
			true
		),
		updated_at = NOW()
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'user_id') = $2 AND COALESCE(metadata->>'is_read','false') = 'false'
	`, tenantID, accountID)
	return err
}

func (r *inAppNotificationPostgresRepository) MarkAsRead(ctx context.Context, id, tenantID, accountID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE comm.inapp_notifications
		SET metadata = jsonb_set(
			jsonb_set(metadata, '{is_read}', 'true'::jsonb, true),
			'{read_at}',
			to_jsonb(NOW()),
			true
		),
		updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL AND (metadata->>'user_id') = $3
	`, id, tenantID, accountID)
	return err
}

type messagePostgresRepository struct {
	db *database.PostgresDB
}

func NewMessagePostgresRepository(db *database.PostgresDB) MessageRepository {
	return &messagePostgresRepository{db: db}
}

func (r *messagePostgresRepository) CreateChannel(ctx context.Context, c *message.Channel) error {
	c.CreatedAt = time.Now()
	c.UpdatedAt = c.CreatedAt
	metadata, _ := json.Marshal(c)
	query := `INSERT INTO comm.channels (id, tenant_id, metadata) VALUES ($1, $2, $3)`
	_, err := r.db.ExecContext(ctx, query, c.ID, c.TenantID, string(metadata))
	return err
}

func (r *messagePostgresRepository) GetChannel(ctx context.Context, id, tenantID string) (*message.Channel, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM comm.channels
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`, id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, channelLookupError(err)
	}
	var c message.Channel
	if err := json.Unmarshal(metadata, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// GetChannelForParticipant busca um canal por id sem filtro de tenant, exigindo
// no próprio SQL que ALGUMA identidade do chamador seja PARTICIPANTE.
//
// SECURITY (achado A9 da revisão de permissões de 2026-09-19): antes existia
// `GetChannelByID`, que devolvia o canal (e o histórico) para qualquer chamador
// que soubesse o id, e o serviço caía nele em qualquer erro de `GetChannel`. O
// caso legítimo — cliente do portal cujo tenant no token é a própria conta —
// continua atendido porque ele é participante do canal do seu negócio; um
// account que não participa não recebe nada, mesmo com o id em mãos.
//
// As identidades vêm do chamador (conta + leads vinculados) e são o MESMO
// principal: o lead só é aceito quando `crm.leads.AccountID` aponta para a conta
// autenticada, então isto não amplia acesso — apenas compara pelo conjunto certo
// (incidente das mensagens do portal, 2026-09-21).
func (r *messagePostgresRepository) GetChannelForParticipant(ctx context.Context, id string, participantIDs []string) (*message.Channel, error) {
	if len(participantIDs) == 0 {
		return nil, message.ErrChannelNotFound
	}
	var metadata []byte
	err := r.db.QueryServiceRow(ctx, `
		SELECT metadata
		FROM comm.channels
		WHERE id = $1 AND deleted_at IS NULL AND (metadata->'participants') ?| $2::text[]
	`, id, participantIDs).Scan(&metadata)
	if err != nil {
		return nil, channelLookupError(err)
	}
	var c message.Channel
	if err := json.Unmarshal(metadata, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// channelLookupError traduz "sem linhas" para o sentinel de domínio e preserva
// qualquer outra falha (banco fora do ar não pode virar "canal não encontrado",
// que autorizaria o fallback por participação).
func channelLookupError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w", message.ErrChannelNotFound)
	}
	return fmt.Errorf("channel lookup failed: %w", err)
}

// FindDirectChannel busca um canal "direct" com EXATAMENTE os participantes
// dados (conversa corretor+cliente já existente — reutiliza em vez de criar).
func (r *messagePostgresRepository) FindDirectChannel(ctx context.Context, tenantID string, participants []string) (*message.Channel, error) {
	if tenantID == "" || len(participants) == 0 {
		return nil, message.ErrChannelNotFound
	}
	partJSON, _ := json.Marshal(participants)
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
  SELECT c.metadata FROM comm.channels c
  CROSS JOIN LATERAL (
   SELECT jsonb_agg(DISTINCT COALESCE(NULLIF(l.metadata->>'AccountID', ''), p.id)) AS ids
   FROM jsonb_array_elements_text(c.metadata->'participants') AS p(id)
   LEFT JOIN crm.leads l ON l.id::text = p.id
    AND l.tenant_id = c.tenant_id AND l.deleted_at IS NULL
  ) normalized
  WHERE c.tenant_id = $1 AND c.deleted_at IS NULL
   AND c.metadata->>'type' = 'direct'
   AND COALESCE(c.metadata->>'status', 'open') = 'open'
   AND normalized.ids @> $2::jsonb AND normalized.ids <@ $2::jsonb
  ORDER BY c.created_at, c.id
  LIMIT 1
 `, tenantID, string(partJSON)).Scan(&metadata)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, message.ErrChannelNotFound
	}
	if err != nil {
		return nil, err
	}
	var c message.Channel
	if err := json.Unmarshal(metadata, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *messagePostgresRepository) GetChannelByReference(ctx context.Context, tenantID, refID, refType string) (*message.Channel, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM comm.channels
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'entity_id') = $2 AND (metadata->>'entity_type') = $3
		ORDER BY created_at DESC
		LIMIT 1
	`, tenantID, refID, refType).Scan(&metadata)
	if err != nil {
		return nil, channelLookupError(err)
	}
	var c message.Channel
	if err := json.Unmarshal(metadata, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *messagePostgresRepository) ListChannels(ctx context.Context, tenantID, accountID, role string) ([]*message.Channel, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM comm.channels
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->'participants') ? $2
		ORDER BY updated_at DESC
	`, tenantID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*message.Channel, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var c message.Channel
		if err := json.Unmarshal(metadata, &c); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *messagePostgresRepository) ListChannelsByParticipant(ctx context.Context, tenantID string, participantIDs []string) ([]*message.Channel, error) {
	// Participation is checked in SQL across tenants using the service pool.
	// P1/A3: o `?participant=` interno não tinha predicado de tenant — uma
	// credencial de serviço listava canais de qualquer tenant. $1 vazio mantém o
	// recorte por participação (chamador pedindo os PRÓPRIOS canais, caso do
	// cliente do portal, cujo tenant no token é a conta dele).
	//
	// O casamento é por CONJUNTO de identidades (`?|`): o canal criado pelo
	// painel nomeia o lead e o portal autentica pela conta — as duas são o mesmo
	// principal (incidente das mensagens do portal, 2026-09-21).
	rows, err := r.db.QueryServiceRows(ctx, `
		SELECT metadata
		FROM comm.channels
		WHERE deleted_at IS NULL
		  AND (metadata->'participants') ?| $2::text[]
		  AND ($1 = '' OR tenant_id = NULLIF($1, '')::uuid)
		ORDER BY updated_at DESC
	`, tenantID, participantIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*message.Channel, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var c message.Channel
		if err := json.Unmarshal(metadata, &c); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *messagePostgresRepository) UpdateChannel(ctx context.Context, id, tenantID string, updates map[string]interface{}) (*message.Channel, error) {
	c, err := r.GetChannel(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	for k, v := range updates {
		m[k] = v
	}
	m["updated_at"] = time.Now().Format(time.RFC3339Nano)

	metadata, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	_, err = r.db.ExecContext(ctx, `
		UPDATE comm.channels
		SET metadata = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL
	`, string(metadata), id, tenantID)
	if err != nil {
		return nil, err
	}

	return r.GetChannel(ctx, id, tenantID)
}

func (r *messagePostgresRepository) UpdateChannelMuted(ctx context.Context, channelID, tenantID, accountID string, muted bool) error {
	// P1/A9: leitura e escrita do canal passam a exigir o tenant resolvido pelo
	// serviço (o canal já foi autorizado por participação antes daqui).
	var metadata []byte
	if err := r.db.QueryRowContext(ctx, "SELECT metadata FROM comm.channels WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", channelID, tenantID).Scan(&metadata); err != nil {
		return channelLookupError(err)
	}
	var c message.Channel
	if err := json.Unmarshal(metadata, &c); err != nil {
		return err
	}
	set := map[string]bool{}
	for _, id := range c.MutedBy {
		set[id] = true
	}
	if muted {
		set[accountID] = true
	} else {
		delete(set, accountID)
	}
	c.MutedBy = make([]string, 0, len(set))
	for id := range set {
		c.MutedBy = append(c.MutedBy, id)
	}
	c.UpdatedAt = time.Now()
	metadata, _ = json.Marshal(&c)
	return r.execChannelWrite(ctx, "UPDATE comm.channels SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), channelID, tenantID)
}

func (r *messagePostgresRepository) UpdateChannelParticipants(ctx context.Context, channelID, tenantID string, participants []string) error {
	return r.execChannelWrite(ctx, `
		UPDATE comm.channels
		SET metadata = jsonb_set(metadata, '{participants}', to_jsonb($1::text[]), true),
		    updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL
	`, participants, channelID, tenantID)
}

// execChannelWrite roda uma escrita de canal e trata "0 linhas" como erro em vez
// de sucesso silencioso: com predicado de tenant no WHERE, 0 linhas significa
// que o canal não é do tenant resolvido (achado B12/A9 da revisão).
func (r *messagePostgresRepository) execChannelWrite(ctx context.Context, query string, args ...interface{}) error {
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("%w", message.ErrChannelNotFound)
	}
	return nil
}

func (r *messagePostgresRepository) UpdateChannelPinned(ctx context.Context, channelID, tenantID, accountID string, pinned bool) error {
	var metadata []byte
	if err := r.db.QueryRowContext(ctx, "SELECT metadata FROM comm.channels WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", channelID, tenantID).Scan(&metadata); err != nil {
		return channelLookupError(err)
	}
	var c message.Channel
	if err := json.Unmarshal(metadata, &c); err != nil {
		return err
	}
	set := map[string]bool{}
	for _, id := range c.PinnedBy {
		set[id] = true
	}
	if pinned {
		set[accountID] = true
	} else {
		delete(set, accountID)
	}
	c.PinnedBy = make([]string, 0, len(set))
	for id := range set {
		c.PinnedBy = append(c.PinnedBy, id)
	}
	c.UpdatedAt = time.Now()
	metadata, _ = json.Marshal(&c)
	return r.execChannelWrite(ctx, "UPDATE comm.channels SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), channelID, tenantID)
}

func (r *messagePostgresRepository) UpdateChannelStatus(ctx context.Context, channelID, tenantID string, status message.ChannelStatus) error {
	return r.execChannelWrite(ctx, `
		UPDATE comm.channels
		SET metadata = jsonb_set(metadata, '{status}', to_jsonb($1::text), true),
		    updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL
	`, string(status), channelID, tenantID)
}

func (r *messagePostgresRepository) UpdateMessageContent(ctx context.Context, id, tenantID, accountID, content string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE comm.messages
		SET metadata = jsonb_set(
			jsonb_set(
				jsonb_set(metadata, '{content}', to_jsonb($1::text), true),
				'{is_edited}',
				'true'::jsonb,
				true
			),
			'{edited_at}',
			to_jsonb(NOW()),
			true
		),
		updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL AND (metadata->>'sender_id') = $4
	`, content, id, tenantID, accountID)
	return err
}

func (r *messagePostgresRepository) UpdateMessagePinStatus(ctx context.Context, id, tenantID, accountID string, pinned bool) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE comm.messages
		SET metadata = jsonb_set(metadata, '{is_pinned}', to_jsonb($1::bool), true),
		    updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL
	`, pinned, id, tenantID)
	return err
}

func (r *messagePostgresRepository) UpdateMessageReactions(ctx context.Context, id, tenantID, accountID string, reactions []message.Reaction) error {
	b, err := json.Marshal(reactions)
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE comm.messages
		SET metadata = jsonb_set(metadata, '{reactions}', $1::jsonb, true),
		    updated_at = NOW()
		WHERE tenant_id = $2
		  AND deleted_at IS NULL
		  AND (id::text = $3 OR (metadata->>'id') = $3)
	`, string(b), tenantID, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("message not found")
	}
	return nil
}

// AccountBelongsToTenant informa se o account id participa do tenant como lead
// (por id ou por AccountID) ou corretor.
//
// SECURITY (achado A9 da revisão de permissões de 2026-09-19): `AddParticipant`
// só exigia que o CHAMADOR fosse membro do canal — qualquer membro adicionava
// qualquer account_id, inclusive de outro tenant, e o cúmplice passava a ler o
// histórico. A validação usa as mesmas fontes que `GetParticipantProfiles` já
// consulta por tenant (leads/corretores), então também aceita o cliente do
// portal, cujo vínculo com o tenant é o `AccountID` do lead.
func (r *messagePostgresRepository) AccountBelongsToTenant(ctx context.Context, tenantID, accountID string) (bool, error) {
	if tenantID == "" || accountID == "" {
		return false, nil
	}
	var exists bool
	err := r.db.QueryServiceRow(ctx, `
		WITH me AS (
			SELECT COALESCE(NULLIF(a.metadata->'Individual'->>'Email', ''), a.metadata->>'Email', '') AS email
			FROM toca_auth.accounts a
			WHERE a.id::text = $2
		)
		SELECT EXISTS (
			SELECT 1 FROM crm.leads
			WHERE tenant_id = $1 AND deleted_at IS NULL
			  AND (id::text = $2 OR metadata->>'AccountID' = $2)
			UNION ALL
			SELECT 1 FROM crm.brokers b, me
			WHERE b.tenant_id = $1 AND b.deleted_at IS NULL
			  AND (b.id::text = $2
			       OR (me.email <> '' AND lower(b.metadata->>'Email') = lower(me.email)))
		)
	`, tenantID, accountID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// ClientAccountForLead devolve a conta do portal vinculada ao lead, ou "".
//
// Invariante do canal (incidente "lead convidado não vê a mensagem do corretor",
// 2026-09-21): participante é a identidade com que o principal AUTENTICA — a
// conta. O painel da imobiliária escolhe o lead (é o id que ele conhece), então
// a escrita do canal precisa traduzir lead → conta; sem isso o portal
// (autenticado como conta) não lista, não autoriza envio nem recebe em tempo
// real o canal em que ele é o próprio dono.
func (r *messagePostgresRepository) ClientAccountForLead(ctx context.Context, tenantID, leadID string) (string, error) {
	if tenantID == "" || leadID == "" {
		return "", nil
	}
	var accountID string
	err := r.db.QueryServiceRow(ctx, `
		SELECT COALESCE(metadata->>'AccountID', '')
		FROM crm.leads
		WHERE tenant_id = $1 AND deleted_at IS NULL AND id::text = $2
	`, tenantID, leadID).Scan(&accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return accountID, nil
}

// LeadIDsForAccount devolve os leads cuja conta do portal é a conta autenticada
// — as outras identidades do MESMO principal.
//
// É o par de `ClientAccountForLead` no sentido inverso: canais criados antes do
// convite (ou antes desta correção) ainda nomeiam o lead, e a leitura precisa
// reconhecê-los como sendo daquele cliente. tenantID vazio busca em qualquer
// tenant: é o caso do cliente do portal, cujo token não traz o tenant da
// imobiliária e que só pode, ainda assim, receber os PRÓPRIOS canais.
func (r *messagePostgresRepository) LeadIDsForAccount(ctx context.Context, tenantID, accountID string) ([]string, error) {
	out := []string{}
	if accountID == "" {
		return out, nil
	}
	rows, err := r.db.QueryServiceRows(ctx, `
		SELECT id::text
		FROM crm.leads
		WHERE deleted_at IS NULL
		  AND metadata->>'AccountID' = $2
		  AND ($1 = '' OR tenant_id = NULLIF($1, '')::uuid)
	`, tenantID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ReplaceChannelParticipant troca uma identidade por outra na lista de
// participantes, deduplicando e preservando as demais. Atômico no SQL para não
// perder participantes adicionados em paralelo (o serviço reescrevia a lista
// inteira a partir de uma cópia lida antes).
//
// Sem linhas afetadas = canal inexistente no tenant ou identidade já ausente;
// `execChannelWrite` traduz isso em ErrChannelNotFound, que os chamadores de
// cura tratam como "nada a fazer".
func (r *messagePostgresRepository) ReplaceChannelParticipant(ctx context.Context, channelID, tenantID, fromID, toID string) error {
	if channelID == "" || tenantID == "" || fromID == "" || toID == "" || fromID == toID {
		return nil
	}
	return r.execChannelWrite(ctx, `
		UPDATE comm.channels
		SET metadata = jsonb_set(
		        metadata,
		        '{participants}',
		        (SELECT COALESCE(jsonb_agg(DISTINCT CASE WHEN p = $3 THEN $4 ELSE p END), '[]'::jsonb)
		           FROM jsonb_array_elements_text(metadata->'participants') AS t(p)),
		        true
		    ),
		    updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
		  AND (metadata->'participants') ? $3
	`, channelID, tenantID, fromID, toID)
}

func (r *messagePostgresRepository) UpdateChannelLastMessage(ctx context.Context, tenantID, channelID string, msg *message.Message) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		UPDATE comm.channels
		SET metadata = jsonb_set(metadata, '{last_message}', $1::jsonb, true),
		    updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL
	`, string(b), channelID, tenantID)
	return err
}

func (r *messagePostgresRepository) CreateTemplate(ctx context.Context, t *message.Template) error {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	metadata, _ := json.Marshal(t)
	_, err := r.db.ExecContext(ctx, `INSERT INTO comm.templates (id, tenant_id, metadata) VALUES ($1, $2, $3)`, t.ID, t.TenantID, string(metadata))
	return err
}

func (r *messagePostgresRepository) DeleteTemplate(ctx context.Context, tenantID, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE comm.templates SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`, id, tenantID)
	return err
}

func (r *messagePostgresRepository) Create(ctx context.Context, msg *message.Message) error {
	return r.SaveMessage(ctx, msg)
}

func (r *messagePostgresRepository) GetTemplateByID(ctx context.Context, tenantID, id string) (*message.Template, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `SELECT metadata FROM comm.templates WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`, id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("template not found: %w", err)
	}
	var t message.Template
	if err := json.Unmarshal(metadata, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *messagePostgresRepository) GetTemplateByName(ctx context.Context, tenantID, name string) (*message.Template, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM comm.templates
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'name') = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, tenantID, name).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("template not found: %w", err)
	}
	var t message.Template
	if err := json.Unmarshal(metadata, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *messagePostgresRepository) ListTemplates(ctx context.Context, tenantID, category string) ([]*message.Template, error) {
	args := []any{tenantID}
	query := `
		SELECT metadata
		FROM comm.templates
		WHERE tenant_id = $1 AND deleted_at IS NULL
	`
	if strings.TrimSpace(category) != "" {
		query += " AND (metadata->>'category') = $2"
		args = append(args, category)
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*message.Template, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var t message.Template
		if err := json.Unmarshal(metadata, &t); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *messagePostgresRepository) UpdateTemplate(ctx context.Context, t *message.Template) error {
	metadata, _ := json.Marshal(t)
	_, err := r.db.ExecContext(ctx, `UPDATE comm.templates SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL`, string(metadata), t.ID, t.TenantID)
	return err
}

func (r *messagePostgresRepository) DeleteMessage(ctx context.Context, id, tenantID, accountID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE comm.messages SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`, id, tenantID)
	return err
}

func (r *messagePostgresRepository) MarkMessagesAsRead(ctx context.Context, channelID, tenantID, participantID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE comm.messages
		SET metadata = jsonb_set(metadata, '{read_by}', COALESCE(metadata->'read_by','[]'::jsonb) || jsonb_build_array($1::text), true),
		    updated_at = NOW()
		WHERE tenant_id = $2
		  AND deleted_at IS NULL
		  AND (metadata->>'channel_id') = $3
		  AND NOT (COALESCE(metadata->'read_by','[]'::jsonb) ? $1)
	`, participantID, tenantID, channelID)
	return err
}

func (r *messagePostgresRepository) SaveMessage(ctx context.Context, msg *message.Message) error {
	metadata, _ := json.Marshal(msg)
	query := `INSERT INTO comm.messages (id, tenant_id, metadata) VALUES ($1, $2, $3)`
	_, err := r.db.ExecContext(ctx, query, msg.ID, msg.TenantID, string(metadata))
	return err
}

func (r *messagePostgresRepository) SearchMessages(ctx context.Context, tenantID, accountID string, channelIDs []string, query string, hashtag string, mention string, limit, offset int) ([]*message.Message, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	where := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []any{tenantID}
	argN := 2

	if len(channelIDs) > 0 {
		where = append(where, fmt.Sprintf("(metadata->>'channel_id') = ANY($%d)", argN))
		args = append(args, channelIDs)
		argN++
	}

	if s := strings.TrimSpace(query); s != "" {
		where = append(where, fmt.Sprintf("(metadata->>'content') ILIKE $%d", argN))
		args = append(args, "%"+s+"%")
		argN++
	}

	if tag := strings.TrimSpace(hashtag); tag != "" {
		where = append(where, fmt.Sprintf("(metadata->'hashtags') ? $%d", argN))
		args = append(args, tag)
		argN++
	}

	if participant := strings.TrimSpace(mention); participant != "" {
		where = append(where, fmt.Sprintf("(metadata->'mentions') ? $%d", argN))
		args = append(args, participant)
		argN++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM comm.messages "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	qSQL := fmt.Sprintf("SELECT metadata FROM comm.messages %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.QueryContext(ctx, qSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*message.Message, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var m message.Message
		if err := json.Unmarshal(metadata, &m); err != nil {
			return nil, 0, err
		}
		out = append(out, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

func (r *messagePostgresRepository) GetMessage(ctx context.Context, id, tenantID, accountID string) (*message.Message, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM comm.messages
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (id::text = $2 OR (metadata->>'id') = $2)
		LIMIT 1
	`, tenantID, id).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("message not found")
		}
		return nil, err
	}
	var m message.Message
	if err := json.Unmarshal(metadata, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *messagePostgresRepository) Update(ctx context.Context, msg *message.Message) error {
	metadata, _ := json.Marshal(msg)
	_, err := r.db.ExecContext(ctx, "UPDATE comm.messages SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), msg.ID, msg.TenantID)
	return err
}

func (r *messagePostgresRepository) GetMessages(ctx context.Context, channelID, tenantID string, limit, offset int) ([]*message.Message, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM comm.messages
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'channel_id') = $2
	`, tenantID, channelID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM comm.messages
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'channel_id') = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, tenantID, channelID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*message.Message, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var m message.Message
		if err := json.Unmarshal(metadata, &m); err != nil {
			return nil, 0, err
		}
		out = append(out, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

func (r *messagePostgresRepository) GetMetrics(ctx context.Context, tenantID string) (*message.Metrics, error) {
	var totalMessages int64
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM comm.messages WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&totalMessages)

	var totalChannels int64
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM comm.channels WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&totalChannels)

	var activeUsers int64
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT (metadata->>'sender_id')) FROM comm.messages WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&activeUsers)

	var flagged int64
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM comm.messages WHERE tenant_id = $1 AND deleted_at IS NULL AND COALESCE(metadata->>'is_flagged','false') = 'true'`, tenantID).Scan(&flagged)

	return &message.Metrics{
		TotalMessages:      totalMessages,
		TotalChannels:      totalChannels,
		ActiveUsers:        activeUsers,
		FlaggedMessages:    flagged,
		AvgResponseTimeMin: 0,
	}, nil
}

func (r *messagePostgresRepository) GetTemplate(ctx context.Context, id, tenantID string) (*message.Template, error) {
	return r.GetTemplateByID(ctx, tenantID, id)
}
