import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router';
import { tenantFinancialSettingsService, type TenantFinancialSettings } from '../services/tenantFinancialSettings.service';
import { commissionRulesService } from '../services/commissionRules.service';
import { financialReportService } from '../services/financialReport.service';
import FinancialSettings from './FinancialSettings';

vi.mock('../services/tenantFinancialSettings.service', () => ({ tenantFinancialSettingsService: { get: vi.fn(), update: vi.fn() } }));
vi.mock('../services/commissionRules.service', () => ({ commissionRulesService: { get: vi.fn(), update: vi.fn() } }));
vi.mock('../services/financialReport.service', () => ({ financialReportService: { chartOfAccounts: vi.fn() } }));
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

const settings: TenantFinancialSettings = {
  tenant_id: 'tenant-1', default_late_fee_pct: 2, default_monthly_interest_pct: 1,
  default_due_day: 10, default_commission_pct_rent: 100, default_commission_pct_sale: 6,
  default_currency: 'BRL', payout_proof_required: true, admin_fee_pct: 10,
  default_lease_term_months: 30, default_inss_pct: 20, default_iss_pct: 2.5, updated_at: '',
};

function Location() {
  const location = useLocation();
  return <output aria-label="Localização atual">{location.pathname}{location.search}</output>;
}

function renderSettings(url = '/financial-settings') {
  render(<MemoryRouter initialEntries={[url]}><FinancialSettings /><Location /></MemoryRouter>);
}

beforeEach(() => {
  vi.mocked(tenantFinancialSettingsService.get).mockReset().mockResolvedValue(settings);
  vi.mocked(tenantFinancialSettingsService.update).mockReset();
  vi.mocked(commissionRulesService.get).mockReset().mockResolvedValue([]);
  vi.mocked(financialReportService.chartOfAccounts).mockReset().mockResolvedValue({ version: '2026-09', accounts: [] });
});

afterEach(() => { vi.restoreAllMocks(); });

describe('FinancialSettings navigation', () => {
  it('abre o plano de contas diretamente pela URL mesmo quando configurações falham', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.mocked(tenantFinancialSettingsService.get).mockRejectedValue(new Error('offline'));
    renderSettings('/financial-settings?tab=accounts');

    expect(await screen.findByRole('table')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Plano de contas' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.queryByRole('button', { name: 'Salvar alterações' })).not.toBeInTheDocument();
    expect(screen.getByText(/são despesas e obrigações até o pagamento/i)).toBeInTheDocument();
  });

  it('alcança o catálogo pela seção de configurações e mantém os demais parâmetros da URL', async () => {
    const user = userEvent.setup();
    renderSettings('/financial-settings?origin=finance');
    await screen.findByRole('button', { name: 'Salvar alterações' });
    await user.click(screen.getByRole('button', { name: 'Plano de contas' }));

    expect(await screen.findByRole('table')).toBeInTheDocument();
    expect(screen.getByLabelText('Localização atual')).toHaveTextContent('/financial-settings?origin=finance&tab=accounts');
    expect(screen.queryByRole('button', { name: 'Salvar alterações' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Repasse' }));
    expect(screen.getByRole('checkbox')).toBeChecked();
    expect(screen.getByText(/Aprovar e confirmar o pagamento continua sendo uma ação do responsável financeiro/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Salvar alterações' })).toBeInTheDocument();
  });

  it('recupera configurações após uma falha e permite abrir o catálogo enquanto a falha está visível', async () => {
    const user = userEvent.setup();
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.mocked(tenantFinancialSettingsService.get).mockRejectedValueOnce(new Error('offline'));
    renderSettings();
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível carregar as configurações financeiras.');
    expect(screen.queryByRole('button', { name: 'Salvar alterações' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Plano de contas' }));
    expect(await screen.findByRole('table')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Valores padrão' }));
    await user.click(screen.getByRole('button', { name: 'Tentar novamente' }));

    await waitFor(() => expect(screen.getByRole('button', { name: 'Salvar alterações' })).toBeEnabled());
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(tenantFinancialSettingsService.get).toHaveBeenCalledTimes(2);
  });
});

/**
 * O prazo padrão da locação é configurável por imobiliária (Padrões) e alimenta
 * a sugestão do plano pré-contrato — que calcula o término e o residual do
 * último aluguel a partir dele.
 */
describe('FinancialSettings — prazo padrão da locação', () => {
  it('mostra o prazo padrão vindo das configurações e salva o valor editado', async () => {
    const user = userEvent.setup();
    const update = vi.mocked(tenantFinancialSettingsService.update).mockResolvedValue({ ...settings, default_lease_term_months: 24 });
    renderSettings();

    const term = await screen.findByLabelText(/Prazo padrão da locação/i);
    expect(term).toHaveValue(30);

    await user.clear(term);
    await user.type(term, '24');
    await user.click(screen.getByRole('button', { name: 'Salvar alterações' }));

    await waitFor(() => expect(update).toHaveBeenCalled());
    expect(update.mock.calls[0][0]).toMatchObject({ default_lease_term_months: 24 });
  });
});

/**
 * As alíquotas padrão de INSS (autônomo) e ISS são o que o lançamento de
 * desconto na comissão pré-preenche. Elas são alíquotas: 0 desliga a retenção e
 * o backend recusa qualquer valor fora de 0–100.
 */
describe('FinancialSettings — retenções de INSS e ISS', () => {
  it('mostra as alíquotas configuradas e salva as duas junto com os demais padrões', async () => {
    const user = userEvent.setup();
    const update = vi
      .mocked(tenantFinancialSettingsService.update)
      .mockResolvedValue({ ...settings, default_inss_pct: 11, default_iss_pct: 3 });
    renderSettings();

    const inss = await screen.findByLabelText('INSS retido do autônomo (%)');
    expect(inss).toHaveValue(20);
    expect(screen.getByLabelText('ISS retido (%)')).toHaveValue(2.5);
    expect(screen.getByText(/0 = não retém\. Corretor autônomo/)).toBeInTheDocument();
    expect(screen.getByText(/ISS varia por município, tipicamente 2% a 5%\. 0 = não retém\./)).toBeInTheDocument();

    await user.clear(inss);
    await user.type(inss, '11');
    await user.click(screen.getByRole('button', { name: 'Salvar alterações' }));

    await waitFor(() => expect(update).toHaveBeenCalled());
    expect(update.mock.calls[0][0]).toMatchObject({ default_inss_pct: 11, default_iss_pct: 2.5 });
  });

  it('recusa alíquota fora de 0–100, explica no campo e não salva', async () => {
    const user = userEvent.setup();
    const update = vi.mocked(tenantFinancialSettingsService.update);
    renderSettings();

    const iss = await screen.findByLabelText('ISS retido (%)');
    await user.clear(iss);
    await user.type(iss, '150');

    expect(await screen.findByRole('alert')).toHaveTextContent('Informe uma alíquota entre 0 e 100.');
    expect(iss).toHaveAttribute('aria-invalid', 'true');
    expect(screen.getByRole('button', { name: 'Salvar alterações' })).toBeDisabled();
    expect(update).not.toHaveBeenCalled();
  });
});
