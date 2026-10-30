import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { financialReportService, type ChartOfAccounts } from '../../services/financialReport.service';
import { ChartOfAccountsSection } from './ChartOfAccountsSection';

vi.mock('../../services/financialReport.service', () => ({
  financialReportService: { chartOfAccounts: vi.fn() },
}));

const catalogue: ChartOfAccounts = {
  version: '2026-09',
  accounts: [
    { code: '1.01', name: 'Caixa', nature: 'asset', ledger_account: 'cash', flows: ['sale', 'rent'], description: 'Recebimentos conciliados.' },
    { code: '2.01', name: 'Vendedores a pagar', nature: 'liability', ledger_account: 'owner_payable', flows: ['sale'], description: 'Obrigação da venda.' },
    { code: '3.02', name: 'Administração de locação', nature: 'revenue', ledger_account: 'revenue', flows: ['rent'], description: 'Taxa mensal retida.' },
  ],
};

beforeEach(() => {
  vi.mocked(financialReportService.chartOfAccounts).mockReset().mockResolvedValue(catalogue);
});

describe('ChartOfAccountsSection', () => {
  it('exibe carregamento até receber o catálogo e apresenta sua versão', async () => {
    let resolve!: (value: ChartOfAccounts) => void;
    vi.mocked(financialReportService.chartOfAccounts).mockReturnValue(new Promise((done) => { resolve = done; }));
    render(<ChartOfAccountsSection />);

    expect(screen.getByRole('status')).toHaveTextContent('Carregando plano de contas');
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
    resolve(catalogue);

    expect(await screen.findByRole('table')).toHaveAccessibleName('Contas financeiras aplicáveis a vendas e locações');
    expect(screen.getByText('3 de 3 contas · Catálogo versão 2026-09')).toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('combina modalidade com busca sem acentos e permite recuperar todos os resultados', async () => {
    const user = userEvent.setup();
    render(<ChartOfAccountsSection />);
    await screen.findByRole('table');
    const search = screen.getByRole('textbox', { name: 'Buscar conta por código ou nome' });

    await user.click(screen.getByRole('button', { name: 'Vendas' }));
    expect(screen.getByRole('button', { name: 'Vendas' })).toHaveAttribute('aria-pressed', 'true');
    expect(within(screen.getByRole('table')).queryByText('Administração de locação')).not.toBeInTheDocument();
    await user.type(search, 'administracao');
    expect(screen.getByText('Nenhuma conta encontrada para os filtros selecionados.')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Locações' }));
    expect(screen.getByRole('table')).toHaveAccessibleName('Contas financeiras aplicáveis a Locação');
    expect(within(screen.getByRole('table')).getByText('Administração de locação')).toBeInTheDocument();
    expect(screen.getByText('1 de 3 contas · Catálogo versão 2026-09')).toBeInTheDocument();

    await user.clear(search);
    await user.type(search, '1.01');
    expect(within(screen.getByRole('table')).getByText('Caixa')).toBeInTheDocument();
    await user.clear(search);
    await user.click(screen.getByRole('button', { name: 'Todas' }));
    expect(screen.getByText('3 de 3 contas · Catálogo versão 2026-09')).toBeInTheDocument();
    expect(financialReportService.chartOfAccounts).toHaveBeenCalledTimes(1);
  });

  it('informa falha sem apresentar catálogo vazio e refaz a consulta pelo botão de nova tentativa', async () => {
    const user = userEvent.setup();
    vi.mocked(financialReportService.chartOfAccounts).mockRejectedValueOnce(new Error('offline'));
    render(<ChartOfAccountsSection />);

    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível carregar o plano de contas.');
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Tentar novamente' }));

    expect(await screen.findByRole('table')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(financialReportService.chartOfAccounts).toHaveBeenCalledTimes(2);
  });
});
