import { listAllPayouts } from "../services/financialLists";
import { useFinancialRead } from "../hooks/useFinancialRead";
import { FinanceLoadError } from "../components/finance/FinanceLoadError";
import { useCallback, useEffect, useMemo, useState } from 'react';
import { Loader2, CheckCircle2, Wallet, Filter } from 'lucide-react';
import { toast } from 'sonner';
import { Card } from '../ui/card';
import { Button } from '../ui/button';
import { PayoutMarkPaidModal } from '../components/PayoutMarkPaidModal';
import { formatMoneyBR } from '../utils/format';
import {
  payoutOrderService,
  type PayoutOrder,
  type PayoutSettlementMethod,
  type PayoutStatus,
} from '../services/payoutOrder.service';
import { FinanceAreaNav } from '../components/finance/FinanceAreaNav';
import { PayoutViewSwitcher } from '../components/payouts/PayoutViewSwitcher';
import { Can } from '../../lib/permissions/Can';

const STATUS_LABEL: Record<PayoutStatus | string, string> = {
  DRAFT: 'Rascunho',
  PENDING_REVIEW: 'Aguardando aprovação',
  APPROVED: 'Aprovado',
  PAID: 'Pago',
  FAILED: 'Falhou',
  CANCELLED: 'Cancelado',
};

const STATUS_TONE: Record<string, string> = {
  PENDING_REVIEW: 'bg-status-warning-soft text-status-warning',
  APPROVED: 'bg-status-info-soft text-status-info',
  PAID: 'bg-status-success-soft text-status-success',
  FAILED: 'bg-status-critical-soft text-status-critical',
  CANCELLED: 'bg-muted text-muted-foreground',
  DRAFT: 'bg-muted text-muted-foreground',
};

const FILTERS: Array<{ value: PayoutStatus | ''; label: string }> = [
  { value: '', label: 'Todos' },
  { value: 'PENDING_REVIEW', label: 'Aguardando aprovação' },
  { value: 'APPROVED', label: 'Aprovados' },
  { value: 'PAID', label: 'Pagos' },
  { value: 'CANCELLED', label: 'Cancelados' },
];

const centsToReais = (v: number) => (typeof v === 'number' ? v / 100 : 0);

export default function Payouts() {
  const [statusFilter, setStatusFilter] = useState<PayoutStatus | ''>('PENDING_REVIEW');
  const [markPaidOpen, setMarkPaidOpen] = useState(false);
  const [selected, setSelected] = useState<PayoutOrder | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const load = useCallback(() => listAllPayouts({ status: statusFilter }), [statusFilter]);
  const read = useFinancialRead(`payouts/${statusFilter}`, load);
  const items = read.data ?? [];
  const loading = read.loading;
  const refresh = read.retry;

  const handleApprove = async (p: PayoutOrder) => {
    setBusyId(p.id);
    try {
      await payoutOrderService.approve(p.id);
      toast.success('Ordem de pagamento aprovada');
      await refresh();
    } catch (err) {
      console.error(err);
      toast.error('Erro ao aprovar a ordem de pagamento.');
      await refresh();
    } finally {
      setBusyId(null);
    }
  };

  const handleOpenMarkPaid = (p: PayoutOrder) => {
    setSelected(p);
    setMarkPaidOpen(true);
  };

  const handleConfirmMarkPaid = async ({
    method,
    proofUrl,
    proofFilename,
    note,
  }: {
    method: PayoutSettlementMethod;
    proofUrl: string;
    proofFilename: string;
    note?: string;
  }) => {
    if (!selected) return;
    try {
      await payoutOrderService.markPaid(selected.id, {
        method,
        proof_url: proofUrl,
        proof_filename: proofFilename,
        note,
      });
      toast.success('Ordem de pagamento marcada como paga');
      await refresh();
    } catch (err) {
      console.error(err);
      await refresh();
      // The modal owns the error toast and only closes after this promise
      // resolves. Re-throwing keeps the proof and note available for a retry
      // instead of presenting a failed payment as completed in the UI.
      throw err;
    }
  };

  const total = useMemo(
    () => items.reduce((acc, p) => acc + (p.amount_cents || 0), 0),
    [items],
  );

  if (read.error) return <FinanceLoadError message={read.error} onRetry={refresh} />;
  if (loading) return <p role="status">Carregando ordens de pagamento…</p>;

  return (
    <div className="p-6 max-w-7xl mx-auto space-y-6">
      <FinanceAreaNav />
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Wallet className="w-6 h-6 text-primary-600" />
            Ordens de pagamento — lista
          </h1>
          <p className="text-sm text-muted-foreground mt-1">
            Pagamentos de comissões, repasses e royalties que liquidam off-platform (PIX, TED).
          </p>
        </div>
        <PayoutViewSwitcher />
      </div>

      <Card className="p-4">
        <div className="flex items-center gap-2 flex-wrap">
          <Filter className="w-4 h-4 text-muted-foreground" />
          {FILTERS.map((f) => (
            <button
              key={f.value}
              type="button"
              onClick={() => setStatusFilter(f.value)}
              aria-pressed={statusFilter === f.value}
              className={`px-3 py-1.5 rounded-full border text-xs font-bold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 ${
                statusFilter === f.value
                  ? 'border-primary bg-primary text-primary-foreground shadow-sm'
                  : 'border-border bg-card text-foreground hover:bg-muted'
              }`}
            >
              {f.label}
            </button>
          ))}
          <div className="flex-1" />
          <span className="text-xs text-muted-foreground">
            {items.length} resultado{items.length === 1 ? '' : 's'} • Total{' '}
            {formatMoneyBR(centsToReais(total))}
          </span>
        </div>
      </Card>

      {loading ? (
        <div className="flex justify-center py-12">
          <Loader2 className="w-6 h-6 animate-spin text-muted-foreground" />
        </div>
      ) : items.length === 0 ? (
        <Card className="p-12 text-center">
          <Wallet className="w-12 h-12 mx-auto text-muted-foreground mb-3" />
          <p className="text-sm text-muted-foreground">Nenhuma ordem de pagamento encontrada neste filtro.</p>
        </Card>
      ) : (
        <div className="space-y-2">
          {items.map((p) => (
            <Card key={p.id} className="p-4">
              <div className="flex items-center gap-4">
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-1">
                    <span
                      className={`px-2 py-0.5 rounded-full text-caption font-semibold uppercase ${STATUS_TONE[p.status] || ''}`}
                    >
                      {STATUS_LABEL[p.status] || p.status}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {new Date(p.created_at).toLocaleDateString('pt-BR')}
                    </span>
                  </div>
                  <p className="font-semibold text-foreground truncate">
                    {p.recipient_label || p.recipient_id || '—'}
                  </p>
                  <p className="text-xs text-muted-foreground truncate">
                    {p.source_type} {p.source_id ? `• ${p.source_id}` : ''}
                    {p.contract_id ? ` • Contrato ${p.contract_id.slice(0, 8)}` : ''}
                  </p>
                </div>
                <div className="text-right">
                  <p className="font-bold text-lg">
                    {formatMoneyBR(centsToReais(p.amount_cents))}
                  </p>
                  {p.recipient_pix_key && (
                    <p className="text-xs text-muted-foreground">PIX: {p.recipient_pix_key}</p>
                  )}
                </div>
                <div className="flex gap-2 shrink-0">
                  {p.status === 'PENDING_REVIEW' && (
                    <Can permission="finance:approve_transfer">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => handleApprove(p)}
                        disabled={busyId === p.id}
                      >
                        {busyId === p.id ? (
                          <Loader2 className="w-4 h-4 animate-spin" />
                        ) : (
                          <CheckCircle2 className="w-4 h-4" />
                        )}
                        <span className="ml-2">Aprovar</span>
                      </Button>
                    </Can>
                  )}
                  {p.status === 'APPROVED' && (
                    <Can permission="finance:approve_transfer">
                      <Button
                        size="sm"
                        onClick={() => handleOpenMarkPaid(p)}
                        disabled={busyId === p.id}
                      >
                        Marcar pago
                      </Button>
                    </Can>
                  )}
                </div>
              </div>
            </Card>
          ))}
        </div>
      )}

      <PayoutMarkPaidModal
        open={markPaidOpen}
        payoutId={selected?.id || null}
        payoutLabel={selected?.recipient_label}
        onClose={() => {
          setMarkPaidOpen(false);
          setSelected(null);
        }}
        onConfirm={handleConfirmMarkPaid}
      />
    </div>
  );
}
