import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Sparkles,
  X,
  Send,
  Square,
  Minimize2,
  Maximize2,
  Wand2,
  Plus,
  Clock,
  Trash2,
  List,
  BarChart3,
  Filter,
  Wrench,
  RefreshCw,
  ChevronDown,
  ChevronUp,
} from "lucide-react";
import { useLocation, useNavigate } from "react-router";
import { Button } from "../../ui/button";
import { Textarea } from "../../ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "../../ui/tooltip";
import { toast } from "sonner";
import {
  chatStream,
  confirmAction,
  clearConversations,
  deleteConversation,
  getSuggestions,
  listConversations,
  listMessages,
  listTools,
  type Conversation,
  type AIMessage,
  type MCPTool,
  ToolEvent,
} from "../../services/ai.service";
import { MessageContent } from "./MessageContent";
import { SuiteeIntelligenceMark } from "./SuiteeIntelligenceMark";
import { decidirDesfechoDoStream, escolherRespostaRecuperada } from "../../utils/chatStreamOutcome";
import { ToolCallBadge } from "./ToolCallBadge";
import { ActionCard, type SuiteeAction } from "./ActionCard";
import { ResultCards, type SuiteeCardsBlock } from "./ResultCards";
import { normalizeAiPath } from "../../utils/aiPaths";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "../../ui/sheet";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "../../ui/alert-dialog";
import { useOptionalAuth } from "../../context/AuthContext";
import { UserAvatar } from "../UserAvatar";
import { SlashMenu } from "./SlashMenu";
import { filterSlashCommands, type SlashCommand } from "./slashCommands";

interface Message {
  id: string;
  role: "user" | "assistant";
  content: string;
  timestamp: Date;
  streaming?: boolean;
}

type AIChatUser =
  | {
      email?: string;
      firstname?: string;
      lastname?: string;
      fullname?: string;
      avatar_url?: string;
    }
  | null
  | undefined;

// ── Ferramentas do sistema (MCP): rótulo amigável + prompt que dispara a
// ferramenta no backend (roteamento automático read-only ou chamada do
// modelo) — mesmo padrão dos cards do chat_service.
const TOOL_LABELS: Record<string, string> = {
  search_leads: "Buscar leads",
  get_lead_by_id: "Detalhar lead",
  get_lead_by_email: "Lead por e-mail",
  get_lead_by_phone: "Lead por telefone",
  get_lead_by_display_id: "Lead por código",
  create_lead: "Criar lead",
  add_lead_note: "Adicionar nota ao lead",
  add_lead_note_from_screen: "Nota ao lead da tela",
  update_lead: "Atualizar lead",
  search_properties: "Buscar imóveis",
  get_property_by_id: "Detalhar imóvel",
  create_property: "Criar imóvel",
  search_brokers: "Buscar corretores",
  get_brokers: "Corretores",
  search_deals: "Buscar negócios",
  get_deal_by_id: "Detalhar negócio",
  get_deals_pipeline: "Pipeline de negócios",
  create_deal: "Criar negócio",
  create_deal_from_lead_and_property: "Negócio de lead + imóvel",
  get_calendar_events: "Compromissos",
  search_appointments: "Buscar compromissos",
  create_appointment: "Criar compromisso",
  search_charges: "Buscar cobranças",
  search_transactions: "Buscar transações",
  search_transfers: "Buscar repasses",
  get_financial_dashboard: "Resumo financeiro",
  get_financial_transactions: "Transações financeiras",
  get_financial_charges: "Cobranças financeiras",
  get_financial_transfers: "Repasses financeiros",
  get_documents: "Acessar documentos",
  get_contracts: "Contratos",
  create_contract: "Criar contrato",
  get_fiscal_reports: "Relatórios fiscais",
  get_delinquency_aging: "Inadimplência",
};

const toolLabel = (name: string): string => TOOL_LABELS[name] ?? name.replace(/_/g, " ");

const toolPrompt = (name: string): string => {
  switch (name) {
    case "search_leads":
      return "Liste os 10 leads mais recentes com nome, telefone, status e corretor.";
    case "search_properties":
      return "Liste os imóveis com status, valor, tipo e bairro.";
    case "get_calendar_events":
      return "Mostre meus compromissos de hoje e amanhã.";
    case "get_deals_pipeline":
      return "Mostre o pipeline completo com quantidade e valor por etapa.";
    case "get_financial_dashboard":
      return "Mostre o resumo financeiro do mês corrente.";
    case "get_lead_by_id":
      return "Resuma este lead em detalhes, incluindo notas e próxima ação.";
    case "get_property_by_id":
      return "Resuma este imóvel em detalhes, incluindo status, valor e condições.";
    case "get_deal_by_id":
      return "Resuma este negócio em detalhes, incluindo proposta e checklist.";
    case "search_charges":
      return "Mostre as cobranças em atraso com valor, cliente e dias de atraso.";
    case "search_transfers":
      return "Mostre os repasses pendentes com valor, proprietário e competência.";
    default:
      return `Use a ferramenta ${name} para me ajudar e resuma o resultado.`;
  }
};

interface AIChatProps {
  isOpen: boolean;
  onClose: () => void;
  mode?: "panel" | "page";
  user?: AIChatUser;
}

// WATCHDOG_SILENCIO_MS é o tempo sem NENHUM evento antes de considerar o stream
// morto. Não é o tempo máximo de uma resposta: com stream já iniciado, silêncio
// é o modelo inferindo, e abortar aí descarta a resposta (o backend cancela a
// geração e nada é persistido). O valor cobre com folga os piores casos medidos
// (10 a 36 segundos antes da correção de payload).
const WATCHDOG_SILENCIO_MS = 180_000;

export function AIChat({ isOpen, onClose, mode = "panel", user: userProp }: AIChatProps) {
  const location = useLocation();
  const navigate = useNavigate();
  const auth = useOptionalAuth();
  const user = userProp ?? (auth?.user as any) ?? null;
  const [isExpanded, setIsExpanded] = useState(false);
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [isTyping, setIsTyping] = useState(false);
  const [activeTools, setActiveTools] = useState<ToolEvent[]>([]);
  const [currentConversationId, setCurrentConversationId] = useState<string | null>(null);
  const [isConnected, setIsConnected] = useState<boolean | null>(null);
  const [availableTools, setAvailableTools] = useState<MCPTool[] | null>(null);
  const [toolsError, setToolsError] = useState<string | null>(null);
  const [showAllTools, setShowAllTools] = useState(false);
  const isPage = mode === "page";
  const open = isPage ? true : isOpen;
  const abortRef = useRef<AbortController | null>(null);
  const gotAssistantMessageRef = useRef(false);
  const limiteAtingidoRef = useRef(false);
  const isToolingRef = useRef(false);
  const hasUserInteractedRef = useRef(false);
  const hydrationSeqRef = useRef(0);
  const messagesLenRef = useRef(0);
  const currentConversationIdRef = useRef<string | null>(null);
  // Resiliência de streaming: reconexão única em falha silenciosa + watchdog
  // (se nenhum evento chegar por 45s, aborta e tenta de novo uma vez).
  const automaticRetryRef = useRef(false);
  const lastEventAtRef = useRef(0);
  const watchdogAbortedRef = useRef(false);
  // Qualquer evento SSE conta como sinal de vida — inclusive `tool_start`, que
  // chega antes do texto.
  const recebeuEventoRef = useRef(false);
  // O stream pode fechar sem `done` nem `error` — o backend perde a conexão no
  // meio (ferramenta MCP falhando, por exemplo) e o `chatStream` resolve como se
  // tivesse acabado. Sem esta marca, o `finally` não distingue "terminou" de
  // "morreu no meio": o usuário fica com a resposta truncada, ou com a bolha
  // "Analisando..." quando o preâmbulo já tinha chegado.
  const conclusaoRecebidaRef = useRef(false);
  // `error` é uma conclusão declarada pelo backend: a mensagem já foi mostrada,
  // então o finally não deve tratar como queda de conexão.
  const erroRecebidoRef = useRef(false);
  const isTypingRef = useRef(false);
  const sendingRef = useRef(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const [showSuggestions, setShowSuggestions] = useState(true);
  const [executingActionId, setExecutingActionId] = useState<string | null>(null);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [conversationPreviews, setConversationPreviews] = useState<Record<string, string>>({});
  const conversationPreviewsRef = useRef<Record<string, string>>({});
  const previewSeqRef = useRef(0);
  const [confirmState, setConfirmState] = useState<
    | { open: false }
    | { open: true; mode: "clear" }
    | { open: true; mode: "delete"; conversationId: string; title: string }
  >({ open: false });
  const [slashOpen, setSlashOpen] = useState(false);
  const [slashActiveIndex, setSlashActiveIndex] = useState(0);

  const lastConversationStorageKey = useMemo(() => `suitee.ai.lastConversationId:${user?.email || "anon"}`, [user?.email]);

  const historyCacheStorageKey = useMemo(() => {
    let tenant = "no_tenant";
    try {
      tenant = localStorage.getItem("current_company_id") || "no_tenant";
    } catch {
      tenant = "no_tenant";
    }
    return `suitee.ai.historyCache:${tenant}:${user?.email || "anon"}`;
  }, [user?.email]);

  useEffect(() => {
    try {
      const raw = localStorage.getItem(historyCacheStorageKey);
      if (!raw) return;
      const parsed = JSON.parse(raw) as {
        conversations?: Conversation[];
        previews?: Record<string, string>;
      };
      if (Array.isArray(parsed.conversations)) setConversations(parsed.conversations);
      if (parsed.previews && typeof parsed.previews === "object") setConversationPreviews(parsed.previews);
    } catch {
    }
  }, [historyCacheStorageKey]);

  useEffect(() => {
    try {
      localStorage.setItem(
        historyCacheStorageKey,
        JSON.stringify({
          conversations,
          previews: conversationPreviews,
        }),
      );
    } catch {
    }
  }, [conversations, conversationPreviews, historyCacheStorageKey]);

  const slashQuery = useMemo(() => {
    if (!input.startsWith("/")) return "";
    return input.slice(1);
  }, [input]);

  const slashCommands = useMemo(() => (slashOpen ? filterSlashCommands(slashQuery) : []), [slashOpen, slashQuery]);

  const userName = useMemo(() => {
    const full = (user?.fullname || `${user?.firstname ?? ""} ${user?.lastname ?? ""}`).trim();
    return full || user?.email || "Você";
  }, [user?.email, user?.firstname, user?.fullname, user?.lastname]);

  const formatConversationTime = useCallback((iso: string) => {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return "";
    return d.toLocaleString("pt-BR", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
  }, []);

  const refreshConversations = useCallback(async () => {
    setHistoryLoading(true);
    try {
      const list = await listConversations(30);
      setConversations(list || []);
    } catch {
      toast.error("Não foi possível atualizar o histórico");
    } finally {
      setHistoryLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!historyOpen) return;
    refreshConversations();
  }, [historyOpen, refreshConversations]);

  useEffect(() => {
    conversationPreviewsRef.current = conversationPreviews;
  }, [conversationPreviews]);

  useEffect(() => {
    if (!historyOpen) return;
    if (conversations.length === 0) return;
    const seq = ++previewSeqRef.current;
    const targets = conversations
      .slice(0, 15)
      .map((c) => c.id)
      .filter((id) => !conversationPreviewsRef.current[id]);
    if (targets.length === 0) return;

    const sanitize = (s: string) => {
      const stripped = s
        .replace(/```suitee_(cards|action)[\s\S]*?```/g, "")
        .replace(/\s+/g, " ")
        .trim();
      if (stripped.length <= 140) return stripped;
      return stripped.slice(0, 140).trim() + "…";
    };

    Promise.all(
      targets.map(async (id) => {
        try {
          const msgs = (await listMessages(id, 200)) as AIMessage[];
          const last = [...msgs].reverse().find((m) => m.role === "assistant" || m.role === "user");
          const preview = sanitize(last?.content || "");
          return { id, preview };
        } catch {
          return { id, preview: "" };
        }
      }),
    ).then((rows) => {
      if (seq !== previewSeqRef.current) return;
      setConversationPreviews((prev) => {
        const next = { ...prev };
        for (const r of rows) next[r.id] = r.preview;
        return next;
      });
    });
  }, [historyOpen, conversations]);

  const loadConversation = useCallback(
    async (id: string) => {
      if (!id) return;
      abortRef.current?.abort();
      gotAssistantMessageRef.current = false;
      hasUserInteractedRef.current = true;
      setActiveTools([]);
      setIsTyping(false);
      setExecutingActionId(null);
      setShowSuggestions(false);
      setCurrentConversationId(id);
      try {
        const msgs = await listMessages(id, 200);
        const ui: Message[] = [];
        for (const m of msgs) {
          if (m.role !== "user" && m.role !== "assistant") continue;
          if (m.role === "assistant" && !m.content) continue;
          ui.push({
            id: m.id,
            role: m.role,
            content: m.content,
            timestamp: new Date(m.created_at),
          });
        }
        setMessages(ui);
        inputRef.current?.focus();
      } catch {
        toast.error("Não foi possível carregar a conversa");
      }
    },
    [],
  );

  const contextLabel = useMemo(() => {
    const p = location.pathname || "";
    if (p.startsWith("/leads/")) return "Leads · Detalhes";
    if (p.startsWith("/leads")) return "Leads";
    if (p.startsWith("/roleta")) return "Roleta de leads";
    if (p.startsWith("/imoveis") || p.startsWith("/properties")) return "Imóveis";
    if (p.startsWith("/captacoes")) return "Captação de imóveis";
    if (p.startsWith("/contratos")) return "Contratos";
    if (p.startsWith("/modelos-contrato")) return "Modelos de contrato";
    if (p.startsWith("/documentos")) return "Documentos";
    if (p.startsWith("/agenda") || p.startsWith("/calendar")) return "Agenda";
    if (p.startsWith("/mensagens") || p.startsWith("/messages")) return "Mensagens";
    if (p.startsWith("/esteira")) return "Esteira";
    if (p.startsWith("/financeiro") || p.startsWith("/finance")) return "Financeiro";
    if (p.startsWith("/fiscal")) return "Fiscal";
    if (p.startsWith("/inadimplencia")) return "Inadimplência";
    if (p.startsWith("/relatorios") || p.startsWith("/reports")) return "Relatórios";
    if (p.startsWith("/locacao")) return "Locação";
    if (p.startsWith("/manutencao")) return "Manutenção";
    if (p.startsWith("/chamados")) return "Chamados";
    if (p.startsWith("/parceiros")) return "Parceiros";
    return "Tela atual";
  }, [location.pathname]);

  const screenContext = useMemo(
    () => {
      const path = location.pathname || "";
      let entity = "";
      let entity_id = "";
      const leadMatch = path.match(/^\/leads\/([^/]+)/);
      if (leadMatch?.[1]) {
        entity = "lead";
        entity_id = leadMatch[1];
      }
      const propMatch = path.match(/^\/(imoveis|properties)\/([^/]+)/);
      if (propMatch?.[2]) {
        entity = "property";
        entity_id = propMatch[2];
      }
      const dealMatch = path.match(/^\/esteira\/deals\/([^/]+)/);
      if (dealMatch?.[1]) {
        entity = "deal";
        entity_id = dealMatch[1];
      }
      return {
        path,
        search: location.search,
        label: contextLabel,
        entity,
        entity_id,
      };
    },
    [contextLabel, location.pathname, location.search],
  );

  const [suggestions, setSuggestions] = useState<Array<{ label: string; prompt: string }>>([]);
  const [isLoadingSuggestions, setIsLoadingSuggestions] = useState(false);

  useEffect(() => {
    if (!open) return;
    if (!showSuggestions) return;
    if (messages.length > 0) return;
    setIsLoadingSuggestions(true);
    getSuggestions(contextLabel, location.pathname)
      .then((s) => setSuggestions(s || []))
      .catch(() =>
        setSuggestions([
          { label: "Resumir contexto", prompt: `Resuma o contexto da tela (${contextLabel}) e sugira os próximos 3 passos.` },
          { label: "Próximos passos", prompt: "Liste os 3 próximos passos mais prováveis e o porquê." },
          { label: "Checar pendências", prompt: "O que eu devo checar agora nesta tela? Seja objetivo." },
        ]),
      )
      .finally(() => setIsLoadingSuggestions(false));
  }, [contextLabel, open, messages.length, showSuggestions, location.pathname]);

  const parseSuiteeActions = (content: string | undefined | null): { cleaned: string; actions: SuiteeAction[] } => {
    const actions: SuiteeAction[] = [];
    const safe = typeof content === "string" ? content : "";
    const re = /```suitee_action\s*([\s\S]*?)```/g;
    const cleaned = safe.replace(re, (full, jsonText) => {
      try {
        const parsed = JSON.parse(String(jsonText).trim());
        if (parsed && typeof parsed === "object" && typeof parsed.title === "string") {
          actions.push(parsed as SuiteeAction);
          return "";
        }
      } catch {
      }
      return full;
    });
    return { cleaned: cleaned.trim(), actions };
  };

  const parseSuiteeCards = (content: string | undefined | null): { cleaned: string; blocks: SuiteeCardsBlock[] } => {
    const blocks: SuiteeCardsBlock[] = [];
    const safe = typeof content === "string" ? content : "";
    const re = /```suitee_cards\s*([\s\S]*?)```/g;
    const cleaned = safe.replace(re, (full, jsonText) => {
      try {
        const parsed = JSON.parse(String(jsonText).trim());
        if (parsed && typeof parsed === "object" && Array.isArray(parsed.items)) {
          blocks.push(parsed as SuiteeCardsBlock);
          return "";
        }
      } catch {
      }
      return full;
    });
    return { cleaned: cleaned.trim(), blocks };
  };

  const getRenderableAssistantContent = (content: string | undefined | null, streaming?: boolean) => {
    const safe = typeof content === "string" ? content : "";
    if (!streaming) return safe;
    const start = safe.indexOf("```suitee_");
    if (start < 0) return safe;
    const end = safe.indexOf("```", start + 3);
    if (end < 0) return safe.slice(0, start).trim();
    return safe;
  };

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages, isTyping, open]);

  useEffect(() => {
    if (!slashOpen) return;
    setSlashActiveIndex((i) => {
      if (slashCommands.length <= 0) return 0;
      if (i < 0) return 0;
      if (i > slashCommands.length - 1) return slashCommands.length - 1;
      return i;
    });
  }, [slashOpen, slashCommands.length]);

  useEffect(() => {
    messagesLenRef.current = messages.length;
  }, [messages.length]);

  useEffect(() => {
    currentConversationIdRef.current = currentConversationId;
  }, [currentConversationId]);

  useEffect(() => {
    try {
      if (currentConversationId) {
        localStorage.setItem(lastConversationStorageKey, currentConversationId);
      } else {
        localStorage.removeItem(lastConversationStorageKey);
      }
    } catch {
    }
  }, [currentConversationId, lastConversationStorageKey]);

  useEffect(() => {
    isTypingRef.current = isTyping;
  }, [isTyping]);

  useEffect(() => {
    if (!open) return;
    setShowSuggestions(true);
  }, [open]);

  const reloadTools = useCallback(async () => {
    try {
      const res = await listTools();
      setIsConnected(res.mcp_connected);
      setAvailableTools(res.tools ?? []);
      setToolsError(res.error ?? null);
    } catch {
      setIsConnected(false);
      setAvailableTools([]);
      setToolsError("Não foi possível consultar as ferramentas do sistema.");
    }
  }, []);

  useEffect(() => {
    if (!open) return;
    void reloadTools();
  }, [open, reloadTools]);

  const availableToolNames = useMemo(
    () => ((availableTools ?? []) as MCPTool[]).map((t) => t.function?.name).filter((n): n is string => Boolean(n)),
    [availableTools],
  );

  // Atalhos de ferramentas para a tela atual — consistente com o roteamento
  // do backend (chat_tool_routing.go): listagens setam o label da tela,
  // detalhes usam entity do contexto.
  const screenShortcutTools = useMemo(() => {
    const has = (...names: string[]) => names.filter((n) => availableToolNames.includes(n));
    const label = contextLabel.toLowerCase();
    const ent = screenContext.entity;
    if (ent === "lead")
      return has("get_lead_by_id", "add_lead_note_from_screen", "add_lead_note", "update_lead", "get_calendar_events");
    if (ent === "property")
      return has("get_property_by_id", "set_property_active_deal", "get_calendar_events", "create_appointment", "search_leads");
    if (ent === "deal")
      return has("get_deal_by_id", "get_deals_pipeline", "add_deal_checklist_item", "update_deal_status", "get_documents");
    if (label.includes("lead"))
      return has("search_leads", "create_lead", "search_properties", "get_calendar_events", "get_deals_pipeline");
    if (label.includes("imóve") || label.includes("imove") || label.includes("propriedad"))
      return has("search_properties", "create_property", "search_leads", "get_calendar_events");
    if (label.includes("esteira") || label.includes("negóci") || label.includes("negocia") || label.includes("deal"))
      return has("get_deals_pipeline", "search_deals", "create_deal", "search_leads", "get_documents");
    if (label.includes("financ"))
      return has("get_financial_dashboard", "search_charges", "search_transactions", "search_transfers", "get_financial_invoices");
    if (label.includes("agenda") || label.includes("calend") || label.includes("compromiss"))
      return has("get_calendar_events", "create_appointment", "search_appointments");
    if (label.includes("document")) return has("get_documents", "search_leads", "get_contracts");
    if (label.includes("contrato")) return has("get_contracts", "create_contract", "get_documents", "search_leads");
    if (label.includes("fiscal")) return has("get_fiscal_reports", "get_documents");
    if (label.includes("inadimpl")) return has("get_delinquency_aging", "search_charges");
    return has("search_leads", "search_properties", "get_calendar_events", "get_deals_pipeline", "get_financial_dashboard", "get_documents");
  }, [availableToolNames, contextLabel, screenContext.entity]);

  useEffect(() => {
    if (!open) return;
    (async () => {
      const seq = ++hydrationSeqRef.current;
      try {
        let preferredId = "";
        try {
          preferredId = localStorage.getItem(lastConversationStorageKey) || "";
        } catch {
          preferredId = "";
        }

        let conversationId = preferredId;
        if (!conversationId) {
          const convs = await listConversations(1);
          const latest = convs?.[0];
          if (!latest) return;
          conversationId = latest.id;
        }

        const msgs = await listMessages(conversationId, 200);
        if (seq !== hydrationSeqRef.current) return;
        if (hasUserInteractedRef.current) return;
        if (isTypingRef.current) return;
        if (messagesLenRef.current > 0) return;
        if (currentConversationIdRef.current) return;

        setCurrentConversationId(conversationId);
        const ui: Message[] = [];
        for (const m of msgs) {
          if (m.role !== "user" && m.role !== "assistant") continue;
          if (m.role === "assistant" && !m.content) continue;
          ui.push({
            id: m.id,
            role: m.role,
            content: m.content,
            timestamp: new Date(m.created_at),
          });
        }
        setMessages(ui);
      } catch {
        try {
          localStorage.removeItem(lastConversationStorageKey);
        } catch {
        }
      }
    })();
  }, [open, lastConversationStorageKey]);

  // Substitui a última bolha do assistente em vez de empilhar outra: evita a
  // bolha órfã ("Analisando...") ficar acima da mensagem de erro.
  // Guarda o texto que está na tela quando o stream cai, para comparar com a
  // resposta persistida e não substituir por igual.
  const ultimaBolhaRef = useRef<string | null>(null);
  // Texto do último envio, para a reconexão poder repetir sem depender do
  // closure do `finally`.
  const ultimoTextoEnviadoRef = useRef<string | null>(null);

  const substituirUltimaBolha = (content: string) => {
    ultimaBolhaRef.current = content;
    setMessages((prev) => {
      const last = prev[prev.length - 1];
      if (last && last.role === "assistant") {
        return [...prev.slice(0, -1), { ...last, content, streaming: false }];
      }
      return [...prev, { id: Date.now().toString(), role: "assistant", content, timestamp: new Date(), streaming: false }];
    });
  };

  // Busca a resposta que ficou no servidor quando o stream caiu.
  //
  // O backend continua processando depois que o cliente desconecta: a resposta
  // é gerada e persistida mesmo sem ninguém lendo. Então a recuperação certa não
  // é pedir para o usuário enviar de novo (o que refaz a pergunta e duplica a
  // bolha) — é buscar o que já foi produzido.
  //
  // Isso também cobre a troca de tela com o chat aberto, que é o caso comum: a
  // conexão cai na navegação, a resposta termina de ser gerada no servidor, e ao
  // voltar o texto já está lá.
  const recuperarRespostaPersistida = async (
    conversationId: string | null,
    opts?: { tentativas?: number; intervaloMs?: number },
  ): Promise<string | null> => {
    if (!conversationId) return null;
    const tentativas = opts?.tentativas ?? 4;
    const intervalo = opts?.intervaloMs ?? 1500;

    for (let i = 0; i < tentativas; i++) {
      if (i > 0) await new Promise((r) => setTimeout(r, intervalo));
      try {
        const msgs = (await listMessages(conversationId, 20)) as AIMessage[];
        // A decisão vive em `escolherRespostaRecuperada` (pura e testada): ela
        // distingue "o backend concluiu" de "só o preâmbulo existe e a síntese
        // ainda vem", o que a leitura ingênua da última mensagem não fazia.
        const texto = escolherRespostaRecuperada(msgs);
        if (texto) return texto;
      } catch {
        // Falha de rede na recuperação não é fatal: tenta de novo no próximo ciclo.
      }
    }
    return null;
  };

  // Caminho quando a resposta NÃO está no servidor: reconecta uma vez (o
  // backend é resiliente a reconexões; o problema é a queda do stream no meio).
  // A reconexão é silenciosa de propósito — ela costuma dar certo, e anunciar
  // uma falha que o usuário não vai precisar resolver só gera ruído. Se a
  // segunda tentativa também falhar, aí sim vem a mensagem.
  const continuarSemRespostaPersistida = async () => {
    if (!automaticRetryRef.current) {
      automaticRetryRef.current = true;
      console.warn("[Suitee Intelligence] stream caiu sem conclusão; reconectando uma vez");
      void sendMessage(ultimoTextoEnviadoRef.current ?? "", { retry: true });
      return;
    }
    automaticRetryRef.current = false;
    substituirUltimaBolha("A resposta não chegou — a conexão pode ter sido interrompida. Tente novamente.");
    setShowSuggestions(true);
  };

  const sendMessage = async (text: string, opts?: { retry?: boolean }) => {
    if (!text.trim()) return;
    if (sendingRef.current) return;
    sendingRef.current = true;
    if (!opts?.retry) automaticRetryRef.current = false;
    abortRef.current?.abort();
    gotAssistantMessageRef.current = false;
    watchdogAbortedRef.current = false;
    recebeuEventoRef.current = false;
    limiteAtingidoRef.current = false;
    conclusaoRecebidaRef.current = false;
    erroRecebidoRef.current = false;
    ultimaBolhaRef.current = null;
    ultimoTextoEnviadoRef.current = text;
    lastEventAtRef.current = Date.now();
    hasUserInteractedRef.current = true;
    setSlashOpen(false);
    setSlashActiveIndex(0);

    const userMessage: Message = {
      id: Date.now().toString(),
      role: "user",
      content: text,
      timestamp: new Date(),
    };

    setMessages((prev) => [...prev, userMessage]);
    setActiveTools([]);
    setInput("");
    setIsTyping(true);

    const controller = new AbortController();
    abortRef.current = controller;

    let watchdog: number | undefined;
    try {
      watchdog = window.setInterval(() => {
        const silencio = Date.now() - lastEventAtRef.current;
        // Silêncio só significa problema se NENHUM evento chegou.
        //
        // O modelo fica quieto enquanto infere — e com volume grande de dados
        // isso levava dezenas de segundos. O watchdog antigo (45s) abortava
        // nesse silêncio normal, o backend recebia o cancelamento e PARAVA de
        // gerar, e a resposta nunca era persistida. Era um watchdog que
        // transformava lentidão em resposta perdida.
        //
        // Com stream já iniciado, o silêncio é inferência em andamento: esperar
        // é o comportamento correto, e a recuperação da resposta persistida
        // cobre a queda real. O watchdog fica como rede para o caso de o stream
        // nunca ter produzido nada.
        const recebeuAlgo = gotAssistantMessageRef.current || recebeuEventoRef.current;
        if (silencio > WATCHDOG_SILENCIO_MS && !recebeuAlgo) {
          console.warn(`[Suitee Intelligence] nenhum evento em ${Math.round(silencio / 1000)}s; abortando`);
          watchdogAbortedRef.current = true;
          abortRef.current?.abort();
        }
      }, 5_000);
      await chatStream(
        currentConversationId,
        text,
        screenContext,
        (evt) => {
          lastEventAtRef.current = Date.now();
          recebeuEventoRef.current = true;
          // Debug: loga eventos SSE para diagnosticar falhas silenciosas
          if (import.meta.env.DEV) {
            const preview = evt.type === "message_delta"
              ? `[delta:${(evt.data.content || "").length}c]`
              : evt.type === "message" || evt.type === "error"
                ? `[content:${String((evt.data as any).content || (evt.data as any).message || "").slice(0, 80)}]`
                : evt.type === "tool_start"
                  ? `[tool:${evt.data.tool_name}]`
                  : evt.type === "tool_end"
                    ? `[tool_end:${evt.data.tool_call_id}]`
                    : "";
            console.debug(`[Suitee SSE] ${evt.type} ${preview}`);
          }
          if (evt.type === "message_start") {
            setCurrentConversationId(evt.data.conversation_id);
            setIsConnected(evt.data.mcp_connected !== false);
            setShowSuggestions(false);
            isToolingRef.current = false;
          }
          if (evt.type === "message_delta") {
            if (isToolingRef.current) return;
            const delta = evt.data.content || "";
            if (!delta) return;
            ultimaBolhaRef.current = (ultimaBolhaRef.current ?? "") + delta;
            gotAssistantMessageRef.current = true;
            setMessages((prev) => {
              const last = prev[prev.length - 1];
              if (last && last.role === "assistant" && last.streaming) {
                const updated: Message = { ...last, content: (last.content || "") + delta, streaming: true };
                return [...prev.slice(0, -1), updated];
              }
              return [
                ...prev,
                {
                  id: Date.now().toString(),
                  role: "assistant",
                  content: delta,
                  timestamp: new Date(),
                  streaming: true,
                },
              ];
            });
          }
          if (evt.type === "tool_start") {
            isToolingRef.current = true;
            setMessages((prev) => {
              const last = prev[prev.length - 1];
              if (last && last.role === "assistant") {
                return [...prev.slice(0, -1), { ...last, content: "Analisando...", streaming: true }];
              }
              return [
                ...prev,
                {
                  id: Date.now().toString(),
                  role: "assistant",
                  content: "Analisando...",
                  timestamp: new Date(),
                  streaming: true,
                },
              ];
            });
            setActiveTools((prev) => [
              ...prev,
              {
                tool_name: evt.data.tool_name,
                tool_call_id: evt.data.tool_call_id,
                status: "loading",
              },
            ]);
          }
          if (evt.type === "tool_end") {
            setActiveTools((prev) =>
              prev.map((t) =>
                t.tool_call_id === evt.data.tool_call_id ? { ...t, status: "done", result_preview: evt.data.result_preview } : t,
              ),
            );
          }
          if (evt.type === "message") {
            setIsTyping(false);
            gotAssistantMessageRef.current = true;
            isToolingRef.current = false;
            setExecutingActionId(null);
            setMessages((prev) => {
              const last = prev[prev.length - 1];
              if (last && last.role === "assistant" && last.streaming) {
                const updated: Message = { ...last, content: evt.data.content, streaming: false };
                return [...prev.slice(0, -1), updated];
              }
              return [
                ...prev,
                {
                  id: Date.now().toString(),
                  role: "assistant",
                  content: evt.data.content,
                  timestamp: new Date(),
                  streaming: false,
                },
              ];
            });
          }
          if (evt.type === "done") {
            conclusaoRecebidaRef.current = true;
            setIsTyping(false);
            setExecutingActionId(null);
            isToolingRef.current = false;
            setMessages((prev) => {
              const last = prev[prev.length - 1];
              if (last && last.role === "assistant" && last.streaming) {
                return [...prev.slice(0, -1), { ...last, streaming: false }];
              }
              return prev;
            });
            if (!gotAssistantMessageRef.current) {
              console.warn("[Suitee Intelligence] stream done sem mensagem renderizável", evt.data);
              gotAssistantMessageRef.current = true;
              setMessages((prev) => [
                ...prev,
                {
                  id: Date.now().toString(),
                  role: "assistant",
                  content: "Não recebi uma resposta renderizável desta solicitação. Tente novamente.",
                  timestamp: new Date(),
                },
              ]);
              setShowSuggestions(true);
            }
          }
          if (evt.type === "error") {
            erroRecebidoRef.current = true;
            conclusaoRecebidaRef.current = true;
            console.error("[Suitee Intelligence] SSE error", evt.data, JSON.stringify(evt.data || {}));
            setIsTyping(false);
            setExecutingActionId(null);
            isToolingRef.current = false;
            gotAssistantMessageRef.current = true;
            const msg = evt.data.message || "Erro ao gerar resposta";
            // Substitui o placeholder/streaming pela mensagem de erro em vez
            // de empilhar uma bolha nova (evita "Analisando..." preso acima).
            toast.error(msg === "Erro ao gerar resposta" ? msg : `Não foi possível processar: ${msg}`);
            setMessages((prev) => {
              const last = prev[prev.length - 1];
              if (last && last.role === "assistant" && (last.streaming || !last.content || last.content === "Analisando...")) {
                return [...prev.slice(0, -1), { ...last, content: msg, streaming: false }];
              }
              return [
                ...prev,
                {
                  id: Date.now().toString(),
                  role: "assistant",
                  content: msg,
                  timestamp: new Date(),
                  streaming: false,
                },
              ];
            });
            setShowSuggestions(true);
          }
        },
        controller.signal,
      );
    } catch (e: any) {
      if (e?.name === "AbortError" && watchdogAbortedRef.current) {
        // Watchdog: stream mudo por 45s. Nenhuma bolha — o finally decide
        // entre reconexão única e mensagem final.
        gotAssistantMessageRef.current = false;
      } else if (e?.name === "AbortError") {
        // Usuário cancelou (ou o painel fechou): remove o placeholder
        // "Analisando..." e informa a interrupção com clareza.
        gotAssistantMessageRef.current = true;
        setMessages((prev) => {
          const last = prev[prev.length - 1];
          if (last && last.role === "assistant" && (!last.content || last.content === "Analisando...")) {
            return [
              ...prev.slice(0, -1),
              {
                id: Date.now().toString(),
                role: "assistant",
                content: "Geração interrompida.",
                timestamp: new Date(),
                streaming: false,
              },
            ];
          }
          return prev.map((m, i) => (i === prev.length - 1 ? { ...m, streaming: false } : m));
        });
      } else if (e?.status === 429) {
        // Limite de uso do assistente (borda). Não é falha de conexão nem
        // resposta interrompida: a pergunta não chegou a ser processada, então
        // a bolha anterior não pode ganhar a marca de "resposta interrompida".
        console.warn("[Suitee Intelligence] teto de uso do assistente atingido", e);
        limiteAtingidoRef.current = true;
        gotAssistantMessageRef.current = true;
        toast.error("O assistente atingiu o limite de uso por agora. Aguarde alguns segundos e envie novamente.");
        setMessages((prev) => [
          ...prev,
          {
            id: Date.now().toString(),
            role: "assistant",
            content:
              "O assistente atingiu o limite de uso por agora. Aguarde alguns segundos e envie a mensagem novamente.",
            timestamp: new Date(),
          },
        ]);
      } else {
        const detail = typeof e?.message === "string" && e.message ? String(e.message).slice(0, 160) : "";
        console.error("[Suitee Intelligence] chat stream failed", e);
        toast.error("Erro ao conectar à Suitee Intelligence" + (detail ? `: ${detail}` : ""));
        gotAssistantMessageRef.current = true;
        setMessages((prev) => [
          ...prev,
          {
            id: Date.now().toString(),
            role: "assistant",
            content: "Não consegui concluir essa solicitação agora. Tente novamente.",
            timestamp: new Date(),
          },
        ]);
      }
      setIsConnected(false);
    } finally {
      if (watchdog !== undefined) window.clearInterval(watchdog);
      sendingRef.current = false;
      if (import.meta.env.DEV) {
        console.debug(`[Suitee SSE] stream ended — gotMessage=${gotAssistantMessageRef.current} typing=${isTyping}`);
      }
      setIsTyping(false);
      setExecutingActionId(null);
      setActiveTools([]);
      isToolingRef.current = false;
      setMessages((prev) => {
        const last = prev[prev.length - 1];
        if (last && last.role === "assistant" && last.streaming) {
          return [...prev.slice(0, -1), { ...last, streaming: false }];
        }
        return prev;
      });
      // O stream pode fechar de três formas, e só duas são conclusões. A
      // terceira — fechar sem `done` nem `error` — é a conexão caindo no meio, e
      // era justamente a que passava batido: o preâmbulo do modelo já marca a
      // resposta como "tem conteúdo", então o caminho antigo só disparava quando
      // NADA chegava, e o usuário ficava com uma resposta truncada (ou com a
      // bolha "Analisando..." parada) sem saber que algo falhou.
      const desfecho = limiteAtingidoRef.current
        ? "concluido"
        : decidirDesfechoDoStream({
            conclusaoRecebida: conclusaoRecebidaRef.current,
            erroRecebido: erroRecebidoRef.current,
            abortadoPorWatchdog: watchdogAbortedRef.current,
            temConteudo: gotAssistantMessageRef.current,
          });

      if (desfecho === "reconectar") {
        // Sem conteúdo nenhum. Antes de reenviar a pergunta, confere se o
        // servidor já concluiu: quando a queda é só do lado do cliente, a
        // resposta existe e reenviar duplicaria o pedido.
        void (async () => {
          const completa = await recuperarRespostaPersistida(currentConversationIdRef.current, {
            tentativas: 2,
            intervaloMs: 1200,
          });
          if (completa) {
            // O texto do assistente pode não existir na tela ainda (o stream caiu
            // antes do preâmbulo). Garante a bolha e encerra sem reenviar nada.
            substituirUltimaBolha(completa);
            setIsConnected(true);
            gotAssistantMessageRef.current = true;
            return;
          }
          await continuarSemRespostaPersistida();
        })();
      } else if (desfecho === "resposta-incompleta") {
        // Conteúdo parcial + conexão caída. Antes isto pedia "envie de novo",
        // o que refazia a pergunta e duplicava a bolha — e ignorava o fato de o
        // backend continuar gerando depois da desconexão.
        //
        // Agora busca a resposta que ficou no servidor. É o caso comum de trocar
        // de tela com o chat aberto: a conexão cai na navegação, a resposta
        // termina de ser gerada, e ao voltar o texto completo já está lá.
        automaticRetryRef.current = false;
        console.warn("[Suitee Intelligence] stream encerrado sem conclusão, com conteúdo parcial");

        const parcial = ultimaBolhaRef.current;
        void (async () => {
          const completa = await recuperarRespostaPersistida(currentConversationIdRef.current);
          if (completa && completa !== parcial) {
            // O backend concluiu: substitui o texto pela metade pela resposta
            // inteira, sem avisar o usuário de nada — para ele a resposta
            // simplesmente chegou.
            substituirUltimaBolha(completa);
            setIsConnected(true);
            return;
          }

          // O servidor não concluiu dentro da janela: aí sim o usuário precisa
          // saber, e o texto na tela parece completo demais para ficar assim.
          toast.error("A resposta foi interrompida no meio.");
          setMessages((prev) => {
            const last = prev[prev.length - 1];
            if (last && last.role === "assistant") {
              return [
                ...prev.slice(0, -1),
                {
                  ...last,
                  streaming: false,
                  content: `${last.content ?? ""}\n\n_(resposta interrompida — a conexão caiu no meio; envie de novo para completar)_`,
                },
              ];
            }
            return prev;
          });
          setShowSuggestions(true);
        })();
      }
    }
  };

  const executeAction = async (action: SuiteeAction, inputs: Record<string, any>) => {
    const actionId = action.action_id || action.id || "";
    if (!currentConversationId || !actionId) {
      hasUserInteractedRef.current = true;
      return sendMessage(action.confirm_message || "CONFIRMAR");
    }
    if (!action.inputs_schema) {
      hasUserInteractedRef.current = true;
      return sendMessage(action.confirm_message || "CONFIRMAR");
    }

    abortRef.current?.abort();
    gotAssistantMessageRef.current = false;
    // Ciclo proprio: nao herda as marcas do envio anterior.
    conclusaoRecebidaRef.current = false;
    erroRecebidoRef.current = false;
    hasUserInteractedRef.current = true;
    sendingRef.current = true;
    setExecutingActionId(actionId);
    setIsTyping(true);

    const controller = new AbortController();
    abortRef.current = controller;

    try {
      await confirmAction(
        currentConversationId,
        actionId,
        inputs || {},
        screenContext,
        (evt) => {
          if (evt.type === "message_start") {
            setCurrentConversationId(evt.data.conversation_id);
            setIsConnected(evt.data.mcp_connected !== false);
            isToolingRef.current = false;
          }
          if (evt.type === "message_delta") {
            if (isToolingRef.current) return;
            const delta = evt.data.content || "";
            if (!delta) return;
            gotAssistantMessageRef.current = true;
            setMessages((prev) => {
              const last = prev[prev.length - 1];
              if (last && last.role === "assistant" && last.streaming) {
                return [...prev.slice(0, -1), { ...last, content: (last.content || "") + delta, streaming: true }];
              }
              return [
                ...prev,
                { id: Date.now().toString(), role: "assistant", content: delta, timestamp: new Date(), streaming: true },
              ];
            });
          }
          if (evt.type === "tool_start") {
            isToolingRef.current = true;
            setMessages((prev) => {
              const last = prev[prev.length - 1];
              if (last && last.role === "assistant") {
                return [...prev.slice(0, -1), { ...last, content: "Analisando...", streaming: true }];
              }
              return [
                ...prev,
                { id: Date.now().toString(), role: "assistant", content: "Analisando...", timestamp: new Date(), streaming: true },
              ];
            });
            setActiveTools((prev) => [
              ...prev,
              { tool_name: evt.data.tool_name, tool_call_id: evt.data.tool_call_id, status: "loading" },
            ]);
          }
          if (evt.type === "tool_end") {
            setActiveTools((prev) =>
              prev.map((t) =>
                t.tool_call_id === evt.data.tool_call_id ? { ...t, status: "done", result_preview: evt.data.result_preview } : t,
              ),
            );
          }
          if (evt.type === "message") {
            setIsTyping(false);
            gotAssistantMessageRef.current = true;
            isToolingRef.current = false;
            setExecutingActionId(null);
            setMessages((prev) => {
              const last = prev[prev.length - 1];
              if (last && last.role === "assistant" && last.streaming) {
                return [...prev.slice(0, -1), { ...last, content: evt.data.content, streaming: false }];
              }
              return [
                ...prev,
                { id: Date.now().toString(), role: "assistant", content: evt.data.content, timestamp: new Date(), streaming: false },
              ];
            });
          }
          if (evt.type === "done") {
            conclusaoRecebidaRef.current = true;
            setIsTyping(false);
            setExecutingActionId(null);
            isToolingRef.current = false;
            setMessages((prev) => {
              const last = prev[prev.length - 1];
              if (last && last.role === "assistant" && last.streaming) {
                return [...prev.slice(0, -1), { ...last, streaming: false }];
              }
              return prev;
            });
          }
          if (evt.type === "error") {
            erroRecebidoRef.current = true;
            conclusaoRecebidaRef.current = true;
            console.error("[Suitee Intelligence] SSE error (action)", evt.data, JSON.stringify(evt.data || {}));
            setIsTyping(false);
            setExecutingActionId(null);
            isToolingRef.current = false;
            gotAssistantMessageRef.current = true;
            const msg = evt.data.message || "Erro ao executar ação";
            toast.error(msg);
            setMessages((prev) => {
              const last = prev[prev.length - 1];
              if (last && last.role === "assistant" && (last.streaming || !last.content || last.content === "Analisando...")) {
                return [...prev.slice(0, -1), { ...last, content: msg, streaming: false }];
              }
              return [
                ...prev,
                { id: Date.now().toString(), role: "assistant", content: msg, timestamp: new Date(), streaming: false },
              ];
            });
          }
        },
        controller.signal,
      );
    } catch (e: any) {
      if (e?.name === "AbortError") {
        setMessages((prev) => {
          const last = prev[prev.length - 1];
          if (last && last.role === "assistant" && (!last.content || last.content === "Analisando...")) {
            return [
              ...prev.slice(0, -1),
              {
                id: Date.now().toString(),
                role: "assistant",
                content: "Ação cancelada.",
                timestamp: new Date(),
                streaming: false,
              },
            ];
          }
          return prev.map((m, i) => (i === prev.length - 1 ? { ...m, streaming: false } : m));
        });
      } else {
        const detail = typeof e?.message === "string" && e.message ? String(e.message).slice(0, 160) : "";
        console.error("[Suitee Intelligence] action confirm failed", e);
        toast.error("Erro ao executar ação" + (detail ? `: ${detail}` : ""));
        setMessages((prev) => [
          ...prev,
          { id: Date.now().toString(), role: "assistant", content: "Não consegui executar essa ação agora. Tente novamente.", timestamp: new Date() },
        ]);
      }
      gotAssistantMessageRef.current = true;
    } finally {
      sendingRef.current = false;
      setIsTyping(false);
      setExecutingActionId(null);
      setActiveTools([]);
      isToolingRef.current = false;
      setMessages((prev) => {
        const last = prev[prev.length - 1];
        if (last && last.role === "assistant" && last.streaming) {
          return [...prev.slice(0, -1), { ...last, streaming: false }];
        }
        return prev;
      });
      // Fallback honesto só quando NENHUM evento chegou (sem mensagem de erro
      // sobreposta) — a fórmula anterior reportava "ferramentas indisponíveis"
      // para qualquer falha, inclusive rede/backend.
      if (!gotAssistantMessageRef.current) {
        setMessages((prev) => {
          const last = prev[prev.length - 1];
          if (last && last.role === "assistant") {
            return [
              ...prev.slice(0, -1),
              {
                id: Date.now().toString(),
                role: "assistant",
                content: "A ação não foi processada — a conexão pode ter sido interrompida. Tente novamente.",
                timestamp: new Date(),
              },
            ];
          }
          return prev;
        });
        setShowSuggestions(true);
      }
    }
  };

  const handleSend = async () => {
    return sendMessage(input);
  };

  const [isMounted, setIsMounted] = useState(false);

  useEffect(() => {
    setIsMounted(true);
  }, []);

  useEffect(() => {
    if (!open) abortRef.current?.abort();
  }, [open]);

  useEffect(() => {
    if (!open) return;
    inputRef.current?.focus();
  }, [open]);

  const handleNewConversation = useCallback(() => {
    abortRef.current?.abort();
    setCurrentConversationId(null);
    currentConversationIdRef.current = null;
    setActiveTools([]);
    setIsTyping(false);
    setMessages([]);
    setShowSuggestions(true);
    setInput("");
    setSlashOpen(false);
    setSlashActiveIndex(0);
    hasUserInteractedRef.current = false;
  }, []);

  const handleSlashSelect = useCallback(
    (cmd: SlashCommand) => {
      setSlashOpen(false);
      setSlashActiveIndex(0);
      setInput("");
      if (cmd.kind === "action" && cmd.action === "clear") {
        handleNewConversation();
        inputRef.current?.focus();
        return;
      }
      if (cmd.expand) {
        sendMessage(cmd.expand);
      }
    },
    [handleNewConversation, sendMessage],
  );

  const greetingName = useMemo(() => {
    const first = (userName || "").trim().split(/\s+/)[0];
    return first || userName;
  }, [userName]);

  return (
    <aside
      className={
        isPage
          ? "fixed inset-0 z-50"
          : `fixed bottom-0 right-0 top-16 z-50 flex w-full flex-col transition-transform duration-300 ease-[cubic-bezier(0.16,1,0.3,1)] md:w-[400px] ${
              isExpanded ? "md:w-[600px]" : ""
            } ${open ? "translate-x-0" : "translate-x-full"}`
      }
      role="complementary"
      aria-label="Suitee Intelligence"
      aria-hidden={!open}
      inert={!open}
      style={{ display: isMounted ? "block" : "none" }}
    >
      <div
        className={
          isPage
            ? "h-full w-full bg-card flex flex-col"
            : "flex h-full w-full flex-col border-l border-border/50 bg-card shadow-2xl md:shadow-none"
        }
        style={{
          visibility: open || isMounted === false ? "visible" : "hidden",
          transitionDelay: open || !isMounted ? "0ms" : "300ms",
          display: isMounted ? "flex" : "none",
        }}
      >
        <div className="flex h-16 shrink-0 items-center justify-between gap-2 border-b border-border/50 bg-background/50 px-3 py-2 backdrop-blur-sm sm:px-4">
          <div className="flex min-w-0 items-center gap-2.5">
            <SuiteeIntelligenceMark size="md" live pulse="hover" overlay />
            <div className="min-w-0">
              <h3 className="truncate text-sm font-semibold tracking-tight">Suitee Intelligence</h3>
              <p className="mt-0.5 flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-caption font-medium text-muted-foreground">
                <span className="relative flex h-1.5 w-1.5 shrink-0">
                  <span className="motion-safe:animate-pulse absolute inline-flex h-full w-full rounded-full bg-primary/60" />
                  <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-primary" />
                </span>
                <span aria-live="polite" className="truncate">
                  {isConnected === false
                    ? "Desconectada"
                    : isConnected === true
                      ? "Conectada"
                      : "Conectando…"}
                </span>
                {isConnected === true && availableToolNames.length > 0 && (
                  <span className="hidden whitespace-nowrap text-muted-foreground/70 sm:inline">
                    · {availableToolNames.length} ferramentas
                  </span>
                )}
                {isConnected === false && (
                  <button
                    type="button"
                    onClick={() => void reloadTools()}
                    title="Tentar conectar novamente"
                    aria-label="Tentar conectar novamente"
                    className="inline-flex items-center gap-1 rounded-full border border-status-warning/30 bg-status-warning-soft px-1.5 py-0.5 text-caption font-semibold text-status-warning hover:bg-status-warning-soft/70"
                  >
                    <RefreshCw className="h-2.5 w-2.5" />
                    Reconectar
                  </button>
                )}
              </p>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-0.5">
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 text-muted-foreground hover:bg-muted"
              onClick={() => {
                setHistoryOpen(true);
                refreshConversations();
              }}
              title="Histórico"
              aria-label="Histórico"
            >
              <Clock className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 text-muted-foreground hover:bg-muted"
              onClick={handleNewConversation}
              title="Nova conversa"
              aria-label="Nova conversa"
            >
              <Plus className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 text-muted-foreground hover:bg-muted"
              onClick={() => setIsExpanded(!isExpanded)}
              title={isExpanded ? "Reduzir painel" : "Expandir painel"}
              aria-label={isExpanded ? "Reduzir painel" : "Expandir painel"}
            >
              {isExpanded ? <Minimize2 className="h-4 w-4" /> : <Maximize2 className="h-4 w-4" />}
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 text-muted-foreground hover:bg-muted hover:text-foreground"
              onClick={onClose}
              title="Fechar inteligência"
              aria-label="Fechar inteligência"
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
        </div>

        <Sheet open={historyOpen} onOpenChange={setHistoryOpen}>
          <SheetContent side="right" className="w-full sm:max-w-[520px] p-0">
            <SheetHeader className="px-5 py-4 border-b border-border/60">
              <SheetTitle>Histórico</SheetTitle>
            </SheetHeader>
            <div className="flex items-center justify-between gap-2 px-5 pt-4">
              <Button
                type="button"
                variant="secondary"
                onClick={() => {
                  setHistoryOpen(false);
                  handleNewConversation();
                }}
              >
                Nova conversa
              </Button>
              <div className="flex items-center gap-2">
                <Button
                  type="button"
                  variant="destructive"
                  onClick={async () => {
                    setConfirmState({ open: true, mode: "clear" });
                  }}
                  disabled={historyLoading}
                >
                  Limpar histórico
                </Button>
                <Button type="button" variant="outline" onClick={refreshConversations} disabled={historyLoading}>
                  Atualizar
                </Button>
              </div>
            </div>
            <div className="flex-1 overflow-y-auto space-y-4 px-5 py-4">
              {historyLoading && <div className="text-sm text-muted-foreground">Carregando…</div>}
              {!historyLoading && conversations.length === 0 && <div className="text-sm text-muted-foreground">Nenhuma conversa encontrada.</div>}
              {!historyLoading && (
                <div className="space-y-4">
                  {["Hoje", "Ontem", "Últimos 7 dias", "Mais antigas"].map((bucket) => {
                    const now = new Date();
                    const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
                    const startOfYesterday = startOfToday - 24 * 60 * 60 * 1000;
                    const startOf7d = startOfToday - 7 * 24 * 60 * 60 * 1000;
                    const group = conversations.filter((c) => {
                      const t = new Date(c.updated_at).getTime();
                      if (Number.isNaN(t)) return bucket === "Mais antigas";
                      if (bucket === "Hoje") return t >= startOfToday;
                      if (bucket === "Ontem") return t >= startOfYesterday && t < startOfToday;
                      if (bucket === "Últimos 7 dias") return t >= startOf7d && t < startOfYesterday;
                      return t < startOf7d;
                    });
                    if (group.length === 0) return null;
                    return (
                      <div key={bucket} className="space-y-2">
                        <div className="text-caption font-semibold text-muted-foreground uppercase tracking-wide">{bucket}</div>
                        <div className="space-y-2">
                          {group.map((c) => {
                            const selected = c.id === currentConversationId;
                            const title = (c.title || "").trim() || `Conversa ${c.id.slice(0, 6)}`;
                            const preview = (conversationPreviews[c.id] || "").trim();
                            return (
                              <button
                                key={c.id}
                                type="button"
                                onClick={() => {
                                  setHistoryOpen(false);
                                  loadConversation(c.id);
                                }}
                                className={`w-full text-left rounded-2xl border px-4 py-3 transition-colors ${
                                  selected ? "border-primary/30 bg-primary/5" : "border-border/60 bg-background hover:bg-muted/30"
                                }`}
                              >
                                <div className="flex items-start justify-between gap-3">
                                  <div className="min-w-0">
                                    <div className="text-sm font-semibold truncate">{title}</div>
                                    {preview && <div className="text-xs text-muted-foreground mt-1 line-clamp-2">{preview}</div>}
                                    <div className="text-caption text-muted-foreground mt-1.5">Atualizado: {formatConversationTime(c.updated_at)}</div>
                                  </div>
                                  <Button
                                    type="button"
                                    size="icon"
                                    variant="ghost"
                                    className="h-8 w-8 text-muted-foreground hover:bg-muted shrink-0"
                                    onClick={(e) => {
                                      e.preventDefault();
                                      e.stopPropagation();
                                      setConfirmState({ open: true, mode: "delete", conversationId: c.id, title });
                                    }}
                                    title="Excluir"
                                  >
                                    <Trash2 className="h-4 w-4" />
                                  </Button>
                                </div>
                              </button>
                            );
                          })}
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </SheetContent>
        </Sheet>

        <AlertDialog
          open={confirmState.open}
          onOpenChange={(o) => {
            if (!o) setConfirmState({ open: false });
          }}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{confirmState.open && confirmState.mode === "clear" ? "Limpar histórico?" : "Excluir conversa?"}</AlertDialogTitle>
              <AlertDialogDescription>
                {confirmState.open && confirmState.mode === "clear"
                  ? "Isso remove todas as conversas do histórico desta conta. Você pode iniciar novas conversas depois."
                  : "Essa ação não pode ser desfeita."}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancelar</AlertDialogCancel>
              <AlertDialogAction
                className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                onClick={async () => {
                  if (!confirmState.open) return;
                  try {
                    if (confirmState.mode === "clear") {
                      await clearConversations();
                      setConversations([]);
                      setConversationPreviews({});
                      try {
                        localStorage.removeItem(historyCacheStorageKey);
                      } catch {
                      }
                      toast.success("Histórico limpo");
                      handleNewConversation();
                      await refreshConversations();
                    } else {
                      await deleteConversation(confirmState.conversationId);
                      setConversations((prev) => prev.filter((c) => c.id !== confirmState.conversationId));
                      setConversationPreviews((prev) => {
                        const next = { ...prev };
                        delete next[confirmState.conversationId];
                        return next;
                      });
                      toast.success("Conversa excluída");
                      if (currentConversationIdRef.current === confirmState.conversationId) {
                        handleNewConversation();
                      }
                      await refreshConversations();
                    }
                  } catch {
                    toast.error("Não foi possível concluir a ação");
                  } finally {
                    setConfirmState({ open: false });
                  }
                }}
              >
                Confirmar
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>

        <div className="flex-1 overflow-y-auto bg-gradient-to-b from-background to-muted/20">
          <div
            className="mx-auto w-full max-w-3xl space-y-6 p-4"
            role="log"
            aria-label="Conversa com a Suitee Intelligence"
            aria-live="polite"
            aria-busy={isTyping}
          >
          {messages.map((msg) => {
            const raw = msg.role === "assistant" ? getRenderableAssistantContent(msg.content, msg.streaming) : (msg.content ?? "");
            const actionsParsed = msg.role === "assistant" && !msg.streaming ? parseSuiteeActions(raw) : { cleaned: raw, actions: [] as SuiteeAction[] };
            const cardsParsed =
              msg.role === "assistant" && !msg.streaming
                ? parseSuiteeCards(actionsParsed.cleaned)
                : { cleaned: actionsParsed.cleaned, blocks: [] as SuiteeCardsBlock[] };
            if (msg.role === "assistant" && !msg.streaming && !cardsParsed.cleaned && actionsParsed.actions.length === 0 && cardsParsed.blocks.length === 0) return null;

            return (
              <div key={msg.id} className={`flex gap-3 sm:gap-4 ${msg.role === "user" ? "flex-row-reverse" : "flex-row"}`}>
                {msg.role === "user" ? (
                  <div className="shrink-0">
                    <UserAvatar
                      name={userName}
                      avatarUrl={user?.avatar_url}
                      className="h-8 w-8 shadow-sm"
                      fallbackClassName="bg-secondary text-secondary-foreground"
                    />
                  </div>
                ) : (
                  <SuiteeIntelligenceMark size="md" pulse="hover" overlay className="shadow-sm" />
                )}
                <div
                  className={`max-w-[92%] sm:max-w-[85%] text-sm leading-relaxed ${
                    msg.role === "user" ? "bg-primary text-primary-foreground rounded-2xl rounded-tr-sm px-5 py-3 shadow-sm" : "text-foreground pt-1"
                  }`}
                >
                  {msg.role === "assistant" ? (
                    <div className="space-y-3">
                      {cardsParsed.cleaned && (
                        <div className="flex items-end gap-2">
                          <div className="min-w-0 flex-1">
                            <MessageContent content={cardsParsed.cleaned} />
                          </div>
                          {msg.streaming && <span className="inline-block h-4 w-2 rounded-sm bg-primary/60 animate-pulse" />}
                        </div>
                      )}
                      {cardsParsed.blocks.map((b, idx) => (
                        <ResultCards
                          key={`${msg.id}_cards_${idx}`}
                          block={b}
                          onOpen={(path) => navigate(normalizeAiPath(path))}
                          onCopy={async (text) => {
                            try {
                              await navigator.clipboard.writeText(text);
                              toast.success("Copiado");
                            } catch {
                              toast.error("Não foi possível copiar");
                            }
                          }}
                          onAction={(prompt) => sendMessage(prompt)}
                        />
                      ))}
                      {actionsParsed.actions.map((a, idx) => (
                        <ActionCard
                          key={`${msg.id}_${idx}`}
                          action={a}
                          isLoading={executingActionId === (a.action_id || a.id || "")}
                          onPrimary={(action, inputs) => executeAction(action, inputs)}
                          onSecondary={(action) => {
                            setInput(action.prefill || "");
                            inputRef.current?.focus();
                          }}
                        />
                      ))}
                    </div>
                  ) : (
                    msg.content
                  )}
                </div>
              </div>
            );
          })}

          {messages.length === 0 && showSuggestions && !isTyping && (isLoadingSuggestions || suggestions.length > 0) && (
            <div className="mx-auto w-full max-w-lg rounded-2xl border border-border/60 bg-background/60 p-4 space-y-2 backdrop-blur-sm">
              <div className="flex items-center gap-2 text-base font-semibold tracking-tight">
                <SuiteeIntelligenceMark size="md" live />
                <div>Olá, {greetingName}. Como posso ajudar?</div>
              </div>
              <div className="text-sm text-muted-foreground">
                Você está em <span className="font-medium text-foreground">{contextLabel}</span>.
              </div>
              <div className="text-caption text-muted-foreground/70">Ctrl/⌘ K abrir · / comandos · Esc fechar</div>
              <div className="flex flex-wrap gap-2 pt-2">
                {isLoadingSuggestions && <div className="text-xs text-muted-foreground">Carregando…</div>}
                {!isLoadingSuggestions &&
                  suggestions.map((s) => (
                    <Button
                      key={s.label}
                      type="button"
                      size="sm"
                      variant="outline"
                      className="rounded-full gap-1.5"
                      onClick={() => sendMessage(s.prompt)}
                    >
                      {(() => {
                        const l = (s.label || "").toLowerCase();
                        const Icon =
                          l.includes("listar") || l.includes("mostrar")
                            ? List
                            : l.includes("resumir") || l.includes("analisar")
                              ? BarChart3
                              : l.includes("filtrar")
                                ? Filter
                                : l.includes("criar") || l.includes("novo")
                                  ? Plus
                                  : Sparkles;
                        return (
                          <>
                            <Icon className="h-3.5 w-3.5" />
                            <span>{s.label}</span>
                          </>
                        );
                      })()}
                    </Button>
                  ))}
              </div>
            </div>
          )}

          {/* Ferramentas do sistema: visíveis apenas no estado inicial da
              conversa — durante e após o uso, o histórico tem precedência e a
              seção não compete com o resultado. */}
          {messages.length === 0 && isConnected !== null && !isTyping && (
            <div className="rounded-2xl border border-border/60 bg-background/60 backdrop-blur-sm p-4 space-y-2">
              <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-2 min-w-0">
                  <Wrench className="h-4 w-4 text-primary shrink-0" />
                  <span className="text-xs font-bold text-foreground uppercase tracking-wide">Ferramentas do sistema</span>
                  {availableToolNames.length > 0 && (
                    <span className="text-caption text-muted-foreground tabular-nums">{availableToolNames.length}</span>
                  )}
                </div>
                {screenShortcutTools.length > 6 && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-6 gap-0.5 text-caption text-muted-foreground"
                    onClick={() => setShowAllTools((v) => !v)}
                  >
                    {showAllTools ? <ChevronUp className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />}
                    {showAllTools ? "Ver menos" : "Ver todas"}
                  </Button>
                )}
              </div>
              {isConnected === false ? (
                <div className="space-y-1">
                  <p className="text-xs text-status-warning font-medium">
                    O servidor de ferramentas (MCP) não está acessível agora. O chat funciona, mas ações com dados do
                    sistema estão indisponíveis.
                  </p>
                  {toolsError && <p className="text-caption text-muted-foreground break-words">{toolsError}</p>}
                </div>
              ) : screenShortcutTools.length === 0 ? (
                <p className="text-xs text-muted-foreground">
                  Nenhuma ferramenta para esta tela — peça em linguagem natural que o assistente resolve.
                </p>
              ) : (
                <div className="flex flex-wrap gap-2">
                  {(showAllTools ? screenShortcutTools : screenShortcutTools.slice(0, 6)).map((name) => (
                    <Button
                      key={name}
                      type="button"
                      size="sm"
                      variant="outline"
                      className="rounded-full gap-1.5 h-7 px-2.5 text-xs"
                      onClick={() => sendMessage(toolPrompt(name))}
                      title={`Usar ferramenta ${name}`}
                    >
                      <Wrench className="h-3 w-3 text-primary" />
                      <span>{toolLabel(name)}</span>
                    </Button>
                  ))}
                </div>
              )}
            </div>
          )}

          {isTyping && (
            <div className="flex gap-4">
              <SuiteeIntelligenceMark size="md" live pulse="always" />
              <div className="pt-2 text-sm space-y-2">
                {activeTools.length > 0 && (
                  <div className="flex flex-wrap gap-2 max-w-full overflow-hidden">
                    {activeTools.map((t) => (
                      <ToolCallBadge key={t.tool_call_id} toolName={t.tool_name} status={t.status} resultPreview={t.result_preview} />
                    ))}
                  </div>
                )}
                <div className="flex gap-1.5 items-center">
                  <span className="h-1.5 w-1.5 bg-primary/40 rounded-full animate-bounce" style={{ animationDelay: "0ms" }}></span>
                  <span className="h-1.5 w-1.5 bg-primary/40 rounded-full animate-bounce" style={{ animationDelay: "150ms" }}></span>
                  <span className="h-1.5 w-1.5 bg-primary/40 rounded-full animate-bounce" style={{ animationDelay: "300ms" }}></span>
                </div>
              </div>
            </div>
          )}
          <div ref={messagesEndRef} />
          </div>
        </div>

        <div
          className={`border-t border-border/50 p-4 bg-background shrink-0 ${isPage ? "" : "transition-opacity duration-700"} ${
            open ? "opacity-100 delay-300" : "opacity-0 delay-0"
          }`}
        >
          <form
            onSubmit={(e) => {
              e.preventDefault();
              if (slashOpen) return;
              handleSend();
            }}
            className="flex gap-2 relative max-w-3xl mx-auto"
          >
            <div className="relative flex-1">
              <SlashMenu open={slashOpen} query={slashQuery} commands={slashCommands} activeIndex={slashActiveIndex} onSelect={handleSlashSelect} />
              <Textarea
                ref={inputRef}
                rows={1}
                value={input}
                onChange={(e) => {
                  const v = e.target.value;
                  setInput(v);
                  const openNow = v.startsWith("/");
                  setSlashOpen(openNow);
                  if (openNow) setSlashActiveIndex(0);
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && !e.shiftKey && !slashOpen) {
                    e.preventDefault();
                    handleSend();
                    return;
                  }
                  if (!slashOpen) return;
                  if (e.key === "Escape") {
                    e.stopPropagation();
                    setSlashOpen(false);
                    return;
                  }
                  if (slashCommands.length === 0) return;
                  if (e.key === "ArrowDown") {
                    e.preventDefault();
                    setSlashActiveIndex((i) => (i + 1) % slashCommands.length);
                    return;
                  }
                  if (e.key === "ArrowUp") {
                    e.preventDefault();
                    setSlashActiveIndex((i) => (i - 1 + slashCommands.length) % slashCommands.length);
                    return;
                  }
                  if (e.key === "Enter") {
                    e.preventDefault();
                    const cmd = slashCommands[slashActiveIndex] ?? slashCommands[0];
                    if (cmd) handleSlashSelect(cmd);
                  }
                }}
                placeholder="Pergunte à Suitee Intelligence..."
                className="w-full rounded-3xl border border-border/50 bg-muted/30 pl-5 pr-12 py-3.5 text-sm outline-none transition-all focus:border-primary/50 focus:bg-background focus:ring-4 focus:ring-primary/10 shadow-sm placeholder:text-muted-foreground/70 max-h-[160px] overflow-y-auto"
              />
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    type={isTyping ? "button" : "submit"}
                    size="icon"
                    className={`absolute right-1.5 top-1.5 h-9 w-9 rounded-full shrink-0 transition-all ${
                      isTyping
                        ? "bg-destructive text-destructive-foreground shadow-md hover:scale-105"
                        : input.trim() && !slashOpen
                          ? "bg-primary text-primary-foreground shadow-md hover:scale-105"
                          : "bg-muted text-muted-foreground"
                    }`}
                    disabled={isTyping ? false : !input.trim() || slashOpen}
                    onClick={() => {
                      if (!isTyping) return;
                      abortRef.current?.abort();
                    }}
                    title={isTyping ? "Parar geração" : "Enviar"}
                    aria-label={isTyping ? "Parar geração" : "Enviar"}
                  >
                    {isTyping ? <Square className="h-4 w-4" /> : <Send className="h-4 w-4 ml-0.5" />}
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="top" sideOffset={6}>
                  {isTyping ? "Parar geração" : "Enviar"}
                </TooltipContent>
              </Tooltip>
            </div>
          </form>
          <div className="mt-2 text-center text-caption text-muted-foreground/60 dark:text-muted-foreground/80">Enter para enviar · Shift+Enter para nova linha</div>
          <div className="mt-3 text-center text-caption text-muted-foreground/70 flex items-center justify-center gap-1.5">
            <Wand2 className="w-3 h-3" />
            A inteligência pode cometer erros. Verifique informações importantes.
          </div>
        </div>
      </div>
    </aside>
  );
}

