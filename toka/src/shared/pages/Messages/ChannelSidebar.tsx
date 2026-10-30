import { getOtherChannelParticipant } from "../../services/message.service";
import { ActivitySquare, BellOff, CheckCheck, Loader2, MessageSquare, Pin, Plus, Search, X } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { clsx } from "clsx";

import { getMediaUrl } from "../../media";
import type { Channel, ChannelType, Message } from "../../services/message.service";
import { Avatar, AvatarFallback, AvatarImage } from "../../ui/avatar";
import { Button } from "../../ui/button";
import {
  CHANNEL_TYPE_META,
  avatarColor,
  formatChannelTime,
  getInitials,
} from "./presentation";

interface ChannelSidebarProps {
  activeChannelId: string | null;
  showMetrics: boolean;
  setShowMetrics: (value: boolean) => void;
  isCreating: boolean;
  setIsCreating: (value: boolean) => void;
  totalUnread: number;
  channelSearch: string;
  setChannelSearch: (value: string) => void;
  messageSearch: string;
  setMessageSearch: (value: string) => void;
  handleSearchMessages: (query: string) => void | Promise<void>;
  isSearchingMessages: boolean;
  searchResults: Message[];
  setSearchResults: (messages: Message[]) => void;
  hasSearched: boolean;
  setHasSearched: (value: boolean) => void;
  channelTypeCounts: Record<string, number>;
  channelFilter: ChannelType | "all" | "archived";
  setChannelFilter: (value: ChannelType | "all" | "archived") => void;
  newChannelName: string;
  setNewChannelName: (value: string) => void;
  newChannelDescription: string;
  setNewChannelDescription: (value: string) => void;
  newChannelType: ChannelType;
  setNewChannelType: (value: ChannelType) => void;
  handleCreateChannel: () => void | Promise<void>;
  isLoading: boolean;
  channels: Channel[];
  filteredChannels: Channel[];
  handleSelectChannel: (id: string) => void;
  resolveParticipantAvatar: (id: string, fallbackUrl?: string) => string | undefined;
  resolveParticipantName: (id: string, name?: string) => string;
  userId: string;
}

export function ChannelSidebar({
  activeChannelId,
  showMetrics,
  setShowMetrics,
  isCreating,
  setIsCreating,
  totalUnread,
  channelSearch,
  setChannelSearch,
  messageSearch,
  setMessageSearch,
  handleSearchMessages,
  isSearchingMessages,
  searchResults,
  setSearchResults,
  hasSearched,
  setHasSearched,
  channelTypeCounts,
  channelFilter,
  setChannelFilter,
  newChannelName,
  setNewChannelName,
  newChannelDescription,
  setNewChannelDescription,
  newChannelType,
  setNewChannelType,
  handleCreateChannel,
  isLoading,
  channels,
  filteredChannels,
  handleSelectChannel,
  resolveParticipantAvatar,
  resolveParticipantName,
  userId,
}: ChannelSidebarProps) {
  return (
      <div
        className={clsx(
          "w-full md:w-80 lg:w-[340px] flex-shrink-0 border-r border-border flex-col h-full bg-card transition-all",
          activeChannelId || showMetrics || (hasSearched && messageSearch.trim()) ? "hidden md:flex" : "flex",
        )}
      >
        <div className="flex-none px-4 pt-5 pb-3">
          <div className="flex justify-between items-center mb-4">
            <div className="flex items-center gap-2">
              <h1 className="text-xl font-bold text-foreground dark:text-white">Mensagens</h1>
              {totalUnread > 0 && (
                <span className="px-2 py-0.5 bg-primary text-primary-foreground text-caption font-bold rounded-full min-w-[20px] text-center">
                  {totalUnread > 99 ? "99+" : totalUnread}
                </span>
              )}
            </div>
            <div className="flex items-center gap-1">
              <Button variant={showMetrics ? "default" : "secondary"} size="icon" onClick={() => setShowMetrics(!showMetrics)} className="size-9" title="Métricas (Admin)">
                <ActivitySquare className="w-4 h-4" />
              </Button>
              <Button variant={isCreating ? "default" : "secondary"} size="icon" onClick={() => setIsCreating(!isCreating)} className="size-9" title="Novo Canal">
                <Plus className="w-4 h-4" />
              </Button>
            </div>
          </div>

          <div className="flex flex-col gap-2 mb-3">
            <div className="relative">
              <Search className="w-4 h-4 text-muted-foreground absolute left-3 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                placeholder="Buscar conversas..."
                value={channelSearch}
                onChange={(e) => setChannelSearch(e.target.value)}
                className="w-full pl-9 pr-4 py-2 bg-muted border-0 rounded-xl text-sm text-foreground placeholder:text-muted-foreground focus:ring-2 focus:ring-primary-500 outline-none transition-all"
              />
            </div>
            <div className="relative">
              <Search className="w-4 h-4 text-muted-foreground absolute left-3 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                placeholder="Buscar em todas mensagens..."
                value={messageSearch}
                onChange={(e) => setMessageSearch(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    handleSearchMessages(messageSearch);
                  }
                }}
                className="w-full pl-9 pr-8 py-2 bg-muted border-0 rounded-xl text-sm text-foreground placeholder:text-muted-foreground focus:ring-2 focus:ring-primary-500 outline-none transition-all"
              />
              {isSearchingMessages ? (
                <Loader2 className="w-4 h-4 absolute right-3 top-1/2 -translate-y-1/2 text-primary-500 animate-spin" />
              ) : messageSearch ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={() => {
                    setMessageSearch("");
                    setSearchResults([]);
                    setHasSearched(false);
                  }}
                  className="absolute right-2 top-1/2 -translate-y-1/2 size-7 text-muted-foreground"
                >
                  <X className="w-4 h-4" />
                </Button>
              ) : null}
            </div>
            {hasSearched && !isSearchingMessages && messageSearch.trim() && (
              <div className="text-xs text-muted-foreground pl-1 mt-1 flex justify-between items-center">
                <span>{searchResults.length} resultados encontrados</span>
              </div>
            )}
          </div>

          <div className="flex gap-1.5 overflow-x-auto pb-1 scrollbar-hide">
            {[
              { key: "all" as const, label: "Todos" },
              ...Object.entries(CHANNEL_TYPE_META)
                .filter(([key]) => channelTypeCounts[key as string] > 0)
                .map(([key, meta]) => ({ key: key as ChannelType, label: meta.label })),
              ...(channelTypeCounts["archived"] > 0 ? [{ key: "archived" as const, label: "Arquivados" }] : []),
            ].map(({ key, label }) => (
              <button
                key={key}
                onClick={() => setChannelFilter(key as ChannelType | "all" | "archived")}
                className={clsx(
                  "px-3 py-1.5 rounded-lg text-xs font-semibold whitespace-nowrap transition-colors border",
                  channelFilter === key ? "bg-primary text-primary-foreground border-primary" : "bg-card text-muted-foreground border-border hover:bg-muted",
                )}
              >
                {label}
                {channelTypeCounts[key as string] !== undefined && channelTypeCounts[key as string] > 0 ? ` (${channelTypeCounts[key as string]})` : ""}
              </button>
            ))}
          </div>
        </div>

        <AnimatePresence>
          {isCreating && (
            <motion.div initial={{ height: 0, opacity: 0 }} animate={{ height: "auto", opacity: 1 }} exit={{ height: 0, opacity: 0 }} className="overflow-hidden border-b border-border ">
              <div className="p-4 space-y-3 bg-muted /50">
                <input
                  type="text"
                  placeholder="Nome do canal"
                  value={newChannelName}
                  onChange={(e) => setNewChannelName(e.target.value)}
                  className="w-full px-3 py-2 text-sm border border-border rounded-lg bg-card text-foreground focus:ring-2 focus:ring-primary-500 outline-none"
                  autoFocus
                />
                <input
                  type="text"
                  placeholder="Descrição (opcional)"
                  value={newChannelDescription}
                  onChange={(e) => setNewChannelDescription(e.target.value)}
                  className="w-full px-3 py-2 text-sm border border-border rounded-lg bg-card text-foreground focus:ring-2 focus:ring-primary-500 outline-none"
                />
                <div className="grid grid-cols-3 gap-1.5">
                  {(Object.entries(CHANNEL_TYPE_META) as [ChannelType, (typeof CHANNEL_TYPE_META)[ChannelType]][]).map(([key, meta]) => {
                    const Icon = meta.icon;
                    return (
                      <button
                        key={key}
                        onClick={() => setNewChannelType(key)}
                        className={clsx(
                          "flex items-center gap-1.5 px-2 py-1.5 rounded-lg text-[11px] font-semibold transition-colors",
                          newChannelType === key ? "bg-primary text-primary-foreground" : "bg-card border border-border text-muted-foreground hover:border-primary-300",
                        )}
                      >
                        <Icon className="w-3 h-3" />
                        {meta.label}
                      </button>
                    );
                  })}
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" onClick={() => setIsCreating(false)} className="flex-1">
                    Cancelar
                  </Button>
                  <Button onClick={handleCreateChannel} disabled={!newChannelName.trim()} className="flex-1">
                    Criar Canal
                  </Button>
                </div>
              </div>
            </motion.div>
          )}
        </AnimatePresence>

        <div className="flex-1 overflow-y-auto">
          {isLoading && channels.length === 0 ? (
            <div className="flex items-center justify-center py-16">
              <Loader2 className="w-6 h-6 text-primary-500 animate-spin" />
            </div>
          ) : filteredChannels.length === 0 ? (
            <div className="p-8 text-center">
              <MessageSquare className="w-10 h-10 text-muted-foreground/60 mx-auto mb-3" />
              <p className="text-sm text-muted-foreground font-medium">{channelSearch ? "Nenhuma conversa encontrada" : "Nenhum canal ainda"}</p>
              <p className="text-xs text-muted-foreground mt-1">Clique em + para criar um novo canal</p>
            </div>
          ) : (
            filteredChannels.map((channel) => {
              const typeMeta = CHANNEL_TYPE_META[channel.type] || CHANNEL_TYPE_META.group;
              const TypeIcon = typeMeta.icon;
              const otherParticipant = getOtherChannelParticipant(channel, userId);
              const displayName = channel.type === 'direct' && otherParticipant
                ? resolveParticipantName(otherParticipant.id, otherParticipant.name) : channel.name;
              const displayAvatar = channel.type === 'direct' && otherParticipant
                ? resolveParticipantAvatar(otherParticipant.id, otherParticipant.avatar_url) : channel.avatar_url;
              const isActive = activeChannelId === channel.id;
              const hasUnread = (channel.unread_count || 0) > 0;

              return (
                <button
                  key={channel.id}
                  onClick={() => handleSelectChannel(channel.id)}
                  className={clsx(
                    "w-full text-left px-4 py-3.5 flex items-start gap-3 transition-all border-b border-border /50",
                    isActive ? "bg-primary-50 dark:bg-primary-900/20 border-l-[3px] border-l-primary-600" : "hover:bg-muted border-l-[3px] border-l-transparent",
                  )}
                >
                  <div className="relative flex-shrink-0">
                    {channel.type === "direct" ? (
                      <Avatar className="w-12 h-12">
                        {displayAvatar && (
                          <AvatarImage src={getMediaUrl(displayAvatar!)} alt="Avatar" />
                        )}
                        <AvatarFallback className={clsx("text-white font-bold text-sm", avatarColor(channel.id))}>
                          {getInitials(displayName)}
                        </AvatarFallback>
                      </Avatar>
                    ) : (
                      <div className={clsx("w-12 h-12 rounded-full flex items-center justify-center text-white font-bold text-sm", avatarColor(channel.id))}>
                        <TypeIcon className="w-5 h-5" />
                      </div>
                    )}
                    {channel.is_pinned && (
                      <span className="absolute -top-1 -right-1 w-4 h-4 bg-status-warning/80 rounded-full flex items-center justify-center">
                        <Pin className="w-2.5 h-2.5 text-white" />
                      </span>
                    )}
                  </div>

                  <div className="flex-1 min-w-0">
                    <div className="flex justify-between items-baseline mb-0.5">
                      <h3 className={clsx("text-sm truncate pr-2", hasUnread ? "font-bold text-foreground dark:text-white" : "font-semibold text-foreground ")}>
                        {displayName}
                      </h3>
                      <span
                        className={clsx(
                          "text-[11px] whitespace-nowrap ml-2 flex-shrink-0",
                          hasUnread ? "text-primary-600 dark:text-primary-400 font-semibold" : "text-muted-foreground",
                        )}
                      >
                        {channel.last_message ? formatChannelTime(channel.last_message.sent_at) : ""}
                      </span>
                    </div>
                    <div className="flex items-center justify-between gap-2">
                      <p className={clsx("text-[13px] truncate", hasUnread ? "text-foreground font-medium" : "text-muted-foreground")}>
                        {channel.last_message ? (
                          <>
                            {channel.last_message.sender_id === userId && <CheckCheck className="w-3.5 h-3.5 inline mr-1 text-primary-500" />}
                            {channel.last_message.content}
                          </>
                        ) : (
                          <span className="italic text-muted-foreground">Sem mensagens</span>
                        )}
                      </p>
                      <div className="flex items-center gap-1.5 flex-shrink-0">
                        {channel.is_muted && <BellOff className="w-3.5 h-3.5 text-muted-foreground" />}
                        {hasUnread && (
                          <span className="px-1.5 py-0.5 bg-primary text-primary-foreground text-caption font-bold rounded-full min-w-[18px] text-center">
                            {channel.unread_count! > 99 ? "99+" : channel.unread_count}
                          </span>
                        )}
                      </div>
                    </div>
                  </div>
                </button>
              );
            })
          )}
        </div>
      </div>

  );
}

