import { useEffect, useState } from 'react';
import { clsx } from 'clsx';
import { toast } from 'sonner';
import { Calendar, ChevronDown, Handshake } from 'lucide-react';
import { ExpandableSidePanel } from '../ExpandableSidePanel';
import { Button } from '../../ui/button';
import { MoneyInput } from '../../ui/money-input';
import {
  collectionService,
  type Collection,
  type CollectionAgreement,
} from '../../services/collection.service';
import { serverMessage } from '../../lib/serverMessage';
import { DelinquencyPanelHeader } from './DelinquencyPanelHeader';
import { PanelNotice } from './PanelNotice';
import {
  AGREEMENT_STATUS_BADGE_CLASS,
  AGREEMENT_STATUS_LABEL,
  BODY_CLASS,
  FIELD_CLASS,
  FOOTER_CLASS,
  LABEL_CLASS,
  TEXTAREA_CLASS,
  addDaysISO,
  agreementDraftError,
  agreementPlan,
  agreementPreviewText,
  agreementSchedule,
  canCreateAgreement,
  dateInputToISO,
  formatDate,
  formatDateTime,
  todayISO,
} from './delinquencyUi';
import { formatCurrencyBRL } from '../../utils/format';

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start justify-between gap-3 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-bold text-foreground text-right">{value}</span>
    </div>
  );
}

/** Primeira parcela sugerida: um mês à frente (o backend recusa data no passado). */
const defaultFirstDueDate = (): string => (addDaysISO(todayISO(), 30) ?? '').slice(0, 10);

/**
 * Acordo do caso por `POST /collections/:id/agreement` — substitui a
 * "negociação" solta que só gravava `negotiated_amount` num PUT.
 *
 * O acordo CANCELA as cobranças originais e cria as parcelas como cobranças
 * novas (`installment_charge_ids`). Isso é dito ao operador ANTES de confirmar,
 * porque pela tela não há como desfazer. Com acordo já registrado
 * (`GET /collections/:id/agreement`), o painel mostra o plano com as parcelas e
 * oferece **Quebrar acordo** (com motivo e a opção de restabelecer o desconto,
 * desmarcada por padrão) e **Cancelar acordo**.
 */
export function AgreementPanel({
  open,
  collection,
  canWrite,
  onClose,
  onSaved,
}: {
  open: boolean;
  collection: Collection | null;
  /** `charges:write` — sem ela o painel fica em modo somente leitura. */
  canWrite: boolean;
  onClose: () => void;
  onSaved: (collection: Collection) => void;
}) {
  const collectionId = collection?.id ?? null;
  const embeddedAgreement = collection?.agreement ?? null;
  const totalDebt = collection?.total_debt;

  const [agreement, setAgreement] = useState<CollectionAgreement | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [loadingAgreement, setLoadingAgreement] = useState(false);
  const [agreementError, setAgreementError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [forceCreate, setForceCreate] = useState(false);

  const [total, setTotal] = useState<number | undefined>(undefined);
  const [discount, setDiscount] = useState<number | undefined>(undefined);
  const [downPayment, setDownPayment] = useState<number | undefined>(undefined);
  const [installments, setInstallments] = useState('1');
  const [firstDueDate, setFirstDueDate] = useState(defaultFirstDueDate());
  const [intervalDays, setIntervalDays] = useState('30');
  const [notes, setNotes] = useState('');
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [touched, setTouched] = useState(false);

  const [saving, setSaving] = useState(false);
  const [breaking, setBreaking] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [breakReason, setBreakReason] = useState('');
  const [restoreDiscount, setRestoreDiscount] = useState(false);
  const [cancelReason, setCancelReason] = useState('');

  useEffect(() => {
    setTotal(totalDebt);
    setDiscount(undefined);
    setDownPayment(undefined);
    setInstallments('1');
    setFirstDueDate(defaultFirstDueDate());
    setIntervalDays('30');
    setNotes('');
    setShowAdvanced(false);
    setTouched(false);
    setBreakReason('');
    setRestoreDiscount(false);
    setCancelReason('');
    setForceCreate(false);
  }, [collectionId, open, totalDebt]);

  useEffect(() => {
    if (!open || !collectionId) {
      setAgreement(null);
      setLoaded(false);
      return;
    }
    let active = true;
    setLoadingAgreement(true);
    setAgreementError(null);
    void collectionService.getAgreement(collectionId).then(
      (data) => {
        if (!active) return;
        setAgreement(data);
        setLoaded(true);
        setLoadingAgreement(false);
      },
      (err) => {
        if (!active) return;
        setAgreementError(serverMessage(err, 'Não foi possível carregar o acordo do caso.'));
        setLoaded(false);
        setLoadingAgreement(false);
      },
    );
    return () => {
      active = false;
    };
  }, [attempt, collectionId, open]);

  const current = loaded ? agreement : embeddedAgreement;
  const draft = { totalAmount: total, discount, downPayment, installments, firstDueDate, intervalDays };
  const draftError = agreementDraftError(draft);
  const plan = agreementPlan(draft);
  const transitionAllowed = canCreateAgreement(collection?.status);
  const showCreateForm = !current || forceCreate;
  const schedule = current ? agreementSchedule(current) : [];

  const createAgreement = async () => {
    setTouched(true);
    if (!collection || !canWrite) return;
    if (draftError) {
      toast.error(draftError);
      return;
    }
    const firstDueISO = dateInputToISO(firstDueDate);
    if (!firstDueISO) {
      toast.error('Informe uma data válida para a primeira parcela.');
      return;
    }
    setSaving(true);
    try {
      const updated = await collectionService.createAgreement(collection.id, {
        total_amount: total ?? 0,
        discount: discount ?? 0,
        down_payment: downPayment ?? 0,
        installments: Number(installments),
        first_due_date: firstDueISO,
        interval_days: intervalDays.trim() ? Number(intervalDays) : 30,
        notes: notes.trim() || undefined,
      });
      onSaved(updated);
      toast.success('Acordo registrado: as cobranças originais foram substituídas pelas parcelas do acordo.');
      onClose();
    } catch (err) {
      toast.error(serverMessage(err, 'Erro ao registrar o acordo.'));
    } finally {
      setSaving(false);
    }
  };

  const breakAgreement = async () => {
    if (!collection || !current || !canWrite) return;
    if (!breakReason.trim()) {
      toast.error('Informe o motivo da quebra do acordo.');
      return;
    }
    setBreaking(true);
    try {
      const updated = await collectionService.breakAgreement(collection.id, {
        reason: breakReason.trim(),
        restore_discount: restoreDiscount,
      });
      onSaved(updated);
      toast.success('Acordo quebrado: o caso voltou para Em Cobrança e a régua retoma.');
      onClose();
    } catch (err) {
      toast.error(serverMessage(err, 'Erro ao quebrar o acordo.'));
    } finally {
      setBreaking(false);
    }
  };

  const cancelAgreement = async () => {
    if (!collection || !current || !canWrite) return;
    if (!cancelReason.trim()) {
      toast.error('Informe o motivo do cancelamento do acordo.');
      return;
    }
    setCancelling(true);
    try {
      const updated = await collectionService.cancelAgreement(collection.id, { reason: cancelReason.trim() });
      onSaved(updated);
      toast.success('Acordo cancelado.');
      onClose();
    } catch (err) {
      toast.error(serverMessage(err, 'Erro ao cancelar o acordo.'));
    } finally {
      setCancelling(false);
    }
  };

  return (
    <ExpandableSidePanel
      open={open}
      onClose={onClose}
      ariaLabel="Acordo de Dívida"
      defaultSize="lg"
      renderHeader={({ expandButton, closeButton }) => (
        <DelinquencyPanelHeader
          title="Acordo de Dívida"
          icon={<Handshake className="w-5 h-5" />}
          contractId={collection?.contract_id}
          debtorName={collection?.debtor_name}
          expandButton={expandButton}
          closeButton={closeButton}
        />
      )}
    >
      <div className={BODY_CLASS}>
        {!canWrite && (
          <PanelNotice tone="critical">
            Seu perfil não tem <strong>charges:write</strong>: o painel está em modo somente leitura.
          </PanelNotice>
        )}

        {agreementError && (
          <PanelNotice tone="critical">
            {agreementError}
            <div>
              <Button variant="outline" size="sm" onClick={() => setAttempt((value) => value + 1)} className="mt-2">
                Tentar novamente
              </Button>
            </div>
          </PanelNotice>
        )}

        {loadingAgreement && (
          <p className="text-sm text-muted-foreground">Verificando se o caso já tem acordo registrado…</p>
        )}

        {current && (
          <>
            <section className="bg-muted border border-border rounded-xl p-5 space-y-3">
              <div className="flex items-center justify-between gap-3">
                <h3 className="text-sm font-bold text-foreground">Acordo registrado</h3>
                <span
                  className={clsx(
                    'px-3 py-1 text-caption font-bold rounded-full whitespace-nowrap',
                    AGREEMENT_STATUS_BADGE_CLASS[current.status] ?? 'bg-muted text-muted-foreground',
                  )}
                >
                  {AGREEMENT_STATUS_LABEL[current.status] ?? current.status}
                </span>
              </div>
              <Field label="Valor original do caso" value={formatCurrencyBRL(current.original_total)} />
              <Field label="Valor do acordo" value={formatCurrencyBRL(current.total_amount)} />
              <Field label="Desconto concedido" value={formatCurrencyBRL(current.discount)} />
              <Field label="Entrada" value={formatCurrencyBRL(current.down_payment)} />
              <Field
                label="Parcelas"
                value={`${current.installments}x a cada ${current.interval_days} dia(s)`}
              />
              <Field label="1ª parcela" value={formatDate(current.first_due_date)} />
              <Field label="Registrado em" value={formatDateTime(current.created_at)} />
              <Field label="Registrado por" value={current.created_by || '—'} />
              {current.approval_required && (
                <Field
                  label="Aprovação"
                  value={current.approved_by ? `Aprovado por ${current.approved_by}` : 'Pendente de aprovação'}
                />
              )}
              {current.broken_at && <Field label="Quebrado em" value={formatDateTime(current.broken_at)} />}
              {current.break_reason && <Field label="Motivo da quebra" value={current.break_reason} />}
              {current.notes && <Field label="Observações" value={current.notes} />}
              <p className="text-xs text-muted-foreground pt-2 border-t border-border">
                As {current.original_charge_ids.length} cobrança(s) originais deste caso foram canceladas e
                substituídas por {current.installment_charge_ids.length} parcela(s) do acordo.
              </p>
            </section>

            <section className="space-y-3">
              <h3 className="text-sm font-bold text-foreground">Parcelas do acordo</h3>
              {schedule.length === 0 ? (
                <p className="text-sm text-muted-foreground">O servidor não informou as parcelas deste acordo.</p>
              ) : (
                <>
                  <div className="bg-card border border-border rounded-xl divide-y divide-border">
                    {schedule.map((row) => (
                      <div key={row.number} className="p-4 flex items-center justify-between gap-3">
                        <div className="min-w-0">
                          <p className="text-sm font-bold text-foreground">
                            {row.number}ª parcela{' '}
                            <span className="font-normal text-muted-foreground">venc. {formatDate(row.dueDate)}</span>
                          </p>
                          {row.chargeId && (
                            <p className="text-xs text-muted-foreground truncate">cobrança {row.chargeId}</p>
                          )}
                        </div>
                        <span className="text-sm font-bold text-foreground whitespace-nowrap">
                          {formatCurrencyBRL(row.amount)}
                        </span>
                      </div>
                    ))}
                  </div>
                  <p className="text-xs text-muted-foreground">
                    Valores por parcela calculados a partir do plano do servidor (a última absorve a diferença de
                    centavos da divisão).
                  </p>
                </>
              )}
            </section>

            {current.status === 'ACTIVE' && (
              <section className="space-y-4 pt-4 border-t border-border">
                <h3 className="text-sm font-bold text-foreground">Quebrar acordo</h3>
                <p className="text-sm text-muted-foreground">
                  Quebrar devolve o caso para <strong className="text-foreground">Em Cobrança (ACTIVE)</strong> e a
                  régua de cobrança retoma sobre o saldo.
                </p>
                <div>
                  <label className={LABEL_CLASS} htmlFor="agreement-break-reason">
                    Motivo da quebra *
                  </label>
                  <textarea
                    id="agreement-break-reason"
                    rows={2}
                    value={breakReason}
                    onChange={(event) => setBreakReason(event.target.value)}
                    placeholder="Ex.: parcela 2 não paga e sem retorno do inquilino..."
                    className={TEXTAREA_CLASS}
                  />
                </div>
                <div className="flex items-start gap-3">
                  <input
                    id="agreement-restore-discount"
                    type="checkbox"
                    checked={restoreDiscount}
                    onChange={(event) => setRestoreDiscount(event.target.checked)}
                    className="mt-0.5 size-4 rounded border-border"
                  />
                  <label htmlFor="agreement-restore-discount" className="text-sm text-foreground">
                    Restabelecer o desconto
                    <span className="block text-xs text-muted-foreground mt-1">
                      Marcado: o desconto concedido no acordo deixa de valer e a dívida volta ao valor original (
                      {formatCurrencyBRL(current.original_total)}). Desmarcado (padrão): o desconto é mantido.
                    </span>
                  </label>
                </div>
                <Button
                  variant="destructive"
                  size="sm"
                  onClick={breakAgreement}
                  loading={breaking}
                  loadingText="Quebrando..."
                  disabled={!canWrite}
                >
                  Quebrar acordo
                </Button>

                <div className="space-y-3 pt-4 border-t border-border">
                  <h3 className="text-sm font-bold text-foreground">Cancelar acordo</h3>
                  <p className="text-sm text-muted-foreground">
                    O cancelamento encerra o acordo e desfaz as parcelas geradas por ele.
                  </p>
                  <div>
                    <label className={LABEL_CLASS} htmlFor="agreement-cancel-reason">
                      Motivo do cancelamento *
                    </label>
                    <textarea
                      id="agreement-cancel-reason"
                      rows={2}
                      value={cancelReason}
                      onChange={(event) => setCancelReason(event.target.value)}
                      placeholder="Ex.: acordo registrado em duplicidade..."
                      className={TEXTAREA_CLASS}
                    />
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={cancelAgreement}
                    loading={cancelling}
                    loadingText="Cancelando..."
                    disabled={!canWrite}
                  >
                    Cancelar acordo
                  </Button>
                </div>
              </section>
            )}
          </>
        )}

        {showCreateForm && (
          <>
            {!transitionAllowed && (
              <PanelNotice tone="critical">
                Este caso está <strong>{collection?.status}</strong> e não aceita criar acordo. O acordo só é
                registrado com o caso em Em Cobrança (ACTIVE), Negociado (NEGOTIATED) ou Judicial (JUDICIAL).
              </PanelNotice>
            )}

            <PanelNotice tone="critical">
              As <strong>cobranças originais deste caso serão canceladas</strong> e substituídas pelas parcelas do
              acordo — o servidor cria uma cobrança nova por parcela. Pela tela isso não pode ser desfeito: para
              voltar atrás é preciso quebrar ou cancelar o acordo.
            </PanelNotice>

            <div className="bg-muted border border-border p-5 rounded-xl space-y-3">
              <div className="flex justify-between items-center text-sm">
                <span className="text-muted-foreground">Valor em atraso (servidor):</span>
                <span className="font-bold text-foreground">{formatCurrencyBRL(collection?.total_debt)}</span>
              </div>
              <div className="h-px bg-border w-full my-2" />
              <div className="flex justify-between items-start gap-3">
                <span className="text-sm text-muted-foreground">Prévia do acordo:</span>
                <span className="text-sm font-bold text-foreground text-right">
                  {plan ? agreementPreviewText(plan) : 'Informe o valor e o número de parcelas.'}
                </span>
              </div>
              {plan && (
                <div className="text-xs text-muted-foreground space-y-1 pt-2 border-t border-border">
                  <p>Total a receber: {formatCurrencyBRL(plan.netAmount)} (valor do acordo menos o desconto).</p>
                  <p>
                    Financiado nas parcelas: {formatCurrencyBRL(plan.financed)}
                    {plan.lastInstallmentValue !== plan.installmentValue
                      ? ` · última parcela de ${formatCurrencyBRL(plan.lastInstallmentValue)}`
                      : ''}
                    {' · intervalo de '}
                    {plan.intervalDays} dia(s).
                  </p>
                </div>
              )}
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div className="sm:col-span-2">
                <label className={LABEL_CLASS} htmlFor="agreement-total">
                  Valor Total do Acordo *
                </label>
                <MoneyInput
                  id="agreement-total"
                  value={total}
                  onChange={setTotal}
                  placeholder="0,00"
                  className={FIELD_CLASS}
                />
              </div>
              <div>
                <label className={LABEL_CLASS} htmlFor="agreement-discount">
                  Desconto
                </label>
                <MoneyInput
                  id="agreement-discount"
                  value={discount}
                  onChange={setDiscount}
                  placeholder="0,00"
                  className={FIELD_CLASS}
                />
              </div>
              <div>
                <label className={LABEL_CLASS} htmlFor="agreement-down-payment">
                  Entrada
                </label>
                <MoneyInput
                  id="agreement-down-payment"
                  value={downPayment}
                  onChange={setDownPayment}
                  placeholder="0,00"
                  className={FIELD_CLASS}
                />
              </div>
              <div>
                <label className={LABEL_CLASS} htmlFor="agreement-installments">
                  Nº de Parcelas *
                </label>
                <input
                  id="agreement-installments"
                  type="number"
                  min={1}
                  max={36}
                  step={1}
                  value={installments}
                  onChange={(event) => setInstallments(event.target.value)}
                  onBlur={() => setTouched(true)}
                  className={FIELD_CLASS}
                />
              </div>
              <div>
                <label className={LABEL_CLASS} htmlFor="agreement-first-due-date">
                  Data da 1ª Parcela *
                </label>
                <div className="relative">
                  <input
                    id="agreement-first-due-date"
                    type="date"
                    value={firstDueDate}
                    onChange={(event) => setFirstDueDate(event.target.value)}
                    className={FIELD_CLASS}
                  />
                  <Calendar className="w-4 h-4 text-muted-foreground absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none" />
                </div>
              </div>
            </div>

            <Button variant="ghost" size="sm" onClick={() => setShowAdvanced((value) => !value)} className="justify-start">
              <ChevronDown className={clsx('w-4 h-4 transition-transform', showAdvanced && 'rotate-180')} />
              {showAdvanced ? 'Ocultar opções avançadas' : 'Mostrar opções avançadas'}
            </Button>

            {showAdvanced && (
              <div>
                <label className={LABEL_CLASS} htmlFor="agreement-interval-days">
                  Intervalo entre parcelas (dias)
                </label>
                <input
                  id="agreement-interval-days"
                  type="number"
                  min={1}
                  step={1}
                  value={intervalDays}
                  onChange={(event) => setIntervalDays(event.target.value)}
                  className={FIELD_CLASS}
                />
                <p className="text-xs text-muted-foreground mt-2">Padrão do servidor: 30 dias entre parcelas.</p>
              </div>
            )}

            <div>
              <label className={LABEL_CLASS} htmlFor="agreement-notes">
                Observações
              </label>
              <textarea
                id="agreement-notes"
                rows={3}
                value={notes}
                onChange={(event) => setNotes(event.target.value)}
                placeholder="Informações adicionais sobre o acordo..."
                className={TEXTAREA_CLASS}
              />
              {collection?.notes && (
                <p className="text-xs text-muted-foreground mt-2">
                  As observações do caso são preservadas; este texto é gravado no acordo.
                </p>
              )}
            </div>

            {touched && draftError && <p className="text-xs text-status-critical">{draftError}</p>}

            {current && (
              <Button variant="outline" size="sm" onClick={() => setForceCreate(false)}>
                Voltar para o acordo registrado
              </Button>
            )}
          </>
        )}
      </div>

      <div className={FOOTER_CLASS}>
        {showCreateForm ? (
          <>
            <Button variant="outline" onClick={onClose} className="flex-1">
              Cancelar
            </Button>
            <Button
              onClick={createAgreement}
              loading={saving}
              loadingText="Registrando..."
              disabled={!transitionAllowed || !canWrite}
              className="flex-2"
            >
              Criar Acordo
            </Button>
          </>
        ) : (
          <>
            {current && (current.status === 'BROKEN' || current.status === 'CANCELLED') && transitionAllowed && (
              <Button variant="outline" onClick={() => setForceCreate(true)} disabled={!canWrite} className="flex-1">
                Novo acordo
              </Button>
            )}
            <Button variant="outline" onClick={onClose} className="flex-1">
              Fechar
            </Button>
          </>
        )}
      </div>
    </ExpandableSidePanel>
  );
}
