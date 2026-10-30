package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oginoino/toca/pkg/logger"
)

type MCPSession struct {
	c *client.Client

	// callMu serializa as chamadas de ferramenta nesta sessão.
	//
	// A sessão é um único `SSEMCPClient`, que correlaciona cada resposta pelo
	// id da requisição no mesmo canal. Fan-out paralelo do modelo (o loop de
	// chat despacha N tool calls em goroutines) faz várias chamadas concorrentes
	// disputarem esse canal, e o transporte morre no meio: em produção, 13 de 28
	// chamadas (46%) voltaram como "transport error: context canceled" ou
	// "failed to send request", e o stream do chat fechava sem evento de
	// conclusão — a Suitee Intelligence ficava em "Analisando..." e a resposta
	// saía incompleta, porque as ferramentas que falharam simplesmente não
	// entravam no contexto do modelo.
	//
	// O paralelismo não é o que dá o ganho aqui: o custo real é a ida ao banco
	// dentro do MCP, e as chamadas já têm timeout individual de 60s. Serializar
	// troca um ganho marginal de latência por chamadas que de fato completam.
	callMu sync.Mutex

	// Instrumentação: id curto para correlacionar com os logs do servidor MCP e
	// contador de chamadas, para saber se uma falha é da primeira chamada da
	// sessão ou de uma que veio depois do transporte já ter morrido.
	id      string
	callSeq int

	// closing marca o encerramento PEDIDO por nós (`defer session.Close()` no
	// handler). O stream SSE cai nos dois casos — fechamento intencional e queda
	// real — e o hook `OnConnectionLost` não distingue um do outro: sem o
	// marcador, todo fim de request virava `ERROR mcp stream sse caiu:
	// context canceled` no log, ruído que esconde a queda de verdade.
	closing *atomic.Bool
}

// streamLossIsAnAlarm diz se a perda do stream SSE é um ALARME ou apenas o
// encerramento que nós mesmos pedimos.
//
// O stream cai nos dois casos e o SDK não distingue um do outro. Sem esta
// separação, cada fim de request (o handler fecha a sessão em `defer`) gerava
// `ERROR mcp stream sse caiu: context canceled` — ruído que esconde a queda
// real, que é justamente a informação que o hook existe para dar.
func streamLossIsAnAlarm(closing *atomic.Bool) bool {
	return closing == nil || !closing.Load()
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func isErrorTexto(isErr bool) string {
	if isErr {
		return "tool_is_error=true"
	}
	return ""
}

func NewMCPClient(serverURL string) *MCPClient {
	return &MCPClient{serverURL: normalizeMCPServerURL(serverURL)}
}

type MCPClient struct {
	serverURL string
}

func (s *MCPSession) Close() error {
	if s == nil {
		return nil
	}
	// Marca ANTES de qualquer saída antecipada: o encerramento é intencional
	// mesmo quando não há cliente para fechar.
	if s.closing != nil {
		s.closing.Store(true)
	}
	if s.c == nil {
		return nil
	}
	return s.c.Close()
}

func normalizeMCPServerURL(in string) string {
	in = strings.TrimSpace(in)
	in = strings.ReplaceAll(in, "`", "")
	in = strings.Trim(in, `"`)
	in = strings.Trim(in, `'`)
	return strings.TrimRight(in, "/")
}

func (c *MCPClient) SanitizedServerURL() string {
	return normalizeMCPServerURL(c.serverURL)
}

func (c *MCPClient) ResolveSSEEndpoint() (string, error) {
	base := normalizeMCPServerURL(c.serverURL)
	if base == "" {
		return "", fmt.Errorf("mcp server url not configured")
	}

	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}

	p := strings.TrimRight(u.Path, "/")
	switch {
	case strings.HasSuffix(p, "/mcp/sse"):
		u.Path = p
	case strings.HasSuffix(p, "/mcp"):
		u.Path = p + "/sse"
	default:
		u.Path = path.Join(p, "/mcp/sse")
	}

	return normalizeMCPServerURL(u.String()), nil
}

func (c *MCPClient) Connect(ctx context.Context, tenantID, authToken string) (*MCPSession, error) {
	endpoint, err := c.ResolveSSEEndpoint()
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"X-Tenant-Id":   tenantID,
		"Authorization": "Bearer " + authToken,
	}

	// O stream SSE da sessão NÃO pode herdar o ciclo de vida do request HTTP.
	//
	// `Start(ctx)` amarra o stream ao contexto recebido: quando o request
	// termina — ou o cliente desconecta — o Go cancela esse contexto, o stream
	// SSE cai e a sessão é removida do mapa do servidor. Qualquer chamada de
	// ferramenta que chegue depois recebe 400 "Invalid session ID", e a que
	// estava em voo espera os 60s de timeout.
	//
	// Medido: com 3 ferramentas em um ciclo, as duas primeiras completavam
	// (37ms e 47ms) e a terceira falhava porque o stream fechou 0,7ms antes de
	// ela chegar. O hook de perda de conexão registrou `context canceled` — o
	// cancelamento do request, não uma queda de rede.
	//
	// `WithoutCancel` preserva tenant e token (que o servidor MCP lê dos
	// headers e do contexto) e solta só o cancelamento. A sessão passa a viver
	// até o handler fechá-la explicitamente (`defer session.Close()`), que é o
	// dono do ciclo.
	sessionCtx := context.WithoutCancel(ctx)

	closing := &atomic.Bool{}

	// Retry com backoff: conexões simultâneas (listTools + chat) podem colidir
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt*300) * time.Millisecond):
			}
		}

		mcpClient, err := client.NewSSEMCPClient(
			endpoint,
			client.WithHeaders(headers),
			transport.WithEndpointTimeout(15*time.Second),
			transport.WithResponseTimeout(60*time.Second),
		)
		if err != nil {
			lastErr = err
			continue
		}

		// O hook de perda de conexão é a única forma de saber que o stream SSE
		// caiu. Sem ele, uma sessão que morre no meio de um ciclo de ferramentas
		// é indistinguível de um cliente usando sessão errada — o sintoma dos
		// dois é o mesmo timeout de 60s no CallTool.
		mcpClient.OnConnectionLost(func(err error) {
			if !streamLossIsAnAlarm(closing) {
				logger.Info("mcp sessão encerrada pelo handler",
					"endpoint", endpoint, "motivo", errString(err))
				return
			}
			logger.Error("mcp stream sse caiu", "endpoint", endpoint, "erro", errString(err))
		})

		if err := mcpClient.Start(sessionCtx); err != nil {
			_ = mcpClient.Close()
			lastErr = err
			continue
		}

		ctxInit, cancel := context.WithTimeout(sessionCtx, 15*time.Second)
		initReq := mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ClientInfo: mcp.Implementation{
					Name:    "toca-ai",
					Version: "1.0.0",
				},
				Capabilities: mcp.ClientCapabilities{},
			},
		}
		_, initErr := mcpClient.Initialize(ctxInit, initReq)
		cancel()
		if initErr != nil {
			_ = mcpClient.Close()
			lastErr = initErr
			continue
		}

		return &MCPSession{c: mcpClient, id: uuid.NewString()[:8], closing: closing}, nil
	}

	return nil, fmt.Errorf("mcp connect failed after 3 attempts: %w", lastErr)
}

func (c *MCPClient) ListTools(ctx context.Context, session *MCPSession) ([]DeepseekTool, error) {
	if session == nil || session.c == nil {
		return []DeepseekTool{}, nil
	}

	resp, err := session.c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}

	out := make([]DeepseekTool, 0, len(resp.Tools))
	for _, t := range resp.Tools {
		schemaBytes, err := json.Marshal(t.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool schema name=%s: %w", t.Name, err)
		}

		var schema any
		if err := json.Unmarshal(schemaBytes, &schema); err != nil {
			return nil, fmt.Errorf("failed to parse tool schema name=%s: %w", t.Name, err)
		}

		readOnly := false
		if t.Annotations.ReadOnlyHint != nil {
			readOnly = *t.Annotations.ReadOnlyHint
		}
		domain := ""
		minRole := ""
		if t.Meta != nil && t.Meta.AdditionalFields != nil {
			if v, ok := t.Meta.AdditionalFields["x-domain"].(string); ok {
				domain = strings.TrimSpace(v)
			}
			if v, ok := t.Meta.AdditionalFields["x-min-role"].(string); ok {
				minRole = strings.TrimSpace(v)
			}
		}

		out = append(out, DeepseekTool{
			Type: "function",
			Function: DeepseekToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  schema,
			},
			ReadOnly: readOnly,
			Domain:   domain,
			MinRole:  minRole,
		})
	}
	return out, nil
}

func (c *MCPClient) CallTool(ctx context.Context, session *MCPSession, name string, args any) (string, bool, error) {
	if session == nil || session.c == nil {
		return "mcp session not available", true, nil
	}

	session.callMu.Lock()
	defer session.callMu.Unlock()

	session.callSeq++
	seq := session.callSeq
	inicio := time.Now()
	ctxErrAntes := ctx.Err()
	logger.Info("mcp tool call iniciando",
		"tool", name, "seq", seq,
		"ctx_err", errString(ctxErrAntes),
		"session_id", session.id,
	)

	res, err := session.c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	})
	duracao := time.Since(inicio)

	if err != nil {
		// Diagnóstico da queda do transporte: o que importa é separar
		// "o chamador cancelou" de "o transporte MCP morreu sozinho". O ctx
		// carrega a resposta para o primeiro caso; o erro do SDK, para o
		// segundo. Sem esses dois campos lado a lado, os dois cenários ficam
		// indistinguíveis no log.
		logger.Error("mcp tool call falhou",
			"tool", name, "seq", seq,
			"ms", duracao.Milliseconds(),
			"ctx_err_durante", errString(ctx.Err()),
			"session_id", session.id,
			"erro", err.Error(),
		)
		return err.Error(), true, nil
	}

	logger.Info("mcp tool call ok",
		"tool", name, "seq", seq,
		"ms", duracao.Milliseconds(),
		"session_id", session.id,
		isErrorTexto(res.IsError),
	)

	var b strings.Builder
	for _, c := range res.Content {
		switch v := c.(type) {
		case mcp.TextContent:
			b.WriteString(v.Text)
		default:
			blob, _ := json.Marshal(v)
			if len(blob) > 0 {
				b.WriteString(string(blob))
			}
		}
	}
	return b.String(), res.IsError, nil
}
