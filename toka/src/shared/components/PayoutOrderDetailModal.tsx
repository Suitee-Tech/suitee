import { useForm } from 'react-hook-form';
import type { Control, UseFormSetValue } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useState, type FormEvent } from 'react';
import { FileCheck2, ShieldCheck, UserRound, WalletCards } from 'lucide-react';
import { toast } from 'sonner';

import { Button } from '../ui/button';
import { Form } from '../ui/form';
import { SelectField, type SelectOption } from '../ui/fields';
import { useProofUpload } from '../hooks/useProofUpload';
import { useProofPolicy } from '../hooks/useProofPolicy';
import { isLikelyProofURL, proofErrorStageMessage } from '../utils/proof';
import { FinancialProofCard } from './lancamentos/FinancialProofCard';
import { ProofAttachmentFields } from './lancamentos/ProofAttachmentFields';
import { RentalFundingProgressCard } from './lancamentos/RentalFundingProgressCard';
import { ConfirmDialog } from './ConfirmDialog';
import {
  FinancialActionNotice,
  FinancialActionPanel,
  PanelSection,
} from './finance/FinancialActionPanel';
import { useCan } from '../permissions/usePermission';
import { PayoutOrder, payoutOrderService } from '../services/payoutOrder.service';
import { formatMoneyBR, formatCompetencia } from '../utils/format';
import { payoutCoverageCopy, payoutOriginLabel, payoutStatusLabel } from '../lib/payoutOrderContext';
import { notifyApiError } from "../services/errorToast";

interface PayoutOrderDetailModalProps {
  order: PayoutOrder;
  onClose: () => void;
  onUpdated: () => void;
  /**
   * Chamado depois de regerar: o painel passa a mostrar a ordem NOVA (a
   * cancelada vira histórico), sem o operador precisar fechar e procurar a
   * ordem na lista.
   */
  onRegenerated?: (order: PayoutOrder) => void;
}

// Rótulos dos descontos que reduzem a comissão (o valor da ordem é o líquido).
const DEDUCTION_LABEL: Record<string, string> = {
  IRRF: 'IRRF retido',
  INSS: 'INSS retido',
  ISS: 'ISS retido',
  OUTROS: 'Outro desconto',
  ADVANCE: 'Adiantamento abatido',
};



// O "chip" do ícone no cabeçalho acompanha o status: verde quando a ordem já foi
// paga e vermelho quando encerrou sem pagamento. Tokens de status, nunca cores
// cruas de paleta.
const ICON_CLASS: Record<PayoutOrder['status'], string> = {
  DRAFT: 'bg-primary-50 text-primary-700',
  PENDING_REVIEW: 'bg-primary-50 text-primary-700',
  APPROVED: 'bg-primary-50 text-primary-700',
  PAID: 'bg-status-ok-soft text-status-ok',
  FAILED: 'bg-status-critical-soft text-status-critical',
  CANCELLED: 'bg-status-critical-soft text-status-critical',
};

const SETTLEMENT_METHODS: SelectOption[] = [
  { value: 'MANUAL_PIX', label: 'PIX manual' },
  { value: 'MANUAL_TED', label: 'TED manual' },
  { value: 'OTHER', label: 'Outro' },
];

const schema = z.object({
  method: z.enum(['MANUAL_PIX', 'MANUAL_TED', 'OTHER']),
  proof_url: z.string().refine(isLikelyProofURL, 'URL deve começar com http:// ou https://'),
  note: z.string(),
});
type FormValues = z.infer<typeof schema>;

// Painel de detalhe da ordem de pagamento, que também faz a BAIXA com
// comprovante — o gesto mais arriscado do fluxo, porque registra um pagamento
// feito fora do sistema e a evidência é o que separa "paguei" de "pretendo
// pagar".
//
// O shell é o padrão das ações financeiras (`FinancialActionPanel`): painel
// lateral direito, cabeçalho com o "o quê" e o "de quem", corpo em seções
// nomeadas e rodapé FIXO com a ação primária, sempre alcançável no celular. A
// ação primária acompanha o status da ordem — aprovar (PENDING_REVIEW), marcar
// como pago (APPROVED) ou apenas fechar (status terminal) — e o corpo do painel
// é o próprio `<form>`, submetido pelo botão do rodapé (`formId`).
//
// O comprovante aceita ARQUIVO (enviado para o armazenamento do projeto) ou
// link. Antes só havia o campo de URL livre: um link do Drive/Dropbox não é
// exibível dentro do sistema, então a evidência existia e não podia ser
// conferida — o visualizador não consegue determinar o formato de um documento
// externo. Com o upload, o comprovante fica no armazenamento da empresa e é
// sempre visualizável.
//
// `ConfirmDialog` fica reservado à única ação crítica do painel: cancelar a
// ordem. Preencher e confirmar a baixa é papel do painel, nunca de um diálogo
// sobreposto.
export function PayoutOrderDetailModal({ order, onClose, onUpdated, onRegenerated }: PayoutOrderDetailModalProps) {
  const can = useCan();
  const [saving, setSaving] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [cancelConfirmOpen, setCancelConfirmOpen] = useState(false);

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: 'onTouched',
    defaultValues: { method: 'MANUAL_PIX', proof_url: '', note: '' },
  });

  // Máquina de estado compartilhada do comprovante (N-06): reaproveita o arquivo
  // já enviado entre tentativas e sinaliza falha de armazenamento separadamente
  // da falha de gravação.
  const proof = useProofUpload({
    purpose: 'payout',
    onFileAccepted: () => {
      // Com arquivo anexado, o link deixa de ser a fonte.
      form.setValue('proof_url', '', { shouldValidate: false });
      setSubmitError(null);
    },
    onFileCleared: () => setSubmitError(null),
  });

  // A política da empresa decide se a baixa manual exige comprovante. O backend
  // aplica a mesma regra (`requireManualProof`), então a exigência mostrada aqui
  // é a antecipação dela — antes a tela exigia comprovante sempre, e desligar a
  // configuração não tinha efeito nenhum.
  const { proofRequired } = useProofPolicy();

  const fundingRequiredCents = order.funding_required_cents ?? 0;
  const fundedAmountCents = order.funded_amount_cents ?? 0;
  const fundingOutstandingCents = Math.max(0, fundingRequiredCents - fundedAmountCents);
  const fundingBlocked = fundingRequiredCents > 0 && fundingOutstandingCents > 0;

  const handleApprove = async () => {
    setSaving(true);
    setSubmitError(null);
    try {
      await payoutOrderService.approve(order.id);
      toast.success('Aprovado.');
      onUpdated();
    } catch (err: any) {
      // O erro da operação aparece no mesmo lugar em todos os fluxos (o corpo do
      // painel, com `role="alert"`), sem perder o toast que já existia.
      const message = err?.response?.data?.message ?? 'Falha ao aprovar.';
      setSubmitError(message);
      toast.error(message);
    } finally {
      setSaving(false);
    }
  };

  const onSubmitMarkPaid = async (values: FormValues) => {
    const typedProofURL = values.proof_url.trim();

    if (proofRequired && !proof.file && !typedProofURL) {
      const message = 'Anexe o arquivo do comprovante ou informe o link antes de marcar como pago.';
      setSubmitError(message);
      toast.error(message);
      return;
    }

    setSaving(true);
    setSubmitError(null);

    // Etapa 1 — envio do arquivo. Uma falha aqui é do armazenamento e o arquivo
    // selecionado permanece para nova tentativa.
    let proofURL: string | undefined;
    let filename = '';
    try {
      proofURL = await proof.resolveProofURL(typedProofURL);
      // O nome vem do próprio upload: era digitado à mão, o que deixava a
      // evidência sem identificação confiável.
      if (proof.file) filename = proof.file.name;
    } catch (err) {
      const message = proofErrorStageMessage(
        err,
        'upload',
        (err as any)?.response?.data?.message ?? 'Falha ao marcar como pago.',
      );
      setSubmitError(message);
      toast.error(message);
      setSaving(false);
      return;
    }

    // Etapa 2 — gravação financeira. Não é falha de armazenamento: o
    // comprovante já está guardado e o resultado do registro é incerto, então a
    // orientação é verificar antes de repetir (a baixa não é idempotente).
    try {
      await payoutOrderService.markPaid(order.id, {
        method: values.method,
        proof_url: proofURL ?? '',
        proof_filename: filename,
        note: values.note.trim(),
      });
      toast.success('Marcado como pago.');
      onUpdated();
    } catch (err) {
      const message = proofErrorStageMessage(
        err,
        'mutation',
        (err as any)?.response?.data?.message ?? 'Falha ao marcar como pago.',
      );
      setSubmitError(message);
      toast.error(message);
    } finally {
      setSaving(false);
    }
  };

  const handleCancel = async () => {
    setSaving(true);
    try {
      await payoutOrderService.cancel(order.id);
      setCancelConfirmOpen(false);
      toast.success('Ordem de pagamento cancelada.');
      onUpdated();
    } catch (err: any) {
      notifyApiError(err, { context: "Cancelar ordem de pagamento" });
      throw err;
    } finally {
      setSaving(false);
    }
  };

  // Regerar é o caminho de volta de um cancelamento: recria a ordem (e reabre a
  // comissão vinculada) para o repasse ser lançado outra vez. Antes, cancelar
  // encerrava a obrigação sem volta e o operador ficava sem ação nenhuma na tela.
  const handleRegenerate = async () => {
    setSaving(true);
    try {
      const fresh = await payoutOrderService.regenerate(order.id);
      toast.success('Ordem regerada — aprove para lançar novamente.');
      setCancelConfirmOpen(false);
      if (onRegenerated) onRegenerated(fresh);
      else onUpdated();
    } catch (err: any) {
      notifyApiError(err, { context: "Regerar ordem de pagamento" });
    } finally {
      setSaving(false);
    }
  };

  const canApprove = can('finance:approve_transfer') && order.status === 'PENDING_REVIEW';
  const canMarkPaid = can('finance:approve_transfer') && order.status === 'APPROVED';
  const canCancel = can('finance:write') && (order.status === 'DRAFT' || order.status === 'PENDING_REVIEW' || order.status === 'APPROVED');
  const canRegenerate = can('finance:write') && order.status === 'CANCELLED';

  // A aprovação só é liberada quando a cobertura de caixa está completa. Com a
  // cobertura em aberto o botão continua no rodapé, desabilitado, ao lado do
  // aviso que explica o que falta — antes a ação simplesmente desaparecia e
  // sobrava só uma dica escondida no rodapé.
  const approvalBlocked = canApprove && fundingBlocked;

  // O rodapé fixo tem UMA ação primária, escolhida pelo status: baixar, aprovar
  // ou apenas fechar o painel. Antes as três viviam em lugares diferentes do
  // Sheet (algumas no corpo, outras no rodapé).
  const primaryAction: 'mark-paid' | 'approve' | 'close' = canMarkPaid
    ? 'mark-paid'
    : canApprove
      ? 'approve'
      : 'close';

  const formId = `payout-order-${order.id}`;
  // Rótulo, ícone e variante da ação primária, pelo status da ordem.
  const primary = {
    'mark-paid': {
      label: 'Marcar como pago',
      icon: <FileCheck2 className="size-4" />,
      variant: 'success' as const,
    },
    approve: {
      label: 'Aprovar pagamento',
      icon: <ShieldCheck className="size-4" />,
      variant: 'default' as const,
    },
    close: { label: 'Fechar', icon: undefined, variant: 'default' as const },
  }[primaryAction];

  const submitting = saving || proof.uploading;
  const submittingLabel = proof.uploading
    ? 'Enviando comprovante…'
    : primaryAction === 'approve'
      ? 'Aprovando…'
      : 'Salvando…';

  const handleFormSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (primaryAction === 'approve') {
      void handleApprove();
      return;
    }
    if (primaryAction === 'close') {
      onClose();
      return;
    }
    void form.handleSubmit(onSubmitMarkPaid)(event);
  };

  const originLabel = payoutOriginLabel(order);
  const statusLabel = payoutStatusLabel(order);
  const coverageCopy = payoutCoverageCopy(order);

  return (
    <>
      <FinancialActionPanel
        onClose={onClose}
        title="Ordem de pagamento"
        subtitle={`${order.recipient_label || order.recipient_id || 'Destinatário não informado'} · ${statusLabel}`}
        icon={<WalletCards className="size-5" />}
        iconClassName={ICON_CLASS[order.status]}
        formId={formId}
        submitLabel={primary.label}
        submitIcon={primary.icon}
        submitVariant={primary.variant}
        submitting={submitting}
        submittingLabel={submittingLabel}
        disableSubmit={approvalBlocked}
        error={submitError}
        ariaLabel="Ordem de pagamento"
        showCancel={primaryAction !== 'close'}
        secondaryAction={
          <>
            {/* Mesma dica que já existia no rodapé quando o caixa recebido ainda
                não cobre a ordem. */}
            {fundingBlocked && (
              <span className="hidden items-center gap-1.5 text-xs font-medium text-muted-foreground sm:flex">
                <WalletCards className="size-3.5" /> Aguardando recebimentos
              </span>
            )}
            {/* Ação crítica do painel: encerra a ordem sem pagamento. É o único
                ponto do fluxo que pede confirmação em diálogo sobreposto. */}
            {canCancel && (
              <Button
                type="button"
                variant="outline"
                onClick={() => setCancelConfirmOpen(true)}
                disabled={submitting}
              >
                Cancelar ordem
              </Button>
            )}
            {canRegenerate && (
              <Button
                type="button"
                variant="outline"
                onClick={handleRegenerate}
                disabled={submitting || saving}
                title="Cria uma nova ordem a partir desta, reabrindo a comissão vinculada para lançar de novo."
              >
                Regerar ordem
              </Button>
            )}
          </>
        }
      >
        <Form {...form}>
          {/* O corpo é o formulário da baixa; o botão do rodapé fixo o submete
              pelo `formId`. Nos status em que não há baixa, o mesmo form carrega
              a aprovação (ou o fechamento) para o rodapé continuar com uma única
              ação primária. */}
          <form id={formId} onSubmit={handleFormSubmit} className="space-y-5 text-sm">
            <PanelSection title="Dados da ordem" description="Destinatário, valor contratual e origem da obrigação.">
                {order.commission && (
                  <div className="rounded-xl border border-border bg-card p-4 space-y-2" data-testid="commission-breakdown">
                    <div className="flex items-start justify-between gap-4">
                      <div>
                        <p className="text-xs font-semibold text-muted-foreground">Valor da comissão</p>
                        <p className="text-lg font-bold text-foreground">{formatMoneyBR(order.commission.gross_cents / 100)}</p>
                        <p className="text-xs text-muted-foreground">{order.commission.role ? `Comissão ${order.commission.role}` : 'Comissão do corretor'}</p>
                      </div>
                      <div className="shrink-0 text-right">
                        <p className="text-xs font-semibold text-muted-foreground">Valor desta ordem</p>
                        <p className="text-lg font-bold text-status-ok">{formatMoneyBR(order.commission.net_cents / 100)}</p>
                        <p className="text-xs text-muted-foreground">líquido após descontos</p>
                      </div>
                    </div>
                    {(order.commission.deductions ?? []).length > 0 && (
                      <dl className="border-t border-border pt-2 space-y-1">
                        {(order.commission.deductions ?? []).map((deduction) => (
                          <div key={deduction.id ?? `${deduction.kind}-${deduction.amount_cents}`} className="flex items-center justify-between gap-3 text-xs">
                            <dt className="text-muted-foreground">
                              {DEDUCTION_LABEL[deduction.kind] ?? deduction.kind}
                              {deduction.rate_pct ? ` · ${deduction.rate_pct}%` : ''}
                              {deduction.description ? ` — ${deduction.description}` : ''}
                            </dt>
                            <dd className="font-medium text-status-critical">− {formatMoneyBR(deduction.amount_cents / 100)}</dd>
                          </div>
                        ))}
                      </dl>
                    )}
                  </div>
                )}
              <div className="flex items-start justify-between gap-4 rounded-xl border border-border bg-card p-4">
                <div className="flex min-w-0 items-start gap-3">
                  <span className="rounded-lg bg-primary/10 p-2 text-primary">
                    <UserRound className="size-4" />
                  </span>
                  <div className="min-w-0">
                    <p className="text-xs font-semibold text-muted-foreground">Destinatário</p>
                    <p className="truncate font-bold text-foreground">{order.recipient_label || order.recipient_id || '—'}</p>
                    <p className="text-xs text-muted-foreground">{order.recipient_type === 'broker' ? 'Corretor' : order.recipient_type}</p>
                  </div>
                </div>
                <div className="shrink-0 text-right">
                  <p className="text-xs font-semibold text-muted-foreground">Valor contratual</p>
                  <p className="font-mono text-lg font-bold text-foreground">{formatMoneyBR(order.amount_cents / 100)}</p>
                  <span className="mt-1 inline-flex rounded-full border border-border bg-muted px-2 py-0.5 text-xs font-bold text-foreground">
                    {statusLabel}
                  </span>
                </div>
              </div>

              <dl className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div className="min-w-0">
                  <dt className="text-xs text-muted-foreground">Origem</dt>
                  <dd className="font-medium text-foreground">{originLabel}</dd>
                  {order.source_id && (
                    <dd className="break-all font-mono text-xs text-muted-foreground">{order.source_id}</dd>
                  )}
                </div>
                <div className="min-w-0">
                  <dt className="text-xs text-muted-foreground">Status</dt>
                  <dd className="font-bold text-foreground">{statusLabel}</dd>
                </div>
                <div className="min-w-0 sm:col-span-2">
                  <dt className="text-xs text-muted-foreground">Identificador da ordem</dt>
                  <dd className="break-all font-mono text-xs font-medium text-foreground">{order.id}</dd>
                </div>
              </dl>
            </PanelSection>

            <PanelSection title={coverageCopy.sectionTitle} description={coverageCopy.description}>
              <RentalFundingProgressCard
                requiredCents={fundingRequiredCents}
                fundedCents={fundedAmountCents}
                entries={order.funding_entries}
                hasReceivedRent={Boolean(order.funding_entries?.length)}
                title={coverageCopy.title}
                description={coverageCopy.description}
                emptyDescription={coverageCopy.emptyDescription}
                emptyBadge={coverageCopy.emptyBadge}
                emptyHint={coverageCopy.emptyHint}
                emptyOutstandingHint={coverageCopy.emptyOutstandingHint}
                completedDescription={coverageCopy.completedDescription}
                ariaLabel={coverageCopy.ariaLabel}
                valueLabel={coverageCopy.valueLabel}
                coveredLabel={coverageCopy.coveredLabel}
                outstandingLabel={coverageCopy.outstandingLabel}
              />

              {fundingBlocked && (
                <FinancialActionNotice>
                  <span className="font-bold">{coverageCopy.blockedPrefix}</span>
                  {formatMoneyBR(fundingOutstandingCents / 100)}
                  {coverageCopy.blockedSuffix}
                </FinancialActionNotice>
              )}
            </PanelSection>

            {order.source_ref && (
              <PanelSection title="Contexto da obrigação">
                <div className="rounded-xl border border-border bg-muted/30 p-4 text-xs">
                  <dl className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    {order.contract_id && <SourceFact label="Contrato" value={order.contract_id} />}
                    {typeof order.source_ref.deal_id === 'string' && <SourceFact label="Negócio" value={order.source_ref.deal_id} />}
                    {typeof order.source_ref.reference_month === 'string' && <SourceFact label="Competência" value={formatCompetencia(order.source_ref.reference_month)} />}
                    {typeof order.source_ref.split_pct === 'number' && <SourceFact label="Participação" value={`${order.source_ref.split_pct}%`} />}
                  </dl>
                </div>
              </PanelSection>
            )}

            {order.history && order.history.length > 0 && (
              <PanelSection title="Histórico">
                <ul className="space-y-1 text-xs">
                  {order.history.map((h, idx) => (
                    <li key={idx}>
                      <span className="font-mono">{new Date(h.at).toLocaleString('pt-BR')}</span>{' '}
                      <span className="font-bold">{h.action}</span>
                      {h.actor && <span className="text-muted-foreground"> por {h.actor}</span>}
                      {h.note && <span className="text-muted-foreground"> — {h.note}</span>}
                    </li>
                  ))}
                </ul>
              </PanelSection>
            )}

            {canMarkPaid && (
              <PanelSection title="Dados do pagamento" description="Baixa manual, registrada fora do sistema.">
                <SelectField
                  control={form.control}
                  name="method"
                  label="Método"
                  required
                  options={SETTLEMENT_METHODS}
                />
              </PanelSection>
            )}

            {canMarkPaid && (
              <PanelSection
                title="Comprovante"
                description="Anexe o arquivo (ou informe o link) para o registro ficar auditável."
              >
                {/* Componente padrão compartilhado por TODOS os fluxos de baixa. */}
                <ProofAttachmentFields
                  control={form.control as Control<any>}
                  setValue={form.setValue as UseFormSetValue<any>}
                  upload={proof}
                />
                {proofRequired && (
                  <FinancialActionNotice>
                    <span className="font-bold">Comprovante obrigatório. </span>
                    A configuração da imobiliária exige evidência para concluir baixas manuais; sem arquivo ou link o
                    servidor recusa o registro.
                  </FinancialActionNotice>
                )}
              </PanelSection>
            )}

            {order.proof_url && (
              <PanelSection
                title={canMarkPaid ? 'Comprovante já registrado' : 'Comprovante'}
                description="Evidência registrada nesta baixa."
              >
                <FinancialProofCard url={order.proof_url} filename={order.proof_filename} />
              </PanelSection>
            )}

            {!canMarkPaid && !canApprove && !canCancel && (
              <PanelSection title="Ações">
                <p className="text-xs text-muted-foreground">Nenhuma ação disponível para este status.</p>
              </PanelSection>
            )}
          </form>
        </Form>
      </FinancialActionPanel>

      <ConfirmDialog
        open={cancelConfirmOpen}
        title="Cancelar ordem de pagamento"
        description={`A ordem para ${order.recipient_label || order.recipient_id || 'o destinatário informado'} será encerrada sem registrar pagamento.`}
        confirmLabel="Cancelar ordem"
        variant="warning"
        onConfirm={handleCancel}
        onCancel={() => setCancelConfirmOpen(false)}
      />
    </>
  );
}

function SourceFact({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs font-semibold text-muted-foreground">{label}</dt>
      <dd className="truncate font-mono text-xs font-medium text-foreground" title={value}>{value}</dd>
    </div>
  );
}
