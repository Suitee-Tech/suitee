import {
  AlertCircle,
  CalendarDays,
  CheckCircle2,
  CreditCard,
  Landmark,
  ListTree,
  Lock,
  ReceiptText,
  ShieldAlert,
  WalletCards,
} from "lucide-react";
import { clsx } from "clsx";
import type { DealDTO, Proposal } from "../../../../shared/services/deal.service";
import type { SaleBillingPlanDraft } from "../../../../shared/services/saleBillingPlan.service";
import type { FirstChargeType } from "../../../../shared/rentals/types";
import { Button } from "../../../../shared/ui/button";
import { formatISODateBR, formatMoneyBR } from "../../../../shared/utils/format";
import { allocationTypeLabel } from "../../../../shared/lib/finance/allocationLabels";
import {
  billingMonths,
  daysBetweenInclusive,
  DEFAULT_LEASE_TERM_MONTHS,
  firstChargeCents,
  firstMonthProRata,
  lastMonthResidual,
  MAX_LEASE_TERM_MONTHS,
  MIN_LEASE_TERM_MONTHS,
} from "../../../../shared/lib/finance/rentalProration";

export interface RentalPreContractPlan {
  startDate: string;
  /** Término efetivo do contrato — dirige o residual do último aluguel. */
  endDate: string;
  /** Término calculado a partir de (início + prazo). */
  suggestedEndDate: string;
  /** Prazo do contrato em meses (padrão 30). */
  termMonths: number;
  firstChargeType: FirstChargeType;
  firstChargeDate: string;
  defaultDueDay: number;
  adminFeePct?: number;
  onStartDateChange: (value: string) => void;
  onEndDateChange: (value: string) => void;
  onTermMonthsChange: (value: number) => void;
  onFirstChargeTypeChange: (value: FirstChargeType) => void;
  onFirstChargeDateChange: (value: string) => void;
  onDefaultDueDayChange: (value: number) => void;
}

interface PreContractFinancialSetupProps {
  deal: DealDTO;
  latestProposal?: Proposal | null;
  saleDraft?: SaleBillingPlanDraft | null;
  saleDraftCurrent: boolean;
  rentalPlan?: RentalPreContractPlan;
  ready: boolean;
  onConfigureSale: () => void;
  onOpenFinancialPreview: () => void;
}

type AccountPreview = {
  code: string;
  label: string;
  group: string;
  description: string;
};

const SALE_ACCOUNTS: AccountPreview[] = [
  {
    code: "accounts_receivable",
    label: "Recebível do comprador",
    group: "Ativo",
    description: "Entrada, parcelas e saldo em dinheiro conforme o plano aprovado.",
  },
  {
    code: "owner_payable",
    label: "Proprietário a repassar",
    group: "Passivo de terceiros",
    description: "Valor econômico do vendedor, separado da receita da imobiliária.",
  },
  {
    code: "commission_payable",
    label: "Corretores a pagar",
    group: "Passivo de terceiros",
    description: "Obrigações calculadas pela divisão de comissão validada na operação.",
  },
  {
    code: "revenue",
    label: "Receita da imobiliária",
    group: "Resultado",
    description: "Parcela efetivamente retida após corretores, parceiros e royalties.",
  },
];

const RENT_ACCOUNTS: AccountPreview[] = [
  {
    code: "accounts_receivable",
    label: "Aluguel a receber",
    group: "Ativo",
    description: "Aluguel-base e itens cobrados do locatário em cada competência.",
  },
  {
    code: "owner_payable",
    label: "Proprietário a repassar",
    group: "Passivo de terceiros",
    description: "Saldo líquido após administração, intermediação e deduções válidas.",
  },
  {
    code: "commission_payable",
    label: "Corretagem a pagar",
    group: "Passivo de terceiros",
    description: "Intermediação financiada pelo primeiro aluguel e, se preciso, pelos seguintes.",
  },
  {
    code: "revenue",
    label: "Receita de administração",
    group: "Resultado",
    description: "Taxa da imobiliária apropriada mensalmente, sem inflar o aluguel do proprietário.",
  },
];

const isValidDate = (value: string) => Boolean(value) && !Number.isNaN(Date.parse(`${value}T00:00:00Z`));

export function isRentalPreContractPlanReady(plan: RentalPreContractPlan): boolean {
  if (!isValidDate(plan.startDate) || !isValidDate(plan.endDate) || !isValidDate(plan.firstChargeDate)) {
    return false;
  }
  if (plan.defaultDueDay < 1 || plan.defaultDueDay > 28) return false;
  if (
    !Number.isFinite(plan.termMonths) ||
    plan.termMonths < MIN_LEASE_TERM_MONTHS ||
    plan.termMonths > MAX_LEASE_TERM_MONTHS
  ) {
    return false;
  }
  const start = Date.parse(`${plan.startDate}T00:00:00Z`);
  const end = Date.parse(`${plan.endDate}T00:00:00Z`);
  const firstCharge = Date.parse(`${plan.firstChargeDate}T00:00:00Z`);
  return end >= start && firstCharge >= start;
}

/**
 * FinancialPlanRestricted — estado exibido na aba Assinatura para quem NÃO tem
 * acesso financeiro (ex.: corretor sem `finance:read`/`charges:write`).
 *
 * Antes, o painel "Plano financeiro antes do contrato" era renderizado para
 * qualquer perfil que enxergasse a aba: o usuário via o plano-base, a taxa de
 * administração, o mapa de contas e os botões de configuração de recebimento,
 * mesmo sem permissão — e as chamadas por trás (settings/draft) voltavam 403,
 * deixando um "Configuração pendente" que ele não tinha como resolver. O
 * feedback aqui diz o que está restrito, por que a minuta depende disso e a
 * quem pedir acesso.
 */
export function FinancialPlanRestricted() {
  return (
    <section
      aria-labelledby="financial-plan-restricted-title"
      className="mb-5 overflow-hidden rounded-2xl border border-border bg-card shadow-sm"
    >
      <div className="flex items-start gap-3 px-5 py-4">
        <div className="rounded-xl bg-status-warning-soft p-2.5 text-status-warning" aria-hidden="true">
          <ShieldAlert className="size-5" />
        </div>
        <div className="max-w-2xl">
          <div className="flex flex-wrap items-center gap-2">
            <h4 id="financial-plan-restricted-title" className="font-bold text-foreground">
              Plano financeiro restrito
            </h4>
            <span className="inline-flex items-center gap-1 rounded-full border border-border bg-muted px-2 py-0.5 text-xs font-bold uppercase tracking-wide text-muted-foreground">
              <Lock className="size-3" /> Área financeira
            </span>
          </div>
          <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
            Seu perfil não acompanha a configuração financeira desta negociação — forma de recebimento, taxa de
            administração, comissões e repasses. Esses dados ficam com o time financeiro.
          </p>
          <p className="mt-3 text-xs leading-relaxed text-muted-foreground">
            A minuta é liberada quando o plano-base é concluído pela área financeira. Se você precisa configurá-lo,
            solicite a permissão <span className="font-medium text-foreground">Financeiro</span> (ou{" "}
            <span className="font-medium text-foreground">Cobranças</span>) a um administrador da imobiliária.
          </p>
        </div>
      </div>
    </section>
  );
}

export function PreContractFinancialSetup({
  deal,
  latestProposal,
  saleDraft,
  saleDraftCurrent,
  rentalPlan,
  ready,
  onConfigureSale,
  onOpenFinancialPreview,
}: PreContractFinancialSetupProps) {
  const isSale = deal.type === "SALE";
  const accounts = isSale ? SALE_ACCOUNTS : RENT_ACCOUNTS;
  const amount = formatMoneyBR(deal.value || 0);
  const adminFee = rentalPlan?.adminFeePct ?? 0;
  const estimatedAdminRevenue = (deal.value || 0) * (adminFee / 100);
  const estimatedOwnerBase = Math.max(0, (deal.value || 0) - estimatedAdminRevenue);

  return (
    <section className="mb-5 overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
      <div className="flex flex-col gap-3 border-b border-border bg-muted/30 px-5 py-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <div className="rounded-xl bg-primary/10 p-2.5 text-primary">
            <WalletCards className="size-5" />
          </div>
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h4 className="font-bold text-foreground">Plano financeiro antes do contrato</h4>
              <span
                className={clsx(
                  "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-caption font-bold uppercase tracking-wide",
                  ready
                    ? "border-status-ok/30 bg-status-ok-soft text-status-ok"
                    : "border-status-warning/30 bg-status-warning-soft text-status-warning",
                )}
              >
                {ready ? <CheckCircle2 className="size-3" /> : <AlertCircle className="size-3" />}
                {ready ? "Pronto para a minuta" : "Configuração pendente"}
              </span>
            </div>
            <p className="mt-1 max-w-2xl text-xs leading-relaxed text-muted-foreground">
              Defina o recebimento que constará da operação e confira como o sistema separará ativos, receita própria
              e dinheiro de terceiros. Nenhum lançamento contábil é criado nesta etapa.
            </p>
          </div>
        </div>
        {isSale && (
          <Button variant="outline" size="sm" onClick={onOpenFinancialPreview} className="shrink-0 gap-1.5">
            <ReceiptText className="size-4" /> Ver projeção estimada
          </Button>
        )}
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
        <div className="space-y-5 p-5 xl:border-r xl:border-border">
          {isSale ? (
            <SalePlanSummary
              amount={amount}
              latestProposal={latestProposal}
              draft={saleDraft}
              draftCurrent={saleDraftCurrent}
              onConfigure={onConfigureSale}
            />
          ) : rentalPlan ? (
            <RentalPlanForm deal={deal} plan={rentalPlan} estimatedAdminRevenue={estimatedAdminRevenue} estimatedOwnerBase={estimatedOwnerBase} />
          ) : null}
        </div>

        <div className="bg-muted/15 p-5">
          <div className="mb-4 flex items-start gap-3">
            <div className="rounded-lg border border-border bg-card p-2 text-muted-foreground">
              <ListTree className="size-4" />
            </div>
            <div>
              <h5 className="text-sm font-bold text-foreground">Mapa de lançamentos e contas</h5>
              <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">
                Classificação gerencial automática aplicada quando o fato financeiro for materializado.
              </p>
            </div>
          </div>

          <div className="space-y-2.5">
            {accounts.map((account) => (
              <div key={account.code} className="rounded-xl border border-border bg-card p-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <p className="text-xs font-bold text-foreground">{account.label}</p>
                  <span className="rounded-full bg-muted px-2 py-0.5 text-caption font-semibold text-muted-foreground">
                    {account.group}
                  </span>
                </div>
                <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">{account.description}</p>
                <code className="mt-2 block text-caption font-semibold text-primary-700">{allocationTypeLabel(account.code)}</code>
              </div>
            ))}
          </div>

          <p className="mt-4 text-[11px] leading-relaxed text-muted-foreground">
            O código da conta não altera o valor contratual. Ele preserva a origem econômica para conciliação,
            auditoria e eventual reclassificação contábil posterior.
          </p>
        </div>
      </div>
    </section>
  );
}

function SalePlanSummary({
  amount,
  latestProposal,
  draft,
  draftCurrent,
  onConfigure,
}: {
  amount: string;
  latestProposal?: Proposal | null;
  draft?: SaleBillingPlanDraft | null;
  draftCurrent: boolean;
  onConfigure: () => void;
}) {
  const draftLabel = draft?.mode === "INSTALLMENTS" ? `${draft.installment_count} parcelas` : "Pagamento à vista";

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Forma de recebimento da venda</p>
          <h5 className="mt-1 text-lg font-bold text-foreground">{draft ? draftLabel : "Ainda não definida"}</h5>
          <p className="mt-1 text-xs text-muted-foreground">
            Valor negociado: <strong className="text-foreground">{amount}</strong>
            {latestProposal?.payment_method ? ` · Proposta: ${latestProposal.payment_method}` : ""}
          </p>
        </div>
        <Button onClick={onConfigure} size="sm" className="gap-1.5">
          <CreditCard className="size-4" /> {draft ? "Revisar recebimento" : "Definir recebimento"}
        </Button>
      </div>

      {!draft ? (
        <InlineNotice tone="warning" title="A proposta não substitui o plano operacional">
          Registre entrada, parcelas, saldo e eventuais bens em dação antes de gerar a minuta. Sem essa escolha, o
          financeiro não consegue materializar os recebíveis da venda na assinatura.
        </InlineNotice>
      ) : !draftCurrent ? (
        <InlineNotice tone="critical" title="Plano desatualizado">
          O valor do negócio mudou depois do último rascunho. Revise a composição para que ela volte a fechar com o
          total contratado.
        </InlineNotice>
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <Metric label="Total do plano" value={formatMoneyBR(draft.total_amount_cents / 100)} />
          <Metric label="Primeiro vencimento" value={formatISODateBR(draft.first_due_date.slice(0, 10))} />
          <Metric
            label="Entrada"
            value={draft.down_payment_cents > 0 ? formatMoneyBR(draft.down_payment_cents / 100) : "Sem entrada"}
          />
        </div>
      )}
    </div>
  );
}

function RentalPlanForm({
  deal,
  plan,
  estimatedAdminRevenue,
  estimatedOwnerBase,
}: {
  deal: DealDTO;
  plan: RentalPreContractPlan;
  estimatedAdminRevenue: number;
  estimatedOwnerBase: number;
}) {
  const ready = isRentalPreContractPlanReady(plan);

  return (
    <div className="space-y-5">
      <div>
        <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Plano-base da locação</p>
        <h5 className="mt-1 text-lg font-bold text-foreground">Calendário e primeiro aluguel</h5>
        <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
          O aluguel de {formatMoneyBR(deal.value || 0)} será a base recorrente. Itens fixos e contas variáveis poderão
          ser vinculados depois, sem misturar receita da imobiliária com o saldo do proprietário.
        </p>
      </div>

      <div className="grid grid-cols-1 gap-x-4 gap-y-3 sm:grid-cols-3">
        <Field label="Início do contrato" hint="Define a primeira competência e o cálculo proporcional.">
          <input
            type="date"
            value={plan.startDate}
            onChange={(event) => plan.onStartDateChange(event.target.value)}
            className="w-full rounded-xl border border-border bg-card px-3 py-2.5 text-sm outline-none transition-all focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>
        <Field
          label="Prazo do contrato (meses)"
          hint={`Padrão de ${DEFAULT_LEASE_TERM_MONTHS} meses. O término é calculado a partir do início.`}
        >
          <input
            type="number"
            min={MIN_LEASE_TERM_MONTHS}
            max={MAX_LEASE_TERM_MONTHS}
            inputMode="numeric"
            value={plan.termMonths}
            onChange={(event) => plan.onTermMonthsChange(Number(event.target.value))}
            className="w-full rounded-xl border border-border bg-card px-3 py-2.5 text-sm outline-none transition-all focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>
        <Field label="Término do contrato" hint={endDateHint(plan)}>
          <input
            type="date"
            value={plan.endDate}
            onChange={(event) => plan.onEndDateChange(event.target.value)}
            className="w-full rounded-xl border border-border bg-card px-3 py-2.5 text-sm outline-none transition-all focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>
      </div>

      <RentalChargeCalendarPreview plan={plan} monthlyAmount={deal.value || 0} />

      <div>
        <p className="mb-2 text-xs font-bold text-foreground">Tratamento do primeiro aluguel</p>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <ChoiceCard
            selected={plan.firstChargeType === "same_month_pro_rata"}
            title="Pró-rata no primeiro mês"
            description="Cobra somente os dias ocupados; intermediação insuficiente continua no mês seguinte."
            onClick={() => plan.onFirstChargeTypeChange("same_month_pro_rata")}
          />
          <ChoiceCard
            selected={plan.firstChargeType === "next_month_accumulated"}
            title="Acumular no próximo mês"
            description="Leva o proporcional para a cobrança seguinte, junto do primeiro mês integral."
            onClick={() => plan.onFirstChargeTypeChange("next_month_accumulated")}
          />
        </div>
      </div>

      <div className="grid grid-cols-1 gap-x-4 gap-y-3 sm:grid-cols-2">
        <Field label="Primeiro vencimento" hint="Nunca pode ser anterior ao início da locação.">
          <input
            type="date"
            value={plan.firstChargeDate}
            onChange={(event) => plan.onFirstChargeDateChange(event.target.value)}
            className="w-full rounded-xl border border-border bg-card px-3 py-2.5 text-sm outline-none transition-all focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>
        <Field label="Dia padrão dos próximos vencimentos" hint="Use de 1 a 28 para existir em todos os meses.">
          <input
            type="number"
            min={1}
            max={28}
            value={plan.defaultDueDay}
            onChange={(event) => plan.onDefaultDueDayChange(Number(event.target.value))}
            className="w-full rounded-xl border border-border bg-card px-3 py-2.5 text-sm outline-none transition-all focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <Metric label="Aluguel-base mensal" value={formatMoneyBR(deal.value || 0)} icon={CalendarDays} />
        <Metric
          label={`Receita adm. estimada${plan.adminFeePct ? ` (${plan.adminFeePct.toLocaleString("pt-BR")}%)` : ""}`}
          value={plan.adminFeePct ? formatMoneyBR(estimatedAdminRevenue) : "Conforme parâmetros"}
          icon={Landmark}
        />
        <Metric
          label="Base mensal do proprietário"
          value={plan.adminFeePct ? formatMoneyBR(estimatedOwnerBase) : "Apurada na liquidação"}
          icon={WalletCards}
        />
      </div>

      {!ready && (
        <InlineNotice tone="critical" title="Revise as datas do plano-base">
          A vigência deve ser válida, o prazo precisa ficar entre {MIN_LEASE_TERM_MONTHS} e {MAX_LEASE_TERM_MONTHS} meses,
          o primeiro vencimento não pode anteceder o início e o dia padrão deve ficar entre 1 e 28.
        </InlineNotice>
      )}
    </div>
  );
}

/**
 * Dica do campo "Término": explica de onde veio a data.
 *
 * Quando o término bate com (início + prazo) é só confirmar o cálculo; quando o
 * operador escolhe outra data, a dica mostra quantos dias sobraram (ou faltaram)
 * em relação ao prazo — são esses dias que o residual do último aluguel cobra.
 */
function endDateHint(plan: RentalPreContractPlan): string {
  const computed = plan.suggestedEndDate;
  if (!plan.endDate) return computed ? `Calculado pelo prazo: ${formatISODateBR(computed)}` : "Informe a vigência final.";
  if (computed && plan.endDate === computed) {
    return `${plan.termMonths} meses a partir do início: ${formatISODateBR(computed)}`;
  }
  if (computed) {
    const diffDays = signedDaysBetween(computed, plan.endDate);
    if (diffDays > 0) {
      return `Término ${diffDays} dia(s) depois do prazo de ${plan.termMonths} meses — o último aluguel será proporcional.`;
    }
    if (diffDays < 0) {
      return `Término ${Math.abs(diffDays)} dia(s) antes do prazo de ${plan.termMonths} meses — o último aluguel será proporcional.`;
    }
  }
  return "Data escolhida manualmente; o prazo foi ajustado para os meses inteiros.";
}

function signedDaysBetween(fromISO: string, toISO: string): number {
  const forward = daysBetweenInclusive(fromISO, toISO);
  if (forward > 0) return forward - 1;
  const backward = daysBetweenInclusive(toISO, fromISO);
  return backward > 0 ? -(backward - 1) : 0;
}

/**
 * RentalChargeCalendarPreview — o que o contrato vai cobrar nas pontas.
 *
 * O backend já calcula o primeiro mês (pró-rata) e o último (residual do
 * término) na geração das cobranças; esta prévia mostra os MESMOS números antes
 * de gerar a minuta, com a base em dias, para o operador perceber o efeito de
 * mudar prazo/início/término — e conferir que o residual do fim não virou um mês
 * cheio nem sumiu.
 */
function RentalChargeCalendarPreview({ plan, monthlyAmount }: { plan: RentalPreContractPlan; monthlyAmount: number }) {
  const monthlyCents = Math.round((monthlyAmount || 0) * 100);
  const startValid = isValidDate(plan.startDate);
  const endValid = isValidDate(plan.endDate);
  if (!startValid || !endValid || monthlyCents <= 0) return null;

  const first = firstMonthProRata(plan.startDate, monthlyCents);
  const residual = lastMonthResidual(plan.endDate, monthlyCents);
  const firstCharge = firstChargeCents(plan.startDate, plan.endDate, monthlyCents, plan.firstChargeType);
  const isSameMonth = plan.startDate.slice(0, 7) === plan.endDate.slice(0, 7);
  const months = billingMonths(plan.startDate, plan.endDate);
  const contractDays = daysBetweenInclusive(plan.startDate, plan.endDate);
  const accumulated = plan.firstChargeType === "next_month_accumulated";

  return (
    <div className="rounded-2xl border border-border bg-muted/20 p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Calendário de cobranças do contrato</p>
        <span className="rounded-full bg-card px-2 py-0.5 text-xs font-semibold text-muted-foreground">
          {months} mês(es) de calendário · {contractDays} dia(s) de contrato
        </span>
      </div>

      <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-3">
        <Metric
          label={accumulated ? "1ª cobrança (pró-rata + mês seguinte)" : "1º aluguel (pró-rata)"}
          value={formatMoneyBR(fromCents(firstCharge))}
          icon={CalendarDays}
        />
        <Metric
          label="Último aluguel (residual do término)"
          value={formatMoneyBR(fromCents(residual.cents))}
          icon={ReceiptText}
        />
        <Metric
          label="Aluguel mensal cheio"
          value={formatMoneyBR(monthlyAmount || 0)}
          icon={WalletCards}
        />
      </div>

      <ul className="mt-3 space-y-1 text-xs leading-relaxed text-muted-foreground">
        <li>
          Pró-rata: <strong className="text-foreground">{first.days} de {first.daysInMonth} dia(s)</strong> do mês inicial
          {accumulated ? " — somado ao mês seguinte, que também entra proporcional se o contrato terminar dentro dele." : "."}
        </li>
        <li>
          Residual: <strong className="text-foreground">{residual.days} de {residual.daysInMonth} dia(s)</strong> do mês final
          {residual.days === residual.daysInMonth ? " (mês cheio — o término cai no último dia)." : isSameMonth ? " (contrato dentro de um único mês)." : "."}
        </li>
        <li>
          O cálculo usa os dias reais de cada mês (28 a 31) — o mesmo critério que o financeiro aplica ao gerar as
          cobranças na assinatura.
        </li>
      </ul>
    </div>
  );
}

function fromCents(cents: number): number {
  return (cents || 0) / 100;
}

function Field({ label, hint, children }: { label: string; hint: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-xs font-bold text-foreground">{label}</span>
      {children}
      <span className="mt-1 block min-h-8 text-caption leading-4 text-muted-foreground">{hint}</span>
    </label>
  );
}

function ChoiceCard({
  selected,
  title,
  description,
  onClick,
}: {
  selected: boolean;
  title: string;
  description: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={selected}
      className={clsx(
        "rounded-xl border p-3 text-left transition-colors",
        selected ? "border-primary bg-primary/10" : "border-border bg-card hover:bg-muted/40",
      )}
    >
      <span className="flex items-center gap-2 text-xs font-bold text-foreground">
        <span className={clsx("size-3 rounded-full border-2", selected ? "border-primary bg-primary" : "border-muted-foreground/50")} />
        {title}
      </span>
      <span className="mt-1.5 block text-caption leading-4 text-muted-foreground">{description}</span>
    </button>
  );
}

function Metric({
  label,
  value,
  icon: Icon,
}: {
  label: string;
  value: string;
  icon?: typeof CalendarDays;
}) {
  return (
    <div className="rounded-xl border border-border bg-muted/25 p-3">
      <p className="flex items-center gap-1.5 text-caption font-bold uppercase tracking-wide text-muted-foreground">
        {Icon && <Icon className="size-3" />}
        {label}
      </p>
      <p className="mt-1 text-sm font-bold tabular-nums text-foreground">{value}</p>
    </div>
  );
}

function InlineNotice({
  tone,
  title,
  children,
}: {
  tone: "warning" | "critical";
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div
      className={clsx(
        "flex items-start gap-3 rounded-xl border p-3",
        tone === "warning"
          ? "border-status-warning/30 bg-status-warning-soft text-status-warning"
          : "border-status-critical/30 bg-status-critical-soft text-status-critical",
      )}
    >
      <AlertCircle className="mt-0.5 size-4 shrink-0" />
      <div>
        <p className="text-xs font-bold">{title}</p>
        <p className="mt-1 text-[11px] leading-relaxed">{children}</p>
      </div>
    </div>
  );
}
