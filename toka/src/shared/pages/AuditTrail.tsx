import { useCallback, useEffect, useMemo, useState } from "react";
import { downloadCsv } from "../utils/csvExport";
import {
  AlertTriangle,
  ArrowRight,
  CalendarRange,
  ChevronDown,
  Clock,
  FileSpreadsheet,
  Filter,
  History,
  Loader2,
  Pencil,
  Plus,
  RotateCcw,
  Search,
  Trash2,
  User,
  X,
} from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { clsx } from "clsx";
import { EmptyState } from "../ui/EmptyState";
import { Button } from "../ui/button";
import { maskAuditValue } from "../utils/pii-mask";
import { auditService, type AuditTrailEntry } from "../services/audit.service";

const PAGE_SIZE = 50;

// ── Apresentação ─────────────────────────────────────────────────────────────

type ActionKind = "create" | "update" | "delete" | "status" | "other";

function actionKind(action: string): ActionKind {
  const a = action.toLowerCase();
  if (/(delete|remove|revoke|exclu|\.deleted|cancel)/.test(a)) return "delete";
  if (/(create|register|signup|send|publish)/.test(a)) return "create";
  if (/(status|state|approve|reject|login|logout|refresh|switch|transition|move)/.test(a)) return "status";
  if (/(update|edit|change|patch|profile|role|banking)/.test(a)) return "update";
  return "other";
}

const ACTION_STYLE: Record<ActionKind, { icon: typeof Plus; color: string; bg: string; border: string }> = {
  create: { icon: Plus, color: "text-status-ok", bg: "bg-status-ok-soft", border: "border-status-ok/30" },
  update: { icon: Pencil, color: "text-primary-700", bg: "bg-primary-50", border: "border-primary-200" },
  delete: { icon: Trash2, color: "text-status-critical", bg: "bg-status-critical-soft", border: "border-status-critical/30" },
  status: { icon: AlertTriangle, color: "text-status-warning", bg: "bg-status-warning-soft", border: "border-status-warning/30" },
  other: { icon: History, color: "text-muted-foreground", bg: "bg-muted", border: "border-border" },
};

const ENTITY_TYPE_LABELS: Record<string, string> = {
  Commission: "Comissão",
  Deal: "Negociação",
  Charge: "Cobrança",
  lead: "Lead",
  Lead: "Lead",
  Transaction: "Transação",
  Contract: "Contrato",
  auth: "Autenticação",
  company_member: "Membro da empresa",
  User: "Usuário",
  ticket: "Ticket",
  Collection: "Cobrança",
  ChecklistItem: "Checklist",
  company_invitation: "Convite",
  company_group: "Grupo",
  access_profile: "Perfil de acesso",
  webhook_endpoint: "Webhook",
  Rental: "Locação",
  WhatsAppMessage: "WhatsApp",
  SMSMessage: "SMS",
  TenantSettings: "Configurações",
  roulette_config: "Roleta",
  role_permissions: "Matriz de permissões",
  PayoutOrder: "ordem de pagamento",
  Party: "Parte",
  maintenance: "Manutenção",
  Inspection: "Vistoria",
  FinancialProof: "Comprovante",
  Document: "Documento",
  company: "Empresa",
  Tenant: "empresa",
  Transfer: "Repasse",
};

function entityLabel(entityType: string): string {
  return ENTITY_TYPE_LABELS[entityType] || entityType;
}

// ── Formatação ───────────────────────────────────────────────────────────────

function formatTimestamp(ts: string): { date: string; time: string; relative: string } {
  const d = new Date(ts);
  const date = d.toLocaleDateString("pt-BR", { day: "2-digit", month: "short", year: "numeric" });
  const time = d.toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" });

  const now = new Date();
  const diffMs = now.getTime() - d.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  let relative = "";
  if (diffMins < 1) relative = "Agora mesmo";
  else if (diffMins < 60) relative = `Há ${diffMins} min`;
  else if (diffHours < 24) relative = `Há ${diffHours}h`;
  else if (diffDays < 7) relative = `Há ${diffDays} dia${diffDays > 1 ? "s" : ""}`;
  else relative = date;

  return { date, time, relative };
}

function stringifyValue(value: unknown): string {
  if (value === undefined || value === null) return "";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

/** Campos cujo valor mudou entre before/after — ou o conjunto inteiro quando só existe um lado. */
function diffFields(before?: Record<string, unknown>, after?: Record<string, unknown>) {
  const out: { field: string; oldValue: string; newValue: string }[] = [];
  const keys = new Set([...Object.keys(before ?? {}), ...Object.keys(after ?? {})]);
  for (const key of keys) {
    const oldV = stringifyValue(before?.[key]);
    const newV = stringifyValue(after?.[key]);
    if (oldV === newV) continue;
    out.push({ field: key, oldValue: oldV, newValue: newV });
  }
  return out;
}

// ── Página ───────────────────────────────────────────────────────────────────

export function AuditTrail() {
  const [entries, setEntries] = useState<AuditTrailEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(false);

  const [entityTypeFilter, setEntityTypeFilter] = useState("");
  const [actionFilter, setActionFilter] = useState("");
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");
  const [searchTerm, setSearchTerm] = useState("");
  const [expandedEntry, setExpandedEntry] = useState<string | null>(null);
  const [entityFilterOpen, setEntityFilterOpen] = useState(false);
  const [actionFilterOpen, setActionFilterOpen] = useState(false);

  const load = useCallback(
    async (filters: { entity_type?: string; action?: string; from?: string; to?: string; created_before?: string }) => {
      const rows = await auditService.listAuditTrail({ ...filters, limit: PAGE_SIZE });
      return rows;
    },
    [],
  );

  // Primeira página: refaz quando um filtro de servidor muda.
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    load({ entity_type: entityTypeFilter, action: actionFilter, from: dateFrom, to: dateTo ? `${dateTo}T23:59:59` : "" })
      .then((rows) => {
        if (cancelled) return;
        setEntries(rows);
        setHasMore(rows.length === PAGE_SIZE);
      })
      .catch((e: unknown) => {
        if (cancelled) return;
        setError(e instanceof Error ? e.message : "Não foi possível carregar a auditoria.");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [load, entityTypeFilter, actionFilter, dateFrom, dateTo]);

  const loadMore = useCallback(async () => {
    if (!entries.length || loadingMore) return;
    const cursor = entries[entries.length - 1].created_at;
    setLoadingMore(true);
    try {
      const rows = await load({ entity_type: entityTypeFilter, action: actionFilter, from: dateFrom, to: dateTo ? `${dateTo}T23:59:59` : "", created_before: cursor });
      setEntries((prev) => {
        const seen = new Set(prev.map((e) => e.id));
        return [...prev, ...rows.filter((r) => !seen.has(r.id))];
      });
      setHasMore(rows.length === PAGE_SIZE);
    } finally {
      setLoadingMore(false);
    }
  }, [entries, loadingMore, load, entityTypeFilter, actionFilter, dateFrom, dateTo]);

  const entityTypes = useMemo(() => {
    const set = new Set(entries.map((e) => e.entity_type));
    return Array.from(set).sort();
  }, [entries]);

  const actions = useMemo(() => {
    const set = new Set(entries.map((e) => e.action));
    return Array.from(set).sort();
  }, [entries]);

  const filteredEntries = useMemo(() => {
    if (!searchTerm.trim()) return entries;
    const q = searchTerm.trim().toLowerCase();
    return entries.filter((e) => {
      const hay = [
        e.action,
        entityLabel(e.entity_type),
        e.entity_id,
        e.actor_id ?? "",
        ...Object.entries(e.before ?? {}).flatMap(([k, v]) => [k, stringifyValue(v)]),
        ...Object.entries(e.after ?? {}).flatMap(([k, v]) => [k, stringifyValue(v)]),
      ]
        .join(" ")
        .toLowerCase();
      return hay.includes(q);
    });
  }, [entries, searchTerm]);

  const hasActiveFilters = Boolean(entityTypeFilter || actionFilter || dateFrom || dateTo || searchTerm.trim());

  const clearFilters = () => {
    setEntityTypeFilter("");
    setActionFilter("");
    setDateFrom("");
    setDateTo("");
    setSearchTerm("");
  };

  const handleExportCSV = () => {
    const sep = ";";
    const lines: string[] = [];
    lines.push(["Data/Hora", "Usuário", "Entidade", "ID", "Ação", "Campo", "Valor Anterior", "Novo Valor"].map((h) => `"${h}"`).join(sep));
    filteredEntries.forEach((entry) => {
      const ts = formatTimestamp(entry.created_at);
      for (const ch of diffFields(entry.before, entry.after)) {
        lines.push(
          [`${ts.date} ${ts.time}`, entry.actor_id ?? entry.actor_type, entityLabel(entry.entity_type), entry.entity_id, entry.action, ch.field, maskAuditValue(ch.field, ch.oldValue), maskAuditValue(ch.field, ch.newValue)]
            .map((v) => `"${(v || "").replace(/"/g, '""')}"`)
            .join(sep),
        );
      }
    });
    const content = lines.join("\r\n");
    downloadCsv(`audit-trail-global-${new Date().toISOString().split("T")[0]}.csv`, content);
  };

  return (
    <div className="p-4 sm:p-6 lg:p-8 max-w-[1400px] mx-auto min-h-full">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div className="flex items-center gap-3">
          <div className="p-3 bg-muted rounded-2xl">
            <History className="w-6 h-6 text-foreground" />
          </div>
          <div>
            <h1 className="text-2xl font-bold text-foreground">Trilha de auditoria</h1>
            <p className="text-sm text-muted-foreground mt-0.5">Auditoria centralizada do sistema — todas as mutações de todos os serviços</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={handleExportCSV} disabled={filteredEntries.length === 0}>
            <FileSpreadsheet className="w-4 h-4" /> Exportar CSV
          </Button>
        </div>
      </div>

      <div className="bg-card rounded-2xl border border-border p-4 mb-6">
        <div className="flex flex-col lg:flex-row items-start lg:items-center gap-3">
          <div className="relative flex-1 min-w-0 w-full">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
            <input
              type="text"
              placeholder="Buscar na página por ação, entidade, ID ou valor..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className="w-full pl-9 pr-8 py-2.5 bg-muted border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 transition-all placeholder:text-muted-foreground"
            />
            {searchTerm && (
              <Button variant="ghost" size="icon" onClick={() => setSearchTerm("")} className="absolute right-2.5 top-1/2 -translate-y-1/2 size-6 rounded-full text-muted-foreground">
                <X className="w-3.5 h-3.5" />
              </Button>
            )}
          </div>

          <div className="flex items-center gap-2 flex-wrap">
            <Filter className="w-4 h-4 text-muted-foreground" />

            <div className="relative">
              <Button variant="outline" size="sm" onClick={() => { setEntityFilterOpen(!entityFilterOpen); setActionFilterOpen(false); }} className="font-medium">
                {entityTypeFilter ? entityLabel(entityTypeFilter) : "Todas as entidades"}
                <ChevronDown className="w-3.5 h-3.5 text-muted-foreground" />
              </Button>
              {entityFilterOpen && (
                <div className="absolute top-full left-0 mt-1 bg-card border border-border rounded-xl shadow-lg z-20 min-w-[200px] max-h-64 overflow-y-auto py-1">
                  <Button key="all" variant="ghost" size="sm" onClick={() => { setEntityTypeFilter(""); setEntityFilterOpen(false); }} className={clsx("w-full justify-start font-normal", !entityTypeFilter && "font-bold text-primary-600 bg-primary-50")}>
                    Todas as entidades
                  </Button>
                  {entityTypes.map((t) => (
                    <Button key={t} variant="ghost" size="sm" onClick={() => { setEntityTypeFilter(t); setEntityFilterOpen(false); }} className={clsx("w-full justify-start font-normal", entityTypeFilter === t && "font-bold text-primary-600 bg-primary-50")}>
                      {entityLabel(t)}
                    </Button>
                  ))}
                </div>
              )}
            </div>

            <div className="relative">
              <Button variant="outline" size="sm" onClick={() => { setActionFilterOpen(!actionFilterOpen); setEntityFilterOpen(false); }} className="font-medium">
                {actionFilter || "Todas as ações"}
                <ChevronDown className="w-3.5 h-3.5 text-muted-foreground" />
              </Button>
              {actionFilterOpen && (
                <div className="absolute top-full left-0 mt-1 bg-card border border-border rounded-xl shadow-lg z-20 min-w-[200px] max-h-64 overflow-y-auto py-1">
                  <Button key="all" variant="ghost" size="sm" onClick={() => { setActionFilter(""); setActionFilterOpen(false); }} className={clsx("w-full justify-start font-normal", !actionFilter && "font-bold text-primary-600 bg-primary-50")}>
                    Todas as ações
                  </Button>
                  {actions.map((a) => (
                    <Button key={a} variant="ghost" size="sm" onClick={() => { setActionFilter(a); setActionFilterOpen(false); }} className={clsx("w-full justify-start font-normal", actionFilter === a && "font-bold text-primary-600 bg-primary-50")}>
                      <span className="font-mono text-xs">{a}</span>
                    </Button>
                  ))}
                </div>
              )}
            </div>

            <div className="flex items-center gap-1.5">
              <CalendarRange className="w-4 h-4 text-muted-foreground" />
              <input type="date" value={dateFrom} onChange={(e) => setDateFrom(e.target.value)} className="px-2 py-1.5 text-sm border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 bg-card" title="Data inicial" />
              <span className="text-xs text-muted-foreground">até</span>
              <input type="date" value={dateTo} onChange={(e) => setDateTo(e.target.value)} className="px-2 py-1.5 text-sm border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 bg-card" title="Data final" />
            </div>

            {hasActiveFilters && (
              <Button variant="ghost" size="sm" onClick={clearFilters} className="text-xs text-muted-foreground hover:text-foreground">
                <RotateCcw className="w-3.5 h-3.5" /> Limpar
              </Button>
            )}
          </div>
        </div>
      </div>

      <div className="bg-card rounded-2xl border border-border">
        <div className="px-6 py-3 border-b border-border flex items-center justify-between">
          <p className="text-sm font-medium text-foreground">
            {filteredEntries.length > 0 ? `${filteredEntries.length} registro${filteredEntries.length !== 1 ? "s" : ""} carregado${filteredEntries.length !== 1 ? "s" : ""}${hasMore ? " (mais antigos disponíveis)" : ""}` : "Nenhum registro"}
          </p>
          <span className="text-micro text-muted-foreground font-medium uppercase tracking-wider">Ordenado por data (mais recente)</span>
        </div>

        <div className="p-6">
          {loading ? (
            <div className="flex items-center justify-center py-12 text-muted-foreground gap-2">
              <Loader2 className="w-5 h-5 animate-spin" /> Carregando auditoria...
            </div>
          ) : error ? (
            <EmptyState icon={AlertTriangle} title="Não foi possível carregar" message={error} className="border-none bg-transparent my-12" />
          ) : filteredEntries.length === 0 ? (
            <EmptyState
              icon={History}
              title="Nenhum registro de alteração encontrado"
              message={hasActiveFilters ? "Tente ajustar os filtros ou busca" : "As mutações do sistema aparecerão aqui"}
              action={
                hasActiveFilters && (
                  <Button variant="link" size="sm" onClick={clearFilters} className="mx-auto">
                    <RotateCcw className="w-4 h-4" /> Limpar todos os filtros
                  </Button>
                )
              }
              className="border-none bg-transparent my-12"
            />
          ) : (
            <div className="relative">
              <div className="absolute left-[19px] top-3 bottom-3 w-px bg-muted" />

              <div className="space-y-1">
                {filteredEntries.map((entry) => {
                  const style = ACTION_STYLE[actionKind(entry.action)];
                  const Icon = style.icon;
                  const ts = formatTimestamp(entry.created_at);
                  const isExpanded = expandedEntry === entry.id;
                  const fields = diffFields(entry.before, entry.after);

                  return (
                    <div key={entry.id} className="relative pl-12">
                      <div className={clsx("absolute left-[11px] top-4 w-[18px] h-[18px] rounded-full border-2 flex items-center justify-center z-10", style.bg, style.border)}>
                        <div className={clsx("w-2 h-2 rounded-full", style.color.replace("text-", "bg-"))} />
                      </div>

                      <button
                        onClick={() => setExpandedEntry(isExpanded ? null : entry.id)}
                        className={clsx("w-full text-left p-3 rounded-xl border transition-all", isExpanded ? "bg-card border-border shadow-sm" : "bg-transparent border-transparent hover:bg-muted hover:border-border")}
                      >
                        <div className="flex items-start justify-between gap-2">
                          <div className="flex items-center gap-2 flex-wrap min-w-0">
                            <span className={clsx("inline-flex items-center gap-1 px-2 py-0.5 text-micro font-bold rounded-md border font-mono", style.bg, style.color, style.border)}>
                              <Icon className="w-3 h-3" /> {entry.action}
                            </span>
                            <span className="text-micro font-bold px-1.5 py-0.5 rounded border bg-muted text-muted-foreground border-border">
                              {entityLabel(entry.entity_type)}
                            </span>
                            <span className="text-micro font-bold text-muted-foreground bg-muted px-1.5 py-0.5 rounded border border-border truncate max-w-[180px]">{entry.entity_id}</span>
                          </div>
                          <div className="flex items-center gap-2 shrink-0">
                            <span className="text-micro text-muted-foreground">{ts.relative}</span>
                            {isExpanded ? <ChevronDown className="w-3.5 h-3.5 text-muted-foreground" /> : <ChevronDown className="w-3.5 h-3.5 text-muted-foreground rotate-[-90deg]" />}
                          </div>
                        </div>

                        <p className="text-xs text-muted-foreground mt-1.5">
                          {fields.length === 1 ? `Campo "${fields[0].field}" alterado` : fields.length > 0 ? `${fields.length} campos alterados` : "Sem diff (consulte os detalhes)"}
                        </p>

                        <AnimatePresence>
                          {isExpanded && (
                            <motion.div initial={{ opacity: 0, height: 0 }} animate={{ opacity: 1, height: "auto" }} exit={{ opacity: 0, height: 0 }} className="mt-3 space-y-3">
                              <div className="flex items-center gap-3 text-xs text-muted-foreground flex-wrap">
                                <div className="flex items-center gap-1.5">
                                  <User className="w-3.5 h-3.5 text-muted-foreground" />
                                  <span className="font-medium">{entry.actor_id || entry.actor_type}</span>
                                  <span className="text-muted-foreground/60">({entry.actor_type})</span>
                                </div>
                                <span className="text-muted-foreground/60">|</span>
                                <div className="flex items-center gap-1.5">
                                  <Clock className="w-3.5 h-3.5 text-muted-foreground" />
                                  <span>{ts.date} às {ts.time}</span>
                                </div>
                                {entry.metadata?.request_id ? (
                                  <>
                                    <span className="text-muted-foreground/60">|</span>
                                    <span className="font-mono text-micro">req {String(entry.metadata.request_id)}</span>
                                  </>
                                ) : null}
                              </div>

                              <div className="space-y-2">
                                {fields.length === 0 && (
                                  <p className="text-xs text-muted-foreground italic">Registro sem campos de diff — a ação em si é o evento auditado.</p>
                                )}
                                {fields.map((ch, ci) => (
                                  <div key={ci} className="bg-muted rounded-lg p-2.5 border border-border">
                                    <p className="text-micro font-bold text-muted-foreground uppercase tracking-wider mb-1.5 break-all">{ch.field}</p>
                                    <div className="flex items-center gap-2 text-xs flex-wrap">
                                      {ch.oldValue ? (
                                        <>
                                          <span className="px-2 py-0.5 bg-status-critical-soft text-status-critical rounded border border-status-critical/20 font-medium line-through break-all">
                                            {maskAuditValue(ch.field, ch.oldValue)}
                                          </span>
                                          <ArrowRight className="w-3 h-3 text-muted-foreground shrink-0" />
                                          <span className="px-2 py-0.5 bg-status-ok-soft text-status-ok rounded border border-status-ok/20 font-medium break-all">
                                            {maskAuditValue(ch.field, ch.newValue)}
                                          </span>
                                        </>
                                      ) : (
                                        <span className="px-2 py-0.5 bg-status-ok-soft text-status-ok rounded border border-status-ok/20 font-medium break-all">
                                          {maskAuditValue(ch.field, ch.newValue)}
                                        </span>
                                      )}
                                    </div>
                                  </div>
                                ))}
                              </div>
                            </motion.div>
                          )}
                        </AnimatePresence>
                      </button>
                    </div>
                  );
                })}
              </div>

              {hasMore && (
                <div className="flex justify-center mt-6">
                  <Button variant="outline" size="sm" onClick={loadMore} disabled={loadingMore} className="text-muted-foreground hover:text-foreground">
                    {loadingMore ? (
                      <>
                        <Loader2 className="w-4 h-4 animate-spin" /> Carregando...
                      </>
                    ) : (
                      "Carregar mais"
                    )}
                  </Button>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
