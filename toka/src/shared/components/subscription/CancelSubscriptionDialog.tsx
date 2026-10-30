import { useEffect, useId, useState } from "react";
import { AlertTriangle, ArrowDownRight, CalendarClock, ShieldCheck } from "lucide-react";
import { Button } from "../../ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "../../ui/dialog";
import { Label } from "../../ui/label";
import { RadioGroup, RadioGroupItem } from "../../ui/radio-group";
import { cancellationReasons, type PlatformPlan, type PlatformSubscription } from "../../services/platformSubscription.service";

const money = (cents: number) => new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(cents / 100);
const date = (value?: string) => (value ? new Date(value).toLocaleDateString("pt-BR") : null);

export interface CancelSubscriptionDialogProps {
  open: boolean;
  state: PlatformSubscription;
  /** Cheaper plan that still fits the current usage, offered before cancelling. */
  retentionPlan?: PlatformPlan;
  busy: boolean;
  onCancel: () => void;
  onConfirm: (reason: string) => void;
  onDowngrade: (plan: PlatformPlan) => void;
}

/**
 * Cancellation is a decision, not a click: the dialog states what changes, why
 * we ask for a reason and what the customer can do instead of leaving.
 */
export function CancelSubscriptionDialog({ open, state, retentionPlan, busy, onCancel, onConfirm, onDowngrade }: CancelSubscriptionDialogProps) {
  const [reason, setReason] = useState<string>(cancellationReasons[0].value);
  const groupId = useId();
  useEffect(() => { if (open) setReason(cancellationReasons[0].value); }, [open]);

  const trialing = state.status === "TRIALING";
  const until = date(state.paid_through) ?? date(state.trial_ends_at);
  const ends = trialing
    ? "O teste termina agora e nenhuma cobrança é feita."
    : until
      ? `Nenhuma cobrança nova é feita. Você continua com acesso até ${until}.`
      : "Nenhuma cobrança nova é feita.";
  const back = trialing
    ? "Para usar a plataforma depois, é preciso assinar de novo."
    : "Quando o período terminar, é só assinar de novo para voltar.";

  return (
    <Dialog open={open} onOpenChange={next => { if (!next) onCancel(); }}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-status-warning" />
            {trialing ? "Encerrar o período de teste" : "Cancelar a renovação"}
          </DialogTitle>
          <DialogDescription>Veja o que muda antes de confirmar.</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <ul className="space-y-2 text-sm">
            <li className="flex items-start gap-2">
              <CalendarClock className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
              <span>{ends}</span>
            </li>
            <li className="flex items-start gap-2">
              <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
              <span>Seus leads, imóveis e contratos continuam salvos: nada é apagado.</span>
            </li>
            <li className="flex items-start gap-2">
              <ArrowDownRight className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
              <span>{back}</span>
            </li>
          </ul>

          {retentionPlan && (
            <div className="rounded-lg border border-primary/30 bg-primary/5 p-4">
              <p className="font-medium">Quer pagar menos sem perder o que já usa?</p>
              <p className="mt-1 text-sm text-muted-foreground">
                O plano {retentionPlan.name} custa {money(retentionPlan.amount_cents)}/{retentionPlan.cycle === "ANNUALLY" ? "ano" : "mês"} e cobre o seu uso atual
                (até {retentionPlan.limits.users} usuários, {retentionPlan.limits.leads} leads e {retentionPlan.limits.properties} imóveis).
              </p>
              <Button className="mt-3" variant="outline" disabled={busy} onClick={() => onDowngrade(retentionPlan)}>
                Mudar para {retentionPlan.name} e continuar
              </Button>
            </div>
          )}

          <div>
            <Label className="text-sm">Para ajudar a melhorar, por que está cancelando?</Label>
            <RadioGroup className="mt-3 space-y-2" value={reason} onValueChange={setReason}>
              {cancellationReasons.map(option => (
                <div key={option.value} className="flex items-center gap-2">
                  <RadioGroupItem value={option.value} id={`${groupId}-${option.value}`} />
                  <Label htmlFor={`${groupId}-${option.value}`} className="font-normal">{option.label}</Label>
                </div>
              ))}
            </RadioGroup>
          </div>
        </div>

        <DialogFooter>
          <Button variant="ghost" disabled={busy} onClick={onCancel}>Voltar</Button>
          <Button variant="destructive" aria-busy={busy} disabled={busy} onClick={() => onConfirm(reason)}>
            {busy ? "Cancelando…" : trialing ? "Encerrar teste" : "Confirmar cancelamento"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
