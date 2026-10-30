import { listAllPayouts } from "../services/financialLists";
import { useFinancialRead } from "../hooks/useFinancialRead";
import { FinanceLoadError } from "./finance/FinanceLoadError";
import { useCallback, useEffect, useMemo, useState } from 'react';
import { Check, Group } from 'lucide-react';
import { toast } from 'sonner';
import {
  PayoutOrder,
  PayoutStatus,
  payoutOrderService,
} from '../services/payoutOrder.service';
import { formatMoneyBR } from '../utils/format';
import { Button } from '../ui/button';
import { ResponsiveTable } from '../ui/responsive-table';
import { PageState, PageStateSkeleton } from '../ui/page-state';
import { PayoutOrderDetailModal } from './PayoutOrderDetailModal';
import { ConfirmDialog } from './ConfirmDialog';
import { useCan } from '../permissions/usePermission';
import { payoutCoveragePendingLabel, payoutOriginLabel, payoutStatusLabel } from '../lib/payoutOrderContext';

const STATUS_FILTERS: Array<{ value: PayoutStatus | ''; label: string }> = [
  { value: '', label: 'Todos' },
  { value: 'DRAFT', label: 'Aguardando cobertura' },
  { value: 'PENDING_REVIEW', label: 'Aguardando revisão' },
  { value: 'APPROVED', label: 'Aprovados' },
  { value: 'PAID', label: 'Pagos' },
  { value: 'CANCELLED', label: 'Cancelados' },
];

const STATUS_LABEL: Record<PayoutStatus, string> = {
  DRAFT: 'Aguardando cobertura',
  PENDING_REVIEW: 'Aguardando revisão',
  APPROVED: 'Aprovado',
  PAID: 'Pago',
  FAILED: 'Falhou',
  CANCELLED: 'Cancelado',
};

// Surfaces the split_payouts module that the operator works with daily.
// Two affordances drive the table: status filters (default
// PENDING_REVIEW) and bulk selection. With selection the toolbar enables
// Grouping (creates a group_id consolidating several payouts) and bulk
// Approve. Individual rows expose details + MarkPaid via the slide-over.
export function PayoutOrderTable() {
  const can = useCan();
  const canGroup = can('finance:write');
  const canApprove = can('finance:approve_transfer');
  const [status, setStatus] = useState<PayoutStatus | ''>('PENDING_REVIEW');
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const selectableFor = (order: PayoutOrder) => {
    const fundingRequired = order.funding_required_cents ?? 0;
    const fundingAmount = order.funded_amount_cents ?? 0;
    const fundingOutstanding = Math.max(0, fundingRequired - fundingAmount);
    return fundingOutstanding === 0 && (
      (order.status === 'PENDING_REVIEW' && (canGroup || canApprove)) ||
      (order.status === 'APPROVED' && canGroup)
    );
  };
  const [detailOrder, setDetailOrder] = useState<PayoutOrder | null>(null);
  const [bulkAction, setBulkAction] = useState<'group' | 'approve' | null>(null);

  const load = useCallback(() => listAllPayouts({ status: status }), [status]);
  const read = useFinancialRead(`payouts/${status}`, load);
  const items = read.data ?? [];
  const loading = read.loading;
  const refresh = read.retry;

  const toggleSelect = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const selectedItems = useMemo(
    () => items.filter((i) => selected.has(i.id)),
    [items, selected]
  );
  const approvalEligibleItems = useMemo(
    () => selectedItems.filter((order) => {
      const outstanding = Math.max(0, (order.funding_required_cents ?? 0) - (order.funded_amount_cents ?? 0));
      return order.status === 'PENDING_REVIEW' && outstanding === 0;
    }),
    [selectedItems],
  );

  const handleGroup = async () => {
    if (selectedItems.length < 2) {
      toast.info('Selecione pelo menos 2 ordens para agrupar.');
      return;
    }
    try {
      const result = await payoutOrderService.group(selectedItems.map((order) => order.id));
      toast.success(`${result.updated.length} ordens agrupadas (${result.group_id.slice(0, 8)}).`);
      setSelected(new Set());
      setBulkAction(null);
      await refresh();
    } catch (error) {
      toast.error('Falha ao agrupar.');
      throw error;
    }
  };

  const handleBulkApprove = async () => {
    if (approvalEligibleItems.length === 0) return;
    let ok = 0;
    let fail = 0;
    for (const order of approvalEligibleItems) {
      try {
        await payoutOrderService.approve(order.id);
        ok++;
      } catch {
        fail++;
      }
    }
    toast.success(`${ok} aprovados${fail ? `, ${fail} falharam` : ''}.`);
    setSelected(new Set());
    setBulkAction(null);
    await refresh();
  };

  if (read.error) return <FinanceLoadError message={read.error} onRetry={refresh} />;
  if (loading) return <p role="status">Carregando ordens de pagamento…</p>;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap gap-2 items-center">
        {STATUS_FILTERS.map((opt) => (
          <Button
            key={opt.value || 'all'}
            size="sm"
            variant="outline"
            aria-pressed={status === opt.value}
            onClick={() => {
              setStatus(opt.value as PayoutStatus | '');
              setSelected(new Set());
            }}
            className={
              status === opt.value
                ? 'border-primary bg-primary text-primary-foreground shadow-sm hover:bg-primary/90 hover:text-primary-foreground'
                : 'border-border bg-card text-foreground hover:bg-muted'
            }
          >
            {opt.label}
          </Button>
        ))}
        <div className="flex-1" />
        {selectedItems.length > 0 && (
          <>
            <span className="text-sm text-muted-foreground">{selectedItems.length} selecionado(s)</span>
            {canGroup && (
              <Button size="sm" variant="outline" onClick={() => setBulkAction('group')} disabled={selectedItems.length < 2}>
                <Group className="w-4 h-4 mr-1" /> Agrupar selecionadas
              </Button>
            )}
            {canApprove && (
              <Button size="sm" onClick={() => setBulkAction('approve')} disabled={approvalEligibleItems.length === 0}>
                <Check className="w-4 h-4 mr-1" /> Aprovar {approvalEligibleItems.length || ''} elegível(is)
              </Button>
            )}
          </>
        )}
      </div>

      <div className="overflow-x-auto rounded-md border border-border">
        {loading ? (
          <PageState loading skeleton={<PageStateSkeleton rows={3} />}>{null}</PageState>
        ) : (
          <ResponsiveTable
            data={items}
            caption="Ordens de pagamento"
            getRowKey={(order) => order.id}
            empty={(
              <div className="px-3 py-6 text-center text-sm italic text-muted-foreground">
                Nenhuma ordem {status ? `em ${(STATUS_LABEL[status] || status).toLowerCase()}` : 'cadastrada'}.
              </div>
            )}
            columns={[
              {
                key: "select",
                header: <span className="sr-only">Selecionar</span>,
                cell: (order) => (
                  <input
                    type="checkbox"
                    className="size-4 cursor-pointer rounded border-border accent-primary disabled:cursor-not-allowed disabled:opacity-40"
                    checked={selected.has(order.id)}
                    onChange={() => toggleSelect(order.id)}
                    disabled={!selectableFor(order)}
                    aria-label={selectableFor(order)
                      ? `Selecionar ordem de ${order.recipient_label || order.recipient_id || 'pagamento'}`
                      : payoutCoveragePendingLabel(
                          order,
                          Math.max(0, (order.funding_required_cents ?? 0) - (order.funded_amount_cents ?? 0)),
                        )}
                  />
                ),
              },
              {
                key: "recipient",
                header: "Destinatário",
                primary: true,
                cell: (order) => (
                  <div className="font-medium">
                    {order.recipient_label || order.recipient_id || '—'}
                    <div className="text-xs text-muted-foreground">{order.recipient_type}</div>
                  </div>
                ),
              },
              {
                key: "source",
                header: "Origem",
                cell: (order) => (
                  <span className="text-muted-foreground">{payoutOriginLabel(order)}</span>
                ),
              },
              {
                key: "amount",
                header: "Valor",
                className: "text-right",
                cell: (order) => {
                  const fundingRequired = order.funding_required_cents ?? 0;
                  const fundingAmount = order.funded_amount_cents ?? 0;
                  const fundingOutstanding = Math.max(0, fundingRequired - fundingAmount);
                  return (
                    <div className="font-mono">
                      {formatMoneyBR(order.amount_cents / 100)}
                      {fundingRequired > 0 && (
                        <div className="mt-0.5 text-caption font-sans text-muted-foreground">
                          {fundingOutstanding > 0 ? `${formatMoneyBR(fundingAmount / 100)} coberto` : 'Cobertura concluída'}
                        </div>
                      )}
                    </div>
                  );
                },
              },
              {
                key: "status",
                header: "Status",
                cell: (order) => (
                  <span className={`inline-block rounded-full px-2 py-0.5 text-xs font-bold ${
                    order.status === 'PAID' ? 'bg-status-ok-soft text-status-ok'
                      : order.status === 'APPROVED' ? 'bg-status-info-soft text-status-info'
                        : order.status === 'DRAFT' ? 'bg-status-warning-soft text-status-warning'
                          : order.status === 'CANCELLED' ? 'bg-status-critical-soft text-status-critical'
                            : 'bg-status-pending-soft text-status-pending'
                  }`}>
                    {payoutStatusLabel(order)}
                  </span>
                ),
              },
              { key: "group", header: "Grupo", cell: (order) => <span className="font-mono text-xs">{order.group_id ? order.group_id.slice(0, 8) : '—'}</span> },
              {
                key: "actions",
                header: "Ações",
                cell: (order) => <Button size="sm" variant="ghost" onClick={() => setDetailOrder(order)}>Detalhar</Button>,
              },
            ]}
          />
        )}
      </div>

      {detailOrder && (
        <PayoutOrderDetailModal
          order={detailOrder}
          onClose={() => setDetailOrder(null)}
          onUpdated={() => {
            setDetailOrder(null);
            refresh();
          }}
          onRegenerated={(fresh) => {
            // O painel continua aberto na ordem nova — é ela que o operador
            // vai aprovar em seguida; a lista se atualiza por trás.
            setDetailOrder(fresh);
            refresh();
          }}
        />
      )}

      <ConfirmDialog
        open={bulkAction !== null}
        title={bulkAction === 'group' ? 'Agrupar ordens selecionadas' : 'Aprovar ordens elegíveis'}
        description={bulkAction === 'group'
          ? `${selectedItems.length} ordens receberão o mesmo grupo de consolidação. Os registros individuais continuarão preservados.`
          : `${approvalEligibleItems.length} ordens cobertas serão aprovadas. Nenhum pagamento será marcado como realizado.`}
        confirmLabel={bulkAction === 'group' ? 'Confirmar agrupamento' : 'Confirmar aprovação'}
        variant="default"
        onConfirm={bulkAction === 'group' ? handleGroup : handleBulkApprove}
        onCancel={() => setBulkAction(null)}
      />
    </div>
  );
}
