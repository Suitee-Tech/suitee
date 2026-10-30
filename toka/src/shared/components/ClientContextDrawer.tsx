import { chargeDueLabel } from "../utils/chargeDueLabel";
import { formatISODateBR } from "../utils/format";
import { useState, useEffect, useMemo } from "react";
import { clsx } from "clsx";
import { dealOptionLabel } from "../lib/deals/dealOptionLabel";
import api, { unwrapApiData } from "../services/api";
import { User, Briefcase, FileText, MessageSquare, Clock, Calendar, Wallet, Ticket } from "lucide-react";
import { usePermissionsContext } from "../context/PermissionsContext";
import { useNavigate } from "react-router";
import { ExpandableSidePanel } from "./ExpandableSidePanel";

export function ClientContextDrawer({
  open,
  onClose,
  clientId,
}: {
  open: boolean;
  onClose: () => void;
  clientId: string;
}) {
  const [activeTab, setActiveTab] = useState("resumo");
  const [summary, setSummary] = useState<any>(null);
  const [tabData, setTabData] = useState<Record<string, any>>({});
  const [loadingSummary, setLoadingSummary] = useState(false);
  const [loadingTab, setLoadingTab] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const { can, isLoading: permsLoading } = usePermissionsContext();
  const navigate = useNavigate();
  const [dealsSearch, setDealsSearch] = useState("");
  const [dealsStatus, setDealsStatus] = useState<"all" | "active" | "converted" | "cancelled">("all");

  useEffect(() => {
    if (open && clientId) {
      setError(null);
      setActiveTab("resumo");
      setTabData({});
      setDealsSearch("");
      setDealsStatus("all");
      setLoadingSummary(true);
      api
        .get(`/customers/${clientId}/summary`)
        .then((res) => {
          setSummary(unwrapApiData(res));
        })
        .catch((err) => {
          console.error("Error fetching client summary:", err);
          setSummary(null);
          setError("Não foi possível carregar os dados.");
        })
        .finally(() => setLoadingSummary(false));
    }
  }, [open, clientId]);

  useEffect(() => {
    if (!open || !clientId) return;
    if (activeTab === "resumo") return;
    if (tabData[activeTab]) return;

    const endpoints: Record<string, string> = {
      dados: `/customers/${clientId}`,
      leads: `/customers/${clientId}/leads`,
      deals: `/customers/${clientId}/deals?limit=50&offset=0`,
      finance: `/customers/${clientId}/charges?limit=50&offset=0`,
      visits: `/customers/${clientId}/appointments?limit=50&offset=0`,
      tickets: `/customers/${clientId}/tickets?limit=50&offset=0`,
    };
    const url = endpoints[activeTab];
    if (!url) return;

    setLoadingTab(true);
    api
      .get(url)
      .then((res) => {
        setTabData((prev) => ({ ...prev, [activeTab]: unwrapApiData(res) }));
      })
      .catch((err) => {
        console.error("Error fetching client tab:", err);
        setTabData((prev) => ({ ...prev, [activeTab]: { __error: true } }));
      })
      .finally(() => setLoadingTab(false));
  }, [open, clientId, activeTab, tabData]);


  const customerIsClient = !!summary?.customer?.is_client;
  const customerRoleRaw = (summary?.customer?.role as string | undefined) || "";
  const customerRole = customerRoleRaw.toLowerCase();
  const customerName = summary?.customer?.name || summary?.customer?.email || clientId;
  const leadsCount = Number(summary?.leads || 0);

  const customerRoleLabel = useMemo(() => {
    if (customerRole === "owner") return "Proprietário";
    if (customerRole === "buyer") return "Comprador";
    if (customerRole === "tenant" || customerRole === "inquilino" || customerRole === "locatario") return "Locatário";
    return customerRoleRaw || "";
  }, [customerRole, customerRoleRaw]);

  const allTabs = useMemo(() => {
    const interestLabel = customerRole === "owner" ? "Captações" : customerIsClient ? "Interesses" : "Leads";
    const showInterestTab = customerRole === "owner" ? true : !customerIsClient || leadsCount > 1;
    const showDeals = customerRole === "buyer" || customerRole === "tenant" || Number(summary?.deals || 0) > 0;
    const showFinance = customerRole === "tenant" || Number(summary?.open_charges || 0) > 0;
    const showVisits = customerRole !== "owner" || !!summary?.next_visit;
    const tabsRaw = [
      { id: "resumo", label: "Resumo", icon: User, permission: "customers:read" },
      { id: "dados", label: "Dados", icon: User, permission: "customers:read" },
      { id: "leads", label: interestLabel, icon: User, permission: "leads:read", visible: showInterestTab },
      { id: "deals", label: "Negócios", icon: Briefcase, permission: "deals:read", visible: showDeals },
      { id: "finance", label: "Cobranças", icon: Wallet, permission: "charges:read", visible: showFinance },
      { id: "visits", label: "Visitas", icon: Calendar, permission: "leads:read", visible: showVisits },
      { id: "tickets", label: "Tickets", icon: Ticket, permission: "tickets:read" },
      { id: "docs", label: "Docs", icon: FileText, permission: "documents:read", visible: false },
      { id: "messages", label: "Mensagens", icon: MessageSquare, permission: "module:crm", visible: false },
      { id: "timeline", label: "Timeline", icon: Clock, permission: "module:crm", visible: false },
    ] as const;
    return tabsRaw.filter((t: any) => t.visible !== false);
  }, [customerIsClient, customerRole, leadsCount, summary?.deals, summary?.next_visit, summary?.open_charges]);

  const tabs = useMemo(() => allTabs.filter((t) => t.permission === "customers:read" || can(t.permission)), [allTabs, can]);

  const summaryCards = useMemo(() => {
    const cards: Array<{ id: string; label: string; icon: any; value: any; tone?: "danger" | "neutral" }> = [];

    if (customerRole === "owner") {
      cards.push({ id: "captacoes", label: "Captações", icon: User, value: leadsCount });
    } else if (!customerIsClient) {
      cards.push({ id: "leads", label: "Leads", icon: User, value: summary?.leads || 0 });
    } else if (leadsCount > 1) {
      cards.push({ id: "interesses", label: "Interesses", icon: User, value: leadsCount });
    }

    if (can("deals:read") && (customerRole === "buyer" || customerRole === "tenant" || Number(summary?.deals || 0) > 0)) {
      cards.push({ id: "deals", label: "Negócios", icon: Briefcase, value: summary?.deals || 0 });
    }

    if (can("tickets:read")) {
      cards.push({ id: "tickets", label: "Tickets", icon: Ticket, value: summary?.tickets_count || 0 });
    }

    if (can("charges:read") && (customerRole === "tenant" || Number(summary?.open_charges || 0) > 0)) {
      const openCharges = Number(summary?.open_charges || 0);
      cards.push({
        id: "open_charges",
        label: "Cobranças em Aberto",
        icon: Wallet,
        value: new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(openCharges),
        tone: openCharges > 0 ? "danger" : "neutral",
      });
    }

    if (can("leads:read") && (customerRole !== "owner" || !!summary?.next_visit)) {
      cards.push({
        id: "next_visit",
        label: "Próxima Visita",
        icon: Calendar,
        value: summary?.next_visit ? new Date(summary.next_visit).toLocaleString("pt-BR") : "Nenhuma agendada",
      });
    }

    return cards;
  }, [can, customerIsClient, leadsCount, summary, customerRole]);

  const dealsList = useMemo(() => {
    const items = Array.isArray(tabData.deals) ? tabData.deals : [];
    const q = dealsSearch.trim().toLowerCase();
    const byText = q ? items.filter((d: any) => String(d.display_id || d.id || "").toLowerCase().includes(q)) : items;

    const statusFiltered = byText.filter((d: any) => {
      const s = String(d.status || "").toUpperCase();
      if (dealsStatus === "all") return true;
      if (dealsStatus === "converted") return s === "CONVERTED";
      if (dealsStatus === "cancelled") return s === "CANCELLED";
      if (dealsStatus === "active") return s !== "CONVERTED" && s !== "CANCELLED";
      return true;
    });

    return statusFiltered;
  }, [tabData.deals, dealsSearch, dealsStatus]);

  if (!open) return null;

  return (
    <ExpandableSidePanel
      open={open}
      onClose={onClose}
      defaultSize="2xl"
      expandedSize="full"
      zIndex={100}
      renderHeader={({ expandButton, closeButton }) => (
        <div className="px-6 py-4 border-b border-border flex items-start justify-between flex-shrink-0">
          <div className="flex items-center gap-3">
            <div className="p-2.5 bg-muted rounded-xl">
              <User className="w-5 h-5 text-muted-foreground" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-foreground">Perfil 360°</h2>
              <div className="mt-0.5 flex items-center gap-2 flex-wrap">
                <p className="text-xs text-muted-foreground">{customerName}</p>
                {summary?.customer?.email && summary?.customer?.name && <span className="text-xs text-muted-foreground/60">•</span>}
                {summary?.customer?.email && summary?.customer?.name && (
                  <p className="text-xs text-muted-foreground">{summary.customer.email}</p>
                )}
                <span className="text-xs text-muted-foreground/60">•</span>
                <span
                  className={clsx(
                    "text-caption font-bold px-2 py-0.5 rounded-full border",
                    customerIsClient
                      ? "bg-status-ok-soft text-status-ok border-status-ok/30"
                      : "bg-muted text-muted-foreground border-border",
                  )}
                >
                  {customerIsClient ? "Cliente" : "Lead"}
                </span>
                {customerRoleLabel && (
                  <span className="text-caption font-bold px-2 py-0.5 rounded-full border bg-primary-50 text-primary-700 border-primary-200">
                    {customerRoleLabel}
                  </span>
                )}
              </div>
            </div>
          </div>
          <div className="flex items-center gap-2">
            {expandButton}
            {closeButton}
          </div>
        </div>
      )}
    >
      <div className="px-6 border-b border-border flex overflow-x-auto no-scrollbar bg-muted/30 flex-shrink-0">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={clsx(
              "flex items-center gap-2 py-4 px-4 border-b-2 text-sm font-medium whitespace-nowrap transition-colors",
              activeTab === tab.id
                ? "border-primary-600 text-primary-600"
                : "border-transparent text-muted-foreground hover:text-foreground hover:border-border",
            )}
          >
            <tab.icon className="w-4 h-4" />
            {tab.label}
          </button>
        ))}
      </div>

      <div className="p-6 overflow-y-auto flex-1 bg-muted/20">
        {permsLoading ? (
          <div className="flex items-center justify-center h-40">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
          </div>
        ) : !can("customers:read") ? (
          <div className="bg-card p-8 rounded-xl shadow-sm border border-border text-center">
            <h3 className="text-lg font-bold text-foreground">Acesso negado</h3>
            <p className="text-muted-foreground mt-1">Você não tem permissão para visualizar clientes.</p>
          </div>
        ) : loadingSummary ? (
          <div className="flex items-center justify-center h-40">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
          </div>
        ) : error ? (
          <div className="text-center text-muted-foreground mt-10">{error}</div>
        ) : summary ? (
          <div className="space-y-6">
            {activeTab === "resumo" && (
              <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
                {summaryCards.map((card) => (
                  <div key={card.id} className="bg-card p-4 rounded-xl shadow-sm border border-border">
                    <div className="text-muted-foreground text-sm mb-1 flex items-center gap-2">
                      <card.icon className="w-4 h-4" /> {card.label}
                    </div>
                    <div
                      className={clsx(
                        "font-bold",
                        typeof card.value === "number" ? "text-2xl" : "text-sm mt-2",
                        card.tone === "danger" ? "text-status-critical" : "text-foreground",
                      )}
                    >
                      {card.value}
                    </div>
                  </div>
                ))}
              </div>
            )}

            {activeTab !== "resumo" && (
              <div className="bg-card p-6 rounded-xl shadow-sm border border-border">
                {loadingTab ? (
                  <div className="flex items-center justify-center h-32">
                    <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
                  </div>
                ) : tabData[activeTab]?.__error ? (
                  <div className="text-center text-muted-foreground py-10">Não foi possível carregar os dados.</div>
                ) : activeTab === "dados" ? (
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div>
                      <div className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Nome</div>
                      <div className="text-sm font-semibold text-foreground mt-1">
                        {(tabData.dados?.primary_lead?.name || summary.customer?.name) ?? "-"}
                      </div>
                    </div>
                    <div>
                      <div className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Email</div>
                      <div className="text-sm font-semibold text-foreground mt-1">
                        {(tabData.dados?.primary_lead?.email || summary.customer?.email) ?? "-"}
                      </div>
                    </div>
                    <div>
                      <div className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Telefone</div>
                      <div className="text-sm font-semibold text-foreground mt-1">
                        {(tabData.dados?.primary_lead?.phone || summary.customer?.phone) ?? "-"}
                      </div>
                    </div>
                    <div>
                      <div className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Perfil</div>
                      <div className="text-sm font-semibold text-foreground mt-1">{customerRoleLabel || "-"}</div>
                    </div>
                    <div className="md:col-span-2">
                      <div className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Account ID</div>
                      <div className="text-sm font-semibold text-foreground mt-1">
                        {tabData.dados?.account_id || summary.customer?.account_id || "-"}
                      </div>
                    </div>
                  </div>
                ) : activeTab === "leads" ? (
                  <div className="space-y-3">
                    {(tabData.leads || []).length === 0 ? (
                      <div className="text-center text-muted-foreground py-10">
                        {customerRole === "owner"
                          ? "Nenhuma captação encontrada."
                          : customerIsClient
                            ? "Nenhum interesse encontrado."
                            : "Nenhum lead encontrado."}
                      </div>
                    ) : (
                      (tabData.leads || []).map((l: any) => (
                        <div
                          key={l.id}
                          className="p-4 rounded-lg border border-border bg-muted/30 flex items-center justify-between"
                        >
                          <div>
                            <div className="text-sm font-bold text-foreground">{l.name}</div>
                            <div className="text-xs text-muted-foreground mt-1">
                              {l.email} · {l.phone}
                            </div>
                          </div>
                          <div className="text-xs text-muted-foreground">{l.display_id || l.id}</div>
                        </div>
                      ))
                    )}
                  </div>
                ) : activeTab === "deals" ? (
                  <div className="space-y-4">
                    <div className="flex flex-col gap-3">
                      <div className="flex items-center gap-2 flex-wrap">
                        <button
                          onClick={() => setDealsStatus("all")}
                          className={clsx(
                            "px-3 py-1.5 rounded-lg text-xs font-bold border",
                            dealsStatus === "all"
                              ? "bg-primary-50 text-primary-700 border-primary-200"
                              : "bg-card text-muted-foreground border-border hover:bg-muted hover:text-foreground",
                          )}
                        >
                          Todos
                        </button>
                        <button
                          onClick={() => setDealsStatus("active")}
                          className={clsx(
                            "px-3 py-1.5 rounded-lg text-xs font-bold border",
                            dealsStatus === "active"
                              ? "bg-primary-50 text-primary-700 border-primary-200"
                              : "bg-card text-muted-foreground border-border hover:bg-muted hover:text-foreground",
                          )}
                        >
                          Ativos
                        </button>
                        <button
                          onClick={() => setDealsStatus("converted")}
                          className={clsx(
                            "px-3 py-1.5 rounded-lg text-xs font-bold border",
                            dealsStatus === "converted"
                              ? "bg-primary-50 text-primary-700 border-primary-200"
                              : "bg-card text-muted-foreground border-border hover:bg-muted hover:text-foreground",
                          )}
                        >
                          Convertidos
                        </button>
                        <button
                          onClick={() => setDealsStatus("cancelled")}
                          className={clsx(
                            "px-3 py-1.5 rounded-lg text-xs font-bold border",
                            dealsStatus === "cancelled"
                              ? "bg-primary-50 text-primary-700 border-primary-200"
                              : "bg-card text-muted-foreground border-border hover:bg-muted hover:text-foreground",
                          )}
                        >
                          Cancelados
                        </button>
                      </div>
                      <input
                        value={dealsSearch}
                        onChange={(e) => setDealsSearch(e.target.value)}
                        placeholder="Buscar por ID do negócio…"
                        className="w-full px-3 py-2 rounded-lg border border-border bg-card text-sm outline-none focus:ring-2 focus:ring-primary-200 focus:border-primary-300"
                      />
                    </div>

                    {dealsList.length === 0 ? (
                      <div className="text-center text-muted-foreground py-10">Nenhum negócio encontrado.</div>
                    ) : (
                      <div className="space-y-3">
                        {dealsList.map((d: any) => {
                          const status = String(d.status || "").toUpperCase();
                          const tone =
                            status === "CONVERTED"
                              ? "emerald"
                              : status === "CANCELLED"
                                ? "red"
                                : status === "CONTRACT"
                                  ? "blue"
                                  : status === "ANALYSIS"
                                    ? "amber"
                                    : "slate";
                          return (
                            <div
                              key={d.id}
                              className="p-4 rounded-lg border border-border bg-muted/30 flex items-center justify-between gap-4"
                            >
                              <div className="min-w-0">
                                <div className="flex items-center gap-2">
                                  <div className="text-sm font-bold text-foreground truncate">
                                    {dealOptionLabel({ deal: d }).title}
                                  </div>
                                  <span
                                    className={clsx(
                                      "text-caption font-bold px-2 py-0.5 rounded-full border",
                                      tone === "emerald" && "bg-status-ok-soft text-status-ok border-status-ok/30",
                                      tone === "red" && "bg-status-critical-soft text-status-critical border-status-critical/30",
                                      tone === "blue" && "bg-status-info-soft text-status-info border-status-info/30",
                                      tone === "amber" && "bg-status-warning-soft text-status-warning border-status-warning/30",
                                      tone === "slate" && "bg-muted text-muted-foreground border-border",
                                    )}
                                  >
                                    {status || "—"}
                                  </span>
                                </div>
                                <div className="text-xs text-muted-foreground mt-1">
                                  {d.type} · {d.created_at ? new Date(d.created_at).toLocaleDateString("pt-BR") : "-"}
                                </div>
                              </div>
                              <div className="flex items-center gap-3">
                                <div className="text-sm font-bold text-foreground whitespace-nowrap">
                                  {new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(d.value || 0)}
                                </div>
                                <button
                                  onClick={() => {
                                    navigate(`/esteira/deals/${d.id}`);
                                    onClose();
                                  }}
                                  className="px-3 py-1.5 rounded-lg text-xs font-bold border border-border bg-card text-foreground hover:bg-muted"
                                >
                                  Abrir
                                </button>
                              </div>
                            </div>
                          );
                        })}
                      </div>
                    )}
                  </div>
                ) : activeTab === "finance" ? (
                  <div className="space-y-3">
                    {(tabData.finance || []).length === 0 ? (
                      <div className="text-center text-muted-foreground py-10">Nenhuma cobrança encontrada.</div>
                    ) : (
                      (tabData.finance || []).map((ch: any) => (
                        <div
                          key={ch.id}
                          className="p-4 rounded-lg border border-border bg-muted/30 flex items-center justify-between"
                        >
                          <div>
                            <div className="text-sm font-bold text-foreground">{ch.description || ch.id}</div>
                            <div className="text-xs text-muted-foreground mt-1">
                              {chargeDueLabel(ch).label} · {formatISODateBR(ch.due_date) || "-"}
                            </div>
                          </div>
                          <div className="text-sm font-bold text-foreground">
                            {new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(ch.total_amount || 0)}
                          </div>
                        </div>
                      ))
                    )}
                  </div>
                ) : activeTab === "visits" ? (
                  <div className="space-y-3">
                    {(tabData.visits || []).length === 0 ? (
                      <div className="text-center text-muted-foreground py-10">Nenhuma visita encontrada.</div>
                    ) : (
                      (tabData.visits || []).map((a: any) => (
                        <div
                          key={a.id}
                          className="p-4 rounded-lg border border-border bg-muted/30 flex items-center justify-between"
                        >
                          <div>
                            <div className="text-sm font-bold text-foreground">{a.title || a.type}</div>
                            <div className="text-xs text-muted-foreground mt-1">
                              {a.date} {a.time} · {a.status}
                            </div>
                          </div>
                          <div className="text-xs text-muted-foreground">{a.property || "-"}</div>
                        </div>
                      ))
                    )}
                  </div>
                ) : activeTab === "tickets" ? (
                  <div className="space-y-3">
                    {(tabData.tickets || []).length === 0 ? (
                      <div className="text-center text-muted-foreground py-10">Nenhum ticket encontrado.</div>
                    ) : (
                      (tabData.tickets || []).map((t: any) => (
                        <div
                          key={t.id}
                          className="p-4 rounded-lg border border-border bg-muted/30 flex items-center justify-between"
                        >
                          <div>
                            <div className="text-sm font-bold text-foreground">{t.subject || t.id}</div>
                            <div className="text-xs text-muted-foreground mt-1">
                              {t.status} · {t.priority}
                            </div>
                          </div>
                          <div className="text-xs text-muted-foreground">
                            {t.created_at ? new Date(t.created_at).toLocaleDateString("pt-BR") : "-"}
                          </div>
                        </div>
                      ))
                    )}
                  </div>
                ) : null}
              </div>
            )}
          </div>
        ) : (
          <div className="text-center text-muted-foreground mt-10">Não foi possível carregar os dados.</div>
        )}
      </div>
    </ExpandableSidePanel>
  );
}
