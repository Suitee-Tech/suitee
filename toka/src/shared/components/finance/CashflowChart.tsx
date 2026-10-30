import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid, Legend } from 'recharts';
import { Card } from '../../ui/card';
import { formatMoneyBR } from '../../utils/format';
import type { CashFlowReport } from '../../services/financialReport.service';

const centsToReais = (v: number) => (typeof v === 'number' ? v / 100 : 0);

interface CashflowChartProps {
  data: CashFlowReport | null;
  loading?: boolean;
}

export function CashflowChart({ data, loading }: CashflowChartProps) {
  if (loading || !data) {
    return (
      <Card className="p-6">
        <div className="h-4 w-32 bg-muted rounded mb-4 animate-pulse" />
        <div className="h-64 bg-muted/30 rounded animate-pulse" />
      </Card>
    );
  }

  const chartData = data.entries.map((e) => ({
    period: e.period,
    Entradas: centsToReais(e.inflow_cents),
    Saídas: centsToReais(e.outflow_cents),
  }));
  const totalInflow = data.entries.reduce((sum, entry) => sum + entry.inflow_cents, 0);
  const totalOutflow = data.entries.reduce((sum, entry) => sum + entry.outflow_cents, 0);
  const netCash = totalInflow - totalOutflow;
  const granularityLabel = ({ day: 'Diária', week: 'Semanal', month: 'Mensal' } as const)[data.granularity];

  return (
    <Card className="p-6">
      <div className="flex flex-wrap items-start justify-between gap-2 mb-4">
        <div>
          <h3 className="text-base font-bold text-foreground">Movimentação de caixa</h3>
          <p className="mt-0.5 text-xs text-muted-foreground">Visão bruta: inclui dinheiro próprio e valores administrados de terceiros.</p>
        </div>
        <span className="rounded-full bg-muted px-2.5 py-1 text-xs text-muted-foreground">
          {granularityLabel}
        </span>
      </div>
      <div className="mb-4 grid grid-cols-1 gap-2 sm:grid-cols-3">
        <CashSummary label="Entradas brutas" cents={totalInflow} tone="success" />
        <CashSummary label="Saídas brutas" cents={totalOutflow} tone="critical" />
        <CashSummary label="Variação de caixa" cents={netCash} tone={netCash >= 0 ? 'default' : 'critical'} signed />
      </div>
      <div className="h-64">
        {chartData.length === 0 ? (
          <div className="h-full flex items-center justify-center text-sm text-muted-foreground">
            Sem movimentação no período selecionado.
          </div>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
              <XAxis dataKey="period" tick={{ fontSize: 11 }} />
              <YAxis
                tick={{ fontSize: 11 }}
                tickFormatter={(v) => formatMoneyBR(v, { withSymbol: false, fractionDigits: 0 })}
              />
              <Tooltip
                formatter={(value) => formatMoneyBR(Number(value))}
                labelStyle={{ fontWeight: 'bold' }}
                contentStyle={{ fontSize: '12px' }}
              />
              <Legend wrapperStyle={{ fontSize: '12px' }} />
              <Bar dataKey="Entradas" fill="#16a34a" />
              <Bar dataKey="Saídas" fill="#dc2626" />
            </BarChart>
          </ResponsiveContainer>
        )}
      </div>
    </Card>
  );
}

function CashSummary({
  label,
  cents,
  tone,
  signed = false,
}: {
  label: string;
  cents: number;
  tone: 'success' | 'critical' | 'default';
  signed?: boolean;
}) {
  const toneClass = tone === 'success'
    ? 'text-status-success'
    : tone === 'critical'
      ? 'text-status-critical'
      : 'text-foreground';
  const prefix = signed && cents > 0 ? '+' : '';
  return (
    <div className="rounded-xl border border-border bg-muted/20 px-3 py-2">
      <p className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">{label}</p>
      <p className={`mt-0.5 text-sm font-bold ${toneClass}`}>{prefix}{formatMoneyBR(centsToReais(cents))}</p>
    </div>
  );
}
