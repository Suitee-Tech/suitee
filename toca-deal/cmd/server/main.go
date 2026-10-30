package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/oginoino/toca/pkg/tracing"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/health"
	pkgmw "github.com/oginoino/toca/pkg/middleware"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/pkg/timeutil"
	"github.com/oginoino/toca/toca-deal/internal/api/v1/routes"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// reconciliationInterval reads RECONCILIATION_INTERVAL_MINUTES from env and
// falls back to 6h. Returns 0 when the env value is "0" so operators can
// disable the loop entirely (useful in CI / tests).
func reconciliationInterval() time.Duration {
	raw := os.Getenv("RECONCILIATION_INTERVAL_MINUTES")
	if raw == "" {
		return 6 * time.Hour
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 6 * time.Hour
	}
	if n == 0 {
		return 0
	}
	return time.Duration(n) * time.Minute
}

// autoReadjustInterval mirrors reconciliationInterval but for the rental
// auto-readjust sweep. Default 24h. AUTO_READJUST_INTERVAL_HOURS=0 disables.
func autoReadjustInterval() time.Duration {
	raw := os.Getenv("AUTO_READJUST_INTERVAL_HOURS")
	if raw == "" {
		return 24 * time.Hour
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 24 * time.Hour
	}
	if n == 0 {
		return 0
	}
	return time.Duration(n) * time.Hour
}

// proposalFollowUpInterval reads PROPOSAL_FOLLOWUP_INTERVAL_MINUTES and falls
// back to 1h. Returns 0 when the value is "0" so ops can disable the sweep
// without touching the cadence (§F6).
func proposalFollowUpInterval() time.Duration {
	raw := os.Getenv("PROPOSAL_FOLLOWUP_INTERVAL_MINUTES")
	if raw == "" {
		return time.Hour
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return time.Hour
	}
	if n == 0 {
		return 0
	}
	return time.Duration(n) * time.Minute
}

// proposalFollowUpSchedule lê a cadência da régua de follow-up de proposta.
// PROPOSAL_FOLLOWUP_MAX_STAGES=0 desliga a régua por completo.
func proposalFollowUpSchedule() deal.ProposalFollowUpSchedule {
	return deal.ProposalFollowUpSchedule{
		Stage1Days: config.GetEnvInt("PROPOSAL_FOLLOWUP_STAGE1_DAYS", deal.DefaultProposalFollowUpStage1Days),
		GapDays:    config.GetEnvInt("PROPOSAL_FOLLOWUP_GAP_DAYS", deal.DefaultProposalFollowUpGapDays),
		MaxStages:  config.GetEnvInt("PROPOSAL_FOLLOWUP_MAX_STAGES", deal.DefaultProposalFollowUpMaxStages),
		MaxAgeDays: config.GetEnvInt("PROPOSAL_FOLLOWUP_MAX_AGE_DAYS", deal.DefaultProposalFollowUpMaxAgeDays),
	}.Norm()
}

// autoReadjustDefaultIndex reads AUTO_READJUST_DEFAULT_INDEX_PCT (string with
// a percentage, e.g. "4.5"). Returns 0 when unset/invalid — the loop skips
// per-contract readjustments in that case (logged), so missing config is a
// safe no-op.
func autoReadjustDefaultIndex() float64 {
	raw := os.Getenv("AUTO_READJUST_DEFAULT_INDEX_PCT")
	if raw == "" {
		return 0
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f < 0 {
		return 0
	}
	return f
}

// @title           Toca Deal API
// @version         1.0
// @description     Deal service for Toca ecosystem
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8084
// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

func main() {

	// Initialize OpenTelemetry
	shutdown, err := tracing.InitProvider("toca-deal")
	if err == nil {
		defer func() { _ = shutdown(context.Background()) }()
	} else {
		log.Printf("Failed to initialize OpenTelemetry: %v", err)
	}
	// Load configuration
	cfg := config.LoadBaseConfig()

	pgDB, err := database.ConnectPostgres(context.Background(), cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	defer pgDB.Close()
	log.Printf("Connected to Postgres database: %s", cfg.Database.Name)

	// Wire the audit recorder once at boot — every audit.Record call in
	// services lands in audit.events via the service pool. Nil-safe if
	// pgDB has no service pool configured (dev fallback).
	audit.SetDefault(audit.NewRecorder(pgDB))

	// Setup EventBus
	var eventBus events.EventBus
	var rb *events.RabbitMQEventBus
	if cfg.RabbitMQ.URI != "" {
		rb, err = events.NewRabbitMQEventBusPG(cfg.RabbitMQ.URI, pgDB)
		if err != nil {
			log.Printf("Failed to connect to RabbitMQ: %v. Using InMemory fallback.", err)
			eventBus = events.NewInMemoryEventBus()
		} else {
			log.Println("Connected to RabbitMQ")
			defer rb.Close()
			eventBus = rb
		}
	} else {
		log.Println("RabbitMQ URI not configured. Using InMemory EventBus.")
		eventBus = events.NewInMemoryEventBus()
	}

	// Setup Storage (local for dev, can swap with S3/GCS in production)
	storageService, err := storage.NewFromEnv()
	if err != nil {
		log.Printf("Warning: Failed to initialize storage: %v. File uploads will be unavailable.", err)
	}

	// Setup Router
	r, services := routes.SetupRouter(cfg, pgDB, eventBus, storageService)
	// RequestID FIRST so trace_id/span_id/request_id are stamped before
	// tracing or any other middleware runs and so every downstream log
	// line carries the same id even when the handler returns early.
	r.Use(pkgmw.RequestID())
	r.Use(tracing.Middleware("toca-deal"))
	// Hard ceiling on per-request processing time. Defends against the
	// downstream "everything hangs at 20s" symptom — if a single handler
	// (or a transitive DB query / external client) stalls past this
	// deadline, the context cancels, the in-flight query is aborted and
	// the request returns 504. Keep BELOW the axios client timeout (20s)
	// so the client sees an explicit 504 before its own timeout fires.
	r.Use(pkgmw.RequestTimeout(18 * time.Second))

	// Subscribe to signature events from toca-signature so the contract is
	// updated as signers complete (or refuse) the Clicksign flow. The
	// envelope_finished handler is what re-emits the existing
	// ContractSignedEvent that toca-financial consumes to create the Rental
	// aggregate — keeping the downstream pipeline ignorant of who signed.
	if rb != nil {
		signerSignedHandler := service.NewSignerSignedHandler(services.Contract)
		if err := rb.SubscribeWithQueue("toca-deal.signature.signer_signed", "signature.signer_signed", signerSignedHandler); err != nil {
			log.Printf("Failed to subscribe to signature.signer_signed: %v", err)
		} else {
			log.Println("Subscribed to signature.signer_signed")
		}

		envelopeFinishedHandler := service.NewEnvelopeFinishedHandler(services.Contract)
		if err := rb.SubscribeWithQueue("toca-deal.signature.envelope_finished", "signature.envelope_finished", envelopeFinishedHandler); err != nil {
			log.Printf("Failed to subscribe to signature.envelope_finished: %v", err)
		} else {
			log.Println("Subscribed to signature.envelope_finished")
		}

		// Second consumer on signature.envelope_finished — dedicated to
		// downloading the signed PDF from Clicksign and persisting it in
		// MinIO via DocumentService. Lives on its own queue so a transient
		// storage outage retries independently from the contract status flip.
		if services.Document != nil {
			persistSignedPDF := service.NewSignedPDFPersistHandler(services.Document, services.Contract)
			if err := rb.SubscribeWithQueue("toca-deal.signature.envelope_finished.persist_signed_pdf", "signature.envelope_finished", persistSignedPDF); err != nil {
				log.Printf("Failed to subscribe to signature.envelope_finished[persist_signed_pdf]: %v", err)
			} else {
				log.Println("Subscribed to signature.envelope_finished[persist_signed_pdf]")
			}
		}

		// Extração OCR+IA: ao receber document.uploaded (documentos de dados
		// enviados no fluxo de upload), roda docling + DeepSeek e auto-aplica os
		// campos de alta confiança na party. Fila própria + DLQ → retry isolado.
		if services.Document != nil && services.Deal != nil {
			extractionHandler := service.NewDocumentExtractionHandler(services.Document, services.Deal, clients.NewAIClient())
			if err := rb.SubscribeWithQueue("toca-deal.document.uploaded.extract", "document.uploaded", extractionHandler); err != nil {
				log.Printf("Failed to subscribe to document.uploaded[extract]: %v", err)
			} else {
				log.Println("Subscribed to document.uploaded[extract]")
			}
		}

		refusedHandler := service.NewSignatureRefusedHandler(services.Contract)
		if err := rb.SubscribeWithQueue("toca-deal.signature.refused", "signature.refused", refusedHandler); err != nil {
			log.Printf("Failed to subscribe to signature.refused: %v", err)
		} else {
			log.Println("Subscribed to signature.refused")
		}

		expiredHandler := service.NewSignatureExpiredHandler(services.Contract)
		if err := rb.SubscribeWithQueue("toca-deal.signature.expired", "signature.expired", expiredHandler); err != nil {
			log.Printf("Failed to subscribe to signature.expired: %v", err)
		} else {
			log.Println("Subscribed to signature.expired")
		}

		envelopeStartedHandler := service.NewEnvelopeStartedHandler(services.Contract)
		if err := rb.SubscribeWithQueue("toca-deal.signature.envelope_started", "signature.envelope_started", envelopeStartedHandler); err != nil {
			log.Printf("Failed to subscribe to signature.envelope_started: %v", err)
		} else {
			log.Println("Subscribed to signature.envelope_started")
		}

		signerViewedHandler := service.NewSignerViewedHandler(services.Contract)
		if err := rb.SubscribeWithQueue("toca-deal.signature.signer_viewed", "signature.signer_viewed", signerViewedHandler); err != nil {
			log.Printf("Failed to subscribe to signature.signer_viewed: %v", err)
		} else {
			log.Println("Subscribed to signature.signer_viewed")
		}

		// Projection refresh subscribers — keep denormalized snapshots in
		// toca-deal in sync with the canonical source services. See
		// docs/architecture/data-consistency-audit.md §8.6 + §8.13. The
		// publishers themselves live in toca-core and toca-crm (Onda 2-3).
		if services.ContractRepo != nil {
			propertyUpdated := service.NewPropertyUpdatedHandler(services.ContractRepo)
			if err := rb.SubscribeWithQueue("toca-deal.property.updated.refresh_title", "property.updated", propertyUpdated); err != nil {
				log.Printf("Failed to subscribe to property.updated: %v", err)
			} else {
				log.Println("Subscribed to property.updated (refresh contract title)")
			}
		}

		if services.DealRepo != nil {
			leadUpdated := service.NewLeadUpdatedHandler(services.DealRepo)
			if err := rb.SubscribeWithQueue("toca-deal.lead.updated.refresh_parties", "lead.updated", leadUpdated); err != nil {
				log.Printf("Failed to subscribe to lead.updated: %v", err)
			} else {
				log.Println("Subscribed to lead.updated (refresh party labels)")
			}
		}

		// FASE 2 (integração total) — toca-ops publishes inspection.completed
		// when a vistoria reaches Concluída with a laudo. ENTRY pins
		// EntryInspectionID on the contract; EXIT only audits (the actual
		// TerminateContract still goes through HasCompletedExitInspection).
		if services.Contract != nil {
			inspectionCompletedHandler := service.NewInspectionCompletedHandler(services.Contract)
			if err := rb.SubscribeWithQueue("toca-deal.inspection.completed", "inspection.completed", inspectionCompletedHandler); err != nil {
				log.Printf("Failed to subscribe to inspection.completed: %v", err)
			} else {
				log.Println("Subscribed to inspection.completed (toca-deal)")
			}
		}

		// FASE 3 (integração total) — Document materialization from cross-
		// service events. inspection.completed → Laudo Document.
		// maintenance.completed → Comprovante Document (per attachment).
		if services.Document != nil {
			laudoHandler := service.NewInspectionCompletedDocumentHandler(services.Document)
			if err := rb.SubscribeWithQueue("toca-deal.inspection.completed.laudo_document", "inspection.completed", laudoHandler); err != nil {
				log.Printf("Failed to subscribe to inspection.completed[laudo_document]: %v", err)
			} else {
				log.Println("Subscribed to inspection.completed[laudo_document]")
			}
			// MaintenanceTicketLookup intentionally nil for now — the
			// attachment fetch lands when the toca-ops internal client is
			// wired in a follow-up. The handler no-ops cleanly with a nil
			// lookup so subscribing now is safe.
			comprovanteHandler := service.NewMaintenanceCompletedDocumentHandler(services.Document, nil)
			if err := rb.SubscribeWithQueue("toca-deal.maintenance.completed.comprovante_document", "maintenance.completed", comprovanteHandler); err != nil {
				log.Printf("Failed to subscribe to maintenance.completed[comprovante_document]: %v", err)
			} else {
				log.Println("Subscribed to maintenance.completed[comprovante_document]")
			}

			// financial.proof_uploaded → Comprovante Document (origem financeiro).
			// Com downloader (storage implementa DownloadService em Supabase/S3),
			// os bytes do comprovante são copiados para o bucket de documentos;
			// sem ele, o handler materializa apenas o metadata.
			var proofDownloader service.FinancialProofDownloader
			if ds, ok := storageService.(storage.DownloadService); ok {
				proofDownloader = ds
			}
			financialProofHandler := service.NewFinancialProofDocumentHandler(services.Document, proofDownloader)
			if err := rb.SubscribeWithQueue("toca-deal.financial.proof_uploaded.comprovante_document", "financial.proof_uploaded", financialProofHandler); err != nil {
				log.Printf("Failed to subscribe to financial.proof_uploaded[comprovante_document]: %v", err)
			} else {
				log.Println("Subscribed to financial.proof_uploaded[comprovante_document]")
			}
		}

		// FASE 4 (integração total) — audit collection escalations
		// (NEGOTIATED → GUARANTEE_ACTIVATED → JUDICIAL). The handler does
		// NOT mutate Contract state; live collection status is owned by
		// toca-financial.
		collectionAudit := service.NewCollectionEscalatedHandler()
		if err := rb.SubscribeWithQueue("toca-deal.collection.escalated", "collection.escalated", collectionAudit); err != nil {
			log.Printf("Failed to subscribe to collection.escalated: %v", err)
		} else {
			log.Println("Subscribed to collection.escalated (toca-deal)")
		}

		// F0.4 do plano de inadimplência: o caso aberto também entra na trilha
		// do contrato. O evento existia sem consumidor (o contrato vivia dentro
		// do toca-financial, em pacote `internal/`, então ninguém podia assinar).
		collectionCreated := service.NewCollectionCreatedHandler()
		if err := rb.SubscribeWithQueue("toca-deal.collection.created", "collection.created", collectionCreated); err != nil {
			log.Printf("Failed to subscribe to collection.created: %v", err)
		} else {
			log.Println("Subscribed to collection.created (toca-deal)")
		}

		// Fase 2 da inadimplência: o ciclo do acordo e a baixa por garantia /
		// desfecho judicial entram na trilha do contrato. Sem estes assinantes os
		// eventos existiriam publicados para ninguém (o mesmo defeito que o
		// diagnóstico apontou em charge.overdue e collection.created).
		for _, assinatura := range []struct {
			fila    string
			evento  string
			acao    string
			handler func(string) events.EventHandler
		}{
			{"toca-deal.collection.agreement_created", "collection.agreement_created", "collection.agreement_created", service.NewCollectionAgreementHandler},
			{"toca-deal.collection.agreement_broken", "collection.agreement_broken", "collection.agreement_broken", service.NewCollectionAgreementHandler},
			{"toca-deal.collection.agreement_cancelled", "collection.agreement_cancelled", "collection.agreement_cancelled", service.NewCollectionAgreementHandler},
			{"toca-deal.collection.agreement_fulfilled", "collection.agreement_fulfilled", "collection.agreement_fulfilled", service.NewCollectionAgreementHandler},
			{"toca-deal.collection.guarantee_settled", "collection.guarantee_settled", "collection.guarantee_settled", service.NewCollectionSettledHandler},
			{"toca-deal.collection.judicial_closed", "collection.judicial_closed", "collection.judicial_closed", service.NewCollectionSettledHandler},
		} {
			if err := rb.SubscribeWithQueue(assinatura.fila, assinatura.evento, assinatura.handler(assinatura.acao)); err != nil {
				log.Printf("Failed to subscribe to %s: %v", assinatura.evento, err)
			} else {
				log.Printf("Subscribed to %s (toca-deal)", assinatura.evento)
			}
		}

		rb.StartDLQConsumers()
	}

	// Background loops share a single context so graceful shutdown stops both
	// the reconciliation sweep and the rental auto-readjust loop at once.
	reconcileCtx, cancelReconcile := context.WithCancel(context.Background())
	defer cancelReconcile()

	// Rental auto-readjust scheduler — daily sweep across tenants applying
	// the configured index to contracts whose NextAdjustment is due and
	// AutoAdjust=true. Disabled when AUTO_READJUST_INTERVAL_HOURS=0. The
	// per-contract readjustment publishes contract.readjusted, which
	// toca-financial consumes to update RentalChargeConfig.MonthlyAmount.
	if services.Contract != nil {
		if interval := autoReadjustInterval(); interval > 0 {
			defaultIdx := autoReadjustDefaultIndex()
			services.Contract.RunAutoReadjustLoop(reconcileCtx, interval, defaultIdx)
			if defaultIdx > 0 {
				log.Printf("Auto-readjust loop scheduled every %s (default index %.2f%%)", interval, defaultIdx)
			} else {
				log.Printf("Auto-readjust loop scheduled every %s (AUTO_READJUST_DEFAULT_INDEX_PCT unset — sweeps will no-op until set)", interval)
			}
		} else {
			log.Println("Auto-readjust loop disabled (AUTO_READJUST_INTERVAL_HOURS=0)")
		}
	}

	if services.Contract != nil {
		reconciliationService := service.NewReconciliationService(pgDB, services.Contract)
		// Admin endpoint: POST /admin/deals/:id/reconcile-signers forces an
		// immediate sync regardless of the 24h age guard. Recovery hatch
		// for support — frontend operators can hit this when a freshly
		// drifted deal blocks the "Concluído" gate without waiting for
		// the periodic sweep. Requires X-Internal-Key.
		service.RegisterDealReconcileAdminRoutes(r, reconciliationService)
		interval := reconciliationInterval()
		if interval > 0 {
			reconciliationService.RunReconciliationLoop(reconcileCtx, interval, 24, 30)
			log.Printf("Reconciliation loop scheduled every %s", interval)
		} else {
			log.Printf("Reconciliation loop disabled (RECONCILIATION_INTERVAL_MINUTES=0)")
		}
	}

	// Régua de follow-up de proposta (§F6): varre negócios com proposta
	// aguardando resposta e enfileira UM lembrete por grupo (proposta +
	// estágio). Cada varredura reproduz o mesmo event_id enquanto o estágio
	// não avança, então o dedup do toca-comm garante um único e-mail.
	// Desligada com PROPOSAL_FOLLOWUP_INTERVAL_MINUTES=0 ou
	// PROPOSAL_FOLLOWUP_MAX_STAGES=0.
	if repo, ok := services.DealRepo.(service.ProposalFollowUpRepository); ok && eventBus != nil {
		sched := proposalFollowUpSchedule()
		if engine, err := service.NewProposalFollowUpEngine(repo, eventBus, pgDB, sched); err != nil {
			log.Printf("Proposal follow-up engine not started: %v", err)
		} else if interval := proposalFollowUpInterval(); interval > 0 && engine.Enabled() {
			engine.RunLoop(reconcileCtx, interval)
			log.Printf("Proposal follow-up loop scheduled every %s (stage1=%dd gap=%dd max=%d maxAge=%dd)",
				interval, sched.Stage1Days, sched.GapDays, sched.MaxStages, sched.MaxAgeDays)
		} else {
			log.Println("Proposal follow-up loop disabled (PROPOSAL_FOLLOWUP_INTERVAL_MINUTES=0 or PROPOSAL_FOLLOWUP_MAX_STAGES=0)")
		}
	} else {
		log.Println("Proposal follow-up loop not wired (repository does not implement the cross-tenant scan)")
	}

	readiness := health.NewReadinessChecker(health.NewPostgresChecker(pgDB.DB.DB))
	if rb != nil {
		readiness.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterGin(r, "toca-deal", readiness)
	// /health/deep performs actual write+read on Postgres so operators can
	// distinguish "pool answers TCP ping" from "pool can serve queries".
	deep := health.NewReadinessChecker(health.NewPostgresDeepChecker(pgDB.DB.DB))
	if rb != nil {
		deep.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterDeepGin(r, "toca-deal", deep)

	// Start Server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		log.Printf("Starting Deal service on port %s", cfg.Port)

		// Fuso configurado pela imobiliária (Configurações → "Fuso horário") manda
		// nos dias civis dos fluxos; sem configuração/core, cai no padrão do
		// ambiente (America/Sao_Paulo).
		timeutil.SetTenantResolver(clients.NewCoreClient().TenantLocationResolver())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}
