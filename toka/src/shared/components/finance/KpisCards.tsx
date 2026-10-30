import { TrendingUp, TrendingDown, AlertTriangle, FileText, Users, DollarSign } from 'lucide-react';
import { useNavigate } from 'react-router';
import { Card } from '../../ui/card';
import { formatMoneyBR } from '../../utils/format';
import type { KPIs } from '../../services/financialReport.service';

const centsToReais = (v: number) => (typeof v === 'number' ? v / 100 : 0);

const localDateISO = (date: Date) => [
  date.getFullYear(),
  String(date.getMonth() + 1).padStart(2, '0'),
  String(date.getDate()).padStart(2, '0'),
].join('-');

interface KpisCardsProps {
  data: KPIs | null;
  loading?: boolean;
}

export function KpisCards({ data, loading }: KpisCardsProps) {
  const navigate = useNavigate();
  if (loading || !data) {
    return (
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-3">
        {Array.from({ length: 8 }).map((_, i) => (
          <Card key={i} className="p-4 animate-pulse">
            <div className="h-3 w-24 bg-muted rounded mb-3" />
            <div className="h-6 w-32 bg-muted rounded" />
          </Card>
        ))}
      </div>
    );
  }

  const now = new Date();
  const monthStart = localDateISO(new Date(now.getFullYear(), now.getMonth(), 1));
  const today = localDateISO(now);
  const cards = [
    {
      label: 'Receita própria do mês',
      value: formatMoneyBR(centsToReais(data.revenue_month_cents)),
      subtitle: 'Exclui valores de terceiros',
      icon: TrendingUp,
      tone: 'text-status-success',
    },
    {
      label: 'Receita própria em 30d',
      value: formatMoneyBR(centsToReais(data.revenue_last_30_cents)),
      icon: DollarSign,
      tone: 'text-primary-600',
    },
    {
      label: 'A receber',
      value: formatMoneyBR(centsToReais(data.receivable_cents)),
      icon: FileText,
      tone: 'text-status-info',
      to: '/financeiro/lancamentos?kind=charge&status=pendente,aprovado,parcial,vencido&flow=receber',
    },
    {
      label: 'Em atraso',
      value: formatMoneyBR(centsToReais(data.overdue_cents)),
      subtitle: `${data.overdue_count} cobrança${data.overdue_count === 1 ? '' : 's'}`,
      icon: AlertTriangle,
      tone: 'text-status-critical',
      to: '/financeiro/lancamentos?kind=charge&status=vencido',
    },
    {
      label: 'Inadimplência',
      value: `${data.delinquency_pct.toLocaleString('pt-BR', { minimumFractionDigits: 1, maximumFractionDigits: 1 })}%`,
      icon: TrendingDown,
      tone: 'text-status-warning',
    },
    {
      label: 'Receita média realizada',
      value: formatMoneyBR(centsToReais(data.average_ticket_cents)),
      icon: DollarSign,
      tone: 'text-foreground',
    },
    {
      label: 'Locações ativas',
      value: String(data.active_contract_count),
      icon: Users,
      tone: 'text-foreground',
    },
    {
      label: 'Comissões distribuídas (mês)',
      value: formatMoneyBR(centsToReais(data.commission_paid_month_cents)),
      subtitle: 'Movimentação destinada aos corretores',
      icon: DollarSign,
      tone: 'text-primary-600',
      to: `/financeiro/lancamentos?kind=commission&status=pago&from=${monthStart}&to=${today}&date_basis=settled`,
    },
  ];

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-3">
      {cards.map(({ label, value, subtitle, icon: Icon, tone, to }) => {
        const clickable = Boolean(to);
        return (
          <Card
            key={label}
            onClick={clickable ? () => navigate(to as string) : undefined}
            role={clickable ? 'button' : undefined}
            tabIndex={clickable ? 0 : undefined}
            onKeyDown={clickable ? (e) => { if (e.key === 'Enter') navigate(to as string); } : undefined}
            className={`p-4 transition-colors ${clickable ? 'cursor-pointer hover:border-primary-400 hover:bg-primary-50/30' : 'hover:border-border/80'}`}
          >
            <div className="flex items-start justify-between mb-2">
              <p className="text-xs text-muted-foreground font-medium uppercase tracking-wide">
                {label}
              </p>
              <Icon className={`w-4 h-4 ${tone}`} />
            </div>
            <p className="text-xl font-bold text-foreground">{value}</p>
            {subtitle && <p className="text-xs text-muted-foreground mt-1">{subtitle}</p>}
          </Card>
        );
      })}
    </div>
  );
}
