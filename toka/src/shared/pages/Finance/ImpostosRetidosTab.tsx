import { useCallback, useState } from 'react';
import { Link } from 'react-router';
import { AlertTriangle, CheckCircle2, Clock, Loader2, Receipt, RefreshCw, Wallet } from 'lucide-react';
import { toast } from 'sonner';
import { clsx } from 'clsx';

import { formatISODateBR, formatMoneyBR } from '../../utils/format';
import { useFinancialRead } from '../../hooks/useFinancialRead';
import { FinanceLoadError } from '../../components/finance/FinanceLoadError';
import { SettleTaxObligationPanel } from '../../components/finance/SettleTaxObligationPanel';
import { Button } from '../../ui/button';
import { ResponsiveTable } from '../../ui/responsive-table';
import {
  taxObligationService,
  type TaxObligation,
  type TaxObligationKind,
  type TaxObligationStatus,
} from '../../services/taxObligation.service';

const KINDS: TaxObligationKind[] = ['IRRF', 'INSS', 'ISS', 'OUTROS'];

const KIND_LABEL: Record<TaxObligationKind, string> = {
  IRRF: 'IRRF',
  INSS: 'INSS',
  ISS: 'ISS',
  OUTROS: 'Outros',
};

const STATUS_LABEL: Record<TaxObligationStatus, string> = {
  OPEN: 'A recolher',
  PAID: 'Recolhido',
  CANCELLED: 'Cancelado',
};

const STATUS_PILL: Record<TaxObligationStatus, string> = {
  OPEN: 'bg-status-warning-soft text-status-warning border-status-warning/30',
  PAID: 'bg-status-ok-soft text-status-ok border-status-ok/30',
  CANCELLED: 'bg-muted text-muted-foreground border-border',
};

type StatusFilter = TaxObligationStatus | 'ALL';

const STATUS_OPTIONS: { value: StatusFilter; label: string }[] = [
  { value: 'OPEN', label: 'A recolher' },
  { value: 'PAID', label: 'Recolhidos' },
  { value: 'ALL', label: 'Todas' },
];

const formatCents = (cents?: number) => formatMoneyBR((cents ?? 0) / 100);

/** "2026-09" → "09/2026" — a competência é sempre o mês do desconto lançado. */
const formatCompetenceMonth = (competence?: string): string => {
  const match = /^(\d{4})-(\d{2})/.exec((competence ?? '').trim());
  return match ? `${match[2]}/${match[1]}` : competence || '—';
};

const isOverdue = (obligation: TaxObligation, now: number) =>
  obligation.status === 'OPEN' && new Date(obligation.due_date).getTime() < now;

/**
 * Aba "Impostos retidos": o passivo que nasce quando a imobiliária retém
 * IRRF/INSS/ISS da comissão do corretor e precisa recolher a guia até o
 * vencimento.
 *
 * Nada aqui é digitado à mão: a obrigação é DERIVADA do desconto lançado na
 * comissão (Financeiro → Comissões → Descontos). A tela só lê a lista, mostra
 * os totais calculados no servidor e dá baixa no recolhimento com comprovante.
 */
export function ImpostosRetidosTab() {
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('OPEN');
  const [kindFilter, setKindFilter] = useState<TaxObligationKind | 'ALL'>('ALL');
  const [competence, setCompetence] = useState('');
  const [overdueOnly, setOverdueOnly] = useState(false);
  const [settling, setSettling] = useState<TaxObligation | null>(null);
  const [recomputing, setRecomputing] = useState(false);

  const load = useCallback(
    () =>
      taxObligationService.list({
        status: statusFilter === 'ALL' ? undefined : statusFilter,
        kind: kindFilter === 'ALL' ? undefined : kindFilter,
        competence: competence || undefined,
        overdue: overdueOnly || undefined,
        limit: 100,
      }),
    [statusFilter, kindFilter, competence, overdueOnly],
  );

  // A chave carrega o filtro inteiro: uma resposta atrasada de um filtro antigo
  // nunca publica dados (nem limpa o erro) do filtro atual.
  const read = useFinancialRead(
    `tax-obligations/${statusFilter}/${kindFilter}/${competence}/${overdueOnly}`,
    load,
  );
  const items = read.data?.items ?? [];
  const totals = read.data?.totals;
  const refresh = read.retry;

  // "Agora" lido uma vez por render: o vencimento é comparado com a MESMA
  // referência em todas as linhas, para duas linhas da mesma tabela não
  // discordarem sobre o que está vencido.
  const now = Date.now();

  const handleRecompute = async () => {
    setRecomputing(true);
    try {
      const result = await taxObligationService.recompute();
      toast.success(
        result.commissions_processed === 1
          ? '1 comissão reconciliada.'
          : `${result.commissions_processed} comissões reconciliadas.`,
      );
      refresh();
    } catch (err) {
      console.error(err);
      toast.error('Não foi possível reconciliar as obrigações com as comissões.');
    } finally {
      setRecomputing(false);
    }
  };

  const byKind = totals?.by_kind ?? {};

  if (read.error) return <FinanceLoadError message={read.error} onRetry={refresh} />;
  if (read.loading) return <p role="status">Carregando impostos retidos…</p>;

  return (
    <div className="space-y-6">
      <p className="text-xs text-muted-foreground">
        Totais do filtro atual, calculados no servidor. Cada tributo nasce do desconto de IRRF/INSS/ISS lançado na
        comissão do corretor.
      </p>

      {/* Totais */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        <TotalCard
          icon={Wallet}
          label="A recolher"
          value={formatCents(totals?.open_amount_cents)}
          sub={`${totals?.open_count ?? 0} obrigações abertas`}
          tone="warning"
        />
        <TotalCard
          icon={AlertTriangle}
          label="Atrasado"
          value={formatCents(totals?.overdue_amount_cents)}
          sub={`${totals?.overdue_count ?? 0} vencidas`}
          tone="critical"
        />
        <TotalCard
          icon={CheckCircle2}
          label="Recolhido no filtro"
          value={formatCents(totals?.paid_amount_cents)}
          sub={`${totals?.paid_count ?? 0} obrigações recolhidas`}
          tone="ok"
        />
      </div>

      {/* Retrato por tipo */}
      <div className="rounded-xl border border-border bg-card p-4">
        <div className="mb-3 flex items-center gap-2">
          <Receipt className="h-4 w-4 text-primary-600" aria-hidden="true" />
          <h3 className="text-sm font-bold text-foreground">Retrato por tipo</h3>
        </div>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {KINDS.map((kind) => {
            const entry = byKind[kind];
            return (
              <div key={kind} className="rounded-lg border border-border bg-muted/30 p-3">
                <p className="text-xs font-bold uppercase tracking-wider text-muted-foreground">{KIND_LABEL[kind]}</p>
                <p className="mt-1 text-sm font-bold tabular-nums text-foreground">
                  {formatCents(entry?.open_amount_cents)}
                </p>
                <p className="text-xs text-muted-foreground">
                  a recolher · {entry?.open_count ?? 0} aberta(s)
                </p>
                <p className="mt-1 text-xs text-status-ok tabular-nums">
                  {formatCents(entry?.paid_amount_cents)} recolhido
                </p>
              </div>
            );
          })}
        </div>
      </div>

      {/* Filtros */}
      <div className="space-y-3 rounded-xl border border-border bg-card p-4">
        <div className="flex flex-wrap items-end gap-3">
          <div>
            <label htmlFor="tax-status" className="mb-1 block text-xs font-bold text-muted-foreground">
              Status
            </label>
            <select
              id="tax-status"
              value={statusFilter}
              onChange={(event) => setStatusFilter(event.target.value as StatusFilter)}
              className="rounded-lg border border-border bg-muted px-3 py-2 text-sm outline-none focus:border-primary-500 focus:bg-card focus:ring-2 focus:ring-primary-500/20"
            >
              {STATUS_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label htmlFor="tax-kind" className="mb-1 block text-xs font-bold text-muted-foreground">
              Tipo
            </label>
            <select
              id="tax-kind"
              value={kindFilter}
              onChange={(event) => setKindFilter(event.target.value as TaxObligationKind | 'ALL')}
              className="rounded-lg border border-border bg-muted px-3 py-2 text-sm outline-none focus:border-primary-500 focus:bg-card focus:ring-2 focus:ring-primary-500/20"
            >
              <option value="ALL">Todos os tipos</option>
              {KINDS.map((kind) => (
                <option key={kind} value={kind}>
                  {KIND_LABEL[kind]}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label htmlFor="tax-competence" className="mb-1 block text-xs font-bold text-muted-foreground">
              Competência
            </label>
            <input
              id="tax-competence"
              type="month"
              value={competence}
              onChange={(event) => setCompetence(event.target.value)}
              className="rounded-lg border border-border bg-muted px-3 py-2 text-sm outline-none focus:border-primary-500 focus:bg-card focus:ring-2 focus:ring-primary-500/20"
            />
          </div>
          <Button
            variant={overdueOnly ? 'secondary' : 'outline'}
            size="sm"
            aria-pressed={overdueOnly}
            onClick={() => setOverdueOnly((current) => !current)}
          >
            <Clock className="h-4 w-4" aria-hidden="true" /> Atrasadas
          </Button>
          {(statusFilter !== 'OPEN' || kindFilter !== 'ALL' || competence || overdueOnly) && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setStatusFilter('OPEN');
                setKindFilter('ALL');
                setCompetence('');
                setOverdueOnly(false);
              }}
            >
              Limpar filtros
            </Button>
          )}
        </div>
        <p className="text-xs text-muted-foreground">
          Mostrando {items.length} {items.length === 1 ? 'obrigação' : 'obrigações'}
          {overdueOnly ? ' vencidas' : ''}
          {competence ? ` na competência ${formatCompetenceMonth(competence)}` : ''}.
        </p>
      </div>

      {/* Tabela */}
      <div className="overflow-hidden rounded-xl border border-border bg-card">
        <div className="overflow-x-auto">
          <ResponsiveTable
            data={items}
            caption="Recolhimentos de impostos retidos"
            getRowKey={(obligation) => obligation.id}
            empty={(
              <EmptyState
                recomputing={recomputing}
                onRecompute={handleRecompute}
                filtered={Boolean(competence || kindFilter !== 'ALL' || statusFilter !== 'OPEN' || overdueOnly)}
              />
            )}
            columns={[
              { key: "kind", header: "Tipo", primary: true, cell: (obligation) => KIND_LABEL[obligation.kind] ?? obligation.kind },
              { key: "competence", header: "Competência", cell: (obligation) => formatCompetenceMonth(obligation.competence) },
              { key: "amount", header: "Valor", className: "text-right", cell: (obligation) => <span className="font-mono font-bold">{formatCents(obligation.amount_cents)}</span> },
              {
                key: "due",
                header: "Vencimento",
                cell: (obligation) => {
                  const overdue = isOverdue(obligation, now);
                  return (
                    <span className={clsx("flex items-center gap-1.5", overdue && "font-bold text-status-critical")}>
                      {formatISODateBR(obligation.due_date) || "—"}
                      {overdue && <span className="inline-flex rounded-full border border-status-critical/30 bg-status-critical-soft px-2 py-0.5 text-xs font-bold text-status-critical">Vencida</span>}
                    </span>
                  );
                },
              },
              {
                key: "status",
                header: "Status",
                cell: (obligation) => (
                  <span className={clsx("inline-flex rounded-full border px-2 py-0.5 text-xs font-bold", STATUS_PILL[obligation.status])}>
                    {STATUS_LABEL[obligation.status] ?? obligation.status}
                  </span>
                ),
              },
              {
                key: "action",
                header: "Ações",
                className: "text-right",
                cell: (obligation) => obligation.status === "OPEN" ? (
                  <Button variant="outline" size="sm" onClick={() => setSettling(obligation)} aria-label={`Dar baixa no recolhimento de ${KIND_LABEL[obligation.kind] ?? obligation.kind} da competência ${formatCompetenceMonth(obligation.competence)}`}>
                    <CheckCircle2 className="h-4 w-4" aria-hidden="true" /> Dar baixa
                  </Button>
                ) : <span className="text-xs text-muted-foreground">—</span>,
              },
            ]}
          />
        </div>
      </div>

      {settling && (
        <SettleTaxObligationPanel
          obligation={settling}
          onClose={() => setSettling(null)}
          onSettled={() => refresh()}
        />
      )}
    </div>
  );
}

function TotalCard({
  icon: Icon,
  label,
  value,
  sub,
  tone,
}: {
  icon: typeof Wallet;
  label: string;
  value: string;
  sub: string;
  tone: 'warning' | 'critical' | 'ok';
}) {
  const cls =
    tone === 'warning'
      ? 'bg-status-warning-soft text-status-warning border-status-warning/30'
      : tone === 'critical'
        ? 'bg-status-critical-soft text-status-critical border-status-critical/30'
        : 'bg-status-ok-soft text-status-ok border-status-ok/30';
  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="flex items-start gap-3">
        <div className={clsx('rounded-lg border p-2', cls)}>
          <Icon className="h-5 w-5" aria-hidden="true" />
        </div>
        <div className="min-w-0">
          <p className="text-xs font-bold uppercase tracking-wider text-muted-foreground">{label}</p>
          <p className="mt-1 truncate text-2xl font-bold text-foreground">{value}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">{sub}</p>
        </div>
      </div>
    </div>
  );
}

/**
 * Estado vazio: a tela não cria obrigação nenhuma, então a explicação precisa
 * dizer DE ONDE a lista vem — e o caminho para lançar o desconto que a gera.
 */
function EmptyState({
  filtered,
  recomputing,
  onRecompute,
}: {
  filtered: boolean;
  recomputing: boolean;
  onRecompute: () => void;
}) {
  return (
    <div className="mx-auto max-w-xl space-y-3 text-left">
      <div className="flex items-start gap-3">
        <div className="rounded-xl border border-status-info/30 bg-status-info-soft p-2 text-status-info">
          <Receipt className="h-5 w-5" aria-hidden="true" />
        </div>
        <div className="space-y-1">
          <p className="text-sm font-bold text-foreground">
            {filtered ? 'Nenhum tributo retido neste filtro' : 'Nenhum tributo retido a recolher'}
          </p>
          <p className="text-xs leading-relaxed text-muted-foreground">
            A lista nasce do <span className="font-bold">desconto de IRRF, INSS, ISS ou Outros</span> lançado na
            comissão do corretor: cada desconto vira uma obrigação por (tipo, competência), com vencimento no mês
            seguinte. Não há lançamento manual aqui.
          </p>
          <p className="text-xs leading-relaxed text-muted-foreground">
            Lance o desconto em Financeiro → Comissões → <span className="font-bold">Descontos</span> (a comissão
            precisa estar pendente, antes da aprovação da ordem de pagamento).
          </p>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2 pl-12">
        <Button asChild variant="outline" size="sm">
          <Link to="/financeiro/comissoes">Abrir comissões</Link>
        </Button>
        <Button variant="ghost" size="sm" onClick={onRecompute} disabled={recomputing}>
          {recomputing ? (
            <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
          ) : (
            <RefreshCw className="h-4 w-4" aria-hidden="true" />
          )}
          Reconciliar com as comissões
        </Button>
      </div>
    </div>
  );
}
