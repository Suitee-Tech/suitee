import { useMemo, useState } from "react";
import {
  AlertTriangle,
  ArrowDownToLine,
  Briefcase,
  Building2,
  ChevronDown,
  ChevronRight,
  Crown,
  FileText,
  Home,
  Layers3,
  ReceiptText,
  ShieldCheck,
  Wallet,
} from "lucide-react";
import { clsx } from "clsx";

import { formatMoneyBR } from "../../utils/format";
import { lancamentoSituationLabel, lancamentoSituationVariant } from "../../lib/lancamentos/chargeDue";
import type { EconomicOperation, RentalChargeComponent, RentalEconomicSummary } from "../../lib/lancamentos/presentation";
import type { Lancamento, LancamentoKind, LancamentoStatus } from "../../lib/lancamentos/types";
import { RentalFundingProgressCard } from "./RentalFundingProgressCard";

/** Mesmos estados "em aberto" da barra lateral de lançamentos. */
const OPEN_STATUSES = new Set<LancamentoStatus>(["pendente", "aprovado", "parcial", "vencido"]);

interface EconomicOperationsTableProps {
  operations: EconomicOperation[];
  onSelect: (lancamento: Lancamento) => void;
  onOpenRentalPlan?: (contractId: string) => void;
  isLoading?: boolean;
  /** `false` quando a lista já está dentro de uma seção com cabeçalho próprio. */
  showSummary?: boolean;
}

const KIND_META: Record<LancamentoKind, { label: string; icon: typeof FileText }> = {
  charge: { label: "Cobrança", icon: FileText },
  commission: { label: "Comissão", icon: Briefcase },
  transfer: { label: "Repasse", icon: Home },
  royalty: { label: "Parceria", icon: Crown },
  transaction: { label: "Caixa", icon: Wallet },
};

export function EconomicOperationsTable({
  operations,
  onSelect,
  onOpenRentalPlan,
  isLoading,
  showSummary = true,
}: EconomicOperationsTableProps) {
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(() => new Set());
  const itemCount = useMemo(() => operations.reduce((total, operation) => total + operation.items.length, 0), [operations]);

  if (isLoading && operations.length === 0) {
    return <div className="rounded-xl border border-border bg-card p-8 text-center text-sm text-muted-foreground">Carregando operações…</div>;
  }

  if (operations.length === 0) {
    return (
      <div className="rounded-xl border border-dashed border-border bg-card p-12 text-center">
        <p className="text-sm font-bold text-foreground">Nenhuma operação encontrada</p>
        <p className="mt-1 text-xs text-muted-foreground">Ajuste os filtros ou aguarde a próxima sincronização.</p>
      </div>
    );
  }

  const toggle = (key: string) => {
    setExpandedKeys((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  return (
    <div className="space-y-3">
      {showSummary && (
        <div className="flex flex-wrap items-center justify-between gap-2 px-1 text-xs text-muted-foreground">
          <span>{operations.length} {operations.length === 1 ? "operação econômica" : "operações econômicas"}</span>
          <span>{itemCount} linhas auditáveis · valores vinculados não devem ser somados entre si</span>
        </div>
      )}
      {operations.map((operation) => {
        const expanded = operation.items.length === 1 || expandedKeys.has(operation.key);
        const hasOverdue = operation.items.some((item) => item.status === "vencido");
        const rental = operation.breakdown?.rental;
        return (
          <article key={operation.key} className="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
            <button
              type="button"
              onClick={() => operation.items.length === 1 ? onSelect(operation.primary) : toggle(operation.key)}
              className="w-full p-4 text-left transition-colors hover:bg-muted/25 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary"
              aria-expanded={operation.items.length > 1 ? expanded : undefined}
            >
              <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                <div className="flex min-w-0 items-start gap-3">
                  <div className="mt-0.5 rounded-xl bg-primary-50 p-2 text-primary-700">
                    <Layers3 className="h-5 w-5" aria-hidden="true" />
                  </div>
                  <div className="min-w-0">
                    <p className="truncate text-sm font-bold text-foreground" title={operation.title}>{operation.title}</p>
                    <p className="mt-0.5 truncate text-xs text-muted-foreground" title={operation.subtitle}>{operation.subtitle || "Origem operacional identificada"}</p>
                    <div className="mt-2 flex flex-wrap items-center gap-1.5">
                      <span className="rounded-full border border-border bg-muted/40 px-2 py-0.5 text-[11px] font-semibold text-muted-foreground">
                        {operation.items.length} {operation.items.length === 1 ? "linha" : "linhas"}
                      </span>
                      {operation.divergenceCount > 0 ? (
                        <span className="inline-flex items-center gap-1 rounded-full border border-status-warning/30 bg-status-warning-soft px-2 py-0.5 text-[11px] font-bold text-status-warning">
                          <AlertTriangle className="h-3 w-3" aria-hidden="true" /> {operation.divergenceCount} para revisar
                        </span>
                      ) : hasOverdue ? (
                        <span className="rounded-full border border-status-critical/30 bg-status-critical-soft px-2 py-0.5 text-[11px] font-bold text-status-critical">Vencida</span>
                      ) : operation.openCount > 0 ? (
                        <span className="rounded-full border border-status-info/30 bg-status-info-soft px-2 py-0.5 text-[11px] font-bold text-status-info">
                          {operation.openCount} {operation.openCount === 1 ? "pendência" : "pendências"}
                        </span>
                      ) : (
                        <span className="rounded-full border border-status-ok/30 bg-status-ok-soft px-2 py-0.5 text-[11px] font-bold text-status-ok">Concluída</span>
                      )}
                    </div>
                  </div>
                </div>

                <div className="flex shrink-0 items-center justify-between gap-4 lg:justify-end">
                  <div className="text-left lg:text-right">
                    <p className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
                      {rental ? rentalAmountLabel(rental) : operation.breakdown ? "Valor da operação" : "Valor do lançamento"}
                    </p>
                    <p className="text-lg font-bold tabular-nums text-foreground">{formatMoneyBR(operation.amount, { withSymbol: true })}</p>
                    {/* A comissão de intermediação não entra no valor do aluguel:
                        ela é uma obrigação à parte, liberada pelos recebimentos.
                        Sem esta linha o cartão some com o valor que o filtro
                        "a pagar" acabou de trazer para a tela. */}
                    {rental && rental.intermediationOutstanding > 0 && (
                      <p className="mt-0.5 text-[11px] font-bold text-status-warning">
                        + {formatMoneyBR(rental.intermediationOutstanding, { withSymbol: true })} de comissão a liberar
                      </p>
                    )}
                  </div>
                  {operation.items.length > 1 ? (
                    <ChevronDown className={clsx("h-5 w-5 text-muted-foreground transition-transform", expanded && "rotate-180")} aria-hidden="true" />
                  ) : (
                    <ChevronRight className="h-5 w-5 text-muted-foreground" aria-hidden="true" />
                  )}
                </div>
              </div>

              {operation.breakdown && !rental && (
                <div className="mt-4 grid grid-cols-2 gap-x-4 gap-y-3 border-t border-border pt-3 sm:grid-cols-4">
                  <BreakdownValue label="Ao proprietário" value={operation.breakdown.ownerPayable} />
                  <BreakdownValue label="Comissões a pagar" value={operation.breakdown.commissionPayable} />
                  <BreakdownValue label="Parceiros" value={operation.breakdown.partnerPayable} />
                  <BreakdownValue label="Receita bruta de intermediação" value={operation.breakdown.agencyRevenue} />
                  {operation.breakdown.agencyNet !== undefined && <BreakdownValue label="Saldo retido pela imobiliária" value={operation.breakdown.agencyNet} accent />}
                </div>
              )}
            </button>

            {rental && (
              <RentalOperationSummary
                summary={rental}
                items={operation.items}
                contractId={operation.primary.presentation?.contract_id}
                onOpenRentalPlan={onOpenRentalPlan}
                onSelect={onSelect}
              />
            )}

            {expanded && operation.items.length > 1 && (
              <div className="border-t border-border bg-muted/15 px-3 py-2 sm:px-4">
                <p className="px-2 pb-2 pt-1 text-[11px] font-bold uppercase tracking-wide text-muted-foreground">Linhas vinculadas</p>
                <div className="divide-y divide-border overflow-hidden rounded-lg border border-border bg-card">
                  {operation.items.map((item) => <OperationLine key={`${item.kind}:${item.id}`} item={item} onSelect={onSelect} />)}
                </div>
              </div>
            )}
          </article>
        );
      })}
    </div>
  );
}

function BreakdownValue({ label, value, accent }: { label: string; value: number; accent?: boolean }) {
  return (
    <div className="min-w-0">
      <p className="truncate text-[11px] text-muted-foreground">{label}</p>
      <p className={clsx("truncate text-sm font-bold tabular-nums", accent ? "text-status-ok" : "text-foreground")}>
        {formatMoneyBR(value, { withSymbol: true })}
      </p>
    </div>
  );
}

function OperationLine({ item, onSelect }: { item: Lancamento; onSelect: (item: Lancamento) => void }) {
  const kind = KIND_META[item.kind];
  const Icon = kind.icon;
  // Cobrança usa a régua de vencimento do backend ("Vence hoje" não é atraso);
  // comissão/repasse/parceria/caixa seguem o badge canônico do status.
  const variant = lancamentoSituationVariant(item);
  const situation = lancamentoSituationLabel(item);
  const meaning = operationLineMeaning(item);
  return (
    <button
      type="button"
      onClick={() => onSelect(item)}
      className="grid w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-3 px-3 py-3 text-left transition-colors hover:bg-muted/30 sm:grid-cols-[210px_minmax(0,1fr)_140px_130px_auto]"
    >
      <span className="min-w-0">
        <span className="inline-flex items-center gap-1.5 text-xs font-bold text-foreground">
          <Icon className="h-3.5 w-3.5 text-primary-600" aria-hidden="true" /> {kind.label}
        </span>
        <span className="mt-0.5 block truncate text-[11px] text-muted-foreground" title={meaning}>{meaning}</span>
      </span>
      <span className="hidden min-w-0 truncate text-sm text-foreground sm:block" title={item.counterparty.label}>{item.counterparty.label}</span>
      <span className="text-right text-sm font-bold tabular-nums text-foreground">{formatMoneyBR(item.amount, { withSymbol: true })}</span>
      <span className={clsx("hidden justify-self-start rounded-md border px-2 py-0.5 text-xs font-semibold sm:inline-flex", variant.bg, variant.text, variant.border)}>
        {situation}
      </span>
      <ChevronRight className="hidden h-4 w-4 text-muted-foreground sm:block" aria-hidden="true" />
      <span className="col-span-2 truncate text-xs text-muted-foreground sm:hidden">{item.counterparty.label} · {situation}</span>
    </button>
  );
}

/**
 * Linha de obrigação dentro do cartão de locação. Não repete o texto genérico
 * da lista de linhas vinculadas: aqui a informação útil é *quando* a obrigação
 * pode ser paga, que é exatamente o que o valor zerado escondia.
 */
function RentalObligationRow({ item, onSelect }: { item: Lancamento; onSelect: (item: Lancamento) => void }) {
  const kind = KIND_META[item.kind];
  const Icon = kind.icon;
  const variant = lancamentoSituationVariant(item);
  const situation = lancamentoSituationLabel(item);
  return (
    <button
      type="button"
      onClick={() => onSelect(item)}
      className="grid w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-3 px-3 py-3 text-left transition-colors hover:bg-muted/30 sm:grid-cols-[210px_minmax(0,1fr)_140px_130px_auto]"
    >
      <span className="min-w-0">
        <span className="inline-flex items-center gap-1.5 text-xs font-bold text-foreground">
          <Icon className="h-3.5 w-3.5 text-primary-600" aria-hidden="true" /> {kind.label}
        </span>
      </span>
      <span className="hidden min-w-0 truncate text-sm text-foreground sm:block" title={item.counterparty.label}>
        {item.counterparty.label}
        <span className="mt-0.5 block truncate text-[11px] text-muted-foreground">{obligationPaymentCondition(item)}</span>
      </span>
      <span className="text-right text-sm font-bold tabular-nums text-foreground">{formatMoneyBR(item.amount, { withSymbol: true })}</span>
      <span className={clsx("hidden justify-self-start rounded-md border px-2 py-0.5 text-xs font-semibold sm:inline-flex", variant.bg, variant.text, variant.border)}>
        {situation}
      </span>
      <ChevronRight className="hidden h-4 w-4 text-muted-foreground sm:block" aria-hidden="true" />
      <span className="col-span-2 truncate text-xs text-muted-foreground sm:hidden">
        {item.counterparty.label} · {obligationPaymentCondition(item)} · {situation}
      </span>
    </button>
  );
}

/** Condição de pagamento da obrigação, dita em uma linha. */
function obligationPaymentCondition(item: Lancamento): string {
  if (item.kind === "commission") {
    const required = item.kind_data.funding_required_cents ?? Math.round(item.amount * 100);
    const funded = Math.min(required, Math.max(0, item.kind_data.funded_amount_cents ?? 0));
    if (required <= 0) return "Paga conforme o contrato";
    if (funded >= required) return "Liberada para pagamento";
    if (funded > 0) {
      return `Liberado ${formatMoneyBR(funded / 100, { withSymbol: true })} de ${formatMoneyBR(required / 100, { withSymbol: true })} · segue com os aluguéis`;
    }
    return "Nada liberado ainda · depende dos aluguéis recebidos";
  }
  if (item.kind === "royalty") return "Obrigação financeira com parceiro";
  return operationLineMeaning(item);
}

function operationLineMeaning(item: Lancamento): string {
  if (item.kind === "charge") return "Cobrança ao inquilino · compõe o total";
  if (item.kind === "commission") return "Obrigação contratual · não somar como nova saída";
  if (item.kind === "transfer") return "Repasse financeiro ao proprietário";
  if (item.kind === "royalty") return "Obrigação financeira com parceiro";
  if (item.kind_data.direction === "entrada") return "Movimento financeiro · entrada";
  if (item.kind_data.transaction_type === "comissao") return "Espelho contábil da comissão · não somar";
  return "Movimento financeiro · saída";
}

/** Rótulo do valor no cabeçalho, sempre ancorado no que o número representa. */
function rentalAmountLabel(rental: RentalEconomicSummary): string {
  if (rental.receivedTotal > 0) return "Recebido do inquilino";
  if (rental.chargedTotal > 0) return "Valor lançado";
  return "Sem aluguel nesta competência";
}

function RentalOperationSummary({
  summary,
  items,
  contractId,
  onOpenRentalPlan,
  onSelect,
}: {
  summary: RentalEconomicSummary;
  items: Lancamento[];
  contractId?: string;
  onOpenRentalPlan?: (contractId: string) => void;
  onSelect: (lancamento: Lancamento) => void;
}) {
  const hasComposition = summary.components.length > 0;
  const extraComponents = summary.components.filter((component) => !component.isBaseRent);
  const isUnlinked = summary.allocationState === "unlinked";
  // A entrada de locação só existe quando há cobrança ou recebimento. Uma
  // operação que reúne apenas a comissão vinculada mostraria três zeros sem
  // dizer nada — melhor explicar por que o cartão está aqui.
  const hasRentalMovement = summary.chargedTotal > 0 || summary.receivedTotal > 0;
  const payableObligations = items.filter((item) => item.kind === "commission" || item.kind === "royalty");
  const fundingEntries = items.flatMap((item) => (item.kind === "commission" ? item.kind_data.funding_entries ?? [] : []));

  return (
    <div className="border-t border-border bg-muted/10 p-4">
      <div className="grid gap-3 xl:grid-cols-[0.9fr_1.35fr]">
        <section className="rounded-xl border border-border bg-card p-3.5">
          <div className="flex items-center gap-2">
            <ReceiptText className="h-4 w-4 text-primary-600" aria-hidden="true" />
            <h3 className="text-xs font-bold text-foreground">
              {hasComposition ? "Composição cobrada do inquilino" : hasRentalMovement ? "Entrada de locação" : "Aluguel nesta competência"}
            </h3>
          </div>
          {hasComposition ? (
            <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4">
              <SummaryValue label="Aluguel base" value={summary.baseRent} />
              <SummaryValue label="Encargos no aluguel" value={summary.includedCharges} />
              <SummaryValue label="Cobranças separadas" value={summary.separateCharges} />
              <SummaryValue label="Total desta competência" value={summary.chargedTotal} strong />
            </div>
          ) : hasRentalMovement ? (
            <div className="mt-3 grid grid-cols-3 gap-3">
              <SummaryValue label="Valor lançado" value={summary.chargedTotal} strong />
              <SummaryValue label="Recebido" value={summary.receivedTotal} />
              <SummaryValue label="Em aberto" value={summary.openTotal} />
            </div>
          ) : (
            <div className="mt-3 rounded-lg border border-dashed border-border bg-muted/30 px-3 py-2.5">
              <p className="text-xs font-bold text-foreground">Nenhum aluguel cobrado ou recebido nesta competência</p>
              <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">
                {payableObligations.length > 0
                  ? "Esta operação está na lista pela obrigação de intermediação vinculada ao contrato — não por uma cobrança ao inquilino."
                  : "Ainda não há cobrança vinculada a esta operação."}
              </p>
            </div>
          )}
        </section>

        <section className="rounded-xl border border-border bg-card p-3.5">
          <div className="flex items-center gap-2">
            <Building2 className="h-4 w-4 text-primary-600" aria-hidden="true" />
            <h3 className="text-xs font-bold text-foreground">Destinos do valor</h3>
            <span className="ml-auto text-caption font-semibold uppercase tracking-wide text-muted-foreground">
              {allocationStateLabel(summary.allocationState)}
            </span>
          </div>
          <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4">
            <SummaryValue label="Repasse disponível" value={summary.availableToOwner} strong />
            <SummaryValue label="Reservado p/ despesas" value={summary.reservedForExpenses} />
            <SummaryValue label="Taxa de administração" value={summary.adminFee} />
            <SummaryValue label="Receita reconhecida" value={summary.agencyRevenue} />
            <SummaryValue label="Despesas da imobiliária" value={summary.agencyExpenses} />
            <SummaryValue label="Saldo líquido imobiliária" value={summary.agencyNet} accent strong />
          </div>
        </section>
      </div>

      {summary.intermediationRequired > 0 && (
        <div className="mt-3 space-y-2">
          <div className="flex items-start gap-2">
            <ShieldCheck className="mt-0.5 size-3.5 shrink-0 text-status-info" aria-hidden="true" />
            <div>
              <p className="text-[11px] font-bold text-foreground">Intermediação é uma obrigação vinculada — não é uma segunda entrada</p>
              <p className="text-[11px] leading-relaxed text-muted-foreground">
                O valor integral permanece auditável; somente a parcela coberta por aluguéis recebidos pode seguir para pagamento.
              </p>
            </div>
          </div>
          <RentalFundingProgressCard
            requiredCents={Math.round(summary.intermediationRequired * 100)}
            fundedCents={Math.round(summary.intermediationFunded * 100)}
            entries={fundingEntries}
            hasReceivedRent={summary.receivedTotal > 0}
          />
        </div>
      )}

      {payableObligations.length > 0 && (
        <div className="mt-3 overflow-hidden rounded-xl border border-border bg-card">
          <div className="flex items-center justify-between gap-3 border-b border-border px-3.5 py-2.5">
            <p className="text-xs font-bold text-foreground">Obrigações desta operação</p>
            <p className="text-[11px] text-muted-foreground">
              {payableObligations.filter((item) => OPEN_STATUSES.has(item.status)).length} em aberto ·{" "}
              {formatMoneyBR(payableObligations.reduce((sum, item) => sum + item.amount, 0), { withSymbol: true })}
            </p>
          </div>
          <div className="divide-y divide-border">
            {payableObligations.map((item) => (
              <RentalObligationRow key={`${item.kind}:${item.id}`} item={item} onSelect={onSelect} />
            ))}
          </div>
        </div>
      )}

      {extraComponents.length > 0 && (
        <div className="mt-3 overflow-hidden rounded-xl border border-border bg-card">
          <div className="flex items-center justify-between gap-3 border-b border-border px-3.5 py-2.5">
            <p className="text-xs font-bold text-foreground">Despesas e encargos da competência</p>
            <p className="text-[11px] text-muted-foreground">Inclusão e responsabilidade econômica</p>
          </div>
          <div className="divide-y divide-border">
            {extraComponents.map((component) => <RentalComponentRow key={component.id} component={component} />)}
          </div>
        </div>
      )}

      {isUnlinked && (
        <div className="mt-3 flex flex-col gap-3 rounded-xl border border-status-warning/35 bg-status-warning-soft px-3.5 py-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-start gap-2">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-status-warning" aria-hidden="true" />
            <div>
              <p className="text-xs font-bold text-foreground">Recebimento sem composição mensal reconciliada</p>
              <p className="mt-0.5 text-[11px] text-muted-foreground">Repasse ao proprietário e receita efetiva ficam “a apurar” até a entrada estar vinculada à cobrança e às regras da locação.</p>
            </div>
          </div>
          {contractId && onOpenRentalPlan && (
            <button
              type="button"
              onClick={() => onOpenRentalPlan(contractId)}
              className="inline-flex shrink-0 items-center justify-center gap-1.5 rounded-lg border border-status-warning/40 bg-card px-3 py-2 text-xs font-bold text-foreground transition-colors hover:bg-muted"
            >
              <ArrowDownToLine className="h-3.5 w-3.5" aria-hidden="true" /> Revisar plano da locação
            </button>
          )}
        </div>
      )}
    </div>
  );
}

function SummaryValue({ label, value, accent, strong }: { label: string; value?: number; accent?: boolean; strong?: boolean }) {
  return (
    <div className="min-w-0">
      <p className="truncate text-caption font-semibold uppercase tracking-wide text-muted-foreground" title={label}>{label}</p>
      <p className={clsx("mt-0.5 truncate text-sm tabular-nums", strong ? "font-extrabold" : "font-bold", accent ? "text-status-ok" : "text-foreground")}>
        {value === undefined ? "A apurar" : formatMoneyBR(value, { withSymbol: true })}
      </p>
    </div>
  );
}

function RentalComponentRow({ component }: { component: RentalChargeComponent }) {
  return (
    <div className="flex flex-col gap-2 px-3.5 py-2.5 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <p className="truncate text-xs font-bold text-foreground" title={component.description}>{component.description}</p>
        <div className="mt-1 flex flex-wrap gap-1.5 text-caption font-semibold">
          <span className="rounded-full border border-border bg-muted/40 px-2 py-0.5 text-muted-foreground">
            {billingLabel(component.billing)}
          </span>
          <span className="rounded-full border border-border bg-muted/40 px-2 py-0.5 text-muted-foreground">
            Responsabilidade: {economicOwnerLabel(component.economicOwner)}
          </span>
          {component.settlementMode === "liquidacao_direta" && (
            <span className="rounded-full border border-status-warning/30 bg-status-warning-soft px-2 py-0.5 text-status-warning">Pago pela imobiliária e reservado</span>
          )}
        </div>
      </div>
      <p className="shrink-0 text-sm font-bold tabular-nums text-foreground">{formatMoneyBR(component.amount, { withSymbol: true })}</p>
    </div>
  );
}

function allocationStateLabel(state: RentalEconomicSummary["allocationState"]): string {
  if (state === "reconciled") return "Reconciliado";
  if (state === "allocated") return "Pagamento alocado";
  if (state === "projected") return "Projeção do plano";
  return "A apurar";
}

function economicOwnerLabel(owner: RentalChargeComponent["economicOwner"]): string {
  if (owner === "owner") return "proprietário";
  if (owner === "agency") return "imobiliária";
  if (owner === "tenant") return "inquilino";
  if (owner === "broker") return "corretor";
  if (owner === "reimbursement") return "reembolso de custo adiantado";
  return "a confirmar";
}

function billingLabel(billing: RentalChargeComponent["billing"]): string {
  if (billing === "included") return "Incluído no aluguel";
  if (billing === "separate") return "Cobrança separada ao inquilino";
  if (billing === "withheld") return "Descontado do repasse";
  return "Pago pela imobiliária · fora da cobrança";
}
