import userEvent from '@testing-library/user-event';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Channel, UIMessage } from '../hooks/usePortalChannels';
import { PortalMessages } from './PortalMessages';

const mocks = vi.hoisted(() => ({
  emptyChannels: false,
  channelOverrides: {} as Partial<Channel>,
  messagesOverride: null as UIMessage[] | null,
  setActiveChannel: vi.fn(),
  refreshChannels: vi.fn(),
  refreshMessages: vi.fn(),
  sendMessage: vi.fn(),
  retryMessage: vi.fn(),
  uploadAttachment: vi.fn(),
  addReaction: vi.fn(),
  removeReaction: vi.fn(),
  editMessage: vi.fn(),
  deleteMessage: vi.fn(),
  togglePinMessage: vi.fn(),
  togglePinChannel: vi.fn(),
  toggleMuteChannel: vi.fn(),
  sendTyping: vi.fn(),
}));

vi.mock('../hooks/usePortalChannels', () => ({
  usePortalChannels: () => ({
    channels: mocks.emptyChannels ? [] : [
      {
        id: 'channel-1',
        name: 'Conversa com Cliente do Portal',
        type: 'direct',
        status: 'open',
        participants: ['portal-client', 'operator-1'],
        updated_at: '2026-09-06T13:00:00.000Z',
        last_message: {
          id: 'message-1',
          channel_id: 'channel-1',
          sender_id: 'operator-1',
          sender_name: 'Maria Corretora',
          content: 'Olá, em que posso ajudar?',
          sent_at: '2026-09-06T13:00:00.000Z',
          read_by: ['operator-1'],
        },
        ...mocks.channelOverrides,
      },
    ],
    activeChannel: mocks.emptyChannels ? null : {
      id: 'channel-1',
      name: 'Conversa com Cliente do Portal',
      type: 'direct',
      status: 'open',
      participants: ['portal-client', 'operator-1'],
      ...mocks.channelOverrides,
    },
    setActiveChannel: mocks.setActiveChannel,
    messages: mocks.messagesOverride ?? [
      {
        id: 'message-1',
        channel_id: 'channel-1',
        sender_id: 'operator-1',
        sender_name: 'Maria Corretora',
        content: 'Olá, em que posso ajudar?',
        sent_at: '2026-09-06T13:00:00.000Z',
        read_by: ['operator-1'],
        _status: 'sent',
      },
    ],
    isLoading: false,
    isLoadingChannels: false,
    isLoadingMessages: false,
    channelsError: null,
    messagesError: null,
    refreshChannels: mocks.refreshChannels,
    refreshMessages: mocks.refreshMessages,
    sendMessage: mocks.sendMessage,
    retryMessage: mocks.retryMessage,
    uploadAttachment: mocks.uploadAttachment,
    addReaction: mocks.addReaction,
    removeReaction: mocks.removeReaction,
    editMessage: mocks.editMessage,
    deleteMessage: mocks.deleteMessage,
    togglePinMessage: mocks.togglePinMessage,
    togglePinChannel: mocks.togglePinChannel,
    toggleMuteChannel: mocks.toggleMuteChannel,
    sendTyping: mocks.sendTyping,
    myAccountId: 'portal-client',
    typingUsers: {},
    brokerInfo: { name: 'Maria Corretora', avatar: '/maria.png' },
    isConnected: true,
  }),
}));

describe('PortalMessages', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.emptyChannels = false;
    mocks.channelOverrides = {};
    mocks.messagesOverride = null;
    mocks.sendMessage.mockResolvedValue({ id: 'sent-message' });
    mocks.editMessage.mockResolvedValue({ id: 'message-1', content: 'Atualizada' });
  });

  it('offers contact on mobile even when the client has no conversations', () => {
    mocks.emptyChannels = true;
    render(page());
    expect(within(screen.getByRole('main')).getByRole('button', { name: 'Falar com a imobiliária' })).toBeInTheDocument();
  });

  it('presents the broker-facing conversation and exposes the same message actions as the admin chat', () => {
    render(page());

    expect(screen.getAllByText('Maria Corretora').length).toBeGreaterThan(0);
    expect(screen.queryByText('Conversa com Cliente do Portal')).not.toBeInTheDocument();
    expect(screen.getAllByText('Olá, em que posso ajudar?').length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole('button', { name: 'Ações da mensagem' }));
    expect(screen.getByRole('button', { name: 'Responder' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Copiar' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Fixar' })).toBeInTheDocument();
  });

  it('replies, emits typing and sends with Enter', async () => {
    render(page());
    fireEvent.click(screen.getByRole('button', { name: 'Ações da mensagem' }));
    fireEvent.click(screen.getByRole('button', { name: 'Responder' }));

    const input = screen.getByRole('textbox', { name: 'Mensagem' });
    fireEvent.change(input, { target: { value: 'Quero agendar uma visita' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(mocks.sendTyping).toHaveBeenCalledWith('channel-1');
    await waitFor(() =>
      expect(mocks.sendMessage).toHaveBeenCalledWith(
        'channel-1',
        'Quero agendar uma visita',
        expect.objectContaining({ id: 'message-1' }),
        [],
      ),
    );
  });

  it('keeps client messages in the deal as their own after persistence', () => {
    mocks.channelOverrides = { type: 'deal', name: 'Compra do apartamento' };
    mocks.messagesOverride = [{
      id: 'client-message',
      channel_id: 'channel-1',
      sender_id: 'portal-client',
      sender_name: 'cliente@portal.local',
      content: 'Enviei meus documentos',
      sent_at: '2026-09-06T13:00:00.000Z',
      _status: 'sent',
    }];
    render(page());

    const row = screen.getByText('Enviei meus documentos').closest('.group') as HTMLElement;
    expect(row).toHaveClass('justify-end');
    expect(row.querySelector('img')).toBeNull();
    expect(within(row).queryByText('Maria Corretora')).not.toBeInTheDocument();
    fireEvent.click(within(row).getByRole('button', { name: 'Ações da mensagem' }));
    expect(screen.getByRole('button', { name: 'Editar' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Apagar' })).toBeInTheDocument();
  });

  it('só apaga após confirmação e mantém o diálogo quando a API falha', async () => {
    mocks.messagesOverride = [{ id: 'client-message', channel_id: 'channel-1', sender_id: 'portal-client', sender_name: 'Cliente', content: 'Minha mensagem', sent_at: '2026-09-06T13:00:00.000Z', _status: 'sent' }];
    mocks.deleteMessage.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(undefined);
    render(page());
    fireEvent.click(screen.getByRole('button', { name: 'Ações da mensagem' }));
    fireEvent.click(screen.getByRole('button', { name: 'Apagar' }));
    expect(mocks.deleteMessage).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }));
    expect(mocks.deleteMessage).not.toHaveBeenCalled();
    expect(screen.getByRole('dialog', { name: 'Ações da mensagem' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Apagar' }));
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Apagar' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível concluir');
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Apagar' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
    expect(mocks.deleteMessage).toHaveBeenCalledTimes(2);
    expect(mocks.deleteMessage).toHaveBeenLastCalledWith('channel-1', 'client-message');
  });

  it('contém o foco das ações, restaura ao fechar e leva ao compositor ao responder', async () => {
    const user = userEvent.setup();
    render(page());
    const opener = screen.getByRole('button', { name: 'Ações da mensagem' });
    await user.click(opener);
    const dialog = screen.getByRole('dialog', { name: 'Ações da mensagem' });
    for (let i = 0; i < 14; i++) {
      await user.tab();
      expect(dialog).toContainElement(document.activeElement as HTMLElement);
    }
    await user.keyboard('{Escape}');
    await waitFor(() => expect(opener).toHaveFocus());
    await user.click(opener);
    await user.click(screen.getByRole('button', { name: 'Responder' }));
    await waitFor(() => expect(screen.getByRole('textbox', { name: 'Mensagem' })).toHaveFocus());
  });

  it('shows another deal client with their own profile and without author-only actions', () => {
    mocks.channelOverrides = {
      type: 'deal',
      name: 'Compra do apartamento',
      participants: [
        'portal-client',
        { id: 'operator-1', name: 'Maria Corretora', avatar_url: '/maria.png' },
        { id: 'other-client', name: 'João Cliente', avatar_url: '/joao.png' },
      ],
    };
    mocks.messagesOverride = [{
      id: 'other-client-message',
      channel_id: 'channel-1',
      sender_id: 'other-client',
      sender_name: 'other-client',
      content: 'Também assinei o contrato',
      sent_at: '2026-09-06T13:00:00.000Z',
      _status: 'sent',
    }];
    render(page());

    const row = screen.getByText('Também assinei o contrato').closest('.group') as HTMLElement;
    expect(row).toHaveClass('justify-start');
    expect(within(row).getByText('João Cliente')).toBeInTheDocument();
    expect(row.querySelector('img')).toHaveAttribute('src', '/joao.png');
    expect(within(row).queryByText('Maria Corretora')).not.toBeInTheDocument();
    fireEvent.click(within(row).getByRole('button', { name: 'Ações da mensagem' }));
    expect(screen.queryByRole('button', { name: 'Editar' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Apagar' })).not.toBeInTheDocument();
  });

  it('never gives an unresolved deal participant the overview broker name or photo', () => {
    mocks.channelOverrides = { type: 'deal', name: 'Compra do apartamento' };
    mocks.messagesOverride = [{
      id: 'unknown-message',
      channel_id: 'channel-1',
      sender_id: 'unknown-client',
      sender_name: 'cliente@portal.local',
      content: 'Aguardo a vistoria',
      sent_at: '2026-09-06T13:00:00.000Z',
      _status: 'sent',
    }];
    render(page());

    const row = screen.getByText('Aguardo a vistoria').closest('.group') as HTMLElement;
    expect(within(row).getByText('Participante da conversa')).toBeInTheDocument();
    expect(within(row).queryByText('Maria Corretora')).not.toBeInTheDocument();
    expect(row.querySelector('img')).toBeNull();
  });
});

function page() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/portal/mensagens"]}>
        <PortalMessages />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

// "Em chat ainda não é possível voltar facilmente" (relato do usuário): o
// cabeçalho da conversa passou a ter um botão Voltar próprio — sair do chat não
// exige mais abrir a lista de conversas.
describe("voltar do chat", () => {
  it("expõe o botão Voltar no cabeçalho da conversa", async () => {
    render(page());
    expect(await screen.findByRole("button", { name: "Voltar" })).toBeInTheDocument();
    // O botão da lista de canais continua existindo, com o nome dele.
    expect(screen.getByRole("button", { name: "Ver conversas" })).toBeInTheDocument();
  });
});
