import { useMemo, useState } from "react";
import { downloadCsv } from "../utils/csvExport";
import { useQuery } from "@tanstack/react-query";
import {
  X,
  Clock,
  Pencil,
  Plus,
  AlertTriangle,
  Trash2,
  ArrowRight,
  ChevronDown,
  ChevronUp,
  History,
  User,
  Filter,
  Search,
  FileSpreadsheet,
  FileText as FileTextIcon,
  CalendarRange,
} from "lucide-react";
import { motion } from "motion/react";
import { clsx } from "clsx";
import type { AuditLogEntry } from "../types/auditLog";
import { ExpandableSidePanel } from "./ExpandableSidePanel";
import { Button } from "../ui/button";
import { maskAuditValue } from "../utils/pii-mask";
import { dealAuditLogQueryKey, fetchDealAuditLog } from "../deals/queries";
import type { DealAuditLogEntry } from "../services/deal.service";
import { useAuth } from "../context/AuthContext";

export interface AuditTrailPanelProps {
  entityId?: string;
  entityLabel?: string;
  entityType?: AuditLogEntry["entityType"];
  onClose: () => void;
  getAuditLog?: (entityId?: string, entityType?: AuditLogEntry["entityType"]) => AuditLogEntry[];
}

const ACTION_CONFIG: Record<AuditLogEntry["action"], { label: string; icon: typeof Clock; color: string; bg: string; border: string }> = {
  criacao: {
    label: "Criacao",
    icon: Plus,
    color: "text-status-ok",
    bg: "bg-status-ok-soft",
    border: "border-status-ok/30",
  },
  edicao: {
    label: "Edição",
    icon: Pencil,
    color: "text-status-info",
    bg: "bg-status-info-soft",
    border: "border-status-info/30",
  },
  exclusao: {
    label: "Exclusão",
    icon: Trash2,
    color: "text-status-critical",
    bg: "bg-status-critical-soft",
    border: "border-status-critical/30",
  },
  status_change: {
    label: "Alteração de Status",
    icon: AlertTriangle,
    color: "text-status-warning",
    bg: "bg-status-warning-soft",
    border: "border-status-warning/30",
  },
};

const ENTITY_TYPE_LABELS: Record<AuditLogEntry["entityType"], { label: string; color: string; bg: string }> = {
  transaction: { label: "Transacao", color: "text-status-info", bg: "bg-status-info-soft" },
  contract: { label: "Contrato", color: "text-violet-700", bg: "bg-violet-50" },
  document: { label: "Documento", color: "text-cyan-700", bg: "bg-cyan-50" },
  ticket: { label: "Ticket", color: "text-orange-700", bg: "bg-orange-50" },
  property: { label: "Imóvel", color: "text-green-700", bg: "bg-green-50" },
  deal: { label: "Negociacao", color: "text-pink-700", bg: "bg-pink-50" },
  lead: { label: "Lead", color: "text-amber-700", bg: "bg-amber-50" },
  roulette_config: { label: "Config Roleta", color: "text-yellow-700", bg: "bg-yellow-50" },
  auth: { label: "Autenticação", color: "text-slate-700", bg: "bg-slate-50" },
  setting: { label: "Configuração", color: "text-stone-700", bg: "bg-stone-50" },
};

const dealStatusLabels: Record<string, string> = {
  NEGOTIATION: "Em Negociação",
  PROPOSAL: "Proposta",
  ANALYSIS: "Análise",
  CONTRACT: "Contrato",
  CONVERTED: "Concluído",
  CANCELLED: "Perdido",
};

const apiLeadStatusToLeadStatus: Record<string, string> = {
  NEW: "Novo",
  PENDING_ACCEPTANCE: "Em Atendimento",
  CONTACTED: "Em Atendimento",
  QUALIFIED: "Qualificado",
  VISIT_SCHEDULED: "Visita Agendada",
  PROPOSAL: "Proposta Enviada",
  NEGOTIATION: "Em Negociação",
  CONVERTED: "Convertido",
  LOST: "Perdido",
};

const formatAuditValue = (field: string, value: string) => {
  if (!value) return value;
  const v = value.trim();
  if (!v) return value;
  const f = (field || "").toLowerCase();
  if (f.includes("status")) return apiLeadStatusToLeadStatus[v] || dealStatusLabels[v] || value;
  if (f.includes("etapa") || f.includes("stage")) return dealStatusLabels[v] || value;
  return value;
};

function mapServerActionToLocalAction(action: string): AuditLogEntry["action"] {
  const a = (action || "").toLowerCase();
  if (a === "create" || a.endsWith(".created")) return "criacao";
  if (a === "delete" || a === "soft_delete" || a.endsWith(".deleted")) return "exclusao";
  if (a.includes("status") || a.endsWith(".status_changed")) return "status_change";
  return "edicao";
}

function mapServerEntryToLocal(entry: DealAuditLogEntry): AuditLogEntry {
  const before = entry.before ?? {};
  const after = entry.after ?? {};
  const keys = new Set<string>([...Object.keys(before), ...Object.keys(after)]);
  const changes: AuditLogEntry["changes"] = [];
  for (const field of keys) {
    const oldValue = (before as any)[field];
    const newValue = (after as any)[field];
    if (JSON.stringify(oldValue) === JSON.stringify(newValue)) continue;
    changes.push({
      field,
      oldValue: oldValue == null ? "" : String(oldValue),
      newValue: newValue == null ? "" : String(newValue),
    });
  }
  if (changes.length === 0) {
    changes.push({ field: "acao", oldValue: "", newValue: entry.action });
  }
  return {
    id: entry.id,
    entityType: "deal",
    entityId: entry.entity_id,
    action: mapServerActionToLocalAction(entry.action),
    changes,
    timestamp: entry.created_at,
    user: entry.actor_id || entry.actor_type || "Sistema",
  };
}

function formatTimestamp(ts: string): { date: string; time: string; relative: string } {
  const d = new Date(ts);
  const date = d.toLocaleDateString("pt-BR", {
    day: "2-digit",
    month: "short",
    year: "numeric",
  });
  const time = d.toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" });

  const now = new Date();
  const diffMs = now.getTime() - d.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  let relative = "";
  if (diffMins < 1) relative = "Agora mesmo";
  else if (diffMins < 60) relative = `Ha ${diffMins} min`;
  else if (diffHours < 24) relative = `Ha ${diffHours}h`;
  else if (diffDays < 7) relative = `Ha ${diffDays} dia${diffDays > 1 ? "s" : ""}`;
  else relative = date;

  return { date, time, relative };
}

export function AuditTrailPanel({ entityId, entityLabel, entityType, onClose, getAuditLog }: AuditTrailPanelProps) {
  const { user } = useAuth();
  const tenantId = user?.current_company_id;
  const [expandedEntry, setExpandedEntry] = useState<string | null>(null);
  const [actionFilter, setActionFilter] = useState<string>("Todos");
  const [entityTypeFilter, setEntityTypeFilter] = useState<string>(entityType || "Todos");
  const [filterOpen, setFilterOpen] = useState(false);
  const [entityFilterOpen, setEntityFilterOpen] = useState(false);
  const [searchTerm, setSearchTerm] = useState("");
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");
  const [showDateFilter, setShowDateFilter] = useState(false);

  const isDealScoped = entityType === "deal" && !!entityId;
  const serverAuditQuery = useQuery({
    queryKey: dealAuditLogQueryKey(entityId || "", tenantId),
    queryFn: () => fetchDealAuditLog(entityId!),
    enabled: Boolean(isDealScoped && tenantId),
  });

  const localEntries = (getAuditLog ? getAuditLog(entityId, entityType) : []) ?? [];
  const serverEntries = useMemo<AuditLogEntry[]>(() => (serverAuditQuery.data ?? []).map(mapServerEntryToLocal), [serverAuditQuery.data]);
  const allEntries = isDealScoped && serverEntries.length > 0 ? serverEntries : localEntries;
  const filteredEntries = allEntries.filter((e) => {
    const matchesAction = actionFilter === "Todos" || e.action === actionFilter;
    const matchesEntityType = entityTypeFilter === "Todos" || e.entityType === entityTypeFilter;

    let matchesDate = true;
    if (dateFrom) matchesDate = matchesDate && e.timestamp >= dateFrom;
    if (dateTo) matchesDate = matchesDate && e.timestamp <= dateTo + "T23:59:59";

    let matchesSearch = true;
    if (searchTerm.trim()) {
      const q = searchTerm.toLowerCase();
      const inUser = e.user.toLowerCase().includes(q);
      const inEntityId = e.entityId.toLowerCase().includes(q);
      const inChanges = e.changes.some(
        (ch) =>
          ch.field.toLowerCase().includes(q) ||
          ch.oldValue.toLowerCase().includes(q) ||
          ch.newValue.toLowerCase().includes(q) ||
          formatAuditValue(ch.field, ch.oldValue).toLowerCase().includes(q) ||
          formatAuditValue(ch.field, ch.newValue).toLowerCase().includes(q),
      );
      const inAction = (ACTION_CONFIG[e.action]?.label || "").toLowerCase().includes(q);
      const inEntityTypeLabel = (ENTITY_TYPE_LABELS[e.entityType]?.label || "").toLowerCase().includes(q);
      matchesSearch = inUser || inEntityId || inChanges || inAction || inEntityTypeLabel;
    }

    return matchesAction && matchesEntityType && matchesSearch && matchesDate;
  });

  const toggleExpand = (id: string) => {
    setExpandedEntry((prev) => (prev === id ? null : id));
  };

  const handleExportCSV = () => {
    const sep = ";";
    const lines: string[] = [];

    lines.push(["Data/Hora", "Usuário", "Entidade", "ID", "Ação", "Campo", "Valor Anterior", "Novo Valor"].map((h) => `"${h}"`).join(sep));

    filteredEntries.forEach((entry) => {
      const ts = formatTimestamp(entry.timestamp);
      const entityTypeLabel = ENTITY_TYPE_LABELS[entry.entityType]?.label || entry.entityType;
      const actionLabel = ACTION_CONFIG[entry.action]?.label || entry.action;

      entry.changes.forEach((ch) => {
        lines.push(
          [
            `${ts.date} ${ts.time}`,
            entry.user,
            entityTypeLabel,
            entry.entityId,
            actionLabel,
            ch.field,
            formatAuditValue(ch.field, ch.oldValue),
            formatAuditValue(ch.field, ch.newValue),
          ]
            .map((v) => `"${(v || "").replace(/"/g, '""')}"`)
            .join(sep),
        );
      });
    });

    const content = lines.join("\r\n");
    downloadCsv(`audit-trail-${entityId || "completo"}-${new Date().toISOString().split("T")[0]}.csv`, content);
  };

  const handleExportPDF = () => {
    const now = new Date();
    const dateStr = now.toLocaleDateString("pt-BR", { day: "2-digit", month: "long", year: "numeric" });
    const timeStr = now.toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" });

    const tableRows = filteredEntries
      .map((entry, i) => {
        const ts = formatTimestamp(entry.timestamp);
        const entityTypeLabel = ENTITY_TYPE_LABELS[entry.entityType]?.label || entry.entityType;
        const actionLabel = ACTION_CONFIG[entry.action]?.label || entry.action;
        const changesText = entry.changes
          .map((ch) => {
            const oldV = formatAuditValue(ch.field, ch.oldValue);
            const newV = formatAuditValue(ch.field, ch.newValue);
            if (oldV) return `${ch.field}: ${oldV} → ${newV}`;
            return `${ch.field}: ${newV}`;
          })
          .join("; ");

        return `<tr style="background:${i % 2 === 0 ? "#fff" : "#f8fafc"};">
<td style="padding:8px 12px;border-bottom:1px solid #e2e8f0;font-size:11px;white-space:nowrap;">${ts.date}<br/><span style="color:#94a3b8;">${ts.time}</span></td>
<td style="padding:8px 12px;border-bottom:1px solid #e2e8f0;font-size:11px;">${entry.user}</td>
<td style="padding:8px 12px;border-bottom:1px solid #e2e8f0;font-size:11px;">${entityTypeLabel}</td>
<td style="padding:8px 12px;border-bottom:1px solid #e2e8f0;font-size:11px;font-weight:600;">${entry.entityId}</td>
<td style="padding:8px 12px;border-bottom:1px solid #e2e8f0;font-size:11px;">${actionLabel}</td>
<td style="padding:8px 12px;border-bottom:1px solid #e2e8f0;font-size:11px;max-width:280px;word-break:break-word;">${changesText}</td>
</tr>`;
      })
      .join("");

    const titleText = entityId ? `Histórico - ${entityLabel || entityId}` : "Histórico de Alterações Completo";

    const filterText = [
      actionFilter !== "Todos" ? `Tipo: ${ACTION_CONFIG[actionFilter as AuditLogEntry["action"]]?.label}` : null,
      entityTypeFilter !== "Todos"
        ? `Entidade: ${ENTITY_TYPE_LABELS[entityTypeFilter as AuditLogEntry["entityType"]]?.label}`
        : null,
      searchTerm ? `Busca: "${searchTerm}"` : null,
    ]
      .filter(Boolean)
      .join(" | ");

    const html = `<!DOCTYPE html><html><head><meta charset="utf-8"/><title>${titleText}</title>
<style>@media print { body { -webkit-print-color-adjust: exact; print-color-adjust: exact; } @page { size: landscape; margin: 12mm; } }</style>
</head><body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;color:#0f172a;max-width:1100px;margin:0 auto;padding:32px;">
<div style="display:flex;justify-content:space-between;align-items:flex-start;margin-bottom:24px;">
<div>
<h1 style="font-size:22px;font-weight:700;margin:0;">${titleText}</h1>
<p style="font-size:12px;color:#64748b;margin:4px 0 0;">Gerado em ${dateStr} as ${timeStr} | ${filteredEntries.length} registro(s)${filterText ? ` | ${filterText}` : ""}</p>
</div>
<div style="text-align:right;">
<p style="font-size:14px;font-weight:700;color:#0f172a;margin:0;">suitee</p>
<p style="font-size:10px;color:#94a3b8;margin:2px 0 0;">Trilha de auditoria</p>
</div>
</div>
<table style="width:100%;border-collapse:collapse;border:1px solid #e2e8f0;border-radius:8px;overflow:hidden;">
<thead>
<tr style="background:#f1f5f9;">
<th style="padding:10px 12px;text-align:left;font-size:10px;text-transform:uppercase;letter-spacing:0.5px;color:#64748b;border-bottom:2px solid #e2e8f0;">Data/Hora</th>
<th style="padding:10px 12px;text-align:left;font-size:10px;text-transform:uppercase;letter-spacing:0.5px;color:#64748b;border-bottom:2px solid #e2e8f0;">Usuário</th>
<th style="padding:10px 12px;text-align:left;font-size:10px;text-transform:uppercase;letter-spacing:0.5px;color:#64748b;border-bottom:2px solid #e2e8f0;">Entidade</th>
<th style="padding:10px 12px;text-align:left;font-size:10px;text-transform:uppercase;letter-spacing:0.5px;color:#64748b;border-bottom:2px solid #e2e8f0;">ID</th>
<th style="padding:10px 12px;text-align:left;font-size:10px;text-transform:uppercase;letter-spacing:0.5px;color:#64748b;border-bottom:2px solid #e2e8f0;">Ação</th>
<th style="padding:10px 12px;text-align:left;font-size:10px;text-transform:uppercase;letter-spacing:0.5px;color:#64748b;border-bottom:2px solid #e2e8f0;">Alterações</th>
</tr>
</thead>
<tbody>${tableRows}</tbody>
</table>
<div style="margin-top:24px;text-align:center;font-size:10px;color:#94a3b8;">
Documento gerado automaticamente pelo suitee - Trilha de auditoria
</div>
</body></html>`;

    const printWindow = window.open("", "_blank");
    if (printWindow) {
      printWindow.document.write(html);
      printWindow.document.close();
      setTimeout(() => {
        printWindow.print();
      }, 500);
    }
  };

  return (
    <ExpandableSidePanel
      open={true}
      onClose={onClose}
      defaultSize="lg"
      expandedSize="full"
      renderHeader={({ expandButton, closeButton }) => (
        <div className="px-6 py-4 border-b border-border flex items-start justify-between flex-shrink-0">
          <div className="flex items-center gap-3">
            <div className="p-2.5 bg-muted rounded-xl">
              <History className="w-5 h-5 text-muted-foreground" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-foreground">Histórico de Alterações</h2>
              <p className="text-xs text-muted-foreground mt-0.5">
                {entityId ? `${entityLabel || entityId} - ${filteredEntries.length} registro(s)` : `${filteredEntries.length} registro(s) no sistema`}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            {expandButton}
            {closeButton}
          </div>
        </div>
      )}
    >
      <div className="px-6 py-3 border-b border-border bg-muted/50 flex items-center justify-between flex-shrink-0 flex-wrap gap-2">
        <div className="flex items-center gap-2 flex-wrap">
          <Filter className="w-3.5 h-3.5 text-muted-foreground" />

          {!entityId && !entityType && (
            <div className="relative">
              <button
                onClick={() => {
                  setEntityFilterOpen(!entityFilterOpen);
                  setFilterOpen(false);
                }}
                className="flex items-center gap-1.5 text-sm font-medium text-foreground px-3 py-1.5 bg-card border border-border rounded-lg hover:bg-muted transition-colors"
              >
                {entityTypeFilter === "Todos"
                  ? "Todas as entidades"
                  : ENTITY_TYPE_LABELS[entityTypeFilter as AuditLogEntry["entityType"]]?.label || entityTypeFilter}
                <ChevronDown className="w-3.5 h-3.5 text-muted-foreground" />
              </button>
              {entityFilterOpen && (
                <div className="absolute top-full left-0 mt-1 bg-card border border-border rounded-xl shadow-lg z-20 min-w-[180px] py-1">
                  {["Todos", "transaction", "contract", "document", "ticket", "deal", "property"].map((t) => (
                    <button
                      key={t}
                      onClick={() => {
                        setEntityTypeFilter(t);
                        setEntityFilterOpen(false);
                      }}
                      className={clsx(
                        "w-full text-left px-4 py-2 text-sm hover:bg-muted transition-colors",
                        entityTypeFilter === t ? "font-bold text-primary-600 bg-primary-50" : "text-foreground",
                      )}
                    >
                      {t === "Todos" ? "Todas as entidades" : ENTITY_TYPE_LABELS[t as AuditLogEntry["entityType"]]?.label || t}
                    </button>
                  ))}
                </div>
              )}
            </div>
          )}

          <div className="relative">
            <button
              onClick={() => {
                setFilterOpen(!filterOpen);
                setEntityFilterOpen(false);
              }}
              className="flex items-center gap-1.5 text-sm font-medium text-foreground px-3 py-1.5 bg-card border border-border rounded-lg hover:bg-muted transition-colors"
            >
              {actionFilter === "Todos" ? "Todos os tipos" : ACTION_CONFIG[actionFilter as AuditLogEntry["action"]]?.label || actionFilter}
              <ChevronDown className="w-3.5 h-3.5 text-muted-foreground" />
            </button>
            {filterOpen && (
              <div className="absolute top-full left-0 mt-1 bg-card border border-border rounded-xl shadow-lg z-20 min-w-[180px] py-1">
                {["Todos", "criacao", "edicao", "status_change", "exclusao"].map((a) => (
                  <button
                    key={a}
                    onClick={() => {
                      setActionFilter(a);
                      setFilterOpen(false);
                    }}
                    className={clsx(
                      "w-full text-left px-4 py-2 text-sm hover:bg-muted transition-colors",
                      actionFilter === a ? "font-bold text-primary-600 bg-primary-50" : "text-foreground",
                    )}
                  >
                    {a === "Todos" ? "Todos os tipos" : ACTION_CONFIG[a as AuditLogEntry["action"]]?.label || a}
                  </button>
                ))}
              </div>
            )}
          </div>

          <div className="relative">
            <button
              onClick={() => {
                setShowDateFilter(!showDateFilter);
                setFilterOpen(false);
                setEntityFilterOpen(false);
              }}
              className={clsx(
                "flex items-center gap-1.5 text-sm font-medium px-3 py-1.5 border rounded-lg transition-colors",
                dateFrom || dateTo ? "text-primary-700 bg-primary-50 border-primary-200" : "text-foreground bg-card border-border hover:bg-muted",
              )}
            >
              <CalendarRange className="w-3.5 h-3.5" />
              {dateFrom || dateTo ? "Periodo" : "Periodo"}
              {(dateFrom || dateTo) && <span className="w-1.5 h-1.5 bg-primary-500 rounded-full" />}
            </button>
            {showDateFilter && (
              <div className="absolute top-full left-0 mt-1 bg-card border border-border rounded-xl shadow-lg z-20 p-3 min-w-[260px]">
                <p className="text-caption font-bold text-muted-foreground uppercase tracking-wider mb-2">Filtrar por periodo</p>
                <div className="space-y-2">
                  <div>
                    <label className="text-xs text-muted-foreground mb-0.5 block">De</label>
                    <input
                      type="date"
                      value={dateFrom}
                      onChange={(e) => setDateFrom(e.target.value)}
                      className="w-full px-3 py-1.5 text-sm border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
                    />
                  </div>
                  <div>
                    <label className="text-xs text-muted-foreground mb-0.5 block">Ate</label>
                    <input
                      type="date"
                      value={dateTo}
                      onChange={(e) => setDateTo(e.target.value)}
                      className="w-full px-3 py-1.5 text-sm border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
                    />
                  </div>
                  <div className="flex items-center justify-between pt-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => {
                        setDateFrom("");
                        setDateTo("");
                      }}
                      className="h-auto px-2 py-1 text-xs text-muted-foreground"
                    >
                      Limpar
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setShowDateFilter(false)}
                      className="h-auto border-primary/30 bg-primary/10 px-3 py-1 text-xs font-bold text-primary hover:bg-primary/15 hover:text-primary"
                    >
                      Aplicar
                    </Button>
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>
        <span className="text-caption text-muted-foreground font-medium uppercase tracking-wider">Ordenado por data</span>
      </div>

      <div className="px-6 py-2.5 border-b border-border flex-shrink-0">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
          <input
            type="text"
            placeholder="Buscar por usuário, ID, campo ou valor..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="w-full pl-9 pr-8 py-2 bg-muted border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 transition-all placeholder:text-muted-foreground"
          />
          {searchTerm && (
            <Button
              type="button"
              variant="ghost"
              size="icon"
              onClick={() => setSearchTerm("")}
              className="absolute right-2 top-1/2 size-7 -translate-y-1/2 rounded-full text-muted-foreground"
            >
              <X className="w-3.5 h-3.5" />
            </Button>
          )}
        </div>
      </div>

      <div className="flex-1 overflow-y-auto px-6 py-4">
        {filteredEntries.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-48 text-center">
            <History className="w-10 h-10 text-muted-foreground/60 mb-3" />
            <p className="text-sm font-medium text-muted-foreground">Nenhum registro de alteração encontrado</p>
            <p className="text-xs text-muted-foreground mt-1">
              {searchTerm
                ? "Tente ajustar os termos da busca"
                : actionFilter !== "Todos" || entityTypeFilter !== "Todos" || dateFrom || dateTo
                  ? "Tente remover os filtros aplicados"
                  : "O histórico será preenchido automaticamente"}
            </p>
          </div>
        ) : (
          <div className="relative">
            <div className="absolute left-[19px] top-3 bottom-3 w-px bg-muted" />

            <div className="space-y-1">
              {filteredEntries.map((entry) => {
                const config = ACTION_CONFIG[entry.action];
                const Icon = config.icon;
                const ts = formatTimestamp(entry.timestamp);
                const isExpanded = expandedEntry === entry.id;

                return (
                  <div key={entry.id} className="relative pl-12">
                    <div
                      className={clsx(
                        "absolute left-[11px] top-4 w-[18px] h-[18px] rounded-full border-2 flex items-center justify-center z-10",
                        config.bg,
                        config.border,
                      )}
                    >
                      <div
                        className={clsx(
                          "w-2 h-2 rounded-full",
                          entry.action === "criacao"
                            ? "bg-status-ok"
                            : entry.action === "edicao"
                              ? "bg-status-info"
                              : entry.action === "status_change"
                                ? "bg-status-warning"
                                : "bg-status-critical",
                        )}
                      />
                    </div>

                    <button
                      onClick={() => toggleExpand(entry.id)}
                      className={clsx(
                        "w-full text-left p-3 rounded-xl border transition-all",
                        isExpanded ? "bg-card border-border shadow-sm" : "bg-transparent border-transparent hover:bg-muted hover:border-border",
                      )}
                    >
                      <div className="flex items-start justify-between gap-2">
                        <div className="flex items-center gap-2 flex-wrap">
                          <span className={clsx("inline-flex items-center gap-1 px-2 py-0.5 text-caption font-bold rounded-md border", config.bg, config.color, config.border)}>
                            <Icon className="w-3 h-3" />
                            {config.label}
                          </span>
                          {!entityId && (
                            <>
                              <span
                                className={clsx(
                                  "text-caption font-bold px-1.5 py-0.5 rounded border border-border",
                                  ENTITY_TYPE_LABELS[entry.entityType]?.bg || "bg-muted",
                                  ENTITY_TYPE_LABELS[entry.entityType]?.color || "text-muted-foreground",
                                )}
                              >
                                {ENTITY_TYPE_LABELS[entry.entityType]?.label || entry.entityType}
                              </span>
                              <span className="text-caption font-bold text-muted-foreground bg-muted px-1.5 py-0.5 rounded border border-border">
                                {entry.entityId}
                              </span>
                            </>
                          )}
                        </div>
                        <div className="flex items-center gap-2 flex-shrink-0">
                          <span className="text-caption text-muted-foreground">{ts.relative}</span>
                          {isExpanded ? (
                            <ChevronUp className="w-3.5 h-3.5 text-muted-foreground" />
                          ) : (
                            <ChevronDown className="w-3.5 h-3.5 text-muted-foreground" />
                          )}
                        </div>
                      </div>

                      <p className="text-xs text-muted-foreground mt-1.5">
                        {entry.changes.length === 1 ? `Campo "${entry.changes[0].field}" alterado` : `${entry.changes.length} campos alterados`}
                      </p>

                      {isExpanded && (
                        <motion.div initial={{ opacity: 0, height: 0 }} animate={{ opacity: 1, height: "auto" }} className="mt-3 space-y-3">
                          <div className="flex items-center gap-3 text-xs text-muted-foreground">
                            <div className="flex items-center gap-1.5">
                              <User className="w-3.5 h-3.5 text-muted-foreground" />
                              <span className="font-medium">{entry.user}</span>
                            </div>
                            <span className="text-muted-foreground/60">|</span>
                            <div className="flex items-center gap-1.5">
                              <Clock className="w-3.5 h-3.5 text-muted-foreground" />
                              <span>
                                {ts.date} as {ts.time}
                              </span>
                            </div>
                          </div>

                          <div className="space-y-2">
                            {entry.changes.map((ch, ci) => (
                              <div key={ci} className="bg-muted rounded-lg p-2.5 border border-border">
                                <p className="text-caption font-bold text-muted-foreground uppercase tracking-wider mb-1.5">{ch.field}</p>
                                <div className="flex items-center gap-2 text-xs">
                                  {ch.oldValue ? (
                                    <>
                                      <span className="px-2 py-0.5 bg-status-critical-soft text-status-critical rounded border border-status-critical/20 font-medium line-through">
                                        {maskAuditValue(ch.field, formatAuditValue(ch.field, ch.oldValue))}
                                      </span>
                                      <ArrowRight className="w-3 h-3 text-muted-foreground flex-shrink-0" />
                                      <span className="px-2 py-0.5 bg-status-ok-soft text-status-ok rounded border border-status-ok/20 font-medium">
                                        {maskAuditValue(ch.field, formatAuditValue(ch.field, ch.newValue))}
                                      </span>
                                    </>
                                  ) : (
                                    <span className="px-2 py-0.5 bg-status-ok-soft text-status-ok rounded border border-status-ok/20 font-medium">
                                      {maskAuditValue(ch.field, formatAuditValue(ch.field, ch.newValue))}
                                    </span>
                                  )}
                                </div>
                              </div>
                            ))}
                          </div>
                        </motion.div>
                      )}
                    </button>
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </div>

      <div className="px-6 py-3 border-t border-border bg-muted/80 flex items-center justify-between flex-shrink-0">
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => handleExportCSV()}
            disabled={filteredEntries.length === 0}
            className="text-xs font-bold text-muted-foreground"
            title="Exportar CSV"
          >
            <FileSpreadsheet className="w-3.5 h-3.5" /> CSV
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => handleExportPDF()}
            disabled={filteredEntries.length === 0}
            className="text-xs font-bold text-muted-foreground"
            title="Exportar PDF"
          >
            <FileTextIcon className="w-3.5 h-3.5" /> PDF
          </Button>
          <span className="text-caption text-muted-foreground ml-1">{filteredEntries.length} registro(s)</span>
        </div>
        <Button variant="outline" onClick={onClose}>
          Fechar
        </Button>
      </div>
    </ExpandableSidePanel>
  );
}
