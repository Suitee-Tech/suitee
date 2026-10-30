import { useCallback, useMemo, useState } from 'react';
import { toast } from 'sonner';
import { ChevronDown, FileText, Info } from 'lucide-react';
import { ExpandableSidePanel } from '../ExpandableSidePanel';
import { Button } from '../../ui/button';
import { collectionService, type Collection } from '../../services/collection.service';
import { contractService } from '../../services/contract.service';
import { chargeService, type Charge, type SecurityDepositBalance } from '../../services/charge.service';
import { unwrapApiData } from '../../services/api';
import { ContractStatus } from '../../contracts/types';
import { useFinancialRead } from '../../hooks/useFinancialRead';
import { serverMessage } from '../../lib/serverMessage';
import { DelinquencyPanelHeader } from './DelinquencyPanelHeader';
import { PanelNotice } from './PanelNotice';
import {
  BODY_CLASS,
  FIELD_CLASS,
  FOOTER_CLASS,
  LABEL_CLASS,
  TEXTAREA_CLASS,
  formatDate,
  formatMetric,
  mergeNotes,
  negotiationFields,
  todayISO,
} from './delinquencyUi';
import { formatCurrencyBRL } from '../../utils/format';

/** Cobranças que ainda exigem dinheiro: as quitadas/canceladas/estornadas saem da conta. */
const OPEN_CHARGE_STATUSES = new Set(['PENDING', 'OVERDUE', 'PARTIALLY_PAID']);
const CLOSED_CONTRACT_STATUSES = new Set<string>([ContractStatus.DRAFT, ContractStatus.PENDING_SIGNATURE]);

/** Início do dia seguinte à data de referência, para contar só o que é futuro. */
const nextDayStart = (date: string): string => {
  const parsed = new Date(`${date}T00:00:00Z`);
  parsed.setUTCDate(parsed.getUTCDate() + 1);
  return parsed.toISOString();
};

const TERMINATION_TYPES = ['Quebra de Contrato', 'Fim de Contrato', 'Acordo Mútuo'];

type TerminationData = {
  contract: Awaited<ReturnType<typeof contractService.getById>>;
  dueCharges: Charge[];
  dueChargesTotal: number;
  futureCharges: number | null;
  deposit: SecurityDepositBalance | null;
};

/**
 * Rescisão com dados REAIS do contrato: cobranças a liquidar/futuras vêm de
 * `GET /charges?contract_id&due_to|due_from`, o saldo devedor é a soma das
 * cobranças em aberto até a data e a caução vem de
 * `GET /contracts/:id/security-deposit-balance`. Nada disso é mais número fixo
 * no código — quando o servidor não informa, a tela mostra "—".
 */
export function TerminationPanel({
  open,
  collection,
  canTerminate,
  canResolveCase,
  onClose,
  onSaved,
}: {
  open: boolean;
  collection: Collection | null;
  canTerminate: boolean;
  /** `charges:write` — sem ela o contrato pode ser encerrado, mas o CASO não. */
  canResolveCase: boolean;
  onClose: () => void;
  onSaved: (collection: Collection) => void;
}) {
  const contractId = collection?.contract_id ?? null;
  const [terminationType, setTerminationType] = useState(TERMINATION_TYPES[0]);
  const [date, setDate] = useState(todayISO());
  const [reason, setReason] = useState('');
  const [notes, setNotes] = useState('');
  const [saving, setSaving] = useState(false);
  const [showAllCharges, setShowAllCharges] = useState(false);

  const load = useCallback(async (): Promise<TerminationData> => {
    if (!contractId) throw new Error('missing contract');
    const contract = await contractService.getById(contractId);
    const dueTo = `${date}T23:59:59Z`;

    const dueEnvelope = await chargeService.getAll({ contract_id: contractId, due_to: dueTo, limit: 100, offset: 0 });
    const dueCharges = unwrapApiData<Charge[]>(dueEnvelope) ?? [];
    const dueTotal = (dueEnvelope as { meta?: { total?: number } })?.meta?.total ?? dueCharges.length;

    // Só o TOTAL interessa para as futuras: uma página de 1 registro já traz o
    // `meta.total` exato do servidor (vencimento estritamente após a data).
    const futureEnvelope = await chargeService.getAll({
      contract_id: contractId,
      due_from: nextDayStart(date),
      limit: 1,
      offset: 0,
    });
    const futureTotal = (futureEnvelope as { meta?: { total?: number } })?.meta?.total;

    const held = typeof contract.security_deposit === 'number' && contract.security_deposit > 0 ? contract.security_deposit : null;
    const deposit = held === null ? null : await chargeService.getSecurityDepositBalance(contractId, held);

    return {
      contract,
      dueCharges,
      dueChargesTotal: dueTotal,
      futureCharges: typeof futureTotal === 'number' ? futureTotal : null,
      deposit: deposit ? unwrapApiData<SecurityDepositBalance>(deposit) ?? null : null,
    };
  }, [contractId, date]);

  const { data, error, loading, retry } = useFinancialRead(`termination:${contractId}:${date}`, load, open && Boolean(contractId));

  const contract = data?.contract ?? null;
  const truncated = Boolean(data && data.dueChargesTotal > data.dueCharges.length);
  const openDueCharges = useMemo(
    () => (data?.dueCharges ?? []).filter((charge) => OPEN_CHARGE_STATUSES.has(charge.status)),
    [data?.dueCharges],
  );
  // Com a página truncada a soma não representa o contrato inteiro — melhor "—"
  // do que um saldo devedor menor do que a realidade.
  const outstanding = data && !truncated ? openDueCharges.reduce((sum, charge) => sum + (charge.total_amount ?? 0), 0) : null;
  const depositReturn = data?.deposit ? data.deposit.return_due : null;

  const alreadyTerminated = Boolean(contract?.termination) || contract?.status === ContractStatus.TERMINATED;
  const beforeSignature = contract ? CLOSED_CONTRACT_STATUSES.has(String(contract.status)) : false;
  const blockedReason = !canTerminate
    ? 'Seu perfil não tem permissão para encerrar contratos (contracts:write).'
    : alreadyTerminated
      ? `Contrato já encerrado${contract?.termination?.terminated_at ? ` em ${formatDate(contract.termination.terminated_at)}` : ''}.`
      : beforeSignature
        ? 'O contrato ainda não foi assinado por todos; a rescisão só é permitida após a assinatura.'
        : null;

  const submit = async () => {
    if (!collection || !contractId || blockedReason) return;
    if (!reason.trim()) {
      toast.error('Descreva o motivo da rescisão.');
      return;
    }
    setSaving(true);
    try {
      await contractService.terminate(contractId, {
        reason: `${terminationType}: ${reason.trim()}`,
        ...(depositReturn !== null ? { security_deposit_return: depositReturn } : {}),
      });
    } catch (err) {
      toast.error(serverMessage(err, 'Erro ao encerrar o contrato.'));
      setSaving(false);
      return;
    }

    // O contrato já foi encerrado: se o encerramento do CASO falhar, o usuário
    // precisa saber que o serviço ficou pela metade — sem toast de sucesso.
    if (!canResolveCase) {
      toast.warning(
        'Contrato encerrado. Seu perfil não tem charges:write para resolver o caso de cobrança — peça a um usuário com essa permissão.',
      );
      onClose();
      setSaving(false);
      return;
    }
    try {
      // O texto digitado é ANEXADO às observações que o caso já tinha: o
      // encerramento não apaga o histórico escrito por outra pessoa.
      const mergedNotes = mergeNotes(collection.notes, notes);
      const updated = await collectionService.update(collection.id, {
        status: 'RESOLVED',
        ...(mergedNotes ? { notes: mergedNotes } : {}),
        ...negotiationFields(collection),
      });
      onSaved(updated);
      toast.success('Contrato encerrado e caso de cobrança resolvido.');
      onClose();
    } catch (err) {
      toast.error(
        `Contrato encerrado, mas o caso de cobrança não foi resolvido: ${serverMessage(err, 'falha ao atualizar o caso.')}`,
      );
    } finally {
      setSaving(false);
    }
  };

  const chargesToShow = showAllCharges ? openDueCharges : openDueCharges.slice(0, 5);

  return (
    <ExpandableSidePanel
      open={open}
      onClose={onClose}
      ariaLabel="Rescisão de Contrato"
      defaultSize="2xl"
      renderHeader={({ expandButton, closeButton }) => (
        <DelinquencyPanelHeader
          title="Rescisão de Contrato"
          icon={<FileText className="w-5 h-5" />}
          contractId={collection?.contract_id}
          debtorName={collection?.debtor_name}
          expandButton={expandButton}
          closeButton={closeButton}
          destructive
        />
      )}
    >
      <div className={BODY_CLASS}>
        {blockedReason && <PanelNotice tone="critical">{blockedReason}</PanelNotice>}
        {error && (
          <PanelNotice tone="critical">
            {error}
            <Button variant="outline" size="sm" onClick={retry} className="mt-2">
              Tentar novamente
            </Button>
          </PanelNotice>
        )}

        <section className="bg-primary-50 border border-primary-100 rounded-xl p-5 space-y-3">
          <div className="flex items-center gap-2 text-primary-800 font-bold mb-2">
            <Info className="w-4 h-4" /> Resumo do Encerramento
          </div>
          <div className="flex justify-between items-center text-sm text-primary-700">
            <span>Cobranças a liquidar até {formatDate(date)}:</span>
            <span className="font-bold">
              {loading ? '…' : truncated ? '—' : openDueCharges.length}
            </span>
          </div>
          <div className="flex justify-between items-center text-sm text-primary-700">
            <span>Saldo devedor (cobranças em aberto):</span>
            <span className="font-bold">{loading ? '…' : formatMetric(outstanding, true)}</span>
          </div>
          <div className="h-px bg-primary-200/60 w-full my-1" />
          <div className="flex justify-between items-center text-sm text-primary-700">
            <span>Cobranças futuras a cancelar:</span>
            <span className="font-bold">{loading ? '…' : formatMetric(data?.futureCharges)}</span>
          </div>
          <div className="flex justify-between items-center text-sm text-primary-700">
            <span>Devolução de caução (servidor):</span>
            <span className="font-bold">{loading ? '…' : formatMetric(depositReturn, true)}</span>
          </div>
          {data?.deposit && (
            <div className="flex justify-between items-center text-sm text-primary-700">
              <span>Deduções pendentes da caução:</span>
              <span className="font-bold">{formatCurrencyBRL(data.deposit.deductions_pending)}</span>
            </div>
          )}
          {truncated && (
            <p className="text-xs text-primary-700">
              O contrato tem {data?.dueChargesTotal} cobranças vencendo até {formatDate(date)} e a listagem foi
              limitada; o saldo devedor não é exibido para não mostrar um valor parcial como se fosse o total.
            </p>
          )}
          {!loading && !data?.deposit && (
            <p className="text-xs text-primary-700">Caução não informada no contrato — devolução exibida como “—”.</p>
          )}
        </section>

        <section className="space-y-3">
          <h3 className="text-sm font-bold text-foreground">Cobranças a Liquidar</h3>
          {loading && <p className="text-sm text-muted-foreground">Carregando cobranças do contrato…</p>}
          {!loading && openDueCharges.length === 0 && (
            <p className="text-sm text-muted-foreground">Nenhuma cobrança em aberto vencendo até a data escolhida.</p>
          )}
          {chargesToShow.length > 0 && (
            <div className="bg-card border border-border rounded-xl divide-y divide-border">
              {chargesToShow.map((charge) => (
                <div key={charge.id} className="p-4 flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <p className="text-sm font-bold text-foreground truncate">
                      {charge.description || charge.type}{' '}
                      <span className="font-normal text-muted-foreground">venc. {formatDate(charge.due_date)}</span>
                    </p>
                    <span className="inline-block mt-1 px-2 py-0.5 bg-status-warning-soft text-status-warning text-micro font-bold rounded-md border border-status-warning/30">
                      {charge.status}
                    </span>
                  </div>
                  <span className="text-sm font-bold text-foreground whitespace-nowrap">
                    {formatCurrencyBRL(charge.total_amount)}
                  </span>
                </div>
              ))}
            </div>
          )}
          {openDueCharges.length > 5 && (
            <Button variant="ghost" size="sm" onClick={() => setShowAllCharges((prev) => !prev)}>
              {showAllCharges ? 'Mostrar menos' : `Mostrar todas as ${openDueCharges.length} cobranças`}
            </Button>
          )}
        </section>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-4 border-t border-border">
          <div className="sm:col-span-2">
            <label className={LABEL_CLASS} htmlFor="termination-type">
              Tipo de Rescisão *
            </label>
            <div className="relative">
              <select
                id="termination-type"
                value={terminationType}
                onChange={(event) => setTerminationType(event.target.value)}
                className={`${FIELD_CLASS} appearance-none`}
              >
                {TERMINATION_TYPES.map((type) => (
                  <option key={type} value={type}>
                    {type}
                  </option>
                ))}
              </select>
              <ChevronDown className="w-4 h-4 text-muted-foreground absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none" />
            </div>
          </div>
          <div>
            <label className={LABEL_CLASS} htmlFor="termination-date">
              Data de Referência *
            </label>
            <input
              id="termination-date"
              type="date"
              value={date}
              onChange={(event) => setDate(event.target.value)}
              className={FIELD_CLASS}
            />
          </div>
          <div className="sm:col-span-2">
            <label className={LABEL_CLASS} htmlFor="termination-reason">
              Motivo da Rescisão *
            </label>
            <textarea
              id="termination-reason"
              rows={2}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
              placeholder="Descreva o motivo da rescisão do contrato..."
              className={TEXTAREA_CLASS}
            />
          </div>
          <div className="sm:col-span-2">
            <label className={LABEL_CLASS} htmlFor="termination-notes">
              Observações do caso de cobrança
            </label>
            <textarea
              id="termination-notes"
              rows={2}
              value={notes}
              onChange={(event) => setNotes(event.target.value)}
              placeholder="Informações adicionais registradas no caso..."
              className={TEXTAREA_CLASS}
            />
            {collection?.notes && (
              <p className="text-xs text-muted-foreground mt-2">
                O caso já tem observações; o texto novo é anexado (sem apagar o que existe).
              </p>
            )}
          </div>
        </div>
      </div>

      <div className={FOOTER_CLASS}>
        <Button variant="outline" onClick={onClose} className="flex-1">
          Cancelar
        </Button>
        <Button
          variant="destructive"
          onClick={submit}
          loading={saving}
          loadingText="Encerrando..."
          disabled={Boolean(blockedReason) || loading}
          className="flex-2"
        >
          Confirmar Encerramento
        </Button>
      </div>
    </ExpandableSidePanel>
  );
}
