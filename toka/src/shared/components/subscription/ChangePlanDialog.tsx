import { AlertTriangle, ArrowDownRight, Check, Minus } from "lucide-react";
import { Button } from "../../ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "../../ui/dialog";
import { money, planPeriod } from "./PlanCard";
import type { PlatformPlan, PlatformSubscription } from "../../services/platformSubscription.service";

export interface ChangePlanDialogProps {
  open: boolean;
  target?: PlatformPlan;
  current: PlatformSubscription;
  usage: { leads: number; properties: number; users: number };
  busy: boolean;
  onCancel: () => void;
  onConfirm: (plan: PlatformPlan) => void;
}

/**
 * Downgrades are a decision with consequences: what shrinks, what stops being
 * included and what the customer keeps until the change takes effect.
 */
export function ChangePlanDialog({ open, target, current, usage, busy, onCancel, onConfirm }: ChangePlanDialogProps) {
  if (!target) return null;
  const from = current.plan;
  const rows = [
    { label: "usuários", now: from?.limits.users ?? 0, next: target.limits.users, used: usage.users },
    { label: "leads", now: from?.limits.leads ?? 0, next: target.limits.leads, used: usage.leads },
    { label: "imóveis", now: from?.limits.properties ?? 0, next: target.limits.properties, used: usage.properties },
  ];
  const overLimit = rows.filter(row => row.used > row.next);
  const lost = (from?.highlights ?? []).filter(item => !(target.highlights ?? []).includes(item));

  return (
    <Dialog open={open} onOpenChange={next => { if (!next) onCancel(); }}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ArrowDownRight className="h-5 w-5 text-status-warning" />
            Mudar para {target.name}
          </DialogTitle>
          <DialogDescription>
            A redução entra em vigor após o fim do período atual. Até a próxima renovação, todos os membros mantêm os benefícios do {from?.name ?? "plano atual"}.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 text-sm">
          <p>
            {money(target.amount_cents)} / {planPeriod(target)}
            {target.cycle === "ANNUALLY" ? ` — equivale a ${money(target.amount_cents / 12)}/mês.` : "."}
          </p>

          <div>
            <p className="font-medium">O que muda na capacidade</p>
            <ul className="mt-2 space-y-1">
              {rows.map(row => (
                <li key={row.label} className="flex items-center gap-2">
                  {row.next < row.now ? <Minus className="h-4 w-4 text-status-warning" /> : <Check className="h-4 w-4 text-status-ok" />}
                  <span>
                    {row.label}: <strong>{row.now.toLocaleString("pt-BR")}</strong> → <strong>{row.next.toLocaleString("pt-BR")}</strong>
                    <span className="text-muted-foreground"> (você usa {row.used.toLocaleString("pt-BR")})</span>
                  </span>
                </li>
              ))}
            </ul>
          </div>

          {lost.length > 0 && (
            <div>
              <p className="font-medium">Deixa de estar incluído</p>
              <ul className="mt-2 space-y-1 text-muted-foreground">
                {lost.map(item => <li key={item}>• {item}</li>)}
              </ul>
            </div>
          )}

          {overLimit.length > 0 ? (
            <div className="flex items-start gap-2 rounded-lg border border-status-critical bg-status-critical-soft p-4 text-status-critical">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>
                Seu uso atual passa do que o {target.name} cobre em {overLimit.map(row => row.label).join(", ")}.
                Reduza antes de mudar — nada é apagado, mas o excedente fica fora do plano.
              </span>
            </div>
          ) : (
            <p className="text-muted-foreground">Seu uso atual cabe neste plano, então nada é interrompido.</p>
          )}
        </div>

        <DialogFooter>
          <Button variant="ghost" disabled={busy} onClick={onCancel}>Voltar</Button>
          <Button disabled={busy || overLimit.length > 0} onClick={() => onConfirm(target)}>
            {busy ? "Agendando…" : `Agendar redução para ${target.name}`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
