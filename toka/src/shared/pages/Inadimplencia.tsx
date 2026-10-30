import { PageHeader } from "../ui/page-header";
import { useCallback, useState } from 'react';
import { AnimatePresence, motion } from 'motion/react';
import { toast } from 'sonner';
import { useFocusMode } from '../context/FocusModeContext';
import { useIsMobile } from '../ui/use-mobile';
import { Button } from '../ui/button';
import { usePermission } from '../permissions/usePermission';
import { serverMessage } from '../lib/serverMessage';
import { delinquencyService } from '../services/delinquency.service';
import type { Collection } from '../services/collection.service';
import {
  useCollectionsPage,
  useDelinquencyOverview,
  useOpenCaseChargeIds,
  useOverdueCharges,
} from '../hooks/useDelinquencyData';
import { CollectionsTable, type CasePanelKind } from '../components/delinquency/CollectionsTable';
import { DelinquencyKpis } from '../components/delinquency/DelinquencyKpis';
import { OverdueChargesWithoutCase } from '../components/delinquency/OverdueChargesWithoutCase';
import { ProcedurePanel } from '../components/delinquency/ProcedurePanel';
import { AgreementPanel } from '../components/delinquency/AgreementPanel';
import { GuaranteePanel } from '../components/delinquency/GuaranteePanel';
import { JudicialPanel } from '../components/delinquency/JudicialPanel';
import { TerminationPanel } from '../components/delinquency/TerminationPanel';
import { autoCreateCaseAfterDays, tabStatus, type DelinquencyTab } from '../components/delinquency/delinquencyUi';

const PAGE_SIZE = 20;

const EMPTY_MESSAGES: Record<DelinquencyTab, string> = {
  inadimplentes: 'Nenhum caso de inadimplência encontrado.',
  cobranca: 'Nenhum caso em cobrança ativa.',
  garantias: 'Nenhum caso com garantia acionada.',
  judicial: 'Nenhum caso em processo judicial.',
};

/**
 * Gestão de Inadimplência.
 *
 * Fontes de verdade: KPIs de `GET /delinquency/dashboard` (nunca somados a
 * partir da lista), casos de `GET /collections` com paginação real e o caso
 * completo (timeline/garantia/judicial) de `GET /collections/:id`. As mutações
 * usam os endpoints do domínio (`/actions`, `/agreement`, `/judicial`,
 * `/judicial/outcome`, `/guarantee`, `/guarantee/status`) e erros 4xx/5xx
 * aparecem com a mensagem do servidor.
 *
 * A aba Inadimplentes mostra também as cobranças vencidas que ainda NÃO têm
 * caso (`GET /charges/overdue`): o caso só nasce depois do limiar de dias de
 * atraso, então sem elas o processamento não produzia efeito visível.
 */
export function Inadimplencia() {
  const { focusMode } = useFocusMode();
  const isMobile = useIsMobile();
  const canReadCharges = usePermission('charges:read');
  const canWrite = usePermission('charges:write');
  const canTerminate = usePermission('contracts:write');

  const [tab, setTab] = useState<DelinquencyTab>('inadimplentes');
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState('');
  const [selected, setSelected] = useState<Collection | null>(null);
  const [panel, setPanel] = useState<CasePanelKind | null>(null);
  const [processing, setProcessing] = useState(false);

  const overview = useDelinquencyOverview();
  const list = useCollectionsPage({ status: tabStatus(tab), page, pageSize: PAGE_SIZE });
  const showOverdueCharges = tab === 'inadimplentes' && canReadCharges;
  const overdue = useOverdueCharges(showOverdueCharges);
  // A varredura dos casos só faz sentido quando há cobrança vencida a conferir.
  const openCases = useOpenCaseChargeIds(showOverdueCharges && overdue.charges.length > 0);
  // Limiar REAL da régua (config do servidor); sem o bloco `config` no payload,
  // o fallback documentado em `delinquencyUi`.
  const threshold = overview.dashboard.config;

  const changeTab = useCallback((next: DelinquencyTab) => {
    setTab(next);
    setPage(1);
    setSearch('');
  }, []);

  const openPanel = useCallback((kind: CasePanelKind, collection: Collection) => {
    setSelected(collection);
    setPanel(kind);
  }, []);

  const closePanel = useCallback(() => setPanel(null), []);

  /**
   * Caso aberto a partir de uma cobrança vencida: o bloco, a lista de casos e os
   * indicadores precisam refletir AGORA — a cobrança sai da faixa e o caso entra
   * na tabela e nos KPIs.
   */
  const handleCaseOpened = useCallback(() => {
    overdue.reload();
    openCases.reload();
    list.reload();
    overview.reload();
  }, [list, openCases, overdue, overview]);

  const handleSaved = useCallback(
    (updated: Collection) => {
      list.replace(updated);
      setSelected((current) => (current && current.id === updated.id ? updated : current));
      overview.reload();
    },
    [list, overview],
  );

  const handleProcess = useCallback(async () => {
    setProcessing(true);
    try {
      const result = await delinquencyService.process();
      // `charges_reverted` é do backend NOVO (marcação de atraso desfeita por
      // não estar vencida na regra de dia civil); o payload antigo não traz o
      // campo e o toast simplesmente não menciona.
      const reverted = result.charges_reverted ?? 0;
      toast.success(
        'Processamento concluído: ' +
          `${result.collections_created ?? 0} caso(s) criado(s), ` +
          `${result.collections_updated ?? 0} atualizado(s), ` +
          `${result.charges_marked_overdue ?? 0} cobrança(s) marcada(s) em atraso` +
          (reverted > 0 ? `, ${reverted} cobrança(s) revertida(s) para pendente.` : '.'),
      );
      overview.reload();
      list.reload();
      // O operador precisa ver o efeito AGORA: as cobranças vencidas (e a
      // conferência de casos abertos) são recarregadas junto com casos e KPIs.
      overdue.reload();
      openCases.reload();
    } catch (err) {
      toast.error(serverMessage(err, 'Erro ao processar a inadimplência manualmente.'));
    } finally {
      setProcessing(false);
    }
  }, [list, openCases, overdue, overview]);

  return (
    <div className="flex flex-col h-full bg-muted/50 font-sans relative overflow-hidden">
      <div className="flex-none px-6 lg:px-8 py-6 border-b border-border bg-card sticky top-0 z-10">
        <PageHeader
          title="Inadimplência"
          description="Cobranças, garantias e processos judiciais com dados do servidor"
        >
          {canWrite ? (
            <Button onClick={handleProcess} loading={processing} loadingText="Processando...">
              Processar Inadimplência
            </Button>
          ) : (
            <p className="text-sm text-muted-foreground">
              Seu perfil não tem <strong>charges:write</strong>: a tela está em modo somente leitura.
            </p>
          )}
        </PageHeader>
      </div>

      <div className="flex-1 overflow-y-auto p-6 lg:p-8 space-y-8">
        <AnimatePresence>
          {!focusMode && (
            <motion.div
              initial={{ opacity: 0, height: 0 }}
              animate={{ opacity: 1, height: 'auto' }}
              exit={{ opacity: 0, height: 0 }}
              transition={{ duration: 0.3, ease: 'easeInOut' }}
            >
              <DelinquencyKpis
                dashboard={overview.dashboard}
                loading={overview.loading}
                error={overview.error}
                onReload={overview.reload}
              />
            </motion.div>
          )}
        </AnimatePresence>

        <CollectionsTable
          tab={tab}
          onTabChange={changeTab}
          items={list.items}
          loading={list.loading}
          error={list.error}
          onReload={list.reload}
          total={list.total}
          page={page}
          totalPages={list.totalPages}
          isMobile={isMobile}
          search={search}
          onSearchChange={setSearch}
          onPageChange={setPage}
          canWrite={canWrite}
          canTerminate={canTerminate}
          onAction={openPanel}
          emptyMessage={EMPTY_MESSAGES[tab]}
        />

        {showOverdueCharges && (
          <OverdueChargesWithoutCase
            charges={overdue.charges}
            caseChargeIds={openCases.chargeIds}
            loading={overdue.loading}
            error={overdue.error}
            crossCheckError={openCases.error}
            thresholdDays={autoCreateCaseAfterDays(threshold)}
            thresholdSource={threshold?.source ?? null}
            canOpenCase={canWrite}
            onCaseOpened={handleCaseOpened}
            onReload={() => {
              overdue.reload();
              openCases.reload();
            }}
          />
        )}
      </div>

      <ProcedurePanel
        open={panel === 'procedimento'}
        collection={selected}
        canWrite={canWrite}
        onClose={closePanel}
        onSaved={handleSaved}
      />
      <AgreementPanel
        open={panel === 'acordo'}
        collection={selected}
        canWrite={canWrite}
        onClose={closePanel}
        onSaved={handleSaved}
      />
      <GuaranteePanel
        open={panel === 'garantia'}
        collection={selected}
        canWrite={canWrite}
        onClose={closePanel}
        onSaved={handleSaved}
      />
      <JudicialPanel
        open={panel === 'judicial'}
        collection={selected}
        canWrite={canWrite}
        onClose={closePanel}
        onSaved={handleSaved}
      />
      <TerminationPanel
        open={panel === 'rescisao'}
        collection={selected}
        canTerminate={canTerminate}
        canResolveCase={canWrite}
        onClose={closePanel}
        onSaved={handleSaved}
      />
    </div>
  );
}
