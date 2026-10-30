import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, RefreshCw, ShieldAlert } from "lucide-react";

import { Button } from "../../ui/button";
import {
  financialReportService,
  type ReconciliationCategory,
  type ReconciliationFinding,
} from "../../services/financialReport.service";

/**
 * Limite por verificação no backend (`LIMIT 100` em reconciliation_postgres).
 * O payload não sinaliza truncamento, então um contador exatamente neste valor
 * é o único indício de que pode haver mais achados do que os listados.
 */
const PER_CHECK_LIMIT = 100;

const CATEGORY_LABEL: Record<ReconciliationCategory, string> = {
  duplicate: "Duplicatas",
  orphan: "Órfãos",
  excess: "Excesso de alocação",
  suballoc: "Subalocação",
  consistency: "Consistência",
};

const ENTITY_LABEL: Record<string, string> = {
  Commission: "Comissão",
  Transaction: "Lançamento",
  Charge: "Cobrança",
  PayoutOrder: "Ordem de pagamento",
  Contract: "Contrato",
};

function formatDateTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString("pt-BR", {
      day: "2-digit",
      month: "2-digit",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
}

function FindingRow({ finding }: { finding: ReconciliationFinding }) {
  const isError = finding.severity === "ERROR";
  return (
    <li className="border-t border-border px-4 py-3 first:border-t-0">
      <div className="flex items-start gap-3">
        <span
          className={
            isError
              ? "mt-0.5 inline-flex shrink-0 items-center gap-1 rounded-full border border-status-critical/30 bg-status-critical-soft px-2 py-0.5 text-micro font-bold uppercase text-status-critical"
              : "mt-0.5 inline-flex shrink-0 items-center gap-1 rounded-full border border-status-warning/30 bg-status-warning-soft px-2 py-0.5 text-micro font-bold uppercase text-status-warning"
          }
        >
          {isError ? <ShieldAlert className="h-3 w-3" aria-hidden="true" /> : <AlertTriangle className="h-3 w-3" aria-hidden="true" />}
          {isError ? "Erro" : "Atenção"}
        </span>
        <div className="min-w-0 flex-1">
          <p className="text-sm text-foreground">{finding.message}</p>
          <p className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span className="font-medium">{CATEGORY_LABEL[finding.category] ?? finding.category}</span>
            <span>{ENTITY_LABEL[finding.entity_type] ?? finding.entity_type}</span>
            {/* O identificador é exibido como referência, sem atalho: em
                duplicatas ele é o source_event_id compartilhado, não o id da
                linha, então não há destino único para onde navegar. */}
            {finding.entity_id && <span className="font-mono break-all">{finding.entity_id}</span>}
            {finding.reference && <span className="break-all">{finding.reference}</span>}
          </p>
        </div>
      </div>
    </li>
  );
}

/**
 * Integridade técnica do fluxo financeiro (FIN-03).
 *
 * O endpoint existia no backend sem nenhum consumidor: a Conferência mostrava
 * as movimentações, mas as verificações de duplicata, órfão e alocação não
 * chegavam ao operador. Este painel é somente leitura — o relatório é dry-run e
 * não altera registros.
 */
export function ReconciliationReportSection() {
  const query = useQuery({
    queryKey: ["reconciliation-report"],
    queryFn: () => financialReportService.reconciliation(),
    staleTime: 0,
    retry: false,
  });

  // Falha do diagnóstico NÃO pode parecer "sem pendências": o backend é
  // fail-closed, então um erro significa que a verificação não rodou.
  if (query.isError) {
    return (
      <section className="rounded-2xl border border-status-critical/30 bg-card shadow-sm">
        <header className="flex items-center gap-2 border-b border-border px-4 py-3">
          <ShieldAlert className="h-5 w-5 text-status-critical" aria-hidden="true" />
          <h2 className="text-base font-bold text-foreground">Integridade técnica</h2>
        </header>
        <div role="alert" className="space-y-3 px-4 py-4">
          <p className="text-sm text-status-critical">
            Não foi possível executar a verificação de integridade. Isso não significa que não há pendências — a
            verificação não rodou.
          </p>
          <Button variant="outline" size="sm" onClick={() => void query.refetch()}>
            <RefreshCw className="h-4 w-4" aria-hidden="true" /> Tentar novamente
          </Button>
        </div>
      </section>
    );
  }

  if (query.isLoading) {
    return (
      <section className="rounded-2xl border border-border bg-card shadow-sm">
        <header className="flex items-center gap-2 border-b border-border px-4 py-3">
          <ShieldAlert className="h-5 w-5 text-muted-foreground" aria-hidden="true" />
          <h2 className="text-base font-bold text-foreground">Integridade técnica</h2>
        </header>
        <p role="status" className="px-4 py-4 text-sm text-muted-foreground">
          Verificando duplicatas, órfãos e alocação…
        </p>
      </section>
    );
  }

  const report = query.data;
  const findings = report?.findings ?? [];
  const total = report?.summary.total_findings ?? 0;

  return (
    <section className="rounded-2xl border border-border bg-card shadow-sm">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-4 py-3">
        <div className="flex items-center gap-2">
          <ShieldAlert className="h-5 w-5 text-muted-foreground" aria-hidden="true" />
          <h2 className="text-base font-bold text-foreground">Integridade técnica</h2>
        </div>
        <div className="flex items-center gap-3">
          {report?.generated_at && (
            <span className="text-xs text-muted-foreground">Verificado em {formatDateTime(report.generated_at)}</span>
          )}
          <Button
            variant="outline"
            size="sm"
            onClick={() => void query.refetch()}
            loading={query.isFetching}
            loadingText="Verificando…"
          >
            <RefreshCw className="h-4 w-4" aria-hidden="true" /> Verificar
          </Button>
        </div>
      </header>

      {total === 0 ? (
        <p className="flex items-center gap-2 px-4 py-4 text-sm text-status-ok">
          <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
          Nenhuma inconsistência encontrada nas verificações de integridade.
        </p>
      ) : (
        <>
          <div className="flex flex-wrap gap-x-6 gap-y-2 px-4 py-3 text-sm">
            {/* Total do relatório, não soma das categorias: uma categoria
                desconhecida não entra em bucket nenhum no backend. */}
            <span className="font-bold text-foreground">{total} achado(s)</span>
            {report && report.summary.errors > 0 && (
              <span className="text-status-critical">{report.summary.errors} erro(s)</span>
            )}
            {report && report.summary.warnings > 0 && (
              <span className="text-status-warning">{report.summary.warnings} atenção</span>
            )}
          </div>

          {findings.length >= PER_CHECK_LIMIT && (
            <p className="border-t border-border bg-muted/40 px-4 py-2 text-xs text-muted-foreground">
              A listagem pode estar incompleta: uma verificação atingiu o limite de {PER_CHECK_LIMIT} achados e o
              relatório não sinaliza truncamento. Priorize os erros e verifique novamente após corrigi-los.
            </p>
          )}

          <ul className="max-h-96 overflow-y-auto">
            {findings.map((finding, index) => (
              <FindingRow key={`${finding.category}-${finding.entity_id}-${index}`} finding={finding} />
            ))}
          </ul>

          <p className="border-t border-border px-4 py-2 text-xs text-muted-foreground">
            O relatório é somente leitura e não altera registros. Cada achado traz o contexto econômico para conferência
            manual.
          </p>
        </>
      )}
    </section>
  );
}
