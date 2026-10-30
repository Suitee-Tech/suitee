package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	appCtx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-ai/internal/services"
)

type Suggestion struct {
	Label  string `json:"label"`
	Prompt string `json:"prompt"`
}

type SuggestionsHandler struct {
	deepseek *services.DeepseekClient
	mcp      *services.MCPClient
	cache    *suggestionsCache
}

type suggestionsCache struct {
	mu sync.Mutex
	m  map[string]suggestionsCacheEntry
}

type suggestionsCacheEntry struct {
	expiresAt   time.Time
	suggestions []Suggestion
}

func newSuggestionsCache() *suggestionsCache {
	return &suggestionsCache{m: map[string]suggestionsCacheEntry{}}
}

func NewSuggestionsHandler(deepseek *services.DeepseekClient, mcp *services.MCPClient) *SuggestionsHandler {
	return &SuggestionsHandler{
		deepseek: deepseek,
		mcp:      mcp,
		cache:    newSuggestionsCache(),
	}
}

func (h *SuggestionsHandler) Suggestions(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID, err := appCtx.RequireTenantID(ctx)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	role, _ := appCtx.GetUserRole(ctx)
	permission, _ := appCtx.GetUserPermission(ctx)
	authToken, _ := appCtx.GetAuthToken(ctx)

	label := strings.TrimSpace(c.Query("screen_label"))
	if label == "" {
		label = "Tela atual"
	}
	screenPath := strings.TrimSpace(c.Query("screen_path"))
	cacheKey := tenantID + ":" + strings.ToLower(strings.TrimSpace(role)) + ":" + strings.ToLower(strings.TrimSpace(permission)) + ":" + strings.ToLower(label) + ":" + strings.ToLower(screenPath)

	if suggestions, ok := h.cacheGet(cacheKey); ok {
		response.Success(c, "ok", gin.H{"suggestions": suggestions})
		return
	}

	tools := []services.DeepseekTool{}
	if h.mcp != nil && authToken != "" {
		session, err := h.mcp.Connect(ctx, tenantID, authToken)
		if err == nil {
			defer session.Close()
			if t, err := h.mcp.ListTools(ctx, session); err == nil {
				tools = t
			}
		}
	}

	suggestions, err := h.generateSuggestions(ctx, label, screenPath, role, permission, tools)
	if err != nil || len(suggestions) == 0 {
		suggestions = []Suggestion{
			{Label: "Resumir contexto", Prompt: "Resuma o contexto da tela e sugira os próximos 3 passos."},
			{Label: "Próximos passos", Prompt: "Liste os 3 próximos passos mais prováveis e o porquê."},
			{Label: "Checar pendências", Prompt: "O que eu devo checar agora nesta tela? Seja objetivo."},
		}
	}

	h.cacheSet(cacheKey, suggestions, 2*time.Minute)
	response.Success(c, "ok", gin.H{"suggestions": suggestions})
}

func (h *SuggestionsHandler) cacheGet(key string) ([]Suggestion, bool) {
	h.cache.mu.Lock()
	defer h.cache.mu.Unlock()
	e, ok := h.cache.m[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expiresAt) {
		delete(h.cache.m, key)
		return nil, false
	}
	return e.suggestions, true
}

func (h *SuggestionsHandler) cacheSet(key string, suggestions []Suggestion, ttl time.Duration) {
	h.cache.mu.Lock()
	defer h.cache.mu.Unlock()
	h.cache.m[key] = suggestionsCacheEntry{expiresAt: time.Now().Add(ttl), suggestions: suggestions}
}

func (h *SuggestionsHandler) generateSuggestions(
	ctx context.Context,
	screenLabel string,
	screenPath string,
	role string,
	permission string,
	tools []services.DeepseekTool,
) ([]Suggestion, error) {
	var toolLines []string
	for _, t := range tools {
		name := strings.TrimSpace(t.Function.Name)
		desc := strings.TrimSpace(t.Function.Description)
		if name == "" || desc == "" {
			continue
		}
		toolLines = append(toolLines, "- "+name+": "+desc)
	}
	toolText := "Ferramentas disponíveis: nenhuma"
	if len(toolLines) > 0 {
		toolText = "Ferramentas disponíveis:\n" + strings.Join(toolLines, "\n")
	}

	system := services.DeepseekMessage{
		Role: "system",
		Content: "Você gera sugestões curtas e acionáveis para a Suitee Intelligence.\n" +
			"Retorne somente um JSON válido no formato:\n" +
			"{\"suggestions\":[{\"label\":\"...\",\"prompt\":\"...\"}]}\n" +
			"Regras:\n" +
			"- 3 a 5 sugestões.\n" +
			"- label curto (2-4 palavras).\n" +
			"- prompt em pt-BR, sem emojis, sem cumprimentos.\n",
	}
	user := services.DeepseekMessage{
		Role: "user",
		Content: "Tela: " + screenLabel + "\nPath: " + strings.TrimSpace(screenPath) + "\n" +
			"Contexto de usuário:\n" +
			"- role: " + strings.TrimSpace(role) + "\n" +
			"- permission: " + strings.TrimSpace(permission) + "\n\n" +
			toolText + "\n\n" +
			"Instruções:\n" +
			"- Sugestões devem mapear para ações que as ferramentas disponíveis suportam para esta tela.\n" +
			"- Não invente capacidades.\n" +
			"- Não mencione nomes de ferramentas no texto (ex.: get_*, search_*, update_*). Escreva a intenção do usuário.\n" +
			"- Evite verbos genéricos como \"buscar\" no label. Prefira verbos como: Liste, Mostre, Resuma, Compare, Filtre.\n" +
			"- Cada prompt deve ser uma instrução direta e específica (ex.: listar/filtrar/analisar/comparar/atualizar).\n",
	}

	msg, _, err := h.deepseek.ChatNoTools(ctx, []services.DeepseekMessage{system, user}, 220)
	if err != nil {
		return nil, err
	}

	raw := strings.TrimSpace(msg.Content)
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var parsed struct {
		Suggestions []Suggestion `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		start := strings.Index(raw, "{")
		end := strings.LastIndex(raw, "}")
		if start >= 0 && end > start {
			_ = json.Unmarshal([]byte(raw[start:end+1]), &parsed)
		}
	}

	out := make([]Suggestion, 0, len(parsed.Suggestions))
	for _, s := range parsed.Suggestions {
		if strings.TrimSpace(s.Label) == "" || strings.TrimSpace(s.Prompt) == "" {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}
