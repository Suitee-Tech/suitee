import { ChevronRight, Info } from 'lucide-react';
import { useState } from 'react';
import { Card } from '../../ui/card';
import { formatMoneyBR, formatISODateBR } from '../../utils/format';
import type { DREReport } from '../../services/financialReport.service';

const centsToReais = (v: number) => (typeof v === 'number' ? v / 100 : 0);

interface DRETableProps {
  data: DREReport | null;
  loading?: boolean;
}

const REVENUE_LABELS: Record<string, string> = {
  RENT: 'Serviços de locação',
  SALE: 'Intermediação de vendas',
  SALE_AGENCY_RETAINED: 'Corretagem retida na venda',
  SALE_INTERMEDIATION: 'Intermediação de vendas',
  RENT_INTERMEDIATION: 'Intermediação e administração de locações',
  RENT_AGENCY_RETAINED: 'Taxa retida na locação',
  RECEITA_MANUAL: 'Receita manual',
  'RECEITA MANUAL': 'Receita manual',
  CONDOMINIO: 'Condomínio',
  IPTU: 'IPTU',
  UTILITY: 'Utilidades (água, luz, gás)',
  MAINTENANCE: 'Manutenção',
  OTHER: 'Outros',
};

const EXPENSE_LABELS: Record<string, string> = {
  Comissão: 'Comissão',
  'Comissão Parceria': 'Comissão de parceria',
  Repasse: 'Repasse ao proprietário',
  Manutenção: 'Manutenção',
  OUTROS: 'Outros',
};

export function DRETable({ data, loading }: DRETableProps) {
  const [openSections, setOpenSections] = useState<Record<string, boolean>>({
    revenue: true,
    expense: true,
  });

  if (loading || !data) {
    return (
      <Card className="p-6">
        <div className="h-4 w-32 bg-muted rounded mb-4 animate-pulse" />
        <div className="space-y-2">
          {Array.from({ length: 6 }).map((_, i) => (
            <div key={i} className="h-6 bg-muted/50 rounded animate-pulse" />
          ))}
        </div>
      </Card>
    );
  }

  const revenueTotal = data.revenue.reduce((acc, r) => acc + r.total_cents, 0);
  const expenseTotal = data.expense.reduce((acc, r) => acc + r.total_cents, 0);

  const toggle = (key: string) => setOpenSections((prev) => ({ ...prev, [key]: !prev[key] }));

  return (
    <Card className="p-6">
      <div className="flex flex-wrap items-start justify-between gap-2 mb-3">
        <div>
          <h3 className="text-base font-bold text-foreground">Resultado econômico da imobiliária</h3>
          <p className="mt-0.5 text-xs text-muted-foreground">Reconhece somente valores que pertencem economicamente à empresa.</p>
        </div>
        <span className="rounded-full bg-muted px-2.5 py-1 text-xs text-muted-foreground">
          {formatISODateBR(data.from)} → {formatISODateBR(data.to)}
        </span>
      </div>

      <div className="mb-4 flex gap-2 rounded-xl border border-status-info/20 bg-status-info-soft p-3 text-xs text-muted-foreground">
        <Info className="mt-0.5 h-4 w-4 shrink-0 text-status-info" aria-hidden="true" />
        <p>Repasses a proprietários e parceiros liquidam obrigações de terceiros. Comissões suportadas pela imobiliária são despesas próprias; o pagamento baixa a obrigação provisionada. Este resultado considera os recebimentos e pagamentos do período.</p>
      </div>

      {!!data.unclassified_receipts_cents && (
        <p role="status" className="mb-4 rounded-xl border border-border p-3 text-sm text-muted-foreground">
          Recebimentos a classificar: {formatMoneyBR(centsToReais(data.unclassified_receipts_cents))}. Esses valores estão no fluxo de caixa e ficam fora da receita própria até a identificação de seus beneficiários.
        </p>
      )}

      <div className="space-y-1">
        <button
          type="button"
          onClick={() => toggle('revenue')}
          className="w-full flex items-center justify-between py-2 hover:bg-muted/50 rounded px-2 transition-colors"
        >
          <div className="flex items-center gap-2">
            <ChevronRight
              className={`w-4 h-4 transition-transform ${openSections.revenue ? 'rotate-90' : ''}`}
            />
            <span className="font-semibold text-sm">Receita própria</span>
          </div>
          <span className="font-bold text-status-success">
            {formatMoneyBR(centsToReais(revenueTotal))}
          </span>
        </button>
        {openSections.revenue && (
          <div className="pl-8 py-1 space-y-1">
            {data.revenue.length === 0 ? (
              <p className="text-xs text-muted-foreground py-2">Nenhuma receita no período.</p>
            ) : (
              data.revenue.map((r) => (
                <div key={r.category} className="flex items-center justify-between text-sm py-1">
                  <span className="text-foreground">
                    {REVENUE_LABELS[r.category] || r.category}
                    <span className="text-muted-foreground ml-2 text-xs">({r.count})</span>
                  </span>
                  <span className="text-foreground">{formatMoneyBR(centsToReais(r.total_cents))}</span>
                </div>
              ))
            )}
          </div>
        )}

        <button
          type="button"
          onClick={() => toggle('expense')}
          className="w-full flex items-center justify-between py-2 hover:bg-muted/50 rounded px-2 transition-colors mt-2"
        >
          <div className="flex items-center gap-2">
            <ChevronRight
              className={`w-4 h-4 transition-transform ${openSections.expense ? 'rotate-90' : ''}`}
            />
            <span className="font-semibold text-sm">Despesas próprias</span>
          </div>
          <span className={`font-bold ${expenseTotal > 0 ? 'text-status-critical' : 'text-muted-foreground'}`}>
            {expenseTotal > 0 ? '-' : ''}{formatMoneyBR(centsToReais(expenseTotal))}
          </span>
        </button>
        {openSections.expense && (
          <div className="pl-8 py-1 space-y-1">
            {data.expense.length === 0 ? (
              <p className="text-xs text-muted-foreground py-2">Nenhuma despesa no período.</p>
            ) : (
              data.expense.map((r) => (
                <div key={r.category} className="flex items-center justify-between text-sm py-1">
                  <span className="text-foreground">
                    {EXPENSE_LABELS[r.category] || r.category}
                    <span className="text-muted-foreground ml-2 text-xs">({r.count})</span>
                  </span>
                  <span className="text-foreground">
                    -{formatMoneyBR(centsToReais(r.total_cents))}
                  </span>
                </div>
              ))
            )}
          </div>
        )}

        <div className="border-t border-border mt-3 pt-3 flex items-center justify-between px-2">
          <span className="font-bold text-base">Resultado da imobiliária</span>
          <span
            className={`font-bold text-base ${
              data.net_cents >= 0 ? 'text-status-success' : 'text-status-critical'
            }`}
          >
            {data.net_cents < 0 ? '-' : ''}
            {formatMoneyBR(Math.abs(centsToReais(data.net_cents)))}
          </span>
        </div>
      </div>
    </Card>
  );
}
