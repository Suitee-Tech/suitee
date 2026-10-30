import { Dialog, DialogContent, DialogTitle } from "../../../shared/ui/dialog";
import { notifyApiError } from "../../../shared/services/errorToast";
import { useConfirmAction } from "../../../shared/hooks/useConfirmAction";
import { useSearchParams } from "react-router";
import { PortalContactChooser } from "../components/PortalContactChooser";
import { getMediaUrl } from '../../../shared/media';
import { useEffect, useMemo, useRef, useState } from 'react';
import {
  AlertCircle,
  ArrowLeft,
  Bell,
  BellOff,
  Check,
  CheckCheck,
  Copy,
  Edit3,
  FileText,
  Loader2,
  Menu,
  MessageSquare,
  MoreHorizontal,
  Paperclip,
  Pin,
  PinOff,
  RefreshCw,
  Reply,
  Search,
  Send,
  Smile,
  Trash2,
  Wifi,
  WifiOff,
  X,
} from 'lucide-react';
import { toast } from 'sonner';
import { clsx } from 'clsx';
import {
  usePortalChannels,
  type Channel,
  type PortalAttachment,
  type UIMessage,
} from '../hooks/usePortalChannels';
import { getPortalAccessToken } from '../services/portal-api';
import { withGatewayToken } from '../../../shared/services/api';
import { fetchPortalContent } from '../services/portal-content';
import { Button } from '../../../shared/ui/button';
import { DocumentViewerDialog } from '../../../shared/components/DocumentViewerDialog';
import { PortalEmpty, PortalError } from '../components/PortalState';
import { PortalPage } from "../shell/PortalPage";
import { PortalPageHeader } from "../shell/PortalPageHeader";
import { usePortalBack } from "../shell/usePortalBack";
import {
  buildPortalMessageTimeline,
  filterPortalMessages,
  formatPortalConversationTime,
  formatPortalMessageDate,
  getPortalChannelName,
  getPortalChannelAvatar,
  getPortalChannelSubtitle,
  getPortalInitials,
  getPortalMessageSender,
} from './PortalMessages.presentation';

const QUICK_REACTIONS = ['👍', '❤️', '😂', '🔥', '👏', '😮'];

function isImageAttachment(attachment: PortalAttachment) {
  return Boolean(
    attachment.type?.startsWith('image/') || /\.(png|jpe?g|gif|webp|avif)$/i.test(attachment.url || ''),
  );
}

function formatFileSize(size?: number) {
  if (!size) return '';
  if (size < 1024 * 1024) return `${Math.max(1, Math.round(size / 1024))} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

export function PortalMessages() {
  const { confirmAction, confirmationDialog } = useConfirmAction();
  const [searchParams, setSearchParams] = useSearchParams();
  const requestedChannel = searchParams.get("channel");
  const goBack = usePortalBack("/portal");
  const {
    channels,
    activeChannel,
    setActiveChannel,
    messages,
    isLoading,
    isLoadingChannels,
    isLoadingMessages,
    channelsError,
    messagesError,
    refreshChannels,
    refreshMessages,
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
  } = usePortalChannels();

  const [input, setInput] = useState('');
  const [replyTo, setReplyTo] = useState<UIMessage | null>(null);
  const [editingMessage, setEditingMessage] = useState<UIMessage | null>(null);
  const [pendingAttachments, setPendingAttachments] = useState<PortalAttachment[]>([]);
  const [uploading, setUploading] = useState(false);
  const [emojiOpen, setEmojiOpen] = useState(false);
  const [channelMenuOpen, setChannelMenuOpen] = useState(false);
  const menuOpener = useRef<HTMLButtonElement | null>(null);
  const menuFocusComposer = useRef(false);
  const messageSearchRef = useRef<HTMLInputElement | null>(null);
  const [messageMenu, setMessageMenu] = useState<UIMessage | null>(null);
  const [channelSearch, setChannelSearch] = useState('');
  const [messageSearch, setMessageSearch] = useState('');
  const [showMessageSearch, setShowMessageSearch] = useState(false);
  const [showMobileChannels, setShowMobileChannels] = useState(searchParams.get("contact") === "1");
  const [viewer, setViewer] = useState<{ url: string; title: string; contentType?: string } | null>(null);
  const fileRef = useRef<HTMLInputElement | null>(null);
  const messagesEndRef = useRef<HTMLDivElement | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const lastTypingAtRef = useRef(0);
  useEffect(() => { if (showMessageSearch) messageSearchRef.current?.focus(); }, [showMessageSearch]);

  const channelName = getPortalChannelName(activeChannel, myAccountId, brokerInfo?.name);
  const channelSubtitle = getPortalChannelSubtitle(activeChannel, brokerInfo?.name);
  const pinnedCount = messages.filter((message) => message.is_pinned).length;

  const filteredChannels = useMemo(() => {
    const query = channelSearch.trim().toLocaleLowerCase('pt-BR');
    if (!query) return channels;
    return channels.filter((channel) =>
      [getPortalChannelName(channel, myAccountId, brokerInfo?.name), channel.last_message?.content]
        .filter(Boolean)
        .some((value) => String(value).toLocaleLowerCase('pt-BR').includes(query)),
    );
  }, [brokerInfo?.name, channelSearch, channels, myAccountId]);

  const visibleMessages = useMemo(
    () => filterPortalMessages(messages, messageSearch),
    [messageSearch, messages],
  );
  const timeline = useMemo(() => buildPortalMessageTimeline(visibleMessages), [visibleMessages]);

  const resolveAttachmentUrl = (attachment: PortalAttachment) => {
    let rawUrl = attachment.url || '';
    if (activeChannel && /toca-message-attachments/i.test(rawUrl)) {
      const clean = rawUrl.split('#')[0]?.split('?')[0] || rawUrl;
      const filename = clean.split('/').pop();
      if (filename) {
        rawUrl = `/api/v1/channels/${activeChannel.id}/messages/attachments/${encodeURIComponent(filename)}/download`;
      }
    }
    return withGatewayToken(rawUrl, getPortalAccessToken());
  };

  useEffect(() => {
    const target = messagesEndRef.current;
    if (target && typeof target.scrollIntoView === 'function') {
      target.scrollIntoView({ behavior: messages.length > 1 ? 'smooth' : 'auto' });
    }
  }, [messages]);

  useEffect(() => {
    if (!requestedChannel) return;
    const target = channels.find((channel) => channel.id === requestedChannel);
    if (target && target.id !== activeChannel?.id) {
      setActiveChannel(target);
      setShowMobileChannels(false);
    }
  }, [requestedChannel, channels, setActiveChannel, activeChannel?.id]);

  useEffect(() => {
    if (requestedChannel) void refreshChannels().catch(() => {});
  }, [requestedChannel, refreshChannels]);

  useEffect(() => {
    if (searchParams.get('contact') === '1') setShowMobileChannels(true);
  }, [searchParams]);

  useEffect(() => {
    setInput('');
    setReplyTo(null);
    setEditingMessage(null);
    setPendingAttachments([]);
    setMessageMenu(null);
    setMessageSearch('');
    setShowMessageSearch(false);
  }, [activeChannel?.id]);

  const selectChannel = (channel: Channel) => {
    setActiveChannel(channel);
    setShowMobileChannels(false);
    setSearchParams({ channel: channel.id }, { replace: true });
  };

  const handleInputChange = (value: string) => {
    setInput(value);
    if (!activeChannel || !value.trim()) return;
    const now = Date.now();
    if (now - lastTypingAtRef.current < 1500) return;
    lastTypingAtRef.current = now;
    void sendTyping(activeChannel.id);
  };

  const handleSubmit = async (event?: React.FormEvent) => {
    event?.preventDefault();
    if (!activeChannel || uploading) return;
    const text = input.trim();

    if (editingMessage) {
      if (!text) return;
      try {
        await editMessage(activeChannel.id, editingMessage.id, text);
        setEditingMessage(null);
        setInput('');
        toast.success('Mensagem editada.');
      } catch {
        toast.error('Não foi possível editar a mensagem.');
      }
      return;
    }

    if (!text && pendingAttachments.length === 0) return;
    const currentReply = replyTo;
    const currentAttachments = pendingAttachments;
    setInput('');
    setReplyTo(null);
    setPendingAttachments([]);
    setEmojiOpen(false);
    try {
      await sendMessage(activeChannel.id, text, currentReply, currentAttachments);
    } catch {
      toast.error('Não foi possível enviar. Use “Reenviar” na mensagem.');
    }
  };

  const handleFileSelect = async (files: FileList | null) => {
    if (!files?.length || !activeChannel) return;
    setUploading(true);
    try {
      const uploaded = await Promise.all(
        Array.from(files).map((file) => uploadAttachment(activeChannel.id, file)),
      );
      setPendingAttachments((current) => [
        ...current,
        ...uploaded.filter((attachment): attachment is PortalAttachment => Boolean(attachment?.url)),
      ]);
    } catch (error: any) {
      notifyApiError(error, { context: "Anexar arquivo" });
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = '';
    }
  };

  const handleReaction = async (message: UIMessage, emoji: string) => {
    if (!activeChannel || message._status !== 'sent') return;
    const hasMyReaction = Boolean(
      message.reactions
        ?.find((reaction) => reaction.emoji === emoji)
        ?.user_ids.includes(myAccountId || 'me'),
    );
    try {
      if (hasMyReaction) await removeReaction(activeChannel.id, message.id, emoji);
      else await addReaction(activeChannel.id, message.id, emoji);
    } catch {
      toast.error('Não foi possível atualizar a reação.');
    } finally {
      setMessageMenu(null);
    }
  };

  const startReply = (message: UIMessage) => {
    menuFocusComposer.current = true;
    setReplyTo(message);
    setEditingMessage(null);
    setMessageMenu(null);
    textareaRef.current?.focus();
  };

  const startEdit = (message: UIMessage) => {
    menuFocusComposer.current = true;
    setEditingMessage(message);
    setReplyTo(null);
    setInput(message.content);
    setMessageMenu(null);
    textareaRef.current?.focus();
  };

  const copyMessage = async (message: UIMessage) => {
    try {
      await navigator.clipboard.writeText(message.content);
      toast.success('Mensagem copiada.');
    } catch {
      toast.error('Não foi possível copiar a mensagem.');
    } finally {
      setMessageMenu(null);
    }
  };

  const handleDelete = (message: UIMessage) => {
    if (!activeChannel) return;
    const channelId = activeChannel.id;
    confirmAction("Apagar esta mensagem? Esta ação não pode ser desfeita.", async () => {
      try {
        await deleteMessage(channelId, message.id);
        setMessageMenu(null);
        toast.success("Mensagem apagada.");
      } catch (error) {
        toast.error("Não foi possível apagar a mensagem.");
        throw error;
      }
    }, { title: "Apagar mensagem", confirmLabel: "Apagar", variant: "danger" });
  };

  const handleToggleMessagePin = async (message: UIMessage) => {
    setMessageMenu(null);
    if (!activeChannel) return;
    try {
      await togglePinMessage(activeChannel.id, message.id, !message.is_pinned);
      toast.success(message.is_pinned ? 'Mensagem desafixada.' : 'Mensagem fixada.');
    } catch {
      toast.error('Não foi possível atualizar a mensagem fixada.');
    }
  };

  const handleToggleChannelPin = async () => {
    if (!activeChannel) return;
    try {
      await togglePinChannel(activeChannel.id, !activeChannel.is_pinned);
      toast.success(activeChannel.is_pinned ? 'Conversa desafixada.' : 'Conversa fixada.');
      setChannelMenuOpen(false);
    } catch {
      toast.error('Não foi possível atualizar a conversa.');
    }
  };

  const handleToggleChannelMute = async () => {
    if (!activeChannel) return;
    try {
      await toggleMuteChannel(activeChannel.id, !activeChannel.is_muted);
      toast.success(activeChannel.is_muted ? 'Notificações ativadas.' : 'Conversa silenciada.');
      setChannelMenuOpen(false);
    } catch {
      toast.error('Não foi possível atualizar as notificações.');
    }
  };

  if (channelsError && channels.length === 0 && !isLoading) {
    return (
      <PortalPage variant="full" className="bg-muted px-5 pt-6 overflow-x-hidden">
        <PortalPageHeader title="Mensagens" className="-mx-5 px-5" />
        <PortalError title="Não foi possível carregar as conversas" onRetry={() => void refreshChannels()} />
      </PortalPage>
    );
  }

  const channelList = (
    <aside className="flex h-full w-full flex-col border-r border-border bg-card md:w-[320px] md:flex-none">
      <div className="border-b border-border px-4 pb-4 pt-5 md:pt-4">
        <div className="mb-3 flex items-center justify-between">
          <div>
            <p className="text-micro font-bold uppercase tracking-[0.12em] text-primary-600">Portal do cliente</p>
            <h1 className="text-title-lg font-bold text-foreground">Mensagens</h1>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            onClick={() => void refreshChannels()}
            disabled={isLoadingChannels}
            className="size-9 text-muted-foreground"
            aria-label="Atualizar conversas"
          >
            <RefreshCw className={clsx('size-4', isLoadingChannels && 'animate-spin')} />
          </Button>
        </div>
        <label className="relative block">
          <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <input
            value={channelSearch}
            onChange={(event) => setChannelSearch(event.target.value)}
            placeholder="Buscar conversas"
            className="h-10 w-full rounded-xl border border-border bg-muted pl-9 pr-9 text-sm outline-none transition focus:border-primary-400 focus:ring-2 focus:ring-primary-100"
          />
          {channelSearch && (
            <button
              type="button"
              onClick={() => setChannelSearch('')}
              className="absolute right-2 top-1/2 -translate-y-1/2 rounded-md p-1 text-muted-foreground hover:bg-card"
              aria-label="Limpar busca"
            >
              <X className="size-3.5" />
            </button>
          )}
        </label>
      </div>

      <div className="flex-1 overflow-y-auto">
        <PortalContactChooser onOpened={() => setShowMobileChannels(false)} />
        {isLoadingChannels && channels.length === 0 ? (
          <div className="flex justify-center py-16"><Loader2 className="size-6 animate-spin text-primary-500" /></div>
        ) : filteredChannels.length === 0 ? (
          <PortalEmpty
            icon={MessageSquare}
            title={channelSearch ? 'Nenhuma conversa encontrada' : 'Nenhuma conversa disponível'}
            description={channelSearch ? 'Tente buscar por outro termo.' : 'Use Falar com a imobiliária para iniciar uma conversa.'}
            className="border-0 py-14 shadow-none"
          />
        ) : (
          filteredChannels.map((channel) => {
            const name = getPortalChannelName(channel, myAccountId, brokerInfo?.name);
            const avatar = getPortalChannelAvatar(channel, myAccountId);
            const isActive = activeChannel?.id === channel.id;
            const unreadCount = channel.unread_count || 0;
            return (
              <button
                key={channel.id}
                type="button"
                onClick={() => selectChannel(channel)}
                className={clsx(
                  'flex w-full items-start gap-3 border-b border-border/70 px-4 py-3.5 text-left transition',
                  isActive ? 'bg-primary-50/80' : 'hover:bg-muted/70',
                )}
              >
                <div className="relative mt-0.5 flex size-11 shrink-0 items-center justify-center rounded-full bg-primary-100 font-bold text-primary-700">
                  {avatar ? (
                    <img src={getMediaUrl(avatar)} alt="" className="size-11 rounded-full object-cover" />
                  ) : getPortalInitials(name)}
                  {channel.is_pinned && (
                    <span className="absolute -right-0.5 -top-0.5 flex size-4 items-center justify-center rounded-full bg-status-warning text-white">
                      <Pin className="size-2.5" />
                    </span>
                  )}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-baseline justify-between gap-2">
                    <span className={clsx('truncate text-sm text-foreground', unreadCount ? 'font-bold' : 'font-semibold')}>{name}</span>
                    <span className={clsx('shrink-0 text-micro', unreadCount ? 'font-bold text-primary-600' : 'text-muted-foreground')}>
                      {formatPortalConversationTime(channel.last_message?.sent_at || channel.updated_at)}
                    </span>
                  </div>
                  <div className="mt-0.5 flex items-center gap-2">
                    {channel.is_muted && <BellOff className="size-3 shrink-0 text-muted-foreground" />}
                    <span className={clsx('line-clamp-1 flex-1 text-xs', unreadCount ? 'font-medium text-foreground' : 'text-muted-foreground')}>
                      {channel.last_message?.content || getPortalChannelSubtitle(channel, brokerInfo?.name)}
                    </span>
                    {unreadCount > 0 && (
                      <span className="flex min-w-5 items-center justify-center rounded-full bg-primary px-1.5 py-0.5 text-micro font-bold text-primary-foreground">
                        {unreadCount > 99 ? '99+' : unreadCount}
                      </span>
                    )}
                  </div>
                </div>
              </button>
            );
          })
        )}
      </div>
    </aside>
  );

  return (
    <PortalPage variant="full" width="wide" className="bg-muted md:p-4 overflow-x-hidden">
      <div className="relative flex h-full overflow-hidden bg-card md:rounded-3xl md:border md:border-border md:shadow-sm">
        <div className="hidden md:block">{channelList}</div>

        {showMobileChannels && (
          <div className="absolute inset-0 z-50 md:hidden">
            <button
              type="button"
              className="absolute inset-0 bg-black/30"
              onClick={() => setShowMobileChannels(false)}
              aria-label="Fechar lista de conversas"
            />
            <div className="relative h-full w-[88%] max-w-sm shadow-2xl">{channelList}</div>
          </div>
        )}

        <main className="flex min-w-0 flex-1 flex-col bg-background">
          {activeChannel ? (
            <>
              <header className="relative flex min-h-[68px] items-center gap-2 border-b border-border bg-card px-2 py-2.5 md:gap-3 md:px-5">
                {/* Voltar sai da conversa e volta para onde a pessoa estava
                    (era o que faltava para sair do chat sem passar pela lista).
                    O botão "Ver conversas" continua abrindo a lista de canais. */}
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={goBack}
                  className="size-9 shrink-0 rounded-full text-muted-foreground hover:text-foreground"
                  aria-label="Voltar"
                  title="Voltar"
                >
                  <ArrowLeft className="size-5" aria-hidden="true" />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={() => setShowMobileChannels(true)}
                  className="size-9 shrink-0 text-muted-foreground md:hidden"
                  aria-label="Ver conversas"
                >
                  <Menu className="size-5" />
                </Button>
                <div className="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-full bg-primary-100 font-bold text-primary-700">
                  {getPortalChannelAvatar(activeChannel, myAccountId) ? (
                    <img src={getMediaUrl(getPortalChannelAvatar(activeChannel, myAccountId)!)} alt="" className="size-full object-cover" />
                  ) : getPortalInitials(channelName)}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5">
                    <h2 className="truncate text-sm font-bold text-foreground md:text-base">{channelName}</h2>
                    {activeChannel.is_pinned && <Pin className="size-3.5 shrink-0 text-status-warning" />}
                    {activeChannel.is_muted && <BellOff className="size-3.5 shrink-0 text-muted-foreground" />}
                  </div>
                  <p className="flex items-center gap-1.5 truncate text-caption text-muted-foreground">
                    {isConnected ? <Wifi className="size-3 text-status-ok" /> : <WifiOff className="size-3 text-status-warning" />}
                    {isConnected ? channelSubtitle : 'Reconectando em tempo real…'}
                  </p>
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={() => {
                    setShowMessageSearch((current) => !current);
                    setChannelMenuOpen(false);
                  }}
                  className={clsx('size-9 text-muted-foreground', showMessageSearch && 'bg-primary-50 text-primary-600')}
                  aria-label="Buscar nesta conversa"
                >
                  <Search className="size-4.5" />
                </Button>
                <div className="relative">
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    onClick={() => setChannelMenuOpen((current) => !current)}
                    className="size-9 text-muted-foreground"
                    aria-label="Opções da conversa"
                  >
                    <MoreHorizontal className="size-5" />
                  </Button>
                  {channelMenuOpen && (
                    <div className="absolute right-0 top-11 z-40 w-56 overflow-hidden rounded-2xl border border-border bg-card py-1 shadow-xl">
                      <button type="button" onClick={handleToggleChannelPin} className="flex w-full items-center gap-3 px-4 py-3 text-left text-sm hover:bg-muted">
                        {activeChannel.is_pinned ? <PinOff className="size-4 text-muted-foreground" /> : <Pin className="size-4 text-muted-foreground" />}
                        {activeChannel.is_pinned ? 'Desafixar conversa' : 'Fixar conversa'}
                      </button>
                      <button type="button" onClick={handleToggleChannelMute} className="flex w-full items-center gap-3 px-4 py-3 text-left text-sm hover:bg-muted">
                        {activeChannel.is_muted ? <Bell className="size-4 text-muted-foreground" /> : <BellOff className="size-4 text-muted-foreground" />}
                        {activeChannel.is_muted ? 'Ativar notificações' : 'Silenciar conversa'}
                      </button>
                      <div className="border-t border-border px-4 py-2.5 text-xs text-muted-foreground">
                        {pinnedCount ? `${pinnedCount} mensagem${pinnedCount > 1 ? 's' : ''} fixada${pinnedCount > 1 ? 's' : ''}` : 'Nenhuma mensagem fixada'}
                      </div>
                    </div>
                  )}
                </div>
              </header>

              {showMessageSearch && (
                <div className="border-b border-border bg-card px-3 py-2 md:px-5">
                  <label className="relative block">
                    <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                    <input
                      ref={messageSearchRef}
                      aria-label="Buscar nesta conversa"
                      value={messageSearch}
                      onChange={(event) => setMessageSearch(event.target.value)}
                      placeholder="Buscar nesta conversa"
                      className="h-10 w-full rounded-xl border border-border bg-muted pl-9 pr-10 text-sm outline-none focus:border-primary-400 focus:ring-2 focus:ring-primary-100"
                    />
                    <button
                      type="button"
                      onClick={() => {
                        setMessageSearch('');
                        setShowMessageSearch(false);
                      }}
                      className="absolute right-2 top-1/2 -translate-y-1/2 rounded-md p-1 text-muted-foreground hover:bg-card"
                      aria-label="Fechar busca"
                    >
                      <X className="size-4" />
                    </button>
                  </label>
                  {messageSearch.trim() && (
                    <p className="mt-1.5 px-1 text-caption text-muted-foreground">
                      {visibleMessages.length} resultado{visibleMessages.length === 1 ? '' : 's'}
                    </p>
                  )}
                </div>
              )}

              <div className="flex-1 overflow-y-auto px-3 py-4 md:px-6">
                {isLoadingMessages && messages.length === 0 && (
                  <div className="flex justify-center py-14"><Loader2 className="size-6 animate-spin text-primary-500" /></div>
                )}

                {Boolean(messagesError) && !isLoadingMessages && (
                  <PortalError
                    title="Não foi possível carregar esta conversa"
                    description="Atualize as mensagens para continuar."
                    onRetry={() => void refreshMessages()}
                    className="mx-auto max-w-md py-8"
                  />
                )}

                {!isLoadingMessages && !messagesError && messages.length === 0 && (
                  <PortalEmpty
                    icon={MessageSquare}
                    title="Comece a conversa"
                    description="Envie uma mensagem para falar com a equipe responsável pelo seu atendimento."
                    className="mx-auto mt-8 max-w-md"
                  />
                )}

                {!isLoadingMessages && !messagesError && messages.length > 0 && visibleMessages.length === 0 && (
                  <PortalEmpty
                    icon={Search}
                    title="Nenhuma mensagem encontrada"
                    description="Tente buscar por outro termo ou nome de arquivo."
                    className="mx-auto mt-8 max-w-md"
                  />
                )}

                {!messagesError && timeline.map((item) => {
                  if (item.type === 'date') {
                    return (
                      <div key={item.id} className="my-5 flex items-center gap-3" role="separator">
                        <span className="h-px flex-1 bg-border" />
                        <span className="rounded-full border border-border bg-card px-3 py-1 text-micro font-semibold text-muted-foreground shadow-sm">
                          {formatPortalMessageDate(item.date)}
                        </span>
                        <span className="h-px flex-1 bg-border" />
                      </div>
                    );
                  }

                  const message = item.message;
                  const { isMe, name: senderName, avatar: senderAvatar } = getPortalMessageSender(message, activeChannel, myAccountId);
                  const status = message._status;
                  const isInteractive = status === 'sent' || !status;

                  return (
                    <div key={item.id} className={clsx('group mb-3 flex items-end gap-2', isMe ? 'justify-end' : 'justify-start')}>
                      {!isMe && (
                        <div className="flex size-7 shrink-0 items-center justify-center overflow-hidden rounded-full bg-primary-100 text-micro font-bold text-primary-700">
                          {senderAvatar ? <img src={getMediaUrl(senderAvatar)} alt="" className="size-full object-cover" /> : getPortalInitials(senderName)}
                        </div>
                      )}
                      <div className={clsx('relative min-w-0 max-w-[82%] md:max-w-[70%]', isMe && 'order-first')}>
                        <div className="flex items-start gap-1">
                          <div
                            className={clsx(
                              'min-w-[72px] overflow-hidden rounded-2xl px-3.5 py-2.5 text-[13.5px] leading-relaxed shadow-sm',
                              isMe ? 'rounded-br-sm bg-primary text-primary-foreground' : 'rounded-bl-sm border border-border bg-card text-foreground',
                              status === 'failed' && 'ring-1 ring-status-critical/50',
                            )}
                          >
                            {message.reply_to && (
                              <div
                                className={clsx(
                                  'mb-2 block w-full rounded-lg border-l-2 px-2 py-1.5 text-left text-caption',
                                  isMe ? 'border-l-primary-200 bg-primary-700/50' : 'border-l-primary-500 bg-muted',
                                )}
                              >
                                <strong className="block truncate">{message.reply_to.sender_name || 'Mensagem'}</strong>
                                <span className="block truncate opacity-80">{message.reply_to.content}</span>
                              </div>
                            )}
                            {!isMe && <p className="mb-0.5 text-caption font-bold text-primary-600">{senderName}</p>}
                            {message.content && <p className="whitespace-pre-wrap break-words">{message.content}</p>}
                            {message.attachments?.length ? (
                              <div className="mt-2 space-y-1.5">
                                {message.attachments.map((attachment, index) => {
                                  const url = resolveAttachmentUrl(attachment);
                                  return isImageAttachment(attachment) ? (
                                    <button
                                      key={`${attachment.id || attachment.name}-${index}`}
                                      type="button"
                                      onClick={() => setViewer({ url, title: attachment.name || 'Imagem', contentType: attachment.type })}
                                      className="block overflow-hidden rounded-xl border border-white/20 bg-black/5"
                                    >
                                      <img src={url} alt={attachment.name || 'Imagem anexada'} className="h-40 w-full max-w-[280px] object-cover" />
                                    </button>
                                  ) : (
                                    <button
                                      key={`${attachment.id || attachment.name}-${index}`}
                                      type="button"
                                      onClick={() => setViewer({ url, title: attachment.name || 'Arquivo', contentType: attachment.type })}
                                      className={clsx('flex w-full items-center gap-2 rounded-xl px-2.5 py-2 text-left', isMe ? 'bg-primary-700/50' : 'bg-muted')}
                                    >
                                      <FileText className="size-4 shrink-0" />
                                      <span className="min-w-0 flex-1">
                                        <span className="block truncate text-caption font-semibold">{attachment.name || 'Arquivo'}</span>
                                        {attachment.size ? <span className="block text-caption opacity-70">{formatFileSize(attachment.size)}</span> : null}
                                      </span>
                                    </button>
                                  );
                                })}
                              </div>
                            ) : null}
                            <div className={clsx('mt-1 flex items-center justify-end gap-1 text-caption', isMe ? 'text-primary-100' : 'text-muted-foreground')}>
                              {message.is_pinned && <Pin className="size-3" />}
                              {message.is_edited && <span className="italic">editada</span>}
                              <span>{new Date(message.sent_at || Date.now()).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })}</span>
                              {status === 'sending' && <Loader2 className="size-3 animate-spin" />}
                              {status === 'failed' && <AlertCircle className="size-3 text-status-critical" />}
                              {isMe && status !== 'sending' && status !== 'failed' && (
                                (message.read_by?.length || 0) > 1 ? <CheckCheck className="size-3.5 text-sky-200" /> : <Check className="size-3.5" />
                              )}
                            </div>
                          </div>
                          {isInteractive && (
                            <button
                              type="button"
                              onClick={(event) => { menuOpener.current = event.currentTarget; menuFocusComposer.current = false; setMessageMenu(message); }}
                              aria-haspopup="dialog"
                              className="mt-1 rounded-full p-1 text-muted-foreground opacity-70 transition hover:bg-muted hover:opacity-100 md:opacity-0 md:group-hover:opacity-100 focus-visible:opacity-100"
                              aria-label="Ações da mensagem"
                            >
                              <MoreHorizontal className="size-4" />
                            </button>
                          )}
                        </div>

                        {message.reactions?.length ? (
                          <div className={clsx('mt-1 flex flex-wrap gap-1', isMe && 'justify-end')}>
                            {message.reactions.map((reaction) => {
                              const mine = reaction.user_ids.includes(myAccountId || 'me');
                              return (
                                <button
                                  key={reaction.emoji}
                                  type="button"
                                  onClick={() => void handleReaction(message, reaction.emoji)}
                                  className={clsx(
                                    'rounded-full border px-2 py-0.5 text-caption font-semibold shadow-sm transition',
                                    mine ? 'border-primary-300 bg-primary-50 text-primary-700' : 'border-border bg-card text-foreground hover:border-primary-200',
                                  )}
                                  aria-label={`${mine ? 'Remover' : 'Adicionar'} reação ${reaction.emoji}`}
                                >
                                  {reaction.emoji} {reaction.user_ids.length}
                                </button>
                              );
                            })}
                          </div>
                        ) : null}

                        {isMe && status === 'failed' && (
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            onClick={() => void retryMessage(message).catch(() => toast.error('Falha ao reenviar.'))}
                            className="mt-1 h-auto gap-1 px-2 py-1 text-caption font-bold text-status-critical hover:text-status-critical"
                          >
                            <RefreshCw className="size-3.5" /> Reenviar
                          </Button>
                        )}
                      </div>
                    </div>
                  );
                })}

                {Object.keys(typingUsers).length > 0 && (
                  <div className="mb-2 flex items-center gap-2 pl-9 text-caption text-muted-foreground">
                    <span className="flex gap-0.5">
                      <span className="size-1.5 animate-bounce rounded-full bg-primary-400 [animation-delay:-0.2s]" />
                      <span className="size-1.5 animate-bounce rounded-full bg-primary-400 [animation-delay:-0.1s]" />
                      <span className="size-1.5 animate-bounce rounded-full bg-primary-400" />
                    </span>
                    {String(Object.values(typingUsers)[0] || 'A equipe')} está digitando…
                  </div>
                )}
                <div ref={messagesEndRef} />
              </div>

              <div className="border-t border-border bg-card px-3 py-3 md:px-5">
                {(replyTo || editingMessage) && (
                  <div className="mb-2 flex items-center justify-between gap-2 rounded-xl border border-border bg-muted px-3 py-2 text-caption">
                    <div className="min-w-0 border-l-2 border-primary-500 pl-2">
                      <strong className="block text-primary-700">{editingMessage ? 'Editando mensagem' : `Respondendo a ${replyTo?.sender_name || 'mensagem'}`}</strong>
                      <span className="block truncate text-muted-foreground">{(editingMessage || replyTo)?.content}</span>
                    </div>
                    <button
                      type="button"
                      onClick={() => {
                        setReplyTo(null);
                        setEditingMessage(null);
                        if (editingMessage) setInput('');
                      }}
                      className="shrink-0 rounded-full p-1 text-muted-foreground hover:bg-card"
                      aria-label="Cancelar"
                    >
                      <X className="size-4" />
                    </button>
                  </div>
                )}

                {(pendingAttachments.length > 0 || uploading) && (
                  <div className="mb-2 flex gap-2 overflow-x-auto pb-1">
                    {pendingAttachments.map((attachment, index) => (
                      <div key={`${attachment.name}-${index}`} className="flex max-w-[220px] shrink-0 items-center gap-2 rounded-xl border border-border bg-muted px-3 py-2 text-caption">
                        <Paperclip className="size-3.5 shrink-0 text-primary-600" />
                        <span className="truncate font-medium">{attachment.name}</span>
                        <button type="button" onClick={() => setPendingAttachments((current) => current.filter((_, itemIndex) => itemIndex !== index))} aria-label={`Remover ${attachment.name}`}>
                          <X className="size-3.5 text-muted-foreground" />
                        </button>
                      </div>
                    ))}
                    {uploading && (
                      <div className="flex shrink-0 items-center gap-2 rounded-xl border border-border bg-muted px-3 py-2 text-caption text-muted-foreground">
                        <Loader2 className="size-3.5 animate-spin" /> Enviando arquivo…
                      </div>
                    )}
                  </div>
                )}

                {activeChannel.status === 'closed' ? (
                  <div className="rounded-xl border border-border bg-muted px-4 py-3 text-center text-sm text-muted-foreground">Esta conversa foi encerrada.</div>
                ) : (
                  <form onSubmit={handleSubmit} className="flex items-end gap-1.5 md:gap-2">
                    <input
                      ref={fileRef}
                      type="file"
                      multiple
                      className="hidden"
                      onChange={(event) => void handleFileSelect(event.target.files)}
                    />
                    {!editingMessage && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        onClick={() => fileRef.current?.click()}
                        disabled={uploading}
                        className="size-10 shrink-0 rounded-full text-muted-foreground"
                        aria-label="Anexar arquivos"
                      >
                        <Paperclip className="size-5" />
                      </Button>
                    )}
                    <div className="relative flex min-w-0 flex-1 items-end rounded-2xl border border-border bg-muted focus-within:border-primary-400 focus-within:ring-2 focus-within:ring-primary-100">
                      <textarea
                        ref={textareaRef}
                        aria-label="Mensagem"
                        value={input}
                        rows={1}
                        onChange={(event) => handleInputChange(event.target.value)}
                        onKeyDown={(event) => {
                          if (event.key === 'Enter' && !event.shiftKey) {
                            event.preventDefault();
                            void handleSubmit();
                          }
                        }}
                        placeholder={editingMessage ? 'Edite sua mensagem' : 'Digite uma mensagem'}
                        className="max-h-32 min-h-11 flex-1 resize-none bg-transparent px-3 py-3 text-sm leading-5 outline-none"
                      />
                      <div className="relative mb-0.5 mr-1">
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          onClick={() => setEmojiOpen((current) => !current)}
                          className={clsx('size-9 rounded-full text-muted-foreground', emojiOpen && 'text-primary-600')}
                          aria-label="Adicionar emoji"
                        >
                          <Smile className="size-5" />
                        </Button>
                        {emojiOpen && (
                          <div className="absolute bottom-11 right-0 z-30 flex gap-0.5 rounded-2xl border border-border bg-card p-2 shadow-xl">
                            {QUICK_REACTIONS.map((emoji) => (
                              <button
                                key={emoji}
                                type="button"
                                onClick={() => {
                                  setInput((current) => current + emoji);
                                  setEmojiOpen(false);
                                  textareaRef.current?.focus();
                                }}
                                className="rounded-lg p-1.5 text-lg hover:bg-muted"
                                aria-label={`Emoji ${emoji}`}
                              >
                                {emoji}
                              </button>
                            ))}
                          </div>
                        )}
                      </div>
                    </div>
                    <Button
                      type="submit"
                      variant="success"
                      size="icon"
                      disabled={uploading || (!input.trim() && pendingAttachments.length === 0)}
                      className="size-11 shrink-0 rounded-full"
                      aria-label={editingMessage ? 'Salvar edição' : 'Enviar mensagem'}
                    >
                      {editingMessage ? <Check className="size-5" /> : <Send className="size-5" />}
                    </Button>
                  </form>
                )}
                <p className="mt-1.5 hidden text-center text-caption text-muted-foreground md:block">Enter envia · Shift + Enter quebra a linha</p>
              </div>
            </>
          ) : (
            <div className="flex flex-1 flex-col items-center justify-center px-6 text-center">
              <div className="mb-4 flex size-16 items-center justify-center rounded-2xl bg-primary-50 text-primary-500"><MessageSquare className="size-8" /></div>
              <h2 className="text-lg font-bold text-foreground">Suas conversas</h2>
              <p className="mt-1 max-w-sm text-sm text-muted-foreground">Selecione uma conversa para falar com a imobiliária e acompanhar seu atendimento.</p>
              <Button type="button" variant="outline" onClick={() => {
                setShowMobileChannels(true);
                if (!channels.length) setSearchParams({ contact: '1' }, { replace: true });
              }} className="mt-5 md:hidden">{channels.length ? 'Ver conversas' : 'Falar com a imobiliária'}</Button>
            </div>
          )}
        </main>

        {messageMenu && activeChannel && (
          <Dialog open onOpenChange={open => { if (!open) setMessageMenu(null); }}>
            <DialogContent showCloseButton={false} aria-describedby={undefined} className="sm:max-w-sm gap-0 overflow-hidden p-0" onCloseAutoFocus={event => {
              event.preventDefault();
              const target = menuFocusComposer.current ? textareaRef.current : menuOpener.current;
              if (target?.isConnected) target.focus();
              else textareaRef.current?.focus();
            }}>
              <div className="flex items-center justify-between border-b border-border px-4 py-3">
                <DialogTitle className="text-sm font-bold text-foreground">Ações da mensagem</DialogTitle>
                <button type="button" onClick={() => setMessageMenu(null)} className="rounded-full p-1 text-muted-foreground hover:bg-muted" aria-label="Fechar"><X className="size-4" /></button>
              </div>
              <div className="flex justify-center gap-1 border-b border-border px-3 py-2">
                {QUICK_REACTIONS.map((emoji) => (
                  <button key={emoji} type="button" onClick={() => void handleReaction(messageMenu, emoji)} className="rounded-lg p-2 text-xl hover:bg-muted" aria-label={`Reagir ${emoji}`}>{emoji}</button>
                ))}
              </div>
              <div className="py-1">
                <button type="button" onClick={() => startReply(messageMenu)} className="flex w-full items-center gap-3 px-4 py-3 text-sm hover:bg-muted"><Reply className="size-4 text-muted-foreground" />Responder</button>
                <button type="button" onClick={() => void copyMessage(messageMenu)} className="flex w-full items-center gap-3 px-4 py-3 text-sm hover:bg-muted"><Copy className="size-4 text-muted-foreground" />Copiar</button>
                <button type="button" onClick={() => void handleToggleMessagePin(messageMenu)} className="flex w-full items-center gap-3 px-4 py-3 text-sm hover:bg-muted">
                  {messageMenu.is_pinned ? <PinOff className="size-4 text-muted-foreground" /> : <Pin className="size-4 text-muted-foreground" />}
                  {messageMenu.is_pinned ? 'Desafixar' : 'Fixar'}
                </button>
                {(messageMenu.sender_id === myAccountId || messageMenu.sender_id === 'me') && (
                  <>
                    <button type="button" onClick={() => startEdit(messageMenu)} className="flex w-full items-center gap-3 px-4 py-3 text-sm hover:bg-muted"><Edit3 className="size-4 text-muted-foreground" />Editar</button>
                    <button type="button" onClick={() => void handleDelete(messageMenu)} className="flex w-full items-center gap-3 px-4 py-3 text-sm text-status-critical hover:bg-status-critical-soft"><Trash2 className="size-4" />Apagar</button>
                  </>
                )}
              </div>
            </DialogContent>
          </Dialog>
        )}

        {channelMenuOpen && <button type="button" className="fixed inset-0 z-30" onClick={() => setChannelMenuOpen(false)} aria-label="Fechar opções" />}
      </div>

      {confirmationDialog}
      {viewer && (
        <DocumentViewerDialog
          open={Boolean(viewer)}
          onOpenChange={(open) => !open && setViewer(null)}
          url={viewer.url}
          title={viewer.title}
          contentType={viewer.contentType}
          fetchContent={fetchPortalContent}
        />
      )}
    </PortalPage>
  );
}
