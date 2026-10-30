import { CreditCard, RefreshCw } from "lucide-react";
import { Badge } from "../../ui/badge";
import { Button } from "../../ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "../../ui/dialog";
import type { PlatformSubscription } from "../../services/platformSubscription.service";

const money = (cents: number) => new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(cents / 100);
const date = (value?: string) => (value ? new Date(value).toLocaleDateString("pt-BR") : null);

export interface PaymentMethodPanelProps {
  state: PlatformSubscription;
  /** What happens to the free period when the new subscription starts. */
  trialNote: string;
  canManage: boolean;
  enabled: boolean;
  busy: boolean;
  onChange: () => void;
  onOpen: () => void;
  dialogOpen: boolean;
  onCloseDialog: () => void;
}

/**
 * The provider tokenizes the card at checkout and exposes no API to replace it,
 * so the panel states what is on file and when a new card can be registered —
 * instead of offering a change that would not happen.
 */
export function PaymentMethodPanel({ state, trialNote, canManage, enabled, busy, onChange, onOpen, dialogOpen, onCloseDialog }: PaymentMethodPanelProps) {
  const method = state.payment_method;
  const plan = state.plan;
  const nextCharge = state.status === "TRIALING" ? state.trial_ends_at : state.paid_through;
  const failed = state.status === "PAST_DUE";
  const hasCard = !!method || !!state.subscription_id;
  const paidDays = state.paid_through ? Math.max(0, Math.ceil((new Date(state.paid_through).getTime() - Date.now()) / 86_400_000)) : 0;
  // The provider defers the first charge by at most 90 days, so a period longer
  // than that (an annual plan) has to wait — and the customer sees the date.
  const longPaidPeriod = state.status !== "PAST_DUE" && paidDays > 90;
  const waiting = longPaidPeriod;
  const owedNote = state.status === "TRIALING"
    ? `Nada foi cobrado ainda: o cartão vale para a primeira cobrança${state.trial_ends_at ? `, em ${date(state.trial_ends_at)}` : ""}.`
    : paidDays > 0
      ? `Você já pagou até ${date(state.paid_through)}. Ao trocar o cartão, a nova assinatura só passa a cobrar quando esse período terminar.`
      : "A próxima cobrança usa este cartão.";
  // No subscription yet, or a checkout still open: the card is informed there.
  const canChange = canManage && enabled && (hasCard || failed);

  return (
    <section aria-label="Forma de pagamento" className="rounded-xl border border-border bg-card p-6 shadow-sm">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="flex items-center gap-2 text-lg font-semibold"><CreditCard className="h-5 w-5 text-primary" />Forma de pagamento</h2>
          {method ? (
            <p className="mt-2 flex flex-wrap items-center gap-2">
              <span>{method.kind === "PIX" ? "Pix recorrente" : "Cartão de crédito"}</span>
              {method.brand && <Badge variant="neutral">{method.brand}</Badge>}
              {method.last4 && <span className="text-muted-foreground">final {method.last4}</span>}
            </p>
          ) : (
            <p className="mt-2 text-muted-foreground">{hasCard ? "Cartão cadastrado no provedor." : "Nenhuma forma de pagamento cadastrada."}</p>
          )}
          {plan && nextCharge && (
            <p className="mt-2 text-sm text-muted-foreground">
              {state.status === "TRIALING" ? "Primeira cobrança" : "Próxima cobrança"}: {money(plan.amount_cents)} em {date(nextCharge)}.
            </p>
          )}
          {failed && <p className="mt-2 text-sm text-status-critical">A última cobrança falhou. Cadastre um cartão novo para não perder a assinatura.</p>}
          {!failed && hasCard && <p className="mt-2 text-sm text-muted-foreground">{owedNote}</p>}
          {waiting && <p className="mt-2 text-sm text-muted-foreground">Período pago longo: a troca fica disponível em {date(state.paid_through)}.</p>}
          {!hasCard && <p className="mt-2 text-sm text-muted-foreground">O cartão é informado no checkout, ao contratar um plano.</p>}
        </div>
        {canChange && (
          <Button variant="outline" disabled={busy || waiting} onClick={onOpen}>
            <RefreshCw className="mr-2 h-4 w-4" />Trocar cartão
          </Button>
        )}
      </div>

      <Dialog open={dialogOpen} onOpenChange={next => { if (!next) onCloseDialog(); }}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Trocar o cartão da assinatura</DialogTitle>
            <DialogDescription>O cartão usado hoje continua valendo até a nova assinatura começar.</DialogDescription>
          </DialogHeader>
          <div className="space-y-3 text-sm">
            <p>
              O provedor guarda o cartão informado no checkout e não permite substituí-lo em uma assinatura existente. Para usar outro cartão,
              criamos uma nova assinatura no mesmo plano: a atual é encerrada e você informa o novo cartão na página de pagamento.
            </p>
            <p className="text-muted-foreground">{trialNote}</p>
            {state.status === "CANCELLED" && (
              <p className="text-muted-foreground">Sua assinatura está encerrada: esta troca também reativa o plano no cartão novo, sem cobrar de novo pelo período que você já pagou.</p>
            )}
            {failed && <p className="text-status-critical">As cobranças no cartão atual estão falhando, então esta troca evita o cancelamento automático.</p>}
          </div>
          <DialogFooter>
            <Button variant="ghost" disabled={busy} onClick={onCloseDialog}>Voltar</Button>
            <Button aria-busy={busy} disabled={busy} onClick={onChange}>{busy ? "Preparando…" : "Informar novo cartão"}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}
