import { useCallback, useMemo, useState } from 'react';
import { Loader2, BarChart3 } from 'lucide-react';
import { useFinancialRead } from '../hooks/useFinancialRead';
import { FinanceLoadError } from '../components/finance/FinanceLoadError';
import { Card } from '../ui/card';
import { PageHeader } from '../ui/page-header';
import { KpisCards } from '../components/finance/KpisCards';
import { FinancialPositionCard } from '../components/finance/FinancialPositionCard';
import { DRETable } from '../components/finance/DRETable';
import { CashflowChart } from '../components/finance/CashflowChart';
import { OwnerStatementPanel } from '../components/finance/OwnerStatementPanel';
import { useLeadsContext } from '../context/LeadsContext';
import {
  financialReportService,
  type DREReport,
  type CashFlowReport,
  type CashFlowGranularity,
  type KPIs,
} from '../services/financialReport.service';

type PeriodPreset = 'month' | 'last3' | 'year' | 'custom';

function presetRange(preset: PeriodPreset): { from: string; to: string } {
  const now = new Date();
  const to = now.toISOString().slice(0, 10);
  if (preset === 'month') {
    const from = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), 1)).toISOString().slice(0, 10);
    return { from, to };
  }
  if (preset === 'last3') {
    const from = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - 2, 1)).toISOString().slice(0, 10);
    return { from, to };
  }
  if (preset === 'year') {
    const from = new Date(Date.UTC(now.getUTCFullYear(), 0, 1)).toISOString().slice(0, 10);
    return { from, to };
  }
  return { from: '', to };
}

interface FinanceDashboardProps {
  embedded?: boolean;
}

export default function FinanceDashboard({ embedded = false }: FinanceDashboardProps) {
  const { leads } = useLeadsContext();
  const [preset, setPreset] = useState<PeriodPreset>('month');
  const [granularity, setGranularity] = useState<CashFlowGranularity>('day');
  const [customFrom, setCustomFrom] = useState('');
  const [customTo, setCustomTo] = useState('');

  const range = useMemo(() => {
    if (preset === 'custom') {
      return { from: customFrom, to: customTo || new Date().toISOString().slice(0, 10) };
    }
    return presetRange(preset);
  }, [preset, customFrom, customTo]);

  const validation = !range.from ? 'Selecione a data inicial.'
    : range.from > range.to ? 'A data final deve ser igual ou posterior à inicial.' : '';
  const loadKpis = useCallback(() => financialReportService.kpis(), []);
  const position = useFinancialRead<KPIs>('position', loadKpis);
  const loadPeriod = useCallback(async () => {
    const [dre, cashflow] = await Promise.all([
      financialReportService.dre(range),
      financialReportService.cashflow(range, granularity),
    ]);
    return { dre, cashflow };
  }, [range, granularity]);
  const period = useFinancialRead<{ dre: DREReport; cashflow: CashFlowReport }>(
    `${range.from}/${range.to}/${granularity}`, loadPeriod, !validation,
  );

  return (
    <div className={embedded ? "space-y-4" : "p-6 max-w-7xl mx-auto space-y-6"}>
      <PageHeader
        as={embedded ? "h2" : "h1"}
        icon={<BarChart3 className="w-6 h-6" />}
        title={embedded ? "Visão geral" : "Dashboard financeiro"}
        description="Posição de caixa, obrigações, resultado econômico e movimentação financeira por período."
      />

      <section className="space-y-3" aria-labelledby="current-position-title">
        <div>
          <h3 id="current-position-title" className="text-sm font-bold text-foreground">Posição atual</h3>
          <p className="text-xs text-muted-foreground">Saldos e pendências no momento da consulta; estes indicadores não mudam com o período abaixo.</p>
        </div>
        {position.error ? <FinanceLoadError message={position.error} onRetry={position.retry} /> : <>
          <KpisCards data={position.data ?? null} loading={position.loading} />
          <FinancialPositionCard data={position.data ?? null} loading={position.loading} />
        </>}
      </section>

      <section className="space-y-3" aria-labelledby="period-performance-title">
        <div>
          <h3 id="period-performance-title" className="text-sm font-bold text-foreground">Desempenho no período</h3>
          <p className="text-xs text-muted-foreground">A DRE considera somente resultado próprio; o fluxo mostra toda movimentação de caixa, inclusive recursos de terceiros. A granularidade afeta somente o gráfico.</p>
        </div>
      <Card className="p-4">
        <div className="flex items-center flex-wrap gap-2">
          {(
            [
              { id: 'month', label: 'Mês atual' },
              { id: 'last3', label: 'Últimos 3 meses' },
              { id: 'year', label: 'Ano atual' },
              { id: 'custom', label: 'Período personalizado' },
            ] as { id: PeriodPreset; label: string }[]
          ).map((p) => (
            <button
              key={p.id}
              type="button"
              aria-pressed={preset === p.id}
              onClick={() => setPreset(p.id)}
              className={`px-3 py-1.5 rounded-full text-xs font-medium transition-colors ${
                preset === p.id
                  ? 'bg-primary text-primary-foreground'
                  : 'bg-muted text-muted-foreground hover:bg-muted/80'
              }`}
            >
              {p.label}
            </button>
          ))}
          {preset === 'custom' && (
            <div className="flex items-center gap-2 ml-2">
              <input
                type="date"
                aria-label="Data inicial"
                value={customFrom}
                onChange={(e) => setCustomFrom(e.target.value)}
                className="px-3 py-1.5 bg-card border border-border rounded-xl text-xs"
              />
              <span className="text-xs text-muted-foreground">até</span>
              <input
                type="date"
                aria-label="Data final"
                min={customFrom || undefined}
                value={customTo}
                onChange={(e) => setCustomTo(e.target.value)}
                className="px-3 py-1.5 bg-card border border-border rounded-xl text-xs"
              />
            </div>
          )}
          <div className="flex-1" />
          <span className="text-xs text-muted-foreground">Granularidade:</span>
          <select
            aria-label="Granularidade"
            value={granularity}
            onChange={(e) => setGranularity(e.target.value as CashFlowGranularity)}
            className="px-3 py-1.5 bg-card border border-border rounded-xl text-xs"
          >
            <option value="day">Diária</option>
            <option value="week">Semanal</option>
            <option value="month">Mensal</option>
          </select>
        </div>
      </Card>

      {validation ? <p role="alert" className="text-sm text-status-critical">{validation}</p> : period.error ? (
        <FinanceLoadError message={period.error} onRetry={period.retry} />
      ) : period.loading ? (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="w-6 h-6 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <DRETable data={period.data?.dre ?? null} />
          <CashflowChart data={period.data?.cashflow ?? null} />
        </div>
      )}
      </section>

      <OwnerStatementPanel owners={leads} />
    </div>
  );
}
