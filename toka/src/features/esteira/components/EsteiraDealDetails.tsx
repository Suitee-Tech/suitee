import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router";
import { getMediaUrl } from "../../../shared/media";
import { motion, AnimatePresence } from "motion/react";
import { Building, DollarSign, FileCheck, Loader2, PenTool, ShieldCheck, UploadCloud, UserPlus, Settings2, ExternalLink, Award, CalendarDays } from "lucide-react";
import { clsx } from "clsx";
import type { EsteiraDeal } from "../../../shared/deals/types";
import { useDealChecklistsContext } from "../../../shared/context/DealChecklistsContext";
import { useAuth } from "../../../shared/context/AuthContext";
import { ExpandableSidePanel } from "../../../shared/components/ExpandableSidePanel";
import { dealService, type DealDTO } from "../../../shared/services/deal.service";
import { shareDealRevision } from "../../../shared/deals/revisions";
import { flags } from "../../../shared/config/flags";
import { getActiveCompanyId } from "../../../shared/utils/tenant";
import { PartiesPanel } from "./PartiesPanel";
import { toast } from "sonner";
import { getImgproxyUrl } from "../../../shared/imgproxy";
import { ProposalTab } from "./esteira/ProposalTab";
import { DocumentationTab } from "./esteira/DocumentationTab";
import { AnalysisTab } from "./esteira/AnalysisTab";
import { ContractTab } from "./esteira/ContractTab";
import { FinanceiroTab } from "./esteira/FinanceiroTab";
import { ProposalSettingsDrawer } from "./esteira/ProposalSettingsDrawer";
import { CounterpartyLancamentosLink } from "../../../shared/components/lancamentos/CounterpartyLancamentosLink";
import { DealPanelSkeleton, PanelError } from "./esteira/PanelPrimitives";
import { dealQueryKey, fetchDeal } from "../../../shared/deals/queries";
import { contractsKeys, fetchContractsByDeal } from "../../../shared/contracts/queries";
import { notifyApiError } from "../../../shared/services/errorToast";

interface EsteiraDealDetailsProps {
  contract: EsteiraDeal;
  onClose: () => void;
}

type TabType = 'proposta' | 'partes' | 'documentacao' | 'analise' | 'contrato' | 'financeiro';

export function EsteiraDealDetails({ contract, onClose }: EsteiraDealDetailsProps) {
  const [activeTab, setActiveTab] = useState<TabType>('proposta');
  const [showSettings, setShowSettings] = useState(false);
  const [visitaOpen, setVisitaOpen] = useState(false);
  const [visita, setVisita] = useState({ date: "", time: "" });
  const [visitaLoading, setVisitaLoading] = useState(false);
  const { dealChecklists, updateChecklistFromDeal } = useDealChecklistsContext();
  const queryClient = useQueryClient();
  const { user } = useAuth();
  const tenantId = getActiveCompanyId(user);

  // Mock IDs (created locally via the new-deal wizard before a backend ID is
  // assigned) start with "NEG-" and shouldn't trigger a fetch — the deal does
  // not exist server-side yet.
  const isPersistedDeal = !!contract.id && !contract.id.startsWith('NEG-');

  const dealQuery = useQuery<DealDTO>({
    queryKey: dealQueryKey(contract.id, tenantId),
    queryFn: () => fetchDeal(contract.id),
    structuralSharing: shareDealRevision,
    enabled: Boolean(isPersistedDeal && tenantId),
  });
  // Mutations and refetches update the same query observed by every tab.
  const dealDetails = dealQuery.data ?? null;
  const loading = dealQuery.isFetching;
  const fetchError = dealQuery.isError ? (dealQuery.error as Error) : null;

  // Keep the checklist store (Documentos tab labels/badges) in sync with the
  // authoritative deal whenever it loads or is refetched after a mutation —
  // the store is local state that is otherwise updated only by checklist
  // mutations, leaving initial loads and cross-mutation changes stale.
  useEffect(() => {
    if (dealDetails) updateChecklistFromDeal(dealDetails.id, dealDetails);
  }, [dealDetails, updateChecklistFromDeal]);

  // Surface a one-shot toast on transitions from success → error so the user
  // notices stale data, without spamming on every retry attempt.
  const lastErrorRef = useRef<unknown>(null);
  useEffect(() => {
    if (!isPersistedDeal) return;
    if (dealQuery.isError && dealQuery.error !== lastErrorRef.current) {
      lastErrorRef.current = dealQuery.error;
      toast.error("Não foi possível carregar os detalhes completos.");
    }
    if (!dealQuery.isError) {
      lastErrorRef.current = null;
    }
  }, [dealQuery.error, dealQuery.isError, isPersistedDeal]);

  // Fetch the contract alongside the deal so the "Assinatura" tab badge can
  // reflect the source-of-truth signature state (contract.status === SIGNED)
  // even when the deal.signers projection lags behind Clicksign webhooks.
  const contractsQuery = useQuery({
    queryKey: contractsKeys.byDeal(contract.id),
    queryFn: () => fetchContractsByDeal(contract.id),
    enabled: isPersistedDeal,
  });
  const latestContract = useMemo(() => {
    const list = contractsQuery.data ?? [];
    if (list.length === 0) return null;
    // Pick the latest contract (last created) — matches what ContractTab uses.
    const sorted = [...list].sort((a, b) => {
      const aTs = a.created_at ? new Date(a.created_at).getTime() : 0;
      const bTs = b.created_at ? new Date(b.created_at).getTime() : 0;
      return bTs - aTs;
    });
    return sorted[0] ?? null;
  }, [contractsQuery.data]);

  const refreshDetails = async () => {
    if (!contract.id) return;
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: dealQueryKey(contract.id, tenantId) }),
      queryClient.invalidateQueries({ queryKey: contractsKeys.byDeal(contract.id) }),
    ]);
  };

  const fetchDetails = refreshDetails;

  const formatCurrency = (val: number) => new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL", maximumFractionDigits: 0 }).format(val);
  const isMultiParty = !!(flags.multiPartyUI && dealDetails?.parties && dealDetails.parties.length > 0);
  const dealId = dealDetails?.id ?? "";
  const handleScheduleVisita = async () => {
    if (!dealId || !visita.date || !visita.time) {
      toast.error("Informe a data e o horário da visita.");
      return;
    }
    setVisitaLoading(true);
    try {
      const [h, m] = visita.time.split(":");
      const end = `${String((Number(h) + 1) % 24).padStart(2, "0")}:${m}`;
      await dealService.createAppointment(dealId, { date: visita.date, time: visita.time, end_time: end, type: "Visita", title: "Visita" });
      toast.success("Visita agendada para o cliente. Ele verá no portal.");
      setVisitaOpen(false);
      setVisita({ date: "", time: "" });
    } catch (err: any) {
      notifyApiError(err, { context: "Agendar visita" });
    } finally {
      setVisitaLoading(false);
    }
  };
  const checklist = dealId ? (dealChecklists[dealId] ?? []) : [];
  const pendingDocs = checklist.filter((i) => (i.status === "pending" || i.status === "rejected") && !i.solicited).length;
  const allSigners = dealDetails?.signers ?? [];
  const requiredSigners = allSigners.filter((s) => s.is_required ?? true);
  // When the contract itself reports SIGNED we trust that over the deal.signers
  // projection (which can lag behind Clicksign webhooks). This keeps the tab
  // badge accurate the moment the contract finishes signing, regardless of
  // whether the per-signer projection has caught up.
  const contractFullySigned = (latestContract?.status ?? "").toUpperCase() === "SIGNED";
  const isSignerSignedEffective = (s: { status?: string }) =>
    contractFullySigned || (s.status ?? "").toUpperCase() === "SIGNED";
  const signedRequiredCount = contractFullySigned
    ? requiredSigners.length
    : requiredSigners.filter(isSignerSignedEffective).length;
  const signedCount = contractFullySigned
    ? allSigners.length
    : allSigners.filter(isSignerSignedEffective).length;
  const signersCount = allSigners.length;
  const requiredSignersCount = requiredSigners.length;
  const proposalsCount = (dealDetails?.proposals ?? []).length;
  const partiesCount = (dealDetails?.parties ?? []).filter((p) => (p.name || p.email) && (p.name ?? "").toLowerCase() !== "unknown").length;

  const customerRole = dealDetails?.type === "RENT" ? "tenant" : "buyer";
  const primaryCustomer = dealDetails?.parties?.find((p) => p.role === customerRole && p.is_primary);
  const primaryOwner = dealDetails?.parties?.find((p) => p.role === "owner" && p.is_primary);
  const hasPrimaryGap = isMultiParty && (!primaryCustomer || !primaryOwner);
  const hasPendingDocs = pendingDocs > 0;
  const hasPendingSigners = requiredSignersCount > 0 && signedRequiredCount < requiredSignersCount;

  return (
    <>
    <ExpandableSidePanel
      open={true}
      onClose={onClose}
      defaultSize="4xl"
      expandedSize="full"
      renderHeader={({ expandButton, closeButton }) => (
        <div className="flex-none px-6 py-5 border-b border-border bg-card">
          {/* No celular esta linha agrupa título + controles. Sem `flex-wrap`
              + `min-w-0` no bloco de informação, o título (nome + UUID + tipo)
              não encolhe e empurrava os controles para fora da tela — o botão
              de FECHAR ficava em x=455 numa viewport de 390px, sem forma de
              sair do painel. */}
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="flex min-w-0 flex-1 items-center gap-4">
              {contract.avatarUrl ? (
                <img
                  src={getImgproxyUrl(getMediaUrl(contract.avatarUrl), { width: 48, height: 48 })}
                  alt={contract.client}
                  className="w-12 h-12 rounded-full object-cover border-2 border-white shadow-sm shrink-0"
                />
              ) : (
                <div className="w-12 h-12 shrink-0 rounded-full bg-primary-100 text-primary-700 flex items-center justify-center font-bold border-2 border-white shadow-sm">
                  {contract.client?.charAt(0).toUpperCase() || 'C'}
                </div>
              )}
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  {/* Phase 4.2 — name links to lead detail when clientId is
                      available; falls back to plain text. ExternalLink icon
                      only on hover to keep the visual quiet. */}
                  {contract.clientId ? (
                    <Link
                      to={`/leads/${contract.clientId}`}
                      className="text-xl font-bold text-foreground hover:text-primary transition-colors group inline-flex items-center gap-1.5"
                      title="Ver perfil do cliente"
                    >
                      {contract.client}
                      <ExternalLink className="w-3.5 h-3.5 opacity-0 group-hover:opacity-60 transition-opacity" />
                    </Link>
                  ) : (
                    <h2 className="text-xl font-bold text-foreground">{contract.client}</h2>
                  )}
                  <span className="inline-block max-w-[8rem] truncate align-middle text-caption font-mono font-medium text-muted-foreground bg-muted px-2 py-0.5 rounded border border-border sm:max-w-none">
                    {contract.id}
                  </span>
                  <span className={clsx(
                    "text-micro font-bold px-2 py-0.5 rounded",
                    contract.type === 'Venda'
                      ? "bg-status-ok-soft text-status-ok"
                      : "bg-status-info-soft text-status-info"
                  )}>
                    {contract.type}
                  </span>
                </div>
                <p className="text-sm text-muted-foreground mt-0.5 flex items-center gap-2 flex-wrap">
                  <Building className="w-3.5 h-3.5" />
                  {contract.propertyId ? (
                    <Link
                      to={`/imoveis/${contract.propertyId}`}
                      className="hover:text-primary transition-colors underline-offset-2 hover:underline"
                      title="Ver detalhes do imóvel"
                    >
                      {contract.property}
                    </Link>
                  ) : (
                    <span>{contract.property}</span>
                  )}
                  <span>•</span>
                  <span className="font-bold text-foreground">{formatCurrency(contract.value)}</span>
                  {/* Captador pill — only renders when the deal has a captador
                      participation. Uses the dealDetails state populated by
                      the existing DealService.getById call. */}
                  {dealDetails?.broker_participations?.find(p => p.role === 'captador')?.broker_id && (
                    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-primary/10 text-primary text-caption font-semibold border border-primary/20">
                      <Award className="w-3 h-3" />
                      Captador
                    </span>
                  )}
                  {/* Atalho para a Inbox de Lançamentos filtrada pelo cliente */}
                  {/* deste deal. Usa o nome do cliente como search por não    */}
                  {/* ter party_id direto exposto no Contract aqui — o termo   */}
                  {/* bate contra counterparty.label de charges/commissions.    */}
                  {contract.client && (
                    <CounterpartyLancamentosLink
                      counterpartyKind="tenant"
                      searchTerm={contract.client}
                      label="Lançamentos"
                    />
                  )}
                </p>
              </div>
            </div>
            <div className="flex w-full shrink-0 flex-wrap items-center justify-end gap-2 sm:w-auto">
              {loading && <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" />}
              {dealDetails && (
                <button
                  type="button"
                  onClick={() => setShowSettings(true)}
                  className="p-2 rounded-lg border border-border bg-card hover:bg-muted transition-colors flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
                  title="Parâmetros da proposta digital"
                >
                  <Settings2 className="w-4 h-4" />
                  <span className="hidden sm:inline">Parâmetros</span>
                </button>
              )}
              {dealDetails && (
                  <button
                    type="button"
                    onClick={() => setVisitaOpen(true)}
                    className="p-2 rounded-lg border border-border bg-card hover:bg-muted transition-colors flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
                    title="Agendar uma visita para o cliente"
                  >
                    <CalendarDays className="w-4 h-4" />
                    <span className="hidden sm:inline">Agendar visita</span>
                  </button>
                )}
              {expandButton}
              {closeButton}
            </div>
          </div>

          {/* Navigation Tabs */}
          {/* Abas rolam horizontalmente no celular: com as 6 abas a última
              ("Financeiro"/Prévia) ficava cortada na borda da tela. */}
          <div className="mt-6 flex gap-1 overflow-x-auto rounded-xl bg-muted/50 p-1 scrollbar-none">
            {[
              { id: 'proposta', label: 'Propostas', icon: FileCheck, badge: proposalsCount, warn: false },
              ...(flags.multiPartyUI ? [{ id: 'partes', label: 'Partes', icon: UserPlus, badge: partiesCount, warn: hasPrimaryGap }] : []),
              { id: 'documentacao', label: 'Documentos', icon: UploadCloud, badge: checklist.length, warn: hasPendingDocs },
              { id: 'analise', label: 'Análise', icon: ShieldCheck, badge: null as any, warn: false },
              { id: 'contrato', label: 'Assinatura', icon: PenTool, badge: signersCount ? `${signedRequiredCount}/${requiredSignersCount || signersCount}` : null, warn: hasPendingSigners },
              // Financeiro — antes do CONVERTED mostra prévia
              // (FinancialPreviewCard projeta charges/commissions/transfers
              // sem persistir). Após CONVERTED, lista charges reais e
              // permite marcar pagas inline, fechando o ciclo downstream.
              { id: 'financeiro', label: 'Financeiro', icon: DollarSign, badge: dealDetails?.status === 'CONVERTED' ? null : 'Prévia', warn: false }
            ].map(tab => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id as TabType)}
                className={clsx(
                  "flex-1 flex items-center justify-center gap-2 py-2.5 text-sm font-medium rounded-lg transition-all",
                  activeTab === tab.id
                    ? "bg-card text-primary-600 shadow-sm border border-border"
                    : "text-muted-foreground hover:text-foreground hover:bg-muted"
                )}
              >
                <tab.icon className="w-4 h-4" />
                <span className="hidden sm:inline">{tab.label}</span>
                {tab.badge ? (
                  <span
                    className={clsx(
                      "ml-1 text-caption font-bold px-2 py-0.5 rounded-full border",
                      activeTab === tab.id
                        ? "bg-muted text-foreground border-border"
                        : "bg-card text-muted-foreground border-border"
                    )}
                  >
                    {tab.badge}
                  </span>
                ) : null}
                {tab.warn ? <span className="w-2 h-2 rounded-full bg-status-warning" /> : null}
              </button>
            ))}
          </div>
        </div>
      )}
    >
      {/* Content Area */}
      <div className="flex-1 overflow-y-auto bg-muted p-6">
        {loading && !dealDetails ? (
          <DealPanelSkeleton />
        ) : fetchError && !dealDetails ? (
          <PanelError
            title="Não foi possível carregar o negócio"
            description="Verifique sua conexão e tente novamente. Se o problema persistir, entre em contato com o suporte."
            onRetry={fetchDetails}
          />
        ) : (
          <AnimatePresence mode="wait">
            <motion.div
              key={activeTab}
              initial={{ opacity: 0, y: 10 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -10 }}
              transition={{ duration: 0.2 }}
              className="h-full"
            >
              {activeTab === 'proposta' && <ProposalTab contract={contract} dealDetails={dealDetails} onRefresh={refreshDetails} onGoToTab={setActiveTab} />}
              {activeTab === 'partes' && dealDetails && <PartiesPanel deal={dealDetails} onRefresh={refreshDetails} />}
              {activeTab === 'documentacao' && <DocumentationTab dealDetails={dealDetails} latestContract={latestContract} />}
              {activeTab === 'analise' && <AnalysisTab dealDetails={dealDetails} />}
              {activeTab === 'contrato' && <ContractTab dealDetails={dealDetails} onRefresh={refreshDetails} onGoToTab={setActiveTab} />}
              {activeTab === 'financeiro' && (
                <FinanceiroTab
                  dealDetails={dealDetails}
                  onRefresh={refreshDetails}
                  onGoToContract={() => setActiveTab('contrato')}
                />
              )}
            </motion.div>
          </AnimatePresence>
        )}
      </div>
    </ExpandableSidePanel>
    {dealDetails && (
      <ProposalSettingsDrawer
        open={showSettings}
        onClose={() => setShowSettings(false)}
        deal={dealDetails}
      />
    )}
    {/* Modal: agendar visita para o cliente */}
    {visitaOpen && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
            <div className="w-full max-w-sm rounded-2xl border border-border bg-card p-5 shadow-lg">
              <h3 className="text-base font-bold text-foreground flex items-center gap-2">
                <CalendarDays className="w-4 h-4 text-primary-600" /> Agendar visita
              </h3>
              <p className="text-sm text-muted-foreground mt-1">Escolha a data e o horário para o cliente visitar o imóvel.</p>
              <div className="mt-4 space-y-3">
                <label className="block">
                  <span className="text-xs font-bold text-foreground mb-1 block">Data</span>
                  <input
                    type="date"
                    value={visita.date}
                    onChange={(e) => setVisita((v) => ({ ...v, date: e.target.value }))}
                    className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-4 py-2.5 outline-none focus:border-primary-500"
                  />
                </label>
                <label className="block">
                  <span className="text-xs font-bold text-foreground mb-1 block">Horário</span>
                  <input
                    type="time"
                    value={visita.time}
                    onChange={(e) => setVisita((v) => ({ ...v, time: e.target.value }))}
                    className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-4 py-2.5 outline-none focus:border-primary-500"
                  />
                </label>
              </div>
              <div className="mt-4 flex justify-end gap-2">
                <button type="button" onClick={() => setVisitaOpen(false)} className="px-4 py-2 rounded-lg border border-border text-sm font-semibold text-muted-foreground hover:text-foreground">Cancelar</button>
                <button type="button" onClick={handleScheduleVisita} disabled={visitaLoading} className="px-4 py-2 rounded-lg bg-primary text-primary-foreground text-sm font-semibold hover:bg-primary-600 disabled:opacity-60">
                  {visitaLoading ? "Agendando…" : "Confirmar visita"}
                </button>
              </div>
            </div>
          </div>
        )}
    </>
  );
}
