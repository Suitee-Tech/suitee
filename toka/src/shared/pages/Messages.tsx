import { Dialog, DialogContent, DialogTitle } from "../ui/dialog";
import { useConfirmAction } from "../hooks/useConfirmAction";
import { useLongPress } from "../hooks/useLongPress";
import { useState, useEffect, useRef, useCallback, useMemo } from "react";
import { useLocation } from "react-router";
import {
  Search,
  Send,
  Paperclip,
  Smile,
  Phone,
  Video,
  Info,
  Plus,
  UserPlus,
  X,
  Trash2,
  Lock,
  Hash,
  Users,
  Reply,
  Pin,
  PinOff,
  MoreHorizontal,
  Check,
  CheckCheck,
  ChevronDown,
  ChevronLeft,
  ArrowDown,
  File as FileIcon,
  BellOff,
  Bell,
  Archive,
  Edit3,
  Copy,
  MessageSquare,
  Loader2,
  Shield,
  LayoutTemplate,
  ActivitySquare,
  AlertTriangle,
  Clock,
  ExternalLink,
} from "lucide-react";
import { clsx } from "clsx";
import { motion, AnimatePresence } from "motion/react";
import {
  messageService,
  normalizeMessageChannel,
  getOtherChannelParticipant,
  type Channel,
  type Message,
  type ChannelType,
  type Participant,
  type Reaction,
  type Attachment,
  type MessageTemplate,
  type ChannelInfo,
  type ChannelInfoSection,
  type ChannelInfoItem,
} from "../services/message.service";
import { leadService } from "../services/lead.service";
import { brokerService } from "../services/broker.service";
import { settingsService, type CompanyMember } from "../services/settings.service";
import { useAuth } from "../context/AuthContext";
import { useLeadsContext } from "../context/LeadsContext";
import { useAgentsContext } from "../context/AgentsContext";
import { toast } from "sonner";
import { useWsSubscription } from "../ws/useWsSubscription";
import { getAccessToken, toGatewayUrl, withGatewayToken } from "../services/api";
import { getImgproxyUrl } from "../services/imgproxy";
import { DocumentViewerDialog } from "../components/DocumentViewerDialog";

import { getMediaUrl } from "../media";
import { Avatar, AvatarImage, AvatarFallback } from "../ui/avatar";
import { Button } from "../ui/button";
import {
  CHANNEL_TYPE_META,
  EMOJI_CATEGORIES,
  QUICK_REACTIONS,
  avatarColor,
  formatChannelTime,
  formatDateSeparator,
  formatFileSize,
  formatMessageTime,
  getInitials,
  highlightText,
} from "./Messages/presentation";
import { ChannelSidebar } from "./Messages/ChannelSidebar";
import { MetricsPanel, SearchResultsPanel } from "./Messages/StatusPanels";

export function Messages() {
  const { confirmAction, confirmationDialog } = useConfirmAction();
  const { user } = useAuth();
  const { leads } = useLeadsContext();
  const { agents } = useAgentsContext();
  const userId = user?.id || "";

  const [companyMembers, setCompanyMembers] = useState<CompanyMember[]>([]);

  const companyId = user?.current_company_id || user?.companies?.[0]?.id;
  useEffect(() => {
    let active = true;
    setCompanyMembers([]);
    if (companyId) {
      void settingsService.listCompanyMembers(companyId)
        .then(members => { if (active) setCompanyMembers(members || []); })
        .catch(() => {});
    }
    return () => { active = false; };
  }, [companyId]);

  const [channels, setChannels] = useState<Channel[]>([]);
  const [activeChannelId, setActiveChannelId] = useState<string | null>(null);
  const [channelSearch, setChannelSearch] = useState("");
  const [channelFilter, setChannelFilter] = useState<ChannelType | "all" | "archived">("all");

  const [messages, setMessages] = useState<Message[]>([]);
  const [messageText, setMessageText] = useState("");
  const [replyTo, setReplyTo] = useState<Message | null>(null);
  const [editingMessage, setEditingMessage] = useState<Message | null>(null);
  const [messageSearch, setMessageSearch] = useState("");
  const [isSearchingMessages, setIsSearchingMessages] = useState(false);
  const [searchResults, setSearchResults] = useState<Message[]>([]);

  const [fetchedParticipants, setFetchedParticipants] = useState<Record<string, { name: string; avatar?: string }>>({});

  const knownUsersMap = useMemo(() => {
    const map = new Map<string, { name: string; avatar?: string }>();

    if (user) map.set(user.id, { name: user.firstname || "Você", avatar: user.avatar_url });
    map.set("aaa000000000000000000001", { name: "Admin (Suporte)" });
    map.set("system", { name: "Sistema" });

    agents.forEach((a) => map.set(a.id, { name: a.name, avatar: a.avatar }));
    leads.forEach((l) => {
      map.set(l.id, { name: l.name, avatar: (l as any).avatar });
      // O cliente/lead conversando via portal aparece por account_id (a conta
      // do convite/OTP) — resolve pelo account_id para mostrar o NOME em /mensagens.
      const acct = (l as any).account_id as string | undefined;
      if (acct) map.set(acct, { name: l.name, avatar: (l as any).avatar });
    });
    companyMembers.forEach((m) => map.set(m.account_id, { name: m.name, avatar: m.avatar_url }));

    Object.entries(fetchedParticipants).forEach(([id, data]) => {
      map.set(id, data);
    });

    channels.forEach((c) => {
      c.participants?.forEach((p) => {
        const participant = typeof p === "string" ? { id: p, name: p, avatar_url: undefined } : p;
        const known = map.get(participant.id);
        if (participant.name && participant.name !== participant.id) {
          map.set(participant.id, { name: participant.name, avatar: participant.avatar_url || known?.avatar });
        } else if (participant.avatar_url && known) {
          map.set(participant.id, { ...known, avatar: participant.avatar_url });
        }
      });

      if (c.last_message) {
        if (!map.has(c.last_message.sender_id) || map.get(c.last_message.sender_id)?.name === c.last_message.sender_id) {
          map.set(c.last_message.sender_id, { name: c.last_message.sender_name, avatar: c.last_message.sender_avatar_url || c.last_message.sender_avatar });
        }
      }
    });

    messages.forEach((m) => {
      if (!map.has(m.sender_id) || map.get(m.sender_id)?.name === m.sender_id) {
        map.set(m.sender_id, { name: m.sender_name, avatar: m.sender_avatar_url || m.sender_avatar });
      }
    });

    return map;
  }, [user, agents, leads, channels, messages, companyMembers, fetchedParticipants]);

  const resolveParticipantName = useCallback(
    (id: string, name?: string) => {
      const known = knownUsersMap.get(id);
      if (known && known.name && known.name !== id) return known.name;

      if (name && name !== id && !name.includes(id) && !name.startsWith("aaa") && !name.startsWith("bbb") && !name.startsWith("ccc")) {
        return name;
      }

      return id.length > 8 ? id.substring(0, 8) : id;
    },
    [knownUsersMap],
  );

  const resolveParticipantAvatar = useCallback(
    (id: string, fallbackUrl?: string) => {
      const known = knownUsersMap.get(id);
      if (known && known.avatar) return known.avatar;

      return fallbackUrl;
    },
    [knownUsersMap],
  );

  const [showDetails, setShowDetails] = useState(false);
  const emojiPickerRef = useRef<HTMLDivElement>(null);
  const participantSearchRef = useRef<HTMLInputElement>(null);
  const [showEmojiPicker, setShowEmojiPicker] = useState(false);
  const [showTemplates, setShowTemplates] = useState(false);
  const [showMetrics, setShowMetrics] = useState(false);
  const [metrics, setMetrics] = useState<any>(null);
  const [templates, setTemplates] = useState<any[]>([]);
  const [isUploading, setIsUploading] = useState(false);
  const [attachments, setAttachments] = useState<File[]>([]);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [emojiCategory, setEmojiCategory] = useState(0);
  const [isCreating, setIsCreating] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [isSending, setIsSending] = useState(false);
  const [showScrollButton, setShowScrollButton] = useState(false);
  const [contextMenu, setContextMenu] = useState<Message | null>(null);
  const menuOpener = useRef<HTMLElement | null>(null);
  const menuFocusComposer = useRef(false);
  // Long-press no mobile: cada linha grava a própria mensagem antes do toque, e
  // o callback (um só, no topo — hooks não podem ser chamados dentro do .map)
  // abre o mesmo menu de ações do clique-direito/hover do desktop.
  const longPressTarget = useRef<Message | null>(null);
  const longPress = useLongPress(() => {
    if (!longPressTarget.current) return;
    menuOpener.current = null;
    menuFocusComposer.current = false;
    setContextMenu(longPressTarget.current);
  });
  const [viewer, setViewer] = useState<{ url: string; title?: string; contentType?: string | null } | null>(null);

  const [autocomplete, setAutocomplete] = useState<{
    type: "mention" | "hashtag" | "command" | null;
    query: string;
    position: number;
    results: any[];
  }>({ type: null, query: "", position: 0, results: [] });

  const [newChannelName, setNewChannelName] = useState("");
  const [newChannelType, setNewChannelType] = useState<ChannelType>("group");
  const [newChannelDescription, setNewChannelDescription] = useState("");

  const [isAddingParticipant, setIsAddingParticipant] = useState(false);
  useEffect(() => { if (isAddingParticipant) participantSearchRef.current?.focus(); }, [isAddingParticipant]);
  const [participantSearch, setParticipantSearch] = useState("");
  const [participantResults, setParticipantResults] = useState<Array<{ id: string; name: string; type: string; avatar_url?: string }>>([]);
  const [isSearchingParticipants, setIsSearchingParticipants] = useState(false);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const typingTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const activeChannel = channels.find((c) => c.id === activeChannelId);

  useEffect(() => {
    if (!activeChannel?.participants) return;

    activeChannel.participants.forEach(async (p) => {
      const pId = typeof p === "string" ? p : p.id;
      if (knownUsersMap.has(pId)) return;
      if (fetchedParticipants[pId]) return;
      setFetchedParticipants((prev) => ({ ...prev, [pId]: { name: pId } }));

      try {
        const brokerResp = await brokerService.getById(pId);
        const b = brokerResp.data?.data || (brokerResp.data as any) || (brokerResp as any).data;
        if (b && b.name) {
          setFetchedParticipants((prev) => ({ ...prev, [pId]: { name: b.name, avatar: b.avatar } }));
          return;
        }
      } catch {
        try {
          const leadResp = await leadService.getById(pId);
          const l = (leadResp as any).data || leadResp;
          if (l && l.name) {
            setFetchedParticipants((prev) => ({ ...prev, [pId]: { name: l.name, avatar: l.avatar } }));
          }
        } catch {
        }
      }
    });
  }, [activeChannel?.participants, knownUsersMap, fetchedParticipants]);

  const activeChannelParticipant = activeChannel ? getOtherChannelParticipant(activeChannel, userId) : undefined;
  const activeChannelAvatar = activeChannelParticipant
    ? resolveParticipantAvatar(activeChannelParticipant.id, activeChannelParticipant.avatar_url)
    : activeChannel?.avatar_url;

  const [typingUsers, setTypingUsers] = useState<Record<string, { userId: string; senderName: string; timeoutId: NodeJS.Timeout }>>({});

  const activeChannelIdRef = useRef(activeChannelId);
  const resolveParticipantNameRef = useRef(resolveParticipantName);
  const lastMarkReadRef = useRef<{ channelId: string; time: number }>({ channelId: "", time: 0 });

  const throttledMarkRead = useCallback(
    (channelId: string) => {
      const now = Date.now();
      if (lastMarkReadRef.current.channelId === channelId && now - lastMarkReadRef.current.time < 3000) return;
      lastMarkReadRef.current = { channelId, time: now };
      messageService.markRead(channelId).catch(() => { });
    },
    [],
  );

  useEffect(() => {
    activeChannelIdRef.current = activeChannelId;
  }, [activeChannelId]);

  useEffect(() => {
    resolveParticipantNameRef.current = resolveParticipantName;
  }, [resolveParticipantName]);
  // Label amigável de canal direto: resolve o OUTRO participante (o lead/cliente
  // ou o corretor) em vez do nome genérico "Conversa".
  const channelDisplayName = useMemo(() => {
    if (!activeChannel) return "";
    if (activeChannel.type !== "direct") return activeChannel.name || "Conversa";
    const other = getOtherChannelParticipant(activeChannel, userId);
    if (other) return resolveParticipantName(other.id, other.name);
    return activeChannel.name || "Conversa";
  }, [activeChannel, userId, resolveParticipantName]);


  const handleWsEvent = useCallback(
    (event: any) => {
      const currentChannelId = activeChannelIdRef.current;
      const currentResolveName = resolveParticipantNameRef.current;

      if (event.type === "new_message") {
        const msg = event.payload as Message;

        setChannels((prev) => {
          const newChannels = [...prev];
          const chIndex = newChannels.findIndex((c) => c.id === msg.channel_id);
          if (chIndex >= 0) {
            newChannels[chIndex].last_message = msg;
            newChannels[chIndex].updated_at = msg.sent_at;
            if (msg.channel_id !== currentChannelId && msg.sender_id !== userId) {
              newChannels[chIndex].unread_count = (newChannels[chIndex].unread_count || 0) + 1;
            }
          }
          return newChannels;
        });

        if (msg.channel_id === currentChannelId) {
          setMessages((prev) => {
            const filtered = prev.filter((m) => !(m.id.startsWith("temp-") && m.content === msg.content));
            if (!filtered.find((m) => m.id === msg.id)) {
              return [...filtered, msg];
            }
            return filtered;
          });

          if (msg.sender_id !== userId) {
            throttledMarkRead(currentChannelId);
          }

          setTypingUsers((prev) => {
            const newTyping = { ...prev };
            if (newTyping[msg.sender_id]) {
              clearTimeout(newTyping[msg.sender_id].timeoutId);
              delete newTyping[msg.sender_id];
            }
            return newTyping;
          });
        }
      } else if (event.type === "typing") {
        const { channel_id, user_id, sender_name } = event.payload;
        if (channel_id === currentChannelId && user_id !== userId) {
          let newTimeoutId: NodeJS.Timeout;
          setTypingUsers((prev) => {
            if (prev[user_id]) {
              clearTimeout(prev[user_id].timeoutId);
            }
            newTimeoutId = setTimeout(() => {
              setTypingUsers((current) => {
                const updated = { ...current };
                delete updated[user_id];
                return updated;
              });
            }, 3000);

            return {
              ...prev,
              [user_id]: { userId: user_id, senderName: currentResolveName(user_id, sender_name), timeoutId: newTimeoutId },
            };
          });
        }
      } else if (event.type === "reaction_added") {
        const { channel_id, message_id, emoji, user_id } = event.payload;
        if (channel_id === currentChannelId) {
          setMessages((prev) =>
            prev.map((m) => {
              if (m.id === message_id) {
                const reactions = [...(m.reactions || [])];
                const existing = reactions.find((r) => r.emoji === emoji);
                if (existing) {
                  if (!existing.user_ids.includes(user_id)) {
                    existing.user_ids.push(user_id);
                  }
                } else {
                  reactions.push({ emoji, user_ids: [user_id] });
                }
                return { ...m, reactions };
              }
              return m;
            }),
          );
        }
      } else if (event.type === "reaction_removed") {
        const { channel_id, message_id, emoji, user_id } = event.payload;
        if (channel_id === currentChannelId) {
          setMessages((prev) =>
            prev.map((m) => {
              if (m.id === message_id) {
                let reactions = [...(m.reactions || [])];
                const existing = reactions.find((r) => r.emoji === emoji);
                if (existing) {
                  existing.user_ids = existing.user_ids.filter((id) => id !== user_id);
                  if (existing.user_ids.length === 0) {
                    reactions = reactions.filter((r) => r.emoji !== emoji);
                  }
                }
                return { ...m, reactions };
              }
              return m;
            }),
          );
        }
      } else if (event.type === "messages_read") {
        const { channel_id, user_id } = event.payload;
        if (channel_id === currentChannelId && user_id) {
          setMessages((prev) =>
            prev.map((message) => ({
              ...message,
              read_by: (message.read_by || []).includes(user_id) ? message.read_by : [...(message.read_by || []), user_id],
            })),
          );
        }
      } else if (event.type === "message_pinned" || event.type === "message_unpinned") {
        const { channel_id, message_id } = event.payload;
        if (channel_id === currentChannelId) {
          setMessages((prev) =>
            prev.map((message) =>
              message.id === message_id
                ? { ...message, is_pinned: event.type === "message_pinned" }
                : message,
            ),
          );
        }
      } else if (event.type === "channel_created") {
        const ch = normalizeMessageChannel(event.payload as Channel);
        setChannels((prev) => {
          if (!prev.find((c) => c.id === ch.id)) {
            return [ch, ...prev];
          }
          return prev;
        });
        void messageService.getChannel(ch.id).then((channel) => {
          setChannels((current) => current.map((item) => item.id === channel.id ? { ...item, participants: channel.participants, participant_profiles: channel.participant_profiles } : item));
        }).catch(() => {});
      } else if (event.type === "message_edited") {
        const { channel_id, message_id, content } = event.payload;
        if (channel_id === currentChannelId) {
          setMessages((prev) => prev.map((m) => (m.id === message_id ? { ...m, content, is_edited: true } : m)));
        }
      } else if (event.type === "message_deleted") {
        const { channel_id, message_id } = event.payload;
        if (channel_id === currentChannelId) {
          setMessages((prev) => prev.filter((m) => m.id !== message_id));
        }
      } else if (event.type === "channel_removed") {
        const { channel_id } = event.payload;
        setChannels((prev) => prev.filter((c) => c.id !== channel_id));
        if (channel_id === currentChannelId) {
          setActiveChannelId(null);
          setMessages([]);
        }
      } else if (event.type === "channel_updated") {
        const { channel_id, status } = event.payload;
        if (status) {
          setChannels((prev) => prev.map((c) => (c.id === channel_id ? { ...c, status } : c)));
        }
      } else if (event.type === "participant_added") {
        const { channel_id, participant_id } = event.payload;
        setChannels((prev) =>
          prev.map((c) => {
            if (c.id === channel_id && !c.participants.some((p) => (typeof p === "string" ? p : p.id) === participant_id)) {
              return { ...c, participants: [...c.participants, { id: participant_id, name: participant_id, role: "member" as const, joined_at: "" }] };
            }
            return c;
          }),
        );
      } else if (event.type === "participant_removed") {
        const { channel_id, participant_id } = event.payload;
        setChannels((prev) =>
          prev.map((c) => {
            if (c.id === channel_id) {
              return { ...c, participants: c.participants.filter((p) => (typeof p === "string" ? p : p.id) !== participant_id) };
            }
            return c;
          }),
        );
      }
    },
    [userId, throttledMarkRead],
  );

  useWsSubscription("*", handleWsEvent);

  const filteredChannels = useMemo(() => {
    let list = channels;
    if (channelFilter === "archived") {
      list = list.filter((c) => c.status === "archived");
    } else {
      list = list.filter((c) => c.status !== "archived" && c.status !== "closed");
      if (channelFilter !== "all") {
        list = list.filter((c) => c.type === channelFilter);
      }
    }

    if (channelSearch.trim()) {
      const q = channelSearch.toLowerCase();
      list = list.filter((c) => c.name.toLowerCase().includes(q) || c.participants.some((p) => resolveParticipantName(typeof p === "string" ? p : p.id, typeof p === "string" ? p : p.name).toLowerCase().includes(q)) || c.last_message?.content.toLowerCase().includes(q));
    }
    return [...list].sort((a, b) => {
      if (a.is_pinned && !b.is_pinned) return -1;
      if (!a.is_pinned && b.is_pinned) return 1;
      return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime();
    });
  }, [channels, channelFilter, channelSearch, resolveParticipantName]);

  const messagesWithDates = useMemo(() => {
    const result: Array<{ type: "date"; date: string } | { type: "message"; msg: Message }> = [];
    let lastDate = "";
    for (const msg of messages) {
      const date = new Date(msg.sent_at).toDateString();
      if (date !== lastDate) {
        result.push({ type: "date", date: msg.sent_at });
        lastDate = date;
      }
      result.push({ type: "message", msg });
    }
    return result;
  }, [messages]);

  const fetchChannels = useCallback(
    async () => {
      try {
        const data = await messageService.listChannels();
        const mapped = (data || []).map((c) => ({
          ...c,
          is_pinned: c.pinned_by?.includes(userId) || false,
          is_muted: c.muted_by?.includes(userId) || false,
        }));
        setChannels(mapped);
        return mapped;
      } catch {
        return [];
      }
    },
    [userId],
  );

  const fetchMessages = useCallback(async (channelId: string) => {
    try {
      const data = await messageService.getMessages(channelId, 100, 0);
      setMessages(() => data?.messages || []);
    } catch {
    }
  }, []);

  const fetchMetrics = useCallback(async () => {
    try {
      const res = await messageService.getMetrics();
      setMetrics(res);
    } catch {
    }
  }, []);

  useEffect(() => {
    let isMounted = true;
    setIsLoading(true);

    fetchChannels()
      .then((loadedChannels) => {
        if (isMounted) {
          setIsLoading(false);
          if (!activeChannelIdRef.current && loadedChannels.length > 0) {
            const firstActive = loadedChannels.find((c) => c.status !== "archived" && c.status !== "closed");
            if (firstActive) {
              setActiveChannelId(firstActive.id);
            }
          }
        }
        fetchMetrics();
      })
      .catch(() => {
        if (isMounted) setIsLoading(false);
      });

    return () => {
      isMounted = false;
    };
  }, [fetchChannels, fetchMetrics]);

  const location = useLocation();

  useEffect(() => {
    const params = new URLSearchParams(location.search);
    const channelIdFromUrl = params.get("channel");
    if (channelIdFromUrl) {
      setActiveChannelId(channelIdFromUrl);
    }
  }, [location.search]);

  useEffect(() => {
    if (!activeChannelId) return;

    let isMounted = true;

    messageService
      .getMessages(activeChannelId, 100, 0)
      .then((data) => {
        if (isMounted) {
          setMessages(data?.messages || []);
          setTimeout(() => {
            messagesEndRef.current?.scrollIntoView({ behavior: "auto" });
          }, 50);
        }
      })
      .catch(() => { });

    throttledMarkRead(activeChannelId);

    return () => {
      isMounted = false;
    };
  }, [activeChannelId, throttledMarkRead]);

  useEffect(() => {
    if (!messageText) {
      setAutocomplete({ type: null, query: "", position: 0, results: [] });
      return;
    }

    const cursorPosition = textareaRef.current?.selectionStart || messageText.length;
    const textBeforeCursor = messageText.slice(0, cursorPosition);

    const mentionMatch = textBeforeCursor.match(/(?:^|\s)@(\w*)$/);
    const hashtagMatch = textBeforeCursor.match(/(?:^|\s)#(\w*)$/);
    const commandMatch = textBeforeCursor.match(/^(\/\w*)$/);

    let type: "mention" | "hashtag" | "command" | null = null;
    let query = "";

    if (commandMatch) {
      type = "command";
      query = commandMatch[1];
    } else if (mentionMatch) {
      type = "mention";
      query = mentionMatch[1];
    } else if (hashtagMatch) {
      type = "hashtag";
      query = hashtagMatch[1];
    }

    if (type) {
      const fetchAutocomplete = async () => {
        try {
          let results: any[] = [];
          if (type === "command") {
            results = [
              { id: "/encerrar", name: "/encerrar", desc: "Encerra o canal" },
              { id: "/arquivar", name: "/arquivar", desc: "Arquiva o canal" },
              { id: "/ping", name: "/ping", desc: "Testa conexão" },
            ].filter((c) => c.name.startsWith(query));
          } else if (type === "mention") {
            results = [{ id: "1", name: "João Corretor" }, { id: "2", name: "Maria Silva" }, { id: "3", name: "Carlos Admin" }].filter((u) =>
              u.name.toLowerCase().includes(query.toLowerCase()),
            );
          } else if (type === "hashtag") {
            results = [{ id: "1", name: "urgente" }, { id: "2", name: "contrato" }, { id: "3", name: "proposta" }].filter((h) =>
              h.name.toLowerCase().includes(query.toLowerCase()),
            );
          }
          setAutocomplete({ type, query, position: cursorPosition, results });
        } catch {
        }
      };
      fetchAutocomplete();
    } else {
      setAutocomplete({ type: null, query: "", position: 0, results: [] });
    }
  }, [messageText]);

  const handleAutocompleteSelect = (selectedId: string, selectedName: string) => {
    const { type, query } = autocomplete;
    if (!type) return;

    const cursorPosition = textareaRef.current?.selectionStart || messageText.length;
    const textBeforeCursor = messageText.slice(0, cursorPosition);
    const textAfterCursor = messageText.slice(cursorPosition);

    let replacement = "";
    let matchLength = 0;

    if (type === "command") {
      replacement = selectedName + " ";
      matchLength = query.length;
    } else if (type === "mention") {
      replacement = `@${selectedName.replace(/\s+/g, "_")} `;
      matchLength = query.length + 1;
    } else if (type === "hashtag") {
      replacement = `#${selectedName} `;
      matchLength = query.length + 1;
    }

    const newTextBeforeCursor = textBeforeCursor.slice(0, -matchLength) + replacement;
    const newText = newTextBeforeCursor + textAfterCursor;

    setMessageText(newText);
    setAutocomplete({ type: null, query: "", position: 0, results: [] });

    setTimeout(() => {
      if (textareaRef.current) {
        textareaRef.current.focus();
        const newCursorPos = newTextBeforeCursor.length;
        textareaRef.current.setSelectionRange(newCursorPos, newCursorPos);
      }
    }, 0);
  };

  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = "auto";
      textareaRef.current.style.height = Math.min(textareaRef.current.scrollHeight, 160) + "px";
    }
  }, [messageText]);

  useEffect(() => {
    if (!showScrollButton) {
      messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
    }
  }, [messages, showScrollButton]);

  useEffect(() => {
    const handler = (event: MouseEvent) => {
      if (!emojiPickerRef.current?.contains(event.target as Node)) setShowEmojiPicker(false);
    };
    document.addEventListener("click", handler);
    return () => document.removeEventListener("click", handler);
  }, []);

  useEffect(() => {
    if (!participantSearch.trim()) {
      setParticipantResults([]);
      return;
    }
    const timer = setTimeout(async () => {
      setIsSearchingParticipants(true);
      try {
        const results: Array<{ id: string; name: string; type: string; avatar_url?: string }> = [];
        try {
          const leadsData = await leadService.getAll({ search: participantSearch, limit: 5 });
          if (leadsData?.data) {
            results.push(
              ...leadsData.data.map((l) => ({
                id: l.id,
                name: l.name,
                type: l.is_client ? "Cliente" : "Lead",
                avatar_url: l.avatar,
              })),
            );
          }
        } catch {
        }
        try {
          const brokersResponse = await brokerService.getAll();
          const brokers = brokersResponse.data?.data || [];
          results.push(
            ...brokers
              .filter((b) => b.name.toLowerCase().includes(participantSearch.toLowerCase()))
              .slice(0, 5)
              .map((b) => ({ id: b.id, name: b.name, type: "Corretor", avatar_url: b.avatar })),
          );
        } catch {
        }
        setParticipantResults(results);
      } finally {
        setIsSearchingParticipants(false);
      }
    }, 400);
    return () => clearTimeout(timer);
  }, [participantSearch]);

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
    setShowScrollButton(false);
  };

  const handleScroll = () => {
    const container = messagesContainerRef.current;
    if (!container) return;
    const isNearBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 150;
    setShowScrollButton(!isNearBottom);
  };

  useEffect(() => {
    messageService.listTemplates().then(setTemplates).catch(() => setTemplates([]));
  }, []);

  const handleTemplateSelect = (template: any) => {
    setMessageText(template.content);
    setShowTemplates(false);
    textareaRef.current?.focus();
  };

  const getAttachmentUrl = (url: string) => toGatewayUrl(url);
  const resolveAttachmentUrl = useCallback(
    (att: Attachment) => {
      let rawUrl = att.url || "";
      if (activeChannelId && /toca-message-attachments/i.test(rawUrl)) {
        const clean = rawUrl.split("#")[0]?.split("?")[0] || rawUrl;
        const filename = clean.split("/").pop();
        if (filename) {
          rawUrl = `/api/v1/channels/${activeChannelId}/messages/attachments/${encodeURIComponent(filename)}/download`;
        }
      }
      return withGatewayToken(getAttachmentUrl(rawUrl), getAccessToken());
    },
    [activeChannelId],
  );

  const openAttachment = useCallback(
    (att: Attachment) => {
      setViewer({ url: resolveAttachmentUrl(att), title: att.name, contentType: att.type || null });
    },
    [resolveAttachmentUrl],
  );

  const handleSendMessage = async () => {
    const text = messageText.trim();
    if (!text && attachments.length === 0) return;
    if (!activeChannelId) return;

    setIsSending(true);
    const tempText = messageText;
    const currentAttachments = [...attachments];

    setMessageText("");
    setAttachments([]);

    const optimistic: Message = {
      id: `temp-${Date.now()}`,
      channel_id: activeChannelId,
      tenant_id: "",
      sender_id: userId,
      sender_name: user?.firstname || user?.email?.split("@")[0] || "Você",
      content: text || "Enviando arquivo...",
      content_type: currentAttachments.length > 0 ? "file" : "text",
      reply_to: replyTo ? { id: replyTo.id, sender_name: replyTo.sender_name, content: replyTo.content } : undefined,
      sent_at: new Date().toISOString(),
      read_by: [userId],
    };
    setMessages((prev) => [...prev, optimistic]);
    setReplyTo(null);
    setEditingMessage(null);
    scrollToBottom();

    try {
      let finalAttachments: Attachment[] = [];
      if (currentAttachments.length > 0) {
        setIsUploading(true);
        const uploadPromises = currentAttachments.map((file) => messageService.uploadAttachment(activeChannelId, file));
        finalAttachments = await Promise.all(uploadPromises);
        setIsUploading(false);
      }

      const sentMsg = await messageService.sendMessage(activeChannelId, {
        content: text || (finalAttachments.length > 0 ? "Arquivo enviado" : ""),
        reply_to_id: replyTo?.id,
        content_type: finalAttachments.length > 0 ? "file" : "text",
        attachments: finalAttachments.length > 0 ? finalAttachments : undefined,
      });
      setMessages((prev) => prev.map((m) => (m.id === optimistic.id ? sentMsg : m)));

      setChannels((prev) => {
        const newChannels = [...prev];
        const chIndex = newChannels.findIndex((c) => c.id === activeChannelId);
        if (chIndex >= 0) {
          newChannels[chIndex].last_message = sentMsg;
          newChannels[chIndex].updated_at = sentMsg.sent_at;
        }
        return newChannels;
      });
    } catch {
      toast.error("Erro ao enviar mensagem");
      setMessageText(tempText);
      setAttachments(currentAttachments);
      setIsUploading(false);
      setMessages((prev) => prev.filter((m) => m.id !== optimistic.id));
    } finally {
      setIsSending(false);
    }
  };

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files.length > 0) {
      setAttachments((prev) => [...prev, ...Array.from(e.target.files!)]);
    }
    if (fileInputRef.current) fileInputRef.current.value = "";
  };

  const removeAttachment = (index: number) => {
    setAttachments((prev) => prev.filter((_, i) => i !== index));
  };

  const handleEditMessage = async () => {
    if (!editingMessage || !messageText.trim() || !activeChannelId) return;
    try {
      await messageService.editMessage(activeChannelId, editingMessage.id, messageText.trim());
      setEditingMessage(null);
      setMessageText("");
      fetchMessages(activeChannelId);
      toast.success("Mensagem editada");
    } catch {
      toast.error("Erro ao editar mensagem");
    }
  };

  const handleDeleteMessage = (msg: Message) => {
    if (!activeChannelId) return;
    const channelId = activeChannelId;
    confirmAction("Apagar esta mensagem? Esta ação não pode ser desfeita.", async () => {
      try {
        await messageService.deleteMessage(channelId, msg.id);
        setContextMenu(null);
        setMessages(prev => prev.filter(message => message.id !== msg.id));
        toast.success("Mensagem apagada.");
      } catch (error) {
        toast.error("Não foi possível apagar a mensagem.");
        throw error;
      }
    }, { title: "Apagar mensagem", confirmLabel: "Apagar", variant: "danger" });
  };

  const handleTogglePin = async (msg: Message) => {
    if (!activeChannelId) return;
    try {
      if (msg.is_pinned) {
        await messageService.togglePinMessage(activeChannelId, msg.id, false);
      } else {
        await messageService.togglePinMessage(activeChannelId, msg.id, true);
      }
      fetchMessages(activeChannelId);
    } catch {
      toast.error("Erro ao fixar mensagem");
    }
  };

  const handleReaction = async (msg: Message, emoji: string) => {
    if (!activeChannelId) return;
    const existing = msg.reactions?.find((r) => r.emoji === emoji);
    try {
      if (existing?.user_ids.includes(userId)) {
        await messageService.removeReaction(activeChannelId, msg.id, emoji);
      } else {
        await messageService.addReaction(activeChannelId, msg.id, emoji);
      }
      fetchMessages(activeChannelId);
    } catch {
    }
  };

  const handleFileUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    handleFileSelect(e);
  };

  const handleCreateChannel = async () => {
    if (!newChannelName.trim()) return;
    setIsLoading(true);
    try {
      const newChannel = await messageService.createChannel({
        name: newChannelName.trim(),
        type: newChannelType,
        description: newChannelDescription.trim() || undefined,
      });
      setNewChannelName("");
      setNewChannelDescription("");
      setIsCreating(false);

      setChannels((prev) => {
        if (!prev.find((c) => c.id === newChannel.id)) {
          return [newChannel, ...prev];
        }
        return prev;
      });

      setActiveChannelId(newChannel.id);
      setMessages([]);
      toast.success("Canal criado!");
    } catch {
      toast.error("Erro ao criar canal");
    } finally {
      setIsLoading(false);
    }
  };

  const handleAddParticipant = async (participantId: string) => {
    if (!activeChannelId) return;
    try {
      await messageService.addParticipant(activeChannelId, { participant_id: participantId });
      setParticipantSearch("");
      setParticipantResults([]);
      setIsAddingParticipant(false);
      await fetchChannels();
      toast.success("Participante adicionado!");
    } catch {
      toast.error("Erro ao adicionar participante");
    }
  };

  const handleRemoveParticipant = async (participantId: string) => {
    if (!activeChannelId) return;
    try {
      await messageService.removeParticipant(activeChannelId, participantId);
      await fetchChannels();
      toast.success("Participante removido");
    } catch {
      toast.error("Erro ao remover participante");
    }
  };

  const handleTogglePinChannel = async () => {
    if (!activeChannel) return;
    try {
      await messageService.togglePinChannel(activeChannel.id, !activeChannel.is_pinned);
      await fetchChannels();
    } catch {
    }
  };

  const handleToggleMuteChannel = async () => {
    if (!activeChannel) return;
    try {
      await messageService.toggleMuteChannel(activeChannel.id, !activeChannel.is_muted);
      await fetchChannels();
    } catch {
    }
  };

  const handleArchiveChannel = async () => {
    if (!activeChannel) return;
    try {
      await messageService.updateChannelStatus(activeChannel.id, "archived");
      setShowDetails(false);
      await fetchChannels();
      toast.success("Canal arquivado");
    } catch {
      toast.error("Erro ao arquivar canal");
    }
  };

  const handleCloseChannel = async () => {
    if (!activeChannel) return;
    try {
      await messageService.updateChannelStatus(activeChannel.id, "closed");
      setShowDetails(false);
      await fetchChannels();
      toast.success("Canal encerrado");
    } catch {
      toast.error("Erro ao encerrar canal");
    }
  };

  const [hasSearched, setHasSearched] = useState(false);

  const handleSearchMessages = useCallback(async (query: string) => {
    if (!query.trim()) {
      setSearchResults([]);
      setHasSearched(false);
      return;
    }
    setIsSearchingMessages(true);
    setHasSearched(true);
    try {
      const results = await messageService.searchMessages(query);
      setSearchResults(results);
    } catch {
      setSearchResults([]);
    } finally {
      setIsSearchingMessages(false);
    }
  }, []);

  const handleTyping = () => {
    if (!activeChannelId) return;
    if (typingTimeoutRef.current) clearTimeout(typingTimeoutRef.current);
    messageService.sendTyping(activeChannelId);
    typingTimeoutRef.current = setTimeout(() => {
      typingTimeoutRef.current = null;
    }, 3000);
  };

  const startReply = (msg: Message) => {
    menuFocusComposer.current = true;
    setReplyTo(msg);
    setEditingMessage(null);
    setContextMenu(null);
    textareaRef.current?.focus();
  };

  const startEdit = (msg: Message) => {
    menuFocusComposer.current = true;
    setEditingMessage(msg);
    setMessageText(msg.content);
    setReplyTo(null);
    setContextMenu(null);
    textareaRef.current?.focus();
  };

  const copyMessage = async (msg: Message) => {
    try {
      await navigator.clipboard.writeText(msg.content);
      toast.success("Mensagem copiada.");
      setContextMenu(null);
    } catch { toast.error("Não foi possível copiar a mensagem."); }
  };

  const handleSelectChannel = (id: string) => {
    if (id === activeChannelId) return;

    setActiveChannelId(id);
    setMessages([]);
    setReplyTo(null);
    setEditingMessage(null);
    setMessageText("");
    setShowScrollButton(false);
    setShowMetrics(false);
  };

  const channelTypeCounts = useMemo(() => {
    const counts: Record<string, number> = { all: channels.filter((c) => c.status !== "archived" && c.status !== "closed").length };
    let archivedCount = 0;
    for (const c of channels) {
      if (c.status === "archived") {
        archivedCount++;
      } else if (c.status !== "closed") {
        counts[c.type] = (counts[c.type] || 0) + 1;
      }
    }
    counts["archived"] = archivedCount;
    return counts;
  }, [channels]);

  const totalUnread = useMemo(() => channels.reduce((sum, c) => sum + (c.unread_count || 0), 0), [channels]);

  return (
    <div className="flex h-full bg-card overflow-hidden relative">
      <ChannelSidebar
        activeChannelId={activeChannelId}
        showMetrics={showMetrics}
        setShowMetrics={setShowMetrics}
        isCreating={isCreating}
        setIsCreating={setIsCreating}
        totalUnread={totalUnread}
        channelSearch={channelSearch}
        setChannelSearch={setChannelSearch}
        messageSearch={messageSearch}
        setMessageSearch={setMessageSearch}
        handleSearchMessages={handleSearchMessages}
        isSearchingMessages={isSearchingMessages}
        searchResults={searchResults}
        setSearchResults={setSearchResults}
        hasSearched={hasSearched}
        setHasSearched={setHasSearched}
        channelTypeCounts={channelTypeCounts}
        channelFilter={channelFilter}
        setChannelFilter={setChannelFilter}
        newChannelName={newChannelName}
        setNewChannelName={setNewChannelName}
        newChannelDescription={newChannelDescription}
        setNewChannelDescription={setNewChannelDescription}
        newChannelType={newChannelType}
        setNewChannelType={setNewChannelType}
        handleCreateChannel={handleCreateChannel}
        isLoading={isLoading}
        channels={channels}
        filteredChannels={filteredChannels}
        handleSelectChannel={handleSelectChannel}
        resolveParticipantAvatar={resolveParticipantAvatar}
        resolveParticipantName={resolveParticipantName}
        userId={userId}
      />
      {showMetrics ? (
        <MetricsPanel metrics={metrics} onClose={() => setShowMetrics(false)} />
      ) : hasSearched && messageSearch.trim() ? (
        <SearchResultsPanel
          messageSearch={messageSearch}
          setMessageSearch={setMessageSearch}
          searchResults={searchResults}
          setSearchResults={setSearchResults}
          setHasSearched={setHasSearched}
          isSearchingMessages={isSearchingMessages}
          handleSelectChannel={handleSelectChannel}
          resolveParticipantName={resolveParticipantName}
        />
      ) : activeChannel ? (
        <div className="flex-1 flex flex-col min-w-0 bg-muted h-full relative">
          <div className="flex-none h-[60px] sm:h-[68px] flex items-center justify-between px-2 sm:px-3 md:px-5 border-b border-border bg-card z-10">
            <div className="flex items-center gap-2 md:gap-3 min-w-0">
              <Button variant="ghost" size="icon" onClick={() => setActiveChannelId(null)} className="md:hidden -ml-1 size-9 shrink-0 text-muted-foreground">
                <ChevronLeft className="w-6 h-6" />
              </Button>
              {activeChannel.type === "direct" ? (
                <Avatar className="w-10 h-10 flex-shrink-0">
                  {activeChannelAvatar && (
                    <AvatarImage src={getMediaUrl(activeChannelAvatar!)} alt="Avatar" />
                  )}
                  <AvatarFallback className={clsx("text-white font-bold text-sm", avatarColor(activeChannel.id))}>
                    {getInitials(channelDisplayName)}
                  </AvatarFallback>
                </Avatar>
              ) : (
                <div className={clsx("w-10 h-10 rounded-full flex items-center justify-center text-white font-bold text-sm flex-shrink-0", avatarColor(activeChannel.id))}>
                  {(() => {
                    const Icon = CHANNEL_TYPE_META[activeChannel.type]?.icon || Hash;
                    return <Icon className="w-5 h-5" />;
                  })()}
                </div>
              )}
              <div className="min-w-0">
                <h2 className="text-sm font-bold text-foreground dark:text-white truncate">{channelDisplayName}</h2>
                <p className="text-xs text-muted-foreground flex items-center gap-1.5 truncate">
                  <span
                    className={clsx(
                      "px-1.5 py-0.5 rounded text-micro font-semibold flex-shrink-0",
                      CHANNEL_TYPE_META[activeChannel.type]?.color || "text-muted-foreground bg-muted",
                    )}
                  >
                    {CHANNEL_TYPE_META[activeChannel.type]?.label || activeChannel.type}
                  </span>
                  <span className="text-muted-foreground/60 flex-shrink-0">|</span>
                  <Users className="w-3 h-3 flex-shrink-0" />
                  <span className="truncate">
                    <span className="hidden sm:inline">{activeChannel.participants?.length || 1} participante(s)</span>
                    <span className="sm:hidden">{activeChannel.participants?.length || 1}</span>
                  </span>
                </p>
              </div>
            </div>

            <div className="flex items-center gap-1">
              {isAddingParticipant ? (
                <div className="flex items-center gap-2 mr-1">
                  <div className="relative">
                    <input
                      type="text"
                      placeholder="Buscar por nome..."
                      value={participantSearch}
                      onChange={(e) => setParticipantSearch(e.target.value)}
                      className="px-3 py-1.5 text-sm border border-border rounded-lg bg-card text-foreground focus:ring-2 focus:ring-primary-500 outline-none w-52"
                      ref={participantSearchRef}
                      aria-label="Buscar participante por nome"
                    />
                    {participantSearch.trim() && (
                      <div className="absolute top-full right-0 mt-1 w-64 bg-card rounded-lg shadow-xl border border-border z-50 overflow-hidden">
                        {isSearchingParticipants ? (
                          <div className="p-3 text-center text-sm text-muted-foreground">
                            <Loader2 className="w-4 h-4 animate-spin mx-auto" />
                          </div>
                        ) : participantResults.length > 0 ? (
                          <ul className="max-h-60 overflow-y-auto">
                            {participantResults.map((r) => (
                              <li key={r.id}>
                                <button onClick={() => handleAddParticipant(r.id)} className="w-full text-left px-4 py-2.5 hover:bg-muted flex items-center gap-3 transition-colors">
                                  <Avatar className="w-8 h-8">
                                    {resolveParticipantAvatar(r.id, r.avatar_url) && <AvatarImage src={getMediaUrl(resolveParticipantAvatar(r.id, r.avatar_url)!)} alt="Avatar" />}
                                    <AvatarFallback className={clsx("text-white text-xs font-bold", avatarColor(r.id))}>{getInitials(r.name)}</AvatarFallback>
                                  </Avatar>
                                  <div>
                                    <div className="text-sm font-medium text-foreground dark:text-white">{r.name}</div>
                                    <div className="text-caption text-muted-foreground">{r.type}</div>
                                  </div>
                                </button>
                              </li>
                            ))}
                          </ul>
                        ) : (
                          <div className="p-3 text-center text-sm text-muted-foreground">Nenhum resultado</div>
                        )}
                      </div>
                    )}
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => {
                      setIsAddingParticipant(false);
                      setParticipantSearch("");
                      setParticipantResults([]);
                    }}
                    className="size-8 text-muted-foreground"
                  >
                    <X className="w-4 h-4" />
                  </Button>
                </div>
              ) : (
                <Button variant="ghost" size="icon" onClick={() => setIsAddingParticipant(true)} className="size-9 text-muted-foreground hover:text-primary hover:bg-primary/10" title="Adicionar Participante">
                  <UserPlus className="w-[18px] h-[18px]" />
                </Button>
              )}
              <Button variant="ghost" size="icon" className="hidden size-9 text-muted-foreground hover:text-primary hover:bg-primary/10 sm:flex" title="Chamada de voz">
                <Phone className="w-[18px] h-[18px]" />
              </Button>
              <Button variant="ghost" size="icon" className="hidden size-9 text-muted-foreground hover:text-primary hover:bg-primary/10 sm:flex" title="Chamada de vídeo">
                <Video className="w-[18px] h-[18px]" />
              </Button>
              <div className="w-px h-5 bg-muted mx-0.5" />
              <Button
                variant="ghost"
                size="icon"
                onClick={() => setShowDetails(!showDetails)}
                className={clsx("size-9", showDetails ? "bg-primary/10 text-primary" : "text-muted-foreground")}
                title="Detalhes do Canal"
              >
                <Info className="w-[18px] h-[18px]" />
              </Button>
            </div>
          </div>

          <div
            ref={messagesContainerRef}
            onScroll={handleScroll}
            className="flex-1 overflow-y-auto overflow-x-hidden px-2 sm:px-4 md:px-6 py-4 space-y-1.5 md:space-y-2"
            style={{ backgroundImage: "radial-gradient(circle at 1px 1px, rgb(226 232 240 / 0.3) 1px, transparent 0)", backgroundSize: "24px 24px" }}
          >
            {messages.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-full text-center">
                <div className="w-16 h-16 rounded-2xl bg-primary-50 dark:bg-primary-900/30 flex items-center justify-center mb-4">
                  <MessageSquare className="w-8 h-8 text-primary-400" />
                </div>
                <h3 className="text-sm font-bold text-foreground mb-1">Nenhuma mensagem ainda</h3>
                <p className="text-xs text-muted-foreground max-w-[220px]">Envie a primeira mensagem para iniciar a conversa neste canal</p>
              </div>
            ) : (
              messagesWithDates.map((item, idx) => {
                if (item.type === "date") {
                  return (
                    <div key={`date-${idx}`} className="flex items-center justify-center py-3">
                      <span className="px-3 py-1 bg-card text-caption font-semibold text-muted-foreground rounded-full shadow-sm border border-border ">{formatDateSeparator(item.date)}</span>
                    </div>
                  );
                }

                const msg = item.msg;
                const isMe = msg.sender_id === userId;
                const isSystem = msg.content_type === "system";
                const isPinned = msg.is_pinned;

                if (isSystem) {
                  return (
                    <div key={msg.id} className="flex justify-center py-2">
                      <span className="px-3 py-1.5 bg-muted text-caption text-muted-foreground rounded-full italic">{msg.content}</span>
                    </div>
                  );
                }

                return (
                  <div
                    key={msg.id}
                    className={clsx(
                      "flex flex-col group mb-1 select-none [-webkit-touch-callout:none]",
                      isMe ? "items-end" : "items-start",
                    )}
                    onTouchStart={(event) => {
                      longPressTarget.current = msg;
                      longPress.onTouchStart(event);
                    }}
                    onTouchMove={longPress.onTouchMove}
                    onTouchEnd={longPress.onTouchEnd}
                    onTouchCancel={longPress.onTouchCancel}
                    onClickCapture={longPress.onClickCapture}
                    onContextMenu={(e) => {
                      e.preventDefault();
                      menuOpener.current = document.activeElement as HTMLElement | null;
                      menuFocusComposer.current = false;
                      setContextMenu(msg);
                    }}
                  >
                    {isPinned && (
                      <div className={clsx("flex items-center gap-1 text-micro text-status-warning font-medium mb-0.5 px-1", isMe ? "flex-row-reverse" : "")}>
                        <Pin className="w-3 h-3" /> Fixada
                      </div>
                    )}

                    <div className={clsx("flex items-end gap-1.5 sm:gap-2 max-w-[85%] sm:max-w-[75%] lg:max-w-[65%]", isMe ? "flex-row-reverse" : "")}>
                      {!isMe && (
                        <Avatar className="w-6 h-6 sm:w-8 sm:h-8 flex-shrink-0 mb-1" title={resolveParticipantName(msg.sender_id, msg.sender_name)}>
                          {resolveParticipantAvatar(msg.sender_id, msg.sender_avatar_url || msg.sender_avatar) && (
                            <AvatarImage src={getMediaUrl(resolveParticipantAvatar(msg.sender_id, msg.sender_avatar_url || msg.sender_avatar)!)} alt="Avatar" />
                          )}
                          <AvatarFallback className={clsx("text-white text-caption font-bold", avatarColor(msg.sender_id))}>{getInitials(resolveParticipantName(msg.sender_id, msg.sender_name))}</AvatarFallback>
                        </Avatar>
                      )}

                      <div className="relative">
                        {msg.reply_to && (
                          <div
                            className={clsx(
                              "mx-1 mb-[-4px] pt-2 pb-3 px-3 rounded-t-xl text-caption border-l-2",
                              isMe ? "bg-primary-700/50 text-primary-200 border-l-primary-300" : "bg-muted text-muted-foreground border-l-primary-400",
                            )}
                          >
                            <span className="font-semibold">{msg.reply_to.sender_name}</span>
                            <p className="truncate max-w-[250px]">{msg.reply_to.content}</p>
                          </div>
                        )}

                        <div
                          className={clsx(
                            "px-3 sm:px-3.5 py-1.5 sm:py-2 rounded-2xl text-[13.5px] sm:text-sm leading-relaxed shadow-sm relative flow-root min-w-[60px]",
                            isMe ? "bg-primary text-primary-foreground rounded-br-sm sm:rounded-br-md" : "bg-card border border-border text-foreground rounded-bl-sm sm:rounded-bl-md",
                          )}
                        >
                          {!isMe && (
                            <div className="text-caption font-bold text-primary-600 dark:text-primary-400 mb-0.5 capitalize">{resolveParticipantName(msg.sender_id, msg.sender_name)}</div>
                          )}

                          {msg.attachments && msg.attachments.length > 0 && (
                            <div className="mb-2">
                              {msg.attachments.map((att, i) =>
                                att.type?.startsWith("image/") ? (
                                  <button
                                    key={i}
                                    type="button"
                                    onClick={(e) => {
                                      e.stopPropagation();
                                      openAttachment(att);
                                    }}
                                    className="block rounded-lg overflow-hidden"
                                  >
                                    <img
                                      src={getImgproxyUrl(resolveAttachmentUrl(att), { width: 280, height: 200 })}
                                      alt={att.name}
                                      className="rounded-lg max-w-[280px] max-h-[200px] object-cover"
                                      loading="lazy"
                                      decoding="async"
                                    />
                                  </button>
                                ) : (
                                  <button
                                    key={i}
                                    type="button"
                                    onClick={(e) => {
                                      e.stopPropagation();
                                      openAttachment(att);
                                    }}
                                    className={clsx("flex items-center gap-2 px-3 py-2 rounded-lg text-xs", isMe ? "bg-primary-700/40" : "bg-muted ")}
                                  >
                                    <FileIcon className="w-4 h-4 flex-shrink-0" />
                                    <div className="min-w-0">
                                      <div className="font-medium truncate">{att.name}</div>
                                      <div className="text-micro opacity-70">{formatFileSize(att.size)}</div>
                                    </div>
                                  </button>
                                ),
                              )}
                            </div>
                          )}

                          <span className="whitespace-pre-wrap break-words">{msg.content}</span>

                          <span
                            className={clsx(
                              "inline-flex items-center gap-1 ml-2 text-micro float-right mt-1.5 relative top-1",
                              isMe ? "text-primary-200" : "text-muted-foreground",
                            )}
                          >
                            {msg.is_edited && <span className="italic">editada</span>}
                            {formatMessageTime(msg.sent_at)}
                            {isMe && (msg.read_by.length > 1 ? <CheckCheck className="w-3.5 h-3.5 text-sky-300" /> : <Check className="w-3.5 h-3.5" />)}
                          </span>
                        </div>

                        {msg.reactions && msg.reactions.length > 0 && (
                          <div className={clsx("flex flex-wrap gap-1 mt-1", isMe ? "justify-end" : "justify-start")}>
                            {msg.reactions.map((r) => (
                              <button
                                key={r.emoji}
                                onClick={() => handleReaction(msg, r.emoji)}
                                className={clsx(
                                  "inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-caption border transition-colors",
                                  r.user_ids.includes(userId)
                                    ? "bg-primary-50 dark:bg-primary-900/30 border-primary-200 dark:border-primary-700 text-primary-600 dark:text-primary-400"
                                    : "bg-card border-border text-muted-foreground hover:bg-muted",
                                )}
                              >
                                {r.emoji} <span className="font-medium">{r.user_ids.length}</span>
                              </button>
                            ))}
                          </div>
                        )}

                        <div
                          className={clsx(
                            "absolute top-0 opacity-0 group-hover:opacity-100 focus-within:opacity-100 transition-opacity flex items-center bg-card rounded-lg shadow-md border border-border overflow-hidden",
                            isMe ? "left-0 -translate-x-full ml-[-8px]" : "right-0 translate-x-full mr-[-8px]",
                          )}
                        >
                          {QUICK_REACTIONS.slice(0, 3).map((emoji) => (
                            <button key={emoji} onClick={(e) => { e.stopPropagation(); handleReaction(msg, emoji); }} className="p-1.5 hover:bg-muted text-sm transition-colors">
                              {emoji}
                            </button>
                          ))}
                          <button onClick={(e) => { e.stopPropagation(); startReply(msg); }} className="p-1.5 hover:bg-muted text-muted-foreground transition-colors" title="Responder">
                            <Reply className="w-3.5 h-3.5" />
                          </button>
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              menuOpener.current = e.currentTarget;
                              menuFocusComposer.current = false;
                              setContextMenu(msg);
                            }}
                            aria-label="Ações da mensagem"
                            aria-haspopup="dialog"
                            className="p-1.5 hover:bg-muted text-muted-foreground transition-colors"
                          >
                            <MoreHorizontal className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                );
              })
            )}
            <div ref={messagesEndRef} />

            {Object.keys(typingUsers).length > 0 && (
              <div className="flex items-center gap-2 px-3 sm:px-4 py-1">
                <div className="flex gap-1 items-center bg-card rounded-full px-3 py-1.5 shadow-sm border border-border ">
                  <div className="w-1.5 h-1.5 bg-muted-foreground/40 rounded-full animate-bounce" style={{ animationDelay: "0ms" }} />
                  <div className="w-1.5 h-1.5 bg-muted-foreground/40 rounded-full animate-bounce" style={{ animationDelay: "150ms" }} />
                  <div className="w-1.5 h-1.5 bg-muted-foreground/40 rounded-full animate-bounce" style={{ animationDelay: "300ms" }} />
                </div>
                <span className="text-xs text-muted-foreground font-medium">
                  {Object.values(typingUsers)
                    .map((u) => u.senderName)
                    .join(", ")}{" "}
                  {Object.keys(typingUsers).length > 1 ? "estão digitando..." : "está digitando..."}
                </span>
              </div>
            )}
          </div>

          <AnimatePresence>
            {showScrollButton && (
              <motion.button
                initial={{ opacity: 0, scale: 0.8 }}
                animate={{ opacity: 1, scale: 1 }}
                exit={{ opacity: 0, scale: 0.8 }}
                onClick={scrollToBottom}
                className="absolute bottom-24 right-6 p-2.5 bg-card border border-border rounded-full shadow-lg text-muted-foreground hover:text-primary-600 transition-colors z-20"
              >
                <ArrowDown className="w-5 h-5" />
              </motion.button>
            )}
          </AnimatePresence>

          <div className="flex-none px-2 sm:px-4 md:px-6 py-2 sm:py-3 bg-card border-t border-border">
            {activeChannel.status === "closed" || activeChannel.status === "archived" ? (
              <div className="flex items-center justify-center p-3 bg-muted border border-border rounded-xl text-muted-foreground text-sm">
                <Lock className="w-4 h-4 mr-2" />
                {activeChannel.status === "closed" ? "Canal encerrado" : "Canal arquivado"}
              </div>
            ) : (
              <>
                <AnimatePresence>
                  {(replyTo || editingMessage) && (
                    <motion.div initial={{ height: 0, opacity: 0 }} animate={{ height: "auto", opacity: 1 }} exit={{ height: 0, opacity: 0 }} className="overflow-hidden">
                      <div className="flex items-center gap-3 px-3 py-2 mb-2 bg-muted rounded-lg border-l-[3px] border-l-primary-500">
                        {editingMessage ? <Edit3 className="w-4 h-4 text-primary-500 flex-shrink-0" /> : <Reply className="w-4 h-4 text-primary-500 flex-shrink-0" />}
                        <div className="flex-1 min-w-0">
                          <div className="text-caption font-bold text-primary-600 dark:text-primary-400">{editingMessage ? "Editando mensagem" : replyTo?.sender_name}</div>
                          <div className="text-xs text-muted-foreground truncate">{editingMessage?.content || replyTo?.content}</div>
                        </div>
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => {
                            setReplyTo(null);
                            setEditingMessage(null);
                            setMessageText("");
                          }}
                          className="size-7 text-muted-foreground"
                        >
                          <X className="w-4 h-4" />
                        </Button>
                      </div>
                    </motion.div>
                  )}
                </AnimatePresence>

                <AnimatePresence>
                  {attachments.length > 0 && (
                    <motion.div initial={{ height: 0, opacity: 0 }} animate={{ height: "auto", opacity: 1 }} exit={{ height: 0, opacity: 0 }} className="overflow-hidden mb-2">
                      <div className="flex flex-wrap gap-2 px-1">
                        {attachments.map((file, idx) => (
                          <div key={idx} className="relative group flex items-center gap-2 px-3 py-2 bg-muted rounded-lg border border-border max-w-xs">
                            {file.type.startsWith("image/") ? (
                              <div className="w-8 h-8 rounded bg-muted overflow-hidden flex-shrink-0">
                                <img src={URL.createObjectURL(file)} alt="preview" className="w-full h-full object-cover" />
                              </div>
                            ) : (
                              <FileIcon className="w-5 h-5 text-primary-500 flex-shrink-0" />
                            )}
                            <div className="min-w-0 flex-1">
                              <p className="text-caption font-medium text-foreground truncate">{file.name}</p>
                              <p className="text-micro text-muted-foreground">{formatFileSize(file.size)}</p>
                            </div>
                            <Button
                              variant="secondary"
                              size="icon"
                              onClick={() => removeAttachment(idx)}
                              className="absolute -top-1.5 -right-1.5 size-5 rounded-full text-muted-foreground opacity-0 shadow-sm transition-opacity hover:text-status-critical group-hover:opacity-100"
                            >
                              <X className="w-3 h-3" />
                            </Button>
                          </div>
                        ))}
                      </div>
                    </motion.div>
                  )}
                </AnimatePresence>

                <div className="flex items-end gap-1 md:gap-2 bg-muted border border-border rounded-2xl px-2 py-1.5 focus-within:ring-2 focus-within:ring-primary-500 focus-within:border-transparent transition-all">
                  <input type="file" multiple ref={fileInputRef} className="hidden" onChange={handleFileUpload} />
                  <Button variant="ghost" size="icon" onClick={() => fileInputRef.current?.click()} className="size-9 shrink-0 text-muted-foreground hover:text-primary" title="Anexar arquivo">
                    <Paperclip className="w-5 h-5" />
                  </Button>

                  <AnimatePresence>
                    {autocomplete.type && autocomplete.results.length > 0 && (
                      <motion.div
                        initial={{ opacity: 0, y: 10 }}
                        animate={{ opacity: 1, y: 0 }}
                        exit={{ opacity: 0, y: 10 }}
                        className="absolute bottom-full left-4 mb-2 bg-card rounded-xl shadow-xl border border-border w-64 max-h-60 overflow-y-auto z-50"
                      >
                        <div className="p-2 border-b border-border bg-muted /50">
                          <span className="text-xs font-semibold text-muted-foreground uppercase">
                            {autocomplete.type === "mention" ? "Mencionar Usuário" : autocomplete.type === "hashtag" ? "Tags Populares" : "Comandos"}
                          </span>
                        </div>
                        <ul className="p-1">
                          {autocomplete.results.map((result) => (
                            <li key={result.id}>
                              <button
                                onClick={() => handleAutocompleteSelect(result.id, result.name)}
                                className="w-full text-left px-3 py-2 text-sm text-foreground hover:bg-primary-50 hover:text-primary-600 dark:hover:text-primary-400 rounded-lg flex items-center justify-between transition-colors"
                              >
                                <span className="font-medium">
                                  {autocomplete.type === "mention" ? "@" : autocomplete.type === "hashtag" ? "#" : ""}
                                  {result.name}
                                </span>
                                {result.desc && <span className="text-xs text-muted-foreground">{result.desc}</span>}
                              </button>
                            </li>
                          ))}
                        </ul>
                      </motion.div>
                    )}
                  </AnimatePresence>

                  <textarea
                    ref={textareaRef}
                    rows={1}
                    placeholder="Digite sua mensagem..."
                    value={messageText}
                    onChange={(e) => {
                      setMessageText(e.target.value);
                      handleTyping();
                    }}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && !e.shiftKey) {
                        e.preventDefault();
                        editingMessage ? handleEditMessage() : handleSendMessage();
                      }
                      if (e.key === "Escape") {
                        setReplyTo(null);
                        setEditingMessage(null);
                        setMessageText("");
                      }
                    }}
                    className="flex-1 bg-transparent border-0 focus:ring-0 resize-none py-2 px-1 text-sm text-foreground placeholder:text-muted-foreground dark:placeholder:text-muted-foreground outline-none"
                    style={{ minHeight: "40px", maxHeight: "160px" }}
                  />

                  <div className="w-px h-6 bg-border mx-1 self-center hidden md:block" />

                  <div className="relative hidden md:block">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => setShowTemplates(!showTemplates)}
                      className={clsx("size-9 shrink-0", showTemplates ? "bg-primary/10 text-primary" : "text-muted-foreground")}
                      title="Templates de Mensagem"
                    >
                      <LayoutTemplate className="w-5 h-5" />
                    </Button>

                    <AnimatePresence>
                      {showTemplates && (
                        <motion.div
                          initial={{ opacity: 0, y: 8 }}
                          animate={{ opacity: 1, y: 0 }}
                          exit={{ opacity: 0, y: 8 }}
                          className="absolute bottom-full right-0 mb-2 w-[320px] bg-card border border-border shadow-2xl rounded-xl z-50 overflow-hidden"
                        >
                          <div className="px-3 py-2 border-b border-border bg-muted /50">
                            <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">Templates Rápidos</span>
                          </div>
                          <div className="max-h-[250px] overflow-y-auto p-2 space-y-1">
                            {templates.map((t) => (
                              <button key={t.id} onClick={() => handleTemplateSelect(t)} className="w-full text-left p-2 rounded-lg hover:bg-muted transition-colors">
                                <div className="text-sm font-medium text-foreground mb-0.5">{t.name}</div>
                                <div className="text-caption text-muted-foreground line-clamp-2 leading-relaxed">{t.content}</div>
                              </button>
                            ))}
                          </div>
                        </motion.div>
                      )}
                    </AnimatePresence>
                  </div>

                  <div className="relative" ref={emojiPickerRef}>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={(e) => {
                        e.stopPropagation();
                        setShowEmojiPicker(!showEmojiPicker);
                      }}
                      aria-label="Emojis"
                      aria-expanded={showEmojiPicker}
                      className={clsx("size-9 shrink-0", showEmojiPicker ? "text-primary" : "text-muted-foreground")}
                    >
                      <Smile className="w-5 h-5" />
                    </Button>

                    <AnimatePresence>
                      {showEmojiPicker && (
                        <motion.div
                          initial={{ opacity: 0, y: 8 }}
                          animate={{ opacity: 1, y: 0 }}
                          exit={{ opacity: 0, y: 8 }}
                          className="absolute bottom-full right-0 mb-2 w-[320px] bg-card border border-border shadow-2xl rounded-2xl z-50 overflow-hidden"
                          onClick={(e) => e.stopPropagation()}
                        >
                          <div className="flex border-b border-border px-2 pt-2">
                            {EMOJI_CATEGORIES.map((cat, i) => (
                              <button
                                key={i}
                                onClick={() => setEmojiCategory(i)}
                                className={clsx(
                                  "px-2.5 py-1.5 text-caption font-semibold rounded-t-lg transition-colors",
                                  emojiCategory === i
                                    ? "bg-primary-50 dark:bg-primary-900/30 text-primary-600 dark:text-primary-400"
                                    : "text-muted-foreground hover:text-foreground",
                                )}
                              >
                                {cat.label}
                              </button>
                            ))}
                          </div>
                          <div className="p-3 grid grid-cols-8 gap-1 max-h-[200px] overflow-y-auto">
                            {EMOJI_CATEGORIES[emojiCategory].emojis.map((emoji) => (
                              <button
                                key={emoji}
                                onClick={() => {
                                  setMessageText((prev) => prev + emoji);
                                  textareaRef.current?.focus();
                                }}
                                className="w-8 h-8 flex items-center justify-center text-lg hover:bg-muted rounded-lg transition-colors"
                              >
                                {emoji}
                              </button>
                            ))}
                          </div>
                        </motion.div>
                      )}
                    </AnimatePresence>
                  </div>

                  <Button
                    size="icon"
                    onClick={editingMessage ? handleEditMessage : handleSendMessage}
                    disabled={editingMessage ? !messageText.trim() : !messageText.trim() && attachments.length === 0}
                    loading={isSending}
                    className="shrink-0"
                  >
                    {editingMessage ? <Check className="w-5 h-5" /> : <Send className="w-5 h-5" />}
                  </Button>
                </div>
              </>
            )}
          </div>
        </div>
      ) : (
        <div className="hidden md:flex flex-1 items-center justify-center bg-muted ">
          <div className="text-center max-w-sm">
            <div className="w-20 h-20 rounded-3xl bg-primary-50 dark:bg-primary-900/30 flex items-center justify-center mx-auto mb-5">
              <MessageSquare className="w-10 h-10 text-primary-400" />
            </div>
            <h2 className="text-lg font-bold text-foreground dark:text-white mb-2">Central de Mensagens</h2>
            <p className="text-sm text-muted-foreground mb-6 leading-relaxed">
              Canal de comunicação para transações imobiliárias. Conecte-se com leads, clientes, proprietários, corretores e parceiros.
            </p>
            <Button onClick={() => setIsCreating(true)}>
              <Plus className="w-4 h-4" /> Iniciar Conversa
            </Button>
          </div>
        </div>
      )}

      <AnimatePresence>
        {activeChannel && showDetails && (
          <motion.div
            initial={{ width: 0, opacity: 0 }}
            animate={{ width: 320, opacity: 1 }}
            exit={{ width: 0, opacity: 0 }}
            transition={{ duration: 0.2 }}
            className="absolute md:relative right-0 z-50 flex-shrink-0 border-l border-border bg-card flex flex-col h-full overflow-hidden shadow-2xl md:shadow-none"
            style={{ maxWidth: "100vw" }}
          >
            <div className="h-[68px] flex items-center justify-between px-5 border-b border-border ">
              <h3 className="font-bold text-foreground dark:text-white text-sm">Detalhes</h3>
              <Button variant="ghost" size="icon" onClick={() => setShowDetails(false)} className="size-8 text-muted-foreground">
                <X className="w-4 h-4" />
              </Button>
            </div>

            <div className="flex-1 overflow-y-auto">
              <div className="px-5 py-6 flex flex-col items-center text-center border-b border-border ">
                <div className={clsx("w-20 h-20 rounded-2xl flex items-center justify-center text-white font-bold text-2xl mb-3", avatarColor(activeChannel.id))}>
                  {activeChannel.type === "direct" ? (
                    activeChannelAvatar ? (
                      <img
                        src={getMediaUrl(activeChannelAvatar!)}
                        alt="Avatar"
                        className="w-full h-full object-cover rounded-full shrink-0"
                      />
                    ) : (
                      getInitials(channelDisplayName)
                    )
                  ) : (
                    (() => {
                      const I = CHANNEL_TYPE_META[activeChannel.type]?.icon || Hash;
                      return <I className="w-8 h-8" />;
                    })()
                  )}
                </div>
                <h4 className="text-base font-bold text-foreground dark:text-white">
                  {channelDisplayName}
                </h4>
                {activeChannel.description && <p className="text-xs text-muted-foreground mt-1">{activeChannel.description}</p>}
                <div className="flex items-center gap-2 mt-3">
                  <span className={clsx("px-2 py-1 rounded-md text-micro font-semibold", CHANNEL_TYPE_META[activeChannel.type]?.color)}>
                    {CHANNEL_TYPE_META[activeChannel.type]?.label}
                  </span>
                  <span
                    className={clsx(
                      "px-2 py-1 rounded-md text-micro font-semibold",
                      activeChannel.status === "open" ? "bg-status-ok-soft text-status-ok" : "bg-muted text-muted-foreground",
                    )}
                  >
                    {activeChannel.status === "open" ? "Ativo" : activeChannel.status === "archived" ? "Arquivado" : "Encerrado"}
                  </span>
                </div>
              </div>

              <div className="px-5 py-4 border-b border-border grid grid-cols-3 gap-2">
                <Button variant="ghost" onClick={handleToggleMuteChannel} className="h-auto flex-col gap-1.5 py-2">
                  {activeChannel.is_muted ? <Bell className="w-4 h-4 text-muted-foreground" /> : <BellOff className="w-4 h-4 text-muted-foreground" />}
                  <span className="text-micro font-medium text-muted-foreground">{activeChannel.is_muted ? "Ativar" : "Silenciar"}</span>
                </Button>
                <Button variant="ghost" onClick={handleTogglePinChannel} className="h-auto flex-col gap-1.5 py-2">
                  {activeChannel.is_pinned ? <PinOff className="w-4 h-4 text-status-warning" /> : <Pin className="w-4 h-4 text-muted-foreground" />}
                  <span className="text-micro font-medium text-muted-foreground">{activeChannel.is_pinned ? "Desafixar" : "Fixar"}</span>
                </Button>
                <Button variant="ghost" onClick={handleArchiveChannel} className="h-auto flex-col gap-1.5 py-2">
                  <Archive className="w-4 h-4 text-muted-foreground" />
                  <span className="text-micro font-medium text-muted-foreground">Arquivar</span>
                </Button>
              </div>

              <div className="px-5 py-4">
                <h5 className="text-caption font-bold text-muted-foreground uppercase tracking-wider mb-3">
                  Participantes ({activeChannel.participants?.length || 0})
                </h5>
                <div className="space-y-2">
                  {activeChannel.participants?.map((p) => {
                    const participant = typeof p === "string" ? { id: p, name: p, role: "member" as const, joined_at: "" } : p;
                    const pId = participant.id;
                    const fallbackName = typeof p === "string" ? pId : participant.name;
                    const pName = resolveParticipantName(pId, fallbackName);
                    const pRole = participant.role;
                    const pEntityType = (participant as Participant).entity_type;

                    return (
                      <div key={pId} className="flex items-center justify-between group py-1">
                        <div className="flex items-center gap-3 min-w-0">
                          <Avatar className="w-9 h-9 flex-shrink-0">
                            {resolveParticipantAvatar(pId, (participant as any).avatar_url) && (
                              <AvatarImage src={getMediaUrl(resolveParticipantAvatar(pId, (participant as any).avatar_url)!)} alt="Avatar" />
                            )}
                            <AvatarFallback className={clsx("text-white text-xs font-bold", avatarColor(pId))}>{getInitials(pName)}</AvatarFallback>
                          </Avatar>
                          <div className="min-w-0">
                            <div className="text-sm font-medium text-foreground truncate">{pName}</div>
                            <div className="flex items-center gap-1.5">
                              {pRole === "admin" && (
                                <span className="text-caption font-bold text-primary-600 dark:text-primary-400 bg-primary-50 dark:bg-primary-900/30 px-1.5 py-0.5 rounded">
                                  Admin
                                </span>
                              )}
                              {pEntityType && <span className="text-micro text-muted-foreground capitalize">{pEntityType}</span>}
                            </div>
                          </div>
                        </div>
                        {pId !== userId && activeChannel.status === "open" && (
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => handleRemoveParticipant(pId)}
                            className="size-7 text-muted-foreground/60 opacity-0 transition-all hover:text-status-critical group-hover:opacity-100"
                            title="Remover"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </Button>
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>

              {/* --- Channel Info Panel (auto-populated, read-only) --- */}
              {activeChannel.type === "deal" && (
                <div className="px-5 py-4 border-b border-border">
                  <h5 className="text-caption font-bold text-muted-foreground uppercase tracking-wider mb-3">
                    Informações do Negócio
                  </h5>

                  {activeChannel.info?.sections?.length ? (
                    <div className="space-y-3">
                      {activeChannel.info.sections.map((section, si) => (
                        <div key={si}>
                          {section.label && (
                            <h6 className="text-micro font-bold text-muted-foreground uppercase tracking-wider mb-1.5">
                              {section.label}
                            </h6>
                          )}
                          <div className="space-y-1.5">
                            {section.items.map((item, ii) => (
                              <div key={ii} className="flex items-center justify-between gap-2 py-0.5">
                                <div className="flex-1 min-w-0">
                                  <div className="text-xs font-medium text-foreground truncate">{item.label}</div>
                                  {item.value && (
                                    <div className="text-micro text-muted-foreground truncate mt-0.5">{item.value}</div>
                                  )}
                                </div>
                                {item.link && (
                                  <a
                                    href={item.link}
                                    target="_blank"
                                    rel="noopener noreferrer"
                                    className="shrink-0 inline-flex items-center gap-1 px-2 py-1 rounded-md text-micro font-medium bg-primary-50 dark:bg-primary-900/20 text-primary-600 dark:text-primary-400 hover:bg-primary-100 dark:hover:bg-primary-900/30 transition-colors"
                                  >
                                    {item.link.startsWith("/esteira/deals/")
                                      ? "Abrir negócio"
                                      : item.link.startsWith("/imoveis/")
                                        ? "Ver imóvel"
                                        : item.link.startsWith("/leads/")
                                          ? "Ver lead"
                                          : "Abrir"}
                                    <ExternalLink className="w-3 h-3" />
                                  </a>
                                )}
                              </div>
                            ))}
                          </div>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="text-xs text-muted-foreground italic">
                      As informações do negócio serão carregadas automaticamente.
                    </p>
                  )}
                </div>
              )}

              {activeChannel.status === "open" && (
                <div className="px-5 py-4 border-t border-border ">
                  <Button
                    variant="ghost"
                    onClick={handleCloseChannel}
                    className="w-full bg-status-critical-soft text-status-critical hover:bg-status-critical-soft hover:text-status-critical"
                  >
                    <Lock className="w-4 h-4" /> Encerrar Canal
                  </Button>
                </div>
              )}
            </div>
          </motion.div>
        )}
      </AnimatePresence>

        {contextMenu && (
          <Dialog open onOpenChange={open => { if (!open) setContextMenu(null); }}>
            <DialogContent aria-describedby={undefined} className="sm:max-w-sm gap-0 overflow-hidden p-0" onCloseAutoFocus={event => {
              event.preventDefault();
              const target = menuFocusComposer.current ? textareaRef.current : menuOpener.current;
              if (target?.isConnected) target.focus();
              else textareaRef.current?.focus();
            }}>
            <DialogTitle className="border-b border-border px-4 py-3 text-sm font-bold">Ações da mensagem</DialogTitle>
            <div className="py-1">
              <button onClick={() => startReply(contextMenu)} className="w-full flex items-center gap-3 px-4 py-2.5 text-sm text-foreground hover:bg-muted transition-colors">
                <Reply className="w-4 h-4 text-muted-foreground" /> Responder
              </button>

              <div className="flex items-center gap-1 px-4 py-2 border-t border-b border-border ">
                {QUICK_REACTIONS.map((emoji) => (
                  <button
                    key={emoji}
                    aria-label={`Reagir ${emoji}`}
                    onClick={() => {
                      handleReaction(contextMenu, emoji);
                      setContextMenu(null);
                    }}
                    className="p-1.5 hover:bg-muted rounded-lg text-base transition-colors"
                  >
                    {emoji}
                  </button>
                ))}
              </div>

              <button onClick={() => copyMessage(contextMenu)} className="w-full flex items-center gap-3 px-4 py-2.5 text-sm text-foreground hover:bg-muted transition-colors">
                <Copy className="w-4 h-4 text-muted-foreground" /> Copiar
              </button>

              <button
                onClick={() => {
                  handleTogglePin(contextMenu);
                  setContextMenu(null);
                }}
                className="w-full flex items-center gap-3 px-4 py-2.5 text-sm text-foreground hover:bg-muted transition-colors"
              >
                {contextMenu.is_pinned ? <PinOff className="w-4 h-4 text-muted-foreground" /> : <Pin className="w-4 h-4 text-muted-foreground" />}
                {contextMenu.is_pinned ? "Desafixar" : "Fixar"}
              </button>

              {contextMenu.sender_id === userId && (
                <>
                  <button onClick={() => startEdit(contextMenu)} className="w-full flex items-center gap-3 px-4 py-2.5 text-sm text-foreground hover:bg-muted transition-colors">
                    <Edit3 className="w-4 h-4 text-muted-foreground" /> Editar
                  </button>
                  <button
                    onClick={() => {
                      handleDeleteMessage(contextMenu);
                    }}
                    className="w-full flex items-center gap-3 px-4 py-2.5 text-sm text-status-critical hover:bg-status-critical-soft dark:hover:bg-status-critical/20 transition-colors"
                  >
                    <Trash2 className="w-4 h-4" /> Apagar
                  </button>
                </>
              )}
            </div>
            </DialogContent>
          </Dialog>
        )}

      {confirmationDialog}
      {viewer && (
        <DocumentViewerDialog
          open={!!viewer}
          onOpenChange={(o) => !o && setViewer(null)}
          url={viewer.url}
          title={viewer.title}
          contentType={viewer.contentType}
        />
      )}
    </div>
  );
}
