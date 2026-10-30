import { Check } from "lucide-react";
import { Badge } from "../../ui/badge";
import { Button } from "../../ui/button";
import { planAddsBenefits, planRemovesBenefits, type PlatformPlan, type PlatformSubscription } from "../../services/platformSubscription.service";

export const money = (cents: number) => new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(cents / 100);
export const planPeriod = (plan: PlatformPlan) => (plan.cycle === "ANNUALLY" ? "ano" : "mês");

export interface PlanCardProps {
  plan: PlatformPlan;
  /** Resolved by the access endpoint; raw subscription status may lag expiry webhooks. */
  entitled?: boolean;
  /** Free days that still apply to this customer (0 when already used). */
  trialDays: number;
  /** Same tier, monthly cycle: the reference for the annual saving. */
  monthlyPlan?: PlatformPlan;
  state: PlatformSubscription;
  usage: { leads: number; properties: number; users: number };
  canManage: boolean;
  enabled: boolean;
  busy: boolean;
  recommended: boolean;
  onSelect: (plan: PlatformPlan) => void;
}

/** One catalogue card: price, what the plan unlocks and the next step. */
export function PlanCard({ plan, trialDays, monthlyPlan, state, entitled: resolvedEntitled, usage, canManage, enabled, busy, recommended, onSelect }: PlanCardProps) {
  if (plan.contact_only) {
    return (
      <article className="flex flex-col rounded-xl border border-border bg-card p-6">
        <h3 className="text-xl font-bold">{plan.name}</h3>
        <p className="mt-2 text-sm text-muted-foreground">{plan.description}</p>
        <p className="my-5 text-3xl font-bold">Sob consulta</p>
        <p className="mt-auto text-sm text-muted-foreground">Fale com o time para uma proposta com as regras da sua operação.</p>
      </article>
    );
  }

  const accessPlan = state.plan ?? state.trial_plan ?? null;
  const active = state.plan?.tier === plan.tier && state.plan?.cycle === plan.cycle;
  const entitled = resolvedEntitled ?? (state.status === "ACTIVE" || state.status === "TRIALING");
  const current = active && entitled;
  const upgrade = !accessPlan || planAddsBenefits(accessPlan, plan);
  const exceeds = usage.leads > plan.limits.leads || usage.properties > plan.limits.properties || usage.users > plan.limits.users;
  // Coming back is always welcome: the checkout defers the first charge to the
  // end of whatever the customer still owns. Only a period too long for the
  // provider to defer (over 90 days) has to wait.
  const paidDays = state.paid_through ? Math.max(0, Math.ceil((new Date(state.paid_through).getTime() - Date.now()) / 86_400_000)) : 0;
  const longPaidPeriod = state.status !== "PAST_DUE" && paidDays > 90;
  const ended = state.status === "CANCELLED" && paidDays > 0 && !longPaidPeriod;
  const blocked = exceeds || state.status === "PAST_DUE" || longPaidPeriod;
  const pending = state.pending_change?.plan.tier === plan.tier && state.pending_change?.plan.cycle === plan.cycle;
  const pendingUpgrade = pending && planAddsBenefits(state.plan, state.pending_change?.plan);
  const pendingDowngrade = pending && planRemovesBenefits(state.plan, state.pending_change?.plan);
  const otherCheckout = !!state.checkout && (state.checkout.plan.tier !== plan.tier || state.checkout.plan.cycle !== plan.cycle);
  const saving = plan.cycle === "ANNUALLY" && monthlyPlan ? monthlyPlan.amount_cents * 12 - plan.amount_cents : 0;

  // O plano que já tem checkout em aberto é o caminho de volta para o
  // pagamento: o clique reusa o checkout e leva direto para a página do
  // provedor.
  const sameCheckout = !!state.checkout && state.checkout.plan.tier === plan.tier && state.checkout.plan.cycle === plan.cycle;
  const label = current
    ? "Seu plano atual"
    : sameCheckout
      ? "Continuar pagamento"
      : pending
      ? pendingUpgrade ? "Upgrade aplicado" : "Mudança agendada"
      : entitled
        ? upgrade ? `Fazer upgrade para ${plan.name}` : !state.subscription_id ? `Assinar ${plan.name}` : `Mudar para ${plan.name}`
        : trialDays > 0
          ? state.trial_granted_at ? `Assinar ${plan.name}` : `Começar ${trialDays} dias grátis`
          : "Assinar agora";

  const helper = planHelper({ exceeds, entitled, pending, pendingUpgrade, pendingDowngrade, otherCheckout, ended, longPaidPeriod, state });

  return (
    <article
      aria-label={`Plano ${plan.name}`}
      className={`flex flex-col rounded-xl border bg-card p-6 ${recommended ? "border-primary shadow-token-sm ring-1 ring-primary/30" : "border-border"}`}
    >
      <div className="flex items-start justify-between gap-3">
        <h3 className="text-xl font-bold">{plan.name}</h3>
        {recommended && !sameCheckout && <Badge variant="default">Mais popular</Badge>}
        {sameCheckout && <Badge variant="pending">Checkout em aberto</Badge>}
        {current && !recommended && !sameCheckout && <Badge variant="success">Ativo</Badge>}
      </div>
      <p className="mt-2 text-sm text-muted-foreground">{plan.description}</p>

      <p className="mt-5 text-3xl font-bold">
        {money(plan.amount_cents)}
        <span className="text-sm font-normal"> /{planPeriod(plan)}</span>
      </p>
      {plan.cycle === "ANNUALLY" && monthlyPlan && (
        <p className="mt-1 text-sm text-muted-foreground">
          Equivale a {money(plan.amount_cents / 12)}/mês — economia de {money(saving)} no ano.
        </p>
      )}
      {trialDays > 0 && !entitled && (
        <p className="mt-3 flex flex-wrap items-center gap-2">
          <Badge variant="info">{trialDays} dias grátis</Badge>
          <span className="text-sm text-muted-foreground">{trialDays === plan.trial_days ? "A primeira cobrança só no fim do teste." : "Dias que restam do seu período de teste."}</span>
        </p>
      )}
      {trialDays === 0 && !entitled && !plan.contact_only && (
        <p className="mt-3 text-sm text-muted-foreground">O período de teste já foi usado nesta conta: a cobrança é feita na contratação.</p>
      )}

      <ul className="mt-5 space-y-2 text-sm">
        <li className="flex items-start gap-2">
          <Check className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
          <span>
            Até <strong>{plan.limits.users}</strong> usuários · <strong>{plan.limits.leads}</strong> leads · <strong>{plan.limits.properties}</strong> imóveis
          </span>
        </li>
        {(plan.highlights ?? []).map(item => (
          <li key={item} className="flex items-start gap-2">
            <Check className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
            <span>{item}</span>
          </li>
        ))}
      </ul>

      {canManage ? (
        <div className="mt-auto pt-6">
          <Button
            className="w-full"
            variant={recommended && !current ? "default" : "outline"}
            disabled={busy || !enabled || (current && !pending) || blocked}
            onClick={() => onSelect(plan)}
          >
            {label}
          </Button>
          {helper && <p className="mt-2 text-sm text-muted-foreground">{helper}</p>}
        </div>
      ) : (
        <p className="mt-auto pt-6 text-sm text-muted-foreground">O administrador da conta pode mudar o plano.</p>
      )}
    </article>
  );
}

function planHelper({ exceeds, entitled, pending, pendingUpgrade, pendingDowngrade, otherCheckout, ended, longPaidPeriod, state }: {
  exceeds: boolean; entitled: boolean; pending: boolean; pendingUpgrade: boolean; pendingDowngrade: boolean; otherCheckout: boolean; ended: boolean; longPaidPeriod: boolean;
  state: PlatformSubscription;
}): string | null {
  if (exceeds) return "Seu uso atual passa deste plano. Reduza antes de mudar.";
  if (state.status === "PAST_DUE") return "Regularize o pagamento pendente para mudar de plano.";
  if (longPaidPeriod) return `Período pago longo: uma nova contratação fica disponível em ${state.paid_through ? new Date(state.paid_through).toLocaleDateString("pt-BR") : "no fim dele"}.`;
  if (ended) return `Você mantém o acesso até ${state.paid_through ? new Date(state.paid_through).toLocaleDateString("pt-BR") : "o fim do período pago"}; a cobrança da nova assinatura só começa quando ele terminar.`;
  if (otherCheckout) return `Você tem um checkout em aberto do ${state.checkout?.plan.name ?? "outro plano"}: ao assinar este, o anterior é substituído e nada é cobrado duas vezes.`;
  if (pendingUpgrade) return "Benefícios liberados para toda a equipe; o novo valor começa na próxima renovação.";
  if (pendingDowngrade) return "Redução aplicada após o fim do período atual, na próxima renovação.";
  if (pending) return "Os benefícios atuais são mantidos; a nova cobrança começa na próxima renovação.";
  if (entitled) return "A mudança de cobrança começa na próxima renovação.";
  return null;
}
