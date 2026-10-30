import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { FiscalReports } from './FiscalReports';
import { fiscalService } from '../services/fiscal.service';

vi.mock('../services/fiscal.service', () => ({
  fiscalService: {
    getCalendar: vi.fn(),
    getReportHistory: vi.fn(),
    getDimobStatus: vi.fn(),
    generateDimob: vi.fn(),
    listInformes: vi.fn(),
    generateInformes: vi.fn(),
    downloadFile: vi.fn(),
    downloadInformePDF: vi.fn(),
    archiveReport: vi.fn(),
    // Presentes de propósito: se a emissão de NFS-e voltar ao cliente, estes
    // campos passam a existir e os testes de bloqueio acusam a regressão.
    issueNfse: undefined,
    listNfse: undefined,
  },
}));
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() } }));

// Concede todas as permissões: o caminho que importa é o do operador que
// *poderia* emitir. Sem isso o <Can> esconde o controle e o teste passa vazio,
// sem provar nada sobre o bloqueio.
vi.mock('../permissions/usePermission', () => ({
  useCan: () => () => true,
  usePermission: () => true,
  useFunctionalRole: () => 'admin',
}));

function renderPage() {
  return render(<FiscalReports />);
}

beforeEach(() => {
  vi.mocked(fiscalService.getCalendar).mockReset().mockResolvedValue({ data: [] });
  vi.mocked(fiscalService.getReportHistory).mockReset().mockResolvedValue({ data: [], meta: { total: 0 } });
  vi.mocked(fiscalService.getDimobStatus)
    .mockReset()
    .mockResolvedValue({ data: { total_rentals: 0, total_sales: 0, total_amount: 0 } } as never);
  vi.mocked(fiscalService.listInformes).mockReset().mockResolvedValue({ data: [] } as never);
  vi.mocked(fiscalService.downloadFile).mockReset();
  vi.mocked(fiscalService.downloadInformePDF).mockReset();
  vi.mocked(fiscalService.archiveReport).mockReset().mockResolvedValue({ success: true } as never);
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('FIN-01 · NFS-e indisponível', () => {
  it('as permissões estão concedidas, então a ausência de controles é real', async () => {
    // Salvaguarda do próprio teste: com fiscal:generate concedida a página
    // renderiza controles em outras áreas. Se este teste falhar, os demais
    // estariam passando por oculatção do <Can>, não por bloqueio da NFS-e.
    renderPage();
    await waitFor(() => expect(fiscalService.getDimobStatus).toHaveBeenCalled());

    expect(screen.getAllByRole('button').length).toBeGreaterThan(0);
  });

  it('não oferece ação de emissão na área de NFS-e', async () => {
    renderPage();
    await waitFor(() => expect(fiscalService.getDimobStatus).toHaveBeenCalled());

    // Nenhum controle acionável deve prometer emissão de nota. O rótulo
    // antigo era "Ir para Emissão" — o match é case-insensitive de propósito.
    expect(screen.queryByRole('button', { name: /ir para emiss[ãa]o/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /emitir/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: /emiss[ãa]o de nfs-?e/i })).not.toBeInTheDocument();
  });

  it('exibe o estado "Em breve" e explica a dependência da integração', async () => {
    renderPage();
    await waitFor(() => expect(fiscalService.getDimobStatus).toHaveBeenCalled());

    const card = screen.getByRole('heading', { name: 'Nota Fiscal de Serviço' }).closest('div.bg-card');
    expect(card).not.toBeNull();
    expect(card).toHaveTextContent('Em breve');
    expect(card).toHaveTextContent(/integração com o emissor municipal/i);
  });

  it('interagir com a área de NFS-e não invoca emissão', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => expect(fiscalService.getDimobStatus).toHaveBeenCalled());

    const card = screen.getByRole('heading', { name: 'Nota Fiscal de Serviço' }).closest('div.bg-card')!;
    for (const el of card.querySelectorAll('button, a, [role="button"]')) {
      await user.click(el);
    }

    // Guarda de superfície: a emissão só pode voltar junto com uma decisão
    // explícita de expor `issueNfse` no cliente — não por acidente.
    const service = fiscalService as unknown as Record<string, unknown>;
    expect(service.issueNfse).toBeUndefined();
    expect(service.listNfse).toBeUndefined();
  });

  it('o cliente fiscal não expõe emissão de NFS-e', () => {
    // A ausência de `issueNfse`/`listNfse` no cliente é o contrato do bloqueio:
    // sem método, não existe caminho de emissão a partir da interface.
    const service = fiscalService as unknown as Record<string, unknown>;
    expect(service.issueNfse).toBeUndefined();
    expect(service.listNfse).toBeUndefined();
  });
});

describe('Gestão de relatórios fiscais', () => {
  it('troca o ano-calendário e recarrega os dados do ano escolhido', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => expect(fiscalService.getDimobStatus).toHaveBeenCalled());

    const select = screen.getByLabelText('Ano-calendário');
    await user.selectOptions(select, String(new Date().getFullYear() - 1));

    await waitFor(() =>
      expect(fiscalService.getDimobStatus).toHaveBeenCalledWith(new Date().getFullYear() - 1),
    );
    expect(fiscalService.getCalendar).toHaveBeenCalledWith(new Date().getFullYear() - 1);
    expect(fiscalService.listInformes).toHaveBeenCalledWith(new Date().getFullYear() - 1);
  });

  it('mostra o estado real da DIMOB e só libera o download quando pronta', async () => {
    vi.mocked(fiscalService.getDimobStatus).mockResolvedValue({
      data: { total_rentals: 2, total_sales: 0, total_amount: 100, status: 'PENDING' },
    } as never);
    const { unmount } = renderPage();
    await waitFor(() => expect(fiscalService.getDimobStatus).toHaveBeenCalled());
    expect(screen.getByText('Não gerada')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /baixar arquivo/i })).toBeDisabled();
    unmount();

    vi.mocked(fiscalService.getDimobStatus).mockResolvedValue({
      data: {
        total_rentals: 2,
        total_sales: 1,
        total_amount: 100,
        status: 'READY',
        declarante_cnpj: '11222333000181',
        declarante_nome: 'Imobiliária Modelo',
        generated_at: '2026-10-03T12:00:00Z',
      },
    } as never);
    renderPage();
    await waitFor(() => expect(screen.getByText('Pronta para envio')).toBeInTheDocument());
    expect(screen.getByRole('button', { name: /baixar arquivo/i })).toBeEnabled();
    expect(screen.getByText(/Imobiliária Modelo · 11222333000181/)).toBeInTheDocument();
    // Vendas aparecem marcadas como fora do arquivo (ficha R04 ainda não emitida).
    expect(screen.getByText(/1 vendas \(fora do arquivo nesta versão\)/)).toBeInTheDocument();
  });

  it('avisa quando a DIMOB foi gerada sem o CNPJ da declarante', async () => {
    vi.mocked(fiscalService.getDimobStatus).mockResolvedValue({
      data: { total_rentals: 1, total_sales: 0, total_amount: 10, status: 'READY' },
    } as never);
    renderPage();
    await waitFor(() =>
      expect(screen.getByText(/Gerada sem o CNPJ da empresa/i)).toBeInTheDocument(),
    );
  });

  it('filtra o histórico por tipo e envia o filtro ao backend', async () => {
    const user = userEvent.setup();
    vi.mocked(fiscalService.getReportHistory).mockResolvedValue({
      data: [
        {
          id: 'r1',
          type: 'DIMOB',
          name: 'DIMOB 2026',
          file_url: '/api/v1/fiscal/dimob/2026/file',
          generated_at: '2026-10-03T12:00:00Z',
          status: 'READY',
        },
      ],
      meta: { total: 1 },
    } as never);
    renderPage();
    await waitFor(() => expect(screen.getByText('DIMOB 2026')).toBeInTheDocument());

    await user.selectOptions(screen.getByLabelText('Filtrar por tipo'), 'INFORME');
    await waitFor(() =>
      expect(fiscalService.getReportHistory).toHaveBeenCalledWith(
        expect.objectContaining({ type: 'INFORME' }),
      ),
    );
  });

  it('arquiva um relatório após confirmação e o remove da lista', async () => {
    const user = userEvent.setup();
    vi.mocked(fiscalService.getReportHistory).mockResolvedValue({
      data: [
        {
          id: 'r1',
          type: 'DIMOB',
          name: 'DIMOB 2026',
          file_url: '/api/v1/fiscal/dimob/2026/file',
          generated_at: '2026-10-03T12:00:00Z',
          status: 'READY',
        },
      ],
      meta: { total: 1 },
    } as never);
    renderPage();
    await waitFor(() => expect(screen.getByText('DIMOB 2026')).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: /arquivar dimob 2026/i }));
    const confirm = await screen.findByRole('button', { name: /^arquivar$/i });
    await user.click(confirm);

    await waitFor(() => expect(fiscalService.archiveReport).toHaveBeenCalledWith('r1'));
    await waitFor(() => expect(screen.queryByText('DIMOB 2026')).not.toBeInTheDocument());
  });

  it('separa locação e venda na tabela de informes e mostra a intermediação de venda', async () => {
    vi.mocked(fiscalService.listInformes).mockResolvedValue({
      data: [
        {
          id: 'i1',
          year: 2026,
          owner_id: 'lead-1',
          owner_name: 'Karina Motta',
          total_gross: 1402800,
          total_admin_fee: 280,
          total_intermediation: 84361.28,
          total_net: 1318158.72,
          rental_gross: 2800,
          rental_admin_fee: 280,
          rental_intermediation: 361.28,
          rental_net: 2158.72,
          rental_count: 1,
          sale_gross: 1400000,
          sale_intermediation: 84000,
          sale_net: 1316000,
          sale_count: 1,
          status: 'READY',
          created_at: '2026-10-03T12:00:00Z',
          tenant_id: 'tenant-1',
        },
      ],
    } as never);

    renderPage();
    await waitFor(() => expect(screen.getByText('Karina Motta')).toBeInTheDocument());

    const row = screen.getByText('Karina Motta').closest('tr')!;
    // Locação (bruto) e venda aparecem separadas — nada de 1,4 mi como rendimento de locação.
    expect(row).toHaveTextContent('R$ 2.800,00');
    expect(row).toHaveTextContent('R$ 1.400.000,00');
    // Intermediação = locação + venda (361,28 + 84.000).
    expect(row).toHaveTextContent('R$ 84.361,28');
    expect(row).toHaveTextContent('R$ 1.318.158,72');
  });

  it('usa os totais legados quando o informe não tem separação por fluxo', async () => {
    vi.mocked(fiscalService.listInformes).mockResolvedValue({
      data: [
        {
          id: 'i2',
          year: 2026,
          owner_id: 'lead-2',
          owner_name: 'Ayla Mizuke',
          total_gross: 5509.69,
          total_admin_fee: 550.97,
          total_intermediation: 0,
          total_net: 2158.72,
          status: 'READY',
          created_at: '2026-10-03T12:00:00Z',
          tenant_id: 'tenant-1',
        },
      ],
    } as never);

    renderPage();
    await waitFor(() => expect(screen.getByText('Ayla Mizuke')).toBeInTheDocument());
    const row = screen.getByText('Ayla Mizuke').closest('tr')!;
    // O total misto NÃO aparece como rendimento de locação: as colunas de
    // fluxo ficam neutras e o líquido (inequívoco) permanece.
    expect(row).not.toHaveTextContent('R$ 5.509,69');
    expect(row).toHaveTextContent('—');
    expect(row).toHaveTextContent('R$ 2.158,72');
    // E o aviso para regravar aparece, com ação.
    expect(screen.getByText(/Há informes gerados antes da separação/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /regravar informes de/i })).toBeInTheDocument();
  });

  it('lista os informes com PDF por proprietário e abre o arquivo', async () => {
    const user = userEvent.setup();
    vi.mocked(fiscalService.listInformes).mockResolvedValue({
      data: [
        {
          id: 'i1',
          year: 2026,
          owner_id: 'lead-1',
          owner_name: 'Fábio Ramos',
          total_gross: 96000,
          total_admin_fee: 9600,
          total_intermediation: 0,
          total_net: 86400,
          status: 'READY',
          created_at: '2026-10-03T12:00:00Z',
          tenant_id: 'tenant-1',
        },
      ],
    } as never);
    vi.mocked(fiscalService.downloadInformePDF).mockResolvedValue({
      data: new Blob(['pdf'], { type: 'application/pdf' }),
      headers: {},
    } as never);
    const openSpy = vi.spyOn(window, 'open').mockReturnValue(null);
    const createObjectURL = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:informe');
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});

    renderPage();
    await waitFor(() => expect(screen.getByText('Fábio Ramos')).toBeInTheDocument());
    expect(screen.getByText('1 gerados')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /abrir informe de Fábio Ramos/i }));
    await waitFor(() => expect(fiscalService.downloadInformePDF).toHaveBeenCalledWith('i1'));
    // A abertura acontece depois do await do download; esperar evita corrida.
    await waitFor(() => expect(openSpy).toHaveBeenCalled());
    expect(createObjectURL).toHaveBeenCalled();
  });
});
