import { useEffect, useMemo, useRef, useState } from "react";
import { ArrowLeftRight, CheckCircle2, Clock, FileCheck, History, Plus, Users } from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { useWsSubscription } from "../../../../shared/ws/useWsSubscription";
import type { EsteiraDeal } from "../../../../shared/deals/types";
import type { DealDTO, Proposal } from "../../../../shared/services/deal.service";
import { useDealsContext } from "../../../../shared/context/DealsContext";
import { useRequestScopeGuard } from "../../../../shared/hooks/useRequestScopeGuard";
import { notifyApiError } from "../../../../shared/services/errorToast";
import { formatCurrencyBRL0, formatISODateBR } from "../../../../shared/utils/format";
import { MoneyInput } from "../../../../shared/ui/money-input";
import { Button } from "../../../../shared/ui/button";
import { StatusDot, StatusPill, type StatusTone } from "./PanelPrimitives";
import { usePermissionsContext } from "../../../../lib/permissions/PermissionsContext";
import { CommissionSplitCard } from "./CommissionSplitCard";

type TabType = "proposta" | "partes" | "documentacao" | "analise" | "contrato";

type ProposalTabProps = {
  contract: EsteiraDeal;
  dealDetails: DealDTO | null;
  onRefresh: () => void;
  onGoToTab: (t: TabType) => void;
};

const ROLE_LABEL: Record<string, string> = {
  tenant: "Inquilino",
  buyer: "Comprador",
  owner: "Proprietário",
  guarantor: "Fiador/Avalista",
  witness: "Testemunha",
  spouse: "Cônjuge",
  attorney: "Procurador",
  legal_representative: "Representante legal",
  guardian: "Tutor",
  curator: "Curador",
};

export function ProposalTab({ contract, dealDetails, onRefresh, onGoToTab }: ProposalTabProps) {
  const [isAdding, setIsAdding] = useState(false);
  const [showHistory, setShowHistory] = useState(false);
  const [newValue, setNewValue] = useState(contract.value);
  const [paymentMethod, setPaymentMethod] = useState("À Vista");
  const [notes, setNotes] = useState("");
  const [savingProposal, setSavingProposal] = useState(false);
  const [optimisticProposal, setOptimisticProposal] = useState<Proposal | null>(null);
  const proposalRequestPending = useRef(false);
  // Issue #421 — gate da ação de "Aceitar Contraproposta" (backend exige
  // deals:approve). Mantemos o botão visível para o operador ver que existe
  // a opção, mas desabilitado quando não tem permissão para evitar erro
  // 403 confuso após o clique.
  const [acceptingCounter, setAcceptingCounter] = useState(false);
  const counterRequestPending = useRef(false);
  const { addProposal, acceptCounterOffer } = useDealsContext();
  const beginScopedRequest = useRequestScopeGuard(contract.id);
  const { can } = usePermissionsContext();
  const canAcceptCounter = can("deals:approve");

  useEffect(() => {
    proposalRequestPending.current = false;
    counterRequestPending.current = false;
    setSavingProposal(false);
    setAcceptingCounter(false);
    setIsAdding(false);
    setShowHistory(false);
    setPaymentMethod("À Vista");
    setNotes("");
    setOptimisticProposal(null);
  }, [contract.id]);

  useEffect(() => {
    if (!isAdding && !savingProposal) {
      setNewValue(contract.value);
    }
  }, [contract.value, isAdding, savingProposal]);

  const proposalStatusConfig: Record<string, { label: string; tone: StatusTone }> = {
    DRAFT: { label: "Rascunho", tone: "neutral" },
    SENT: { label: "Enviada", tone: "info" },
    ACCEPTED: { label: "Aceita", tone: "ok" },
    REJECTED: { label: "Recusada", tone: "critical" },
    COUNTER_OFFER: { label: "Contraproposta", tone: "warning" },
  };

  useWsSubscription("proposal_update", (event) => {
    if (event?.payload?.deal_id === contract.id) {
      onRefresh();
    }
  });

  const generateTempId = () =>
    `optimistic-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;

  const handleAddProposal = async () => {
    if (proposalRequestPending.current) return;
    const isCurrent = beginScopedRequest();
    proposalRequestPending.current = true;
    setSavingProposal(true);

    // Optimistic: insert temporary proposal immediately into UI
    const tempId = generateTempId();
    const tempProposal: Proposal = {
      id: tempId,
      value: Number(newValue),
      payment_method: paymentMethod,
      notes,
      installments: 1,
      down_payment: 0,
      status: "SENT",
      created_at: new Date().toISOString(),
      created_by: "",
    };
    setOptimisticProposal(tempProposal);
    setNotes("");
    setPaymentMethod("À Vista");

    try {
      await addProposal(contract.id, {
        value: Number(newValue),
        payment_method: paymentMethod,
        notes,
        installments: 1,
        down_payment: 0,
      });
      if (!isCurrent()) return;
      toast.success("Proposta adicionada com sucesso!");
      // Clear optimistic — cache update from upsertDeal will provide real data
      setOptimisticProposal(null);
      // Close the form only on success; keep it open (with the saving state
      // and any error) while the request is in-flight or has failed.
      setIsAdding(false);
    } catch (error) {
      if (!isCurrent()) return;
      // Rollback: remove optimistic entry
      setOptimisticProposal(null);
      notifyApiError(error, { context: "Adicionar proposta" });
    } finally {
      if (isCurrent()) {
        proposalRequestPending.current = false;
        setSavingProposal(false);
      }
    }
  };

  // Issue #421 — aceitar contraproposta dispara o endpoint dedicado em
  // toca-deal (#424). Espera proposalId explícito porque um deal pode ter
  // mais de uma proposta histórica; cobramos do caller passar a certa
  // (aqui usamos `latestProposal.id` já calculada para o painel).
  const handleAcceptCounterOffer = async (proposalId: string) => {
    if (counterRequestPending.current) return;
    const isCurrent = beginScopedRequest();
    counterRequestPending.current = true;
    setAcceptingCounter(true);
    try {
      await acceptCounterOffer(contract.id, proposalId);
      if (!isCurrent()) return;
      toast.success("Contraproposta aceita. Negociação atualizada.");
    } catch (err: any) {
      if (!isCurrent()) return;
      const status = err?.response?.status;
      const msg = err?.response?.data?.message || err?.message || "Erro ao aceitar contraproposta.";
      if (status === 409) {
        // Proposta já não está em COUNTER_OFFER — provavelmente outro
        // operador aceitou enquanto este estava lendo. Re-fetch o painel
        // pra sair do estado obsoleto.
        toast.error("Proposta já não está em contraproposta. Atualizando…");
        onRefresh();
      } else if (status === 403) {
        toast.error("Você não tem permissão para aceitar contrapropostas.");
      } else {
        toast.error(msg);
      }
    } finally {
      if (isCurrent()) {
        counterRequestPending.current = false;
        setAcceptingCounter(false);
      }
    }
  };

  const proposals = dealDetails?.proposals || [];
  const allProposals = useMemo(() => {
    if (!optimisticProposal) return proposals;
    if (proposals.some((p) => p.id === optimisticProposal.id)) return proposals;
    // Append at end — matches existing oldest-first ordering
    return [...proposals, optimisticProposal];
  }, [proposals, optimisticProposal]);

  const latestProposal = allProposals.length > 0 ? allProposals[allProposals.length - 1] : null;
  const parties = dealDetails?.parties ?? [];
  const customerRole = dealDetails?.type === "RENT" ? "tenant" : "buyer";
  const primaryCustomer = parties.find((p) => p.role === customerRole && p.is_primary);
  const primaryOwner = parties.find((p) => p.role === "owner" && p.is_primary);
  const acceptedBy = latestProposal?.accepted_by ?? [];
  const isAcceptedBy = (partyId?: string) => !!partyId && acceptedBy.some((a) => a.party_id === partyId);
  const requiredParties = [primaryCustomer, primaryOwner].filter(Boolean) as any[];
  const allRequiredAccepted = requiredParties.length > 0 && requiredParties.every((p) => isAcceptedBy(p.id));
  const missingRequiredPrimaries = !!dealDetails?.parties && (!primaryCustomer || !primaryOwner);

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <section className="col-span-2 bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
          <header className="px-5 py-3 border-b border-border flex items-center justify-between gap-3">
            <div className="flex items-center gap-2.5 min-w-0">
              <FileCheck className="w-4 h-4 text-primary" aria-hidden="true" />
              <h3 className="text-label font-bold text-foreground uppercase tracking-wide truncate">
                Proposta Atual
              </h3>
            </div>
            {latestProposal && (
              <span className="inline-flex items-center gap-1 text-caption text-muted-foreground tabular-nums">
                <Clock className="w-3 h-3" aria-hidden="true" />
                {formatISODateBR(latestProposal.created_at)}
              </span>
            )}
          </header>

          <div className="p-5">
            {latestProposal ? (
              <dl className="grid grid-cols-1 sm:grid-cols-2 gap-px rounded-xl border border-border bg-border overflow-hidden">
                <div className="px-4 py-3 bg-card flex flex-col gap-1">
                  <dt className="text-micro font-bold text-muted-foreground uppercase tracking-wide">Valor Ofertado</dt>
                  <dd className="text-lg font-bold text-foreground tabular-nums">{formatCurrencyBRL0(latestProposal.value)}</dd>
                  <dd className="mt-0.5">
                    <StatusPill tone={proposalStatusConfig[latestProposal.status]?.tone ?? "neutral"}>
                      {proposalStatusConfig[latestProposal.status]?.label ?? latestProposal.status}
                    </StatusPill>
                  </dd>
                </div>
                <div className="px-4 py-3 bg-card flex flex-col gap-1">
                  <dt className="text-micro font-bold text-muted-foreground uppercase tracking-wide">Forma de Pagamento</dt>
                  <dd className="text-sm font-semibold text-foreground">{latestProposal.payment_method}</dd>
                  {latestProposal.notes && (
                    <dd className="text-caption text-muted-foreground italic mt-0.5">"{latestProposal.notes}"</dd>
                  )}
                </div>
                {latestProposal.status === "COUNTER_OFFER" && latestProposal.counter_value && (
                  <div className="col-span-2 px-4 py-3 bg-status-warning-soft border-t border-status-warning/20 motion-safe:animate-pulse motion-safe:[animation-duration:2.5s]">
                    <div className="flex items-start justify-between gap-3 flex-wrap">
                      <div className="min-w-0">
                        <dt className="text-micro font-bold text-status-warning uppercase tracking-wide flex items-center gap-1.5">
                          <ArrowLeftRight className="w-3.5 h-3.5" aria-hidden="true" /> Contraproposta do cliente
                        </dt>
                        <dd className="text-lg font-bold text-status-warning tabular-nums">{formatCurrencyBRL0(latestProposal.counter_value)}</dd>
                        <dd className="text-caption text-status-warning/85 mt-0.5">
                          Diferença sobre valor inicial:{" "}
                          <strong className="tabular-nums">
                            {formatCurrencyBRL0(latestProposal.counter_value - latestProposal.value)}
                          </strong>
                        </dd>
                      </div>
                      {/* Issue #421 — botão proeminente (verde) para aceitar
                        a contraproposta. Gated por deals:approve; quando o
                        usuário não tem permissão o botão ainda aparece
                        (mas desabilitado) pra ele saber que existe a
                        ação — evita "sumiço silencioso" da UI. */}
                      <Button
                        size="sm"
                        onClick={() => handleAcceptCounterOffer(latestProposal.id)}
                        disabled={!canAcceptCounter}
                        loading={acceptingCounter}
                        loadingText="Aceitando…"
                        title={canAcceptCounter ? "Aceitar contraproposta do cliente" : "Sem permissão para aceitar contrapropostas"}
                        className="bg-status-ok text-white hover:bg-status-ok/90 disabled:opacity-60 disabled:cursor-not-allowed shadow-sm shrink-0 gap-1.5"
                      >
                        <CheckCircle2 className="w-4 h-4" />
                        Aceitar Contraproposta
                      </Button>
                    </div>
                  </div>
                )}
              </dl>
            ) : (
              <div className="text-center py-6 text-muted-foreground text-sm">Nenhuma proposta registrada.</div>
            )}

            {missingRequiredPrimaries && (
              <div className="mt-4 p-3 bg-status-warning-soft rounded-xl border border-status-warning/30 flex items-start justify-between gap-3">
                <div className="min-w-0 flex items-start gap-2.5">
                  <StatusDot tone="warning" className="mt-1.5" />
                  <div>
                    <p className="text-sm font-bold text-status-warning">Partes primárias pendentes</p>
                    <p className="text-caption text-status-warning/85 mt-0.5">
                      Defina {ROLE_LABEL[customerRole] ?? customerRole} primário e Proprietário primário para avançar.
                    </p>
                  </div>
                </div>
                <Button
                  size="sm"
                  variant="outline"
                  className="h-8 border-status-warning/30 text-status-warning hover:bg-status-warning-soft shrink-0"
                  onClick={() => onGoToTab("partes")}
                >
                  Ir para Partes
                </Button>
              </div>
            )}

            {latestProposal && requiredParties.length > 0 && (
              <div className="mt-4 rounded-xl border border-border bg-muted/40 overflow-hidden">
                <div className="px-3 py-2 border-b border-border flex items-center justify-between gap-2">
                  <div className="flex items-center gap-2 min-w-0">
                    <StatusDot tone={allRequiredAccepted ? "ok" : "warning"} />
                    <p className="text-micro font-bold text-foreground uppercase tracking-wide">Aceite das partes primárias</p>
                  </div>
                  <StatusPill tone={allRequiredAccepted ? "ok" : "warning"}>
                    {allRequiredAccepted ? "Unânime" : "Pendente"}
                  </StatusPill>
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-px bg-border">
                  {requiredParties.map((p) => (
                    <div key={p.id} className="flex items-center justify-between px-3 py-2 bg-card">
                      <div className="min-w-0">
                        <div className="text-caption font-bold text-foreground truncate">{p.name || p.email || p.id}</div>
                        <div className="text-micro text-muted-foreground truncate">{ROLE_LABEL[p.role] ?? p.role}</div>
                      </div>
                      <StatusPill tone={isAcceptedBy(p.id) ? "ok" : "warning"}>
                        {isAcceptedBy(p.id) ? "Aceitou" : "Aguardando"}
                      </StatusPill>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Em telas estreitas (≤360px) os dois botões não cabem lado a
                lado: com `flex-1` fixo o "Histórico" passava da borda da tela.
                Empilha abaixo de `xs`-like (sm) e mantém lado a lado no resto. */}
            <div className="mt-4 flex flex-col gap-2 sm:flex-row">
              <Button
                onClick={() => setIsAdding(!isAdding)}
                disabled={savingProposal}
                loading={savingProposal}
                loadingText="Enviando…"
                className="gap-2 sm:flex-1"
              >
                <Plus className="w-4 h-4" /> Nova Proposta
              </Button>
              <Button variant="outline" onClick={() => setShowHistory((v) => !v)} className="gap-2 sm:flex-1">
                <History className="w-4 h-4" /> Histórico ({allProposals.length})
              </Button>
            </div>

            {isAdding && (
              <div className="mt-4 p-4 bg-muted rounded-xl border border-border animate-in fade-in slide-in-from-top-2">
                <h4 className="font-bold text-sm text-foreground mb-3">Nova Proposta</h4>
                <div className="space-y-3">
                  <div>
                    <label className="text-xs font-medium text-foreground">Valor</label>
                    <MoneyInput
                      fractionDigits={0}
                      withSymbol
                      value={newValue}
                      onChange={(v) => setNewValue(v ?? 0)}
                      disabled={savingProposal}
                      className="w-full mt-1 px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                    />
                  </div>
                  <div>
                    <label className="text-xs font-medium text-foreground">Forma de pagamento</label>
                    <input
                      type="text"
                      value={paymentMethod}
                      onChange={(e) => setPaymentMethod(e.target.value)}
                      disabled={savingProposal}
                      className="w-full mt-1 px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                    />
                  </div>
                  <div>
                    <label className="text-xs font-medium text-foreground">Observações</label>
                    <textarea
                      value={notes}
                      onChange={(e) => setNotes(e.target.value)}
                      disabled={savingProposal}
                      rows={3}
                      className="w-full mt-1 px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary resize-none"
                    />
                  </div>
                  <div className="flex justify-end gap-2 mt-2">
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => setIsAdding(false)}
                      disabled={savingProposal}
                      className="text-xs font-bold"
                    >
                      Cancelar
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      onClick={handleAddProposal}
                      loading={savingProposal}
                      loadingText="Salvando…"
                      className="text-xs font-bold"
                    >
                      <Plus className="w-3 h-3" /> Salvar Proposta
                    </Button>
                  </div>
                </div>
              </div>
            )}
          </div>
        </section>

        <section className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden flex flex-col">
          <header className="px-5 py-3 border-b border-border flex items-center gap-2.5">
            <Users className="w-4 h-4 text-primary" aria-hidden="true" />
            <h3 className="text-label font-bold text-foreground uppercase tracking-wide">Participantes</h3>
            <span className="ml-auto text-caption text-muted-foreground tabular-nums">{parties.length}</span>
          </header>
          <div className="p-4">
            <div>
              {parties.length === 0 ? (
                <p className="text-xs text-muted-foreground">Nenhuma parte cadastrada.</p>
              ) : (
                <div className="space-y-2 max-h-64 overflow-auto pr-1">
                  {parties.map((p) => (
                    <div key={p.id} className="flex items-center justify-between px-3 py-2 rounded-xl bg-muted">
                      <div className="min-w-0">
                        <div className="text-xs font-bold text-foreground truncate">{p.name || p.email || p.id}</div>
                        <div className="text-micro text-muted-foreground truncate">
                          {ROLE_LABEL[p.role] ?? p.role}
                          {p.is_primary && " • Primário"}
                        </div>
                      </div>
                      {isAcceptedBy(p.id) && (
                        <StatusPill tone="ok">Aceitou</StatusPill>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </section>
      </div>

      {/* Issue #420 — distribuição de comissão visível direto na aba de
          Proposta para SALE. Read-only para corretor, editável para
          admin/gestor via deals:write (gate dentro do componente). Para
          RENT a comissão recorrente é gerida no rental_charge_config,
          então só renderizamos no fluxo de venda. Lock derivado do
          deal.status (CONVERTED/CANCELLED — mesmo predicado do backend
          em deal.IsBrokerSplitLocked). */}
      {dealDetails && dealDetails.type === "SALE" && (
        <CommissionSplitCard
          deal={dealDetails}
          dealValue={dealDetails.value}
          locked={dealDetails.status === "CONVERTED" || dealDetails.status === "CANCELLED"}
          onUpdated={onRefresh}
        />
      )}

      {showHistory && (
        <section className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
          <header className="px-5 py-3 border-b border-border flex items-center justify-between gap-2">
            <div className="flex items-center gap-2.5">
              <History className="w-4 h-4 text-primary" aria-hidden="true" />
              <h4 className="text-label font-bold text-foreground uppercase tracking-wide">Histórico de Propostas</h4>
            </div>
            <span className="text-caption text-muted-foreground tabular-nums">{allProposals.length} registro{allProposals.length === 1 ? "" : "s"}</span>
          </header>
          <div className="p-5">
            {allProposals.length === 0 ? (
              <p className="text-caption text-muted-foreground">Nenhuma proposta registrada.</p>
            ) : (
              <div className="space-y-2 max-h-72 overflow-auto pr-1">
                {allProposals
                  .slice()
                  .reverse()
                  .map((p) => (
                    <div key={p.id} className="flex items-start justify-between gap-3 px-3 py-2 rounded-xl bg-muted">
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2 flex-wrap">
                          <p className="text-sm font-bold text-foreground tabular-nums">{formatCurrencyBRL0(p.value)}</p>
                          <span className="text-micro text-muted-foreground">
                            {formatISODateBR(p.created_at)}
                          </span>
                          <StatusPill tone={proposalStatusConfig[p.status]?.tone ?? "neutral"}>
                            {proposalStatusConfig[p.status]?.label ?? p.status}
                          </StatusPill>
                        </div>
                        <p className="text-caption text-muted-foreground mt-0.5">{p.payment_method}</p>
                        {p.notes && <p className="text-caption text-muted-foreground mt-1 italic">"{p.notes}"</p>}
                      </div>
                    </div>
                  ))}
              </div>
            )}
          </div>
        </section>
      )}
    </div>
  );
}
