import { useEffect, useState } from 'react';
import { toast } from 'sonner';
import { ChevronDown, Gavel, Plus, Trash2 } from 'lucide-react';
import { ExpandableSidePanel } from '../ExpandableSidePanel';
import { Button } from '../../ui/button';
import { MoneyInput } from '../../ui/money-input';
import {
  collectionService,
  type Collection,
  type JudicialCost,
  type JudicialOutcomeStatus,
  type JudicialStatus,
} from '../../services/collection.service';
import { serverMessage } from '../../lib/serverMessage';
import { DelinquencyPanelHeader } from './DelinquencyPanelHeader';
import { PanelNotice } from './PanelNotice';
import {
  BODY_CLASS,
  FIELD_CLASS,
  FOOTER_CLASS,
  JUDICIAL_OUTCOME_EFFECT,
  JUDICIAL_OUTCOME_LABEL,
  JUDICIAL_STATUS_LABEL,
  LABEL_CLASS,
  TEXTAREA_CLASS,
  canTransitionTo,
  dateInputToISO,
  formatDate,
  isValidCNJ,
  todayISO,
} from './delinquencyUi';
import { formatCurrencyBRL } from '../../utils/format';

const JUDICIAL_STATUSES: JudicialStatus[] = [
  'EM_TRAMITE',
  'ACORDO',
  'SENTENCA_PROCEDENTE',
  'SENTENCA_IMPROCEDENTE',
  'ARQUIVADO',
];

const JUDICIAL_OUTCOMES: JudicialOutcomeStatus[] = [
  'ACORDO',
  'SENTENCA_PROCEDENTE',
  'SENTENCA_IMPROCEDENTE',
  'ARQUIVADO',
];

interface CostDraft {
  description: string;
  amount: number | undefined;
}

/**
 * Processo judicial por `POST /collections/:id/judicial` (o fluxo antigo
 * mandava `status: 'LEGAL_ACTION'` num `PUT /collections/:id` — valor que não
 * existe no domínio e por isso voltava 409 apresentado como sucesso).
 * `case_number` (CNJ) e `court` são obrigatórios no contrato.
 *
 * O DESFECHO é registrado por `POST /collections/:id/judicial/outcome` e só
 * existe com o caso em JUDICIAL: cada status tem um efeito diferente sobre o
 * caso (acordo → NEGOTIATED; procedente → segue em JUDICIAL; improcedente ou
 * arquivado → volta para ACTIVE) e as custas viram despesa do contrato. O
 * painel diz isso ao operador antes de confirmar.
 */
export function JudicialPanel({
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
  const judicial = collection?.judicial;
  const [caseNumber, setCaseNumber] = useState('');
  const [court, setCourt] = useState('');
  const [lawyerName, setLawyerName] = useState('');
  const [lawyerOab, setLawyerOab] = useState('');
  const [lawyerEmail, setLawyerEmail] = useState('');
  const [lawyerPhone, setLawyerPhone] = useState('');
  const [claimValue, setClaimValue] = useState<number | undefined>(undefined);
  const [status, setStatus] = useState<JudicialStatus>('EM_TRAMITE');
  const [nextHearing, setNextHearing] = useState('');
  const [notes, setNotes] = useState('');
  const [saving, setSaving] = useState(false);
  const [touched, setTouched] = useState(false);

  const [outcomeStatus, setOutcomeStatus] = useState<JudicialOutcomeStatus>('ACORDO');
  const [decidedAt, setDecidedAt] = useState(todayISO());
  const [outcomeNotes, setOutcomeNotes] = useState('');
  const [costs, setCosts] = useState<CostDraft[]>([]);
  const [savingOutcome, setSavingOutcome] = useState(false);

  useEffect(() => {
    setCaseNumber(judicial?.case_number ?? '');
    setCourt(judicial?.court ?? '');
    setLawyerName(judicial?.lawyer?.name ?? '');
    setLawyerOab(judicial?.lawyer?.oab ?? '');
    setLawyerEmail(judicial?.lawyer?.email ?? '');
    setLawyerPhone(judicial?.lawyer?.phone ?? '');
    setClaimValue(judicial?.claim_value ?? collection?.total_debt ?? undefined);
    setStatus(judicial?.status ?? 'EM_TRAMITE');
    setNextHearing(judicial?.next_hearing ? judicial.next_hearing.slice(0, 10) : '');
    setNotes('');
    setTouched(false);
    setOutcomeStatus('ACORDO');
    setDecidedAt(todayISO());
    setOutcomeNotes('');
    setCosts([]);
  }, [collection?.id, open, judicial, collection?.total_debt]);

  const transitionAllowed = collection ? canTransitionTo(collection.status, 'JUDICIAL') : false;
  const cnjValid = isValidCNJ(caseNumber);
  const canSubmit = Boolean(collection) && transitionAllowed && cnjValid && court.trim().length > 0;
  const outcomeAllowed = Boolean(collection) && collection?.status === 'JUDICIAL';

  const submit = async () => {
    setTouched(true);
    if (!collection || !canWrite) return;
    if (!cnjValid || !court.trim()) return;
    setSaving(true);
    try {
      const updated = await collectionService.setJudicial(collection.id, {
        case_number: caseNumber.trim(),
        court: court.trim(),
        lawyer_name: lawyerName.trim() || undefined,
        lawyer_oab: lawyerOab.trim() || undefined,
        lawyer_email: lawyerEmail.trim() || undefined,
        lawyer_phone: lawyerPhone.trim() || undefined,
        claim_value: claimValue,
        status,
        next_hearing: dateInputToISO(nextHearing),
        notes: notes.trim() || undefined,
      });
      onSaved(updated);
      toast.success('Processo judicial registrado com sucesso!');
      onClose();
    } catch (err) {
      toast.error(serverMessage(err, 'Erro ao registrar o processo judicial.'));
    } finally {
      setSaving(false);
    }
  };

  const handleCostChange = (index: number, patch: Partial<CostDraft>) => {
    setCosts((prev) => prev.map((cost, position) => (position === index ? { ...cost, ...patch } : cost)));
  };

  const removeCost = (index: number) => {
    setCosts((prev) => prev.filter((_, position) => position !== index));
  };

  const submitOutcome = async () => {
    if (!collection || !canWrite) return;
    if (!decidedAt) {
      toast.error('Informe a data da decisão.');
      return;
    }
    const filled = costs.filter((cost) => cost.description.trim() || cost.amount !== undefined);
    const invalid = filled.find((cost) => !cost.description.trim() || !cost.amount || cost.amount <= 0);
    if (invalid) {
      toast.error('Cada custa precisa de descrição e valor maior que zero.');
      return;
    }
    const payloadCosts: JudicialCost[] = filled.map((cost) => ({
      description: cost.description.trim(),
      amount: cost.amount ?? 0,
    }));
    const decidedISO = dateInputToISO(decidedAt);
    if (!decidedISO) {
      toast.error('Informe uma data válida para a decisão.');
      return;
    }
    setSavingOutcome(true);
    try {
      const updated = await collectionService.registerJudicialOutcome(collection.id, {
        status: outcomeStatus,
        decided_at: decidedISO,
        notes: outcomeNotes.trim() || undefined,
        costs: payloadCosts.length > 0 ? payloadCosts : undefined,
      });
      onSaved(updated);
      toast.success('Desfecho judicial registrado!');
      onClose();
    } catch (err) {
      toast.error(serverMessage(err, 'Erro ao registrar o desfecho judicial.'));
    } finally {
      setSavingOutcome(false);
    }
  };

  return (
    <ExpandableSidePanel
      open={open}
      onClose={onClose}
      ariaLabel="Processo Judicial"
      defaultSize="lg"
      renderHeader={({ expandButton, closeButton }) => (
        <DelinquencyPanelHeader
          title="Processo Judicial"
          icon={<Gavel className="w-5 h-5" />}
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

        {judicial && (
          <section className="bg-muted border border-border rounded-xl p-5 space-y-2 text-sm">
            <h3 className="text-sm font-bold text-foreground">Processo registrado</h3>
            <p className="text-muted-foreground">
              Nº <strong className="text-foreground">{judicial.case_number}</strong> · {judicial.court}
            </p>
            <p className="text-muted-foreground">
              Status: <strong className="text-foreground">{JUDICIAL_STATUS_LABEL[judicial.status] ?? judicial.status}</strong>
              {' · '}Valor da causa: <strong className="text-foreground">{formatCurrencyBRL(judicial.claim_value)}</strong>
            </p>
            <p className="text-muted-foreground">
              Advogado: <strong className="text-foreground">{judicial.lawyer?.name || '—'}</strong>
              {judicial.lawyer?.oab ? ` (OAB ${judicial.lawyer.oab})` : ''}
            </p>
            <p className="text-muted-foreground">Próxima audiência: {formatDate(judicial.next_hearing)}</p>
          </section>
        )}

        {!transitionAllowed && (
          <PanelNotice tone="critical">
            Este caso está <strong>{collection?.status}</strong> e não aceita transição para JUDICIAL.
          </PanelNotice>
        )}

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div className="sm:col-span-2">
            <label className={LABEL_CLASS} htmlFor="judicial-case-number">
              Número do Processo (CNJ) *
            </label>
            <input
              id="judicial-case-number"
              value={caseNumber}
              onChange={(event) => setCaseNumber(event.target.value)}
              onBlur={() => setTouched(true)}
              placeholder="0000000-00.0000.0.00.0000"
              aria-invalid={touched && !cnjValid}
              className={FIELD_CLASS}
            />
            {touched && !cnjValid && (
              <p className="text-xs text-status-critical mt-1">
                Use o formato CNJ (0000000-00.0000.0.00.0000) ou os 20 dígitos do processo.
              </p>
            )}
          </div>
          <div className="sm:col-span-2">
            <label className={LABEL_CLASS} htmlFor="judicial-court">
              Vara/Comarca *
            </label>
            <input
              id="judicial-court"
              value={court}
              onChange={(event) => setCourt(event.target.value)}
              onBlur={() => setTouched(true)}
              placeholder="Ex.: 1ª Vara Cível de São Paulo"
              className={FIELD_CLASS}
            />
          </div>
          <div>
            <label className={LABEL_CLASS} htmlFor="judicial-lawyer-name">
              Nome do Advogado
            </label>
            <input
              id="judicial-lawyer-name"
              value={lawyerName}
              onChange={(event) => setLawyerName(event.target.value)}
              placeholder="Nome completo"
              className={FIELD_CLASS}
            />
          </div>
          <div>
            <label className={LABEL_CLASS} htmlFor="judicial-lawyer-oab">
              OAB
            </label>
            <input
              id="judicial-lawyer-oab"
              value={lawyerOab}
              onChange={(event) => setLawyerOab(event.target.value)}
              placeholder="Ex.: 123456/SP"
              className={FIELD_CLASS}
            />
          </div>
          <div>
            <label className={LABEL_CLASS} htmlFor="judicial-lawyer-email">
              E-mail do Advogado
            </label>
            <input
              id="judicial-lawyer-email"
              type="email"
              value={lawyerEmail}
              onChange={(event) => setLawyerEmail(event.target.value)}
              placeholder="advogado@escritorio.com.br"
              className={FIELD_CLASS}
            />
          </div>
          <div>
            <label className={LABEL_CLASS} htmlFor="judicial-lawyer-phone">
              Telefone do Advogado
            </label>
            <input
              id="judicial-lawyer-phone"
              value={lawyerPhone}
              onChange={(event) => setLawyerPhone(event.target.value)}
              placeholder="(11) 99999-9999"
              className={FIELD_CLASS}
            />
          </div>
          <div>
            <label className={LABEL_CLASS} htmlFor="judicial-claim-value">
              Valor da Causa
            </label>
            <MoneyInput
              id="judicial-claim-value"
              value={claimValue}
              onChange={setClaimValue}
              placeholder="0,00"
              className={FIELD_CLASS}
            />
          </div>
          <div>
            <label className={LABEL_CLASS} htmlFor="judicial-status">
              Andamento
            </label>
            <div className="relative">
              <select
                id="judicial-status"
                value={status}
                onChange={(event) => setStatus(event.target.value as JudicialStatus)}
                className={`${FIELD_CLASS} appearance-none`}
              >
                {JUDICIAL_STATUSES.map((option) => (
                  <option key={option} value={option}>
                    {JUDICIAL_STATUS_LABEL[option]}
                  </option>
                ))}
              </select>
              <ChevronDown className="w-4 h-4 text-muted-foreground absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none" />
            </div>
          </div>
          <div>
            <label className={LABEL_CLASS} htmlFor="judicial-next-hearing">
              Data da Audiência
            </label>
            <input
              id="judicial-next-hearing"
              type="date"
              value={nextHearing}
              onChange={(event) => setNextHearing(event.target.value)}
              className={FIELD_CLASS}
            />
          </div>
          <div className="sm:col-span-2">
            <label className={LABEL_CLASS} htmlFor="judicial-notes">
              Observações
            </label>
            <textarea
              id="judicial-notes"
              rows={3}
              value={notes}
              onChange={(event) => setNotes(event.target.value)}
              placeholder="Informações adicionais sobre o processo..."
              className={TEXTAREA_CLASS}
            />
          </div>
        </div>

        <section className="space-y-4 pt-4 border-t border-border">
          <h3 className="text-sm font-bold text-foreground">Desfecho judicial</h3>
          {!outcomeAllowed ? (
            <PanelNotice>
              O desfecho só é registrado com o caso em <strong>JUDICIAL</strong>. Este caso está{' '}
              <strong>{collection?.status}</strong>.
            </PanelNotice>
          ) : (
            <>
              <div>
                <label className={LABEL_CLASS} htmlFor="judicial-outcome-status">
                  Resultado da decisão *
                </label>
                <div className="relative">
                  <select
                    id="judicial-outcome-status"
                    value={outcomeStatus}
                    onChange={(event) => setOutcomeStatus(event.target.value as JudicialOutcomeStatus)}
                    className={`${FIELD_CLASS} appearance-none`}
                  >
                    {JUDICIAL_OUTCOMES.map((option) => (
                      <option key={option} value={option}>
                        {JUDICIAL_OUTCOME_LABEL[option]}
                      </option>
                    ))}
                  </select>
                  <ChevronDown className="w-4 h-4 text-muted-foreground absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none" />
                </div>
              </div>

              <PanelNotice>{JUDICIAL_OUTCOME_EFFECT[outcomeStatus]}</PanelNotice>

              <div>
                <label className={LABEL_CLASS} htmlFor="judicial-decided-at">
                  Data da Decisão *
                </label>
                <input
                  id="judicial-decided-at"
                  type="date"
                  value={decidedAt}
                  onChange={(event) => setDecidedAt(event.target.value)}
                  className={FIELD_CLASS}
                />
              </div>

              <div className="space-y-3">
                <div className="flex items-center justify-between gap-3">
                  <span className="text-sm font-bold text-foreground">Custas e honorários</span>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setCosts((prev) => [...prev, { description: '', amount: undefined }])}
                  >
                    <Plus className="w-4 h-4" /> Adicionar custa
                  </Button>
                </div>
                {costs.length === 0 && (
                  <p className="text-xs text-muted-foreground">Nenhuma custa informada para este desfecho.</p>
                )}
                {costs.map((cost, index) => (
                  <div key={index} className="grid grid-cols-1 sm:grid-cols-[1fr_130px_40px] gap-2 items-end">
                    <div>
                      <label className={LABEL_CLASS} htmlFor={`judicial-cost-description-${index}`}>
                        Descrição *
                      </label>
                      <input
                        id={`judicial-cost-description-${index}`}
                        value={cost.description}
                        onChange={(event) => handleCostChange(index, { description: event.target.value })}
                        placeholder="Ex.: custas de distribuição"
                        className={FIELD_CLASS}
                      />
                    </div>
                    <div>
                      <label className={LABEL_CLASS} htmlFor={`judicial-cost-amount-${index}`}>
                        Valor *
                      </label>
                      <MoneyInput
                        id={`judicial-cost-amount-${index}`}
                        value={cost.amount}
                        onChange={(amount) => handleCostChange(index, { amount })}
                        placeholder="0,00"
                        className={FIELD_CLASS}
                      />
                    </div>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => removeCost(index)}
                      title="Remover custa"
                      className="size-9 text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft"
                    >
                      <Trash2 className="w-4 h-4" />
                    </Button>
                  </div>
                ))}
                <p className="text-xs text-muted-foreground">As custas serão lançadas como despesa do contrato.</p>
              </div>

              <div>
                <label className={LABEL_CLASS} htmlFor="judicial-outcome-notes">
                  Observações do desfecho
                </label>
                <textarea
                  id="judicial-outcome-notes"
                  rows={3}
                  value={outcomeNotes}
                  onChange={(event) => setOutcomeNotes(event.target.value)}
                  placeholder="Ex.: sentença publicada em 09/10, prazo de recurso em aberto..."
                  className={TEXTAREA_CLASS}
                />
              </div>

              <Button
                onClick={submitOutcome}
                loading={savingOutcome}
                loadingText="Registrando..."
                disabled={!canWrite}
                className="w-full"
              >
                Registrar Desfecho
              </Button>
            </>
          )}
        </section>
      </div>

      <div className={FOOTER_CLASS}>
        <Button variant="outline" onClick={onClose} className="flex-1">
          Cancelar
        </Button>
        <Button
          onClick={submit}
          loading={saving}
          loadingText="Registrando..."
          disabled={!canSubmit || !canWrite}
          className="flex-2"
        >
          {judicial ? 'Atualizar Processo' : 'Registrar Processo'}
        </Button>
      </div>
    </ExpandableSidePanel>
  );
}
