import { AlertTriangle, ArrowRight, ShieldCheck, Wallet } from 'lucide-react';
import { useNavigate } from 'react-router';

import type { KPIs } from '../../services/financialReport.service';
import { Card } from '../../ui/card';
import { formatMoneyBR } from '../../utils/format';

const centsToReais = (cents: number) => (Number.isFinite(cents) ? cents / 100 : 0);

interface FinancialPositionCardProps {
  data: KPIs | null;
  loading?: boolean;
}

export function FinancialPositionCard({ data, loading }: FinancialPositionCardProps) {
  const navigate = useNavigate();

  if (loading || !data) {
    return (
      <Card className="p-5 animate-pulse" aria-label="Carregando posição de caixa e obrigações">
        <div className="h-5 w-64 rounded bg-muted" />
        <div className="mt-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, index) => (
            <div key={index} className="h-20 rounded-xl bg-muted" />
          ))}
        </div>
      </Card>
    );
  }

  const committedRatio = data.cash_balance_cents > 0
    ? (data.committed_funds_cents / data.cash_balance_cents) * 100
    : data.committed_funds_cents > 0
      ? 100
      : 0;
  const progressValue = Math.min(Math.max(committedRatio, 0), 100);
  const partnerObligations = data.partner_payable_cents + data.royalty_payable_cents;
  const retainedObligations = data.tax_withheld_cents + data.other_payable_cents;
  const hasCoverageGap = data.coverage_gap_cents > 0;
  const hasCommitments = data.committed_funds_cents > 0;

  return (
    <Card className="p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary-50 text-primary-700">
            <Wallet className="h-5 w-5" aria-hidden="true" />
          </div>
          <div>
            <h3 className="text-base font-bold text-foreground">Posição de caixa e obrigações</h3>
            <p className="mt-0.5 text-xs text-muted-foreground">
              Separa o caixa contabilizado entre valores comprometidos e disponibilidade da imobiliária.
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <span
            className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-semibold ${
              hasCoverageGap
                ? 'bg-status-critical-soft text-status-critical'
                : 'bg-status-success-soft text-status-success'
            }`}
          >
            {hasCoverageGap ? (
              <AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />
            ) : (
              <ShieldCheck className="h-3.5 w-3.5" aria-hidden="true" />
            )}
            {hasCoverageGap ? 'Cobertura insuficiente' : hasCommitments ? 'Obrigações cobertas' : 'Sem obrigações abertas'}
          </span>
          <button
            type="button"
            onClick={() => navigate('/financeiro/lancamentos?flow=pagar&status=pendente,aprovado,parcial,vencido')}
            className="inline-flex items-center gap-1 rounded-lg px-2.5 py-1.5 text-xs font-semibold text-primary-700 hover:bg-primary-50"
          >
            Ver a pagar
            <ArrowRight className="h-3.5 w-3.5" aria-hidden="true" />
          </button>
        </div>
      </div>

      <div className="mt-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <PositionMetric label="Caixa contábil" cents={data.cash_balance_cents} />
        <PositionMetric label="Recursos comprometidos" cents={data.committed_funds_cents} tone={hasCoverageGap ? 'critical' : 'warning'} />
        <PositionMetric label="Livre da imobiliária" cents={data.agency_available_cash_cents} tone="success" emphasis />
        <PositionMetric
          label={hasCoverageGap ? 'Falta para cobertura' : 'Recursos de terceiros'}
          cents={hasCoverageGap ? data.coverage_gap_cents : data.third_party_obligations_cents}
          tone={hasCoverageGap ? 'critical' : 'default'}
        />
      </div>

      {hasCommitments && (
        <div className="mt-4 rounded-xl border border-border bg-muted/20 p-3">
          <div className="flex items-center justify-between gap-3 text-xs">
            <span className="font-medium text-foreground">Cobertura das obrigações</span>
            <span className={hasCoverageGap ? 'font-semibold text-status-critical' : 'text-muted-foreground'}>
              {hasCoverageGap
                ? `${formatMoneyBR(centsToReais(data.coverage_gap_cents))} sem cobertura`
                : `${committedRatio.toFixed(1)}% do caixa comprometido`}
            </span>
          </div>
          <div
            className="mt-2 h-2 overflow-hidden rounded-full bg-muted"
            role="progressbar"
            aria-label="Parcela do caixa comprometida"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={Math.round(progressValue)}
          >
            <div
              className={`h-full rounded-full ${hasCoverageGap ? 'bg-status-critical' : 'bg-status-warning'}`}
              style={{ width: `${progressValue}%` }}
            />
          </div>
        </div>
      )}

      <div className="mt-4 grid grid-cols-2 gap-x-5 gap-y-2 border-t border-border pt-4 text-xs md:grid-cols-5">
        <Breakdown label="Proprietários" cents={data.owner_payable_cents} />
        <Breakdown label="Corretores" cents={data.commission_payable_cents} />
        <Breakdown label="Parceiros e royalties" cents={partnerObligations} />
        <Breakdown label="Tributos e retenções" cents={retainedObligations} />
        <Breakdown label="Saques autorizados" cents={data.withdrawal_payable_cents} />
      </div>

      {data.unexpected_payable_debit_cents > 0 && (
        <div className="mt-4 flex gap-2 rounded-xl border border-status-critical/30 bg-status-critical-soft p-3 text-xs text-status-critical">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <p>
            Há {formatMoneyBR(centsToReais(data.unexpected_payable_debit_cents))} em saldos devedores inesperados nas contas a pagar. Revise a conciliação antes de movimentar o caixa livre.
          </p>
        </div>
      )}

      <p className="mt-4 text-[11px] leading-relaxed text-muted-foreground">
        Posição derivada do journal auditável. Não representa saldo bancário conciliado enquanto não houver integração com extrato ou PSP.
      </p>
    </Card>
  );
}

function PositionMetric({
  label,
  cents,
  tone = 'default',
  emphasis = false,
}: {
  label: string;
  cents: number;
  tone?: 'default' | 'success' | 'warning' | 'critical';
  emphasis?: boolean;
}) {
  const toneClass = tone === 'success'
    ? 'text-status-success'
    : tone === 'warning'
      ? 'text-status-warning'
      : tone === 'critical'
        ? 'text-status-critical'
        : 'text-foreground';
  return (
    <div className={`rounded-xl border px-3 py-3 ${emphasis ? 'border-status-success/30 bg-status-success-soft/40' : 'border-border bg-card'}`}>
      <p className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">{label}</p>
      <p className={`mt-1 text-lg font-bold ${toneClass}`}>{formatMoneyBR(centsToReais(cents))}</p>
    </div>
  );
}

function Breakdown({ label, cents }: { label: string; cents: number }) {
  return (
    <div className="min-w-0">
      <p className="truncate text-muted-foreground">{label}</p>
      <p className="mt-0.5 font-semibold text-foreground">{formatMoneyBR(centsToReais(cents))}</p>
    </div>
  );
}
