import api from "./api";

export type ChannelType = "direct" | "group" | "support" | "lead" | "deal" | "property" | "contract" | "team" | "partner";
export type ChannelStatus = "open" | "archived" | "closed";
export type ContentType = "text" | "image" | "file" | "audio" | "video" | "system" | "location";
export type ParticipantRole = "admin" | "member" | "observer";

export interface Participant {
  id: string;
  name: string;
  role: ParticipantRole;
  avatar_url?: string;
  entity_type?: string;
  joined_at: string;
  last_seen?: string;
  is_online?: boolean;
}

export interface Reaction {
  emoji: string;
  user_ids: string[];
}

export interface Attachment {
  id: string;
  name: string;
  url: string;
  type: string;
  size: number;
}

export interface Message {
  id: string;
  channel_id: string;
  tenant_id: string;
  sender_id: string;
  sender_name: string;
  sender_avatar?: string;
  sender_avatar_url?: string;
  content: string;
  content_type: ContentType;
  attachments?: Attachment[];
  reply_to_id?: string;
  reply_to?: {
    id: string;
    sender_name: string;
    content: string;
  };
  reactions?: Reaction[];
  is_pinned?: boolean;
  is_edited?: boolean;
  sent_at: string;
  edited_at?: string;
  read_by: string[];
  is_flagged?: boolean;
}

export interface Metrics {
  total_messages: number;
  total_channels: number;
  active_users: number;
  flagged_messages: number;
  avg_response_time_minutes: number;
}

export interface ChannelInfoItem {
  label: string;
  value?: string;
  link?: string;
}

export interface ChannelInfoSection {
  label: string;
  items: ChannelInfoItem[];
}

export interface ChannelInfo {
  sections: ChannelInfoSection[];
}

export interface Channel {
  id: string;
  tenant_id: string;
  name: string;
  description?: string;
  avatar_url?: string;
  type: ChannelType;
  status: ChannelStatus;
  participants: Participant[];
  participant_profiles?: Pick<Participant, "id" | "name" | "avatar_url">[];
  entity_id?: string;
  entity_type?: string;
  created_by: string;
  created_at: string;
  updated_at: string;
  last_message?: Message;
  unread_count?: number;
  is_pinned?: boolean;
  is_muted?: boolean;
  pinned_by?: string[];
  muted_by?: string[];
  info?: ChannelInfo;
}

export interface CreateChannelDTO {
  name: string;
  type: ChannelType;
  description?: string;
  participants?: string[];
  entity_id?: string;
  entity_type?: string;
}

export interface SendMessageDTO {
  content: string;
  content_type?: ContentType;
  reply_to_id?: string;
  attachments?: { name: string; url: string; type: string; size: number }[];
}

export interface AddParticipantDTO {
  participant_id: string;
  role?: ParticipantRole;
}

export interface UpdateChannelDTO {
  name?: string;
  description?: string;
  status?: ChannelStatus;
  is_pinned?: boolean;
  is_muted?: boolean;
  info?: ChannelInfo;
}

export interface MessageTemplate {
  id: string;
  tenant_id: string;
  name: string;
  category: string;
  content: string;
  variables?: string[];
  created_by: string;
  created_at: string;
}

export function normalizeMessageChannel(channel: Channel): Channel {
 const profiles = new Map((channel.participant_profiles || []).map((profile) => [profile.id, profile]));
 return {
  ...channel,
  participants: (channel.participants || []).map((participant) => {
   const base = typeof participant === 'string' ? { id: participant, name: participant, role: 'member' as const, joined_at: '' } : participant;
   return { ...base, ...profiles.get(base.id) };
  }),
 };
}

export function getOtherChannelParticipant(channel: Channel, accountId: string): Participant | undefined {
 return normalizeMessageChannel(channel).participants.find((participant) => participant.id !== accountId);
}

class MessageService {
  private readonly baseUrl = "/channels";

  async listTemplates(): Promise<MessageTemplate[]> {
    const response = await api.get<{ data: MessageTemplate[] }>("/templates");
    return response.data?.data || [];
  }

  async listChannels(entityId?: string): Promise<Channel[]> {
    const params: Record<string, string> = {};
    if (entityId) params.entity_id = entityId;
    const response = await api.get<{ data: Channel[] }>(this.baseUrl, { params });
    return (response.data?.data || []).map(normalizeMessageChannel);
  }

  async getChannel(id: string): Promise<Channel> {
    const response = await api.get<{ data: Channel }>(`${this.baseUrl}/${id}`);
    return normalizeMessageChannel(response.data?.data);
  }

  async createChannel(data: CreateChannelDTO): Promise<Channel> {
    const response = await api.post<{ data: Channel }>(this.baseUrl, data);
    return response.data?.data;
  }

  async updateChannel(id: string, data: UpdateChannelDTO): Promise<Channel> {
    const response = await api.patch<{ data: Channel }>(`${this.baseUrl}/${id}`, data);
    return response.data?.data;
  }

  async updateChannelInfo(id: string, info: ChannelInfo): Promise<Channel> {
    return this.updateChannel(id, { info });
  }

  async getMessages(channelId: string, limit = 50, offset = 0): Promise<{ messages: Message[]; total: number }> {
    const response = await api.get<{ data: { messages: Message[]; total: number } }>(`${this.baseUrl}/${channelId}/messages?limit=${limit}&offset=${offset}`);
    return response.data?.data;
  }

  async getMetrics(): Promise<Metrics> {
    const response = await api.get<{ data: Metrics }>("/metrics");
    return response.data?.data;
  }

  async uploadAttachment(channelId: string, file: File): Promise<Attachment> {
    const formData = new FormData();
    formData.append("file", file);

    const response = await api.post<{ data: Attachment }>(`${this.baseUrl}/${channelId}/messages/upload`, formData);
    return response.data?.data;
  }

  async sendMessage(channelId: string, data: SendMessageDTO): Promise<Message> {
    const response = await api.post<{ data: Message }>(`${this.baseUrl}/${channelId}/messages`, data);
    return response.data?.data;
  }

  async editMessage(channelId: string, messageId: string, content: string): Promise<Message> {
    const response = await api.patch<{ data: Message }>(`${this.baseUrl}/${channelId}/messages/${messageId}`, { content });
    return response.data?.data;
  }

  async deleteMessage(channelId: string, messageId: string): Promise<void> {
    await api.delete(`${this.baseUrl}/${channelId}/messages/${messageId}`);
  }

  async addReaction(channelId: string, messageId: string, emoji: string): Promise<void> {
    await api.post(`${this.baseUrl}/${channelId}/messages/${messageId}/reactions`, { emoji });
  }

  async removeReaction(channelId: string, messageId: string, emoji: string): Promise<void> {
    await api.delete(`${this.baseUrl}/${channelId}/messages/${messageId}/reactions/${encodeURIComponent(emoji)}`);
  }

  async searchMessages(query: string, channelId?: string, hashtag?: string, mention?: string) {
    let url = `/messages/search?q=${encodeURIComponent(query)}`;
    if (channelId) url += `&channelId=${channelId}`;
    if (hashtag) url += `&hashtag=${encodeURIComponent(hashtag)}`;
    if (mention) url += `&mention=${encodeURIComponent(mention)}`;
    const response = await api.get(url);
    return response.data?.data?.messages || [];
  }

  async togglePinMessage(channelId: string, messageId: string, isPinned: boolean) {
    const response = await api.patch(`${this.baseUrl}/${channelId}/messages/${messageId}/pin`, { is_pinned: isPinned });
    return response.data;
  }

  async markRead(channelId: string): Promise<void> {
    await api.post(`${this.baseUrl}/${channelId}/read`);
  }

  async addParticipant(channelId: string, data: AddParticipantDTO): Promise<void> {
    await api.post(`${this.baseUrl}/${channelId}/participants`, data);
  }

  async removeParticipant(channelId: string, participantId: string): Promise<void> {
    await api.delete(`${this.baseUrl}/${channelId}/participants/${participantId}`);
  }

  async updateChannelStatus(channelId: string, status: string): Promise<void> {
    await api.patch(`${this.baseUrl}/${channelId}/status`, { status });
  }

  async togglePinChannel(channelId: string, isPinned: boolean): Promise<void> {
    await api.patch(`${this.baseUrl}/${channelId}/pin`, { is_pinned: isPinned });
  }

  async toggleMuteChannel(channelId: string, isMuted: boolean): Promise<void> {
    await api.patch(`${this.baseUrl}/${channelId}/mute`, { is_muted: isMuted });
  }

  async sendTyping(channelId: string): Promise<void> {
    await api.post(`${this.baseUrl}/${channelId}/typing`).catch(() => { });
  }

  async uploadFile(channelId: string, file: File): Promise<Attachment> {
    const formData = new FormData();
    formData.append("file", file);
    const response = await api.post<{ data: Attachment }>(`${this.baseUrl}/${channelId}/messages/upload`, formData);
    return response.data?.data;
  }
}

export const messageService = new MessageService();

