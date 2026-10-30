// Helpers puros e constantes dos relatórios (extraídos de Reports.tsx —
// P1-04/T-07): janela de período, rótulos, cores e formatação. Sem React.
import { Award, BarChart3, Home, Users, Wallet } from "lucide-react";
import type { ReportFilterState } from "../../components/reports/ReportFilters";
import { chargeReceivableStatus } from "../../lib/reports/financeTotals";
import type { Charge } from "../../services/charge.service";

export type ReportTab = "overview" | "sales" | "leads" | "finance" | "performance";

export const TABS: { id: ReportTab; label: string; icon: typeof BarChart3 }[] = [
  { id: "overview", label: "Visão Geral", icon: BarChart3 },
  { id: "sales", label: "Vendas & Locações", icon: Home },
  { id: "leads", label: "Leads & Funil", icon: Users },
  { id: "finance", label: "Financeiro", icon: Wallet },
  { id: "performance", label: "Desempenho", icon: Award },
];

export const PERIODS = [
  { value: "7d", label: "7 dias" },
  { value: "30d", label: "30 dias" },
  { value: "90d", label: "90 dias" },
  { value: "12m", label: "12 meses" },
  { value: "ytd", label: "Ano atual" },
];

/**
 * Janela do relatório. O fuso é o do operador (fuso da empresa): o backend
 * resolve data-only no fuso da imobiliária, então enviamos datas simples.
 */
export function resolveReportWindow(filters: ReportFilterState, period: string): { from: Date; to: Date } {
  const now = new Date();
  const to = filters.dateTo
    ? new Date(`${filters.dateTo}T23:59:59.999`)
    : now;
  let from: Date;
  if (filters.dateFrom) {
    from = new Date(`${filters.dateFrom}T00:00:00`);
  } else {
    const days: Record<string, number> = { "7d": 7, "30d": 30, "90d": 90, "12m": 365, ytd: 365 };
    const span = days[period] ?? 365;
    const base = period === "ytd" ? new Date(now.getFullYear(), 0, 1) : new Date(to.getTime() - span * 86400000);
    from = base;
  }
  return { from, to };
}

/** Filtros que os agregados do servidor não sabem aplicar. */
export function hasFinanceDimensionFilters(filters: ReportFilterState): boolean {
  return filters.propertyTypes.length > 0 || filters.agentIds.length > 0;
}

export function dateOnly(value: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${value.getFullYear()}-${pad(value.getMonth() + 1)}-${pad(value.getDate())}`;
}

export const propertyTypeLabel = (raw: string): string => {
  const v = String(raw || "").trim();
  const k = v.toLowerCase();
  if (k === "apartment" || k === "apartamento") return "Apartamento";
  if (k === "house" || k === "casa") return "Casa";
  if (k === "land" || k === "terreno") return "Terreno";
  if (k === "commercial" || k === "comercial") return "Comercial";
  return v || "Apartamento";
};

export const COLORS = {
  primary: "#6366f1",
  emerald: "#10b981",
  amber: "#f59e0b",
  rose: "#f43f5e",
  sky: "#0ea5e9",
  violet: "#8b5cf6",
  slate: "#64748b",
  teal: "#14b8a6",
  orange: "#f97316",
  pink: "#ec4899",
};

export const PIE_COLORS = [COLORS.primary, COLORS.emerald, COLORS.amber, COLORS.rose, COLORS.sky, COLORS.violet, COLORS.teal, COLORS.orange];

export function currency(v: number): string {
  return v.toLocaleString("pt-BR", { style: "currency", currency: "BRL", minimumFractionDigits: 0 });
}

export function pct(v: number | null | undefined): string {
  if (v === null || v === undefined || !Number.isFinite(v)) return "N/D";
  return `${v.toFixed(1)}%`;
}

export const chargeStatusLabel = (charge: Charge): string => {
  const status = chargeReceivableStatus(charge);
  return status === "pago" ? "Pago" : status === "vencido" ? "Em Atraso" : "Pendente";
};
