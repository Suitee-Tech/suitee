import { getPortalParticipants } from '../utils/channelParticipants';
import { useState, useEffect, useCallback, useRef } from 'react';
import { usePortalAuth } from '../context/PortalAuthContext';
import portalApi, { unwrapPortalData } from '../services/portal-api';
import { usePortalWebSocket } from './usePortalWebSocket';

export interface PortalParticipant {
  id: string;
  name?: string;
  avatar_url?: string;
  role?: string;
}

export interface PortalAttachment {
  id?: string;
  name: string;
  url: string;
  type?: string;
  size?: number;
}

export interface PortalReaction {
  emoji: string;
  user_ids: string[];
}

export interface Channel {
  id: string;
  name: string;
  description?: string;
  type: string;
  status?: 'open' | 'archived' | 'closed';
  participants: Array<string | PortalParticipant>;
  participant_profiles?: PortalParticipant[];
  entity_id?: string;
  entity_type?: string;
  updated_at?: string;
  last_message?: Message;
  unread_count?: number;
  pinned_by?: string[];
  muted_by?: string[];
  is_pinned?: boolean;
  is_muted?: boolean;
}

export interface Message {
  id: string;
  channel_id: string;
  tenant_id?: string;
  sender_id: string;
  sender_name: string;
  sender_avatar_url?: string;
  sender_avatar?: string;
  content: string;
  content_type?: string;
  attachments?: PortalAttachment[];
  reply_to_id?: string;
  reply_to?: {
    id: string;
    sender_name: string;
    content: string;
  };
  reactions?: PortalReaction[];
  is_pinned?: boolean;
  is_edited?: boolean;
  sent_at: string;
  edited_at?: string;
  read_by?: string[];
}

export type MessageDeliveryStatus = 'sent' | 'sending' | 'failed';

interface RetryPayload {
  content: string;
  reply_to_id?: string;
  attachments?: PortalAttachment[];
  content_type?: string;
}

export type UIMessage = Message & {
  _local_id?: string;
  _status?: MessageDeliveryStatus;
  _retry_payload?: RetryPayload;
};

function channelIdOf(payload: any): string {
  if (typeof payload?.channel_id === 'string') return payload.channel_id;
  return typeof payload?.channel === 'object' && payload.channel?.id ? payload.channel.id : '';
}

export function normalizePortalChannel(channel: Channel, accountId: string | null): Channel {
  return {
    ...channel,
    participants: getPortalParticipants(channel),
    is_pinned: Boolean(accountId && channel.pinned_by?.includes(accountId)),
    is_muted: Boolean(accountId && channel.muted_by?.includes(accountId)),
  };
}

export function sortPortalChannels(channels: Channel[]): Channel[] {
  return [...channels].sort((a, b) => {
    if (a.is_pinned !== b.is_pinned) return a.is_pinned ? -1 : 1;
    const aTime = new Date(a.updated_at || a.last_message?.sent_at || 0).getTime();
    const bTime = new Date(b.updated_at || b.last_message?.sent_at || 0).getTime();
    return bTime - aTime;
  });
}

export function applyPortalReaction(
  messages: UIMessage[],
  payload: { message_id?: string; emoji?: string; user_id?: string },
  action: 'add' | 'remove',
): UIMessage[] {
  const { message_id: messageId, emoji, user_id: userId } = payload;
  if (!messageId || !emoji || !userId) return messages;

  return messages.map((message) => {
    if (message.id !== messageId) return message;

    let reactions = (message.reactions || []).map((reaction) => ({
      ...reaction,
      user_ids: [...reaction.user_ids],
    }));
    const existing = reactions.find((reaction) => reaction.emoji === emoji);

    if (action === 'add') {
      if (existing && !existing.user_ids.includes(userId)) existing.user_ids.push(userId);
      if (!existing) reactions.push({ emoji, user_ids: [userId] });
    } else if (existing) {
      existing.user_ids = existing.user_ids.filter((id) => id !== userId);
      reactions = reactions.filter((reaction) => reaction.user_ids.length > 0);
    }

    return { ...message, reactions };
  });
}

function mergeConfirmedMessage(messages: UIMessage[], incoming: Message, localId?: string): UIMessage[] {
  let inserted = false;
  // Realtime edits, reactions and reads may already be newer than this HTTP
  // response. Keep the persisted message's current state when it is present.
  const existing = messages.find((message) => message.id === incoming.id);
  const confirmed: UIMessage = { ...incoming, ...existing, _status: 'sent' };
  const merged = messages.flatMap((message) => {
    if (
      message.channel_id !== incoming.channel_id ||
      (message.id !== incoming.id && (!localId || message._local_id !== localId))
    ) return [message];

    // HTTP and WebSocket can confirm the same send in either order. Replace
    // the draft and any copy of the persisted ID together, preserving position.
    if (inserted) return [];
    inserted = true;
    return [confirmed];
  });
  return inserted ? merged : [...merged, confirmed];
}

function mergeRealtimeMessage(messages: UIMessage[], incoming: Message, accountId: string | null): UIMessage[] {
  if (messages.some((message) => message.id === incoming.id)) return messages;

  if (incoming.sender_id === accountId) {
    const optimisticIndex = messages.findIndex(
      (message) =>
        message._status === 'sending' &&
        message.sender_id === accountId &&
        message.channel_id === incoming.channel_id &&
        message.content === incoming.content,
    );
    if (optimisticIndex >= 0) {
      return messages.map((message, index) =>
        index === optimisticIndex ? { ...incoming, _status: 'sent' as const } : message,
      );
    }
  }

  return [...messages, { ...incoming, _status: 'sent' as const }];
}

export function usePortalChannels() {
  const { user } = usePortalAuth();
  // /auth/me returns the same local account used as sender_id by the API.
  // An OTP token's Supabase subject can refer to a different account ID.
  const myAccountId = user?.id || null;
  const [channels, setChannels] = useState<Channel[]>([]);
  const [activeChannel, setActiveChannelState] = useState<Channel | null>(null);
  const [messages, setMessages] = useState<UIMessage[]>([]);
  const [isLoadingChannels, setIsLoadingChannels] = useState(false);
  const [isLoadingMessages, setIsLoadingMessages] = useState(false);
  const [channelsError, setChannelsError] = useState<unknown>(null);
  const [messagesError, setMessagesError] = useState<unknown>(null);
  const [typingUsers, setTypingUsers] = useState<Record<string, string>>({});
  const [brokerInfo, setBrokerInfo] = useState<{ name?: string; avatar?: string } | null>(null);
  const { onMessage, isConnected } = usePortalWebSocket();
  const channelsRef = useRef(channels);
  channelsRef.current = channels;
  const activeChannelIdRef = useRef<string | null>(null);
  const loadChannelsRequestRef = useRef(0);
  const loadMessagesRequestRef = useRef(0);
  const deletedMessageIdsRef = useRef(new Set<string>());
  const typingTimersRef = useRef<Record<string, ReturnType<typeof setTimeout>>>({});

  const setActiveChannel = useCallback((channel: Channel | null) => {
    activeChannelIdRef.current = channel?.id || null;
    setActiveChannelState(channel);
    setTypingUsers({});
    if (channel) {
      setChannels((current) =>
        current.map((item) => (item.id === channel.id ? { ...item, unread_count: 0 } : item)),
      );
    }
  }, []);

  const loadChannels = useCallback(async () => {
    const requestId = ++loadChannelsRequestRef.current;
    try {
      setIsLoadingChannels(true);
      setChannelsError(null);
      const res = await portalApi.get('/channels/mine');
      const list = unwrapPortalData<Channel[]>(res) || [];
      const normalized = sortPortalChannels(
        (Array.isArray(list) ? list : []).map((channel) => normalizePortalChannel(channel, myAccountId)),
      );
      if (requestId !== loadChannelsRequestRef.current) return normalized;
      setChannels(normalized);
      setActiveChannelState((current) => {
        const currentChannel = current && normalized.find((channel) => channel.id === current.id);
        if (currentChannel) {
          activeChannelIdRef.current = currentChannel.id;
          return currentChannel;
        }

        const target = new URLSearchParams(window.location.search).get('channel');
        const next = (target && normalized.find((channel) => channel.id === target)) || normalized[0] || null;
        activeChannelIdRef.current = next?.id || null;
        return next;
      });
      return normalized;
    } catch (error) {
      if (requestId === loadChannelsRequestRef.current) setChannelsError(error);
      throw error;
    } finally {
      if (requestId === loadChannelsRequestRef.current) setIsLoadingChannels(false);
    }
  }, [myAccountId]);

  const loadMessages = useCallback(async (channelId: string) => {
    const requestId = ++loadMessagesRequestRef.current;
    try {
      setIsLoadingMessages(true);
      setMessagesError(null);
      const res = await portalApi.get(`/channels/${channelId}/messages?limit=100&offset=0`);
      const unwrapped = unwrapPortalData<Message[] | { messages?: Message[] }>(res);
      const list = Array.isArray(unwrapped) ? unwrapped : unwrapped?.messages || [];
      if (requestId !== loadMessagesRequestRef.current || activeChannelIdRef.current !== channelId) return;
      setMessages((Array.isArray(list) ? list : []).map((message) => ({ ...message, _status: 'sent' as const })));
    } catch (error) {
      if (requestId !== loadMessagesRequestRef.current) return;
      setMessagesError(error);
      setMessages([]);
      throw error;
    } finally {
      if (requestId === loadMessagesRequestRef.current) setIsLoadingMessages(false);
    }
  }, []);

  const markChannelRead = useCallback(async (channelId: string) => {
    try {
      await portalApi.post(`/channels/${channelId}/read`);
    } catch {
      // Leitura é uma melhoria de estado e não deve bloquear a conversa.
    }
  }, []);

  useEffect(() => {
    import('../services/portal.service')
      .then(({ portalService }) =>
        portalService
          .getOverview()
          .then((overview) => {
            const lead = overview?.[0];
            if (lead?.broker_name) {
              setBrokerInfo({ name: lead.broker_name, avatar: lead.broker_avatar || undefined });
            }
          })
          .catch(() => {}),
      )
      .catch(() => {});
  }, []);

  useEffect(() => {
    void loadChannels().catch(() => {});
  }, [loadChannels, isConnected]);

  useEffect(() => {
    const refresh = () => { if (document.visibilityState !== 'hidden') void loadChannels().catch(() => {}); };
    window.addEventListener('focus', refresh);
    document.addEventListener('visibilitychange', refresh);
    const timer = window.setInterval(refresh, 30000);
    return () => {
      window.removeEventListener('focus', refresh);
      document.removeEventListener('visibilitychange', refresh);
      window.clearInterval(timer);
    };
  }, [loadChannels]);

  useEffect(() => {
    activeChannelIdRef.current = activeChannel?.id || null;
    if (!activeChannel) {
      setMessages([]);
      return;
    }

    setMessages([]);
    void loadMessages(activeChannel.id).catch(() => {});
    void markChannelRead(activeChannel.id);
  }, [activeChannel?.id, loadMessages, markChannelRead]);

  useEffect(() => {
    const timers = typingTimersRef.current;
    return () => Object.values(timers).forEach(clearTimeout);
  }, []);

  useEffect(() => {
    return onMessage((type: string, payload: any) => {
      const currentChannelId = activeChannelIdRef.current;
      const eventChannelId = channelIdOf(payload);

      if (type === 'new_message' && payload) {
        const incoming = payload as Message;
        if (!channelsRef.current.some((channel) => channel.id === incoming.channel_id)) {
          void loadChannels().catch(() => {});
        }
        if (deletedMessageIdsRef.current.has(incoming.id)) return;
        setChannels((current) =>
          sortPortalChannels(
            current.map((channel) =>
              channel.id === incoming.channel_id
                ? {
                    ...channel,
                    last_message: incoming,
                    updated_at: incoming.sent_at,
                    unread_count:
                      incoming.channel_id !== currentChannelId && incoming.sender_id !== myAccountId
                        ? (channel.unread_count || 0) + 1
                        : channel.unread_count,
                  }
                : channel,
            ),
          ),
        );

        if (incoming.channel_id === currentChannelId) {
          setMessages((current) => mergeRealtimeMessage(current, incoming, myAccountId));
          if (incoming.sender_id !== myAccountId) void markChannelRead(incoming.channel_id);
          const timer = typingTimersRef.current[incoming.sender_id];
          if (timer) clearTimeout(timer);
          setTypingUsers((current) => {
            const next = { ...current };
            delete next[incoming.sender_id];
            return next;
          });
        }
        return;
      }

      if (type === 'typing' && eventChannelId === currentChannelId) {
        const userId = payload?.user_id || payload?.sender_id;
        if (!userId || userId === myAccountId) return;
        const existing = typingTimersRef.current[userId];
        if (existing) clearTimeout(existing);
        setTypingUsers((current) => ({ ...current, [userId]: payload?.sender_name || 'A equipe' }));
        typingTimersRef.current[userId] = setTimeout(() => {
          setTypingUsers((current) => {
            const next = { ...current };
            delete next[userId];
            return next;
          });
          delete typingTimersRef.current[userId];
        }, 3000);
        return;
      }

      if ((type === 'reaction_added' || type === 'reaction_removed') && eventChannelId === currentChannelId) {
        setMessages((current) => applyPortalReaction(current, payload, type === 'reaction_added' ? 'add' : 'remove'));
        return;
      }

      if (type === 'message_edited' && eventChannelId === currentChannelId) {
        setMessages((current) =>
          current.map((message) =>
            message.id === payload?.message_id
              ? { ...message, content: payload.content, is_edited: true, edited_at: new Date().toISOString() }
              : message,
          ),
        );
        return;
      }

      if (type === 'message_deleted' && eventChannelId === currentChannelId) {
        if (payload?.message_id) deletedMessageIdsRef.current.add(payload.message_id);
        setMessages((current) => current.filter((message) => message.id !== payload?.message_id));
        return;
      }

      if ((type === 'message_pinned' || type === 'message_unpinned') && eventChannelId === currentChannelId) {
        setMessages((current) =>
          current.map((message) =>
            message.id === payload?.message_id
              ? { ...message, is_pinned: type === 'message_pinned' }
              : message,
          ),
        );
        return;
      }

      if (type === 'messages_read' && eventChannelId === currentChannelId && payload?.user_id) {
        setMessages((current) =>
          current.map((message) => ({
            ...message,
            read_by: message.read_by?.includes(payload.user_id)
              ? message.read_by
              : [...(message.read_by || []), payload.user_id],
          })),
        );
        return;
      }

      if (type === 'channel_created' && payload?.id) {
        void loadChannels().catch(() => {});
        setChannels((current) => {
          if (current.some((channel) => channel.id === payload.id)) return current;
          return sortPortalChannels([normalizePortalChannel(payload as Channel, myAccountId), ...current]);
        });
        return;
      }

      if (type === 'channel_updated' && eventChannelId) {
        setChannels((current) =>
          current.map((channel) =>
            channel.id === eventChannelId
              ? normalizePortalChannel({ ...channel, ...payload }, myAccountId)
              : channel,
          ),
        );
        return;
      }

      if (type === 'channel_removed' && eventChannelId) {
        setChannels((current) => current.filter((channel) => channel.id !== eventChannelId));
        if (eventChannelId === currentChannelId) {
          setActiveChannel(null);
          setMessages([]);
        }
      }
    });
  }, [loadChannels, markChannelRead, myAccountId, onMessage, setActiveChannel]);

  const updateChannelPreview = useCallback((channelId: string, message: Message) => {
    setChannels((current) =>
      sortPortalChannels(
        current.map((channel) =>
          channel.id === channelId
            ? { ...channel, last_message: message, updated_at: message.sent_at }
            : channel,
        ),
      ),
    );
  }, []);

  const sendMessage = useCallback(
    async (
      channelId: string,
      content: string,
      replyTo?: Message | null,
      attachments?: PortalAttachment[],
    ) => {
      const normalizedContent = content.trim() || (attachments?.length ? 'Arquivo enviado' : '');
      if (!normalizedContent) return undefined;

      const localId = `${Date.now()}_${Math.random().toString(16).slice(2)}`;
      const payload: RetryPayload = {
        content: normalizedContent,
        reply_to_id: replyTo?.id,
        attachments: attachments?.length ? attachments : undefined,
        content_type: attachments?.length ? 'file' : 'text',
      };
      const optimistic: UIMessage = {
        id: localId,
        _local_id: localId,
        _status: 'sending',
        _retry_payload: payload,
        channel_id: channelId,
        sender_id: myAccountId || 'me',
        sender_name: 'Você',
        content: normalizedContent,
        content_type: payload.content_type,
        attachments: payload.attachments,
        reply_to_id: payload.reply_to_id,
        reply_to: replyTo
          ? { id: replyTo.id, sender_name: replyTo.sender_name, content: replyTo.content }
          : undefined,
        sent_at: new Date().toISOString(),
        read_by: myAccountId ? [myAccountId] : [],
      };
      setMessages((current) => [...current, optimistic]);

      try {
        const res = await portalApi.post(`/channels/${channelId}/messages`, payload);
        const newMessage = unwrapPortalData<Message>(res);
        if (newMessage && !deletedMessageIdsRef.current.has(newMessage.id)) {
          if (activeChannelIdRef.current === channelId) {
            setMessages((current) => mergeConfirmedMessage(current, newMessage, localId));
          }
          updateChannelPreview(channelId, newMessage);
        }
        return newMessage;
      } catch (error) {
        setMessages((current) =>
          current.map((message) =>
            message._local_id === localId ? { ...message, _status: 'failed' as const } : message,
          ),
        );
        throw error;
      }
    },
    [myAccountId, updateChannelPreview],
  );

  const retryMessage = useCallback(
    async (message: UIMessage) => {
      const channelId = message.channel_id;
      if (activeChannelIdRef.current !== channelId) return;
      const localId = message._local_id || message.id;
      const payload = message._retry_payload || {
        content: message.content,
        reply_to_id: message.reply_to_id,
        attachments: message.attachments,
        content_type: message.content_type,
      };
      setMessages((current) =>
        current.map((item) =>
          (item._local_id || item.id) === localId ? { ...item, _status: 'sending' as const } : item,
        ),
      );
      try {
        const res = await portalApi.post(`/channels/${channelId}/messages`, payload);
        const newMessage = unwrapPortalData<Message>(res);
        if (!newMessage) throw new Error('Resposta inválida ao reenviar mensagem');
        if (deletedMessageIdsRef.current.has(newMessage.id)) return newMessage;
        if (activeChannelIdRef.current === channelId) {
          setMessages((current) => mergeConfirmedMessage(current, newMessage, localId));
        }
        updateChannelPreview(channelId, newMessage);
        return newMessage;
      } catch (error) {
        setMessages((current) =>
          current.map((item) =>
            (item._local_id || item.id) === localId ? { ...item, _status: 'failed' as const } : item,
          ),
        );
        throw error;
      }
    },
    [updateChannelPreview],
  );

  const uploadAttachment = useCallback(async (channelId: string, file: File) => {
    const form = new FormData();
    form.append('file', file);
    const res = await portalApi.post(`/channels/${channelId}/messages/upload`, form, {
      headers: { 'Content-Type': 'multipart/form-data' },
    } as any);
    return unwrapPortalData<PortalAttachment>(res);
  }, []);

  const addReaction = useCallback(async (channelId: string, messageId: string, emoji: string) => {
    await portalApi.post(`/channels/${channelId}/messages/${messageId}/reactions`, { emoji });
    setMessages((current) => applyPortalReaction(current, { message_id: messageId, emoji, user_id: myAccountId || 'me' }, 'add'));
  }, [myAccountId]);

  const removeReaction = useCallback(async (channelId: string, messageId: string, emoji: string) => {
    await portalApi.delete(`/channels/${channelId}/messages/${messageId}/reactions/${encodeURIComponent(emoji)}`);
    setMessages((current) => applyPortalReaction(current, { message_id: messageId, emoji, user_id: myAccountId || 'me' }, 'remove'));
  }, [myAccountId]);

  const editMessage = useCallback(async (channelId: string, messageId: string, content: string) => {
    const res = await portalApi.patch(`/channels/${channelId}/messages/${messageId}`, { content });
    const edited = unwrapPortalData<Message>(res);
    setMessages((current) =>
      current.map((message) =>
        message.id === messageId
          ? { ...message, ...(edited || {}), content, is_edited: true, edited_at: edited?.edited_at || new Date().toISOString() }
          : message,
      ),
    );
    return edited;
  }, []);

  const deleteMessage = useCallback(async (channelId: string, messageId: string) => {
    await portalApi.delete(`/channels/${channelId}/messages/${messageId}`);
    deletedMessageIdsRef.current.add(messageId);
    setMessages((current) => current.filter((message) => message.id !== messageId));
  }, []);

  const togglePinMessage = useCallback(async (channelId: string, messageId: string, pinned: boolean) => {
    await portalApi.patch(`/channels/${channelId}/messages/${messageId}/pin`, { is_pinned: pinned });
    setMessages((current) =>
      current.map((message) => (message.id === messageId ? { ...message, is_pinned: pinned } : message)),
    );
  }, []);

  const togglePinChannel = useCallback(async (channelId: string, pinned: boolean) => {
    await portalApi.patch(`/channels/${channelId}/pin`, { is_pinned: pinned });
    setChannels((current) =>
      sortPortalChannels(
        current.map((channel) => (channel.id === channelId ? { ...channel, is_pinned: pinned } : channel)),
      ),
    );
    setActiveChannelState((current) => (current?.id === channelId ? { ...current, is_pinned: pinned } : current));
  }, []);

  const toggleMuteChannel = useCallback(async (channelId: string, muted: boolean) => {
    await portalApi.patch(`/channels/${channelId}/mute`, { is_muted: muted });
    setChannels((current) =>
      current.map((channel) => (channel.id === channelId ? { ...channel, is_muted: muted } : channel)),
    );
    setActiveChannelState((current) => (current?.id === channelId ? { ...current, is_muted: muted } : current));
  }, []);

  const sendTyping = useCallback(async (channelId: string) => {
    await portalApi.post(`/channels/${channelId}/typing`).catch(() => {});
  }, []);

  return {
    channels,
    activeChannel,
    setActiveChannel,
    messages,
    isLoading: isLoadingChannels || isLoadingMessages,
    isLoadingChannels,
    isLoadingMessages,
    channelsError,
    messagesError,
    error: channelsError || messagesError,
    sendMessage,
    retryMessage,
    uploadAttachment,
    addReaction,
    removeReaction,
    editMessage,
    deleteMessage,
    togglePinMessage,
    togglePinChannel,
    toggleMuteChannel,
    sendTyping,
    myAccountId,
    typingUsers,
    brokerInfo,
    isConnected,
    refreshChannels: loadChannels,
    refreshMessages: () => (activeChannel ? loadMessages(activeChannel.id) : Promise.resolve()),
  };
}
