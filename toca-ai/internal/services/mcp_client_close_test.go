package services

import (
	"sync/atomic"
	"testing"
)

// A perda do stream SSE tem dois significados possíveis e o SDK reporta os dois
// pelo mesmo hook. O fechamento pedido pelo handler (`defer session.Close()`)
// não pode virar alarme: senão todo fim de request polui o log com
// "mcp stream sse caiu: context canceled" e a queda de verdade se perde.
func TestMCPSessionCloseIsNotTreatedAsStreamLoss(t *testing.T) {
	// Sem marcador (sessão antiga/inalcançável) o padrão é alarmar.
	if !streamLossIsAnAlarm(nil) {
		t.Fatal("sem marcador de fechamento, a perda do stream deve alarmar")
	}

	closing := &atomic.Bool{}
	if !streamLossIsAnAlarm(closing) {
		t.Fatal("antes do Close, a perda do stream deve alarmar")
	}

	session := &MCPSession{closing: closing}
	if err := session.Close(); err != nil {
		t.Fatalf("close de sessão vazia não deve falhar: %v", err)
	}
	if !closing.Load() {
		t.Fatal("Close precisa marcar o fechamento intencional")
	}
	if streamLossIsAnAlarm(closing) {
		t.Fatal("depois do Close, a perda do stream é encerramento — não alarme")
	}
}
