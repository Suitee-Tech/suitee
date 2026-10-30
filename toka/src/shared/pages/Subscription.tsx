import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AlertCircle, ArrowDownRight, ArrowUpRight, Check, CreditCard, Crown, ExternalLink, PartyPopper, RefreshCw, Sparkles } from "lucide-react";
import { useAuth } from "../context/AuthContext";
import { usePermission } from "../permissions/usePermission";
import { useConfirmAction } from "../hooks/useConfirmAction";
import type { PlatformSubscription } from "../services/platformSubscription.service";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { ResponsiveTable } from "../ui/responsive-table";
import { Progress } from "../ui/progress";
import { PlanCard, money, planPeriod } from "../components/subscription/PlanCard";
import { CancelSubscriptionDialog } from "../components/subscription/CancelSubscriptionDialog";
import { ChangePlanDialog } from "../components/subscription/ChangePlanDialog";
import { PaymentMethodPanel } from "../components/subscription/PaymentMethodPanel";
import { dateBR as date, remainingDays } from "../utils/subscription";
import { planAddsBenefits, planRemovesBenefits, platformSubscriptionService, safeSubscriptionURL, type PlatformPlan, type SubscriptionCycle, type SubscriptionSnapshot } from "../services/platformSubscription.service";
import { useSubscriptionAccess } from "../context/SubscriptionAccessContext";

const statusLabel: Record<string, string> = {
  NONE: "Sem assinatura", PENDING: "Aguardando pagamento", ACTIVE: "Assinatura ativa",
  TRIALING: "Período de teste", PAST_DUE: "Pagamento pendente", CANCELLED: "Renovação cancelada", EXPIRED: "Período encerrado",
};
const statusVariant: Record<string, "success" | "info" | "critical" | "pending" | "neutral"> = {
  ACTIVE: "success", TRIALING: "info", PAST_DUE: "critical", PENDING: "pending", CANCELLED: "neutral", EXPIRED: "neutral",
};

/** The payment provider sends the customer back with this flag. */
const paymentReturned = () => {
  if (typeof window === "undefined") return false;
  const params = new URLSearchParams(window.location.search);
  if (params.get("payment") !== "returned") return false;
  params.delete("payment");
  const query = params.toString();
  window.history.replaceState({}, "", window.location.pathname + (query ? `?${query}` : ""));
  return true;
};

/**
 * Mensagem mostrada quando o checkout falha.
 *
 * A causa vem do servidor ("a conta do provedor não tem cobrança recorrente
 * habilitada", "encerre o checkout pendente antes de escolher outro plano") e
 * tem precedência: a mensagem genérica antiga mandava atualizar a página mesmo
 * quando repetir não resolvia nada — foi o que escondeu, em produção, que o
 * problema era a conta do AbacatePay.
 */
/**
 * Texto do banner de teste sobre assinar durante o período gratuito.
 *
 * Existe como função para o teste travar a regra: assinar durante o teste NÃO
 * antecipa cobrança — os dias restantes continuam valendo até o fim do período.
 */
export function textoDoTrial(state: { trial_ends_at?: string; trial_granted_at?: string }): string {
  const dias = state.trial_ends_at ? remainingDays(state.trial_ends_at) : 0;
  if (dias <= 0) {
    return "A primeira cobrança acontece no fim do período de teste.";
  }
  const data = state.trial_ends_at ? date(state.trial_ends_at) : "no fim do teste";
  return `Assinar agora não antecipa cobrança: os ${dias} ${dias === 1 ? "dia restante continua valendo" : "dias restantes continuam valendo"} e a primeira cobrança acontece em ${data}.`;
}

export function selectErroServidor(err: unknown): string {
  const doServidor = (err as { response?: { data?: { message?: string } } })?.response?.data?.message;
  return (
    (typeof doServidor === "string" && doServidor.trim()) ||
    "Não foi possível confirmar a operação. Atualize a assinatura para recuperar um checkout pendente antes de tentar novamente."
  );
}

export function Subscription() {
  const { user } = useAuth();
  const { refresh: refreshPlanAccess } = useSubscriptionAccess();
  const tenant = user?.current_company_id;
  const canManage = usePermission("settings:manage_general");
  const [snapshot, setSnapshot] = useState<SubscriptionSnapshot | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [cycle, setCycle] = useState<SubscriptionCycle>("MONTHLY");
  const [notice, setNotice] = useState<string | null>(null);
  const [celebrate, setCelebrate] = useState(false);
  const [cancelOpen, setCancelOpen] = useState(false);
  const [downgrade, setDowngrade] = useState<PlatformPlan | null>(null);
  const [methodOpen, setMethodOpen] = useState(false);
  const generation = useRef(0);
  const activeTenant = useRef(tenant);
  activeTenant.current = tenant;
  const { confirmAction, confirmationDialog } = useConfirmAction();

  const load = useCallback(async () => {
    const request = ++generation.current;
    try {
      const result = await platformSubscriptionService.get();
      if (request !== generation.current) return;
      setSnapshot(result);
      setError(null);
    } catch {
      if (request === generation.current) setError("Não foi possível consultar a assinatura. Tente novamente.");
    } finally { if (request === generation.current) setLoading(false); }
  }, []);
  const invalidate = useCallback(() => { generation.current++; }, []);
  useEffect(() => {
    setSnapshot(null); setError(null); setNotice(null); setLoading(true);
    if (paymentReturned()) setCelebrate(true);
    void load();
    return invalidate;
  }, [tenant, load, invalidate]);

  const pending = snapshot?.subscription.status === "PENDING";
  useEffect(() => {
    const refresh = () => { if (document.visibilityState === "visible") void load(); };
    window.addEventListener("focus", refresh);
    const interval = pending ? window.setInterval(refresh, 10000) : undefined;
    return () => { window.removeEventListener("focus", refresh); window.clearInterval(interval); };
  }, [pending, load]);

  async function select(plan: PlatformPlan) {
    const operationTenant = tenant;
    if (activeTenant.current !== operationTenant) throw new Error("A imobiliária ativa mudou. Selecione o plano novamente.");
    setBusy(true); setError(null); setNotice(null);
    try {
      const result = await platformSubscriptionService.select(plan.tier, plan.cycle);
      if (activeTenant.current !== operationTenant) return;
      const checkoutURL = safeSubscriptionURL(result.checkout?.url);
      if (checkoutURL) { window.location.assign(checkoutURL); return; }
      const isUpgrade = !!result.pending_change && planAddsBenefits(result.plan, result.pending_change.plan);
      setNotice(result.pending_change
        ? isUpgrade
          ? `Upgrade para ${result.pending_change.plan.name} liberado para todos os membros. A nova cobrança começa na próxima renovação.`
          : `Mudança para ${result.pending_change.plan.name} agendada para depois do período atual.`
        : "Plano atualizado.");
      await load();
      await refreshPlanAccess();
    } catch (err) {
      if (activeTenant.current === operationTenant) setError(selectErroServidor(err));
      throw new Error("Falha ao confirmar a assinatura");
    } finally { setBusy(false); }
  }
  async function cancel(reason: string) {
    const operationTenant = tenant;
    if (activeTenant.current !== operationTenant) throw new Error("A imobiliária ativa mudou. Confirme o cancelamento novamente.");
    setBusy(true);
    try {
      await platformSubscriptionService.cancel(reason);
      if (activeTenant.current === operationTenant) {
        setCancelOpen(false);
        setNotice("Cancelamento confirmado. Nenhuma cobrança nova será feita.");
        await load();
      }
    } finally { setBusy(false); }
  }

  const changeMethod = useCallback(async () => {
    const operationTenant = tenant;
    setBusy(true); setError(null); setNotice(null);
    try {
      const result = await platformSubscriptionService.changePaymentMethod();
      if (activeTenant.current !== operationTenant) return;
      const checkoutURL = safeSubscriptionURL(result.checkout?.url);
      if (checkoutURL) { window.location.assign(checkoutURL); return; }
      setMethodOpen(false);
      setNotice("Checkout criado para cadastrar o novo cartão. Atualize para continuar.");
      await load();
    } catch {
      if (activeTenant.current === operationTenant) setError("Não foi possível preparar a troca de cartão. Atualize a assinatura e tente novamente.");
      throw new Error("Falha ao trocar o cartão");
    } finally { setBusy(false); }
  }, [tenant, load]);
  const openReceipt = useCallback(async (paymentId: string) => {
    setError(null);
    try {
      const blob = await platformSubscriptionService.receipt(paymentId);
      const url = URL.createObjectURL(blob);
      window.open(url, "_blank", "noopener,noreferrer");
      // Revoke later: the new tab needs the URL alive while it loads.
      window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
    } catch {
      setError("Não foi possível emitir o comprovante deste pagamento. Tente novamente.");
    }
  }, []);
  const revertChange = useCallback(async () => {
    const operationTenant = tenant;
    setBusy(true); setError(null); setNotice(null);
    try {
      await platformSubscriptionService.cancelChange();
      if (activeTenant.current !== operationTenant) return;
      setNotice("Alteração cancelada. O plano atual continua com a equipe e será cobrado na próxima renovação.");
      await load();
      await refreshPlanAccess();
    } catch {
      if (activeTenant.current === operationTenant) setError("Não foi possível cancelar a alteração agendada. Atualize e tente novamente.");
      throw new Error("Falha ao cancelar a alteração");
    } finally { setBusy(false); }
  }, [tenant, load, refreshPlanAccess]);

  const state = snapshot?.subscription;
  const plan = state?.plan ?? state?.trial_plan ?? undefined;
  // O que a equipe PODE usar vem do endpoint de acesso: um upgrade confirmado já
  // está em vigor, mesmo com o plano gravado (e a cobrança) seguindo o anterior
  // até a renovação. Uso e sugestão de upgrade olham para os limites efetivos.
  const effectivePlan = snapshot?.access?.access_control_enabled ? snapshot.access.plan ?? plan : plan;
  const pendingUpgrade = !!state?.pending_change && planAddsBenefits(state.plan, state.pending_change.plan);
  const pendingDowngrade = !!state?.pending_change && planRemovesBenefits(state.plan, state.pending_change.plan);
  const entitled = snapshot?.access?.entitled ?? (state?.status === "ACTIVE" || state?.status === "TRIALING");
  const subscriptionStatus = snapshot?.access?.access_control_enabled ? snapshot.access.status : state?.status ?? "NONE";
  const isTrialing = snapshot?.access?.access_control_enabled ? snapshot.access.status === "TRIALING" : state?.status === "TRIALING";
  const paidPeriod = !!state?.paid_through && new Date(state.paid_through).getTime() > Date.now();
  const planosRef = useRef<HTMLElement | null>(null);
  const plans = useMemo(() => (snapshot?.plans ?? []).filter(p => p.cycle === cycle || p.contact_only), [snapshot?.plans, cycle]);
  const monthlyOf = useCallback((tier: string) => snapshot?.plans.find(p => p.tier === tier && p.cycle === "MONTHLY"), [snapshot?.plans]);

  // Growth path: the next tier up is the one worth showing.
  const upgradePlan = useMemo(() => {
    if (!snapshot) return undefined;
    const currentLeads = effectivePlan?.limits.leads ?? 0;
    return snapshot.plans
      .filter(p => p.cycle === "MONTHLY" && !p.contact_only && p.limits.leads > currentLeads)
      .sort((a, b) => a.limits.leads - b.limits.leads)[0];
  }, [snapshot, effectivePlan]);
  const recommended = useMemo(() => {
    if (!snapshot) return undefined;
    const monthly = snapshot.plans.filter(p => p.cycle === "MONTHLY" && !p.contact_only);
    if (upgradePlan) return monthly.find(p => p.tier === upgradePlan.tier);
    return monthly.find(p => p.tier === "pro") ?? monthly[0];
  }, [snapshot, upgradePlan]);

  // Retention: a cheaper plan that still fits what the customer already uses.
  const retentionPlan = useMemo(() => {
    if (!snapshot || !plan) return undefined;
    return snapshot.plans
      .filter(p => p.cycle === plan.cycle && !p.contact_only && p.amount_cents < plan.amount_cents
        && snapshot.usage.leads <= p.limits.leads && snapshot.usage.properties <= p.limits.properties && snapshot.usage.users <= p.limits.users)
      .sort((a, b) => b.amount_cents - a.amount_cents)[0];
  }, [snapshot, plan]);

  const pressure = useMemo(() => {
    if (!snapshot || !effectivePlan) return null;
    const rows = [
      { key: "leads", label: "Leads cadastrados", used: snapshot.usage.leads, limit: effectivePlan.limits.leads },
      { key: "imoveis", label: "Imóveis cadastrados", used: snapshot.usage.properties, limit: effectivePlan.limits.properties },
      { key: "usuarios", label: "Usuários da equipe", used: snapshot.usage.users, limit: effectivePlan.limits.users },
    ];
    const tightest = rows.reduce((worst, row) => (row.limit > 0 && row.used / row.limit > worst.ratio ? { ratio: row.used / row.limit, row } : worst), { ratio: 0, row: rows[0] });
    return { rows, tightest };
  }, [snapshot, effectivePlan]);

  const showCelebration = celebrate && entitled && !!plan;
  const trialDays = state?.trial_ends_at ? remainingDays(state.trial_ends_at) : 0;
  // The free period is granted once: after that the checkout charges right away.
  const trialGranted = !!state?.trial_granted_at;
  const checkoutTrialDays = !trialGranted ? 15 : state?.trial_ends_at ? remainingDays(state.trial_ends_at) : 0;
  const changePlanCopy = checkoutTrialDays > 0
    ? `Nada é cobrado agora: ${checkoutTrialDays} ${checkoutTrialDays === 1 ? "dia continua valendo" : "dias continuam valendo"} do que você já tem, e a primeira cobrança no cartão novo acontece no fim desse período.`
    : "A nova assinatura é cobrada no cartão novo já na contratação e renova automaticamente.";

  return (
    <div className="flex-1 overflow-auto bg-muted text-foreground">
      <div className="mx-auto max-w-7xl space-y-6 p-6 lg:p-8">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <h1 className="text-3xl font-bold">Assinatura e Consumo</h1>
            <p className="mt-1 text-muted-foreground">Seu plano, o que ele libera e quanto da capacidade você já usa.</p>
          </div>
          <Button variant="outline" onClick={() => void load()} disabled={busy || loading}><RefreshCw className="mr-2 h-4 w-4" />Atualizar</Button>
        </div>

        {snapshot?.dev_mode && <div className="rounded-lg border border-border bg-card p-4 text-sm">Ambiente beta: pagamentos de teste no sandbox do AbacatePay.</div>}
        {error && <div role="alert" className="flex items-center gap-2 rounded-lg border border-status-critical bg-status-critical-soft p-4 text-status-critical"><AlertCircle className="h-5 w-5" />{error}</div>}
        {notice && <div role="status" className="rounded-lg border border-border bg-card p-4">{notice}</div>}
        {loading && <p role="status">Carregando assinatura…</p>}

        {snapshot && state && <>
          {!snapshot.enabled && <div role="alert" className="rounded-lg border border-border bg-card p-4">Pagamentos de assinatura indisponíveis neste ambiente. Consulte o suporte.</div>}

          {showCelebration && (
            <section aria-label="Assinatura liberada" className="rounded-xl border border-status-ok bg-status-ok-soft p-6">
              <h2 className="flex items-center gap-2 text-xl font-semibold text-status-ok"><PartyPopper className="h-6 w-6" />Tudo pronto: {plan!.name} liberado</h2>
              <p className="mt-2 text-sm">
                {state.status === "TRIALING"
                  ? `Seu teste de ${plan!.trial_days} dias começou. A primeira cobrança de ${money(plan!.amount_cents)} só acontece em ${state.trial_ends_at ? date(state.trial_ends_at) : "no fim do teste"}.`
                  : `Sua conta agora tem ${plan!.limits.leads} leads, ${plan!.limits.properties} imóveis e ${plan!.limits.users} usuários.`}
              </p>
              <ul className="mt-4 grid gap-2 text-sm sm:grid-cols-2">
                {(plan!.highlights ?? []).map(item => (
                  <li key={item} className="flex items-start gap-2"><Check className="mt-0.5 h-4 w-4 shrink-0 text-status-ok" />{item}</li>
                ))}
              </ul>
              <div className="mt-4 flex flex-wrap gap-3">
                <Button variant="outline" onClick={() => setCelebrate(false)}>Começar a usar</Button>
                <a className="inline-flex items-center gap-2 self-center font-medium text-primary" href="/ajuda">Ver como começar na Central de Ajuda<ExternalLink className="h-4 w-4" /></a>
              </div>
            </section>
          )}

          {isTrialing && !showCelebration && (
            <section aria-label="Período de teste" className="rounded-xl border border-status-info bg-status-info-soft p-6">
              <h2 className="flex items-center gap-2 text-lg font-semibold text-status-info"><Sparkles className="h-5 w-5" />Período de teste: {trialDays} {trialDays === 1 ? "dia restante" : "dias restantes"}</h2>
              <p className="mt-2 text-sm">
                {state.trial_plan && !state.subscription_id
                  ? `Seu teste do ${state.trial_plan.name} começou no primeiro acesso, sem cartão. Escolha um plano para manter os recursos depois de ${state.trial_ends_at ? date(state.trial_ends_at) : "15 dias"}.`
                  : `Você está usando ${plan?.name} sem pagar nada. A primeira cobrança de ${plan ? money(plan.amount_cents) : ""} acontece em ${state.trial_ends_at ? date(state.trial_ends_at) : "no fim do teste"} — cancele antes e nada é cobrado.`}
              </p>
              {/* Sem CTA o trial só "avisa"; assinar durante o teste é a ação
                  esperada e não perde os dias que restam. */}
              {canManage && snapshot.enabled && !state.checkout && (
                <p className="mt-2 text-sm text-muted-foreground">{textoDoTrial(state)}</p>
              )}
              {canManage && snapshot.enabled && !state.checkout && (
                <div className="mt-4 flex flex-wrap gap-3">
                  <Button onClick={() => planosRef.current?.scrollIntoView({ behavior: "smooth", block: "start" })}>
                    {trialDays > 0 ? "Assinar agora e manter o teste" : "Escolher um plano"}
                  </Button>
                  <a className="inline-flex items-center gap-2 self-center text-sm font-medium text-primary" href="/ajuda">Tirar dúvidas<ExternalLink className="h-4 w-4" /></a>
                </div>
              )}
            </section>
          )}

          {state.pending_change && !showCelebration && (
            <section aria-label="Mudança agendada" className="rounded-xl border border-primary/30 bg-primary/5 p-6">
              <h2 className="flex items-center gap-2 text-lg font-semibold text-primary">
                {pendingUpgrade
                  ? <ArrowUpRight className="h-5 w-5" />
                  : pendingDowngrade ? <ArrowDownRight className="h-5 w-5" /> : <CreditCard className="h-5 w-5" />}
                {pendingUpgrade
                  ? `Upgrade para ${state.pending_change.plan.name} já liberado`
                  : pendingDowngrade
                    ? `Seu plano reduz para ${state.pending_change.plan.name} na renovação`
                    : `Sua cobrança muda para ${state.pending_change.plan.name} na renovação`}
              </h2>
              <p className="mt-2 text-sm">
                {pendingUpgrade
                  ? `Todos os membros da empresa já têm os benefícios e limites do ${state.pending_change.plan.name}. A nova cobrança de ${money(state.pending_change.plan.amount_cents)} / ${planPeriod(state.pending_change.plan)} começa na próxima renovação.`
                  : pendingDowngrade
                    ? `Até o fim do período atual, todos os membros mantêm os benefícios do ${plan?.name ?? "plano vigente"}. Depois, na próxima renovação, passam a valer ${state.pending_change.plan.limits.leads.toLocaleString("pt-BR")} leads, ${state.pending_change.plan.limits.properties.toLocaleString("pt-BR")} imóveis e ${state.pending_change.plan.limits.users} usuários, por ${money(state.pending_change.plan.amount_cents)} / ${planPeriod(state.pending_change.plan)}.`
                    : `Os benefícios e limites continuam iguais. A cobrança de ${money(state.pending_change.plan.amount_cents)} / ${planPeriod(state.pending_change.plan)} começa na próxima renovação.`}
              </p>
              {(state.pending_change.plan.highlights ?? []).length > 0 && (
                <ul className="mt-4 grid gap-2 text-sm sm:grid-cols-2">
                  {(state.pending_change.plan.highlights ?? []).map(item => (
                    <li key={item} className="flex items-start gap-2"><Check className="mt-0.5 h-4 w-4 shrink-0 text-primary" />{item}</li>
                  ))}
                </ul>
              )}
              {canManage && snapshot.enabled && (
                <div className="mt-4 flex flex-wrap items-center gap-3">
                  <Button
                    variant="outline"
                    disabled={busy}
                    onClick={() => confirmAction(
                      pendingUpgrade
                        ? `O upgrade para ${state.pending_change!.plan.name} será desfeito agora para todos os membros. A próxima renovação continua cobrando o ${plan?.name ?? "plano atual"}.`
                        : pendingDowngrade
                          ? `A redução agendada para ${state.pending_change!.plan.name} será cancelada e a próxima renovação continua cobrando o ${plan?.name ?? "plano atual"}.`
                          : `A mudança de cobrança agendada para ${state.pending_change!.plan.name} será cancelada.`,
                      revertChange,
                      { title: "Desfazer alteração agendada", confirmLabel: "Desfazer alteração" },
                    )}
                  >
                    Desfazer alteração
                  </Button>
                  <span className="text-sm text-muted-foreground">Se mudar de ideia, dá para voltar atrás sem falar com o suporte.</span>
                </div>
              )}
            </section>
          )}

          <section className="rounded-xl border border-border bg-card p-6 shadow-sm">
            <div className="flex flex-wrap items-start justify-between gap-4">
              <div>
                <h2 className="flex items-center gap-2 text-lg font-semibold">
                  <Crown className="h-5 w-5 text-primary" />Plano Atual: {plan?.name ?? "Nenhum"}
                  {subscriptionStatus !== "NONE" && <Badge variant={statusVariant[subscriptionStatus] ?? "neutral"}>{statusLabel[subscriptionStatus] ?? subscriptionStatus}</Badge>}
                </h2>
                {plan && <p className="mt-2">{money(plan.amount_cents)} / {planPeriod(plan)}</p>}
                {!plan && <p className="mt-2 text-muted-foreground">{statusLabel[state.status] ?? "A apurar"}</p>}
                {/* Dois testes diferentes: o de primeiro acesso (sem cartão) termina e
                    o cliente escolhe um plano; o contratado no provedor já tem cartão
                    e converte em cobrança — a data da primeira cobrança é a
                    informação que o cliente precisa ver. */}
                {isTrialing && state.trial_ends_at && (
                  <p className="mt-2 text-sm">
                    {state.subscription_id
                      ? `Primeira cobrança em ${date(state.trial_ends_at)}.`
                      : `Acesso de teste até ${date(state.trial_ends_at)}.`}
                  </p>
                )}
                {state.paid_through && <p className="mt-2 text-sm">{state.status === "CANCELLED" ? "Período pago até" : "Próxima renovação prevista"}: {date(state.paid_through)}</p>}
                {state.status === "PAST_DUE" && <p className="mt-2 text-status-critical">A cobrança falhou. As retentativas continuam automaticamente; se persistir, refaça a assinatura.</p>}
                {state.status === "ACTIVE" && !paidPeriod && <p className="mt-2 text-status-critical">Aguardando confirmação da renovação. Atualize para consultar o pagamento.</p>}
                {state.status === "CANCELLED" && (paidPeriod
                  ? <p className="mt-2 text-sm">Nenhuma cobrança nova será feita. Você mantém o acesso até {state.paid_through ? date(state.paid_through) : "o fim do período pago"} e pode voltar a assinar a qualquer momento — a cobrança só começa quando esse período terminar.</p>
                  : <p className="mt-2 text-sm">Assinatura encerrada e nenhuma cobrança foi feita. Escolha um plano abaixo para voltar a usar.</p>)}
                {state.pending_change && (
                  <p className="mt-2 text-sm">
                    Próximo plano: {state.pending_change.plan.name} — {money(state.pending_change.plan.amount_cents)} / {planPeriod(state.pending_change.plan)}. {pendingUpgrade ? "Benefícios liberados para toda a equipe; cobrança no novo valor a partir da próxima renovação." : pendingDowngrade ? "Redução aplicada após o fim do período atual." : "Benefícios atuais mantidos; nova cobrança a partir da próxima renovação."}
                  </p>
                )}
              </div>
              {canManage && snapshot.enabled && (state.subscription_id && state.status !== "CANCELLED" || state.checkout) && (
                <Button variant="outline" disabled={busy} onClick={() => setCancelOpen(true)}>
                  {state.subscription_id ? "Cancelar renovação" : "Encerrar checkout"}
                </Button>
              )}
            </div>

            {state.checkout && (
              <div className="mt-4 rounded-lg border border-status-pending bg-status-pending-soft p-4">
                <h3 className="flex items-center gap-2 font-semibold text-status-pending"><CreditCard className="h-5 w-5" />Falta concluir o pagamento</h3>
                <p className="mt-2">Plano solicitado: <strong>{state.checkout.plan.name}</strong> — {money(state.checkout.plan.amount_cents)} / {planPeriod(state.checkout.plan)}.</p>
                <p className="mt-1 text-sm">
                  A assinatura é liberada assim que o pagamento é confirmado — se você já pagou, basta atualizar.
                  {" "}Para escolher outro plano, clique nele abaixo: o checkout atual é substituído e nada é cobrado duas vezes.
                </p>
                <div className="mt-3 flex flex-wrap items-center gap-3">
                  {canManage
                    ? <Button disabled={busy} onClick={() => { const checkout = state.checkout; if (checkout) void select(checkout.plan).catch(() => undefined); }}>Continuar pagamento<ExternalLink className="h-4 w-4" /></Button>
                    : <p className="text-sm">Checkout em processamento. Peça a um administrador para continuar o pagamento.</p>}
                  {canManage && <Button variant="outline" onClick={() => planosRef.current?.scrollIntoView({ behavior: "smooth", block: "start" })}>Trocar de plano</Button>}
                </div>
              </div>
            )}

            {plan && pressure && (
              <div className="mt-6">
                <h3 className="text-sm font-medium text-muted-foreground">Uso do plano</h3>
                <div className="mt-3 grid gap-4 md:grid-cols-3">
                  {pressure.rows.map(row => {
                    const ratio = row.limit > 0 ? row.used / row.limit : 0;
                    const full = ratio >= 1;
                    const close = ratio >= 0.8 && !full;
                    return (
                      <div key={row.key} className="rounded-lg border border-border p-4">
                        <div className="flex items-center justify-between gap-2">
                          <p className="text-sm text-muted-foreground">{row.label}</p>
                          <p className="text-sm font-medium">{row.used} / {row.limit || "∞"}</p>
                        </div>
                        {row.limit > 0 && <Progress className="mt-3" value={Math.min(100, Math.round(ratio * 100))} />}
                        {full && <p className="mt-2 text-sm text-status-critical">Limite do plano atingido.</p>}
                        {close && <p className="mt-2 text-sm text-status-warning">{Math.round(ratio * 100)}% do limite usado.</p>}
                      </div>
                    );
                  })}
                </div>
                {upgradePlan && effectivePlan && !state.pending_change && pressure.tightest.ratio >= 0.8 && (
                  <div className="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-primary/30 bg-primary/5 p-4">
                    <p className="text-sm">
                      Você já usa {Math.round(pressure.tightest.ratio * 100)}% de {pressure.tightest.row.label.toLowerCase()} do {effectivePlan.name}.
                      O {upgradePlan.name} amplia para {upgradePlan.limits.leads.toLocaleString("pt-BR")} leads, {upgradePlan.limits.properties.toLocaleString("pt-BR")} imóveis e {upgradePlan.limits.users} usuários.
                    </p>
                    {canManage && (
                      <Button
                        disabled={busy || !snapshot.enabled}
                        onClick={() => confirmAction(
                          `Os benefícios do ${upgradePlan.name} serão liberados imediatamente para todos os membros. A cobrança de ${money(upgradePlan.amount_cents)} / ${planPeriod(upgradePlan)} começa na próxima renovação.`,
                          () => select(upgradePlan),
                          { title: `Fazer upgrade para ${upgradePlan.name}`, confirmLabel: "Liberar benefícios agora" },
                        )}
                      >
                        <ArrowUpRight className="mr-2 h-4 w-4" />Fazer upgrade
                      </Button>
                    )}
                  </div>
                )}
              </div>
            )}
          </section>

          <PaymentMethodPanel
            state={state}
            trialNote={changePlanCopy}
            canManage={canManage}
            enabled={snapshot.enabled}
            busy={busy}
            onChange={() => void changeMethod().catch(() => setMethodOpen(false))}
            onOpen={() => setMethodOpen(true)}
            dialogOpen={methodOpen}
            onCloseDialog={() => setMethodOpen(false)}
          />

          <section ref={planosRef} id="planos" className="scroll-mt-6">
            <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
              <div>
                <h2 className="text-xl font-semibold">Planos</h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  {checkoutTrialDays > 0
                    ? isTrialing
                      ? `Seu teste já está ativo: restam ${checkoutTrialDays} dias e eles continuam valendo quando você assinar.`
                      : checkoutTrialDays === (plan?.trial_days || 15)
                        ? `Todos os planos incluem ${checkoutTrialDays} dias grátis para testar com os seus dados.`
                        : `Restam ${checkoutTrialDays} dias do seu período de teste: eles continuam valendo na próxima assinatura.`
                    : "O período de teste já foi usado nesta conta. A partir de agora a assinatura é cobrada na contratação."}
                </p>
              </div>
              <div className="flex gap-2">
                <Button variant={cycle === "MONTHLY" ? "default" : "outline"} onClick={() => setCycle("MONTHLY")}>Mensal</Button>
                <Button variant={cycle === "ANNUALLY" ? "default" : "outline"} onClick={() => setCycle("ANNUALLY")}>
                  Anual <Badge className="ml-2" variant={cycle === "ANNUALLY" ? "secondary" : "success"}>-20%</Badge>
                </Button>
              </div>
            </div>
            <div className="grid gap-5 lg:grid-cols-3">
              {plans.map(item => (
                <PlanCard
                  key={`${item.tier}-${item.cycle}`}
                  plan={item}
                  trialDays={item.contact_only ? 0 : checkoutTrialDays}
                  monthlyPlan={item.cycle === "ANNUALLY" ? monthlyOf(item.tier) : undefined}
                  state={state}
                  entitled={entitled}
                  usage={snapshot.usage}
                  canManage={canManage}
                  enabled={snapshot.enabled}
                  busy={busy}
                  recommended={recommended?.tier === item.tier && recommended?.cycle === item.cycle}
                  onSelect={chosen => {
                    if (!entitled || !state.subscription_id || state.status === "CANCELLED") {
                      void select(chosen).catch(() => undefined);
                      return;
                    }
                    // A real downgrade is a lower tier; the annual discount of
                    // the same tier is not a downgrade.
                    if (plan && planRemovesBenefits(plan, chosen)) { setDowngrade(chosen); return; }
                    const isUpgrade = !!plan && planAddsBenefits(plan, chosen);
                    confirmAction(
                      isUpgrade
                        ? `Os benefícios do ${chosen.name} serão liberados imediatamente para todos os membros. A cobrança de ${money(chosen.amount_cents)} / ${planPeriod(chosen)} começa na próxima renovação.`
                        : `A mudança de cobrança para ${chosen.name}, por ${money(chosen.amount_cents)} / ${planPeriod(chosen)}, começa na próxima renovação.`,
                      () => select(chosen),
                      { title: isUpgrade ? `Fazer upgrade para ${chosen.name}` : "Confirmar mudança de plano", confirmLabel: isUpgrade ? "Liberar benefícios agora" : "Agendar mudança" },
                    );
                  }}
                />
              ))}
            </div>
          </section>

          <section className="rounded-xl border border-border bg-card p-6 shadow-sm">
            <h2 className="mb-4 flex items-center gap-2 text-lg font-semibold"><CreditCard className="h-5 w-5" />Histórico de Pagamentos</h2>
            {snapshot.payments.length === 0
              ? <p className="text-muted-foreground">{entitled ? "Nenhum pagamento ainda — seu período de teste está em andamento." : "Nenhum pagamento confirmado."}</p>
              : <div className="overflow-x-auto"><ResponsiveTable
              data={snapshot.payments}
              caption="Últimos 100 pagamentos confirmados da assinatura"
              getRowKey={(payment) => payment.id}
              columns={[
                { key: "id", header: "Pagamento", primary: true, cell: (payment) => payment.id },
                { key: "date", header: "Data", cell: (payment) => date(payment.paid_at) },
                { key: "amount", header: "Valor", cell: (payment) => money(payment.amount_cents) },
                {
                  key: "receipt",
                  header: "Comprovante",
                  cell: (payment) => (
                    <div className="flex flex-wrap items-center gap-3">
                      <Button variant="outline" size="sm" onClick={() => void openReceipt(payment.id)}>Ver comprovante</Button>
                      {safeSubscriptionURL(payment.receipt_url) && (
                        <a href={safeSubscriptionURL(payment.receipt_url)!} target="_blank" rel="noopener noreferrer" className="text-primary">Recibo do provedor</a>
                      )}
                    </div>
                  ),
                },
              ]}
            /><p className="mt-3 text-xs text-muted-foreground">Últimos 100 pagamentos confirmados.</p></div>}
          </section>
        </>}
      </div>
      {confirmationDialog}
      {snapshot && state && (
        <ChangePlanDialog
          open={!!downgrade}
          target={downgrade ?? undefined}
          current={state}
          usage={snapshot.usage}
          busy={busy}
          onCancel={() => setDowngrade(null)}
          onConfirm={chosen => { setDowngrade(null); void select(chosen).catch(() => undefined); }}
        />
      )}
      {snapshot && state && (
        <CancelSubscriptionDialog
          open={cancelOpen}
          state={state}
          retentionPlan={retentionPlan}
          busy={busy}
          onCancel={() => setCancelOpen(false)}
          onConfirm={reason => void cancel(reason).catch(() => setCancelOpen(false))}
          onDowngrade={chosen => { setCancelOpen(false); confirmAction(
            `Todos os membros mantêm os benefícios atuais até o fim do período atual${state.paid_through ? `, previsto para ${date(state.paid_through)}` : ""}. Depois, a redução para ${chosen.name} e a nova cobrança de ${money(chosen.amount_cents)} / ${planPeriod(chosen)} entram na próxima renovação.`,
            () => select(chosen),
            { title: "Confirmar redução de plano", confirmLabel: "Agendar redução" },
          ); }}
        />
      )}
    </div>
  );
}
