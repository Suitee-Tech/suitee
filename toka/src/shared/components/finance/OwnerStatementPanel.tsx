import { FinanceLoadError } from "./FinanceLoadError";
import { useEffect, useMemo, useRef, useState } from "react";
import { Download, FileText, Loader2, RefreshCw, Wallet } from "lucide-react";
import { toast } from "sonner";

import { Card } from "../../ui/card";
import { Button } from "../../ui/button";
import { Input } from "../../ui/input";
import { SearchableSelect } from "../../ui/searchable-select";
import type { Lead } from "../../data";
import { formatMoneyBR, formatCompetencia, formatISODateBR } from "../../utils/format";
import {
  ownerStatementService,
  type MonthlyStatement,
  type OwnerBalanceSummary,
  type StatementDeduction,
  type StatementFlow,
  type StatementFlowSummary,
} from "../../services/ownerStatement.service";

const MONTHS = ["Jan", "Fev", "Mar", "Abr", "Mai", "Jun", "Jul", "Ago", "Set", "Out", "Nov", "Dez"];

const MANAGED_BILL_STATUS_LABELS: Record<string, string> = {
  RESERVED: "Reservada",
  APPROVED: "Aprovada",
  PAID: "Paga",
  RECONCILED: "Conciliada",
  CANCELLED: "Cancelada",
};

const FLOW_LABELS: Record<StatementFlow, string> = { sale: "Vendas", rent: "Aluguéis", unknown: "Modalidade a apurar" };
const DEDUCTION_TYPE_LABELS: Record<string, string> = {
  MAINTENANCE: "Manutenção",
  UTILITY: "Consumo",
  FEE: "Taxa",
  OTHER: "Outras",
};

/** Deduções do período com a competência do repasse de origem. */
interface StatementDeductionLine extends StatementDeduction {
  referenceMonth: string;
}

function statementDeductions(statement: MonthlyStatement): StatementDeductionLine[] {
  return statement.transfers.flatMap((transfer) =>
    (transfer.deduction_details ?? []).map((deduction) => ({
      ...deduction,
      referenceMonth: transfer.reference_month,
    })),
  );
}
const LINE_FLOW_LABELS: Record<StatementFlow, string> = { sale: "Venda", rent: "Locação", unknown: "A apurar" };

function transferFlow(t: MonthlyStatement["transfers"][number]): StatementFlow {
  if (t.statement_flow) return t.statement_flow;
  for (const value of [t.settlement_type, t.economic_breakdown?.flow]) {
    const flow = value?.trim().toLowerCase();
    if (flow === "sale" || flow === "rent") return flow;
  }
  return "unknown";
}

function brokerageAmount(t: MonthlyStatement["transfers"][number]): number | null {
  if (t.brokerage_amount != null) return t.brokerage_amount;
  const breakdown = t.economic_breakdown;
  if (transferFlow(t) === "sale" && breakdown?.flow.trim().toLowerCase() === "sale") {
    return (breakdown.agency_retained_cents + breakdown.commission_payable_cents) / 100;
  }
  return null;
}

/**
 * Intermediação descontada do repasse (comissão do primeiro aluguel diluída nos
 * primeiros meses). Prefere o campo do demonstrativo e cai para o breakdown
 * econômico do próprio repasse — o painel precisa funcionar durante deploy
 * rolante, quando o backend novo ainda não respondeu.
 */
function intermediationAmount(transfer: MonthlyStatement["transfers"][number]): number {
  if (typeof transfer.intermediation_amount === "number") return transfer.intermediation_amount;
  return (transfer.economic_breakdown?.intermediation_revenue_cents ?? 0) / 100;
}

function intermediationAmountFor(transfers: MonthlyStatement["transfers"]): number {
  return transfers.reduce((sum, transfer) => sum + intermediationAmount(transfer), 0);
}

function intermediationCommissionFunded(transfer: MonthlyStatement["transfers"][number]): number {
  if (typeof transfer.intermediation_commission_funded_amount === "number") return transfer.intermediation_commission_funded_amount;
  return (transfer.economic_breakdown?.commission_funded_cents ?? 0) / 100;
}

/** Quanto ainda será descontado de intermediação (0 quando não se aplica). */
function intermediationOutstanding(statement: MonthlyStatement): number {
  if (typeof statement.intermediation_outstanding_amount === "number") return statement.intermediation_outstanding_amount;
  return statement.transfers.reduce((sum, transfer) => sum + (transfer.intermediation_outstanding_amount ?? 0), 0);
}

// Rolling deployment: classify only transfers carrying authoritative modality.
function flowSummaries(statement: MonthlyStatement): StatementFlowSummary[] {
  if (statement.flow_summaries) return statement.flow_summaries;
  return (["sale", "rent", "unknown"] as const).flatMap((flow) => {
    const transfers = statement.transfers.filter((t) => transferFlow(t) === flow);
    if (!transfers.length) return [];
    const sum = (field: "gross_rent" | "admin_fee_amount" | "net_amount") => transfers.reduce((total, t) => total + t[field], 0);
    const brokerages = transfers.map(brokerageAmount);
    return [{ flow, transfer_count: transfers.length, gross_amount: sum("gross_rent"), admin_fee_amount: sum("admin_fee_amount"),
      brokerage_amount: flow === "sale" && brokerages.every((value) => value != null) ? brokerages.reduce<number>((sum, value) => sum + (value ?? 0), 0) : null,
      maintenance_deductions: transfers.reduce((sum, t) => sum + (t.maintenance_deductions ?? 0), 0),
      other_deductions: transfers.reduce((sum, t) => sum + (t.other_deductions ?? 0), 0),
      discount_amount: transfers.reduce((sum, t) => sum + (t.economic_breakdown?.discount_cents ?? 0) / 100, 0),
      intermediation_amount: intermediationAmountFor(transfers),
      intermediation_commission_funded_amount: transfers.reduce((sum, t) => sum + (t.economic_breakdown?.commission_funded_cents ?? 0) / 100, 0),
      net_amount: sum("net_amount") }];
  });
}

const MANUAL_OWNER = "__manual_owner__";
const NO_OWNER = "__no_owner__";

export function OwnerStatementPanel({ owners = [] }: { owners?: Lead[] }) {
  const now = new Date();
  const [ownerLeadId, setOwnerLeadId] = useState("");
  const [ownerSelection, setOwnerSelection] = useState("");
  const [year, setYear] = useState(now.getFullYear());
  const [month, setMonth] = useState(now.getMonth() + 1);
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<{ key: string; data: MonthlyStatement } | null>(null);
  const selectionKey = `${ownerLeadId.trim()}/${year}/${month}`;
  const selectionRef = useRef(selectionKey);
  selectionRef.current = selectionKey;
  const requestRef = useRef(0);
  const balanceRequestRef = useRef(0);
  const statement = result?.key === selectionKey ? result.data : null;
  const [loadError, setLoadError] = useState<string | null>(null);
  const [balanceError, setBalanceError] = useState<string | null>(null);
  const [balance, setBalance] = useState<OwnerBalanceSummary | null>(null);
  const [balanceLoading, setBalanceLoading] = useState(false);

  useEffect(() => {
    requestRef.current++;
    balanceRequestRef.current++;
    setResult(null);
    setBalance(null);
    setLoadError(null);
    setBalanceError(null);
    setLoading(false);
    setBalanceLoading(false);
    return () => { requestRef.current++; balanceRequestRef.current++; };
  }, [selectionKey]);

  const ownerBalanceSummary = useMemo(() => {
    if (!statement) return null;
    const transfers = statement.transfers ?? [];
    const coveredCount = statement.balance_covered_count ?? 0;
    if (coveredCount > 0) {
      return {
        total: statement.owner_balance_total ?? 0,
        reserved: statement.reserved_amount ?? 0,
        blocked: statement.blocked_amount ?? 0,
        available: statement.available_amount ?? 0,
        coveredTransfers: coveredCount,
        totalTransfers: transfers.length,
      };
    }

    const balances = transfers.flatMap((transfer) => (transfer.owner_balance ? [transfer.owner_balance] : []));
    if (balances.length === 0) return null;

    const cents = balances.reduce(
      (sum, balance) => ({
        total: sum.total + balance.total_cents,
        reserved: sum.reserved + balance.reserved_cents,
        blocked: sum.blocked + balance.blocked_cents,
        available: sum.available + Math.max(balance.total_cents - balance.reserved_cents - balance.blocked_cents, 0),
      }),
      { total: 0, reserved: 0, blocked: 0, available: 0 },
    );

    return {
      total: cents.total / 100,
      reserved: cents.reserved / 100,
      blocked: cents.blocked / 100,
      available: cents.available / 100,
      coveredTransfers: balances.length,
      totalTransfers: transfers.length,
    };
  }, [statement]);

  const summaries = useMemo(() => statement ? flowSummaries(statement) : [], [statement]);
  // Intermediação do período, a fatia que pagou comissões e o que ainda será
  // descontado: sem isso o painel mostrava só a taxa de administração e o
  // repasse parecia incompleto.
  const intermediationTotal = summaries.reduce((sum, summary) => sum + (summary.intermediation_amount ?? 0), 0);
  const intermediationFunded = summaries.reduce((sum, summary) => sum + (summary.intermediation_commission_funded_amount ?? 0), 0);
  const intermediationOutstandingTotal = statement ? intermediationOutstanding(statement) : 0;

  const managedBills = useMemo(() => statement?.managed_bills ?? [], [statement]);
  const deductionLines = useMemo(() => (statement ? statementDeductions(statement) : []), [statement]);
  const ownerOptions = useMemo(
    () => [...owners]
      .sort((left, right) => left.name.localeCompare(right.name, "pt-BR"))
      .map((owner) => ({
        value: owner.id,
        label: owner.name,
        searchText: `${owner.name} ${owner.email || ""} ${owner.phone || ""} ${owner.displayId || ""} ${owner.id}`,
      })),
    [owners],
  );

  const loadBalance = async (leadId: string) => {
    const key = selectionKey;
    const request = ++balanceRequestRef.current;
    const current = () => key === selectionRef.current && request === balanceRequestRef.current;
    setBalance(null);
    setBalanceError(null);
    setBalanceLoading(true);
    try {
      const b = await ownerStatementService.getBalance(leadId);
      if (current()) setBalance(b);
    } catch (err) {
      console.error(err);
      if (current()) setBalanceError("Não foi possível carregar o saldo do proprietário.");
    } finally {
      if (current()) setBalanceLoading(false);
    }
  };

  const generate = async () => {
    const leadId = ownerLeadId.trim();
    if (!leadId) {
      toast.error("Selecione ou informe o proprietário.");
      return;
    }
    if (!Number.isInteger(year) || year < 1900 || year > 9999) {
      setLoadError("Informe um ano válido.");
      return;
    }
    const request = ++requestRef.current;
    const key = selectionKey;
    const current = () => request === requestRef.current && key === selectionRef.current;
    setLoading(true);
    setLoadError(null);
    setResult(null);
    setBalance(null);
    setBalanceError(null);
    try {
      const s = await ownerStatementService.getAdmin(leadId, year, month);
      if (!current()) return;
      setResult({ key, data: s });
      void loadBalance(leadId);
      if (!s || (s.transfers?.length ?? 0) === 0) toast.info("Nenhum repasse no período.");
    } catch (err) {
      console.error(err);
      if (current()) setLoadError("Não foi possível gerar o extrato.");
    } finally {
      if (current()) setLoading(false);
    }
  };

  const downloadPdf = async () => {
    if (!statement || result?.key !== selectionKey) return;
    try {
      const blob = await ownerStatementService.downloadAdminPdf(ownerLeadId.trim(), year, month);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `extrato-${ownerLeadId.trim()}-${year}-${String(month).padStart(2, "0")}.pdf`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
    } catch (err) {
      console.error(err);
      toast.error("Não foi possível baixar o PDF.");
    }
  };

  return (
    <Card className="space-y-4 p-4">
      <div className="flex items-center gap-2">
        <FileText className="h-4 w-4 text-primary-600" />
        <h3 className="text-sm font-bold text-foreground">Extrato do Proprietário</h3>
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-4">
        <div className="sm:col-span-2 space-y-2">
          {ownerOptions.length > 0 && (
            <div>
              <label className="mb-1 block text-xs font-bold text-foreground">Proprietário</label>
              <SearchableSelect
                value={ownerSelection || NO_OWNER}
                onValueChange={(next) => {
                  if (next === NO_OWNER) {
                    setOwnerSelection("");
                    setOwnerLeadId("");
                    return;
                  }
                  setOwnerSelection(next);
                  if (next !== MANUAL_OWNER) setOwnerLeadId(next);
                  else setOwnerLeadId("");
                }}
                options={[
                  { value: NO_OWNER, label: "Selecione um proprietário…" },
                  ...ownerOptions,
                  { value: MANUAL_OWNER, label: "Informar ID manualmente (exceção)" },
                ]}
                searchPlaceholder="Buscar por nome, e-mail, telefone ou ID…"
                emptyMessage="Nenhum proprietário encontrado."
                aria-label="Proprietário"
              />
            </div>
          )}
          {(ownerOptions.length === 0 || ownerSelection === MANUAL_OWNER) && (
            <div>
              <label htmlFor="owner-statement-owner-id" className="mb-1 block text-xs font-bold text-foreground">ID do proprietário (lead)</label>
              <Input id="owner-statement-owner-id" value={ownerLeadId} onChange={(e) => setOwnerLeadId(e.target.value)} placeholder="Cole o ID do lead" />
              {ownerOptions.length > 0 && <p className="mt-1 text-[11px] text-muted-foreground">Use apenas quando o proprietário ainda não estiver disponível no cadastro.</p>}
            </div>
          )}
        </div>
        <div>
          <label htmlFor="owner-statement-month" className="mb-1 block text-xs font-bold text-foreground">Mês</label>
          <select
            id="owner-statement-month"
            value={month}
            onChange={(e) => setMonth(Number(e.target.value))}
            className="h-10 w-full rounded-xl border border-border bg-card px-3 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500/20"
          >
            {MONTHS.map((m, i) => (
              <option key={m} value={i + 1}>{m}</option>
            ))}
          </select>
        </div>
        <div>
          <label htmlFor="owner-statement-year" className="mb-1 block text-xs font-bold text-foreground">Ano</label>
          <Input id="owner-statement-year" type="number" value={year} onChange={(e) => setYear(Number(e.target.value))} />
        </div>
      </div>

      <div className="flex items-center gap-2">
        <Button onClick={generate} disabled={loading}>
          {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <FileText className="h-4 w-4" />} Gerar extrato
        </Button>
        {statement && (
          <Button variant="outline" onClick={downloadPdf}>
            <Download className="h-4 w-4" /> Baixar PDF
          </Button>
        )}
      </div>

      {loadError && <FinanceLoadError message={loadError} onRetry={() => void generate()} />}
      {statement && (
        <div className="space-y-3">
          {balanceError && <FinanceLoadError message={balanceError} onRetry={() => void loadBalance(ownerLeadId.trim())} />}
          {(balanceLoading || balance) && (
            <div data-testid="owner-balance-widget" className="rounded-xl border border-primary-200 bg-primary-50/40 p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex items-center gap-2">
                  <Wallet className="h-4 w-4 text-primary-600" />
                  <p className="text-sm font-bold text-foreground">Saldo atual do proprietário</p>
                  {balance?.has_balance && balance.reference_month && (
                    <span className="text-xs text-muted-foreground">(competência {formatCompetencia(balance.reference_month)})</span>
                  )}
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => void loadBalance(ownerLeadId.trim())}
                  disabled={balanceLoading || !ownerLeadId.trim()}
                >
                  {balanceLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
                  Atualizar
                </Button>
              </div>
              {balance?.has_balance ? (
                <>
                  <div className="mt-2 grid grid-cols-2 gap-3 sm:grid-cols-4">
                    <Stat label="Total" value={balance.total_cents / 100} />
                    <Stat label="Reservado" value={balance.reserved_cents / 100} />
                    <Stat label="Bloqueado" value={balance.blocked_cents / 100} />
                    <Stat label="Disponível para repasse" value={balance.available_cents / 100} accent />
                  </div>
                  <p className="mt-2 text-xs text-muted-foreground">
                    Saldo corrente derivado do repasse mais recente com subledger de saldo (base + movimentações de contas
                    administradas), independente do mês do extrato. Não representa saldo bancário.
                  </p>
                </>
              ) : (
                <p className="mt-2 text-xs text-status-warning">
                  {balanceLoading
                    ? "Carregando saldo…"
                    : "Nenhuma composição de saldo disponível para este proprietário — repasse legado sem subledger ou sem repasse gerado."}
                </p>
              )}
            </div>
          )}

          <div className="grid grid-cols-2 gap-3">
            <Stat label="Total bruto" value={statement.gross_rent} />
            <Stat label="Total líquido dos repasses" value={statement.net_amount} accent />
          </div>

          {intermediationTotal > 0 && (
            <section
              aria-label="Intermediação descontada do repasse"
              className="space-y-1 rounded-xl border border-border bg-muted/20 p-3"
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h4 className="text-sm font-bold text-foreground">Intermediação (comissão do primeiro aluguel)</h4>
                <span className="text-sm font-bold text-status-critical">− {formatMoneyBR(intermediationTotal, { withSymbol: true })}</span>
              </div>
              <p className="text-xs text-muted-foreground">
                Descontada do próprio aluguel nos primeiros meses, por isso o repasse fica menor que
                {" "}“aluguel bruto − taxa de administração”. A taxa de administração continua sendo cobrada todo mês.
              </p>
              {intermediationFunded > 0 && (
                <p className="text-xs text-muted-foreground">
                  Deste valor, {formatMoneyBR(intermediationFunded, { withSymbol: true })} foram para comissões de corretores
                  {intermediationTotal - intermediationFunded > 0.005
                    ? ` e ${formatMoneyBR(intermediationTotal - intermediationFunded, { withSymbol: true })} ficaram com a imobiliária.`
                    : "."}
                </p>
              )}
              {intermediationOutstandingTotal > 0 && (
                <p className="text-xs text-muted-foreground">
                  Ainda serão descontados <strong className="text-foreground">{formatMoneyBR(intermediationOutstandingTotal, { withSymbol: true })}</strong> nos
                  próximos repasses até completar a intermediação contratada
                  {intermediationTotal + intermediationOutstandingTotal > 0
                    ? ` (total contratado: ${formatMoneyBR(intermediationTotal + intermediationOutstandingTotal, { withSymbol: true })} neste recorte).`
                    : "."}
                </p>
              )}
            </section>
          )}

          {summaries.map((summary) => (
            <section key={summary.flow} aria-label={FLOW_LABELS[summary.flow]} className="space-y-2 rounded-xl border border-border bg-muted/20 p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h4 className="text-sm font-bold text-foreground">{FLOW_LABELS[summary.flow]}</h4>
                <span className="text-xs text-muted-foreground">{summary.transfer_count} repasse(s)</span>
              </div>
              {summary.flow === "unknown" && <p className="text-xs text-status-warning">Registros sem modalidade confirmada; valores separados até a identificação da origem.</p>}
              <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
                <Stat label={summary.flow === "sale" ? "Venda bruta" : summary.flow === "rent" ? "Aluguel bruto" : "Bruto a apurar"} value={summary.gross_amount} />
                {summary.flow === "sale" ? (
                  summary.brokerage_amount != null ? <Stat label="Corretagem" value={summary.brokerage_amount} negative /> :
                    <div className="rounded-xl border border-border bg-card p-3"><p className="text-xs text-muted-foreground">Corretagem</p><p className="text-sm font-bold text-status-warning">A apurar</p></div>
                ) : <Stat label={summary.flow === "rent" ? "Taxa de administração" : "Taxas registradas"} value={summary.admin_fee_amount} negative />}
                {summary.flow === "sale" && summary.admin_fee_amount !== 0 && <Stat label="Taxa registrada" value={summary.admin_fee_amount} negative />}
                {(summary.intermediation_amount ?? 0) > 0 && (
                  <Stat label="Intermediação" value={summary.intermediation_amount ?? 0} negative />
                )}
                <Stat label="Manutenção" value={summary.maintenance_deductions} negative />
                <Stat label="Outras deduções" value={summary.other_deductions} negative />
                {summary.discount_amount > 0 && <Stat label="Descontos" value={summary.discount_amount} negative />}
                <Stat label="Líquido do segmento" value={summary.net_amount} accent />
              </div>
            </section>
          ))}

          {ownerBalanceSummary && (
            <div className="space-y-2 rounded-xl border border-border bg-muted/20 p-3">
              <div>
                <p className="text-sm font-bold text-foreground">Composição econômica dos repasses</p>
                <p className="text-xs text-muted-foreground">
                  Bolsões informados pelo motor financeiro para esta competência; não representam saldo bancário.
                </p>
              </div>
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                <Stat label="Total do proprietário" value={ownerBalanceSummary.total} />
                <Stat label="Reservado para encargos" value={ownerBalanceSummary.reserved} />
                <Stat label="Bloqueado" value={ownerBalanceSummary.blocked} />
                <Stat label="Disponível para repasse" value={ownerBalanceSummary.available} accent />
              </div>
              {ownerBalanceSummary.coveredTransfers < ownerBalanceSummary.totalTransfers && (
                <p className="text-xs text-status-warning">
                  Composição disponível em {ownerBalanceSummary.coveredTransfers} de {ownerBalanceSummary.totalTransfers} repasses; registros legados foram omitidos deste resumo.
                </p>
              )}
            </div>
          )}

          {managedBills.length > 0 && (
            <div className="space-y-2 rounded-xl border border-border bg-muted/20 p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="text-sm font-bold text-foreground">Contas administradas no período</p>
                <p className="text-xs font-bold text-foreground">
                  Total: {formatMoneyBR(statement.managed_bills_total ?? 0, { withSymbol: true })}
                </p>
              </div>
              <p className="text-xs text-muted-foreground">
                Consumo do reservado do proprietário na competência; o total considera todas as linhas, independentemente do status.
              </p>
              <div className="overflow-hidden rounded-xl border border-border">
                <table className="w-full text-sm">
                  <thead className="bg-muted/40 text-xs text-muted-foreground">
                    <tr>
                      <th className="px-3 py-2 text-left font-bold">Componente</th>
                      <th className="px-3 py-2 text-left font-bold">Credor</th>
                      <th className="px-3 py-2 text-left font-bold">Descrição</th>
                      <th className="px-3 py-2 text-right font-bold">Valor</th>
                      <th className="px-3 py-2 text-right font-bold">Vencimento</th>
                      <th className="px-3 py-2 text-left font-bold">Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {managedBills.map((bill) => (
                      <tr key={bill.id} className="border-t border-border">
                        <td className="px-3 py-2 font-bold capitalize">{bill.component}</td>
                        <td className="px-3 py-2">{bill.creditor_name || "—"}</td>
                        <td className="px-3 py-2 text-muted-foreground">{bill.description || "—"}</td>
                        <td className="px-3 py-2 text-right">{formatMoneyBR(bill.amount, { withSymbol: true })}</td>
                        <td className="px-3 py-2 text-right">
                          {bill.due_date ? (formatISODateBR(bill.due_date) || "—") : "—"}
                        </td>
                        <td className="px-3 py-2">{MANAGED_BILL_STATUS_LABELS[bill.status] ?? bill.status}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {statement.journal_totals && (
            <div className="space-y-2 rounded-xl border border-border bg-muted/20 p-3">
              <p className="text-sm font-bold text-foreground">Conferência contábil (ledger)</p>
              <p className="text-xs text-muted-foreground">
                Totais por conta do razão (append-only) dos contratos do período — débito positivo, crédito negativo — para
                conferir o demonstrativo contra a contabilização real.
              </p>
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                <Stat label="Receita (revenue)" value={statement.journal_totals.revenue} />
                <Stat label="A pagar ao proprietário" value={statement.journal_totals.owner_payable} />
                <Stat label="Contas a receber" value={statement.journal_totals.accounts_receivable} />
                <Stat label="Caixa (cash)" value={statement.journal_totals.cash} />
              </div>
            </div>
          )}

          {deductionLines.length > 0 && (
            <div className="space-y-2 rounded-xl border border-border bg-muted/20 p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="text-sm font-bold text-foreground">Deduções do período</p>
                <p className="text-xs font-bold text-foreground">
                  Total: − {formatMoneyBR(deductionLines.reduce((sum, line) => sum + line.amount, 0), { withSymbol: true })}
                </p>
              </div>
              <p className="text-xs text-muted-foreground">
                Cada desconto aplicado ao repasse, com a origem. Manutenções lançadas no chamado aparecem aqui com o serviço e a OS.
              </p>
              <div className="overflow-hidden rounded-xl border border-border">
                <table className="w-full text-sm">
                  <thead className="bg-muted/40 text-xs text-muted-foreground">
                    <tr>
                      <th className="px-3 py-2 text-left font-bold">Competência</th>
                      <th className="px-3 py-2 text-left font-bold">Origem</th>
                      <th className="px-3 py-2 text-left font-bold">Descrição</th>
                      <th className="px-3 py-2 text-left font-bold">Referência</th>
                      <th className="px-3 py-2 text-right font-bold">Valor</th>
                    </tr>
                  </thead>
                  <tbody>
                    {deductionLines.map((line, index) => (
                      <tr key={`${line.referenceMonth}-${line.reference_id ?? index}`} className="border-t border-border">
                        <td className="px-3 py-2">{formatCompetencia(line.referenceMonth)}</td>
                        <td className="px-3 py-2 font-bold">{DEDUCTION_TYPE_LABELS[line.type] ?? line.type}</td>
                        <td className="px-3 py-2 text-muted-foreground">{line.description || "—"}</td>
                        <td className="px-3 py-2 text-muted-foreground">{line.reference_id || "—"}</td>
                        <td className="px-3 py-2 text-right text-status-critical">
                          − {formatMoneyBR(line.amount, { withSymbol: true })}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {(statement.transfers?.length ?? 0) > 0 && (
            <div className="overflow-hidden rounded-xl border border-border">
              <table className="w-full text-sm">
                <thead className="bg-muted/40 text-xs text-muted-foreground">
                  <tr>
                    <th className="px-3 py-2 text-left font-bold">Modalidade</th>
                    <th className="px-3 py-2 text-left font-bold">Mês ref.</th>
                    <th className="px-3 py-2 text-right font-bold">Bruto</th>
                    <th className="px-3 py-2 text-right font-bold">Taxa / corretagem</th>
                    <th className="px-3 py-2 text-right font-bold">Intermediação</th>
                    <th className="px-3 py-2 text-right font-bold">Deduções</th>
                    <th className="px-3 py-2 text-right font-bold">Líquido</th>
                    <th className="px-3 py-2 text-left font-bold">Status</th>
                  </tr>
                </thead>
                <tbody>
                  {statement.transfers.map((t) => (
                    <tr key={t.id} className="border-t border-border">
                      <td className="px-3 py-2 font-bold">{LINE_FLOW_LABELS[transferFlow(t)]}</td>
                      <td className="px-3 py-2">{formatCompetencia(t.reference_month)}</td>
                      <td className="px-3 py-2 text-right">{formatMoneyBR(t.gross_rent, { withSymbol: true })}</td>
                      <td className="px-3 py-2 text-right">{transferFlow(t) === "sale" ? (brokerageAmount(t) != null ? formatMoneyBR(brokerageAmount(t)!, { withSymbol: true }) : "A apurar") : formatMoneyBR(t.admin_fee_amount, { withSymbol: true })}</td>
                      <td className="px-3 py-2 text-right">
                        {intermediationAmount(t) > 0 ? (
                          <span className="text-status-critical" title={`Comissões de corretor: ${formatMoneyBR(intermediationCommissionFunded(t), { withSymbol: true })}`}>
                            − {formatMoneyBR(intermediationAmount(t), { withSymbol: true })}
                          </span>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </td>
                      <td className="px-3 py-2 text-right text-status-critical">
                        {(t.maintenance_deductions ?? 0) + (t.other_deductions ?? 0) > 0
                          ? `− ${formatMoneyBR((t.maintenance_deductions ?? 0) + (t.other_deductions ?? 0), { withSymbol: true })}`
                          : "—"}
                      </td>
                      <td className="px-3 py-2 text-right">{formatMoneyBR(t.net_amount, { withSymbol: true })}</td>
                      <td className="px-3 py-2 text-muted-foreground">{t.status}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </Card>
  );
}

function Stat({ label, value, negative, accent }: { label: string; value: number; negative?: boolean; accent?: boolean }) {
  return (
    <div className="rounded-xl border border-border bg-card p-3">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className={`text-sm font-bold ${accent ? "text-status-ok" : negative ? "text-status-critical" : "text-foreground"}`}>
        {negative && value > 0 ? "− " : ""}
        {formatMoneyBR(value, { withSymbol: true })}
      </p>
    </div>
  );
}
