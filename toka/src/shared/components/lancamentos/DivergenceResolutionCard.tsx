import { AlertTriangle, ArrowRight, FileCheck2 } from "lucide-react";

import { Button } from "../../ui/button";
import {
  computeDivergence,
  divergenceReasonLabel,
  type DivergenceInfo,
} from "../../lib/lancamentos/divergence";
import type { Lancamento } from "../../lib/lancamentos/types";

interface DivergenceResolutionCardProps {
  lancamento: Lancamento;
  actionLabel?: string;
  onAction?: () => void;
}

const formatCurrency = (value: number | undefined) =>
  value === undefined
    ? "—"
    : value.toLocaleString("pt-BR", {
        style: "currency",
        currency: "BRL",
        minimumFractionDigits: 2,
      });

function resolutionCopy(lancamento: Lancamento, info: DivergenceInfo): string {
  if (info.reason === "missing_proof") {
    if (lancamento.kind === "commission") {
      return "Anexe o comprovante do pagamento. Se a comissão for gerenciada por uma ordem de pagamento, faça a correção na ordem vinculada.";
    }
    return "Abra a ordem de pagamento vinculada e anexe o comprovante da liquidação para completar a trilha de auditoria.";
  }

  if (info.reason === "missing_timestamp") {
    return "Confira o histórico do processamento e registre a data efetiva da liquidação. O status final precisa ter uma data auditável.";
  }

  if (lancamento.kind === "charge") {
    return "Compare o total esperado com os pagamentos registrados abaixo. Corrija pela conciliação ou estorne o pagamento incorreto antes de uma nova baixa.";
  }
  if (lancamento.kind === "commission") {
    return "Revise o valor bruto, as deduções e o líquido. Se a comissão já estiver aprovada ou paga, estorne-a antes de recriar a composição correta.";
  }
  if (lancamento.kind === "transfer") {
    return "Revise aluguel bruto, taxa administrativa e deduções. Reprocesse o repasse somente depois que o líquido esperado estiver correto.";
  }
  return "Revise a composição econômica e a origem do lançamento antes de concluir a liquidação.";
}

export function DivergenceResolutionCard({
  lancamento,
  actionLabel,
  onAction,
}: DivergenceResolutionCardProps) {
  const info = computeDivergence(lancamento);
  if (!info) return null;

  return (
    <section className="rounded-xl border border-status-warning/35 bg-status-warning-soft/60 p-4" aria-label="Resolução de divergência">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <div className="rounded-lg bg-background/80 p-2 text-status-warning">
            <AlertTriangle className="h-4 w-4" aria-hidden="true" />
          </div>
          <div className="min-w-0">
            <p className="text-sm font-bold text-foreground">Revisão necessária</p>
            <p className="mt-0.5 text-xs text-muted-foreground">
              Este lançamento não está necessariamente com o valor errado; há uma pendência de conferência.
            </p>
          </div>
        </div>
        <span className="shrink-0 rounded-md border border-status-warning/30 bg-background/70 px-2 py-1 text-xs font-bold text-status-warning">
          {divergenceReasonLabel(info)}
        </span>
      </div>

      {info.reason === "amount_mismatch" ? (
        <div className="mt-4 grid grid-cols-1 gap-2 sm:grid-cols-3">
          <Metric label="Esperado" value={formatCurrency(info.expected)} />
          <Metric label="Registrado" value={formatCurrency(info.received)} />
          <Metric label="Diferença" value={formatCurrency(Math.abs(info.delta ?? 0))} critical />
        </div>
      ) : (
        <div className="mt-4 rounded-lg border border-border/70 bg-background/70 px-3 py-2">
          <p className="text-[11px] font-bold uppercase tracking-wide text-muted-foreground">Valor sob revisão</p>
          <p className="mt-0.5 text-base font-bold tabular-nums text-foreground">{formatCurrency(lancamento.amount)}</p>
        </div>
      )}

      <div className="mt-4 flex items-start gap-2 text-xs text-foreground/80">
        <FileCheck2 className="mt-0.5 h-4 w-4 shrink-0 text-status-warning" aria-hidden="true" />
        <div>
          <p className="font-bold text-foreground">Como resolver</p>
          <p className="mt-0.5 leading-relaxed">{resolutionCopy(lancamento, info)}</p>
        </div>
      </div>

      {actionLabel && onAction && (
        <div className="mt-4 flex justify-end">
          <Button type="button" size="sm" variant="outline" onClick={onAction}>
            {actionLabel}
            <ArrowRight className="h-4 w-4" aria-hidden="true" />
          </Button>
        </div>
      )}
    </section>
  );
}

function Metric({ label, value, critical }: { label: string; value: string; critical?: boolean }) {
  return (
    <div className="rounded-lg border border-border/70 bg-background/70 px-3 py-2">
      <p className="text-[11px] font-bold uppercase tracking-wide text-muted-foreground">{label}</p>
      <p className={critical ? "mt-0.5 font-bold tabular-nums text-status-critical" : "mt-0.5 font-bold tabular-nums text-foreground"}>
        {value}
      </p>
    </div>
  );
}
