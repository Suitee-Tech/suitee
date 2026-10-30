package service

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/oginoino/toca/toca-signature/internal/domain/envelope"
)

// DefaultReconcileSweepInterval é o intervalo padrão da varredura.
//
// Cinco minutos é curto o bastante para o operador não ficar esperando um
// contrato que já foi assinado, e longo o bastante para não martelar a API do
// provedor (a varredura só toca envelopes sem atualização há mais tempo que
// `staleAfter`, então na prática quase sempre não encontra nada).
const DefaultReconcileSweepInterval = 5 * time.Minute

// staleAfter é a idade mínima da última atualização para um envelope entrar na
// varredura. Evita competir com o caminho normal: um envelope recém-enviado
// ainda vai receber seus webhooks.
const staleAfter = 10 * time.Minute

// ReconcileSweeper sincroniza com o provedor os envelopes que ficaram sem
// atualização.
//
// Existe porque depender só de webhook não é suficiente. A entrega do provedor
// pode atrasar ou se perder — em produção `signature.webhook_events` estava
// vazio e um envelope já finalizado seguia como RUNNING do nosso lado, deixando
// o contrato preso em PENDING_SIGNATURE sem ninguém perceber. Até então a única
// forma de sincronizar era o operador clicar "Sincronizar" na tela, envelope por
// envelope.
//
// A varredura reusa o MESMO Reconcile do botão: não há uma segunda noção de
// sincronização para divergir da primeira.
type ReconcileSweeper struct {
	svc      *EnvelopeService
	repo     envelope.Repository
	interval time.Duration
	batch    int
}

// NewReconcileSweeper monta a varredura. `interval` <= 0 usa o padrão.
func NewReconcileSweeper(svc *EnvelopeService, repo envelope.Repository, interval time.Duration) *ReconcileSweeper {
	if interval <= 0 {
		interval = ReconcileSweepIntervalFromEnv()
	}
	return &ReconcileSweeper{svc: svc, repo: repo, interval: interval, batch: 50}
}

// ReconcileSweepIntervalFromEnv lê o intervalo de
// SIGNATURE_RECONCILE_INTERVAL_MINUTES. Valor ausente, inválido ou <= 0 usa o
// padrão; a varredura pode ser desligada com 0 explícito em
// SIGNATURE_RECONCILE_ENABLED=false.
func ReconcileSweepIntervalFromEnv() time.Duration {
	raw := os.Getenv("SIGNATURE_RECONCILE_INTERVAL_MINUTES")
	if raw == "" {
		return DefaultReconcileSweepInterval
	}
	minutes, err := strconv.Atoi(raw)
	if err != nil || minutes <= 0 {
		return DefaultReconcileSweepInterval
	}
	return time.Duration(minutes) * time.Minute
}

// Run executa uma varredura e devolve quantos envelopes foram sincronizados.
//
// Exportado para permitir teste e uma execução sob demanda.
func (w *ReconcileSweeper) Run(ctx context.Context) int {
	stale, err := w.repo.ListStaleActive(ctx, time.Now().Add(-staleAfter), w.batch)
	if err != nil {
		log.Printf("signature reconcile sweep: list stale envelopes: %v", err)
		return 0
	}

	synced := 0
	for _, env := range stale {
		if ctx.Err() != nil {
			return synced
		}
		// Um envelope que falha não pode interromper os outros: o provedor
		// costuma falhar por instabilidade momentânea, e os demais seguem
		// precisando de sincronização.
		// Escopo de serviço: a varredura roda sem JWT, então leitura e escrita
		// precisam ir pelo pool service-role (a RLS esconderia a linha).
		if _, err := w.svc.ReconcileServiceScope(ctx, env.TenantID, env.ID); err != nil {
			log.Printf("signature reconcile sweep: envelope %s (%s): %v", env.ID, env.ExternalEnvelopeID, err)
			continue
		}
		synced++
	}
	if synced > 0 {
		log.Printf("signature reconcile sweep: %d envelope(s) sincronizado(s) de %d", synced, len(stale))
	}
	return synced
}

// Start roda a varredura periodicamente até o contexto ser cancelado.
func (w *ReconcileSweeper) Start(ctx context.Context) {
	if os.Getenv("SIGNATURE_RECONCILE_ENABLED") == "false" {
		log.Println("signature reconcile sweep: desabilitado por SIGNATURE_RECONCILE_ENABLED=false")
		return
	}
	log.Printf("signature reconcile sweep: a cada %s", w.interval)

	// Primeira passada depois de um atraso curto: dá tempo de o serviço terminar
	// de subir antes de falar com o provedor.
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			w.Run(ctx)
			timer.Reset(w.interval)
		}
	}
}
