package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-comm/internal/domain/message"
	"github.com/oginoino/toca/toca-comm/internal/repository"
)

// P0/A11 x A9 — o chat do portal precisa continuar funcionando.
//
// O ajuste de A11 (P0) pôs `RejectFunctionalRole("cliente")` no grupo inteiro
// `/channels` do toca-comm. Só que o portal do cliente **usa esse grupo**: o
// `usePortalChannels` chama `GET/POST /channels/:id/messages`,
// `/channels/:id/read`, reações, upload e pin/mute — depois de listar os canais
// pelo `GET /client-portal/channels` (toca-deal → `comm/internal/channels`).
// Rejeitar `cliente` no grupo todo apaga o chat do portal.
//
// A separação correta é por superfície:
//
//   - superfícies do TENANT (listar/criar canal, templates, métricas, busca
//     global, administração de participantes) continuam fechadas ao `cliente`;
//   - superfícies do CANAL (mensagens, reações, leitura, pin/mute, upload) são
//     autorizadas por **participação** — que é o que A9 exige que seja
//     fail-closed. Um `cliente` só alcança o canal em que é participante; o
//     mesmo vale para um corretor de outro tenant.
//
// Estes testes travam as duas pontas: o cliente participante entra, o não
// participante não entra, e o cliente não volta a ter superfície de tenant.

const portalClientID = "acc-cliente-portal"

// chatRepo é um dublê em memória com canais de tenants diferentes.
type chatRepo struct {
	repository.MessageRepository
	channels map[string]*message.Channel
	messages []*message.Message
	// tenantFiltered registra se a listagem por participante veio com tenant.
	lastParticipantTenant string
	// participantWrites registra as escritas de participantes (canal, tenant,
	// lista) para provar que uma adição recusada não toca o banco.
	participantWrites []participantWrite
	// leadsByAccount / accountsByLead simulam o vínculo lead ↔ conta do portal
	// (`crm.leads.AccountID`) usado para casar o canal nomeado pelo lead com a
	// conta autenticada.
	leadsByAccount map[string][]string
	accountsByLead map[string]string
}

type participantWrite struct {
	channelID string
	tenantID  string
	list      []string
}

func newChatRepo(channels ...*message.Channel) *chatRepo {
	r := &chatRepo{channels: map[string]*message.Channel{}}
	for _, ch := range channels {
		r.channels[ch.ID] = ch
	}
	return r
}

func (r *chatRepo) GetChannel(_ context.Context, id, tenantID string) (*message.Channel, error) {
	ch, ok := r.channels[id]
	if !ok || ch.TenantID != tenantID {
		return nil, fmt.Errorf("channel not found: %w", message.ErrChannelNotFound)
	}
	return ch, nil
}

func (r *chatRepo) GetChannelByID(_ context.Context, id string) (*message.Channel, error) {
	ch, ok := r.channels[id]
	if !ok {
		return nil, fmt.Errorf("channel not found: %w", message.ErrChannelNotFound)
	}
	return ch, nil
}

// GetChannelForParticipant só devolve o canal se ALGUMA identidade do chamador
// estiver na lista de participantes — o repositório real faz o mesmo no WHERE.
func (r *chatRepo) GetChannelForParticipant(_ context.Context, id string, participantIDs []string) (*message.Channel, error) {
	ch, ok := r.channels[id]
	if !ok || len(participantIDs) == 0 {
		return nil, fmt.Errorf("channel not found: %w", message.ErrChannelNotFound)
	}
	for _, participantID := range participantIDs {
		for _, p := range ch.Participants {
			if p == participantID {
				return ch, nil
			}
		}
	}
	return nil, fmt.Errorf("channel not found: %w", message.ErrChannelNotFound)
}

// LeadIDsForAccount simula a leitura do vínculo lead → conta do portal
// (`crm.leads.AccountID`): um canal aberto pelo painel nomeia o lead, e o
// cliente do portal autentica pela conta.
func (r *chatRepo) LeadIDsForAccount(_ context.Context, _, accountID string) ([]string, error) {
	return r.leadsByAccount[accountID], nil
}

func (r *chatRepo) ClientAccountForLead(_ context.Context, _, leadID string) (string, error) {
	return r.accountsByLead[leadID], nil
}

func (r *chatRepo) ReplaceChannelParticipant(_ context.Context, _, _, fromID, toID string) error {
	for _, ch := range r.channels {
		healed := []string{}
		for _, p := range ch.Participants {
			if p == fromID {
				p = toID
			}
			healed = append(healed, p)
		}
		ch.Participants = healed
	}
	return nil
}

func (r *chatRepo) GetMessages(_ context.Context, channelID, tenantID string, _, _ int) ([]*message.Message, int, error) {
	ch, ok := r.channels[channelID]
	if !ok || ch.TenantID != tenantID {
		return nil, 0, fmt.Errorf("channel not found: %w", message.ErrChannelNotFound)
	}
	out := []*message.Message{}
	for _, m := range r.messages {
		if m.ChannelID == channelID {
			out = append(out, m)
		}
	}
	return out, len(out), nil
}

func (r *chatRepo) ListChannelsByParticipant(_ context.Context, tenantID string, participantIDs []string) ([]*message.Channel, error) {
	r.lastParticipantTenant = tenantID
	out := []*message.Channel{}
	for _, ch := range r.channels {
		if tenantID != "" && ch.TenantID != tenantID {
			continue
		}
		for _, p := range ch.Participants {
			for _, participantID := range participantIDs {
				if p == participantID {
					out = append(out, ch)
					break
				}
			}
		}
	}
	return out, nil
}

func (r *chatRepo) MarkMessagesAsRead(_ context.Context, _, _, _ string) error { return nil }

func (r *chatRepo) SaveMessage(_ context.Context, msg *message.Message) error {
	r.messages = append(r.messages, msg)
	return nil
}

func (r *chatRepo) GetMessage(_ context.Context, id, tenantID, _ string) (*message.Message, error) {
	for _, m := range r.messages {
		if m.ID == id && m.TenantID == tenantID {
			return m, nil
		}
	}
	return nil, fmt.Errorf("message not found: %w", message.ErrChannelNotFound)
}

func (r *chatRepo) UpdateChannelLastMessage(_ context.Context, _, _ string, _ *message.Message) error {
	return nil
}

func (r *chatRepo) UpdateChannelParticipants(_ context.Context, channelID, tenantID string, participants []string) error {
	r.participantWrites = append(r.participantWrites, participantWrite{
		channelID: channelID,
		tenantID:  tenantID,
		list:      append([]string(nil), participants...),
	})
	return nil
}

func (r *chatRepo) AccountBelongsToTenant(_ context.Context, tenantID, accountID string) (bool, error) {
	// Conhecidos no tenant do canal: o cliente do portal, o corretor e um lead.
	switch {
	case tenantID != "tenant-vitima":
		return false, nil
	case accountID == portalClientID, accountID == "acc-corretor", accountID == "acc-lead-do-tenant":
		return true, nil
	}
	return false, nil
}

func newPortalClientChannel() *message.Channel {
	ch := message.NewChannel("tenant-vitima", "Negócio 123", message.ChannelTypeDeal, "acc-corretor", []string{portalClientID, "acc-corretor"})
	ch.ID = "channel-do-cliente"
	return ch
}

func decodeChannels(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("resposta não é JSON de canais: %v (%s)", err, string(body))
	}
	return env.Data
}

// O cliente do portal LÊ as mensagens do canal em que é participante. Antes de
// A9 isto passava sem nenhuma checagem de tenant; depois de A11 passou a ser
// 403 e o chat do portal morreu. O estado correto é este: participa → lê.
func TestPortalChat_ClientReadsOwnChannelMessages(t *testing.T) {
	jwks := newCommTestJWKS(t)
	defer jwks.server.Close()

	ch := newPortalClientChannel()
	repo := newChatRepo(ch)
	r := commRouterWithRepo(t, jwks.server.URL, repo)
	token := jwks.tokenWithRole(t, portalClientID, portalClientID, "cliente")

	w := doCommRequest(t, r, http.MethodGet, "/api/v1/channels/"+ch.ID+"/messages?limit=20&offset=0", "",
		map[string]string{"Authorization": "Bearer " + token})

	if w.Code != http.StatusOK {
		t.Fatalf("chat do portal quebrado: cliente participante recebeu %d ao ler o próprio canal: %s",
			w.Code, w.Body.String())
	}
}

// O cliente do portal ENVIA mensagem no canal em que é participante.
func TestPortalChat_ClientSendsOwnChannelMessage(t *testing.T) {
	jwks := newCommTestJWKS(t)
	defer jwks.server.Close()

	ch := newPortalClientChannel()
	repo := newChatRepo(ch)
	r := commRouterWithRepo(t, jwks.server.URL, repo)
	token := jwks.tokenWithRole(t, portalClientID, portalClientID, "cliente")

	body := `{"content":"olá","content_type":"text"}`
	w := doCommRequest(t, r, http.MethodPost, "/api/v1/channels/"+ch.ID+"/messages", body,
		map[string]string{"Authorization": "Bearer " + token})

	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("cliente participante não conseguiu enviar mensagem (%d): %s", w.Code, w.Body.String())
	}
}

// Cliente que NÃO é participante do canal não lê o histórico — nem no próprio
// tenant, nem em outro. É a autorização do A9 (participação), não o papel.
func TestPortalChat_ClientCannotReadChannelItDoesNotParticipate(t *testing.T) {
	jwks := newCommTestJWKS(t)
	defer jwks.server.Close()

	alheio := message.NewChannel("tenant-vitima", "Negócio de outro", message.ChannelTypeDeal, "acc-corretor", []string{"acc-corretor", "acc-outro-cliente"})
	alheio.ID = "canal-alheio"
	repo := newChatRepo(alheio)
	r := commRouterWithRepo(t, jwks.server.URL, repo)
	token := jwks.tokenWithRole(t, portalClientID, portalClientID, "cliente")

	w := doCommRequest(t, r, http.MethodGet, "/api/v1/channels/"+alheio.ID+"/messages", "",
		map[string]string{"Authorization": "Bearer " + token})

	if w.Code == http.StatusOK {
		t.Fatalf("VAZAMENTO: cliente não-participante leu o histórico do canal (%d): %s", w.Code, w.Body.String())
	}
}

// Superfície de TENANT continua fechada ao cliente: listar todos os canais e
// criar canal seguem exigindo usuário interno (P0/A11).
func TestPortalChat_ClientStillCannotUseTenantWideChannelSurfaces(t *testing.T) {
	jwks := newCommTestJWKS(t)
	defer jwks.server.Close()

	ch := newPortalClientChannel()
	repo := newChatRepo(ch)
	r := commRouterWithRepo(t, jwks.server.URL, repo)
	token := jwks.tokenWithRole(t, portalClientID, portalClientID, "cliente")
	headers := map[string]string{"Authorization": "Bearer " + token}

	if w := doCommRequest(t, r, http.MethodGet, "/api/v1/channels", "", headers); w.Code != http.StatusForbidden {
		t.Errorf("cliente listando todos os canais do tenant: esperado 403, veio %d", w.Code)
	}
	if w := doCommRequest(t, r, http.MethodPost, "/api/v1/channels",
		`{"name":"novo","type":"direct","participants":["`+portalClientID+`"]}`, headers); w.Code != http.StatusForbidden {
		t.Errorf("cliente criando canal: esperado 403, veio %d", w.Code)
	}
	if w := doCommRequest(t, r, http.MethodPost, "/api/v1/channels/"+ch.ID+"/participants",
		`{"participant_id":"acc-cumplice"}`, headers); w.Code != http.StatusForbidden {
		t.Errorf("cliente na administração de participantes: esperado 403, veio %d", w.Code)
	}
}

// A listagem por participante (usada pelo portal via toca-deal) não pode
// devolver canal de outro tenant quando quem chama é uma credencial de serviço
// com allowlist: era o `?participant=` sem predicado de tenant do A3.
func TestPortalChat_InternalChannelListIsTenantScopedForServiceCallers(t *testing.T) {
	doisTenants := []*message.Channel{
		{ID: "canal-a", TenantID: "tenant-a", Participants: []string{"acc-1"}, Type: message.ChannelTypeDeal, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "canal-b", TenantID: "tenant-b", Participants: []string{"acc-1"}, Type: message.ChannelTypeDeal, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	repo := newChatRepo(doisTenants...)
	r := commRouterWithRepo(t, "", repo)
	key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

	w := doCommRequest(t, r, http.MethodGet, "/api/v1/comm/internal/channels?participant=acc-1&tenant_id=tenant-a", "",
		map[string]string{"X-Internal-Key": key})

	if w.Code != http.StatusOK {
		t.Fatalf("listagem interna legítima deveria responder 200, veio %d: %s", w.Code, w.Body.String())
	}
	channels := decodeChannels(t, w.Body.Bytes())
	for _, c := range channels {
		if c["tenant_id"] != "tenant-a" {
			t.Fatalf("VAZAMENTO CROSS-TENANT: credencial de tenant-a recebeu canal de %v", c["tenant_id"])
		}
	}
}

// ─────────── A9: quem entra no canal precisa ser do tenant ───────────
//
// Qualquer membro do canal podia adicionar QUALQUER account id (o UPDATE era
// sem predicado de tenant) e o adicionado, como participante, passava a ler o
// histórico completo — inclusive uma conta de outro tenant, o "cúmplice".
func TestAddParticipant_RejectsAccountUnknownInChannelTenant(t *testing.T) {
	jwks := newCommTestJWKS(t)
	defer jwks.server.Close()

	ch := newPortalClientChannel()
	repo := newChatRepo(ch)
	r := commRouterWithRepo(t, jwks.server.URL, repo)
	token := jwks.tokenWithRole(t, "acc-corretor", "tenant-vitima", "corretor")

	body := `{"participant_id":"acc-cumplice-de-outro-tenant"}`
	w := doCommRequest(t, r, http.MethodPost, "/api/v1/channels/"+ch.ID+"/participants", body,
		map[string]string{"Authorization": "Bearer " + token})

	if w.Code == http.StatusOK || w.Code == http.StatusCreated {
		t.Fatalf("VAZAMENTO CROSS-TENANT: membro adicionou conta de outro tenant ao canal (%d): %s", w.Code, w.Body.String())
	}
	if len(repo.participantWrites) != 0 {
		t.Fatalf("a escrita de participantes aconteceu mesmo com a adição recusada: %+v", repo.participantWrites)
	}
}

// O caminho legítimo continua: membro adiciona alguém CONHECIDO no tenant do
// canal (é o que a busca de participantes do frontend oferece — leads e
// corretores do tenant).
func TestAddParticipant_AllowsAccountKnownInChannelTenant(t *testing.T) {
	jwks := newCommTestJWKS(t)
	defer jwks.server.Close()

	ch := newPortalClientChannel()
	repo := newChatRepo(ch)
	r := commRouterWithRepo(t, jwks.server.URL, repo)
	token := jwks.tokenWithRole(t, "acc-corretor", "tenant-vitima", "corretor")

	body := `{"participant_id":"` + portalClientID + `"}`
	w := doCommRequest(t, r, http.MethodPost, "/api/v1/channels/"+ch.ID+"/participants", body,
		map[string]string{"Authorization": "Bearer " + token})

	if w.Code != http.StatusOK {
		t.Fatalf("adição legítima de participante do tenant falhou (%d): %s", w.Code, w.Body.String())
	}
	// O cliente já estava no canal: nenhuma escrita duplicada é esperada.
	// Adiciona um segundo conhecido para provar a escrita com tenant do canal.
	repo2 := newChatRepo(ch)
	r2 := commRouterWithRepo(t, jwks.server.URL, repo2)
	w2 := doCommRequest(t, r2, http.MethodPost, "/api/v1/channels/"+ch.ID+"/participants",
		`{"participant_id":"acc-lead-do-tenant"}`, map[string]string{"Authorization": "Bearer " + token})
	if w2.Code != http.StatusOK {
		t.Fatalf("adição de segundo participante falhou (%d): %s", w2.Code, w2.Body.String())
	}
	if len(repo2.participantWrites) != 1 {
		t.Fatalf("esperava 1 escrita de participantes, veio %+v", repo2.participantWrites)
	}
	if got := repo2.participantWrites[0].tenantID; got != ch.TenantID {
		t.Fatalf("escrita de participantes sem o tenant do canal: %q", got)
	}
}

// Precedência credencial × JWT nas rotas internas.
//
// `BaseClient.Do` (pkg/clients) manda SEMPRE `X-Internal-Key` e, quando a
// chamada nasce dentro de uma requisição de usuário, encaminha também o JWT
// dele. `InternalOrUserMiddleware` decide pela CHAVE quando ela está presente —
// o JWT só é considerado quando não há chave. É isso que mantém os fluxos do
// portal funcionando: o toca-deal chama `POST /comm/internal/channels/ensure`
// com o tenant do NEGÓCIO (que não é o tenant do token do cliente, cujo
// `tenant_id` é a própria conta) e a chamada é autenticada como serviço.
//
// O teste existe para travar essa precedência: se alguém inverter a ordem
// (JWT primeiro), o portal do cliente passa a receber 403 ao abrir o canal do
// negócio — sem nenhum outro teste quebrando.
func TestInternalRoutes_ServiceKeyTakesPrecedenceOverForwardedUserJWT(t *testing.T) {
	jwks := newCommTestJWKS(t)
	defer jwks.server.Close()

	msgRepo := &recordingMessageRepo{}
	r := commRouterWithRepo(t, jwks.server.URL, msgRepo)
	key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-vitima")
	// Cliente do portal: o tenant no token é a PRÓPRIA CONTA (VerifyOTP).
	clientToken := jwks.tokenWithRole(t, portalClientID, portalClientID, "cliente")

	body := `{"tenant_id":"tenant-vitima","deal_id":"deal-1","participants":["` + portalClientID + `"]}`
	w := doCommRequest(t, r, http.MethodPost, ensurePath, body, map[string]string{
		"X-Internal-Key": key,
		"Authorization":  "Bearer " + clientToken,
	})

	if w.Code != http.StatusOK {
		t.Fatalf("chamada do toca-deal em nome do cliente do portal foi recusada (%d): %s — a chave interna deve ter precedência sobre o JWT encaminhado",
			w.Code, w.Body.String())
	}

	// Sem a chave, o mesmo JWT pedindo o tenant do negócio é cross-tenant.
	w2 := doCommRequest(t, r, http.MethodPost, ensurePath, body,
		map[string]string{"Authorization": "Bearer " + clientToken})
	if w2.Code != http.StatusForbidden {
		t.Fatalf("VAZAMENTO CROSS-TENANT: JWT de cliente abriu canal em outro tenant (%d): %s", w2.Code, w2.Body.String())
	}
}

// Credencial de serviço SEM tenant não lista nada: era o `?participant=` sem
// predicado de tenant que devolvia canais de qualquer tenant para quem tivesse a
// chave interna.
func TestPortalChat_InternalChannelListRequiresTenantForServiceCallers(t *testing.T) {
	repo := newChatRepo(newPortalClientChannel())
	r := commRouterWithRepo(t, "", repo)
	key := withNamedServiceKey(t, "toca-deal", "key-deal")

	w := doCommRequest(t, r, http.MethodGet, "/api/v1/comm/internal/channels?participant=acc-1", "",
		map[string]string{"X-Internal-Key": key})

	if w.Code != http.StatusBadRequest && w.Code != http.StatusForbidden {
		t.Fatalf("listagem interna sem tenant deveria ser recusada, veio %d: %s", w.Code, w.Body.String())
	}
	if repo.lastParticipantTenant != "" {
		t.Fatalf("o repositório foi consultado sem tenant: %q", repo.lastParticipantTenant)
	}
}

// P1/A8 — superfícies de chat do TENANT sem verificação de permissão.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A8) registrou o toca-comm com 40 rotas e ZERO `RequirePermission`:
// busca de mensagens, autocomplete, métricas do chat e templates eram acessíveis
// a qualquer autenticado não-`cliente`. As rotas de CANAL já ficaram
// participant-scoped no A9; estas são as de TENANT, agora com `messages:*`.
func TestChatSurfaces_RequireMessagesPermission(t *testing.T) {
	core := &chatPermissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newCommTestJWKS(t)
	defer jwks.server.Close()
	router := commRouterWithRepo(t, jwks.server.URL, newChatRepo())
	token := jwks.tokenWithRole(t, "acc-corretor", "tenant-1", "corretor")

	cases := []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/autocomplete?q=mar", ""},
		{http.MethodGet, "/api/v1/messages/search?q=abc", ""},
		{http.MethodGet, "/api/v1/metrics", ""},
		{http.MethodGet, "/api/v1/templates", ""},
		{http.MethodPost, "/api/v1/templates", `{"name":"Boas-vindas","category":"geral","content":"Olá"}`},
	}

	// (1) Sem permissão: 403 em todas.
	core.set()
	for _, tc := range cases {
		w := doCommRequest(t, router, tc.method, tc.path, tc.body, map[string]string{"Authorization": "Bearer " + token})
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s %s sem `messages:*` respondeu %d: %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}

	// (2) Só leitura: as leituras passam, a escrita de template continua 403.
	core.set("messages:read")
	for _, tc := range cases[:4] {
		w := doCommRequest(t, router, tc.method, tc.path, tc.body, map[string]string{"Authorization": "Bearer " + token})
		if w.Code == http.StatusForbidden || w.Code == http.StatusUnauthorized {
			t.Fatalf("%s %s com `messages:read` foi barrado: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	if w := doCommRequest(t, router, http.MethodPost, "/api/v1/templates", `{"name":"x","category":"geral","content":"y"}`,
		map[string]string{"Authorization": "Bearer " + token}); w.Code != http.StatusForbidden {
		t.Fatalf("criar template sem `messages:write` respondeu %d: %s", w.Code, w.Body.String())
	}

	// (3) Com escrita: o guarda deixa passar.
	core.set("messages:read", "messages:write")
	if w := doCommRequest(t, router, http.MethodPost, "/api/v1/templates", `{"name":"x","category":"geral","content":"y"}`,
		map[string]string{"Authorization": "Bearer " + token}); w.Code == http.StatusForbidden {
		t.Fatalf("criar template com `messages:write` foi barrado: %s", w.Body.String())
	}
}

// chatPermissionsServer devolve GET /api/v1/me/permissions como o toca-core faria.
type chatPermissionsServer struct {
	mu          sync.Mutex
	permissions []string
}

func (p *chatPermissionsServer) set(perms ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.permissions = perms
}

func (p *chatPermissionsServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		perms := append([]string(nil), p.permissions...)
		p.mu.Unlock()
		if r.URL.Path != "/api/v1/me/permissions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"functional_role": "corretor",
			"profile":         "member",
			"permissions":     perms,
		})
	}))
}

func TestPortalChat_MyChannelsUsesAuthenticatedParticipantWithoutDeal(t *testing.T) {
	jwks := newCommTestJWKS(t)
	own := newPortalClientChannel()
	other := &message.Channel{ID: "other", TenantID: "other-tenant", Participants: []string{"other-client"}}
	repo := newChatRepo(own, other)
	router := commRouterWithRepo(t, jwks.server.URL, repo)
	token := jwks.tokenWithRole(t, portalClientID, portalClientID, "cliente")
	w := doCommRequest(t, router, http.MethodGet, "/api/v1/channels/mine?participant=other-client&tenant_id=other-tenant", "", map[string]string{"Authorization": "Bearer " + token})
	if w.Code != http.StatusOK {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	channels := decodeChannels(t, w.Body.Bytes())
	if len(channels) != 1 || channels[0]["id"] != own.ID {
		t.Fatalf("wrong conversations: %v", channels)
	}
	if repo.lastParticipantTenant != "" {
		t.Fatal("portal token tenant hid participant channel")
	}
	w = doCommRequest(t, router, http.MethodGet, "/api/v1/channels/mine", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status %d", w.Code)
	}
}
