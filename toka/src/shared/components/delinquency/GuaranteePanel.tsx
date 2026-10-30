import { useEffect, useState } from 'react';
import { toast } from 'sonner';
import { ChevronDown, Shield } from 'lucide-react';
import { ExpandableSidePanel } from '../ExpandableSidePanel';
import { Button } from '../../ui/button';
import { MoneyInput } from '../../ui/money-input';
import {
  collectionService,
  type Collection,
  type GuaranteeActivationStatus,
  type GuaranteeType,
} from '../../services/collection.service';
import { serverMessage } from '../../lib/serverMessage';
import { DelinquencyPanelHeader } from './DelinquencyPanelHeader';
import { PanelNotice } from './PanelNotice';
import {
  BODY_CLASS,
  FIELD_CLASS,
  FOOTER_CLASS,
  GUARANTEE_STATUS_LABEL,
  GUARANTEE_TYPE_LABEL,
  LABEL_CLASS,
  TEXTAREA_CLASS,
  canTransitionTo,
  formatDate,
  formatDateTime,
} from './delinquencyUi';
import { formatCurrencyBRL } from '../../utils/format';

const GUARANTEE_TYPES: GuaranteeType[] = ['SEGURO_FIANCA', 'FIADOR', 'CAUCAO'];
/**
 * Status do fluxo de RESPOSTA do garantidor. `PAID` saiu daqui de propósito: a
 * baixa por indenização exige `settled_amount` e tem seção própria
 * ("Registrar indenização recebida").
 */
const STATUS_OPTIONS: GuaranteeActivationStatus[] = ['REQUESTED', 'ACCEPTED', 'REJECTED', 'CANCELLED'];

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start justify-between gap-3 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-bold text-foreground text-right">{value}</span>
    </div>
  );
}

/**
 * Acionamento de garantia pelo endpoint correto: `POST /collections/:id/guarantee`
 * com `{type, amount, reason}` e resposta do garantidor por
 * `PUT /collections/:id/guarantee/status`. O fluxo antigo mandava só um
 * `PUT /collections/:id` com `notes: "Garantia acionada"` e descartava
 * modalidade, valor e motivo digitados.
 *
 * A BAIXA POR GARANTIA (indenização recebida) também é um
 * `PUT /guarantee/status`, com `status: 'PAID'` e `settled_amount` (> 0): o
 * sistema dá baixa nas cobranças do caso (mais antiga primeiro), o excedente
 * vira crédito do caso e, se quitar tudo, o caso é RESOLVIDO. Para `CAUCAO` o
 * valor é debitado da caução do contrato e o acerto da rescisão passa a
 * refletir a dedução. `REJECTED`/`CANCELLED` devolvem o caso para ACTIVE.
 */
export function GuaranteePanel({
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
  const [type, setType] = useState<GuaranteeType>('SEGURO_FIANCA');
  const [amount, setAmount] = useState<number | undefined>(undefined);
  const [reason, setReason] = useState('');
  const [nextStatus, setNextStatus] = useState<GuaranteeActivationStatus>('ACCEPTED');
  const [insurerResponse, setInsurerResponse] = useState('');
  const [saving, setSaving] = useState(false);
  const [settledAmount, setSettledAmount] = useState<number | undefined>(undefined);
  const [settlementReference, setSettlementReference] = useState('');
  const [settlementNotes, setSettlementNotes] = useState('');
  const [settling, setSettling] = useState(false);

  const guarantee = collection?.guarantee;

  useEffect(() => {
    setType('SEGURO_FIANCA');
    setAmount(undefined);
    setReason('');
    setInsurerResponse('');
    setNextStatus('ACCEPTED');
    setSettledAmount(undefined);
    setSettlementReference('');
    setSettlementNotes('');
  }, [collection?.id, open]);

  const activationAllowed = collection ? canTransitionTo(collection.status, 'GUARANTEE_ACTIVATED') : false;

  const activate = async () => {
    if (!collection || !canWrite) return;
    if (!amount || amount <= 0) {
      toast.error('Informe o valor a acionar da garantia.');
      return;
    }
    setSaving(true);
    try {
      const updated = await collectionService.activateGuarantee(collection.id, {
        type,
        amount,
        reason: reason.trim() || undefined,
      });
      onSaved(updated);
      toast.success('Garantia acionada com sucesso!');
      onClose();
    } catch (err) {
      toast.error(serverMessage(err, 'Erro ao acionar a garantia.'));
    } finally {
      setSaving(false);
    }
  };

  const updateStatus = async () => {
    if (!collection || !canWrite) return;
    setSaving(true);
    try {
      const updated = await collectionService.updateGuaranteeStatus(collection.id, {
        status: nextStatus,
        insurer_response: insurerResponse.trim() || undefined,
      });
      onSaved(updated);
      toast.success('Resposta do garantidor registrada!');
      onClose();
    } catch (err) {
      toast.error(serverMessage(err, 'Erro ao registrar a resposta do garantidor.'));
    } finally {
      setSaving(false);
    }
  };

  const registerSettlement = async () => {
    if (!collection || !canWrite) return;
    if (!settledAmount || settledAmount <= 0) {
      toast.error('Informe o valor da indenização recebida (maior que zero).');
      return;
    }
    setSettling(true);
    try {
      const updated = await collectionService.updateGuaranteeStatus(collection.id, {
        status: 'PAID',
        settled_amount: settledAmount,
        settlement_reference: settlementReference.trim() || undefined,
        insurer_response: settlementNotes.trim() || undefined,
      });
      onSaved(updated);
      toast.success('Indenização registrada: as cobranças do caso foram baixadas.');
      onClose();
    } catch (err) {
      toast.error(serverMessage(err, 'Erro ao registrar a indenização recebida.'));
    } finally {
      setSettling(false);
    }
  };

  return (
    <ExpandableSidePanel
      open={open}
      onClose={onClose}
      ariaLabel="Acionamento de Garantia"
      defaultSize="lg"
      renderHeader={({ expandButton, closeButton }) => (
        <DelinquencyPanelHeader
          title="Acionamento de Garantia"
          icon={<Shield className="w-5 h-5" />}
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

        {guarantee ? (
          <>
            <section className="bg-muted border border-border rounded-xl p-5 space-y-3">
              <h3 className="text-sm font-bold text-foreground">Acionamento registrado</h3>
              <Field label="Modalidade" value={GUARANTEE_TYPE_LABEL[guarantee.type] ?? guarantee.type} />
              <Field label="Valor acionado" value={formatCurrencyBRL(guarantee.amount)} />
              <Field label="Status" value={GUARANTEE_STATUS_LABEL[guarantee.status] ?? guarantee.status} />
              <Field label="Protocolo" value={guarantee.insurer_protocol || '—'} />
              <Field label="Seguradora" value={guarantee.insurer_name || '—'} />
              <Field label="Resposta do garantidor" value={guarantee.insurer_response || '—'} />
              <Field label="Solicitado em" value={formatDate(guarantee.requested_at)} />
              {guarantee.resolved_at && <Field label="Resolvido em" value={formatDateTime(guarantee.resolved_at)} />}
              {guarantee.reason && <Field label="Motivo" value={guarantee.reason} />}
              {typeof guarantee.settled_amount === 'number' && (
                <Field label="Indenização recebida" value={formatCurrencyBRL(guarantee.settled_amount)} />
              )}
              {guarantee.settlement_reference && (
                <Field label="Referência do recebimento" value={guarantee.settlement_reference} />
              )}
              {(guarantee.security_deposit_deduction_id ?? collection?.security_deposit_deduction_id) && (
                <Field
                  label="Dedução na caução"
                  value={guarantee.security_deposit_deduction_id ?? collection?.security_deposit_deduction_id ?? '—'}
                />
              )}
            </section>

            <section className="space-y-4 pt-4 border-t border-border">
              <h3 className="text-sm font-bold text-foreground">Registrar resposta do garantidor</h3>
              <div>
                <label className={LABEL_CLASS} htmlFor="guarantee-status">
                  Novo status *
                </label>
                <div className="relative">
                  <select
                    id="guarantee-status"
                    value={nextStatus}
                    onChange={(event) => setNextStatus(event.target.value as GuaranteeActivationStatus)}
                    className={`${FIELD_CLASS} appearance-none`}
                  >
                    {STATUS_OPTIONS.map((option) => (
                      <option key={option} value={option}>
                        {GUARANTEE_STATUS_LABEL[option]}
                      </option>
                    ))}
                  </select>
                  <ChevronDown className="w-4 h-4 text-muted-foreground absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none" />
                </div>
              </div>
              {(nextStatus === 'REJECTED' || nextStatus === 'CANCELLED') && (
                <PanelNotice>
                  Com o acionamento <strong>{GUARANTEE_STATUS_LABEL[nextStatus]}</strong>, o caso volta para cobrança
                  ativa (ACTIVE) e a régua de cobrança retoma.
                </PanelNotice>
              )}
              <div>
                <label className={LABEL_CLASS} htmlFor="guarantee-response">
                  Resposta recebida
                </label>
                <textarea
                  id="guarantee-response"
                  rows={3}
                  value={insurerResponse}
                  onChange={(event) => setInsurerResponse(event.target.value)}
                  placeholder="Ex.: protocolo 12345, valor liberado em 10 dias..."
                  className={TEXTAREA_CLASS}
                />
              </div>
            </section>

            <section className="space-y-4 pt-4 border-t border-border">
              <h3 className="text-sm font-bold text-foreground">Registrar indenização recebida</h3>
              {guarantee.status === 'PAID' ? (
                <PanelNotice>
                  Este acionamento já está com status <strong>{GUARANTEE_STATUS_LABEL.PAID}</strong>
                  {typeof guarantee.settled_amount === 'number'
                    ? ` (${formatCurrencyBRL(guarantee.settled_amount)} recebidos)`
                    : ''}
                  : a baixa das cobranças já foi feita.
                </PanelNotice>
              ) : (
                <>
                  <PanelNotice>
                    Registrar a indenização dá baixa nas cobranças deste caso da <strong>mais antiga para a mais
                    recente</strong>; o que sobrar vira <strong>crédito do caso</strong> e, se quitar tudo, o caso é{' '}
                    <strong>RESOLVIDO</strong>.
                    {guarantee.type === 'CAUCAO' ? (
                      <>
                        {' '}
                        Como a garantia é <strong>Depósito Caução</strong>, o valor é debitado da caução do contrato e o
                        acerto da rescisão passa a refletir a dedução.
                      </>
                    ) : (
                      ' O valor vem do garantidor (seguradora/fiador), não da caução do contrato.'
                    )}
                  </PanelNotice>
                  <div>
                    <label className={LABEL_CLASS} htmlFor="guarantee-settled-amount">
                      Valor recebido *
                    </label>
                    <MoneyInput
                      id="guarantee-settled-amount"
                      value={settledAmount}
                      onChange={setSettledAmount}
                      placeholder="0,00"
                      className={FIELD_CLASS}
                    />
                  </div>
                  <div>
                    <label className={LABEL_CLASS} htmlFor="guarantee-settlement-reference">
                      Referência / protocolo
                    </label>
                    <input
                      id="guarantee-settlement-reference"
                      value={settlementReference}
                      onChange={(event) => setSettlementReference(event.target.value)}
                      placeholder="Ex.: aviso de sinistro 12345 / TED 987654"
                      className={FIELD_CLASS}
                    />
                  </div>
                  <div>
                    <label className={LABEL_CLASS} htmlFor="guarantee-settlement-notes">
                      Observação
                    </label>
                    <textarea
                      id="guarantee-settlement-notes"
                      rows={3}
                      value={settlementNotes}
                      onChange={(event) => setSettlementNotes(event.target.value)}
                      placeholder="Ex.: indenização creditada em 10/10, conferida com o extrato..."
                      className={TEXTAREA_CLASS}
                    />
                  </div>
                  <Button
                    onClick={registerSettlement}
                    loading={settling}
                    loadingText="Registrando..."
                    disabled={!canWrite}
                    className="w-full"
                  >
                    Registrar Indenização
                  </Button>
                </>
              )}
            </section>
          </>
        ) : (
          <>
            {!activationAllowed && (
              <PanelNotice tone="critical">
                Este caso está <strong>{collection?.status}</strong> e não aceita transição para GUARANTEE_ACTIVATED.
              </PanelNotice>
            )}
            <div>
              <label className={LABEL_CLASS} htmlFor="guarantee-type">
                Tipo de Garantia *
              </label>
              <div className="relative">
                <select
                  id="guarantee-type"
                  value={type}
                  onChange={(event) => setType(event.target.value as GuaranteeType)}
                  className={`${FIELD_CLASS} appearance-none`}
                >
                  {GUARANTEE_TYPES.map((option) => (
                    <option key={option} value={option}>
                      {GUARANTEE_TYPE_LABEL[option]}
                    </option>
                  ))}
                </select>
                <ChevronDown className="w-4 h-4 text-muted-foreground absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none" />
              </div>
            </div>

            <div>
              <label className={LABEL_CLASS} htmlFor="guarantee-amount">
                Valor a Acionar *
              </label>
              <MoneyInput
                id="guarantee-amount"
                value={amount}
                onChange={setAmount}
                placeholder="0,00"
                className={FIELD_CLASS}
              />
            </div>

            <div>
              <label className={LABEL_CLASS} htmlFor="guarantee-reason">
                Motivo do Acionamento
              </label>
              <textarea
                id="guarantee-reason"
                rows={4}
                value={reason}
                onChange={(event) => setReason(event.target.value)}
                placeholder="Descreva o motivo do acionamento da garantia..."
                className={TEXTAREA_CLASS}
              />
            </div>
          </>
        )}
      </div>

      <div className={FOOTER_CLASS}>
        <Button variant="outline" onClick={onClose} className="flex-1">
          Cancelar
        </Button>
        {guarantee ? (
          <Button
            onClick={updateStatus}
            loading={saving}
            loadingText="Registrando..."
            disabled={!canWrite}
            className="flex-2"
          >
            Registrar Resposta
          </Button>
        ) : (
          <Button
            onClick={activate}
            loading={saving}
            loadingText="Acionando..."
            disabled={!activationAllowed || !canWrite}
            className="flex-2"
          >
            Acionar Garantia
          </Button>
        )}
      </div>
    </ExpandableSidePanel>
  );
}
