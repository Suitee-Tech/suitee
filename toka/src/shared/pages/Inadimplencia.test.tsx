import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Inadimplencia } from './Inadimplencia';
import { AgreementPanel } from '../components/delinquency/AgreementPanel';
import { GuaranteePanel } from '../components/delinquency/GuaranteePanel';
import { JudicialPanel } from '../components/delinquency/JudicialPanel';
import {
  DEFAULT_AUTO_CREATE_CASE_AFTER_DAYS,
  agreementDraftError,
  agreementPlan,
  agreementPreviewText,
  autoCreateCaseAfterDays,
  autoCreateThresholdNote,
} from '../components/delinquency/delinquencyUi';
import {
  canOpenCaseFromCharge,
  openCasePayloadFromCharge,
} from '../components/delinquency/openCaseFromCharge';
import type { Charge } from '../services/charge.service';
import type { Collection } from '../services/collection.service';

const mocks = vi.hoisted(() => ({
  listPage: vi.fn(),
  getById: vi.fn(),
  addAction: vi.fn(),
  update: vi.fn(),
  setJudicial: vi.fn(),
  activateGuarantee: vi.fn(),
  updateGuaranteeStatus: vi.fn(),
  createAgreement: vi.fn(),
  getAgreement: vi.fn(),
  breakAgreement: vi.fn(),
  cancelAgreement: vi.fn(),
  registerJudicialOutcome: vi.fn(),
  createCollection: vi.fn(),
  getDashboard: vi.fn(),
  process: vi.fn(),
  getOverdue: vi.fn(),
  getAllCharges: vi.fn(),
  getSecurityDepositBalance: vi.fn(),
  canRead: true,
  canWrite: true,
  canTerminate: true,
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}));

vi.mock('../services/collection.service', () => ({
  collectionService: {
    listPage: mocks.listPage,
    getById: mocks.getById,
    addAction: mocks.addAction,
    update: mocks.update,
    setJudicial: mocks.setJudicial,
    activateGuarantee: mocks.activateGuarantee,
    updateGuaranteeStatus: mocks.updateGuaranteeStatus,
    createAgreement: mocks.createAgreement,
    getAgreement: mocks.getAgreement,
    breakAgreement: mocks.breakAgreement,
    cancelAgreement: mocks.cancelAgreement,
    registerJudicialOutcome: mocks.registerJudicialOutcome,
    create: mocks.createCollection,
  },
}));

vi.mock('../services/charge.service', () => ({
  chargeService: {
    getOverdue: mocks.getOverdue,
    getAll: mocks.getAllCharges,
    getSecurityDepositBalance: mocks.getSecurityDepositBalance,
  },
}));

vi.mock('../services/delinquency.service', () => ({
  delinquencyService: { getDashboard: mocks.getDashboard, process: mocks.process },
}));

vi.mock('sonner', () => ({ toast: { success: mocks.success, error: mocks.error, info: mocks.info } }));
vi.mock('../context/FocusModeContext', () => ({ useFocusMode: () => ({ focusMode: false }) }));
vi.mock('../ui/use-mobile', () => ({ useIsMobile: () => false }));
vi.mock('../permissions/usePermission', () => ({
  usePermission: (id: string) =>
    id === 'charges:read' ? mocks.canRead : id === 'charges:write' ? mocks.canWrite : mocks.canTerminate,
}));

const collection = (overrides: Partial<Collection> = {}): Collection => ({
  id: 'col-1',
  tenant_id: 'tenant-1',
  contract_id: 'ctr-1',
  debtor_id: 'debtor-1',
  debtor_name: 'Gabriela Prado',
  total_debt: 999.99,
  days_overdue: 18,
  charge_ids: ['charge-1'],
  status: 'ACTIVE',
  actions: [],
  created_at: '2026-10-01T10:00:00Z',
  updated_at: '2026-10-01T10:00:00Z',
  ...overrides,
});

/** Cobrança como `GET /charges/overdue` devolve (com ou sem `days_overdue`). */
const charge = (overrides: Partial<Charge> = {}): Charge => ({
  id: 'charge-vencida',
  tenant_id: 'tenant-1',
  contract_id: 'ctr-1',
  tenant_party_id: 'parte-1',
  owner_id: 'owner-1',
  property_id: 'imovel-1',
  type: 'RENT',
  description: 'Aluguel 10/2026',
  reference_month: '2026-10',
  amount: 1200,
  late_fee: 0,
  interest: 0,
  discount: 0,
  total_amount: 1200,
  due_date: `${dateInDays(-3)}T00:00:00Z`,
  status: 'OVERDUE',
  created_at: '2026-10-01T10:00:00Z',
  updated_at: '2026-10-01T10:00:00Z',
  ...overrides,
});

const page = (items: Collection[]) => ({
  items,
  total: items.length,
  page: 1,
  limit: 20,
  totalPages: 1,
});

const serverConflict = (message: string) => ({
  response: { status: 409, data: { message } },
});

/** Datas dos testes são relativas a hoje: o backend recusa parcela no passado. */
const dateInDays = (days: number) => {
  const d = new Date();
  d.setDate(d.getDate() + days);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
};

/**
 * Data no calendário LOCAL do runner: `chargeDueLabel` conta dias civis pelo
 * fuso do navegador, então uma data derivada de UTC pode cair no dia anterior.
 */
const localDateInDays = (days: number) => {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
};

/** Mesma leitura de `formatISODateBR`: a data é o DIA em UTC. */
const isoDateBR = (iso: string) => new Intl.DateTimeFormat('pt-BR', { timeZone: 'UTC' }).format(new Date(`${iso}T00:00:00Z`));

/** O Intl separa "R$" do número com NBSP; normaliza para comparar texto. */
const text = (node: HTMLElement) => (node.textContent ?? '').replace(/\u00a0/g, ' ');

const judicialProcess = {
  case_number: '0000000-00.0000.0.00.0000',
  court: '1ª Vara Cível de São Paulo',
  lawyer: { name: 'Ana Ribeiro', oab: '123456/SP' },
  claim_value: 999.99,
  status: 'EM_TRAMITE' as const,
  created_at: '2026-10-01T10:00:00Z',
  updated_at: '2026-10-01T10:00:00Z',
};

const guaranteeActivation = {
  type: 'CAUCAO' as const,
  amount: 1500,
  reason: 'Três parcelas em atraso',
  status: 'ACCEPTED' as const,
  insurer_protocol: 'PROT-123',
  insurer_name: 'Seguradora Alfa',
  requested_at: '2026-10-05T10:00:00Z',
  updated_at: '2026-10-06T10:00:00Z',
};

describe('Gestão de inadimplência', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.canRead = true;
    mocks.canWrite = true;
    mocks.canTerminate = true;
    mocks.listPage.mockResolvedValue(page([collection()]));
    mocks.getById.mockResolvedValue(collection());
    mocks.getAgreement.mockResolvedValue(null);
    // Padrão: nenhuma cobrança vencida (a faixa some do caminho dos demais casos).
    mocks.getOverdue.mockResolvedValue({ data: [] });
    mocks.getAllCharges.mockResolvedValue({ data: [], meta: { total: 0 } });
    mocks.getSecurityDepositBalance.mockResolvedValue({ data: null });
    mocks.getDashboard.mockResolvedValue({
      stats: {
        total_overdue: 4,
        total_debt_amount: 123456.78,
        average_debt_amount: 30864.19,
        average_days_overdue: 32,
        active_collections: 7,
        recovery_rate: 41.5,
      },
      aging_report: [{ label: '31-60', min_days: 31, max_days: 60, count: 3, total_debt: 45000 }],
      collection_stats: {
        active: 7,
        negotiated: 2,
        guarantee_activated: 1,
        judicial: 1,
        resolved: 3,
        total_debt: 200000,
        recovered_amount: 30000,
      },
    });
  });

  it('mostra os KPIs do endpoint de delinquency, não a soma da lista', async () => {
    render(<Inadimplencia />);

    // O caso da lista vale R$ 999,99: os KPIs têm de continuar sendo os do servidor.
    const kpis = await screen.findByLabelText('Indicadores de inadimplência');
    const totalCard = within(kpis).getByText('Total em Atraso').closest('div')?.parentElement as HTMLElement;
    expect(within(totalCard).getByText(/123\.456,78/)).toBeInTheDocument();
    expect(within(totalCard).queryByText(/999,99/)).not.toBeInTheDocument();

    const activeCard = within(kpis).getByText('Em Cobrança').closest('div')?.parentElement as HTMLElement;
    expect(within(activeCard).getByText('7')).toBeInTheDocument();
    expect(within(kpis).getByText(/41,5%/)).toBeInTheDocument();
    expect(within(kpis).getByText('31-60 dias')).toBeInTheDocument();
  });

  it('não transforma o 409 do judicial em sucesso', async () => {
    mocks.setJudicial.mockRejectedValue(
      serverConflict('invalid collection status transition: ACTIVE -> JUDICIAL'),
    );
    render(<Inadimplencia />);
    await screen.findByText('Gabriela Prado');

    fireEvent.click(screen.getByTitle('Processo Judicial'));
    const dialog = screen.getByRole('dialog', { name: 'Processo Judicial' });
    fireEvent.change(within(dialog).getByLabelText('Número do Processo (CNJ) *'), {
      target: { value: '0000000-00.0000.0.00.0000' },
    });
    fireEvent.change(within(dialog).getByLabelText('Vara/Comarca *'), {
      target: { value: '1ª Vara Cível de São Paulo' },
    });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Registrar Processo' }));

    await waitFor(() => expect(mocks.setJudicial).toHaveBeenCalledTimes(1));
    expect(mocks.setJudicial).toHaveBeenCalledWith(
      'col-1',
      expect.objectContaining({ case_number: '0000000-00.0000.0.00.0000', court: '1ª Vara Cível de São Paulo' }),
    );
    expect(mocks.success).not.toHaveBeenCalled();
    await waitFor(() =>
      expect(mocks.error).toHaveBeenCalledWith(
        'invalid collection status transition: ACTIVE -> JUDICIAL',
      ),
    );
    // O painel continua aberto para o usuário corrigir o dado.
    expect(screen.getByRole('dialog', { name: 'Processo Judicial' })).toBeInTheDocument();
  });

  it('cria o acordo enviando parcelas, entrada e desconto no formato do contrato', async () => {
    const firstDue = dateInDays(45);
    mocks.createAgreement.mockResolvedValue(collection({ status: 'NEGOTIATED' }));
    render(<Inadimplencia />);
    await screen.findByText('Gabriela Prado');

    fireEvent.click(screen.getByTitle('Acordo de Dívida'));
    const dialog = screen.getByRole('dialog', { name: 'Acordo de Dívida' });

    // O aviso de substituição das cobranças originais aparece ANTES de confirmar.
    expect(dialog).toHaveTextContent(/cobranças originais deste caso serão canceladas/);

    fireEvent.change(within(dialog).getByLabelText('Valor Total do Acordo *'), { target: { value: '1000000' } });
    fireEvent.change(within(dialog).getByLabelText('Desconto'), { target: { value: '100000' } });
    fireEvent.change(within(dialog).getByLabelText('Entrada'), { target: { value: '50000' } });
    fireEvent.change(within(dialog).getByLabelText('Nº de Parcelas *'), { target: { value: '4' } });
    fireEvent.change(within(dialog).getByLabelText('Data da 1ª Parcela *'), { target: { value: firstDue } });
    fireEvent.change(within(dialog).getByLabelText('Observações'), { target: { value: 'Acordo com o inquilino' } });

    // Prévia calculada no cliente: 10.000 − 1.000 − 500 = 8.500 em 4x de 2.125,00.
    const preview = within(dialog).getByText(/primeira em/);
    expect(text(preview)).toContain('4x de R$ 2.125,00 + entrada de R$ 500,00');

    fireEvent.click(within(dialog).getByRole('button', { name: 'Criar Acordo' }));

    await waitFor(() => expect(mocks.createAgreement).toHaveBeenCalledTimes(1));
    expect(mocks.createAgreement).toHaveBeenCalledWith('col-1', {
      total_amount: 10000,
      discount: 1000,
      down_payment: 500,
      installments: 4,
      first_due_date: `${firstDue}T12:00:00.000Z`,
      interval_days: 30,
      notes: 'Acordo com o inquilino',
    });
    expect(mocks.success).toHaveBeenCalledWith(
      'Acordo registrado: as cobranças originais foram substituídas pelas parcelas do acordo.',
    );
  });

  it('mostra o 400 do acordo com a mensagem do servidor e não fecha o painel', async () => {
    mocks.createAgreement.mockRejectedValue({
      response: { status: 400, data: { message: 'down_payment + installments inconsistentes' } },
    });
    render(<Inadimplencia />);
    await screen.findByText('Gabriela Prado');

    fireEvent.click(screen.getByTitle('Acordo de Dívida'));
    const dialog = screen.getByRole('dialog', { name: 'Acordo de Dívida' });
    // O valor total já vem preenchido com o `total_debt` do caso.
    fireEvent.change(within(dialog).getByLabelText('Nº de Parcelas *'), { target: { value: '4' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Criar Acordo' }));

    await waitFor(() => expect(mocks.createAgreement).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(mocks.error).toHaveBeenCalledWith('down_payment + installments inconsistentes'));
    expect(mocks.success).not.toHaveBeenCalled();
    expect(screen.getByRole('dialog', { name: 'Acordo de Dívida' })).toBeInTheDocument();
  });

  it('mostra o acordo registrado com as parcelas e quebra com motivo e restabelecimento do desconto', async () => {
    mocks.getAgreement.mockResolvedValue({
      id: 'agr-1',
      collection_id: 'col-1',
      status: 'ACTIVE',
      original_total: 999.99,
      total_amount: 900,
      discount: 99.99,
      down_payment: 100,
      installments: 2,
      interval_days: 30,
      first_due_date: '2026-11-10T12:00:00Z',
      original_charge_ids: ['charge-1', 'charge-2'],
      installment_charge_ids: ['charge-3', 'charge-4'],
      created_at: '2026-10-10T12:00:00Z',
      created_by: 'user-1',
      approval_required: false,
      notes: 'Acordo fechado por telefone',
    });
    mocks.breakAgreement.mockResolvedValue(collection({ status: 'ACTIVE' }));
    render(<Inadimplencia />);
    await screen.findByText('Gabriela Prado');

    fireEvent.click(screen.getByTitle('Acordo de Dívida'));
    const dialog = screen.getByRole('dialog', { name: 'Acordo de Dívida' });
    await waitFor(() => expect(within(dialog).getByText('Acordo registrado')).toBeInTheDocument());

    expect(dialog).toHaveTextContent('2x a cada 30 dia(s)');
    expect(dialog).toHaveTextContent('cobrança charge-3');
    expect(dialog).toHaveTextContent(/2 cobrança\(s\) originais deste caso foram canceladas/);
    // O formulário de criação não aparece: o caso já tem acordo.
    expect(within(dialog).queryByLabelText('Valor Total do Acordo *')).not.toBeInTheDocument();

    fireEvent.change(within(dialog).getByLabelText('Motivo da quebra *'), {
      target: { value: 'Parcela 2 não paga' },
    });
    fireEvent.click(within(dialog).getByLabelText(/Restabelecer o desconto/));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Quebrar acordo' }));

    await waitFor(() => expect(mocks.breakAgreement).toHaveBeenCalledTimes(1));
    expect(mocks.breakAgreement).toHaveBeenCalledWith('col-1', {
      reason: 'Parcela 2 não paga',
      restore_discount: true,
    });
  });

  it('avisa que a sentença improcedente devolve o caso para a cobrança ativa e envia o desfecho com as custas', async () => {
    mocks.listPage.mockResolvedValue(page([collection({ status: 'JUDICIAL', judicial: judicialProcess })]));
    mocks.registerJudicialOutcome.mockResolvedValue(collection({ status: 'ACTIVE' }));
    render(<Inadimplencia />);
    await screen.findByText('Gabriela Prado');

    fireEvent.click(screen.getByTitle('Processo Judicial'));
    const dialog = screen.getByRole('dialog', { name: 'Processo Judicial' });

    fireEvent.change(within(dialog).getByLabelText('Resultado da decisão *'), {
      target: { value: 'SENTENCA_IMPROCEDENTE' },
    });
    expect(dialog).toHaveTextContent(/volta para cobrança ativa \(ACTIVE\) e a régua de cobrança retoma/);

    const decidedAt = dateInDays(-5);
    fireEvent.change(within(dialog).getByLabelText('Data da Decisão *'), { target: { value: decidedAt } });

    fireEvent.click(within(dialog).getByRole('button', { name: 'Adicionar custa' }));
    fireEvent.change(within(dialog).getByLabelText('Descrição *'), { target: { value: 'Custas de distribuição' } });
    fireEvent.change(within(dialog).getByLabelText('Valor *'), { target: { value: '25000' } });
    expect(dialog).toHaveTextContent(/custas serão lançadas como despesa do contrato/);

    fireEvent.click(within(dialog).getByRole('button', { name: 'Registrar Desfecho' }));

    await waitFor(() => expect(mocks.registerJudicialOutcome).toHaveBeenCalledTimes(1));
    expect(mocks.registerJudicialOutcome).toHaveBeenCalledWith('col-1', {
      status: 'SENTENCA_IMPROCEDENTE',
      decided_at: `${decidedAt}T12:00:00.000Z`,
      notes: undefined,
      costs: [{ description: 'Custas de distribuição', amount: 250 }],
    });
    expect(mocks.success).toHaveBeenCalledWith('Desfecho judicial registrado!');
  });

  it('dá baixa por garantia enviando PAID com o valor recebido e explica o efeito na caução', async () => {
    mocks.listPage.mockResolvedValue(
      page([collection({ status: 'GUARANTEE_ACTIVATED', guarantee: guaranteeActivation })]),
    );
    mocks.updateGuaranteeStatus.mockResolvedValue(collection({ status: 'RESOLVED' }));
    render(<Inadimplencia />);
    await screen.findByText('Gabriela Prado');

    fireEvent.click(screen.getByTitle('Acionamento de Garantia'));
    const dialog = screen.getByRole('dialog', { name: 'Acionamento de Garantia' });

    expect(dialog).toHaveTextContent(/mais antiga para a mais recente/);
    expect(dialog).toHaveTextContent(/crédito do caso/);
    expect(dialog).toHaveTextContent(/debitado da caução do contrato/);

    fireEvent.change(within(dialog).getByLabelText('Valor recebido *'), { target: { value: '123456' } });
    fireEvent.change(within(dialog).getByLabelText('Referência / protocolo'), {
      target: { value: 'SINISTRO-987' },
    });
    fireEvent.change(within(dialog).getByLabelText('Observação'), { target: { value: 'Crédito conferido' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Registrar Indenização' }));

    await waitFor(() => expect(mocks.updateGuaranteeStatus).toHaveBeenCalledTimes(1));
    expect(mocks.updateGuaranteeStatus).toHaveBeenCalledWith('col-1', {
      status: 'PAID',
      settled_amount: 1234.56,
      settlement_reference: 'SINISTRO-987',
      insurer_response: 'Crédito conferido',
    });
  });

  it('avisa que recusar/cancelar a garantia devolve o caso para a cobrança ativa', async () => {
    mocks.listPage.mockResolvedValue(
      page([collection({ status: 'GUARANTEE_ACTIVATED', guarantee: guaranteeActivation })]),
    );
    render(<Inadimplencia />);
    await screen.findByText('Gabriela Prado');

    fireEvent.click(screen.getByTitle('Acionamento de Garantia'));
    const dialog = screen.getByRole('dialog', { name: 'Acionamento de Garantia' });
    fireEvent.change(within(dialog).getByLabelText('Novo status *'), { target: { value: 'REJECTED' } });

    expect(dialog).toHaveTextContent(/caso volta para cobrança ativa \(ACTIVE\) e a régua de cobrança retoma/);
  });

  it('aciona a garantia enviando tipo, valor e motivo para o endpoint do domínio', async () => {
    mocks.activateGuarantee.mockResolvedValue(
      collection({ status: 'GUARANTEE_ACTIVATED', guarantee: { ...guaranteeActivation, type: 'FIADOR', status: 'REQUESTED' } }),
    );
    render(<Inadimplencia />);
    await screen.findByText('Gabriela Prado');

    fireEvent.click(screen.getByTitle('Acionamento de Garantia'));
    const dialog = screen.getByRole('dialog', { name: 'Acionamento de Garantia' });
    fireEvent.change(within(dialog).getByLabelText('Tipo de Garantia *'), { target: { value: 'FIADOR' } });
    fireEvent.change(within(dialog).getByLabelText('Valor a Acionar *'), { target: { value: '150000' } });
    fireEvent.change(within(dialog).getByLabelText('Motivo do Acionamento'), {
      target: { value: 'Três parcelas em atraso' },
    });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Acionar Garantia' }));

    await waitFor(() => expect(mocks.activateGuarantee).toHaveBeenCalledTimes(1));
    expect(mocks.activateGuarantee).toHaveBeenCalledWith('col-1', {
      type: 'FIADOR',
      amount: 1500,
      reason: 'Três parcelas em atraso',
    });
    expect(mocks.success).toHaveBeenCalledWith('Garantia acionada com sucesso!');
  });

  it('registra ação de cobrança sem enviar user_id', async () => {
    mocks.addAction.mockResolvedValue(
      collection({ actions: [{ date: '2026-10-10T12:00:00Z', type: 'Ligação Telefônica', description: 'Sem resposta', user_id: '' }] }),
    );
    render(<Inadimplencia />);
    await screen.findByText('Gabriela Prado');

    fireEvent.click(screen.getByTitle('Procedimento de Cobrança'));
    const dialog = screen.getByRole('dialog', { name: 'Procedimento de Cobrança' });
    fireEvent.change(within(dialog).getByLabelText('Resposta/Retorno *'), { target: { value: 'Sem resposta' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Registrar Procedimento' }));

    await waitFor(() => expect(mocks.addAction).toHaveBeenCalledTimes(1));
    expect(mocks.addAction).toHaveBeenCalledWith('col-1', {
      type: 'Ligação Telefônica',
      description: 'Sem resposta',
    });
    expect(mocks.addAction.mock.calls[0][1]).not.toHaveProperty('user_id');
  });

  it('esconde as ações de escrita sem charges:write e explica a rescisão sem contracts:write', async () => {
    mocks.canWrite = false;
    mocks.canTerminate = false;
    render(<Inadimplencia />);
    await screen.findByText('Gabriela Prado');

    expect(screen.queryByTitle('Procedimento de Cobrança')).not.toBeInTheDocument();
    expect(screen.queryByTitle('Acordo de Dívida')).not.toBeInTheDocument();
    expect(screen.queryByTitle('Acionamento de Garantia')).not.toBeInTheDocument();
    expect(screen.queryByTitle('Processo Judicial')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Processar Inadimplência' })).not.toBeInTheDocument();
    // O desfecho judicial vive dentro do painel do processo: sem a ação, não há como chegar nele.
    expect(screen.queryByRole('button', { name: 'Registrar Desfecho' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Criar Acordo' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Registrar Indenização' })).not.toBeInTheDocument();

    const rescisao = screen.getByTitle('Seu perfil não tem permissão para encerrar contratos (contracts:write)');
    expect(rescisao).toBeDisabled();
    expect(screen.getByText(/somente leitura/)).toBeInTheDocument();
  });

  it('mantém os painéis de acordo, desfecho e indenização em somente leitura sem charges:write', async () => {
    const noop = () => {};
    const agreement = render(
      <AgreementPanel open collection={collection()} canWrite={false} onClose={noop} onSaved={noop} />,
    );
    const agreementDialog = screen.getByRole('dialog', { name: 'Acordo de Dívida' });
    await waitFor(() => expect(agreementDialog).toHaveTextContent(/não tem charges:write/));
    await waitFor(() => expect(within(agreementDialog).getByRole('button', { name: 'Criar Acordo' })).toBeDisabled());
    agreement.unmount();

    const judicial = render(
      <JudicialPanel
        open
        collection={collection({ status: 'JUDICIAL', judicial: judicialProcess })}
        canWrite={false}
        onClose={noop}
        onSaved={noop}
      />,
    );
    const judicialDialog = screen.getByRole('dialog', { name: 'Processo Judicial' });
    expect(judicialDialog).toHaveTextContent(/não tem charges:write/);
    expect(within(judicialDialog).getByRole('button', { name: 'Registrar Desfecho' })).toBeDisabled();
    judicial.unmount();

    render(
      <GuaranteePanel
        open
        collection={collection({ status: 'GUARANTEE_ACTIVATED', guarantee: guaranteeActivation })}
        canWrite={false}
        onClose={noop}
        onSaved={noop}
      />,
    );
    const guaranteeDialog = screen.getByRole('dialog', { name: 'Acionamento de Garantia' });
    expect(guaranteeDialog).toHaveTextContent(/não tem charges:write/);
    expect(within(guaranteeDialog).getByRole('button', { name: 'Registrar Indenização' })).toBeDisabled();
  });

  it('filtra de verdade por aba e pagina com limit/offset do servidor', async () => {
    mocks.listPage.mockResolvedValue(page([collection()]));
    render(<Inadimplencia />);
    await screen.findByText('Gabriela Prado');
    expect(mocks.listPage).toHaveBeenLastCalledWith({ status: undefined, limit: 20, offset: 0 });

    fireEvent.click(screen.getByRole('tab', { name: 'Judicial' }));
    await waitFor(() =>
      expect(mocks.listPage).toHaveBeenLastCalledWith({ status: 'JUDICIAL', limit: 20, offset: 0 }),
    );

    fireEvent.click(screen.getByRole('tab', { name: 'Garantias' }));
    await waitFor(() =>
      expect(mocks.listPage).toHaveBeenLastCalledWith({ status: 'GUARANTEE_ACTIVATED', limit: 20, offset: 0 }),
    );

    mocks.listPage.mockResolvedValue({ ...page([collection()]), totalPages: 3, total: 45 });
    fireEvent.click(screen.getByRole('tab', { name: 'Cobrança' }));
    await waitFor(() =>
      expect(mocks.listPage).toHaveBeenLastCalledWith({ status: 'ACTIVE', limit: 20, offset: 0 }),
    );
    fireEvent.click(screen.getByTitle('Próxima página'));
    await waitFor(() =>
      expect(mocks.listPage).toHaveBeenLastCalledWith({ status: 'ACTIVE', limit: 20, offset: 20 }),
    );
  });
});

describe('cobranças vencidas sem caso aberto (efeito do processamento)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.canRead = true;
    mocks.canWrite = true;
    mocks.canTerminate = true;
    mocks.listPage.mockResolvedValue(page([collection({ charge_ids: ['charge-com-caso'] })]));
    mocks.getById.mockResolvedValue(collection());
    mocks.getAgreement.mockResolvedValue(null);
    mocks.getDashboard.mockResolvedValue({ stats: null, aging_report: null, collection_stats: null });
    mocks.getAllCharges.mockResolvedValue({ data: [], meta: { total: 0 } });
    mocks.getSecurityDepositBalance.mockResolvedValue({ data: null });
    mocks.createCollection.mockResolvedValue({ data: collection({ id: 'col-novo' }) });
  });

  it('mostra a cobrança vencida SEM caso (com o total no rótulo) e esconde a que já tem caso', async () => {
    mocks.getOverdue.mockResolvedValue({
      data: [
        charge({ id: 'charge-sem-caso', description: 'Aluguel sem caso', days_overdue: 10, due_date: `${dateInDays(-10)}T00:00:00Z` }),
        charge({ id: 'charge-com-caso', description: 'Aluguel com caso', days_overdue: 6, due_date: `${dateInDays(-6)}T00:00:00Z` }),
      ],
    });

    render(<Inadimplencia />);

    // A aba Inadimplentes tem UM caso aberto (o caso da lista) e UMA cobrança
    // vencida fora dele: antes desta faixa a tela parecia não ter feito nada.
    expect(await screen.findByText('Cobranças vencidas sem caso aberto (1)')).toBeInTheDocument();
    expect(screen.getByText('Aluguel sem caso')).toBeInTheDocument();
    expect(screen.getByText('Venceu há 10 dias')).toBeInTheDocument();
    expect(screen.queryByText('Aluguel com caso')).not.toBeInTheDocument();
    // A conferência percorre a carteira de casos inteira (não só a página).
    expect(mocks.listPage).toHaveBeenCalledWith({ limit: 100, offset: 0 });
    // A nota explica por que a cobrança ainda não virou caso.
    expect(screen.getByText(/aberto automaticamente quando o atraso alcança 5 dias/)).toBeInTheDocument();
    // Valor e vencimento da cobrança vencida ficam visíveis para a cobrança.
    expect(screen.getByText(/1\.200,00/)).toBeInTheDocument();
    expect(screen.getByText(isoDateBR(dateInDays(-10)))).toBeInTheDocument();
  });

  it('não quebra com payload antigo sem days_overdue e deriva o rótulo do vencimento', async () => {
    mocks.getOverdue.mockResolvedValue({
      data: [charge({ id: 'charge-legado', description: 'Aluguel legado', due_date: `${dateInDays(-1)}T00:00:00Z` })],
    });

    render(<Inadimplencia />);

    expect(await screen.findByText('Cobranças vencidas sem caso aberto (1)')).toBeInTheDocument();
    expect(screen.getByText('Venceu ontem')).toBeInTheDocument();
  });

  it('cobrança que vence HOJE aparece como "Vence hoje", nunca como vencida', async () => {
    mocks.getOverdue.mockResolvedValue({
      data: [charge({ id: 'charge-hoje', description: 'Aluguel do dia', status: 'PENDING', days_overdue: 0, due_date: `${dateInDays(0)}T00:00:00Z` })],
    });

    render(<Inadimplencia />);

    expect(await screen.findByText('Vence hoje')).toBeInTheDocument();
    expect(screen.queryByText('Vencido')).not.toBeInTheDocument();
  });

  it('recarrega as cobranças vencidas depois de processar a inadimplência', async () => {
    mocks.getOverdue.mockResolvedValue({
      data: [charge({ id: 'charge-2', description: 'Aluguel recarregado', days_overdue: 2, due_date: `${dateInDays(-2)}T00:00:00Z` })],
    });
    mocks.process.mockResolvedValue({
      charges_marked_overdue: 1,
      charges_reverted: 1,
      late_fees_calculated: 1,
      collections_created: 0,
      collections_updated: 0,
      processed_at: '2026-10-10T12:00:00Z',
    });

    render(<Inadimplencia />);
    await screen.findByText('Cobranças vencidas sem caso aberto (1)');
    expect(mocks.getOverdue).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole('button', { name: 'Processar Inadimplência' }));

    await waitFor(() => expect(mocks.process).toHaveBeenCalledTimes(1));
    // O efeito do processamento tem de aparecer: a lista de cobranças vencidas
    // é recarregada junto com casos e indicadores.
    await waitFor(() => expect(mocks.getOverdue).toHaveBeenCalledTimes(2));
    expect(mocks.success).toHaveBeenCalledWith(expect.stringContaining('1 cobrança(s) revertida(s) para pendente'));
  });

  it('esconde a faixa sem charges:read e não chama o endpoint de cobranças', async () => {
    mocks.canRead = false;
    mocks.getOverdue.mockResolvedValue({ data: [charge()] });

    render(<Inadimplencia />);
    await screen.findByText('Gabriela Prado');

    expect(screen.queryByText(/Cobranças vencidas sem caso aberto/)).not.toBeInTheDocument();
    expect(mocks.getOverdue).not.toHaveBeenCalled();
  });

  it('mostra o nome do inquilino quando o backend resolve e cai no id quando não vem', async () => {
    mocks.getOverdue.mockResolvedValue({
      data: [
        charge({ id: 'charge-com-nome', description: 'Aluguel com nome', tenant_party_id: 'parte-1', debtor_name: 'Marcos Vinícius' }),
        charge({ id: 'charge-sem-nome', description: 'Aluguel sem nome', tenant_party_id: 'parte-2' }),
      ],
    });

    render(<Inadimplencia />);

    expect(await screen.findByText('Cobranças vencidas sem caso aberto (2)')).toBeInTheDocument();
    expect(screen.getByText('Marcos Vinícius')).toBeInTheDocument();
    // A resolução do nome no CRM pode falhar em silêncio: o id continua na tela.
    expect(screen.getByText('parte-2')).toBeInTheDocument();
    expect(screen.queryByText('parte-1')).not.toBeInTheDocument();
  });

  it('usa o limiar do servidor na nota e diz que ele é da imobiliária', async () => {
    mocks.getOverdue.mockResolvedValue({ data: [charge({ id: 'charge-limiar', days_overdue: 3 })] });
    mocks.getDashboard.mockResolvedValue({
      stats: null,
      aging_report: null,
      collection_stats: null,
      config: {
        auto_create_collection_after_days: 7,
        process_interval_minutes: 60,
        late_fee_percent: 2,
        monthly_interest_percent: 1,
        source: 'tenant',
      },
    });

    render(<Inadimplencia />);

    expect(await screen.findByText(/alcança 7 dias/)).toBeInTheDocument();
    expect(screen.getByText(/limiar configurado pela própria imobiliária/)).toBeInTheDocument();
    expect(screen.queryByText(/alcança 5 dias/)).not.toBeInTheDocument();
  });

  it('cai no fallback documentado de 5 dias quando o dashboard não traz config', async () => {
    mocks.getOverdue.mockResolvedValue({ data: [charge({ id: 'charge-sem-config', days_overdue: 3 })] });

    render(<Inadimplencia />);

    expect(await screen.findByText(/alcança 5 dias/)).toBeInTheDocument();
    expect(screen.queryByText(/limiar configurado pela própria imobiliária/)).not.toBeInTheDocument();
    expect(screen.queryByText(/padrão da plataforma/)).not.toBeInTheDocument();
  });

  it('abre o caso com o payload do contrato e só depois de confirmar', async () => {
    mocks.getOverdue.mockResolvedValue({
      data: [
        charge({
          id: 'charge-abrir',
          description: 'Aluguel para abrir caso',
          contract_id: 'ctr-9',
          tenant_party_id: 'parte-9',
          debtor_name: 'Marcos Vinícius',
          total_amount: 1234.56,
          days_overdue: 9,
          due_date: `${dateInDays(-9)}T00:00:00Z`,
        }),
      ],
    });
    mocks.createCollection.mockResolvedValue({ data: collection({ id: 'col-novo' }) });

    render(<Inadimplencia />);
    await screen.findByText('Cobranças vencidas sem caso aberto (1)');

    fireEvent.click(screen.getByRole('button', { name: 'Abrir caso' }));

    // Ação que cria dívida formal: nada é enviado antes da confirmação explícita.
    expect(mocks.createCollection).not.toHaveBeenCalled();
    const dialog = screen.getByRole('alertdialog');
    expect(dialog).toHaveTextContent('Marcos Vinícius');
    expect(dialog).toHaveTextContent('ctr-9');
    expect(text(dialog)).toContain('R$ 1.234,56');
    expect(dialog).toHaveTextContent('9 dia(s) de atraso');

    fireEvent.click(within(dialog).getByRole('button', { name: 'Abrir caso' }));

    await waitFor(() => expect(mocks.createCollection).toHaveBeenCalledTimes(1));
    expect(mocks.createCollection).toHaveBeenCalledWith({
      contract_id: 'ctr-9',
      debtor_id: 'parte-9',
      debtor_name: 'Marcos Vinícius',
      total_debt: 1234.56,
      days_overdue: 9,
      charge_ids: ['charge-abrir'],
    });
    await waitFor(() => expect(mocks.success).toHaveBeenCalledWith('Caso de cobrança aberto para Marcos Vinícius.'));
    // Bloco, lista de casos e indicadores recarregados depois do sucesso.
    await waitFor(() => expect(mocks.getOverdue).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(mocks.getDashboard).toHaveBeenCalledTimes(2));
  });

  it('mostra o 409 do servidor sem criar nada', async () => {
    mocks.getOverdue.mockResolvedValue({ data: [charge({ id: 'charge-conflito', debtor_name: 'Marcos Vinícius', days_overdue: 4 })] });
    mocks.createCollection.mockRejectedValue(
      serverConflict('cobrança charge-conflito já está no caso de cobrança col-9 (Em Cobrança)'),
    );

    render(<Inadimplencia />);
    await screen.findByText('Cobranças vencidas sem caso aberto (1)');
    expect(mocks.getOverdue).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole('button', { name: 'Abrir caso' }));
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Abrir caso' }));

    await waitFor(() =>
      expect(mocks.error).toHaveBeenCalledWith('cobrança charge-conflito já está no caso de cobrança col-9 (Em Cobrança)'),
    );
    expect(mocks.success).not.toHaveBeenCalled();
    // Nada foi criado: o bloco não é recarregado como se a ação tivesse dado certo.
    expect(mocks.getOverdue).toHaveBeenCalledTimes(1);
    expect(screen.getByText('Cobranças vencidas sem caso aberto (1)')).toBeInTheDocument();
  });

  it('mostra o 400 de valor divergente com a mensagem do servidor', async () => {
    mocks.getOverdue.mockResolvedValue({ data: [charge({ id: 'charge-divergente', days_overdue: 4 })] });
    mocks.createCollection.mockRejectedValue({
      response: { status: 400, data: { message: 'total_debt (1200) não bate com a soma das cobranças (1150.50)' } },
    });

    render(<Inadimplencia />);
    await screen.findByText('Cobranças vencidas sem caso aberto (1)');

    fireEvent.click(screen.getByRole('button', { name: 'Abrir caso' }));
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Abrir caso' }));

    await waitFor(() =>
      expect(mocks.error).toHaveBeenCalledWith('total_debt (1200) não bate com a soma das cobranças (1150.50)'),
    );
    expect(mocks.success).not.toHaveBeenCalled();
  });

  it('esconde o botão de abrir caso sem charges:write', async () => {
    mocks.canWrite = false;
    mocks.getOverdue.mockResolvedValue({ data: [charge({ id: 'charge-sem-permissao', debtor_name: 'Marcos Vinícius' })] });

    render(<Inadimplencia />);

    expect(await screen.findByText('Cobranças vencidas sem caso aberto (1)')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Abrir caso' })).not.toBeInTheDocument();
    expect(screen.queryByRole('columnheader', { name: 'Ação' })).not.toBeInTheDocument();
  });

  it('não oferece abrir caso em linha sem contrato ou sem devedor', async () => {
    mocks.getOverdue.mockResolvedValue({
      data: [charge({ id: 'charge-sem-contrato', contract_id: '' }), charge({ id: 'charge-sem-devedor', tenant_party_id: '' })],
    });

    render(<Inadimplencia />);

    expect(await screen.findByText('Cobranças vencidas sem caso aberto (2)')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Abrir caso' })).not.toBeInTheDocument();
  });

  it('mostra o saldo da cobrança parcialmente paga (total − pago) e o quanto já foi pago', async () => {
    mocks.getOverdue.mockResolvedValue({
      data: [
        charge({
          id: 'charge-parcial',
          description: 'Aluguel parcial',
          total_amount: 1200,
          paid_amount: 200.5,
          days_overdue: 8,
        }),
      ],
    });

    render(<Inadimplencia />);

    expect(await screen.findByText('Cobranças vencidas sem caso aberto (1)')).toBeInTheDocument();
    // É ESTE saldo que vai em `total_debt` no corpo de POST /collections.
    expect(text(screen.getByText(/999,50/))).toBe('R$ 999,50');
    expect(screen.getByText('Pago R$ 200,50')).toBeInTheDocument();
  });
});

describe('corpo do "Abrir caso" e limiar da régua (espelham o backend)', () => {
  it('usa o id do inquilino quando o nome não vem', () => {
    const payload = openCasePayloadFromCharge(
      charge({ id: 'charge-x', days_overdue: 2, total_amount: 100.005 }),
    );
    expect(payload).toEqual({
      contract_id: 'ctr-1',
      debtor_id: 'parte-1',
      debtor_name: 'parte-1',
      total_debt: 100.01,
      days_overdue: 2,
      charge_ids: ['charge-x'],
    });
  });

  // Cobrança que ainda não venceu não abre caso: o backend recusa ("ainda não
  // venceu") e a dívida não está vencida — o botão simplesmente não aparece.
  it('não monta payload para cobrança sem atraso', () => {
    const semAtraso = charge({ id: 'charge-hoje', days_overdue: 0 });
    expect(openCasePayloadFromCharge(semAtraso)).toBeNull();
    expect(canOpenCaseFromCharge(semAtraso)).toBe(false);

    const vencida = charge({ id: 'charge-ontem', days_overdue: 1 });
    expect(canOpenCaseFromCharge(vencida)).toBe(true);
    expect(openCasePayloadFromCharge(vencida)?.days_overdue).toBe(1);
  });

  it('deriva o atraso do vencimento sem days_overdue e recusa linha sem contrato ou devedor', () => {
    const legacy = openCasePayloadFromCharge(
      charge({ id: 'charge-legado', due_date: `${localDateInDays(-3)}T00:00:00Z` }),
    );
    expect(legacy?.days_overdue).toBe(3);
    expect(openCasePayloadFromCharge(charge({ contract_id: '' }))).toBeNull();
    expect(openCasePayloadFromCharge(charge({ tenant_party_id: '' }))).toBeNull();
  });

  it('envia o saldo (total − pago) da cobrança parcialmente paga, como o backend valida', () => {
    const parcial = charge({ id: 'charge-parcial', total_amount: 1200, paid_amount: 200.5 });
    expect(openCasePayloadFromCharge(parcial)?.total_debt).toBe(999.5);
  });

  it('descreve o limiar do servidor pela origem (imobiliária, plataforma ou régua)', () => {
    expect(autoCreateThresholdNote(7, 'tenant')).toMatch(/7 dias \(limiar configurado pela própria imobiliária\)/);
    expect(autoCreateThresholdNote(7, 'global')).toMatch(/7 dias \(padrão da plataforma\)/);
    expect(autoCreateThresholdNote(7)).toMatch(/7 dias \(limiar da régua\)/);
    expect(autoCreateCaseAfterDays({ auto_create_collection_after_days: 7 })).toBe(7);
    // Payload antigo (sem `config`): fallback documentado, nunca NaN nem zero.
    expect(autoCreateCaseAfterDays(null)).toBe(DEFAULT_AUTO_CREATE_CASE_AFTER_DAYS);
    expect(autoCreateCaseAfterDays({ auto_create_collection_after_days: 0 })).toBe(DEFAULT_AUTO_CREATE_CASE_AFTER_DAYS);
  });
});

describe('validação e prévia do acordo (espelham o backend)', () => {
  const base = {
    totalAmount: 1000,
    discount: 0,
    downPayment: 0,
    installments: 4,
    firstDueDate: dateInDays(30),
    intervalDays: 30,
  };

  it('recusa total ≤ 0, desconto ≥ total, entrada que zera as parcelas, parcelas fora de 1..36 e data no passado', () => {
    expect(agreementDraftError({ ...base, totalAmount: 0 })).toMatch(/valor total/);
    expect(agreementDraftError({ ...base, discount: 1000 })).toMatch(/desconto não pode ser igual ou maior/);
    expect(agreementDraftError({ ...base, downPayment: 1000 })).toMatch(/não sobra valor para as parcelas/);
    expect(agreementDraftError({ ...base, installments: 0 })).toMatch(/entre 1 e 36/);
    expect(agreementDraftError({ ...base, installments: 37 })).toMatch(/entre 1 e 36/);
    expect(agreementDraftError({ ...base, firstDueDate: dateInDays(-1) })).toMatch(/não pode estar no passado/);
    expect(agreementDraftError(base)).toBeNull();
  });

  it('calcula a prévia das parcelas com a última absorvendo os centavos', () => {
    const plan = agreementPlan({
      totalAmount: 1000,
      discount: 100,
      downPayment: 100,
      installments: 3,
      firstDueDate: dateInDays(30),
      intervalDays: 30,
    });
    expect(plan?.financed).toBe(800);
    expect(plan?.installmentValue).toBe(266.67);
    expect(plan?.lastInstallmentValue).toBe(266.66);
    expect(agreementPreviewText(plan!).replace(/\u00a0/g, ' ')).toContain(
      '3x de R$ 266,67 + entrada de R$ 100,00',
    );
  });
});
