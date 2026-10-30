import { useEffect, useMemo, useState } from "react";
import { AlertTriangle, Loader2, TrendingUp } from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";

import type { Contract } from "../contracts/types";
import { contractService, type ReadjustContractRequest } from "../services/contract.service";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogTitle,
} from "../ui/alert-dialog";
import { Button } from "../ui/button";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "../ui/sheet";

/**
 * Painel lateral de reajuste do contrato.
 *
 * Substitui o modal que aplicava o reajuste direto no clique: é uma alteração de
 * valor com efeito financeiro em cobranças já emitidas, e não havia nenhuma
 * etapa de confirmação — o operador só descobria o efeito depois de aplicar.
 * Aqui o formulário fica no painel (padrão do app) e o AlertDialog é usado
 * apenas para confirmar a decisão.
 */
interface ReadjustContractPanelProps {
  open: boolean;
  onClose: () => void;
  contract: Contract;
  onCompleted?: (updated: Contract) => void;
}

const formatCurrency = (value: number) =>
  new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(value || 0);

const toDateInput = (date: Date) => date.toISOString().slice(0, 10);

export function ReadjustContractPanel({ open, onClose, contract, onCompleted }: ReadjustContractPanelProps) {
  const currentValue = contract.value || 0;
  // `adjust_index` é o campo usado pelo gerador de contrato; `index` é o legado.
  const indexFromContract = contract.adjust_index || contract.index || "IGPM";

  const [mode, setMode] = useState<"index" | "value">("index");
  const [indexName, setIndexName] = useState(indexFromContract);
  const [indexValue, setIndexValue] = useState<number>(0);
  const [newValue, setNewValue] = useState<number>(currentValue);
  const [effectiveFrom, setEffectiveFrom] = useState<string>(() => {
    // O reajuste normalmente vigora na data já agendada no contrato; sem
    // agendamento, hoje.
    const scheduled = contract.next_adjustment ? new Date(contract.next_adjustment) : null;
    return scheduled && !Number.isNaN(scheduled.getTime()) ? toDateInput(scheduled) : toDateInput(new Date());
  });
  const [nextAdjustment, setNextAdjustment] = useState<string>(() => {
    const base = contract.next_adjustment ? new Date(contract.next_adjustment) : new Date();
    const next = Number.isNaN(base.getTime()) ? new Date() : base;
    next.setFullYear(next.getFullYear() + 1);
    return toDateInput(next);
  });
  const [note, setNote] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);

  useEffect(() => {
    if (!open) return;
    setMode("index");
    setIndexName(indexFromContract);
    setIndexValue(0);
    setNewValue(currentValue);
    const scheduled = contract.next_adjustment ? new Date(contract.next_adjustment) : null;
    setEffectiveFrom(
      scheduled && !Number.isNaN(scheduled.getTime()) ? toDateInput(scheduled) : toDateInput(new Date()),
    );
    const base = scheduled && !Number.isNaN(scheduled.getTime()) ? scheduled : new Date();
    const next = new Date(base);
    next.setFullYear(next.getFullYear() + 1);
    setNextAdjustment(toDateInput(next));
    setNote("");
    setConfirmOpen(false);
  }, [open, indexFromContract, currentValue, contract.next_adjustment]);

  const calculatedValue = useMemo(() => {
    if (mode === "value") return newValue;
    return Math.round(currentValue * (1 + indexValue / 100) * 100) / 100;
  }, [mode, newValue, currentValue, indexValue]);

  const delta = calculatedValue - currentValue;
  const deltaPct = currentValue > 0 ? (delta / currentValue) * 100 : 0;
  const valorInvalido = calculatedValue <= 0 || (mode === "index" && indexValue === 0);

  const applyReadjust = async () => {
    setSubmitting(true);
    try {
      const req: ReadjustContractRequest = {
        index_name: indexName,
        effective_from: new Date(effectiveFrom + "T00:00:00").toISOString(),
        next_adjustment: nextAdjustment,
        note: note.trim() || undefined,
      };
      if (mode === "value") req.new_value = newValue;
      else req.index_value = indexValue;

      const updated = await contractService.readjust(contract.id, req);
      toast.success(`Contrato reajustado. Novo valor: ${formatCurrency(calculatedValue)}.`);
      onCompleted?.(updated);
      setConfirmOpen(false);
      onClose();
    } catch (err: unknown) {
      const message = (err as { response?: { data?: { message?: string } } })?.response?.data?.message;
      toast.error(message || "Erro ao reajustar contrato.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <>
      <Sheet open={open} onOpenChange={(next: boolean) => { if (!next && !submitting) onClose(); }}>
        <SheetContent side="right" expandable className="w-full gap-0 overflow-hidden p-0 sm:max-w-lg">
          <div className="flex items-start gap-3 border-b border-border px-6 py-4 pr-20">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary-50">
              <TrendingUp className="h-5 w-5 text-primary-600" />
            </div>
            <div className="min-w-0 flex-1">
              <SheetTitle className="text-xl font-bold text-foreground">Reajustar contrato</SheetTitle>
              <SheetDescription className="mt-1">
                Aplica o índice ou o valor novo ao contrato e recalcula as cobranças em aberto a partir da vigência.
              </SheetDescription>
            </div>
          </div>

          <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-6 py-5">
            <div className="flex items-center justify-between rounded-xl border border-border bg-muted px-4 py-3">
              <div>
                <p className="text-xs text-muted-foreground">Valor atual</p>
                <p className="text-lg font-bold text-foreground">{formatCurrency(currentValue)}</p>
              </div>
              <div className="text-right">
                <p className="text-xs text-muted-foreground">Novo valor</p>
                <p className={clsx("text-lg font-bold", delta === 0 ? "text-muted-foreground" : "text-primary-600")}>
                  {formatCurrency(calculatedValue)}
                </p>
                <p
                  className={clsx(
                    "text-xs font-medium",
                    delta > 0 ? "text-status-warning" : delta < 0 ? "text-status-info" : "text-muted-foreground",
                  )}
                >
                  {delta >= 0 ? "+" : ""}
                  {formatCurrency(delta)} ({deltaPct.toFixed(2)}%)
                </p>
              </div>
            </div>

            <div className="flex items-center gap-2 rounded-xl border border-border bg-muted p-1">
              {(
                [
                  { id: "index", label: "Por índice" },
                  { id: "value", label: "Valor direto" },
                ] as const
              ).map((option) => (
                <button
                  key={option.id}
                  type="button"
                  onClick={() => setMode(option.id)}
                  className={clsx(
                    "flex-1 rounded-lg px-3 py-2 text-sm font-medium transition-colors",
                    mode === option.id ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  {option.label}
                </button>
              ))}
            </div>

            {mode === "index" ? (
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label htmlFor="readjust-index" className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-foreground">
                    Índice
                  </label>
                  <select
                    id="readjust-index"
                    value={indexName}
                    onChange={(event) => setIndexName(event.target.value)}
                    className="h-10 w-full rounded-lg border border-border bg-card px-3 text-sm outline-none focus:ring-2 focus:ring-primary-500"
                  >
                    {["IGPM", "IPCA", "INCC", "IGP-DI", "IPC-FIPE", "OUTROS"].map((item) => (
                      <option key={item} value={item}>{item}</option>
                    ))}
                  </select>
                </div>
                <div>
                  <label htmlFor="readjust-pct" className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-foreground">
                    % de reajuste
                  </label>
                  <input
                    id="readjust-pct"
                    type="number"
                    step="0.01"
                    value={indexValue}
                    onChange={(event) => setIndexValue(Number(event.target.value))}
                    className="h-10 w-full rounded-lg border border-border bg-card px-3 text-sm outline-none focus:ring-2 focus:ring-primary-500"
                  />
                </div>
              </div>
            ) : (
              <div>
                <label htmlFor="readjust-value" className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-foreground">
                  Novo valor mensal
                </label>
                <input
                  id="readjust-value"
                  type="number"
                  step="0.01"
                  value={newValue}
                  onChange={(event) => setNewValue(Number(event.target.value))}
                  className="h-10 w-full rounded-lg border border-border bg-card px-3 text-sm outline-none focus:ring-2 focus:ring-primary-500"
                />
              </div>
            )}

            <div className="grid grid-cols-2 gap-4">
              <div>
                <label htmlFor="readjust-from" className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-foreground">
                  Vigente a partir de
                </label>
                <input
                  id="readjust-from"
                  type="date"
                  value={effectiveFrom}
                  onChange={(event) => setEffectiveFrom(event.target.value)}
                  className="h-10 w-full rounded-lg border border-border bg-card px-3 text-sm outline-none focus:ring-2 focus:ring-primary-500"
                />
              </div>
              <div>
                <label htmlFor="readjust-next" className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-foreground">
                  Próximo reajuste
                </label>
                <input
                  id="readjust-next"
                  type="date"
                  value={nextAdjustment}
                  onChange={(event) => setNextAdjustment(event.target.value)}
                  className="h-10 w-full rounded-lg border border-border bg-card px-3 text-sm outline-none focus:ring-2 focus:ring-primary-500"
                />
              </div>
            </div>

            <div>
              <label htmlFor="readjust-note" className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-foreground">
                Observação (opcional)
              </label>
              <textarea
                id="readjust-note"
                value={note}
                onChange={(event) => setNote(event.target.value)}
                rows={2}
                placeholder="Ex.: reajuste anual pelo IGP-M acumulado"
                className="w-full resize-none rounded-lg border border-border bg-muted px-3 py-2 text-sm outline-none focus:bg-card focus:ring-2 focus:ring-primary-500"
              />
            </div>

            {/*
              O aviso anterior dizia que cobranças já emitidas não são alteradas.
              Não é o que o financeiro faz: as PENDING com vencimento a partir da
              vigência SÃO recalculadas. Pagas e vencidas antes da vigência ficam
              intactas. Dizer o contrário escondia do operador o efeito real.
            */}
            <p className="rounded-xl border border-border bg-muted/60 px-4 py-3 text-xs leading-relaxed text-muted-foreground">
              O reajuste altera o valor do contrato e <strong>recalcula as cobranças em aberto com vencimento a
              partir da vigência</strong>. Cobranças já pagas e as vencidas antes dessa data não mudam.
            </p>
          </div>

          <div className="flex items-center justify-end gap-3 border-t border-border bg-muted/30 px-6 py-4">
            <Button variant="outline" onClick={onClose} disabled={submitting}>
              Cancelar
            </Button>
            <Button onClick={() => setConfirmOpen(true)} disabled={submitting || valorInvalido}>
              <TrendingUp className="h-4 w-4" />
              Revisar e aplicar
            </Button>
          </div>
        </SheetContent>
      </Sheet>

      {/* O AlertDialog existe só para a decisão: é alteração de valor com efeito
          em cobranças já emitidas, e o resumo precisa estar na frente do
          operador no momento do clique. */}
      <AlertDialog
        open={confirmOpen}
        onOpenChange={(next) => {
          if (!next && !submitting) setConfirmOpen(false);
        }}
      >
        <AlertDialogContent className="gap-4 sm:max-w-lg">
          <div className="flex items-start gap-4">
            <div className="shrink-0 rounded-xl bg-status-warning-soft p-3">
              <AlertTriangle className="h-6 w-6 text-status-warning" />
            </div>
            <div className="min-w-0 flex-1">
              <AlertDialogTitle className="text-lg font-bold text-foreground">
                Confirmar reajuste do contrato
              </AlertDialogTitle>
              <AlertDialogDescription className="mt-1">
                {mode === "index"
                  ? `Reajuste de ${indexValue}% pelo índice ${indexName}.`
                  : "Reajuste por valor direto."}
              </AlertDialogDescription>
            </div>
          </div>

          <dl className="space-y-2 rounded-xl border border-border bg-muted px-4 py-3 text-sm">
            <div className="flex items-center justify-between gap-4">
              <dt className="text-muted-foreground">Valor mensal</dt>
              <dd className="text-right font-medium text-foreground">
                {formatCurrency(currentValue)} <span className="text-muted-foreground">→</span>{" "}
                <strong className="text-primary-600">{formatCurrency(calculatedValue)}</strong>
              </dd>
            </div>
            <div className="flex items-center justify-between gap-4">
              <dt className="text-muted-foreground">Variação</dt>
              <dd className={clsx("font-bold", delta > 0 ? "text-status-warning" : delta < 0 ? "text-status-info" : "text-muted-foreground")}>
                {delta >= 0 ? "+" : ""}
                {formatCurrency(delta)} ({deltaPct.toFixed(2)}%)
              </dd>
            </div>
            <div className="flex items-center justify-between gap-4">
              <dt className="text-muted-foreground">Vigência</dt>
              <dd className="font-medium text-foreground">
                {new Date(effectiveFrom + "T00:00:00").toLocaleDateString("pt-BR")}
              </dd>
            </div>
            <div className="flex items-center justify-between gap-4">
              <dt className="text-muted-foreground">Próximo reajuste</dt>
              <dd className="font-medium text-foreground">
                {new Date(nextAdjustment + "T00:00:00").toLocaleDateString("pt-BR")}
              </dd>
            </div>
          </dl>

          <p className="text-xs leading-relaxed text-muted-foreground">
            As cobranças <strong>em aberto com vencimento a partir da vigência serão recalculadas</strong> com o novo
            valor. Cobranças pagas e as vencidas antes dessa data permanecem como estão.
          </p>

          <AlertDialogFooter>
            <AlertDialogCancel disabled={submitting}>Voltar</AlertDialogCancel>
            <Button onClick={() => void applyReadjust()} disabled={submitting}>
              {submitting ? <Loader2 className="h-4 w-4 animate-spin" /> : <TrendingUp className="h-4 w-4" />}
              Aplicar reajuste
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
