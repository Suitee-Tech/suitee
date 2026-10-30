import { useState, useMemo, useCallback, useRef, useEffect } from "react";
import {
  BarChart3,
  Users,
  Building2,
  Wallet,
  FileText,
  Calendar,
  Download,
  ChevronDown,
  DollarSign,
  Target,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Home,
  Key,
  Percent,
  ArrowUpRight,
  ArrowDownRight,
  Briefcase,
  Star,
  Award,
  Activity,
  GitCompareArrows,
} from "lucide-react";
import {
  BarChart,
  Bar,
  LineChart,
  Line,
  AreaChart,
  Area,
  PieChart as RechartsPie,
  Pie,
  Cell,
  RadarChart,
  Radar,
  PolarGrid,
  PolarAngleAxis,
  PolarRadiusAxis,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
} from "recharts";
import { clsx } from "clsx";
import { motion, AnimatePresence } from "motion/react";
import { toast } from "sonner";
import { useFocusMode } from "../context/FocusModeContext";
import { buildMonthlySalesData } from "../reports/monthlySales";
import { usePropertiesContext } from "../context/PropertiesContext";
import { useContractsContext } from "../context/ContractsContext";
import { useLeadsContext } from "../context/LeadsContext";
import { useAgentsContext } from "../context/AgentsContext";
import { useFinanceContext } from "../context/FinanceContext";
import { useMaintenanceContext } from "../context/MaintenanceContext";
import { useCalendarContext } from "../context/CalendarContext";
import { useEsteiraDeals } from "../hooks/useEsteiraDeals";
import { ContractStatus } from "../contracts/types";
import { formatPropertyAddress, resolvePropertyDisplay } from "../data";
import { propertyStatusFromBackend } from "../lib/properties/mappers";
import { ReportKPICard } from "../components/reports/ReportKPICard";
import { ChartCard } from "../components/reports/ChartCard";
import { ExportReportModal, type ExportConfig } from "../components/ExportReportModal";
import { ReportFilters, EMPTY_FILTERS, hasActiveFilters, type ReportFilterState } from "../components/reports/ReportFilters";
import { DrillDownPanel, type DrillDownData } from "../components/reports/DrillDownPanel";
import { ComparisonIndicator } from "../components/reports/ComparisonIndicator";
import { MediaImage } from "../components/MediaImage";
import { Button } from "../ui/button";
import { PageHeader } from "../ui/page-header";
import { financialReportService } from "../services/financialReport.service";
import { Can } from "../permissions/Can";
import { useFinancialRead } from "../hooks/useFinancialRead";
import { applyFinanceFilters } from "../lib/reports/financeFilters";
import {
  chargeEconomicDate,
  chargeReceivableStatus,
  commissionEconomicDate,
  commissionPaidValue,
  computeFinanceTotals,
  computeFinanceMonthly,
  monthLabelOf,
  transferEconomicDate,
  type FinancePeriodTotals,
  type FinanceMonthlyBucket,
} from "../lib/reports/financeTotals";
import { listAllCommissions } from "../services/financialLists";
import {
  COLORS,
  PIE_COLORS,
  PERIODS,
  TABS,
  chargeStatusLabel,
  currency,
  dateOnly,
  hasFinanceDimensionFilters,
  pct,
  propertyTypeLabel,
  resolveReportWindow,
  type ReportTab,
} from "./Reports/helpers";

/**
 * Donut em SVG próprio.
 *
 * Duas pizzas da aba Vendas & Locações não geravam geometria com o Recharts —
 * mesmo dado e mesmo componente renderizam em outras abas, e não foi a medição,
 * a animação, os Cells nem o conteúdo dos dados (tudo verificado no navegador).
 * Em vez de continuar contornando a biblioteca, estes dois cards usam um donut
 * próprio: o desenho é determinístico e não depende de estado interno dela.
 */
function Donut({
  data,
  colors,
  size = 190,
  thickness = 34,
  centerLabel,
  centerValue,
  onSliceClick,
}: {
  data: { name: string; value: number }[];
  colors: string[];
  size?: number;
  thickness?: number;
  centerLabel?: string;
  centerValue?: string;
  onSliceClick?: (name: string) => void;
}) {
  const total = data.reduce((sum, item) => sum + (Number(item.value) || 0), 0);
  const radius = (size - thickness) / 2;
  const circumference = 2 * Math.PI * radius;
  const center = size / 2;

  if (total <= 0) {
    return (
      <div className="flex items-center justify-center text-sm text-muted-foreground" style={{ height: size }}>
        Sem dados no período
      </div>
    );
  }

  let offset = 0;
  return (
    <div className="flex flex-wrap items-center justify-center gap-6">
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} role="img" aria-label="Distribuição">
        <g transform={`rotate(-90 ${center} ${center})`}>
          {data.map((item, index) => {
            const value = Number(item.value) || 0;
            const fraction = value / total;
            const dash = fraction * circumference;
            const slice = (
              <circle
                key={item.name}
                cx={center}
                cy={center}
                r={radius}
                fill="none"
                stroke={colors[index % colors.length]}
                strokeWidth={thickness}
                strokeDasharray={`${dash} ${circumference - dash}`}
                strokeDashoffset={-offset}
                className={onSliceClick ? "cursor-pointer" : undefined}
                onClick={onSliceClick ? () => onSliceClick(item.name) : undefined}
              />
            );
            offset += dash;
            return slice;
          })}
        </g>
        {(centerValue || centerLabel) && (
          <>
            <text x={center} y={center} textAnchor="middle" dominantBaseline="middle" className="fill-foreground text-lg font-bold">
              {centerValue ?? ""}
            </text>
            <text x={center} y={center + 18} textAnchor="middle" dominantBaseline="middle" className="fill-muted-foreground text-caption">
              {centerLabel ?? ""}
            </text>
          </>
        )}
      </svg>
      <ul className="space-y-1.5">
        {data.map((item, index) => (
          <li key={item.name} className="flex items-center gap-2 text-xs">
            <span className="inline-block h-3 w-3 shrink-0 rounded-sm" style={{ backgroundColor: colors[index % colors.length] }} />
            <span className="text-muted-foreground">{item.name}</span>
            <span className="font-bold text-foreground">
              {total > 0 ? `${Math.round(((Number(item.value) || 0) / total) * 100)}%` : "—"}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function RC({ height, children }: { height: number; children: React.ReactElement<Record<string, unknown>> }) {
  const ref = useRef<HTMLDivElement>(null);
  const [dims, setDims] = useState<{ w: number; h: number } | null>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;

    const measure = () => {
      const { width: w, height: h } = el.getBoundingClientRect();
      if (w > 0 && h > 0) {
        setDims((prev) => (prev && prev.w === Math.round(w) && prev.h === Math.round(h) ? prev : { w: Math.round(w), h: Math.round(h) }));
      }
    };

    const raf = requestAnimationFrame(measure);
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => {
      cancelAnimationFrame(raf);
      ro.disconnect();
    };
  }, []);

  const chart =
    dims && dims.w > 0 && dims.h > 0
      ? (() => {
        const { type: Chart, props } = children;
        return <Chart {...props} width={dims.w} height={dims.h} />;
      })()
      : null;

  return (
    <div ref={ref} style={{ width: "100%", height, minHeight: 1, minWidth: 1, position: "relative" }}>
      {chart}
    </div>
  );
}

/**
 * Percentual formatado. `null` significa "sem base de cálculo" (por exemplo,
 * corretor sem leads): mostrar 0% afirmaria "nenhuma conversão" e 100% afirmaria
 * "converteu tudo", ambas falsas. N/D é a resposta honesta.
 */
export function Reports() {
  const [activeTab, setActiveTab] = useState<ReportTab>("overview");
  const [period, setPeriod] = useState("12m");
  const [filters, setFilters] = useState<ReportFilterState>(EMPTY_FILTERS);
  const reportWindow = useMemo(() => resolveReportWindow(filters, period), [filters, period]);
  const prevWindow = useMemo(() => {
    const duration = reportWindow.to.getTime() - reportWindow.from.getTime();
    const prevTo = new Date(reportWindow.from.getTime() - 1);
    return { from: new Date(prevTo.getTime() - duration), to: prevTo };
  }, [reportWindow]);

  // Totais autoritativos vêm do servidor (DRE usa a fronteira econômica);
  // a granularidade local usa as mesmas listas que a Conferência.
  const rangeKey = `${dateOnly(reportWindow.from)}/${dateOnly(reportWindow.to)}`;
  // Só busca o agregado do servidor quando o filtro ativo é o que ele entende
  // (período). Com imóvel ou corretor selecionado, o agregado do servidor
  // ignoraria o filtro e o cartão divergiria dos gráficos.
  const serverAggregateApplies = !hasFinanceDimensionFilters(filters);
  const periodRead = useFinancialRead(
    `reports.finance.${rangeKey}`,
    useCallback(
      () =>
        Promise.all([
          financialReportService.dre({ from: dateOnly(reportWindow.from), to: dateOnly(reportWindow.to) }),
          financialReportService.cashflow({ from: dateOnly(reportWindow.from), to: dateOnly(reportWindow.to) }, "month"),
        ]).then(([dre, cashflow]) => ({ dre, cashflow })),
      [rangeKey],
    ),
    serverAggregateApplies,
  );
  const dre = serverAggregateApplies ? periodRead.data?.dre : undefined;
  const cashflow = periodRead.data?.cashflow;

  // Janela anterior, para a comparação "vs. período anterior" usar a MESMA
  // fronteira econômica do servidor, não uma aproximação local.
  const prevRangeKey = `${dateOnly(prevWindow.from)}/${dateOnly(prevWindow.to)}`;
  const prevPeriodRead = useFinancialRead(
    `reports.finance.prev.${prevRangeKey}`,
    useCallback(
      () => financialReportService.dre({ from: dateOnly(prevWindow.from), to: dateOnly(prevWindow.to) }),
      [prevRangeKey],
    ),
    serverAggregateApplies,
  );
  const prevDre = serverAggregateApplies ? prevPeriodRead.data : undefined;
  const [showPeriod, setShowPeriod] = useState(false);
  const [exportConfig, setExportConfig] = useState<ExportConfig | null>(null);
  const [drillDown, setDrillDown] = useState<DrillDownData | null>(null);
  const [showComparison, setShowComparison] = useState(false);

  const { properties } = usePropertiesContext();
  const { contracts } = useContractsContext();
  const { leads } = useLeadsContext();
  const { agents } = useAgentsContext();
  const { transactions, charges, transfers } = useFinanceContext();
  const { deals } = useEsteiraDeals();
  const commissionsRead = useFinancialRead(
    "reports.commissions",
    () => listAllCommissions(),
    activeTab === "finance" || activeTab === "sales" || activeTab === "performance",
  );
  const commissions = commissionsRead.data ?? [];
  const { tickets } = useMaintenanceContext();
  const { appointments } = useCalendarContext();

  const filteredData = useMemo(() => {
    const active = hasActiveFilters(filters);
    let fp = properties;
    let fc = contracts;
    let fl = leads;
    let ft = transactions;
    let fd = deals;

    if (active) {
      if (filters.propertyTypes.length > 0) {
        fp = fp.filter((p) => filters.propertyTypes.includes(p.type));
        const filteredPropIds = new Set(fp.map((p) => p.id));
        fc = fc.filter((c) => filteredPropIds.has(c.property_id || ""));
      }

      if (filters.agentIds.length > 0) {
        const agentSet = new Set(filters.agentIds);
        fl = fl.filter((l) => l.agentId && agentSet.has(l.agentId));
        const agentIndices = agents.map((a, i) => (agentSet.has(a.id) ? i : -1)).filter((i) => i >= 0);
        fd = fd.filter((_, i) => agentIndices.includes(i % agents.length));
      }

      if (filters.dateFrom) {
        const from = new Date(filters.dateFrom);
        fl = fl.filter((l) => new Date(l.date) >= from);
        ft = ft.filter((t) => new Date(t.date) >= from);
        fc = fc.filter((c) => new Date(c.start_date) >= from);
      }
      if (filters.dateTo) {
        const to = new Date(filters.dateTo);
        to.setHours(23, 59, 59, 999);
        fl = fl.filter((l) => new Date(l.date) <= to);
        ft = ft.filter((t) => new Date(t.date) <= to);
        fc = fc.filter((c) => new Date(c.start_date) <= to);
      }
    }

    return { properties: fp, contracts: fc, leads: fl, transactions: ft, deals: fd };
  }, [properties, contracts, leads, transactions, deals, agents, filters]);

  const availablePropertyTypes = useMemo(() => [...new Set(properties.map((p) => propertyTypeLabel(p.type)))].sort(), [properties]);

  // ── Finanças: fonte econômica correta ─────────────────────────────────────
  // Receita é a PARCELA PRÓPRIA da imobiliária (taxa/intermediação), não o valor
  // arrecadado de terceiros. Os endpoints do servidor aceitam só período, então
  // imóvel e corretor são aplicados aqui — sem isso o cartão exibiria o total da
  // carteira como se fosse do filtro selecionado.
  const financeLists = useMemo(
    () => applyFinanceFilters(charges, transfers, commissions, properties, contracts, filters),
    [charges, transfers, commissions, properties, contracts, filters],
  );

  const financeCurrent = useMemo<FinancePeriodTotals>(
    () => computeFinanceTotals(financeLists.charges, financeLists.transfers, financeLists.commissions, reportWindow),
    [financeLists, reportWindow],
  );
  const financePrevious = useMemo<FinancePeriodTotals>(
    () => computeFinanceTotals(financeLists.charges, financeLists.transfers, financeLists.commissions, prevWindow),
    [financeLists, prevWindow],
  );
  const financeMonthly = useMemo<FinanceMonthlyBucket[]>(
    () => computeFinanceMonthly(financeLists.charges, financeLists.commissions, reportWindow),
    [financeLists, reportWindow],
  );

  const computeAnalytics = useCallback(
    (
      props: typeof properties,
      contr: typeof contracts,
      lds: typeof leads,
      txns: typeof transactions,
      dls: typeof deals,
      kms: typeof commissions,
      finance: FinancePeriodTotals,
    ) => {
      const totalProperties = props.length;
      // O status chega CRU da API ("rented"/"sold") e precisa do rótulo do
      // domínio ("Alugado"/"Vendido"): sem isso o gráfico mostra a chave interna,
      // que não é o que o operador lê no resto do sistema.
      const propertiesByStatus = props.reduce<Record<string, number>>((acc, p) => {
        const raw = String(p.status ?? "").trim();
        const label = propertyStatusFromBackend[raw] ?? raw ?? "";
        const key = label || "Sem status";
        acc[key] = (acc[key] || 0) + 1;
        return acc;
      }, {});
      const propertiesByType = props.reduce<Record<string, number>>((acc, p) => {
        // `propertyTypeLabel` devolve string vazia quando o tipo não vem; um
        // balde com chave vazia não é legível e vira um setor sem nome.
        const key = propertyTypeLabel(p.type) || "Não informado";
        acc[key] = (acc[key] || 0) + 1;
        return acc;
      }, {});
      const portfolioValue = props.reduce((s, p) => s + p.price, 0);
      const avgPropertyPrice = totalProperties > 0 ? portfolioValue / totalProperties : 0;

      const activeContracts = contr.filter((c) => c.status === ContractStatus.ACTIVE);
      const salesContracts = contr.filter((c) => c.type === "venda");
      const rentalContracts = contr.filter((c) => c.type === "locacao");
      const totalSalesValue = salesContracts.reduce((s, c) => s + (c.value || 0), 0);
      const totalRentalValue = rentalContracts.reduce((s, c) => s + (c.value || 0), 0);
      // Comissão gerada vem do registro de COMISSÕES — a entidade que carrega o
      // valor. As duas tentativas anteriores liam campos que chegam nulos
      // (`contract.commission_pct`) ou vazios (`deal.commission`), e o cartão
      // exibia R$ 0 com comissões registradas no sistema.
      const totalCommissions = kms.reduce((sum, c) => sum + commissionPaidValue(c), 0);
      const commissionCount = kms.length;

      const totalLeads = lds.length;
      const leadsByStatus = lds.reduce<Record<string, number>>((acc, l) => {
        acc[l.status] = (acc[l.status] || 0) + 1;
        return acc;
      }, {});
      const convertedLeads = leadsByStatus["Convertido"] || 0;
      const lostLeads = leadsByStatus["Perdido"] || 0;
      const conversionRate = totalLeads > 0 ? (convertedLeads / totalLeads) * 100 : 0;

      // Fronteira econômica (não arrecadação): os valores vêm dos totais
      // financeiros computados sobre charges/transfers/comissões — recursos de
      // proprietários e corretores não são receita da imobiliária.
      const totalRevenue = finance.revenue;
      const totalExpenses = finance.commissionExpense;
      const totalRepasses = finance.transfers;
      const paidRevenue = finance.received;
      const pendingRevenue = finance.pending;
      const overdueRevenue = finance.overdue;

      const totalDeals = dls.length;
      const pipelineValue = dls.reduce((s, d) => s + d.value, 0);
      const dealsByStage = dls.reduce<Record<string, { count: number; value: number }>>((acc, d) => {
        if (!acc[d.stage]) acc[d.stage] = { count: 0, value: 0 };
        acc[d.stage].count++;
        acc[d.stage].value += d.value;
        return acc;
      }, {});

      const openTickets = tickets.filter((t) => t.status !== "Concluído").length;
      const highPriorityTickets = tickets.filter((t) => t.priority === "Alta").length;

      const totalAppointments = appointments.length;
      const completedAppointments = appointments.filter((a) => a.status === "Concluido").length;
      const cancelledAppointments = appointments.filter((a) => a.status === "Cancelado").length;

      // Vínculo REAL, não rodízio. A distribuição por índice (i % agentes) dava a
      // todo corretor a mesma fatia da carteira: todos apareciam com 1 lead e
      // 100% de conversão, inclusive quem não tinha lead nenhum. Negócio tem
      // broker_id e lead tem agentId — usar os dois é o que torna a métrica
      // verificável.
      const agentPerformance = agents.map((agent) => {
        const agentDeals = dls.filter((d) => d.brokerId === agent.id);
        const agentLeads = lds.filter((l) => l.agentId === agent.id);
        const convertedLeads = agentLeads.filter((l) => l.status === "Convertido").length;
        const agentAppointments = appointments.filter((a) => a.agentId === agent.id);
        return {
          id: agent.id,
          name: agent.name,
          role: agent.role,
          avatar: agent.avatar,
          deals: agentDeals.length,
          dealsValue: agentDeals.reduce((s, d) => s + d.value, 0),
          // Comissão dos negócios do corretor: campo próprio do negócio, que o
          // contrato não carrega (commission_pct vem nulo).
          dealsCommission: agentDeals.reduce((s, d) => s + (d.commission || 0), 0),
          leads: agentLeads.length,
          convertedLeads,
          appointments: agentAppointments.length,
          completedAppointments: agentAppointments.filter((a) => a.status === "Concluido").length,
          // Sem lead não há taxa: 0% afirmaria "nenhuma conversão" e 100%
          // afirmaria "converteu tudo". `null` é a resposta honesta e a interface
          // mostra N/D.
          conversionRate: agentLeads.length > 0 ? (convertedLeads / agentLeads.length) * 100 : null,
          avgResponseTime: null as number | null,
          satisfaction: null as string | null,
        };
      });

      return {
        totalProperties,
        propertiesByStatus,
        propertiesByType,
        portfolioValue,
        avgPropertyPrice,
        activeContracts: activeContracts.length,
        salesContracts: salesContracts.length,
        rentalContracts: rentalContracts.length,
        totalSalesValue,
        totalRentalValue,
        totalCommissions,
        commissionCount,
        totalLeads,
        leadsByStatus,
        convertedLeads,
        lostLeads,
        conversionRate,
        totalRevenue,
        totalExpenses,
        totalRepasses,
        paidRevenue,
        pendingRevenue,
        overdueRevenue,
        totalDeals,
        pipelineValue,
        dealsByStage,
        openTickets,
        highPriorityTickets,
        totalAppointments,
        completedAppointments,
        cancelledAppointments,
        agentPerformance,
      };
    },
    [agents, tickets, appointments],
  );

  const analytics = useMemo(
    () => computeAnalytics(filteredData.properties, filteredData.contracts, filteredData.leads, filteredData.transactions, filteredData.deals, commissions, financeCurrent),
    [filteredData, computeAnalytics, commissions, financeCurrent],
  );

  const prevAnalytics = useMemo(() => {
    const now = new Date();
    const toDate: Date = filters.dateTo ? new Date(filters.dateTo) : now;
    let fromDate: Date;

    if (filters.dateFrom) {
      fromDate = new Date(filters.dateFrom);
    } else {
      const periodDays: Record<string, number> = {
        "7d": 7,
        "30d": 30,
        "90d": 90,
        "12m": 365,
        ytd: Math.floor((now.getTime() - new Date(now.getFullYear(), 0, 1).getTime()) / 86400000),
      };
      const daysToSubtract = periodDays[period] || 365;
      fromDate = new Date(toDate.getTime() - daysToSubtract * 86400000);
    }

    const durationMs = toDate.getTime() - fromDate.getTime();
    const prevTo = new Date(fromDate.getTime() - 1);
    const prevFrom = new Date(prevTo.getTime() - durationMs);

    const prevProps = properties.filter(() => true);
    const prevContracts = contracts.filter((c) => {
      const d = new Date(c.start_date);
      return d >= prevFrom && d <= prevTo;
    });
    const prevLeads = leads.filter((l) => {
      const d = new Date(l.date);
      return d >= prevFrom && d <= prevTo;
    });
    const prevTxns = transactions.filter((t) => {
      const d = new Date(t.date);
      return d >= prevFrom && d <= prevTo;
    });
    const prevDeals = deals.filter((d) => {
      const dt = new Date(d.date);
      return dt >= prevFrom && dt <= prevTo;
    });

    const prev = computeAnalytics(prevProps, prevContracts, prevLeads, prevTxns, prevDeals, commissions, financePrevious);
    return {
      portfolioValue: prev.portfolioValue,
      totalRevenue: prev.totalRevenue,
      totalExpenses: prev.totalExpenses,
      totalLeads: prev.totalLeads,
      conversionRate: prev.conversionRate,
      pipelineValue: prev.pipelineValue,
      totalSalesValue: prev.totalSalesValue,
      totalRentalValue: prev.totalRentalValue,
      totalCommissions: prev.totalCommissions,
      commissionCount: prev.commissionCount,
      paidRevenue: prev.paidRevenue,
      pendingRevenue: prev.pendingRevenue,
      overdueRevenue: prev.overdueRevenue,
      totalRepasses: prev.totalRepasses,
      convertedLeads: prev.convertedLeads,
      lostLeads: prev.lostLeads,
      totalDeals: prev.totalDeals,
    };
  }, [properties, contracts, leads, transactions, deals, filters, period, computeAnalytics, financePrevious]);

  // Série mensal da fronteira econômica (receita própria × comissões pagas),
  // com o mês no rótulo brasileiro que os gráficos esperam.
  const monthlyRevenueData = useMemo(
    () =>
      financeMonthly.map((bucket) => ({
        month: monthLabelOf(bucket.month),
        receita: bucket.revenue,
        despesa: bucket.expense,
        lucro: bucket.revenue - bucket.expense,
      })),
    [financeMonthly],
  );

  const leadFunnelData = useMemo(() => {
    const stages = [
      { name: "Novo", value: analytics.leadsByStatus["Novo"] || 0, fill: COLORS.primary },
      { name: "Em Atendimento", value: analytics.leadsByStatus["Em Atendimento"] || 0, fill: COLORS.sky },
      { name: "Qualificado", value: analytics.leadsByStatus["Qualificado"] || 0, fill: COLORS.teal },
      { name: "Visita Agendada", value: analytics.leadsByStatus["Visita Agendada"] || 0, fill: COLORS.emerald },
      { name: "Em Negociacao", value: analytics.leadsByStatus["Em Negociação"] || 0, fill: COLORS.amber },
      { name: "Proposta Enviada", value: analytics.leadsByStatus["Proposta Enviada"] || 0, fill: COLORS.orange },
      { name: "Convertido", value: analytics.leadsByStatus["Convertido"] || 0, fill: COLORS.violet },
    ];
    return stages.filter((s) => s.value > 0);
  }, [analytics.leadsByStatus]);

  const dealsPipelineData = useMemo(() => {
    const stages: Array<{ stage: string; qtd: number; valor: number }> = [];
    const order = ["Em Negociação", "Proposta", "Análise", "Contrato", "Concluído"];
    order.forEach((stage) => {
      const d = analytics.dealsByStage[stage];
      stages.push({ stage, qtd: d?.count || 0, valor: d?.value || 0 });
    });
    return stages;
  }, [analytics.dealsByStage]);

  // Valor numérico garantido e entradas vazias fora: o Recharts desenha um setor
  // sem geometria quando o valor não é um número válido, e o gráfico fica em
  // branco com a legenda presente.
  const toPieData = (buckets: Record<string, number>) =>
    Object.entries(buckets)
      .map(([name, value]) => ({ name: name || "Não informado", value: Number(value) || 0 }))
      .filter((item) => item.value > 0);

  const propertyTypeData = useMemo(() => toPieData(analytics.propertiesByType), [analytics.propertiesByType]);

  const propertyStatusData = useMemo(() => toPieData(analytics.propertiesByStatus), [analytics.propertiesByStatus]);

  // O gráfico é da CARTEIRA DE COBRANÇAS: valores brutos arrecadados, a
  // receber e vencidos — não a fronteira econômica da agência.
  const revenueByStatusData = useMemo(
    () => [
      { name: "Pago", value: financeCurrent.grossReceived, fill: COLORS.emerald },
      { name: "Pendente", value: financeCurrent.pending, fill: COLORS.amber },
      { name: "Em Atraso", value: financeCurrent.overdue, fill: COLORS.rose },
    ],
    [financeCurrent],
  );

  const agentRadarData = useMemo(
    () =>
      analytics.agentPerformance.map((a) => ({
        name: a.name.split(" ")[0],
        negocios: a.deals * 20,
        leads: a.leads * 15,
        conversao: a.conversionRate,
        agendamentos: a.appointments * 10,
        satisfacao: 0,
      })),
    [analytics.agentPerformance],
  );

  // Agrupamento por mês dos negócios da esteira. Fica fora do componente (e
  // testado) porque o `date` do negócio é a data EXIBIDA em pt-BR
  // ("30/09/2026"): `new Date(deal.date)` devolve Invalid Date e o mês vira NaN,
  // o que zerava os gráficos de Vendas & Locações.
  const monthlySalesData = useMemo(() => buildMonthlySalesData(filteredData.deals), [filteredData.deals]);

  const leadSourceData = useMemo(() => {
    const grouped: Record<string, number> = {};
    for (const lead of filteredData.leads) {
      const source = lead.source || "Outros";
      grouped[source] = (grouped[source] || 0) + 1;
    }
    return Object.entries(grouped)
      .map(([name, value]) => ({ name, value }))
      .sort((a, b) => b.value - a.value);
  }, [filteredData.leads]);

  // Fluxo de caixa vem do servidor (entradas e saídas reais do journal), não
  // de lançamentos de tipo: a fronteira de caixa é a mesma da Conferência.
  const cashFlowData = useMemo(
    () =>
      (cashflow?.entries ?? []).map((entry) => ({
        month: monthLabelOf(entry.period),
        entradas: entry.inflow_cents / 100,
        saidas: entry.outflow_cents / 100,
        saldo: entry.net_cents / 100,
      })),
    [cashflow],
  );

  const handleDrillDownLeadStatus = useCallback(
    (status: string) => {
      const matched = filteredData.leads.filter((l) => l.status === status);
      setDrillDown({
        title: `Leads: ${status}`,
        subtitle: `${matched.length} lead${matched.length !== 1 ? "s" : ""} neste status`,
        summary: [
          { label: "Quantidade", value: String(matched.length) },
          { label: "Orçamento medio", value: currency(matched.length > 0 ? matched.reduce((s, l) => s + l.budget, 0) / matched.length : 0) },
        ],
        records: matched.map((l) => ({
          id: l.id,
          label: l.name,
          sublabel: resolvePropertyDisplay(l.property, properties),
          value: currency(l.budget),
          status: l.status,
          statusColor: "bg-primary-50 text-primary-700 border-primary-200",
        })),
      });
    },
    [filteredData.leads, properties],
  );

  const handleDrillDownDealStage = useCallback(
    (stage: string) => {
      const matched = filteredData.deals.filter((d) => d.stage === stage);
      setDrillDown({
        title: `Negócios: ${stage}`,
        subtitle: `${matched.length} negocio${matched.length !== 1 ? "s" : ""} nesta etapa`,
        summary: [
          { label: "Quantidade", value: String(matched.length) },
          { label: "Valor Total", value: currency(matched.reduce((s, d) => s + d.value, 0)) },
        ],
        records: matched.map((d) => ({
          id: d.id,
          label: d.client,
          sublabel: `${d.property} - ${d.type}`,
          value: currency(d.value),
          status: d.stage,
          statusColor: "bg-violet-50 text-violet-700 border-violet-200",
        })),
      });
    },
    [filteredData.deals],
  );

  const handleDrillDownPropertyType = useCallback(
    (type: string) => {
      const matched = filteredData.properties.filter((p) => p.type === type);
      setDrillDown({
        title: `Imóveis: ${type}`,
        subtitle: `${matched.length} imóvel(is)`,
        summary: [
          { label: "Quantidade", value: String(matched.length) },
          { label: "Valor Total", value: currency(matched.reduce((s, p) => s + p.price, 0)) },
        ],
        records: matched.map((p) => ({
          id: p.id,
          label: p.title,
          sublabel: formatPropertyAddress(p.address),
          value: currency(p.price),
          status: p.status,
          statusColor: "bg-status-ok-soft text-status-ok border-status-ok/30",
        })),
      });
    },
    [filteredData.properties],
  );

  const handleDrillDownRevenueStatus = useCallback(
    (status: string) => {
      // Drill-down sobre a MESMA carteira do gráfico (já filtrada por imóvel e
      // corretor): abrir um painel com cobranças fora do filtro contradiria o
      // número clicado.
      const matched = financeLists.charges.filter((c) => chargeStatusLabel(c) === status);
      setDrillDown({
        title: `Cobranças: ${status}`,
        subtitle: `${matched.length} cobrança(s)`,
        summary: [
          { label: "Quantidade", value: String(matched.length) },
          { label: "Valor Total", value: currency(matched.reduce((sum, c) => sum + (c.total_amount || c.amount || 0), 0)) },
        ],
        records: matched.map((c) => ({
          id: c.id,
          label: c.description || "Cobrança",
          sublabel: `${chargeEconomicDate(c)} - Contrato ${c.contract_id}`,
          value: currency(c.total_amount || c.amount || 0),
          status: chargeStatusLabel(c),
          statusColor:
            chargeStatusLabel(c) === "Pago"
              ? "bg-status-ok-soft text-status-ok border-status-ok/30"
              : chargeStatusLabel(c) === "Em Atraso"
                ? "bg-status-critical-soft text-status-critical border-status-critical/30"
                : "bg-status-warning-soft text-status-warning border-status-warning/30",
        })),
      });
    },
    [financeLists],
  );

  const handleExportOverview = useCallback(() => {
    setExportConfig({
      title: "Relatório Geral - CRM Imobiliário",
      subtitle: `Período: ${PERIODS.find((p) => p.value === period)?.label}`,
      columns: [
        { key: "metrica", label: "Métrica" },
        { key: "valor", label: "Valor" },
      ],
      data: [
        { metrica: "Total de Imóveis", valor: analytics.totalProperties },
        { metrica: "Valor do Portfólio", valor: currency(analytics.portfolioValue) },
        { metrica: "Contratos Ativos", valor: analytics.activeContracts },
        { metrica: "Total de Leads", valor: analytics.totalLeads },
        { metrica: "Taxa de Conversão", valor: pct(analytics.conversionRate) },
        { metrica: "Receita Total", valor: currency(analytics.totalRevenue) },
        { metrica: "Despesas Totais", valor: currency(analytics.totalExpenses) },
        { metrica: "Negócios no Pipeline", valor: analytics.totalDeals },
        { metrica: "Valor do Pipeline", valor: currency(analytics.pipelineValue) },
        { metrica: "Comissões Geradas", valor: currency(analytics.totalCommissions) },
      ],
    });
  }, [analytics, period]);

  const handleExportLeads = useCallback(() => {
    setExportConfig({
      title: "Relatório de Leads",
      subtitle: "Distribuição por status e fonte",
      columns: [
        { key: "id", label: "ID" },
        { key: "name", label: "Nome" },
        { key: "property", label: "Interesse" },
        { key: "budget", label: "Orçamento", format: (v: number) => currency(v) },
        { key: "status", label: "Status" },
      ],
      data: leads.map((l) => ({ id: l.id, name: l.name, property: l.property, budget: l.budget, status: l.status })),
    });
  }, [leads]);

  const handleExportFinance = useCallback(() => {
    setExportConfig({
      title: "Relatório Financeiro",
      subtitle: "Transações consolidadas",
      columns: [
        { key: "id", label: "ID" },
        { key: "description", label: "Descrição" },
        { key: "type", label: "Tipo" },
        { key: "value", label: "Valor", format: (v: number) => currency(v) },
        { key: "status", label: "Status" },
        { key: "date", label: "Data" },
      ],
      // Exportação das três fontes econômicas: cobranças (carteira), repasses
      // (proprietários) e comissões pagas. Lançamentos do ledger ficam fora:
      // são espelho contábil, não fato econômico primário.
      data: [
        ...charges.map((c) => ({
          id: c.id,
          description: c.description || "Cobrança",
          type: c.type === "RENT" ? "Locação" : c.type === "SALE" ? "Venda" : "Cobrança",
          value: c.total_amount || c.amount || 0,
          status: chargeStatusLabel(c),
          date: chargeEconomicDate(c),
        })),
        ...transfers
          .filter((t) => String(t.status).toUpperCase() === "PROCESSED")
          .map((t) => ({
            id: t.id,
            description: `Repasse ao proprietário`,
            type: "Repasse",
            value: t.net_amount || 0,
            status: "Processado",
            date: transferEconomicDate(t),
          })),
        ...commissions
          .filter((c) => String(c.status).toUpperCase() === "PAID")
          .map((c) => ({
            id: c.id,
            description: `Comissão ${c.role ? `- ${c.role}` : ""}`,
            type: "Comissão",
            value: commissionPaidValue(c),
            status: "Paga",
            date: commissionEconomicDate(c),
          })),
      ],
      dateKey: "date",
    });
  }, [charges, transfers, commissions]);

  const handleExportAgents = useCallback(() => {
    setExportConfig({
      title: "Relatório de Desempenho de Agentes",
      columns: [
        { key: "name", label: "Agente" },
        { key: "role", label: "Função" },
        { key: "deals", label: "Negócios" },
        { key: "dealsValue", label: "Valor Negócios", format: (v: number) => currency(v) },
        { key: "leads", label: "Leads" },
        { key: "conversionRate", label: "Conversão", format: (v: number) => pct(v) },
        { key: "appointments", label: "Agendamentos" },
      ],
      data: analytics.agentPerformance,
    });
  }, [analytics.agentPerformance]);

  return (
    <div className="flex flex-col h-full bg-muted/50 font-sans overflow-hidden">
      <div className="flex-none px-6 lg:px-8 py-5 border-b border-border bg-card sticky top-0 z-20">
        <PageHeader
          title={<span className="flex items-center gap-3">Relatórios <span className="px-2.5 py-1 bg-primary-50 text-primary-700 text-xs font-bold rounded-full border border-primary-100">2026</span></span>}
          description="Análises, métricas e relatórios consolidados do CRM imobiliário"
        >
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              onClick={() => setShowComparison((c) => !c)}
              className={clsx(
                showComparison && "bg-violet-50 border-violet-200 text-violet-700 hover:bg-violet-50 hover:text-violet-700",
              )}
              title="Comparativo entre periodos"
            >
              <GitCompareArrows className="w-4 h-4" />
              <span className="hidden sm:inline">Comparar</span>
            </Button>

            <div className="relative">
              <Button variant="outline" onClick={() => setShowPeriod((p) => !p)}>
                <Calendar className="w-4 h-4 text-muted-foreground" />
                {PERIODS.find((p) => p.value === period)?.label}
                <ChevronDown className="w-3.5 h-3.5 text-muted-foreground" />
              </Button>
              <AnimatePresence>
                {showPeriod && (
                  <motion.div
                    initial={{ opacity: 0, y: -4 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0, y: -4 }}
                    className="absolute right-0 top-full mt-1 bg-card border border-border rounded-xl shadow-lg py-1 min-w-35 z-50"
                  >
                    {PERIODS.map((p) => (
                      <Button
                        key={p.value}
                        variant="ghost"
                        size="sm"
                        onClick={() => {
                          setPeriod(p.value);
                          setShowPeriod(false);
                        }}
                        className={clsx(
                          "w-full justify-start font-normal",
                          period === p.value ? "text-primary-600 font-bold bg-primary-50/50" : "text-foreground",
                        )}
                      >
                        {p.label}
                      </Button>
                    ))}
                  </motion.div>
                )}
              </AnimatePresence>
            </div>

            {/* `reports:export` era catálogo sem NENHUM consumidor: a
                documentação (docs/features/ROLES-MATRIX.md) afirmava que a
                permissão "esconde o botão", mas nenhuma tela a usava — o toggle
                salvava e nada mudava. Aqui ela passa a valer o que a doc diz:
                capacidade de UI, não controle de confidencialidade (a exportação
                roda no navegador, sobre dados já carregados). */}
            <Can permission="reports:export">
              <Button
                onClick={() => {
                  if (activeTab === "overview") handleExportOverview();
                  else if (activeTab === "leads") handleExportLeads();
                  else if (activeTab === "finance") handleExportFinance();
                  else if (activeTab === "performance") handleExportAgents();
                  else toast.info("Exportação disponível nas demais abas");
                }}
              >
                <Download className="w-4 h-4" />
                <span className="hidden sm:inline">Exportar</span>
              </Button>
            </Can>
          </div>
        </PageHeader>

        <div className="mt-4 -mb-px flex gap-1 overflow-x-auto scrollbar-hide">
          {TABS.map((tab) => {
            const Icon = tab.icon;
            const active = activeTab === tab.id;
            return (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={clsx(
                  "flex items-center gap-2 px-4 py-2.5 text-sm font-bold rounded-t-xl border border-b-0 whitespace-nowrap transition-colors",
                  active ? "bg-card text-primary-600 border-border" : "bg-transparent text-muted-foreground border-transparent hover:text-foreground hover:bg-muted",
                )}
              >
                <Icon className="w-4 h-4" />
                {tab.label}
              </button>
            );
          })}
        </div>
      </div>

      <div className="flex-1 overflow-y-auto">
        <div className="px-6 lg:px-8 pt-5">
          <ReportFilters
            agents={agents}
            propertyTypes={availablePropertyTypes}
            filters={filters}
            onChange={setFilters}
            onReset={() => setFilters(EMPTY_FILTERS)}
          />
        </div>
        <AnimatePresence mode="wait" initial={false}>
          <motion.div
            key={activeTab}
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.15 }}
            className="p-6 lg:p-8"
            style={{ minWidth: 0 }}
          >
            {activeTab === "overview" && (
              <OverviewTab
                analytics={analytics}
                prevAnalytics={prevAnalytics}
                showComparison={showComparison}
                monthlyRevenueData={monthlyRevenueData}
                dealsPipelineData={dealsPipelineData}
                propertyTypeData={propertyTypeData}
                leadFunnelData={leadFunnelData}
                onDrillDownLeadStatus={handleDrillDownLeadStatus}
                onDrillDownDealStage={handleDrillDownDealStage}
                onDrillDownPropertyType={handleDrillDownPropertyType}
                dre={dre}
              />
            )}
            {activeTab === "sales" && (
              <SalesTab
                analytics={analytics}
                prevAnalytics={prevAnalytics}
                showComparison={showComparison}
                monthlySalesData={monthlySalesData}
                propertyStatusData={propertyStatusData}
                propertyTypeData={propertyTypeData}
              />
            )}
            {activeTab === "leads" && (
              <LeadsTab
                analytics={analytics}
                prevAnalytics={prevAnalytics}
                showComparison={showComparison}
                leadFunnelData={leadFunnelData}
                leadSourceData={leadSourceData}
                dealsPipelineData={dealsPipelineData}
                onDrillDownLeadStatus={handleDrillDownLeadStatus}
                onDrillDownDealStage={handleDrillDownDealStage}
              />
            )}
            {activeTab === "finance" && (
              <FinanceTab
                analytics={analytics}
                prevAnalytics={prevAnalytics}
                showComparison={showComparison}
                monthlyRevenueData={monthlyRevenueData}
                revenueByStatusData={revenueByStatusData}
                cashFlowData={cashFlowData}
                onDrillDownRevenueStatus={handleDrillDownRevenueStatus}
                dre={dre}
                prevDre={prevDre}
                financeCurrent={financeCurrent}
                dimensionFilterActive={!serverAggregateApplies}
              />
            )}
            {activeTab === "performance" && <PerformanceTab analytics={analytics} agentRadarData={agentRadarData} />}
          </motion.div>
        </AnimatePresence>
      </div>

      <DrillDownPanel data={drillDown} onClose={() => setDrillDown(null)} />

      {exportConfig && <ExportReportModal config={exportConfig} onClose={() => setExportConfig(null)} />}
    </div>
  );
}

function OverviewTab({
  analytics,
  prevAnalytics,
  showComparison,
  monthlyRevenueData,
  dealsPipelineData,
  propertyTypeData,
  leadFunnelData,
  onDrillDownLeadStatus,
  onDrillDownDealStage,
  onDrillDownPropertyType,
  dre,
}: any) {
  // Receita = soma dos buckets de receita da DRE. net_cents é receita −
  // despesa e não cabe no cartão de receita (a despesa tem cartão próprio).
  const revenueTotal = dre
    ? dre.revenue.reduce((sum: number, bucket: { total_cents: number }) => sum + bucket.total_cents, 0) / 100
    : analytics.totalRevenue;
  const compChange = (cur: number, prev: number) => (prev !== 0 ? ((cur - prev) / prev) * 100 : 0);
  const { focusMode } = useFocusMode();

  return (
    <div className="space-y-6">
      <AnimatePresence>
        {!focusMode && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.3, ease: "easeInOut" }}
          >
            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
              <ReportKPICard
                label="Valor do Portfólio"
                value={currency(analytics.portfolioValue)}
                change={compChange(analytics.portfolioValue, prevAnalytics.portfolioValue)}
                changeLabel="vs. periodo anterior"
                icon={Building2}
                iconBg="bg-primary-50"
                iconColor="text-primary-600"
                delay={0}
                sparkData={[45, 52, 48, 61, 55, 70, 68, 82]}
              />
              <ReportKPICard
                label="Receita Total"
                value={currency(revenueTotal)}
                change={compChange(revenueTotal, prevAnalytics.totalRevenue)}
                changeLabel="vs. periodo anterior"
                icon={DollarSign}
                iconBg="bg-status-ok-soft"
                iconColor="text-status-ok"
                delay={0.05}
                sparkData={[30, 42, 35, 55, 48, 62, 58, 72]}
              />
              <ReportKPICard
                label="Taxa de Conversão"
                value={pct(analytics.conversionRate)}
                change={compChange(analytics.conversionRate, prevAnalytics.conversionRate)}
                changeLabel="vs. periodo anterior"
                icon={Target}
                iconBg="bg-status-warning-soft"
                iconColor="text-status-warning"
                delay={0.1}
                sparkData={[18, 22, 20, 15, 14, 16, 12, 14]}
              />
              <ReportKPICard
                label="Pipeline Ativo"
                value={currency(analytics.pipelineValue)}
                change={compChange(analytics.pipelineValue, prevAnalytics.pipelineValue)}
                changeLabel={`${analytics.totalDeals} negócios ativos`}
                icon={Activity}
                iconBg="bg-violet-50"
                iconColor="text-violet-600"
                delay={0.15}
                sparkData={[25, 32, 28, 40, 38, 52, 48, 60]}
              />
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {!focusMode && showComparison && (
        <motion.div
          initial={{ opacity: 0, height: 0 }}
          animate={{ opacity: 1, height: "auto" }}
          exit={{ opacity: 0, height: 0 }}
          className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4"
        >
          <ComparisonIndicator currentValue={analytics.portfolioValue} previousValue={prevAnalytics.portfolioValue} format={currency} label="Atual" previousLabel="Anterior" />
          <ComparisonIndicator currentValue={revenueTotal} previousValue={prevAnalytics.totalRevenue} format={currency} label="Receita Atual" previousLabel="Receita Ant." />
          <ComparisonIndicator currentValue={analytics.totalLeads} previousValue={prevAnalytics.totalLeads} format={(v) => String(v)} label="Leads Atual" previousLabel="Leads Ant." />
          <ComparisonIndicator currentValue={analytics.pipelineValue} previousValue={prevAnalytics.pipelineValue} format={currency} label="Pipeline Atual" previousLabel="Pipeline Ant." />
        </motion.div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <ChartCard title="Receita Mensal" subtitle="Receitas vs despesas -- clique para detalhes" className="lg:col-span-2" delay={0.1}>
          <RC height={280}>
            <BarChart data={monthlyRevenueData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#f1f5f9" />
              <XAxis dataKey="month" tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} />
              <YAxis tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} tickFormatter={(v) => `${(v / 1000).toFixed(0)}k`} />
              <Tooltip contentStyle={{ borderRadius: 12, border: "1px solid #e2e8f0", fontSize: 12 }} formatter={(value) => currency(Number(value))} />
              <Legend wrapperStyle={{ fontSize: 11 }} />
              <Bar dataKey="receita" name="Receita" fill={COLORS.primary} radius={[4, 4, 0, 0]} className="cursor-pointer" isAnimationActive={false} />
              <Bar dataKey="despesa" name="Despesa" fill={COLORS.rose} radius={[4, 4, 0, 0]} className="cursor-pointer" isAnimationActive={false} />
            </BarChart>
          </RC>
        </ChartCard>

        <ChartCard title="Portfólio por tipo" subtitle="Clique para detalhes" delay={0.15}>
          <RC height={280}>
            <RechartsPie>
              <Pie
                data={propertyTypeData}
                cx="50%"
                cy="50%"
                innerRadius={55}
                outerRadius={90}
                paddingAngle={3}
                dataKey="value"
                onClick={(entry: any) => onDrillDownPropertyType(entry.name)}
                className="cursor-pointer"
                isAnimationActive={false}>
                {propertyTypeData.map((_: any, i: number) => (
                  <Cell key={i} fill={PIE_COLORS[i % PIE_COLORS.length]} />
                ))}
              </Pie>
              <Tooltip contentStyle={{ borderRadius: 12, border: "1px solid #e2e8f0", fontSize: 12 }} />
              <Legend wrapperStyle={{ fontSize: 11 }} />
            </RechartsPie>
          </RC>
        </ChartCard>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <ChartCard title="Pipeline de Negócios" subtitle="Clique na barra para detalhes" delay={0.2}>
          <RC height={260}>
            <BarChart
              data={dealsPipelineData}
              layout="vertical"
              onClick={(state: any) => {
                if (state?.activePayload?.[0]?.payload?.stage) {
                  onDrillDownDealStage(state.activePayload[0].payload.stage);
                }
              }}
              className="cursor-pointer"
            >
              <CartesianGrid strokeDasharray="3 3" stroke="#f1f5f9" horizontal={false} />
              <XAxis type="number" tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} />
              <YAxis dataKey="stage" type="category" tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} width={100} />
              <Tooltip
                contentStyle={{ borderRadius: 12, border: "1px solid #e2e8f0", fontSize: 12 }}
                formatter={(value, name) => [name === "valor" ? currency(Number(value)) : value, name === "valor" ? "Valor" : "Quantidade"]}
              />
              <Bar dataKey="qtd" name="Quantidade" fill={COLORS.primary} radius={[0, 4, 4, 0]} isAnimationActive={false} />
            </BarChart>
          </RC>
        </ChartCard>

        <ChartCard title="Funil de Leads" subtitle="Clique na barra para detalhes" delay={0.25}>
          <RC height={260}>
            <BarChart
              data={leadFunnelData}
              onClick={(state: any) => {
                if (state?.activePayload?.[0]?.payload?.name) {
                  onDrillDownLeadStatus(state.activePayload[0].payload.name);
                }
              }}
              className="cursor-pointer"
            >
              <CartesianGrid strokeDasharray="3 3" stroke="#f1f5f9" />
              <XAxis dataKey="name" tick={{ fontSize: 10, fill: "#94a3b8" }} axisLine={false} tickLine={false} />
              <YAxis tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} />
              <Tooltip contentStyle={{ borderRadius: 12, border: "1px solid #e2e8f0", fontSize: 12 }} />
              <Bar dataKey="value" name="Leads" radius={[4, 4, 0, 0]}>
                {leadFunnelData.map((entry: any, i: number) => (
                  <Cell key={i} fill={entry.fill} />
                ))}
              </Bar>
            </BarChart>
          </RC>
        </ChartCard>
      </div>

      <AnimatePresence>
        {!focusMode && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.3, ease: "easeInOut" }}
          >
            <div className="grid grid-cols-2 lg:grid-cols-5 gap-4">
              {[
                { label: "Imóveis", value: analytics.totalProperties, icon: Building2, color: "text-primary-600", bg: "bg-primary-50" },
                { label: "Contratos Ativos", value: analytics.activeContracts, icon: FileText, color: "text-status-ok", bg: "bg-status-ok-soft" },
                { label: "Leads Ativos", value: analytics.totalLeads - analytics.convertedLeads - analytics.lostLeads, icon: Users, color: "text-status-info", bg: "bg-sky-50" },
                { label: "OS Abertas", value: analytics.openTickets, icon: AlertCircle, color: "text-status-warning", bg: "bg-status-warning-soft" },
                { label: "Compromissos", value: analytics.totalAppointments, icon: Clock, color: "text-violet-600", bg: "bg-violet-50" },
              ].map((item, i) => (
                <motion.div
                  key={item.label}
                  initial={{ opacity: 0, y: 12 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: 0.3 + i * 0.04 }}
                  className="bg-card rounded-xl border border-border p-4 flex items-center gap-3"
                >
                  <div className={clsx("w-9 h-9 rounded-lg flex items-center justify-center", item.bg)}>
                    <item.icon className={clsx("w-4.5 h-4.5", item.color)} />
                  </div>
                  <div>
                    <p className="text-lg font-bold text-foreground">{item.value}</p>
                    <p className="text-caption text-muted-foreground">{item.label}</p>
                  </div>
                </motion.div>
              ))}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

function SalesTab({ analytics, prevAnalytics, showComparison, monthlySalesData, propertyStatusData, propertyTypeData }: any) {
  const compChange = (cur: number, prev: number) => (prev !== 0 ? ((cur - prev) / prev) * 100 : 0);
  const { focusMode } = useFocusMode();

  return (
    <div className="space-y-6">
      <AnimatePresence>
        {!focusMode && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.3, ease: "easeInOut" }}
          >
            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
              <ReportKPICard
                label="Total Vendas"
                value={currency(analytics.totalSalesValue)}
                change={compChange(analytics.totalSalesValue, prevAnalytics.totalSalesValue)}
                icon={Home}
                iconBg="bg-primary-50"
                iconColor="text-primary-600"
                delay={0}
                sparkData={[20, 35, 28, 42, 55, 60, 72, 80]}
              />
              <ReportKPICard
                label="Total Locações (mensal)"
                value={currency(analytics.totalRentalValue)}
                change={compChange(analytics.totalRentalValue, prevAnalytics.totalRentalValue)}
                icon={Key}
                iconBg="bg-status-ok-soft"
                iconColor="text-status-ok"
                delay={0.05}
                sparkData={[30, 32, 35, 33, 38, 40, 42, 45]}
              />
              <ReportKPICard
                label="Contratos de Venda"
                value={String(analytics.salesContracts)}
                change={10}
                icon={FileText}
                iconBg="bg-status-warning-soft"
                iconColor="text-status-warning"
                delay={0.1}
              />
              <ReportKPICard
                label="Contratos de Locação"
                value={String(analytics.rentalContracts)}
                change={20}
                icon={FileText}
                iconBg="bg-sky-50"
                iconColor="text-status-info"
                delay={0.15}
              />
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {!focusMode && showComparison && (
        <motion.div
          initial={{ opacity: 0, height: 0 }}
          animate={{ opacity: 1, height: "auto" }}
          exit={{ opacity: 0, height: 0 }}
          className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4"
        >
          <ComparisonIndicator currentValue={analytics.totalSalesValue} previousValue={prevAnalytics.totalSalesValue} format={currency} label="Vendas Atual" previousLabel="Vendas Ant." />
          <ComparisonIndicator currentValue={analytics.totalRentalValue} previousValue={prevAnalytics.totalRentalValue} format={currency} label="Locações Atual" previousLabel="Locações Ant." />
          <ComparisonIndicator currentValue={analytics.totalCommissions} previousValue={prevAnalytics.totalCommissions} format={currency} label="Comissões Atual" previousLabel="Comissões Ant." />
        </motion.div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <ChartCard title="Vendas & Locações Mensais" subtitle="Quantidade de contratos por mês" className="lg:col-span-2" delay={0.1}>
          <RC height={300}>
            <AreaChart data={monthlySalesData}>
              <defs>
                <linearGradient id="gradVendas" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor={COLORS.primary} stopOpacity={0.3} />
                  <stop offset="100%" stopColor={COLORS.primary} stopOpacity={0} />
                </linearGradient>
                <linearGradient id="gradLocacoes" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor={COLORS.emerald} stopOpacity={0.3} />
                  <stop offset="100%" stopColor={COLORS.emerald} stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="#f1f5f9" />
              <XAxis dataKey="month" tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} />
              <YAxis tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} />
              <Tooltip contentStyle={{ borderRadius: 12, border: "1px solid #e2e8f0", fontSize: 12 }} />
              <Legend wrapperStyle={{ fontSize: 11 }} />
              <Area type="monotone" dataKey="vendas" name="Vendas" stroke={COLORS.primary} fill="url(#gradVendas)" strokeWidth={2} isAnimationActive={false} />
              <Area type="monotone" dataKey="locacoes" name="Locações" stroke={COLORS.emerald} fill="url(#gradLocacoes)" strokeWidth={2} isAnimationActive={false} />
            </AreaChart>
          </RC>
        </ChartCard>

        <ChartCard title="Imóveis por Status" subtitle="Distribuição do portfolio" delay={0.15}>
          <div className="flex h-75 items-center justify-center">
            <Donut data={propertyStatusData} colors={PIE_COLORS} />
          </div>
        </ChartCard>
      </div>

      <ChartCard title="Valor por Tipo de Transacao" subtitle="Vendas vs locacoes - volume financeiro" delay={0.2}>
        <RC height={280}>
          <BarChart data={monthlySalesData}>
            <CartesianGrid strokeDasharray="3 3" stroke="#f1f5f9" />
            <XAxis dataKey="month" tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} />
            <YAxis tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} tickFormatter={(v) => `${(v / 1000000).toFixed(1)}M`} />
            <Tooltip contentStyle={{ borderRadius: 12, border: "1px solid #e2e8f0", fontSize: 12 }} formatter={(v) => currency(Number(v))} />
            <Legend wrapperStyle={{ fontSize: 11 }} />
            <Bar dataKey="valorVendas" name="Volume Vendas" fill={COLORS.primary} radius={[4, 4, 0, 0]} isAnimationActive={false} />
            <Bar dataKey="valorLocacoes" name="Volume Locações" fill={COLORS.teal} radius={[4, 4, 0, 0]} isAnimationActive={false} />
          </BarChart>
        </RC>
      </ChartCard>

      <div className="bg-card rounded-2xl border border-border shadow-sm p-6">
        <div className="flex items-center gap-3 mb-4">
          <div className="w-10 h-10 rounded-xl bg-violet-50 flex items-center justify-center">
            <Percent className="w-5 h-5 text-violet-600" />
          </div>
          <div>
            <h3 className="text-sm font-bold text-foreground">Comissões Geradas</h3>
            <p className="text-caption text-muted-foreground">
              {"Soma das comissões registradas"}{analytics.commissionCount > 0 ? ` (${analytics.commissionCount})` : ""}
            </p>
          </div>
        </div>
        <p className="text-3xl font-bold text-foreground">{currency(analytics.totalCommissions)}</p>
      </div>

      <ChartCard title="Distribuição por Tipo de Imóvel" subtitle="Segmentação do portfólio" delay={0.25}>
        <div className="flex h-70 items-center justify-center">
          <Donut data={propertyTypeData} colors={PIE_COLORS} />
        </div>
      </ChartCard>
    </div>
  );
}

function LeadsTab({ analytics, prevAnalytics, showComparison, leadFunnelData, leadSourceData, dealsPipelineData, onDrillDownLeadStatus, onDrillDownDealStage }: any) {
  const compChange = (cur: number, prev: number) => (prev !== 0 ? ((cur - prev) / prev) * 100 : 0);
  const { focusMode } = useFocusMode();

  return (
    <div className="space-y-6">
      <AnimatePresence>
        {!focusMode && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.3, ease: "easeInOut" }}
          >
            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
              <ReportKPICard
                label="Total de Leads"
                value={String(analytics.totalLeads)}
                change={compChange(analytics.totalLeads, prevAnalytics.totalLeads)}
                icon={Users}
                iconBg="bg-primary-50"
                iconColor="text-primary-600"
                delay={0}
                sparkData={[5, 8, 6, 10, 9, 12, 11, 14]}
              />
              <ReportKPICard
                label="Taxa de Conversão"
                value={pct(analytics.conversionRate)}
                change={compChange(analytics.conversionRate, prevAnalytics.conversionRate)}
                icon={Target}
                iconBg="bg-status-ok-soft"
                iconColor="text-status-ok"
                delay={0.05}
                sparkData={[18, 22, 20, 15, 14, 16, 12, 14]}
              />
              <ReportKPICard
                label="Leads Convertidos"
                value={String(analytics.convertedLeads)}
                change={compChange(analytics.convertedLeads, prevAnalytics.convertedLeads)}
                icon={CheckCircle2}
                iconBg="bg-teal-50"
                iconColor="text-status-info"
                delay={0.1}
              />
              <ReportKPICard
                label="Leads Perdidos"
                value={String(analytics.lostLeads)}
                change={compChange(analytics.lostLeads, prevAnalytics.lostLeads)}
                icon={XCircle}
                iconBg="bg-status-critical-soft"
                iconColor="text-status-critical"
                delay={0.15}
              />
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {!focusMode && showComparison && (
        <motion.div
          initial={{ opacity: 0, height: 0 }}
          animate={{ opacity: 1, height: "auto" }}
          exit={{ opacity: 0, height: 0 }}
          className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4"
        >
          <ComparisonIndicator currentValue={analytics.totalLeads} previousValue={prevAnalytics.totalLeads} format={(v) => String(v)} label="Leads Atual" previousLabel="Leads Ant." />
          <ComparisonIndicator currentValue={analytics.convertedLeads} previousValue={prevAnalytics.convertedLeads} format={(v) => String(v)} label="Convertidos" previousLabel="Conv. Ant." />
          <ComparisonIndicator currentValue={analytics.lostLeads} previousValue={prevAnalytics.lostLeads} format={(v) => String(v)} label="Perdidos" previousLabel="Perd. Ant." />
          <ComparisonIndicator currentValue={analytics.totalDeals} previousValue={prevAnalytics.totalDeals} format={(v) => String(v)} label="Negócios" previousLabel="Neg. Ant." />
        </motion.div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <ChartCard title="Funil de Conversão" subtitle="Clique na barra para detalhes" delay={0.1}>
          <RC height={300}>
            <BarChart
              data={leadFunnelData}
              onClick={(state: any) => {
                if (state?.activePayload?.[0]?.payload?.name) {
                  onDrillDownLeadStatus(state.activePayload[0].payload.name);
                }
              }}
              className="cursor-pointer"
            >
              <CartesianGrid strokeDasharray="3 3" stroke="#f1f5f9" />
              <XAxis dataKey="name" tick={{ fontSize: 10, fill: "#94a3b8" }} axisLine={false} tickLine={false} angle={-20} textAnchor="end" height={50} />
              <YAxis tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} />
              <Tooltip contentStyle={{ borderRadius: 12, border: "1px solid #e2e8f0", fontSize: 12 }} />
              <Bar dataKey="value" name="Leads" radius={[6, 6, 0, 0]}>
                {leadFunnelData.map((entry: any, i: number) => (
                  <Cell key={i} fill={entry.fill} />
                ))}
              </Bar>
            </BarChart>
          </RC>
        </ChartCard>

        <ChartCard title="Origem dos Leads" subtitle="Distribuição por canal de captacao" delay={0.15}>
          <RC height={300}>
            <RechartsPie>
              <Pie
                data={leadSourceData}
                cx="50%"
                cy="50%"
                innerRadius={60}
                outerRadius={95}
                paddingAngle={3}
                dataKey="value"
                label={({ name, percent }) => `${name} ${((percent ?? 0) * 100).toFixed(0)}%`}
                isAnimationActive={false}>
                {leadSourceData.map((_: any, i: number) => (
                  <Cell key={i} fill={PIE_COLORS[i % PIE_COLORS.length]} />
                ))}
              </Pie>
              <Tooltip contentStyle={{ borderRadius: 12, border: "1px solid #e2e8f0", fontSize: 12 }} />
            </RechartsPie>
          </RC>
        </ChartCard>
      </div>

      <ChartCard title="Pipeline por Etapa da Esteira" subtitle="Clique para detalhes" delay={0.2}>
        <RC height={260}>
          <BarChart
            data={dealsPipelineData}
            layout="vertical"
            onClick={(state: any) => {
              if (state?.activePayload?.[0]?.payload?.stage) {
                onDrillDownDealStage(state.activePayload[0].payload.stage);
              }
            }}
            className="cursor-pointer"
          >
            <CartesianGrid strokeDasharray="3 3" stroke="#f1f5f9" horizontal={false} />
            <XAxis type="number" tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} tickFormatter={(v) => `${(v / 1000).toFixed(0)}k`} />
            <YAxis dataKey="stage" type="category" tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} width={110} />
            <Tooltip
              contentStyle={{ borderRadius: 12, border: "1px solid #e2e8f0", fontSize: 12 }}
              formatter={(v, name) => [name === "valor" ? currency(Number(v)) : v, name === "valor" ? "Valor" : "Quantidade"]}
            />
            <Legend wrapperStyle={{ fontSize: 11 }} />
            <Bar dataKey="qtd" name="Quantidade" fill={COLORS.primary} radius={[0, 4, 4, 0]} isAnimationActive={false} />
            <Bar dataKey="valor" name="Valor" fill={COLORS.emerald} radius={[0, 4, 4, 0]} isAnimationActive={false} />
          </BarChart>
        </RC>
      </ChartCard>

      <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
        <div className="px-5 py-4 border-b border-border">
          <h3 className="text-sm font-bold text-foreground">Detalhamento por Status</h3>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-muted border-b border-border">
                <th className="text-left px-5 py-3 text-xs font-bold text-muted-foreground uppercase tracking-wider">Status</th>
                <th className="text-right px-5 py-3 text-xs font-bold text-muted-foreground uppercase tracking-wider">Quantidade</th>
                <th className="text-right px-5 py-3 text-xs font-bold text-muted-foreground uppercase tracking-wider">% do Total</th>
                <th className="px-5 py-3 text-xs font-bold text-muted-foreground uppercase tracking-wider">Distribuição</th>
              </tr>
            </thead>
            <tbody>
              {Object.entries(analytics.leadsByStatus).map(([status, count]: [string, any]) => {
                const pctVal = analytics.totalLeads > 0 ? (count / analytics.totalLeads) * 100 : 0;
                return (
                  <tr key={status} className="border-b border-border/50 hover:bg-muted/50 transition-colors">
                    <td className="px-5 py-3 font-bold text-foreground">{status}</td>
                    <td className="px-5 py-3 text-right text-foreground">{count}</td>
                    <td className="px-5 py-3 text-right text-foreground">{pctVal.toFixed(1)}%</td>
                    <td className="px-5 py-3">
                      <div className="h-2 bg-muted rounded-full overflow-hidden max-w-50">
                        <div className="h-full bg-primary-500 rounded-full" style={{ width: `${pctVal}%` }} />
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}

function FinanceTab({ analytics, prevAnalytics, showComparison, monthlyRevenueData, revenueByStatusData, cashFlowData, onDrillDownRevenueStatus, dre, prevDre, financeCurrent, dimensionFilterActive }: any) {
  // Procedência dos números. O agregado do servidor (DRE) é autoritativo, mas
  // não sabe filtrar por imóvel nem corretor; com esses filtros ativos o total
  // passa a ser calculado localmente sobre as listas já filtradas.
  const origem = dimensionFilterActive ? "filtro de imóvel/corretor aplicado" : "consolidado do período";
  // Totais autoritativos do servidor quando presentes; a fronteira local é o
  // fallback para antes da resposta chegar.
  // Receita = soma dos buckets de receita da DRE. net_cents é receita −
  // despesa e não cabe no cartão de receita (a despesa tem cartão próprio).
  const revenueTotal = dre
    ? dre.revenue.reduce((sum: number, bucket: { total_cents: number }) => sum + bucket.total_cents, 0) / 100
    : analytics.totalRevenue;
  const expensesTotal = dre ? dre.expense.reduce((sum: number, b: { total_cents: number }) => sum + b.total_cents, 0) / 100 : analytics.totalExpenses;
  const prevRevenueTotal = prevDre
    ? prevDre.revenue.reduce((sum: number, b: { total_cents: number }) => sum + b.total_cents, 0) / 100
    : prevAnalytics.totalRevenue;
  const prevExpensesTotal = prevDre
    ? prevDre.expense.reduce((sum: number, b: { total_cents: number }) => sum + b.total_cents, 0) / 100
    : prevAnalytics.totalExpenses;
  const compChange = (cur: number, prev: number) => (prev !== 0 ? ((cur - prev) / prev) * 100 : 0);
  const { focusMode } = useFocusMode();

  return (
    <div className="space-y-6">
      <AnimatePresence>
        {!focusMode && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.3, ease: "easeInOut" }}
          >
            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
              <ReportKPICard
                label="Receita Total"
                value={currency(revenueTotal)}
                change={compChange(revenueTotal, prevAnalytics.totalRevenue)}
                icon={ArrowUpRight}
                iconBg="bg-status-ok-soft"
                iconColor="text-status-ok"
                delay={0}
                sparkData={[30, 42, 35, 55, 48, 62, 58, 72]}
              />
              <ReportKPICard
                label="Despesas Totais"
                value={currency(expensesTotal)}
                change={compChange(expensesTotal, prevAnalytics.totalExpenses)}
                icon={ArrowDownRight}
                iconBg="bg-status-critical-soft"
                iconColor="text-status-critical"
                delay={0.05}
                sparkData={[10, 12, 15, 11, 18, 14, 16, 15]}
              />
              <ReportKPICard
                label="Repasses"
                value={currency(analytics.totalRepasses)}
                change={compChange(analytics.totalRepasses, prevAnalytics.totalRepasses)}
                icon={Wallet}
                iconBg="bg-status-warning-soft"
                iconColor="text-status-warning"
                delay={0.1}
              />
              <ReportKPICard
                label="Inadimplência"
                value={currency(analytics.overdueRevenue)}
                change={compChange(analytics.overdueRevenue, prevAnalytics.overdueRevenue)}
                icon={AlertCircle}
                iconBg="bg-status-critical-soft"
                iconColor="text-status-critical"
                delay={0.15}
              />
            </div>
            {/* Sem isto, o operador não saberia se o número é o consolidado
                do período ou o recorte por imóvel/corretor. */}
            <p className="mt-3 text-xs text-muted-foreground">
              Origem dos totais: {origem}.
            </p>
          </motion.div>
        )}
      </AnimatePresence>

      {!focusMode && showComparison && (
        <motion.div
          initial={{ opacity: 0, height: 0 }}
          animate={{ opacity: 1, height: "auto" }}
          exit={{ opacity: 0, height: 0 }}
          className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4"
        >
          <ComparisonIndicator currentValue={revenueTotal} previousValue={prevRevenueTotal} format={currency} label="Receita Atual" previousLabel="Receita Ant." />
          <ComparisonIndicator currentValue={expensesTotal} previousValue={prevExpensesTotal} format={currency} label="Despesas Atual" previousLabel="Despesas Ant." />
          <ComparisonIndicator currentValue={analytics.paidRevenue} previousValue={prevAnalytics.paidRevenue} format={currency} label="Recebido" previousLabel="Receb. Ant." />
          <ComparisonIndicator currentValue={analytics.overdueRevenue} previousValue={prevAnalytics.overdueRevenue} format={currency} label="Inadimplência" previousLabel="Inadimp. Ant." />
        </motion.div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <ChartCard title="Fluxo de Caixa" subtitle="Entradas, saidas e saldo" className="lg:col-span-2" delay={0.1}>
          <RC height={300}>
            <AreaChart data={cashFlowData}>
              <defs>
                <linearGradient id="gradEntrada" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor={COLORS.emerald} stopOpacity={0.25} />
                  <stop offset="100%" stopColor={COLORS.emerald} stopOpacity={0} />
                </linearGradient>
                <linearGradient id="gradSaida" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor={COLORS.rose} stopOpacity={0.15} />
                  <stop offset="100%" stopColor={COLORS.rose} stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="#f1f5f9" />
              <XAxis dataKey="month" tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} />
              <YAxis tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} tickFormatter={(v) => `${(v / 1000).toFixed(0)}k`} />
              <Tooltip contentStyle={{ borderRadius: 12, border: "1px solid #e2e8f0", fontSize: 12 }} formatter={(v) => currency(Number(v))} />
              <Legend wrapperStyle={{ fontSize: 11 }} />
              <Area type="monotone" dataKey="entradas" name="Entradas" stroke={COLORS.emerald} fill="url(#gradEntrada)" strokeWidth={2} isAnimationActive={false} />
              <Area type="monotone" dataKey="saidas" name="Saidas" stroke={COLORS.rose} fill="url(#gradSaida)" strokeWidth={2} isAnimationActive={false} />
              <Line type="monotone" dataKey="saldo" name="Saldo" stroke={COLORS.primary} strokeWidth={2.5} strokeDasharray="6 3" dot={{ r: 3, fill: COLORS.primary }} isAnimationActive={false} />
            </AreaChart>
          </RC>
        </ChartCard>

        <ChartCard title="Cobranças por Status" subtitle="Carteira arrecadada, pendente e vencida -- clique para detalhes" delay={0.15}>
          <RC height={300}>
            <RechartsPie>
              <Pie
                data={revenueByStatusData}
                cx="50%"
                cy="50%"
                innerRadius={55}
                outerRadius={90}
                paddingAngle={4}
                dataKey="value"
                label={({ name, percent }) => `${name} ${((percent ?? 0) * 100).toFixed(0)}%`}
                onClick={(entry: any) => onDrillDownRevenueStatus(entry.name)}
                className="cursor-pointer"
                isAnimationActive={false}>
                {revenueByStatusData.map((entry: any, i: number) => (
                  <Cell key={i} fill={entry.fill} />
                ))}
              </Pie>
              <Tooltip contentStyle={{ borderRadius: 12, border: "1px solid #e2e8f0", fontSize: 12 }} formatter={(v) => currency(Number(v))} />
            </RechartsPie>
          </RC>
        </ChartCard>
      </div>

      <ChartCard title="Receita vs Despesa Mensal" subtitle="Evolucao ao longo do ano" delay={0.2}>
        <RC height={280}>
          <LineChart data={monthlyRevenueData}>
            <CartesianGrid strokeDasharray="3 3" stroke="#f1f5f9" />
            <XAxis dataKey="month" tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} />
            <YAxis tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} tickFormatter={(v) => `${(v / 1000).toFixed(0)}k`} />
            <Tooltip contentStyle={{ borderRadius: 12, border: "1px solid #e2e8f0", fontSize: 12 }} formatter={(v) => currency(Number(v))} />
            <Legend wrapperStyle={{ fontSize: 11 }} />
            <Line type="monotone" dataKey="receita" name="Receita" stroke={COLORS.emerald} strokeWidth={2.5} dot={{ r: 3 }} isAnimationActive={false} />
            <Line type="monotone" dataKey="despesa" name="Despesa" stroke={COLORS.rose} strokeWidth={2.5} dot={{ r: 3 }} isAnimationActive={false} />
            <Line type="monotone" dataKey="lucro" name="Lucro" stroke={COLORS.primary} strokeWidth={2.5} dot={{ r: 3 }} isAnimationActive={false} />
          </LineChart>
        </RC>
      </ChartCard>

      <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
        <div className="px-5 py-4 border-b border-border flex items-center justify-between">
          <h3 className="text-sm font-bold text-foreground">Resumo Financeiro</h3>
        </div>
        <div className="p-5 grid grid-cols-1 sm:grid-cols-3 gap-6">
          {[
            { label: "Receita Recebida", value: financeCurrent.received, color: "text-status-ok", bg: "bg-status-ok-soft", icon: CheckCircle2 },
            { label: "A Receber (Pendente)", value: financeCurrent.pending, color: "text-status-warning", bg: "bg-status-warning-soft", icon: Clock },
            { label: "Em Atraso", value: financeCurrent.overdue, color: "text-status-critical", bg: "bg-status-critical-soft", icon: AlertCircle },
          ].map((item) => (
            <div key={item.label} className="flex items-center gap-4">
              <div className={clsx("w-12 h-12 rounded-xl flex items-center justify-center", item.bg)}>
                <item.icon className={clsx("w-6 h-6", item.color)} />
              </div>
              <div>
                <p className={clsx("text-xl font-bold", item.color)}>{currency(item.value)}</p>
                <p className="text-xs text-muted-foreground">{item.label}</p>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function PerformanceTab({ analytics, agentRadarData }: any) {
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {analytics.agentPerformance.map((agent: any, i: number) => (
          <motion.div
            key={agent.id}
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: i * 0.08 }}
            className="bg-card rounded-2xl border border-border shadow-sm p-5"
          >
            <div className="flex items-center gap-3 mb-4">
              {agent.avatar ? (
                <MediaImage src={agent.avatar} alt={agent.name} className="w-11 h-11 rounded-full object-cover border border-border shrink-0" />
              ) : (
                <div className="w-11 h-11 rounded-full bg-linear-to-br from-primary-500 to-violet-500 flex items-center justify-center text-white font-bold text-sm shrink-0">
                  {agent.name
                    .split(" ")
                    .map((n: string) => n[0])
                    .join("")
                    .slice(0, 2)}
                </div>
              )}
              <div>
                <h3 className="text-sm font-bold text-foreground">{agent.name}</h3>
                <p className="text-caption text-muted-foreground">{agent.role}</p>
              </div>
              {i === 0 && (
                <span className="ml-auto px-2 py-1 bg-status-warning-soft text-status-warning text-micro font-bold rounded-full border border-status-warning/30 flex items-center gap-1">
                  <Star className="w-3 h-3" /> Top
                </span>
              )}
            </div>

            <div className="grid grid-cols-2 gap-3">
              {[
                { label: "Negócios", value: agent.deals, icon: Briefcase },
                { label: "Leads", value: agent.leads, icon: Users },
                { label: "Conversão", value: pct(agent.conversionRate), icon: Target },
                { label: "Agendamentos", value: agent.appointments, icon: Calendar },
              ].map((metric) => (
                <div key={metric.label} className="bg-muted rounded-lg p-2.5 flex items-center gap-2">
                  <metric.icon className="w-3.5 h-3.5 text-muted-foreground" />
                  <div>
                    <p className="text-xs font-bold text-foreground">{metric.value}</p>
                    <p className="text-micro text-muted-foreground">{metric.label}</p>
                  </div>
                </div>
              ))}
            </div>

            <div className="mt-4 flex items-center justify-between text-xs">
              <span className="text-muted-foreground">Tempo medio de resposta</span>
              <span className="font-bold text-muted-foreground">{agent.avgResponseTime != null ? `${agent.avgResponseTime}min` : "N/D"}</span>
            </div>
            <div className="mt-1 flex items-center justify-between text-xs">
              <span className="text-muted-foreground">Satisfacao</span>
              <span className="font-bold text-muted-foreground flex items-center gap-1">
                {agent.satisfaction != null ? (
                  <>
                    <Star className="w-3 h-3 fill-amber-400 text-status-warning" />{" "}
                    <span className="text-status-warning">{agent.satisfaction}/5.0</span>
                  </>
                ) : (
                  "N/D"
                )}
              </span>
            </div>

            <div className="mt-3 h-1.5 bg-muted rounded-full overflow-hidden">
              <div className="h-full rounded-full bg-linear-to-r from-primary-500 to-emerald-500" style={{ width: `${Math.min(agent.conversionRate * 4, 100)}%` }} />
            </div>
          </motion.div>
        ))}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <ChartCard title="Comparativo de Desempenho" subtitle="Radar multi-dimensional" delay={0.15}>
          <RC height={320}>
            <RadarChart data={agentRadarData}>
              <PolarGrid stroke="#e2e8f0" />
              <PolarAngleAxis dataKey="name" tick={{ fontSize: 11, fill: "#64748b" }} />
              <PolarRadiusAxis tick={{ fontSize: 9, fill: "#94a3b8" }} />
              <Tooltip contentStyle={{ borderRadius: 12, border: "1px solid #e2e8f0", fontSize: 12 }} />
              <Radar name="Negócios" dataKey="negocios" stroke={COLORS.primary} fill={COLORS.primary} fillOpacity={0.15} strokeWidth={2} />
              <Radar name="Leads" dataKey="leads" stroke={COLORS.emerald} fill={COLORS.emerald} fillOpacity={0.1} strokeWidth={2} />
              <Radar name="Conversão" dataKey="conversao" stroke={COLORS.amber} fill={COLORS.amber} fillOpacity={0.1} strokeWidth={2} />
              <Legend wrapperStyle={{ fontSize: 11 }} />
            </RadarChart>
          </RC>
        </ChartCard>

        <ChartCard title="Volume de Negócios por Agente" subtitle="Valor total em negócios" delay={0.2}>
          <RC height={320}>
            <BarChart data={analytics.agentPerformance.map((a: any) => ({ name: a.name.split(" ")[0], valor: a.dealsValue, qtd: a.deals }))}>
              <CartesianGrid strokeDasharray="3 3" stroke="#f1f5f9" />
              <XAxis dataKey="name" tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} />
              <YAxis tick={{ fontSize: 11, fill: "#94a3b8" }} axisLine={false} tickLine={false} tickFormatter={(v) => `${(v / 1000000).toFixed(1)}M`} />
              <Tooltip contentStyle={{ borderRadius: 12, border: "1px solid #e2e8f0", fontSize: 12 }} formatter={(v) => currency(Number(v))} />
              <Bar dataKey="valor" name="Valor" fill={COLORS.primary} radius={[6, 6, 0, 0]}>
                {analytics.agentPerformance.map((_: any, i: number) => (
                  <Cell key={i} fill={PIE_COLORS[i % PIE_COLORS.length]} />
                ))}
              </Bar>
            </BarChart>
          </RC>
        </ChartCard>
      </div>

      <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
        <div className="px-5 py-4 border-b border-border">
          <h3 className="text-sm font-bold text-foreground">Ranking de Agentes</h3>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-muted border-b border-border">
                <th className="text-left px-5 py-3 text-xs font-bold text-muted-foreground uppercase tracking-wider">#</th>
                <th className="text-left px-5 py-3 text-xs font-bold text-muted-foreground uppercase tracking-wider">Agente</th>
                <th className="text-right px-5 py-3 text-xs font-bold text-muted-foreground uppercase tracking-wider">Negócios</th>
                <th className="text-right px-5 py-3 text-xs font-bold text-muted-foreground uppercase tracking-wider">Valor</th>
                <th className="text-right px-5 py-3 text-xs font-bold text-muted-foreground uppercase tracking-wider">Leads</th>
                <th className="text-right px-5 py-3 text-xs font-bold text-muted-foreground uppercase tracking-wider">Conversão</th>
                <th className="text-right px-5 py-3 text-xs font-bold text-muted-foreground uppercase tracking-wider">Satisfacao</th>
              </tr>
            </thead>
            <tbody>
              {analytics.agentPerformance
                .sort((a: any, b: any) => b.dealsValue - a.dealsValue)
                .map((agent: any, i: number) => (
                  <tr key={agent.id} className="border-b border-border/50 hover:bg-muted/50 transition-colors">
                    <td className="px-5 py-3">
                      <span
                        className={clsx(
                          "w-6 h-6 rounded-full flex items-center justify-center text-micro font-bold",
                          i === 0 ? "bg-status-warning-soft text-status-warning" : i === 1 ? "bg-muted text-muted-foreground" : "bg-orange-100 text-orange-700",
                        )}
                      >
                        {i + 1}
                      </span>
                    </td>
                    <td className="px-5 py-3">
                      <div className="flex items-center gap-3">
                        {agent.avatar ? (
                          <MediaImage src={agent.avatar} alt={agent.name} className="w-8 h-8 rounded-full object-cover border border-border shrink-0" />
                        ) : (
                          <div className="w-8 h-8 rounded-full bg-linear-to-br from-primary-500 to-violet-500 flex items-center justify-center text-white font-bold text-xs shrink-0">
                            {agent.name
                              .split(" ")
                              .map((n: string) => n[0])
                              .join("")
                              .slice(0, 2)}
                          </div>
                        )}
                        <div>
                          <p className="font-bold text-foreground">{agent.name}</p>
                          <p className="text-caption text-muted-foreground">{agent.role}</p>
                        </div>
                      </div>
                    </td>
                    <td className="px-5 py-3 text-right font-bold text-foreground">{agent.deals}</td>
                    <td className="px-5 py-3 text-right font-bold text-foreground">{currency(agent.dealsValue)}</td>
                    <td className="px-5 py-3 text-right text-foreground">{agent.leads}</td>
                    <td className="px-5 py-3 text-right">
                      <span
                        className={clsx(
                          "font-bold",
                          agent.conversionRate > 15 ? "text-status-ok" : agent.conversionRate > 10 ? "text-status-warning" : "text-status-critical",
                        )}
                      >
                        {pct(agent.conversionRate)}
                      </span>
                    </td>
                    <td className="px-5 py-3 text-right">
                      {agent.satisfaction != null ? (
                        <span className="flex items-center justify-end gap-1 font-bold text-status-warning">
                          <Star className="w-3 h-3 fill-amber-400 text-status-warning" />
                          {agent.satisfaction}
                        </span>
                      ) : (
                        <span className="font-bold text-muted-foreground">N/D</span>
                      )}
                    </td>
                  </tr>
                ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
