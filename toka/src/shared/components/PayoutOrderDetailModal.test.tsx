import { describe, expect, it, vi, beforeAll, afterEach } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { PayoutOrderDetailModal } from './PayoutOrderDetailModal';
import { payoutOrderService, type PayoutOrder } from '../services/payoutOrder.service';
import { proofService } from '../services/proof.service';

vi.mock('../permissions/usePermission', () => ({
  useCan: () => () => true,
  usePermission: () => true,
  useFunctionalRole: () => 'admin',
}));
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() } }));

// A política de comprovante vem do backend via React Query. Aqui é fixada para o
// teste não precisar de QueryClientProvider e ficar configurável por caso: o
// padrão é exigir comprovante (igual ao backend) e o caso que exercita a
// configuração desligada do tenant a desliga explicitamente.
const proofPolicy = { proofRequired: true };
vi.mock('../hooks/useProofPolicy', () => ({
  useProofPolicy: () => ({ proofRequired: proofPolicy.proofRequired, loading: false }),
}));

const baseOrder = {
  id: 'payout-1',
  tenant_id: 'tenant-1',
  recipient_id: 'broker-1',
  recipient_label: 'Corretora Alfa',
  status: 'APPROVED',
  amount_cents: 320000,
  method: 'MANUAL_PIX',
  created_at: '2026-09-01T10:00:00Z',
  updated_at: '2026-09-02T10:00:00Z',
  source_type: 'charge',
  recipient_type: 'broker',
  currency: 'BRL',
} as unknown as PayoutOrder;

const orderWith = (overrides: Partial<PayoutOrder>): PayoutOrder => ({ ...baseOrder, ...overrides });

beforeAll(() => {
  // jsdom não implementa as APIs de ponteiro/scroll que o Radix Dialog usa.
  const proto = Element.prototype as unknown as Record<string, unknown>;
  proto.scrollIntoView ??= vi.fn();
  proto.hasPointerCapture ??= vi.fn(() => false);
  proto.setPointerCapture ??= vi.fn();
  proto.releasePointerCapture ??= vi.fn();
  (globalThis as Record<string, unknown>).ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});

afterEach(() => {
  vi.restoreAllMocks();
  proofPolicy.proofRequired = true;
});

const renderModal = (order: PayoutOrder = baseOrder) => {
  const onUpdated = vi.fn();
  const onClose = vi.fn();
  const onRegenerated = vi.fn();
  render(
    <PayoutOrderDetailModal order={order} onClose={onClose} onUpdated={onUpdated} onRegenerated={onRegenerated} />,
  );
  return { onUpdated, onClose, onRegenerated };
};

const pdfFile = () => new File(['%PDF-1.4 conteudo'], 'comprovante-pix.pdf', { type: 'application/pdf' });

const panel = () => screen.findByRole('dialog', { name: 'Ordem de pagamento' });

/**
 * Antes só havia um campo de URL livre: o comprovante da ordem de pagamento era
 * um link externo, que o visualizador do sistema não consegue exibir — a
 * evidência existia e não podia ser conferida. O upload coloca o arquivo no
 * armazenamento do tenant, que é sempre visualizável.
 */
describe('PayoutOrderDetailModal · comprovante por upload', () => {
  it('oferece anexar arquivo, não apenas informar link', async () => {
    renderModal();
    const dialog = await panel();

    expect(within(dialog).getByText(/Selecionar PDF, JPG ou PNG/i)).toBeInTheDocument();
    expect(dialog.querySelector('input[type="file"]')).not.toBeNull();
    expect(within(dialog).getByLabelText(/URL do comprovante/i)).toBeInTheDocument();
  });

  it('envia o arquivo e grava a URL do armazenamento com o nome do próprio arquivo', async () => {
    const upload = vi.spyOn(proofService, 'upload').mockResolvedValue({
      url: 'https://storage.example/comprovante-pix.pdf',
      filename: 'comprovante-pix.pdf',
      size_bytes: 20,
    } as never);
    const markPaid = vi.spyOn(payoutOrderService, 'markPaid').mockResolvedValue(baseOrder);
    const user = userEvent.setup({ pointerEventsCheck: 0 });

    renderModal();
    const dialog = await panel();
    await user.upload(dialog.querySelector('input[type="file"]') as HTMLInputElement, pdfFile());
    await user.click(within(dialog).getByRole('button', { name: /Marcar como pago/i }));

    await waitFor(() => expect(markPaid).toHaveBeenCalled());
    expect(upload).toHaveBeenCalledWith(expect.any(File), 'payout');
    const [, payload] = markPaid.mock.calls[0];
    expect(payload.proof_url).toBe('https://storage.example/comprovante-pix.pdf');
    // O nome vem do upload: era digitado à mão e podia ficar vazio ou divergir.
    expect(payload.proof_filename).toBe('comprovante-pix.pdf');
  });

  it('mantém o link como alternativa', async () => {
    const markPaid = vi.spyOn(payoutOrderService, 'markPaid').mockResolvedValue(baseOrder);
    const user = userEvent.setup({ pointerEventsCheck: 0 });

    renderModal();
    const dialog = await panel();
    await user.type(within(dialog).getByLabelText(/URL do comprovante/i), 'https://drive.example/recibo.pdf');
    await user.click(within(dialog).getByRole('button', { name: /Marcar como pago/i }));

    await waitFor(() => expect(markPaid).toHaveBeenCalled());
    expect(markPaid.mock.calls[0][1].proof_url).toBe('https://drive.example/recibo.pdf');
  });

  it('não conclui sem arquivo nem link quando a política exige comprovante', async () => {
    const markPaid = vi.spyOn(payoutOrderService, 'markPaid').mockResolvedValue(baseOrder);
    const user = userEvent.setup({ pointerEventsCheck: 0 });

    renderModal();
    const dialog = await panel();
    await user.click(within(dialog).getByRole('button', { name: /Marcar como pago/i }));

    expect(await screen.findByRole('alert')).toHaveTextContent(/Anexe o arquivo do comprovante ou informe o link/i);
    expect(markPaid).not.toHaveBeenCalled();
  });

  it('segue a configuração do tenant quando o comprovante não é exigido', async () => {
    // O backend só recusa a baixa sem comprovante quando `payout_proof_required`
    // está ligado; a tela não pode ser mais rígida que o servidor.
    proofPolicy.proofRequired = false;
    const markPaid = vi.spyOn(payoutOrderService, 'markPaid').mockResolvedValue(baseOrder);
    const user = userEvent.setup({ pointerEventsCheck: 0 });

    renderModal();
    const dialog = await panel();
    await user.click(within(dialog).getByRole('button', { name: /Marcar como pago/i }));

    await waitFor(() => expect(markPaid).toHaveBeenCalled());
    expect(markPaid.mock.calls[0][1].proof_url).toBe('');
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('atribui falha do arquivo ao armazenamento, não à gravação', async () => {
    vi.spyOn(proofService, 'upload').mockRejectedValue({ response: { status: 503 } });
    const markPaid = vi.spyOn(payoutOrderService, 'markPaid').mockResolvedValue(baseOrder);
    const user = userEvent.setup({ pointerEventsCheck: 0 });

    renderModal();
    const dialog = await panel();
    await user.upload(dialog.querySelector('input[type="file"]') as HTMLInputElement, pdfFile());
    await user.click(within(dialog).getByRole('button', { name: /Marcar como pago/i }));

    expect(await screen.findByRole('alert')).toHaveTextContent(/armazenamento de comprovantes/i);
    expect(markPaid).not.toHaveBeenCalled();
  });

  it('valida a URL do comprovante no próprio campo, sem diálogo', async () => {
    const markPaid = vi.spyOn(payoutOrderService, 'markPaid').mockResolvedValue(baseOrder);
    const user = userEvent.setup({ pointerEventsCheck: 0 });

    renderModal();
    const dialog = await panel();
    await user.type(within(dialog).getByLabelText(/URL do comprovante/i), 'drive.example/recibo.pdf');
    await user.click(within(dialog).getByRole('button', { name: /Marcar como pago/i }));

    expect(await within(dialog).findByText(/URL deve começar com http/i)).toBeInTheDocument();
    expect(markPaid).not.toHaveBeenCalled();
    expect(screen.queryByRole('alertdialog')).toBeNull();
  });
});

/**
 * O detalhe da ordem passou a usar o shell padrão das ações financeiras
 * (`FinancialActionPanel`): seções nomeadas no corpo, ação primária no rodapé
 * fixo e o diálogo reservado só para a ação crítica de cancelar a ordem.
 */
describe('PayoutOrderDetailModal · painel padrão das ações financeiras', () => {
  it('organiza o conteúdo em seções e mantém a ação primária no rodapé fixo', async () => {
    renderModal();
    const dialog = await panel();

    expect(within(dialog).getByRole('heading', { name: 'Dados da ordem' })).toBeInTheDocument();
    expect(within(dialog).getByRole('heading', { name: 'Comprovante' })).toBeInTheDocument();
    expect(within(dialog).getByRole('heading', { name: 'Dados do pagamento' })).toBeInTheDocument();

    // O botão do rodapé é o submit do formulário do corpo (`formId`), não um
    // botão solto que dispara a gravação por conta própria.
    const submit = within(dialog).getByRole('button', { name: /Marcar como pago/i });
    expect(submit).toHaveAttribute('type', 'submit');
    expect(submit).toHaveAttribute('form');
    expect(within(dialog).getByRole('button', { name: 'Cancelar' })).toBeInTheDocument();
  });

  it('trata comissão de venda pelo recebimento da venda, não por aluguéis', async () => {
    renderModal(
      orderWith({
        status: 'DRAFT',
        source_type: 'manual',
        source_ref: { deal_id: 'deal-1', split_pct: 50 },
        funding_required_cents: 100000,
        funded_amount_cents: 40000,
      }),
    );

    const dialog = await panel();
    expect(within(dialog).getAllByText(/Aguardando recebimento da venda/).length).toBeGreaterThan(0);
    expect(within(dialog).getByRole('heading', { name: 'Cobertura da comissão de venda' })).toBeInTheDocument();
    expect(within(dialog).getByRole('heading', { name: 'Cobertura pelo recebimento da venda' })).toBeInTheDocument();
    expect(within(dialog).queryByText(/aluguéis/i)).not.toBeInTheDocument();
    expect(within(dialog).getAllByText(/recebimento da venda/).length).toBeGreaterThan(0);
  });

  it('não repete a ação de fechar quando a ação primária já é Fechar', async () => {
    renderModal(orderWith({ status: 'DRAFT', source_type: 'manual', source_ref: { deal_id: 'deal-1' } }));

    const dialog = await panel();
    expect(within(dialog).queryByRole('button', { name: 'Cancelar' })).not.toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Fechar' })).toBeInTheDocument();
  });

  it('não confirma a baixa em diálogo sobreposto', async () => {
    vi.spyOn(proofService, 'upload').mockResolvedValue({
      url: 'https://storage.example/comprovante-pix.pdf',
      filename: 'comprovante-pix.pdf',
      size_bytes: 20,
    } as never);
    const markPaid = vi.spyOn(payoutOrderService, 'markPaid').mockResolvedValue(baseOrder);
    const user = userEvent.setup({ pointerEventsCheck: 0 });

    renderModal();
    const dialog = await panel();
    await user.upload(dialog.querySelector('input[type="file"]') as HTMLInputElement, pdfFile());
    await user.click(within(dialog).getByRole('button', { name: /Marcar como pago/i }));

    await waitFor(() => expect(markPaid).toHaveBeenCalled());
    expect(screen.queryByRole('alertdialog')).toBeNull();
  });

  it('usa a aprovação como ação primária quando a ordem aguarda revisão', async () => {
    const approve = vi.spyOn(payoutOrderService, 'approve').mockResolvedValue(baseOrder);
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const { onUpdated } = renderModal(orderWith({ status: 'PENDING_REVIEW' }));

    const dialog = await panel();
    expect(within(dialog).queryByRole('button', { name: /Marcar como pago/i })).toBeNull();

    await user.click(within(dialog).getByRole('button', { name: /Aprovar pagamento/i }));
    await waitFor(() => expect(approve).toHaveBeenCalledWith('payout-1'));
    expect(onUpdated).toHaveBeenCalled();
  });

  it('mantém a aprovação visível e desabilitada enquanto falta cobertura de caixa', async () => {
    renderModal(
      orderWith({ status: 'PENDING_REVIEW', funding_required_cents: 100000, funded_amount_cents: 40000 }),
    );

    const dialog = await panel();
    expect(within(dialog).getByRole('button', { name: /Aprovar pagamento/i })).toBeDisabled();
    // O aviso explica o que falta — é ele que justifica o botão desabilitado,
    // em vez de a ação simplesmente desaparecer do rodapé.
    const blocked = within(dialog).getByText(/Aprovação ainda bloqueada/i).parentElement as HTMLElement;
    expect(blocked).toHaveTextContent(/R\$ 600,00/);
    expect(blocked).toHaveTextContent(/aluguéis completarem a cobertura/i);
  });

  it('confirma o cancelamento da ordem em diálogo — a única ação crítica do painel', async () => {
    const cancel = vi.spyOn(payoutOrderService, 'cancel').mockResolvedValue(baseOrder);
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const { onUpdated } = renderModal();

    const dialog = await panel();
    await user.click(within(dialog).getByRole('button', { name: 'Cancelar ordem' }));

    const confirm = await screen.findByRole('alertdialog');
    expect(within(confirm).getByText(/será encerrada sem registrar pagamento/i)).toBeInTheDocument();
    expect(cancel).not.toHaveBeenCalled();

    await user.click(within(confirm).getByRole('button', { name: 'Cancelar ordem' }));
    await waitFor(() => expect(cancel).toHaveBeenCalledWith('payout-1'));
    expect(onUpdated).toHaveBeenCalled();
  });

  it('em status terminal só oferece fechar o painel e explica a ausência de ações', async () => {
    renderModal(
      orderWith({
        status: 'PAID',
        proof_url: 'https://storage.example/recibo.pdf',
        proof_filename: 'recibo.pdf',
      }),
    );

    const dialog = await panel();
    expect(within(dialog).queryByRole('button', { name: /Marcar como pago/i })).toBeNull();
    expect(within(dialog).queryByRole('button', { name: /Aprovar pagamento/i })).toBeNull();
    expect(within(dialog).queryByRole('button', { name: 'Cancelar ordem' })).toBeNull();
    expect(within(dialog).getByText(/Nenhuma ação disponível para este status/i)).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: /^Fechar$/ })).toBeInTheDocument();
    expect(within(dialog).getByRole('heading', { name: 'Comprovante' })).toBeInTheDocument();
  });
  // Relato: "depois de cancelar uma ordem deve ser possível regerar e lançar
  // novamente". A ordem cancelada não pode ser um beco sem saída — o painel
  // oferece Regerar, e o resultado é a ordem NOVA (pronta para aprovar).
  it('oferece regerar a ordem cancelada e entrega a ordem nova ao chamador', async () => {
    const refreshed = orderWith({ id: 'payout-2', status: 'PENDING_REVIEW' });
    const regenerate = vi.spyOn(payoutOrderService, 'regenerate').mockResolvedValue(refreshed);
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const { onRegenerated } = renderModal(orderWith({ status: 'CANCELLED' }));

    const dialog = await panel();
    expect(within(dialog).queryByRole('button', { name: 'Cancelar ordem' })).toBeNull();
    await user.click(within(dialog).getByRole('button', { name: 'Regerar ordem' }));

    await waitFor(() => expect(regenerate).toHaveBeenCalledWith('payout-1'));
    expect(onRegenerated).toHaveBeenCalledWith(refreshed);
  });

  it('não oferece regerar em ordem viva (evita obrigação duplicada)', async () => {
    renderModal(orderWith({ status: 'APPROVED' }));
    const dialog = await panel();
    expect(within(dialog).queryByRole('button', { name: 'Regerar ordem' })).toBeNull();
  });
});

  // Relato: no detalhamento da ordem só aparecia o valor final (que é o LÍQUIDO).
  // O operador precisa ver a comissão que a originou: bruto, o que foi retido ou
  // abatido e o líquido que sobrou.
  it('mostra o valor da comissão, os descontos e o líquido da ordem', async () => {
    renderModal(
      orderWith({
        amount_cents: 240000000,
        commission: {
          commission_id: 'comm-1',
          role: 'captador',
          gross_cents: 288000000,
          net_cents: 240000000,
          deductions: [
            { id: 'd1', kind: 'INSS', amount_cents: 57600000, rate_pct: 20, base_cents: 288000000, description: 'INSS 20%' },
            { id: 'd2', kind: 'ADVANCE', amount_cents: 43200000, description: 'adiantamento de viagem' },
          ],
        },
      } as Partial<PayoutOrder>),
    );
    const dialog = await panel();
    expect(within(dialog).getByText('Valor da comissão')).toBeInTheDocument();
    expect(within(dialog).getByText(/2\.880\.000,00/)).toBeInTheDocument();
    expect(within(dialog).getByText(/INSS retido · 20%/)).toBeInTheDocument();
    expect(within(dialog).getByText(/Adiantamento abatido/)).toBeInTheDocument();
    expect(within(dialog).getByText('Valor desta ordem')).toBeInTheDocument();
});
