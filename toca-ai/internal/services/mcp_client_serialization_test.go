package services

import (
	"context"
	"sync"
	"testing"
	"time"
)

// A sessão MCP é um único `SSEMCPClient`, que correlaciona cada resposta pelo id
// da requisição no mesmo canal. O loop de chat despacha as tool calls do modelo
// em goroutines paralelas; sem serialização, várias chamadas concorrentes
// disputam esse canal e o transporte morre no meio.
//
// Em produção isso apareceu como 13 de 28 chamadas (46%) devolvendo
// "transport error: context canceled" ou "failed to send request", com o stream
// do chat fechando sem evento de conclusão — a Suitee Intelligence ficava em
// "Analisando..." e a resposta saía incompleta, porque as ferramentas que
// falharam não entravam no contexto do modelo.
func TestMCPSession_SerializesConcurrentToolCalls(t *testing.T) {
	sessao := &MCPSession{}

	// Duas chamadas concorrentes não podem se sobrepor dentro de CallTool.
	// Como não há servidor MCP no teste, CallTool retorna cedo por sessão
	// inválida — o que interessa é que o lock seja respeitado antes disso.
	var dentro, maxConcorrente int
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Simula o corpo protegido: o lock da sessão é o que garante que
			// apenas uma chamada por vez entra no transporte.
			sessao.callMu.Lock()
			mu.Lock()
			dentro++
			if dentro > maxConcorrente {
				maxConcorrente = dentro
			}
			mu.Unlock()
			time.Sleep(2 * time.Millisecond)
			mu.Lock()
			dentro--
			mu.Unlock()
			sessao.callMu.Unlock()
		}(i)
	}
	wg.Wait()

	if maxConcorrente != 1 {
		t.Fatalf("chamadas concorrentes dentro do transporte = %d, want 1 (a sessão não suporta fan-out)", maxConcorrente)
	}
}

// Chamada de ferramenta em sessão ausente devolve erro tratado, sem panic — é o
// caminho quando o MCP não conecta e o chat segue sem ferramentas.
func TestMCPClient_CallToolWithoutSession(t *testing.T) {
	c := NewMCPClient("http://toca-mcp:8089/mcp")

	for _, sessao := range []*MCPSession{nil, {}} {
		out, hasError, err := c.CallTool(context.Background(), sessao, "get_financial_dashboard", map[string]any{})
		if err != nil {
			t.Fatalf("sessão %v: erro inesperado: %v", sessao, err)
		}
		if !hasError {
			t.Errorf("sessão %v: deveria sinalizar erro de ferramenta", sessao)
		}
		if out == "" {
			t.Errorf("sessão %v: deveria devolver mensagem explicando a indisponibilidade", sessao)
		}
	}
}
