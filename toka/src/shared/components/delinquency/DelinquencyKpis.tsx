import { AlertCircle, Phone, Shield, Gavel, TrendingUp, Clock, RefreshCw } from 'lucide-react';
import type { DelinquencyDashboard } from '../../services/delinquency.service';
import { Button } from '../../ui/button';
import { formatCurrencyBRL } from '../../utils/format';
import { formatMetric, formatPercent } from './delinquencyUi';

/**
 * KPIs de inadimplência. TODOS os números vêm de `GET /delinquency/dashboard`
 * (stats + collection_stats + aging_report) — nenhum é somado a partir da lista
 * paginada, então busca e troca de página não mexem no valor exibido.
 * Métrica ausente vira "—": a tela nunca inventa zero.
 */
export function DelinquencyKpis({
  dashboard,
  loading,
  error,
  onReload,
}: {
  dashboard: DelinquencyDashboard;
  loading: boolean;
  error: string | null;
  onReload: () => void;
}) {
  const stats = dashboard.stats;
  const collectionStats = dashboard.collection_stats;
  const aging = dashboard.aging_report ?? [];

  const cards = [
    {
      label: 'Total em Atraso',
      icon: AlertCircle,
      value: formatMetric(stats?.total_debt_amount, true),
      hint: `${formatMetric(stats?.total_overdue)} cobrança(s) em atraso`,
    },
    {
      label: 'Em Cobrança',
      icon: Phone,
      value: formatMetric(collectionStats?.active),
      hint: `${formatMetric(stats?.average_days_overdue)} dias em média`,
    },
    {
      label: 'Garantias Acionadas',
      icon: Shield,
      value: formatMetric(collectionStats?.guarantee_activated),
      hint: `${formatMetric(collectionStats?.negotiated)} negociado(s)`,
    },
    {
      label: 'Processos Judiciais',
      icon: Gavel,
      value: formatMetric(collectionStats?.judicial),
      hint: `${formatMetric(collectionStats?.resolved)} resolvido(s)`,
    },
  ];

  return (
    <section aria-label="Indicadores de inadimplência" className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <TrendingUp className="w-4 h-4" />
          <span>
            Recuperação: <strong className="text-foreground">{formatPercent(stats?.recovery_rate)}</strong>
            {stats ? ` · Ticket médio ${formatCurrencyBRL(stats.average_debt_amount)}` : ''}
          </span>
        </div>
        <div className="flex items-center gap-2">
          {error && (
            <span className="text-sm text-status-critical" role="status">
              {error}
            </span>
          )}
          <Button variant="outline" size="sm" onClick={onReload} loading={loading} loadingText="Atualizando...">
            <RefreshCw className="w-4 h-4" /> Atualizar indicadores
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
        {cards.map((card) => (
          <div key={card.label} className="bg-card p-6 rounded-2xl border border-border shadow-sm flex flex-col gap-4">
            <div className="flex items-center gap-3">
              <div className="w-8 h-8 rounded-full bg-muted flex items-center justify-center">
                <card.icon className="w-4 h-4 text-muted-foreground" />
              </div>
              <span className="text-sm font-medium text-muted-foreground">{card.label}</span>
            </div>
            <h2 className="text-3xl font-bold text-foreground">{loading ? '…' : card.value}</h2>
            <p className="text-xs text-muted-foreground">{card.hint}</p>
          </div>
        ))}
      </div>

      <div className="bg-card rounded-2xl border border-border shadow-sm p-6">
        <div className="flex items-center gap-2 mb-4">
          <Clock className="w-4 h-4 text-muted-foreground" />
          <h3 className="text-sm font-bold text-foreground">Aging da carteira em atraso</h3>
        </div>
        {aging.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {loading ? 'Carregando faixas…' : 'Sem faixas de atraso retornadas pelo servidor.'}
          </p>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            {aging.map((bucket) => (
              <div key={`${bucket.label}-${bucket.min_days}`} className="bg-muted/60 border border-border rounded-xl p-4">
                <p className="text-xs font-medium text-muted-foreground">
                  {bucket.label} dias
                </p>
                <p className="text-xl font-bold text-foreground mt-1">{formatMetric(bucket.count)}</p>
                <p className="text-xs text-muted-foreground mt-1">{formatCurrencyBRL(bucket.total_debt)}</p>
              </div>
            ))}
          </div>
        )}
      </div>
    </section>
  );
}
