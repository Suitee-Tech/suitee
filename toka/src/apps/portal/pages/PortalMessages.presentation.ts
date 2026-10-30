import type { Channel, UIMessage } from '../hooks/usePortalChannels';
import { getPortalParticipants } from '../utils/channelParticipants';

export type PortalMessageTimelineItem =
  | { type: 'date'; id: string; date: string }
  | { type: 'message'; id: string; message: UIMessage };

const genericChannelNames = new Set(['', 'Conversa', 'Canal direto', 'Direct']);

export function getPortalMessageSender(
  message: UIMessage,
  channel: Channel | null,
  accountId: string | null,
): { isMe: boolean; name: string; avatar?: string } {
  const isMe = message.sender_id === accountId || message.sender_id === 'me';
  if (isMe) return { isMe, name: 'Você' };

  const participant = channel && getPortalParticipants(channel)
    ?.find((candidate) => candidate.id === message.sender_id);
  const profile = typeof participant === 'object' ? participant : undefined;
  const name = [profile?.name, message.sender_name]
    .map((value) => value?.trim())
    .find((value) => value && value !== message.sender_id && !value.includes('@') && !value.includes('+'));

  // A conversa pode ter vários clientes e corretores. Só use dados ligados ao
  // sender_id: o corretor do overview não identifica o autor desta mensagem.
  return {
    isMe,
    name: name || (message.sender_id === 'system' ? 'Sistema' : 'Participante da conversa'),
    avatar: profile?.avatar_url || message.sender_avatar_url || message.sender_avatar,
  };
}

export function getPortalChannelName(
  channel: Channel | null,
  accountId: string | null,
  brokerName?: string,
): string {
  if (!channel) return 'Mensagens';

  const otherParticipant = getPortalParticipants(channel)
    .find((participant) => participant.id !== accountId);
  if (channel.type === 'direct') {
    if (otherParticipant?.name && otherParticipant.name !== otherParticipant.id) return otherParticipant.name;
    if (brokerName) return brokerName;
    if (!genericChannelNames.has((channel.name || '').trim()) && !/^conversa com\b/i.test(channel.name)) return channel.name;
    return 'Seu corretor';
  }
  if (!genericChannelNames.has((channel.name || '').trim())) return channel.name;
  if (channel.type === 'support') return 'Atendimento';
  if (channel.type === 'deal') return 'Conversa do negócio';
  return 'Equipe da imobiliária';
}

export function getPortalChannelSubtitle(channel: Channel | null, brokerName?: string): string {
  if (!channel) return 'Fale com a equipe responsável';
  if (channel.status === 'closed') return 'Conversa encerrada';
  if (channel.status === 'archived') return 'Conversa arquivada';
  if (channel.type === 'direct') return brokerName ? 'Seu corretor na imobiliária' : 'Atendimento da imobiliária';
  if (channel.type === 'support') return 'Atendimento da imobiliária';
  if (channel.type === 'deal') return 'Assunto ligado ao seu negócio';
  const count = channel.participants?.length || 0;
  return count > 0 ? `${count} participantes` : 'Atendimento da imobiliária';
}

export function getPortalInitials(value: string): string {
  const initials = value
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0])
    .join('')
    .toUpperCase();
  return initials || '?';
}

export function formatPortalConversationTime(value?: string): string {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  const now = new Date();
  if (date.toDateString() === now.toDateString()) {
    return date.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
  }
  return date.toLocaleDateString('pt-BR', { day: '2-digit', month: '2-digit' });
}

export function formatPortalMessageDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  const now = new Date();
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (date.toDateString() === now.toDateString()) return 'Hoje';
  if (date.toDateString() === yesterday.toDateString()) return 'Ontem';
  return date.toLocaleDateString('pt-BR', { day: '2-digit', month: 'long', year: 'numeric' });
}

export function buildPortalMessageTimeline(messages: UIMessage[]): PortalMessageTimelineItem[] {
  const items: PortalMessageTimelineItem[] = [];
  let previousDay = '';

  for (const message of messages) {
    const parsed = new Date(message.sent_at);
    const day = Number.isNaN(parsed.getTime()) ? 'unknown' : parsed.toDateString();
    if (day !== previousDay) {
      items.push({ type: 'date', id: `date-${day}-${message.id}`, date: message.sent_at });
      previousDay = day;
    }
    items.push({ type: 'message', id: message._local_id || message.id, message });
  }

  return items;
}

export function filterPortalMessages(messages: UIMessage[], query: string): UIMessage[] {
  const normalizedQuery = query.trim().toLocaleLowerCase('pt-BR');
  if (!normalizedQuery) return messages;
  return messages.filter((message) =>
    [message.content, message.sender_name, ...((message.attachments || []).map((attachment) => attachment.name))]
      .filter(Boolean)
      .some((value) => String(value).toLocaleLowerCase('pt-BR').includes(normalizedQuery)),
  );
}

export function getPortalChannelAvatar(channel: Channel, accountId: string | null): string | undefined {
 if (channel.type !== 'direct') return undefined;
 const other = getPortalParticipants(channel).find((p) => p.id !== accountId);
 return typeof other === 'object' ? other.avatar_url : undefined;
}
