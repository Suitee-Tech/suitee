import { useCallback, useMemo, useState } from 'react';
import { Link } from 'react-router';
import { Ban, CheckCircle2, HandCoins, Plus, Wallet } from 'lucide-react';
import { clsx } from 'clsx';

import { formatISODateBR, formatMoneyBR } from '../../utils/format';
import { useFinancialRead } from '../../hooks/useFinancialRead';
import { useAgentsContext } from '../../context/AgentsContext';
import { FinanceLoadError } from '../../components/finance/FinanceLoadError';
import { BrokerAdvancePanel } from '../../components/finance/BrokerAdvancePanel';
import { CancelBrokerAdvancePanel } from '../../components/finance/CancelBrokerAdvancePanel';
import { Button } from '../../ui/button';
import { ResponsiveTable } from '../../ui/responsive-table';
import { PageState, PageStateSkeleton } from '../../ui/page-state';
import {
  advanceOutstandingCents,
  brokerAdvanceService,
  totalOutstandingCents,
  totalSettledCents,
  type BrokerAdvance,
  type BrokerAdvanceStatus,
} from '../../services/brokerAdvance.service';

const STATUS_LABEL: Record<BrokerAdvanceStatus, string> = {
  OPEN: 'Em aberto',
  SETTLED: 'Quitado',
  CANCELLED: 'Cancelado',
};

const STATUS_PILL: Record<BrokerAdvanceStatus, string> = {
  OPEN: 'bg-status-info-soft text-status-info border-status-info/30',
  SETTLED: 'bg-status-ok-soft text-status-ok border-status-ok/30',
  CANCELLED: 'bg-muted text-muted-foreground border-border',
};

type StatusFilter = BrokerAdvanceStatus | 'ALL';

const STATUS_OPTIONS: { value: StatusFilter; label: string }[] = [
  { value: 'ALL', label: 'Todos os status' },
  { value: 'OPEN', label: 'Em aberto' },
  { value: 'SETTLED', label: 'Quitados' },
  { value: 'CANCELLED', label: 'Cancelados' },
];

const formatCents = (cents?: number) => formatMoneyBR((cents ?? 0) / 100);

/**
 * Um adiantamento só pode ser cancelado enquanto NADA foi abatido: os
 * abatimentos vivem nos descontos das comissões e desfazê-los mudaria o líquido
 * já lançado. O backend responde 422 nesse caso — a tela nem oferece a ação.
 */
const canCancel = (advance: BrokerAdvance): boolean =>
  advance.status === 'OPEN' && advance.settled_cents === 0;

/**
 * Aba "Adiantamentos": o dinheiro que a imobiliária adiantou ao corretor e
 * recupera abatendo nas comissões seguintes.
 *
 * A tela registra a CONCESSÃO (dinheiro saindo do caixa) e lê o saldo a
 * recuperar; o abatimento em si NÃO acontece aqui — ele é um desconto do tipo
 * "Adiantamento" lançado na comissão do corretor (Financeiro → Comissões →
 * Descontos), que consome o saldo do mais antigo para o mais novo. Por isso o
 * estado vazio aponta para lá em vez de oferecer um botão que não existe.
 */
export function AdiantamentosTab() {
  const [brokerFilter, setBrokerFilter] = useState('ALL');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('ALL');
  const [creating, setCreating] = useState(false);
  const [cancelling, setCancelling] = useState<BrokerAdvance | null>(null);

  // Nomes de corretor vêm do contexto compartilhado (mesma fonte da aba de
  // comissões). Sem ele — quem não tem `brokers:read` — o id continua sendo
  // exibido: a tela nunca inventa um rótulo.
  const { agents } = useAgentsContext();
  const brokers = useMemo(() => agents.map((agent) => ({ id: agent.id, name: agent.name })), [agents]);

  const load = useCallback(
    () =>
      brokerAdvanceService.list({
        broker_id: brokerFilter === 'ALL' ? undefined : brokerFilter,
        status: statusFilter === 'ALL' ? undefined : statusFilter,
        limit: 100,
      }),
    [brokerFilter, statusFilter],
  );

  // A chave carrega o filtro inteiro: uma resposta atrasada de um filtro antigo
  // nunca publica dados (nem limpa o erro) do filtro atual.
  const read = useFinancialRead(`broker-advances/${brokerFilter}/${statusFilter}`, load);
  // `?? []` a cada render trocaria a identidade da lista e invalidaria as
  // derivações abaixo (opções do filtro) sem necessidade.
  const items = useMemo(() => read.data?.items ?? [], [read.data]);
  const balances = read.data?.balances;
  const refresh = read.retry;

  const brokerLabel = useCallback(
    (brokerId: string) => {
      const agent = agents.find((candidate) => candidate.id === brokerId);
      return agent?.name?.trim() || brokerId;
    },
    [agents],
  );

  // Opções do filtro: todos os corretores conhecidos + os ids que aparecem na
  // lista (um corretor pode ter saído do CRM e continuar com saldo em aberto).
  const brokerOptions = useMemo(() => {
    const options = new Map<string, string>();
    brokers.forEach((broker) => options.set(broker.id, broker.name?.trim() || broker.id));
    items.forEach((advance) => {
      if (!options.has(advance.broker_id)) options.set(advance.broker_id, brokerLabel(advance.broker_id));
    });
    return Array.from(options, ([id, name]) => ({ id, name })).sort((a, b) => a.name.localeCompare(b.name));
  }, [brokers, items, brokerLabel]);

  // Totais da empresa, calculados no servidor (`meta.balances`) — não são a soma
  // da página nem mudam com o filtro.
  const outstandingCents = totalOutstandingCents(balances);
  const settledCents = totalSettledCents(balances);
  const openCount = (balances ?? []).reduce((sum, entry) => sum + (entry.open_count ?? 0), 0);
  const filtered = brokerFilter !== 'ALL' || statusFilter !== 'ALL';

  if (read.error) return <FinanceLoadError message={read.error} onRetry={refresh} />;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <p className="max-w-2xl text-xs text-muted-foreground">
          Saldo que a imobiliária adiantou ao corretor e ainda tem a recuperar, com o que já foi abatido nas comissões.
        </p>
        <Button onClick={() => setCreating(true)}>
          <Plus className="h-4 w-4" aria-hidden="true" /> Novo adiantamento
        </Button>
      </div>

      {/* Totais do tenant */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <TotalCard
          icon={Wallet}
          label="Saldo a recuperar"
          value={formatCents(outstandingCents)}
          sub={`${openCount} ${openCount === 1 ? 'adiantamento em aberto' : 'adiantamentos em aberto'}`}
          tone="warning"
        />
        <TotalCard
          icon={CheckCircle2}
          label="Já abatido"
          value={formatCents(settledCents)}
          sub="recuperado em descontos nas comissões"
          tone="ok"
        />
      </div>
      <p className="text-xs text-muted-foreground">
        Totais da empresa, calculados no servidor: não mudam com o filtro da lista abaixo.
      </p>

      {/* Filtros */}
      <div className="space-y-3 rounded-xl border border-border bg-card p-4">
        <div className="flex flex-wrap items-end gap-3">
          <div>
            <label htmlFor="advance-broker-filter" className="mb-1 block text-xs font-bold text-muted-foreground">
              Corretor
            </label>
            <select
              id="advance-broker-filter"
              value={brokerFilter}
              onChange={(event) => setBrokerFilter(event.target.value)}
              className="max-w-xs rounded-lg border border-border bg-muted px-3 py-2 text-sm outline-none focus:border-primary-500 focus:bg-card focus:ring-2 focus:ring-primary-500/20"
            >
              <option value="ALL">Todos os corretores</option>
              {brokerOptions.map((broker) => (
                <option key={broker.id} value={broker.id}>
                  {broker.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label htmlFor="advance-status-filter" className="mb-1 block text-xs font-bold text-muted-foreground">
              Status
            </label>
            <select
              id="advance-status-filter"
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
          {filtered && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setBrokerFilter('ALL');
                setStatusFilter('ALL');
              }}
            >
              Limpar filtros
            </Button>
          )}
        </div>
        <p className="text-xs text-muted-foreground">
          Mostrando {items.length} {items.length === 1 ? 'adiantamento' : 'adiantamentos'}
          {brokerFilter !== 'ALL' ? ` de ${brokerLabel(brokerFilter)}` : ''}.
        </p>
      </div>

      {/* Tabela */}
      <div className="overflow-hidden rounded-xl border border-border bg-card">
        <div className="overflow-x-auto">
          {read.loading ? (
            <PageState loading skeleton={<PageStateSkeleton rows={3} />}>{null}</PageState>
          ) : (
            <ResponsiveTable
              data={items}
              caption="Adiantamentos a corretores"
              getRowKey={(advance) => advance.id}
              empty={<EmptyState filtered={filtered} onCreate={() => setCreating(true)} />}
              columns={[
                { key: "broker", header: "Corretor", primary: true, cell: (advance) => brokerLabel(advance.broker_id) },
                { key: "amount", header: "Valor", className: "text-right", cell: (advance) => <span className="font-mono">{formatCents(advance.amount_cents)}</span> },
                { key: "settled", header: "Abatido", className: "text-right", cell: (advance) => <span className="font-mono text-muted-foreground">{formatCents(advance.settled_cents)}</span> },
                {
                  key: "outstanding",
                  header: "Saldo",
                  className: "text-right",
                  cell: (advance) => {
                    const outstanding = advanceOutstandingCents(advance);
                    return <span className={clsx("font-mono font-bold", outstanding > 0 ? "text-status-warning" : "text-muted-foreground")}>{formatCents(outstanding)}</span>;
                  },
                },
                { key: "date", header: "Data", cell: (advance) => formatISODateBR(advance.granted_at) || "—" },
                {
                  key: "status",
                  header: "Status",
                  cell: (advance) => (
                    <span className={clsx("inline-flex rounded-full border px-2 py-0.5 text-xs font-bold", STATUS_PILL[advance.status] ?? STATUS_PILL.CANCELLED)}>
                      {STATUS_LABEL[advance.status] ?? advance.status}
                    </span>
                  ),
                },
                {
                  key: "actions",
                  header: "Ações",
                  className: "text-right",
                  cell: (advance) => canCancel(advance) ? (
                    <Button variant="outline" size="sm" onClick={() => setCancelling(advance)} aria-label={`Cancelar adiantamento de ${formatCents(advance.amount_cents)} de ${brokerLabel(advance.broker_id)}`}>
                      <Ban className="h-4 w-4" aria-hidden="true" /> Cancelar
                    </Button>
                  ) : advance.settled_cents > 0 ? (
                    <span className="text-xs text-muted-foreground">já tem abatimento</span>
                  ) : (
                    <span className="text-xs text-muted-foreground">—</span>
                  ),
                },
              ]}
            />
          )}
        </div>
      </div>

      {creating && (
        <BrokerAdvancePanel brokers={brokers} onClose={() => setCreating(false)} onCreated={() => refresh()} />
      )}

      {cancelling && (
        <CancelBrokerAdvancePanel
          advance={cancelling}
          brokerLabel={brokerLabel(cancelling.broker_id)}
          onClose={() => setCancelling(null)}
          onCancelled={() => refresh()}
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
  tone: 'warning' | 'ok';
}) {
  const cls =
    tone === 'warning'
      ? 'bg-status-warning-soft text-status-warning border-status-warning/30'
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
 * Estado vazio: o saldo do adiantamento só é recuperado por um desconto lançado
 * na comissão do corretor, então a saída daqui é o caminho para as comissões —
 * não uma ação local que não existiria no backend.
 */
function EmptyState({ filtered, onCreate }: { filtered: boolean; onCreate: () => void }) {
  return (
    <div className="mx-auto max-w-xl space-y-3 text-left">
      <div className="flex items-start gap-3">
        <div className="rounded-xl border border-status-info/30 bg-status-info-soft p-2 text-status-info">
          <HandCoins className="h-5 w-5" aria-hidden="true" />
        </div>
        <div className="space-y-1">
          <p className="text-sm font-bold text-foreground">
            {filtered ? 'Nenhum adiantamento neste filtro' : 'Nenhum adiantamento registrado'}
          </p>
          <p className="text-xs leading-relaxed text-muted-foreground">
            O adiantamento é o dinheiro que a imobiliária entrega ao corretor antes das comissões. Ele vira um{' '}
            <span className="font-bold">saldo a recuperar</span>, abatido aos poucos nas comissões seguintes.
          </p>
          <p className="text-xs leading-relaxed text-muted-foreground">
            Registre a concessão em <span className="font-bold">Novo adiantamento</span>. A recuperação acontece ao
            lançar um desconto do tipo <span className="font-bold">Adiantamento</span> na comissão do corretor
            (Financeiro → Comissões → Descontos), que consome o saldo do mais antigo para o mais novo.
          </p>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2 pl-12">
        <Button variant="outline" size="sm" onClick={onCreate}>
          <Plus className="h-4 w-4" aria-hidden="true" /> Novo adiantamento
        </Button>
        <Button asChild variant="ghost" size="sm">
          <Link to="/financeiro/comissoes">Abrir comissões</Link>
        </Button>
      </div>
    </div>
  );
}
