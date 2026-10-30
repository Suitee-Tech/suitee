import { useState } from 'react';
import { AlertTriangle, FolderPlus, Info, RefreshCw } from 'lucide-react';
import { clsx } from 'clsx';
import { toast } from 'sonner';
import type { Charge } from '../../services/charge.service';
import { collectionService } from '../../services/collection.service';
import type { DelinquencyConfig } from '../../services/delinquency.service';
import { serverMessage } from '../../lib/serverMessage';
import { ConfirmDialog } from '../ConfirmDialog';
import { Button } from '../../ui/button';
import { formatCurrencyBRL, formatISODateBR } from '../../utils/format';
import { CHARGE_DUE_TONE_CLASS, chargeDueLabel } from '../../utils/chargeDueLabel';
import { useIsMobile } from '../../ui/use-mobile';
import { autoCreateThresholdNote } from './delinquencyUi';
import {
  canOpenCaseFromCharge,
  chargeOutstandingAmount,
  openCasePayloadFromCharge,
} from './openCaseFromCharge';

/**
 * Faixa de COBRANÇAS VENCIDAS SEM CASO ABERTO.
 *
 * Por que existe: o caso de cobrança só nasce depois do limiar de dias de
 * atraso (o valor REAL vem da configuração do servidor — ver
 * `autoCreateThresholdNote`). Entre o vencimento e o limiar, `GET /collections`
 * não tem nada para a aba Inadimplentes mostrar — o operador processava a
 * inadimplência e continuava vendo a tela vazia, sem saber se o processamento
 * fez efeito. Aqui aparecem justamente essas cobranças
 * (`GET /charges/overdue`), com o rótulo da régua do backend: "Venceu ontem",
 * "Venceu há N dias" — e "Vence hoje" quando a cobrança ainda está no dia dela
 * (vence no dia, atrasa no dia seguinte).
 *
 * A régua de dias vem de `days_overdue` (backend); sem o campo, do `due_date`.
 *
 * Cada linha também pode ABRIR O CASO na hora (`POST /collections`), sem esperar
 * o limiar: a ação cria dívida formal, então pede confirmação explícita e mostra
 * a mensagem do servidor quando ele recusa (400 de valor divergente, 409 de
 * cobrança que já está em um caso aberto).
 */
export function OverdueChargesWithoutCase({
  charges,
  caseChargeIds,
  loading,
  error,
  crossCheckError,
  thresholdDays,
  thresholdSource,
  canOpenCase,
  onCaseOpened,
  onReload,
}: {
  /** Cobranças devolvidas por `GET /charges/overdue`. */
  charges: Charge[];
  /** IDs que já estão dentro de um caso aberto — não repetir na faixa. */
  caseChargeIds: Set<string>;
  loading: boolean;
  error: string | null;
  /** Falha ao conferir os casos abertos: a lista pode repetir cobranças. */
  crossCheckError: string | null;
  /** Limiar de dias vindo de `config.auto_create_collection_after_days`. */
  thresholdDays: number;
  /** Origem do limiar (`config.source`): imobiliária ou padrão da plataforma. */
  thresholdSource?: DelinquencyConfig['source'] | null;
  /** `charges:write` — sem ele a linha não oferece "Abrir caso". */
  canOpenCase: boolean;
  /** Caso criado: quem chamou recarrega bloco, lista de casos e indicadores. */
  onCaseOpened: () => void;
  onReload: () => void;
}) {
  const isMobile = useIsMobile();
  const [confirming, setConfirming] = useState<Charge | null>(null);
  const [openingId, setOpeningId] = useState<string | null>(null);

  const rows = charges
    .map((charge) => ({ charge, due: chargeDueLabel(charge) }))
    .filter(({ charge }) => !caseChargeIds.has(charge.id))
    .sort((a, b) => b.due.daysOverdue - a.due.daysOverdue || a.charge.due_date.localeCompare(b.charge.due_date));

  /** Nome resolvido pelo CRM quando vem; o id do inquilino como fallback. */
  const debtorLabel = (charge: Charge) => charge.debtor_name?.trim() || charge.tenant_party_id || '—';

  /**
   * Pagamento parcial aparece na linha: o valor exibido/enviado é o SALDO
   * (total − pago), então o operador precisa ver de onde saiu a diferença.
   */
  const paidHint = (charge: Charge) =>
    typeof charge.paid_amount === 'number' && charge.paid_amount > 0
      ? `Pago ${formatCurrencyBRL(charge.paid_amount)}`
      : null;

  const handleOpenCase = async () => {
    if (!confirming) return;
    const payload = openCasePayloadFromCharge(confirming);
    if (!payload) {
      setConfirming(null);
      toast.error('Esta cobrança não tem contrato ou devedor: não é possível abrir o caso por aqui.');
      return;
    }
    setOpeningId(confirming.id);
    try {
      await collectionService.create(payload);
      toast.success(`Caso de cobrança aberto para ${payload.debtor_name}.`);
      setConfirming(null);
      onCaseOpened();
    } catch (err) {
      // 400 (dívida informada não bate com as cobranças) e 409 (cobrança já está
      // em um caso aberto) chegam com a explicação do servidor: o operador vê a
      // causa real e NADA é recarregado como se tivesse dado certo.
      toast.error(serverMessage(err, 'Não foi possível abrir o caso de cobrança.'));
      setConfirming(null);
    } finally {
      setOpeningId(null);
    }
  };

  const confirmPayload = confirming ? openCasePayloadFromCharge(confirming) : null;

  return (
    <section
      aria-label="Cobranças vencidas sem caso aberto"
      className="rounded-2xl border border-border bg-card shadow-sm overflow-hidden"
    >
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-border bg-muted/50 p-4">
        <div className="min-w-0">
          <h2 className="text-sm font-bold text-foreground">
            {`Cobranças vencidas sem caso aberto (${rows.length})`}
          </h2>
          <p className="mt-1 flex items-start gap-1.5 text-xs text-muted-foreground">
            <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
            <span>{autoCreateThresholdNote(thresholdDays, thresholdSource)}</span>
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={onReload} loading={loading} loadingText="Atualizando...">
          <RefreshCw className="w-4 h-4" /> Atualizar cobranças
        </Button>
      </div>

      {crossCheckError && (
        <p className="flex items-start gap-2 border-b border-border bg-status-warning-soft/50 px-4 py-3 text-xs text-status-warning">
          <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
          {`${crossCheckError} A lista pode repetir cobranças que já estão em um caso.`}
        </p>
      )}

      {error && (
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border bg-status-critical-soft/40 px-4 py-3">
          <span className="text-sm text-status-critical">{error}</span>
          <Button variant="outline" size="sm" onClick={onReload}>
            Tentar novamente
          </Button>
        </div>
      )}

      {rows.length === 0 ? (
        <p className="p-6 text-center text-sm text-muted-foreground">
          {loading
            ? 'Carregando cobranças vencidas…'
            : error
              ? 'Não foi possível listar as cobranças vencidas.'
              : 'Nenhuma cobrança vencida fora de um caso de cobrança.'}
        </p>
      ) : isMobile ? (
        <ul className="divide-y divide-border">
          {rows.map(({ charge, due }) => (
            <li key={charge.id} className="p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="truncate text-sm font-bold text-foreground" title={charge.description}>
                    {charge.description || 'Cobrança'}
                  </p>
                  <p className="mt-0.5 truncate text-xs text-muted-foreground" title={debtorLabel(charge)}>
                    Inquilino {debtorLabel(charge)}
                  </p>
                  <p className="truncate text-xs text-muted-foreground" title={charge.contract_id}>
                    Contrato {charge.contract_id || '—'}
                  </p>
                </div>
                <div className="shrink-0 text-right">
                  <p className="text-sm font-bold tabular-nums text-foreground">
                    {formatCurrencyBRL(chargeOutstandingAmount(charge))}
                  </p>
                  {paidHint(charge) && <p className="text-xs text-muted-foreground">{paidHint(charge)}</p>}
                  <p className="text-xs text-muted-foreground">{formatISODateBR(charge.due_date) || '—'}</p>
                </div>
              </div>
              <div className="mt-2 flex flex-wrap items-center gap-3">
                <span
                  className={clsx(
                    'inline-flex rounded-full px-2.5 py-1 text-micro font-bold',
                    CHARGE_DUE_TONE_CLASS[due.tone],
                  )}
                >
                  {due.label}
                </span>
                {canOpenCase && canOpenCaseFromCharge(charge) && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setConfirming(charge)}
                    loading={openingId === charge.id}
                    loadingText="Abrindo..."
                  >
                    <FolderPlus className="w-4 h-4" /> Abrir caso
                  </Button>
                )}
              </div>
            </li>
          ))}
        </ul>
      ) : (
        <div className="overflow-x-auto">
          <table className={clsx('w-full border-collapse text-left', canOpenCase ? 'min-w-[1000px]' : 'min-w-[880px]')}>
            <thead>
              <tr className="border-b border-border text-xs font-medium tracking-wide text-muted-foreground">
                <th className="px-4 py-3">Cobrança</th>
                <th className="px-4 py-3">Inquilino</th>
                <th className="px-4 py-3">Contrato</th>
                <th className="px-4 py-3">Vencimento</th>
                <th className="px-4 py-3 text-right">Valor</th>
                <th className="px-4 py-3">Situação</th>
                {canOpenCase && <th className="px-4 py-3 text-right">Ação</th>}
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {rows.map(({ charge, due }) => (
                <tr key={charge.id} className="hover:bg-muted/60 transition-colors">
                  <td className="px-4 py-3 text-sm font-bold text-foreground">
                    <span className="block max-w-[260px] truncate" title={charge.description}>
                      {charge.description || 'Cobrança'}
                    </span>
                    {charge.reference_month && (
                      <span className="block text-xs font-normal text-muted-foreground">
                        Competência {charge.reference_month}
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-sm text-foreground">
                    <span
                      className="block max-w-[180px] truncate"
                      title={
                        charge.debtor_name?.trim()
                          ? `${charge.debtor_name.trim()} (${charge.tenant_party_id})`
                          : charge.tenant_party_id
                      }
                    >
                      {debtorLabel(charge)}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-sm text-foreground">
                    <span className="block max-w-[180px] truncate" title={charge.contract_id}>
                      {charge.contract_id || '—'}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-sm tabular-nums text-muted-foreground">
                    {formatISODateBR(charge.due_date) || '—'}
                  </td>
                  <td className="px-4 py-3 text-right text-sm font-bold tabular-nums text-foreground">
                    <span className="block">{formatCurrencyBRL(chargeOutstandingAmount(charge))}</span>
                    {paidHint(charge) && (
                      <span className="block text-xs font-normal text-muted-foreground">{paidHint(charge)}</span>
                    )}
                  </td>
                  <td className="px-4 py-3">
                    <span
                      className={clsx(
                        'inline-flex whitespace-nowrap rounded-full px-2.5 py-1 text-micro font-bold',
                        CHARGE_DUE_TONE_CLASS[due.tone],
                      )}
                    >
                      {due.label}
                    </span>
                  </td>
                  {canOpenCase && (
                    <td className="px-4 py-3 text-right">
                      {canOpenCaseFromCharge(charge) && (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setConfirming(charge)}
                          loading={openingId === charge.id}
                          loadingText="Abrindo..."
                        >
                          <FolderPlus className="w-4 h-4" /> Abrir caso
                        </Button>
                      )}
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <ConfirmDialog
        open={Boolean(confirming)}
        title="Abrir caso de cobrança?"
        variant="warning"
        confirmLabel="Abrir caso"
        onCancel={() => setConfirming(null)}
        onConfirm={handleOpenCase}
        description={
          confirming ? (
            <>
              Vai ser criado um caso de cobrança para <strong>{debtorLabel(confirming)}</strong> (contrato{' '}
              <strong>{confirming.contract_id}</strong>) com a dívida de{' '}
              <strong>{formatCurrencyBRL(confirmPayload?.total_debt ?? chargeOutstandingAmount(confirming))}</strong> e{' '}
              {confirmPayload?.days_overdue ?? 1} dia(s) de atraso. A cobrança passa a seguir a régua de cobrança do
              caso, com juros e multa — não se desfaz pela tela.
            </>
          ) : (
            ''
          )
        }
      />
    </section>
  );
}
