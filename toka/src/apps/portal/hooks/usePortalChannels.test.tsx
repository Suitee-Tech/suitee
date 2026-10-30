import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { usePortalChannels, type Channel, type Message } from './usePortalChannels';

const mocks = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  delete: vi.fn(),
  user: { id: 'local-client-account' } as { id: string } | null,
  listener: null as ((type: string, payload: unknown) => void) | null,
  onMessage: vi.fn(),
}));

vi.mock('../context/PortalAuthContext', () => ({
  usePortalAuth: () => ({ user: mocks.user }),
}));

vi.mock('../services/portal-api', () => ({
  default: { get: mocks.get, post: mocks.post, delete: mocks.delete },
  unwrapPortalData: (response: { data: unknown }) => response.data,
  getPortalAccessToken: () => `header.${btoa(JSON.stringify({
    sub: 'supabase-user-id',
    app_metadata: { functional_role: 'cliente', tenant_id: 'local-client-account' },
  }))}.signature`,
}));

vi.mock('../services/portal.service', () => ({
  portalService: { getOverview: async () => [] },
}));

vi.mock('./usePortalWebSocket', () => ({
  usePortalWebSocket: () => ({ onMessage: mocks.onMessage, isConnected: true }),
}));

const channels: Channel[] = ['deal-channel', 'other-channel'].map((id) => ({
  id,
  name: id,
  type: 'deal',
  participants: ['local-client-account', 'broker-account', 'other-client-account'],
}));

function message(overrides: Partial<Message> = {}): Message {
  return {
    id: 'persisted-message',
    channel_id: 'deal-channel',
    sender_id: 'local-client-account',
    sender_name: 'Cliente do portal',
    content: 'Minha primeira mensagem',
    content_type: 'text',
    sent_at: '2026-09-09T12:00:00.000Z',
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

async function openConversation() {
  const view = renderHook(() => usePortalChannels());
  await waitFor(() => {
    expect(view.result.current.activeChannel?.id).toBe('deal-channel');
    expect(view.result.current.isLoadingMessages).toBe(false);
  });
  return view;
}

function receive(incoming: Message) {
  act(() => mocks.listener?.('new_message', incoming));
}

describe('usePortalChannels delivery and sender identity', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.user = { id: 'local-client-account' };
    mocks.listener = null;
    mocks.get.mockImplementation(async (url: string) => ({
      data: url === '/channels/mine' ? channels : [],
    }));
    mocks.post.mockResolvedValue({ data: undefined });
    mocks.delete.mockResolvedValue({ data: undefined });
    mocks.onMessage.mockImplementation((listener) => {
      mocks.listener = listener;
      return () => { mocks.listener = null; };
    });
  });

  afterEach(cleanup);

  it('uses the authenticated local account when the Supabase subject differs', async () => {
    const view = await openConversation();
    expect(view.result.current.myAccountId).toBe('local-client-account');

    mocks.user = { id: 'refreshed-local-account' };
    view.rerender();
    await waitFor(() => expect(view.result.current.myAccountId).toBe('refreshed-local-account'));
  });

  it.each(['websocket-first', 'http-first'])('shows one message with %s confirmation', async (order) => {
    const view = await openConversation();
    const request = deferred<{ data: Message }>();
    mocks.post.mockReturnValueOnce(request.promise);
    let sending!: Promise<Message | undefined>;
    act(() => { sending = view.result.current.sendMessage('deal-channel', message().content); });
    expect(view.result.current.messages).toHaveLength(1);
    expect(view.result.current.messages[0].sender_id).toBe('local-client-account');

    if (order === 'websocket-first') {
      receive(message());
      expect(view.result.current.messages).toHaveLength(1);
    }
    await act(async () => { request.resolve({ data: message() }); await sending; });
    receive(message());
    receive(message());

    expect(view.result.current.messages).toMatchObject([{ id: 'persisted-message', _status: 'sent' }]);
    expect(view.result.current.messages).toHaveLength(1);
    expect(view.result.current.channels[0].last_message?.id).toBe('persisted-message');
  });

  it('deduplicates the HTTP acknowledgement even when the realtime payload could not match the draft', async () => {
    const view = await openConversation();
    const request = deferred<{ data: Message }>();
    mocks.post.mockReturnValueOnce(request.promise);
    let sending!: Promise<Message | undefined>;
    act(() => { sending = view.result.current.sendMessage('deal-channel', '/status'); });
    const incoming = message({ content: 'Status da negociação', sender_id: 'system', sender_name: 'Sistema' });
    receive(incoming);
    await act(async () => { request.resolve({ data: incoming }); await sending; });

    expect(view.result.current.messages).toHaveLength(1);
    expect(view.result.current.messages[0]).toMatchObject({ id: incoming.id, content: incoming.content, _status: 'sent' });
  });

  it('keeps another participant message with identical text separate from the outgoing draft', async () => {
    const view = await openConversation();
    const request = deferred<{ data: Message }>();
    mocks.post.mockReturnValueOnce(request.promise);
    let sending!: Promise<Message | undefined>;
    act(() => { sending = view.result.current.sendMessage('deal-channel', message().content); });
    receive(message({ id: 'other-message', sender_id: 'other-client-account', sender_name: 'Outro cliente' }));
    expect(view.result.current.messages).toHaveLength(2);
    expect(view.result.current.messages[0]._status).toBe('sending');
    receive(message());
    await act(async () => { request.resolve({ data: message() }); await sending; });

    expect(view.result.current.messages.map((item) => item.id)).toEqual(['persisted-message', 'other-message']);
  });

  it('preserves realtime reads, reactions and edits received before the HTTP response', async () => {
    const view = await openConversation();
    const request = deferred<{ data: Message }>();
    mocks.post.mockReturnValueOnce(request.promise);
    let sending!: Promise<Message | undefined>;
    act(() => { sending = view.result.current.sendMessage('deal-channel', message().content); });
    receive(message());
    act(() => {
      mocks.listener?.('messages_read', { channel_id: 'deal-channel', user_id: 'broker-account' });
      mocks.listener?.('reaction_added', {
        channel_id: 'deal-channel', message_id: 'persisted-message', user_id: 'broker-account', emoji: '👍',
      });
      mocks.listener?.('message_edited', {
        channel_id: 'deal-channel', message_id: 'persisted-message', content: 'Mensagem editada',
      });
    });
    await act(async () => { request.resolve({ data: message() }); await sending; });

    expect(view.result.current.messages).toHaveLength(1);
    expect(view.result.current.messages[0]).toMatchObject({
      id: 'persisted-message',
      content: 'Mensagem editada',
      is_edited: true,
      read_by: ['broker-account'],
      reactions: [{ emoji: '👍', user_ids: ['broker-account'] }],
      _status: 'sent',
    });
  });

  it('keeps two intentional identical sends when confirmations arrive out of order', async () => {
    const view = await openConversation();
    const first = deferred<{ data: Message }>();
    const second = deferred<{ data: Message }>();
    mocks.post.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    let sendingFirst!: Promise<Message | undefined>;
    let sendingSecond!: Promise<Message | undefined>;
    act(() => {
      sendingFirst = view.result.current.sendMessage('deal-channel', message().content);
      sendingSecond = view.result.current.sendMessage('deal-channel', message().content);
    });
    const secondMessage = message({ id: 'second-message' });
    receive(secondMessage);
    await act(async () => { second.resolve({ data: secondMessage }); await sendingSecond; });
    await act(async () => { first.resolve({ data: message() }); await sendingFirst; });
    receive(message());

    expect(view.result.current.messages).toHaveLength(2);
    expect(new Set(view.result.current.messages.map((item) => item.id))).toEqual(new Set(['persisted-message', 'second-message']));
  });

  it.each(['local', 'realtime'])('does not restore a message deleted through %s before the send response', async (source) => {
    const view = await openConversation();
    const request = deferred<{ data: Message }>();
    mocks.post.mockReturnValueOnce(request.promise);
    let sending!: Promise<Message | undefined>;
    act(() => { sending = view.result.current.sendMessage('deal-channel', message().content); });
    receive(message());
    if (source === 'local') {
      await act(async () => { await view.result.current.deleteMessage('deal-channel', 'persisted-message'); });
    } else {
      act(() => mocks.listener?.('message_deleted', { channel_id: 'deal-channel', message_id: 'persisted-message' }));
    }
    await act(async () => { request.resolve({ data: message() }); await sending; });
    receive(message());

    expect(view.result.current.messages).toEqual([]);
  });

  it('retries a failed send and reconciles the realtime confirmation with the response', async () => {
    const view = await openConversation();
    mocks.post.mockRejectedValueOnce(new Error('Network unavailable'));
    await act(async () => {
      await expect(view.result.current.sendMessage('deal-channel', message().content)).rejects.toThrow('Network unavailable');
    });
    expect(view.result.current.messages[0]._status).toBe('failed');
    const request = deferred<{ data: Message }>();
    mocks.post.mockReturnValueOnce(request.promise);
    let retrying!: Promise<Message | undefined>;
    act(() => { retrying = view.result.current.retryMessage(view.result.current.messages[0]); });
    receive(message());
    await act(async () => { request.resolve({ data: message() }); await retrying; });

    expect(view.result.current.messages).toHaveLength(1);
    expect(view.result.current.messages[0]).toMatchObject({ id: 'persisted-message', _status: 'sent' });
  });

  it('keeps a send response in its original channel after switching conversations', async () => {
    const view = await openConversation();
    const request = deferred<{ data: Message }>();
    mocks.post.mockReturnValueOnce(request.promise);
    let sending!: Promise<Message | undefined>;
    act(() => { sending = view.result.current.sendMessage('deal-channel', message().content); });
    act(() => view.result.current.setActiveChannel(channels[1]));
    await waitFor(() => expect(view.result.current.isLoadingMessages).toBe(false));
    await act(async () => { request.resolve({ data: message() }); await sending; });

    expect(view.result.current.activeChannel?.id).toBe('other-channel');
    expect(view.result.current.messages).toEqual([]);
    expect(view.result.current.channels.find((item) => item.id === 'deal-channel')?.last_message?.id).toBe('persisted-message');
  });
});

describe('portal conversation discovery', () => {
 beforeEach(() => {
  vi.clearAllMocks(); mocks.user = { id: 'local-client-account' };
  mocks.onMessage.mockImplementation((listener) => { mocks.listener = listener; return () => {}; });
  mocks.get.mockImplementation(async (url: string) => ({ data: url === '/channels/mine' ? channels : [] }));
  mocks.post.mockResolvedValue({ data: {} });
 });
 afterEach(cleanup);
 it('fetches an unknown conversation announced by a message', async () => {
  const view = await openConversation();
  const newChannel = { ...channels[0], id:'broker-initiated' };
  mocks.get.mockImplementation(async (url: string) => ({ data: url === '/channels/mine' ? [...channels,newChannel] : [] }));
  receive(message({channel_id:newChannel.id,sender_id:'broker-account'}));
  await waitFor(() => expect(view.result.current.channels.some(channel => channel.id===newChannel.id)).toBe(true));
 });
 it('recovers conversations missed while the portal was away', async () => {
  const view = await openConversation();
  mocks.get.mockImplementation(async (url: string) => ({ data: url === '/channels/mine' ? [...channels,{...channels[0],id:'missed'}] : [] }));
  act(() => window.dispatchEvent(new Event('focus')));
  await waitFor(() => expect(view.result.current.channels.some(channel => channel.id==='missed')).toBe(true));
 });
});
