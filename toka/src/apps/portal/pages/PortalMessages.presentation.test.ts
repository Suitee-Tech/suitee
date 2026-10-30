import { describe, expect, it } from 'vitest';
import {
  applyPortalReaction,
  normalizePortalChannel,
  sortPortalChannels,
  type Channel,
  type UIMessage,
} from '../hooks/usePortalChannels';
import {
  buildPortalMessageTimeline,
  filterPortalMessages,
  getPortalChannelName,
  getPortalChannelAvatar,
  getPortalInitials,
  getPortalMessageSender,
} from './PortalMessages.presentation';

function message(overrides: Partial<UIMessage> = {}): UIMessage {
  return {
    id: 'message-1',
    channel_id: 'channel-1',
    sender_id: 'operator-1',
    sender_name: 'Maria Corretora',
    content: 'Vamos revisar o contrato',
    sent_at: '2026-09-05T13:00:00.000Z',
    read_by: ['operator-1'],
    ...overrides,
  };
}

function channel(overrides: Partial<Channel> = {}): Channel {
  return {
    id: 'channel-1',
    name: 'Conversa',
    type: 'direct',
    participants: ['portal-client', 'operator-1'],
    updated_at: '2026-09-05T13:00:00.000Z',
    ...overrides,
  };
}

describe('portal message presentation', () => {
  it('recognizes persisted client messages by account identity even with a technical sender name', () => {
    expect(getPortalMessageSender(
      message({ sender_id: 'portal-client', sender_name: 'cliente@portal.local' }),
      channel({ type: 'deal' }),
      'portal-client',
    )).toEqual({ isMe: true, name: 'Você' });
  });

  it('resolves other deal participants by sender id without borrowing the broker identity', () => {
    const dealChannel = channel({
      type: 'deal',
      participants: [
        { id: 'operator-1', name: 'Maria Corretora', avatar_url: '/maria.png' },
        { id: 'other-client', name: 'João Cliente', avatar_url: '/joao.png' },
        'portal-client',
      ],
    });

    expect(getPortalMessageSender(
      message({ sender_id: 'other-client', sender_name: 'other-client' }),
      dealChannel,
      'portal-client',
    )).toEqual({ isMe: false, name: 'João Cliente', avatar: '/joao.png' });
    expect(getPortalMessageSender(message(), dealChannel, 'portal-client')).toEqual({
      isMe: false,
      name: 'Maria Corretora',
      avatar: '/maria.png',
    });
  });

  it.each(['unknown-client', 'cliente@portal.local', '+5511999999999', ''])('uses a neutral label for unresolved author %j', (senderName) => {
    expect(getPortalMessageSender(
      message({ sender_id: 'unknown-client', sender_name: senderName }),
      channel({ type: 'deal', participants: ['portal-client', 'operator-1', 'unknown-client'] }),
      'portal-client',
    )).toEqual({ isMe: false, name: 'Participante da conversa', avatar: undefined });
  });

  it('preserves the name and avatar supplied for the actual sender', () => {
    expect(getPortalMessageSender(
      message({ sender_id: 'other-client', sender_name: 'João Cliente', sender_avatar_url: '/joao.png' }),
      channel({ type: 'deal' }),
      'portal-client',
    )).toEqual({ isMe: false, name: 'João Cliente', avatar: '/joao.png' });
  });

  it('uses the broker name instead of a generic direct-channel label', () => {
    expect(getPortalChannelName(channel(), 'portal-client', 'Maria Corretora')).toBe('Maria Corretora');
    expect(
      getPortalChannelName(channel({ name: 'Conversa com Cliente do Portal' }), 'portal-client', 'Maria Corretora'),
    ).toBe('Maria Corretora');
    expect(getPortalInitials('Maria Corretora')).toBe('MC');
  });

  it('groups messages by day while preserving their order', () => {
    const timeline = buildPortalMessageTimeline([
      message(),
      message({ id: 'message-2', sent_at: '2026-09-05T14:00:00.000Z' }),
      message({ id: 'message-3', sent_at: '2026-09-06T10:00:00.000Z' }),
    ]);

    expect(timeline.map((item) => item.type)).toEqual(['date', 'message', 'message', 'date', 'message']);
    expect(timeline.filter((item) => item.type === 'message').map((item) => item.message.id)).toEqual([
      'message-1',
      'message-2',
      'message-3',
    ]);
  });

  it('searches message content, sender and attachment names', () => {
    const messages = [
      message(),
      message({
        id: 'message-2',
        content: 'Segue o documento',
        attachments: [{ name: 'vistoria-final.pdf', url: '/file.pdf' }],
      }),
    ];

    expect(filterPortalMessages(messages, 'contrato').map((item) => item.id)).toEqual(['message-1']);
    expect(filterPortalMessages(messages, 'vistoria').map((item) => item.id)).toEqual(['message-2']);
    expect(filterPortalMessages(messages, 'maria').length).toBe(2);
  });
});

describe('portal channel and reaction state', () => {
  it('normalizes personal pin and mute state and sorts pinned conversations first', () => {
    const pinned = normalizePortalChannel(
      channel({ id: 'pinned', pinned_by: ['portal-client'], muted_by: ['portal-client'] }),
      'portal-client',
    );
    const recent = normalizePortalChannel(
      channel({ id: 'recent', updated_at: '2026-09-06T13:00:00.000Z' }),
      'portal-client',
    );

    expect(pinned.is_pinned).toBe(true);
    expect(pinned.is_muted).toBe(true);
    expect(sortPortalChannels([recent, pinned]).map((item) => item.id)).toEqual(['pinned', 'recent']);
  });

  it('adds and removes realtime reactions without mutating the previous message', () => {
    const original = message({ reactions: [{ emoji: '👍', user_ids: ['operator-1'] }] });
    const added = applyPortalReaction(
      [original],
      { message_id: original.id, emoji: '👍', user_id: 'portal-client' },
      'add',
    );
    const removed = applyPortalReaction(
      added,
      { message_id: original.id, emoji: '👍', user_id: 'portal-client' },
      'remove',
    );

    expect(original.reactions?.[0].user_ids).toEqual(['operator-1']);
    expect(added[0].reactions?.[0].user_ids).toEqual(['operator-1', 'portal-client']);
    expect(removed[0].reactions?.[0].user_ids).toEqual(['operator-1']);
  });
});

describe('channel participant profiles', () => {
 it('resolves broker identity in the list, header and historical messages by account ID', () => {
  const conversation = channel({ participant_profiles: [
   { id: 'portal-client', name: 'Lucas Cliente', avatar_url: '/lucas.png' },
   { id: 'operator-1', name: 'Juliana Cabral', avatar_url: '/juliana.png' },
  ] });
  expect(getPortalChannelName(conversation, 'portal-client', 'Outro corretor')).toBe('Juliana Cabral');
  expect(getPortalChannelAvatar(conversation, 'portal-client')).toBe('/juliana.png');
  expect(getPortalMessageSender(message({ sender_name: 'operator-1' }), conversation, 'portal-client')).toEqual({
   isMe: false, name: 'Juliana Cabral', avatar: '/juliana.png',
  });
  expect(getPortalMessageSender(message({ sender_id: 'other-client', sender_name: 'other-client' }), conversation, 'portal-client').name).toBe('Participante da conversa');
 });
});
