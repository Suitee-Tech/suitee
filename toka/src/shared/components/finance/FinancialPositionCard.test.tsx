import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';

import type { KPIs } from '../../services/financialReport.service';
import { FinancialPositionCard } from './FinancialPositionCard';

const basePosition: KPIs = {
  tenant_id: 'tenant-1',
  generated_at: '2026-08-31T12:00:00Z',
  revenue_month_cents: 2_352_000,
  revenue_last_30_cents: 2_352_000,
  receivable_cents: 0,
  overdue_cents: 0,
  overdue_count: 0,
  delinquency_pct: 0,
  average_ticket_cents: 2_352_000,
  active_contract_count: 1,
  commission_paid_month_cents: 3_528_000,
  cash_balance_cents: 2_367_000,
  owner_payable_cents: 0,
  commission_payable_cents: 0,
  partner_payable_cents: 0,
  royalty_payable_cents: 0,
  tax_withheld_cents: 10_000,
  other_payable_cents: 5_000,
  withdrawal_payable_cents: 0,
  third_party_obligations_cents: 0,
  other_commitments_cents: 15_000,
  committed_funds_cents: 15_000,
  agency_available_cash_cents: 2_352_000,
  coverage_gap_cents: 0,
  unexpected_payable_debit_cents: 0,
};

function renderPosition(data: KPIs) {
  render(
    <MemoryRouter>
      <FinancialPositionCard data={data} />
    </MemoryRouter>,
  );
}

describe('FinancialPositionCard', () => {
  it('separa caixa, compromissos e disponibilidade da imobiliária', () => {
    renderPosition(basePosition);

    expect(screen.getByRole('heading', { name: 'Posição de caixa e obrigações' })).toBeInTheDocument();
    expect(screen.getByText('Obrigações cobertas')).toBeInTheDocument();
    expect(screen.getByText(/R\$\s*23\.670,00/)).toBeInTheDocument();
    expect(screen.getAllByText(/R\$\s*150,00/)).toHaveLength(2);
    expect(screen.getByText(/R\$\s*23\.520,00/)).toBeInTheDocument();
    expect(screen.getByText(/0\.6% do caixa comprometido/)).toBeInTheDocument();
    expect(screen.getByRole('progressbar', { name: 'Parcela do caixa comprometida' })).toHaveAttribute('aria-valuenow', '1');
    expect(screen.getByRole('button', { name: /Ver a pagar/ })).toBeInTheDocument();
    expect(screen.getByText(/Não representa saldo bancário conciliado/)).toBeInTheDocument();
  });

  it('destaca falta de cobertura sem apresentar saldo livre negativo', () => {
    renderPosition({
      ...basePosition,
      cash_balance_cents: 10_000,
      owner_payable_cents: 12_000,
      commission_payable_cents: 3_000,
      tax_withheld_cents: 2_500,
      other_payable_cents: 0,
      third_party_obligations_cents: 15_000,
      other_commitments_cents: 2_500,
      committed_funds_cents: 17_500,
      agency_available_cash_cents: 0,
      coverage_gap_cents: 7_500,
      unexpected_payable_debit_cents: 300,
    });

    expect(screen.getByText('Cobertura insuficiente')).toBeInTheDocument();
    expect(screen.getAllByText(/R\$\s*0,00/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/R\$\s*75,00/).length).toBeGreaterThanOrEqual(2);
    expect(screen.getByText(/R\$\s*3,00 em saldos devedores inesperados/)).toBeInTheDocument();
  });
});
