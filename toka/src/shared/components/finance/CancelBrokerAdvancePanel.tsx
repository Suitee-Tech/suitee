import { useState, type FormEvent } from 'react';
import { Ban } from 'lucide-react';
import { toast } from 'sonner';

import { formatISODateBR, formatMoneyBR } from '../../utils/format';
import { Textarea } from '../../ui/textarea';
import {
  advanceOutstandingCents,
  brokerAdvanceService,
  type BrokerAdvance,
} from '../../services/brokerAdvance.service';
import { FinancialActionNotice, FinancialActionPanel, PanelSection } from './FinancialActionPanel';

export interface CancelBrokerAdvancePanelProps {
  advance: BrokerAdvance;
  /** Rótulo do corretor quando conhecido; cai para o id — nunca um nome inventado. */
  brokerLabel?: string;
  onClose: () => void;
  /** Chamado com o adiantamento já cancelado. */
  onCancelled: (advance: BrokerAdvance) => void;
}

const formatCents = (cents?: number) => formatMoneyBR((cents ?? 0) / 100);

const errorMessage = (error: unknown): string => {
  const candidate = error as { response?: { data?: { message?: string } }; message?: string };
  return (
    candidate?.response?.data?.message ||
    candidate?.message ||
    'Não foi possível cancelar o adiantamento. Revise os dados e tente novamente.'
  );
};

/**
 * Cancelamento do adiantamento sem nenhum abatimento, no shell único das ações
 * financeiras.
 *
 * É confirmação de ação crítica — mas o painel é o mesmo de todos os fluxos
 * porque aqui se COLETA o motivo do cancelamento (o histórico do adiantamento
 * guarda quem cancelou e por quê). O backend recusa (422) quando já existe
 * desconto abatendo o saldo: desfazer os descontos nas comissões é o caminho.
 */
export function CancelBrokerAdvancePanel({
  advance,
  brokerLabel,
  onClose,
  onCancelled,
}: CancelBrokerAdvancePanelProps) {
  const [reason, setReason] = useState('');
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const outstanding = advanceOutstandingCents(advance);
  const label = brokerLabel?.trim() || advance.broker_id;
  const formId = `cancel-broker-advance-${advance.id}`;

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    setSubmitError(null);
    setSubmitting(true);
    try {
      const cancelled = await brokerAdvanceService.cancel(advance.id, reason.trim() || undefined);
      toast.success('Adiantamento cancelado.');
      onCancelled(cancelled);
      onClose();
    } catch (err) {
      const message = errorMessage(err);
      setSubmitError(message);
      toast.error(message);
      console.error(err);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <FinancialActionPanel
      onClose={onClose}
      title="Cancelar adiantamento"
      subtitle={`${label} · ${formatCents(advance.amount_cents)}`}
      icon={<Ban className="size-5" />}
      iconClassName="bg-status-critical-soft text-status-critical"
      formId={formId}
      submitLabel="Cancelar adiantamento"
      submitIcon={<Ban className="size-4" />}
      submitVariant="default"
      cancelLabel="Voltar"
      submitting={submitting}
      submittingLabel="Cancelando…"
      error={submitError}
      ariaLabel="Cancelar adiantamento"
    >
      <form id={formId} onSubmit={handleSubmit} className="space-y-5">
        <PanelSection title="Adiantamento" description="Confira o que será cancelado antes de confirmar.">
          <div className="space-y-1.5 rounded-xl border border-border bg-muted/30 p-4 text-sm">
            <div className="flex items-center justify-between gap-3">
              <span className="text-muted-foreground">Corretor</span>
              <span className="truncate font-bold">{label}</span>
            </div>
            <div className="flex items-center justify-between gap-3">
              <span className="text-muted-foreground">Data</span>
              <span className="font-bold">{formatISODateBR(advance.granted_at) || '—'}</span>
            </div>
            <div className="flex items-center justify-between gap-3">
              <span className="text-muted-foreground">Valor</span>
              <span className="font-bold tabular-nums">{formatCents(advance.amount_cents)}</span>
            </div>
            <div className="flex items-center justify-between gap-3">
              <span className="text-muted-foreground">Abatido</span>
              <span className="font-bold tabular-nums">{formatCents(advance.settled_cents)}</span>
            </div>
            <div className="flex items-center justify-between gap-3 border-t border-border pt-2">
              <span className="text-muted-foreground">Saldo a recuperar</span>
              <span className="font-bold tabular-nums">{formatCents(outstanding)}</span>
            </div>
            {advance.description && <p className="text-xs text-muted-foreground">{advance.description}</p>}
          </div>
        </PanelSection>

        <PanelSection title="Motivo" description="Opcional — fica registrado no histórico do adiantamento.">
          <Textarea
            aria-label="Motivo do cancelamento"
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder="ex.: valor lançado em duplicidade"
            rows={3}
          />
        </PanelSection>

        <FinancialActionNotice>
          O cancelamento só é aceito enquanto <span className="font-bold">não existe abatimento</span> lançado nas
          comissões. O lançamento do adiantamento é estornado no razão e o saldo sai do total a recuperar.
        </FinancialActionNotice>
      </form>
    </FinancialActionPanel>
  );
}
