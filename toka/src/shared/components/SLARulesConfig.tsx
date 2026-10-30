import { useState, useEffect } from "react";
import { motion, AnimatePresence } from "motion/react";
import {
  Clock,
  RefreshCw,
  Filter,
  Zap,
  AlertCircle,
  MessageSquare,
  Smartphone,
  Mail,
  Bell,
  Plus,
  Trash2,
  Save,
  RotateCcw,
  ChevronDown,
  ChevronUp,
  TrendingUp,
  Timer,
  ArrowRightLeft,
  Pause,
  AlertTriangle,
  X,
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { leadService, SLATier, EscalationStep, NotifChannel } from "../services/lead.service";
import { Button } from "../ui/button";

type DistMethod = "sequential" | "load_balance" | "weighted" | "priority";

const DEFAULT_SLA_TIERS: SLATier[] = [
  {
    id: "sla-standard",
    name: "Padrão",
    color: "primary",
    timeLimitMin: 5,
    maxRetries: 2,
    actions: {
      redirectNext: true,
      autoPauseBroker: true,
      notifyManager: false,
      escalateToSupervisor: false,
      markAsUrgent: false,
    },
    appliesTo: ["Locação", "Medio Padrão"],
  },
  {
    id: "sla-priority",
    name: "Prioritario",
    color: "amber",
    timeLimitMin: 3,
    maxRetries: 1,
    actions: {
      redirectNext: true,
      autoPauseBroker: true,
      notifyManager: true,
      escalateToSupervisor: false,
      markAsUrgent: true,
    },
    appliesTo: ["Alto Padrão", "Investimento"],
  },
  {
    id: "sla-vip",
    name: "VIP / Urgente",
    color: "rose",
    timeLimitMin: 1,
    maxRetries: 1,
    actions: {
      redirectNext: true,
      autoPauseBroker: true,
      notifyManager: true,
      escalateToSupervisor: true,
      markAsUrgent: true,
    },
    appliesTo: ["VIP", "Indicacao Diretoria"],
  },
];

const DEFAULT_ESCALATION: EscalationStep[] = [
  { id: "esc-1", afterMinutes: 5, action: "notify_manager", label: "Notificar gerente via push" },
  { id: "esc-2", afterMinutes: 10, action: "redirect_team", label: "Redirecionar para equipe de plantao" },
  { id: "esc-3", afterMinutes: 15, action: "notify_whatsapp", label: "Alerta WhatsApp para supervisao" },
  { id: "esc-4", afterMinutes: 30, action: "auto_assign_supervisor", label: "Atribuir automaticamente ao supervisor" },
];

const DEFAULT_NOTIF: NotifChannel[] = [
  { id: "ch-1", name: "WhatsApp", icon: "whatsapp", enabled: true, description: "Notificação imediata via WhatsApp Business API" },
  { id: "ch-2", name: "Push App", icon: "push", enabled: true, description: "Notificação push no aplicativo mobile do corretor" },
  { id: "ch-3", name: "E-mail", icon: "email", enabled: false, description: "E-mail de notificação (pode ter atraso de 1-2 min)" },
  { id: "ch-4", name: "SMS", icon: "sms", enabled: false, description: "SMS direto para o celular do corretor" },
];

const DIST_METHODS: { value: DistMethod; label: string; desc: string }[] = [
  {
    value: "sequential",
    label: "Distribuição Sequencial (Roleta)",
    desc: "Distribui 1 a 1 de forma justa para o próximo corretor da fila. Pula corretores offline/em pausa.",
  },
  {
    value: "load_balance",
    label: "Balanceamento de Carga",
    desc: "Prioriza corretores com menos leads ativos no momento. Evita sobrecarga.",
  },
  {
    value: "weighted",
    label: "Peso Ponderado",
    desc: "Distribui proporcionalmente ao peso/performance configurado por corretor. Corretores com maior taxa de conversao recebem mais leads.",
  },
  {
    value: "priority",
    label: "Prioridade por Especialidade",
    desc: "Leads são enviados primeiro para especialistas na categoria. Se não houver disponível, vai para a roleta geral.",
  },
];

const COLOR_MAP: Record<string, { bg: string; text: string; border: string; badge: string }> = {
  primary: { bg: "bg-primary-50", text: "text-primary-700", border: "border-primary-200", badge: "bg-primary-100" },
  amber: { bg: "bg-status-warning-soft", text: "text-status-warning", border: "border-status-warning/30", badge: "bg-status-warning-soft" },
  rose: { bg: "bg-status-critical-soft", text: "text-status-critical", border: "border-status-critical/30", badge: "bg-status-critical-soft" },
  emerald: { bg: "bg-status-ok-soft", text: "text-status-ok", border: "border-status-ok/30", badge: "bg-status-ok-soft" },
  sky: { bg: "bg-sky-50", text: "text-sky-700", border: "border-sky-200", badge: "bg-sky-100" },
};

const NOTIF_ICONS: Record<string, React.ComponentType<any>> = {
  whatsapp: MessageSquare,
  push: Smartphone,
  email: Mail,
  sms: Bell,
};

let tierCounter = 4;
const genTierId = () => `sla-custom-${tierCounter++}`;
let escCounter = 5;
const genEscId = () => `esc-${escCounter++}`;

const normalizeSlaTiers = (input: any): SLATier[] => {
  if (!Array.isArray(input)) return DEFAULT_SLA_TIERS;
  const out = input
    .map((t: any): SLATier | null => {
      if (!t) return null;
      const rawActions = t.actions ?? t.Actions;
      return {
        id: String(t.id ?? t.ID ?? ""),
        name: String(t.name ?? t.Name ?? ""),
        color: String(t.color ?? t.Color ?? "primary"),
        timeLimitMin: Number(t.timeLimitMin ?? t.TimeLimitMin ?? 5),
        maxRetries: Number(t.maxRetries ?? t.MaxRetries ?? 1),
        actions: {
          redirectNext: !!(rawActions?.redirectNext ?? rawActions?.RedirectNext),
          autoPauseBroker: !!(rawActions?.autoPauseBroker ?? rawActions?.AutoPauseBroker),
          notifyManager: !!(rawActions?.notifyManager ?? rawActions?.NotifyManager),
          escalateToSupervisor: !!(rawActions?.escalateToSupervisor ?? rawActions?.EscalateToSupervisor),
          markAsUrgent: !!(rawActions?.markAsUrgent ?? rawActions?.MarkAsUrgent),
        },
        appliesTo: Array.isArray(t.appliesTo ?? t.AppliesTo) ? (t.appliesTo ?? t.AppliesTo) : [],
      };
    })
    .filter(Boolean) as SLATier[];
  return out.length > 0 ? out : DEFAULT_SLA_TIERS;
};

export function SLARulesConfig() {
  const [distMethod, setDistMethod] = useState<DistMethod>("sequential");
  const [respectSpecialty, setRespectSpecialty] = useState(true);
  const [respectRegion, setRespectRegion] = useState(true);
  const [respectSchedule, setRespectSchedule] = useState(false);
  const [autoCapLimit, setAutoCapLimit] = useState(true);
  const [updatedBy, setUpdatedBy] = useState<string>("");
  const [updatedAt, setUpdatedAt] = useState<string>("");
  const [initialSnapshot, setInitialSnapshot] = useState<any | null>(null);

  const [slaTiers, setSlaTiers] = useState<SLATier[]>(DEFAULT_SLA_TIERS);
  const [expandedTier, setExpandedTier] = useState<string | null>(null);

  const [escalation, setEscalation] = useState<EscalationStep[]>(DEFAULT_ESCALATION);
  const [showEscalation, setShowEscalation] = useState(true);

  const [notifChannels, setNotifChannels] = useState<NotifChannel[]>(DEFAULT_NOTIF);

  const [dirty, setDirty] = useState(false);
  const [showAddTier, setShowAddTier] = useState(false);
  const [newTierName, setNewTierName] = useState("");
  const [newTierColor, setNewTierColor] = useState("emerald");

  const markDirty = () => setDirty(true);

  useEffect(() => {
    loadConfig();
  }, []);

  const applySnapshot = (snap: any) => {
    setDistMethod((snap?.distMethod as DistMethod) ?? "sequential");
    setRespectSpecialty(!!snap?.respectSpecialty);
    setRespectRegion(!!snap?.respectRegion);
    setRespectSchedule(!!snap?.respectSchedule);
    setAutoCapLimit(!!snap?.autoCapLimit);
    setSlaTiers(normalizeSlaTiers(snap?.slaTiers));
    setEscalation(Array.isArray(snap?.escalation) && snap.escalation.length > 0 ? snap.escalation : DEFAULT_ESCALATION);
    setNotifChannels(Array.isArray(snap?.notifChannels) && snap.notifChannels.length > 0 ? snap.notifChannels : DEFAULT_NOTIF);
  };

  const loadConfig = async () => {
    try {
      const config = await leadService.getRouletteConfig();
      if (config) {
        applySnapshot(config);
        setUpdatedBy(String((config as any).updatedBy || ""));
        setUpdatedAt(String((config as any).updated_at || ""));
        setInitialSnapshot(config);
        setDirty(false);
      }
    } catch (error) {
      console.error("Error loading SLA config:", error);
      toast.error("Erro ao carregar configurações");
    }
  };

  const addTier = () => {
    if (!newTierName.trim()) {
      toast.error("Informe o nome do nível SLA");
      return;
    }
    const tier: SLATier = {
      id: genTierId(),
      name: newTierName.trim(),
      color: newTierColor,
      timeLimitMin: 5,
      maxRetries: 2,
      actions: {
        redirectNext: true,
        autoPauseBroker: false,
        notifyManager: false,
        escalateToSupervisor: false,
        markAsUrgent: false,
      },
      appliesTo: [],
    };
    setSlaTiers((prev) => [...prev, tier]);
    setNewTierName("");
    setShowAddTier(false);
    setExpandedTier(tier.id);
    markDirty();
    toast.success(`Nível SLA "${tier.name}" criado`);
  };

  const removeTier = (id: string) => {
    const removed = slaTiers.find((t) => t.id === id);
    setSlaTiers((prev) => prev.filter((t) => t.id !== id));
    markDirty();
    if (removed) toast.success(`Nível "${removed.name}" removido`);
  };

  const updateTier = (id: string, patch: Partial<SLATier>) => {
    setSlaTiers((prev) => prev.map((t) => (t.id === id ? { ...t, ...patch } : t)));
    markDirty();
  };

  const updateTierAction = (id: string, actionKey: keyof SLATier["actions"], val: boolean) => {
    setSlaTiers((prev) => prev.map((t) => (t.id === id ? { ...t, actions: { ...t.actions, [actionKey]: val } } : t)));
    markDirty();
  };

  const addEscalationStep = () => {
    const lastMin = escalation.length > 0 ? escalation[escalation.length - 1].afterMinutes : 0;
    setEscalation((prev) => [
      ...prev,
      {
        id: genEscId(),
        afterMinutes: lastMin + 10,
        action: "notify_manager",
        label: "Nova ação de escalação",
      },
    ]);
    markDirty();
  };

  const removeEscalationStep = (id: string) => {
    setEscalation((prev) => prev.filter((e) => e.id !== id));
    markDirty();
  };

  const updateEscalation = (id: string, patch: Partial<EscalationStep>) => {
    setEscalation((prev) => prev.map((e) => (e.id === id ? { ...e, ...patch } : e)));
    markDirty();
  };

  const toggleNotif = (id: string) => {
    setNotifChannels((prev) => prev.map((n) => (n.id === id ? { ...n, enabled: !n.enabled } : n)));
    markDirty();
  };

  const [tagInput, setTagInput] = useState<Record<string, string>>({});
  const addTag = (tierId: string) => {
    const val = (tagInput[tierId] || "").trim();
    if (!val) return;
    setSlaTiers((prev) =>
      prev.map((t) =>
        t.id === tierId && !t.appliesTo.includes(val) ? { ...t, appliesTo: [...t.appliesTo, val] } : t,
      ),
    );
    setTagInput((prev) => ({ ...prev, [tierId]: "" }));
    markDirty();
  };

  const removeTag = (tierId: string, tag: string) => {
    setSlaTiers((prev) => prev.map((t) => (t.id === tierId ? { ...t, appliesTo: t.appliesTo.filter((a) => a !== tag) } : t)));
    markDirty();
  };

  const handleSave = async () => {
    try {
      await leadService.updateRouletteConfig({
        distMethod,
        respectSpecialty,
        respectRegion,
        respectSchedule,
        autoCapLimit,
        slaTiers,
        escalation,
        notifChannels,
      });
      setDirty(false);
      setInitialSnapshot({
        distMethod,
        respectSpecialty,
        respectRegion,
        respectSchedule,
        autoCapLimit,
        slaTiers,
        escalation,
        notifChannels,
      });
    } catch (error) {
      console.error("Error saving SLA config:", error);
      toast.error("Erro ao salvar configurações");
    }
  };

  const handleReset = () => {
    if (initialSnapshot) {
      applySnapshot(initialSnapshot);
      setDirty(false);
    } else {
      setDistMethod("sequential");
      setRespectSpecialty(true);
      setRespectRegion(true);
      setRespectSchedule(false);
      setAutoCapLimit(true);
      setSlaTiers(DEFAULT_SLA_TIERS);
      setEscalation(DEFAULT_ESCALATION);
      setNotifChannels(DEFAULT_NOTIF);
      setDirty(false);
    }
    setExpandedTier(null);
    toast.info("Configurações restauradas para o padrão (não salvo)");
  };

  return (
    <div className="space-y-6 max-w-5xl">
      <div className="bg-card rounded-2xl border border-border shadow-sm p-6">
        <h3 className="text-lg font-bold text-foreground mb-1 flex items-center gap-2">
          <RefreshCw className="w-5 h-5 text-primary-600" /> Método de Distribuição
        </h3>
        <p className="text-sm text-muted-foreground mb-5">Escolha como os leads serão distribuídos entre os corretores da roleta.</p>
        {(updatedBy || updatedAt) && (
          <div className="text-xs text-muted-foreground mb-5">
            Última alteração{updatedBy ? ` por ${updatedBy}` : ""}{updatedAt ? ` em ${new Date(updatedAt).toLocaleString()}` : ""}
          </div>
        )}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {DIST_METHODS.map((dm) => (
            <label
              key={dm.value}
              className={clsx(
                "flex items-start gap-4 p-4 border rounded-xl cursor-pointer transition-colors",
                distMethod === dm.value
                  ? "border-primary-600 bg-primary-50/50 ring-1 ring-primary-600/20"
                  : "border-border hover:border-primary-300",
              )}
              onClick={() => {
                setDistMethod(dm.value);
                markDirty();
              }}
            >
              <input
                type="radio"
                name="dist_method"
                checked={distMethod === dm.value}
                onChange={() => {
                  setDistMethod(dm.value);
                  markDirty();
                }}
                className="mt-1 text-primary-600 focus:ring-primary-600 border-border"
              />
              <div>
                <span className="block text-sm font-bold text-foreground">{dm.label}</span>
                <span className="block text-xs text-muted-foreground mt-1">{dm.desc}</span>
              </div>
            </label>
          ))}
        </div>
      </div>

      <div className="bg-card rounded-2xl border border-border shadow-sm p-6">
        <h3 className="text-lg font-bold text-foreground mb-1 flex items-center gap-2">
          <Filter className="w-5 h-5 text-primary-600" /> Filtros e Especialidades
        </h3>
        <p className="text-sm text-muted-foreground mb-5">Configure como os filtros de especialidade e regiao afetam a distribuição.</p>
        <div className="space-y-3">
          {[
            {
              checked: respectSpecialty,
              onChange: (v: boolean) => {
                setRespectSpecialty(v);
                markDirty();
              },
              label: "Respeitar especialidade do corretor",
              desc: 'Lead de "Locação" vai apenas para corretores marcados com "Locação"',
            },
            {
              checked: respectRegion,
              onChange: (v: boolean) => {
                setRespectRegion(v);
                markDirty();
              },
              label: "Respeitar regiao de atuacao",
              desc: "Cruza a regiao de interesse do lead com a regiao do corretor",
            },
            {
              checked: respectSchedule,
              onChange: (v: boolean) => {
                setRespectSchedule(v);
                markDirty();
              },
              label: "Respeitar horário de expediente do corretor",
              desc: "Não distribui fora do horário do corretor (fallback: horário da imobiliária em Configurações)",
            },
            {
              checked: autoCapLimit,
              onChange: (v: boolean) => {
                setAutoCapLimit(v);
                markDirty();
              },
              label: "Limite automático de carga diária",
              desc: "Pausa automaticamente corretores que atingirem o limite maximo de leads/dia",
            },
          ].map((item, i) => (
            <label
              key={i}
              className={clsx(
                "flex items-start gap-3 p-4 border rounded-xl cursor-pointer transition-colors",
                item.checked ? "bg-primary-50/30 border-primary-200" : "bg-muted border-border",
              )}
            >
              <input
                type="checkbox"
                checked={item.checked}
                onChange={(e) => item.onChange(e.target.checked)}
                className="mt-0.5 text-primary-600 rounded border-border focus:ring-primary-600"
              />
              <div>
                <span className="block text-sm font-bold text-foreground">{item.label}</span>
                <span className="block text-xs text-muted-foreground mt-0.5">{item.desc}</span>
              </div>
            </label>
          ))}
        </div>
      </div>

      <div className="bg-card rounded-2xl border border-border shadow-sm p-6">
        <div className="flex items-center justify-between mb-1">
          <h3 className="text-lg font-bold text-foreground flex items-center gap-2">
            <Clock className="w-5 h-5 text-primary-600" /> Niveis de SLA
          </h3>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setShowAddTier(true)}
            className="text-sm font-bold text-primary hover:text-primary"
          >
            <Plus className="w-4 h-4" /> Novo Nível
          </Button>
        </div>
        <p className="text-sm text-muted-foreground mb-5">
          Configure tempos limite, ações pós-expiração e critérios de aplicação para cada nível de urgência.
        </p>

        <div className="space-y-4">
          {slaTiers.map((tier) => {
            const colors = COLOR_MAP[tier.color] || COLOR_MAP.primary;
            const isExpanded = expandedTier === tier.id;

            return (
              <motion.div
                key={tier.id}
                layout
                className={clsx("border rounded-2xl overflow-hidden transition-colors", isExpanded ? `${colors.border}` : "border-border")}
              >
                <div
                  className={clsx("flex items-center gap-4 px-5 py-4 cursor-pointer transition-colors", isExpanded ? colors.bg : "bg-card hover:bg-muted")}
                  onClick={() => setExpandedTier(isExpanded ? null : tier.id)}
                >
                  <div className={clsx("w-10 h-10 rounded-xl flex items-center justify-center", colors.badge)}>
                    <Timer className={clsx("w-5 h-5", colors.text)} />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-bold text-foreground">{tier.name}</span>
                      <span className={clsx("px-2 py-0.5 text-micro font-bold rounded-full", colors.badge, colors.text)}>
                        {tier.timeLimitMin}min
                      </span>
                      {tier.actions.markAsUrgent && (
                        <span className="px-1.5 py-0.5 text-caption font-bold text-status-critical bg-status-critical-soft rounded border border-status-critical/30">
                          URGENTE
                        </span>
                      )}
                    </div>
                    <div className="flex flex-wrap gap-1 mt-1">
                      {tier.appliesTo.map((tag) => (
                        <span key={tag} className="text-micro bg-muted text-muted-foreground px-1.5 py-0.5 rounded font-medium">
                          {tag}
                        </span>
                      ))}
                      {tier.appliesTo.length === 0 && <span className="text-micro text-muted-foreground italic">Nenhum criterio definido</span>}
                    </div>
                  </div>
                  <div className="flex items-center gap-2 flex-shrink-0">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={(e) => {
                        e.stopPropagation();
                        removeTier(tier.id);
                      }}
                      className="size-8 text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft"
                    >
                      <Trash2 className="w-4 h-4" />
                    </Button>
                    {isExpanded ? <ChevronUp className="w-4 h-4 text-muted-foreground" /> : <ChevronDown className="w-4 h-4 text-muted-foreground" />}
                  </div>
                </div>

                <AnimatePresence>
                  {isExpanded && (
                    <motion.div
                      initial={{ height: 0, opacity: 0 }}
                      animate={{ height: "auto", opacity: 1 }}
                      exit={{ height: 0, opacity: 0 }}
                      transition={{ duration: 0.2 }}
                      className="overflow-hidden"
                    >
                      <div className="px-5 pb-5 pt-3 border-t border-border bg-muted/40 space-y-5">
                        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                          <div>
                            <label className="block text-xs font-bold text-foreground mb-1.5">Tempo Limite (min)</label>
                            <input
                              type="number"
                              min={1}
                              max={60}
                              value={tier.timeLimitMin}
                              onChange={(e) => updateTier(tier.id, { timeLimitMin: Math.max(1, parseInt(e.target.value) || 1) })}
                              className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
                            />
                            <p className="text-micro text-muted-foreground mt-1">Corretor tem {tier.timeLimitMin} min para aceitar o lead</p>
                          </div>
                          <div>
                            <label className="block text-xs font-bold text-foreground mb-1.5">Tentativas Max</label>
                            <input
                              type="number"
                              min={1}
                              max={10}
                              value={tier.maxRetries}
                              onChange={(e) => updateTier(tier.id, { maxRetries: Math.max(1, parseInt(e.target.value) || 1) })}
                              className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
                            />
                            <p className="text-micro text-muted-foreground mt-1">Quantos corretores tentar antes de escalar</p>
                          </div>
                          <div>
                            <label className="block text-xs font-bold text-foreground mb-1.5">Nome do Nível</label>
                            <input
                              type="text"
                              value={tier.name}
                              onChange={(e) => updateTier(tier.id, { name: e.target.value })}
                              className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
                            />
                          </div>
                        </div>

                        <div>
                          <label className="text-xs font-bold text-foreground mb-2 flex items-center gap-1.5">
                            <AlertTriangle className="w-3.5 h-3.5 text-status-warning" /> Ações Pós-SLA Expirado
                          </label>
                          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                            {[
                              { key: "redirectNext" as const, label: "Redirecionar para próximo da fila", icon: ArrowRightLeft },
                              { key: "autoPauseBroker" as const, label: "Pausar corretor automaticamente", icon: Pause },
                              { key: "notifyManager" as const, label: "Notificar gerente/supervisor", icon: Bell },
                              { key: "escalateToSupervisor" as const, label: "Escalar para supervisor", icon: TrendingUp },
                              { key: "markAsUrgent" as const, label: "Marcar lead como urgente", icon: AlertCircle },
                            ].map((act) => {
                              const ActIcon = act.icon;
                              return (
                                <label
                                  key={act.key}
                                  className={clsx(
                                    "flex items-center gap-2.5 p-3 border rounded-xl cursor-pointer transition-colors",
                                    tier.actions[act.key] ? "bg-status-warning-soft/50 border-status-warning/30" : "bg-card border-border hover:border-border",
                                  )}
                                >
                                  <input
                                    type="checkbox"
                                    checked={tier.actions[act.key]}
                                    onChange={(e) => updateTierAction(tier.id, act.key, e.target.checked)}
                                    className="rounded border-border text-status-warning focus:ring-amber-600"
                                  />
                                  <ActIcon
                                    className={clsx(
                                      "w-3.5 h-3.5 flex-shrink-0",
                                      tier.actions[act.key] ? "text-status-warning" : "text-muted-foreground",
                                    )}
                                  />
                                  <span className="text-xs font-bold text-foreground">{act.label}</span>
                                </label>
                              );
                            })}
                          </div>
                        </div>

                        <div>
                          <label className="block text-xs font-bold text-foreground mb-2">Criterios de Aplicacao (tags)</label>
                          <p className="text-micro text-muted-foreground mb-2">
                            Adicione tags que definem quando este nível SLA se aplica (Ex: "Alto Padrão", "VIP", "Locação").
                          </p>
                          <div className="flex flex-wrap gap-2 mb-2">
                            {tier.appliesTo.map((tag) => (
                              <span
                                key={tag}
                                className={clsx("flex items-center gap-1 px-2.5 py-1 text-xs font-bold rounded-lg", colors.badge, colors.text)}
                              >
                                {tag}
                                <button type="button" onClick={() => removeTag(tier.id, tag)} className="hover:opacity-70">
                                  <X className="w-3 h-3" />
                                </button>
                              </span>
                            ))}
                          </div>
                          <div className="flex items-center gap-2">
                            <input
                              type="text"
                              value={tagInput[tier.id] || ""}
                              onChange={(e) => setTagInput((prev) => ({ ...prev, [tier.id]: e.target.value }))}
                              onKeyDown={(e) => (e.key === "Enter" ? addTag(tier.id) : undefined)}
                              placeholder="Adicionar tag..."
                              className="flex-1 px-3 py-1.5 bg-card border border-border rounded-lg text-xs focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
                            />
                            <Button variant="ghost" size="icon" onClick={() => addTag(tier.id)} className="size-8 text-primary-600 hover:bg-primary-50">
                              <Plus className="w-4 h-4" />
                            </Button>
                          </div>
                        </div>
                      </div>
                    </motion.div>
                  )}
                </AnimatePresence>
              </motion.div>
            );
          })}

          <AnimatePresence>
            {showAddTier ? (
              <motion.div
                initial={{ opacity: 0, y: 8 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -8 }}
                className="border-2 border-dashed border-primary-200 rounded-2xl p-5 bg-primary-50/30"
              >
                <h4 className="text-sm font-bold text-foreground mb-3 flex items-center gap-2">
                  <Plus className="w-4 h-4 text-primary-600" /> Novo Nível SLA
                </h4>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-4">
                  <div>
                    <label className="block text-xs font-bold text-foreground mb-1.5">Nome</label>
                    <input
                      type="text"
                      value={newTierName}
                      onChange={(e) => setNewTierName(e.target.value)}
                      placeholder="Ex: Corporativo"
                      className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
                      autoFocus
                      onKeyDown={(e) => (e.key === "Enter" ? addTier() : undefined)}
                    />
                  </div>
                  <div>
                    <label className="block text-xs font-bold text-foreground mb-1.5">Cor</label>
                    <div className="flex items-center gap-2">
                      {Object.keys(COLOR_MAP).map((c) => (
                        <button
                          type="button"
                          key={c}
                          onClick={() => setNewTierColor(c)}
                          className={clsx(
                            "w-8 h-8 rounded-lg border-2 transition-all",
                            COLOR_MAP[c].badge,
                            newTierColor === c ? `${COLOR_MAP[c].border}` : "border-transparent",
                          )}
                        />
                      ))}
                    </div>
                  </div>
                </div>
                <div className="flex items-center justify-end gap-2">
                  <Button
                    variant="outline"
                    onClick={() => {
                      setShowAddTier(false);
                      setNewTierName("");
                    }}
                  >
                    Cancelar
                  </Button>
                  <Button onClick={addTier}>
                    <Plus className="w-4 h-4" /> Criar Nível
                  </Button>
                </div>
              </motion.div>
            ) : null}
          </AnimatePresence>
        </div>
      </div>

      <div className="bg-card rounded-2xl border border-border shadow-sm p-6">
        <div className="flex items-center justify-between cursor-pointer" onClick={() => setShowEscalation(!showEscalation)}>
          <h3 className="text-lg font-bold text-foreground flex items-center gap-2">
            <TrendingUp className="w-5 h-5 text-primary-600" /> Cadeia de Escalação
          </h3>
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground font-medium">{escalation.length} passos</span>
            {showEscalation ? <ChevronUp className="w-4 h-4 text-muted-foreground" /> : <ChevronDown className="w-4 h-4 text-muted-foreground" />}
          </div>
        </div>
        <p className="text-sm text-muted-foreground mt-1 mb-4">
          Defina ações automáticas em cadeia quando nenhum corretor aceitar o lead após múltiplas tentativas.
        </p>

        <AnimatePresence>
          {showEscalation && (
            <motion.div initial={{ height: 0, opacity: 0 }} animate={{ height: "auto", opacity: 1 }} exit={{ height: 0, opacity: 0 }} className="overflow-hidden">
              <div className="space-y-3">
                {escalation.map((step, index) => (
                  <div key={step.id} className="flex items-center gap-4">
                    <div className="flex flex-col items-center flex-shrink-0">
                      <div
                        className={clsx(
                          "w-8 h-8 rounded-full flex items-center justify-center text-xs font-bold border-2",
                          index === 0 ? "bg-primary text-primary-foreground border-primary" : "bg-card text-muted-foreground border-border",
                        )}
                      >
                        {index + 1}
                      </div>
                      {index < escalation.length - 1 && <div className="w-0.5 h-6 bg-muted mt-1" />}
                    </div>

                    <div className="flex-1 flex items-center gap-3 p-3 bg-muted border border-border rounded-xl">
                      <div className="flex items-center gap-2 flex-shrink-0">
                        <label className="text-micro font-bold text-muted-foreground uppercase">Após</label>
                        <input
                          type="number"
                          min={1}
                          value={step.afterMinutes}
                          onChange={(e) => updateEscalation(step.id, { afterMinutes: parseInt(e.target.value) || 1 })}
                          className="w-16 px-2 py-1 bg-card border border-border rounded-lg text-xs text-center focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
                        />
                        <label className="text-micro font-bold text-muted-foreground">min</label>
                      </div>
                      <div className="flex-1">
                        <input
                          type="text"
                          value={step.label}
                          onChange={(e) => updateEscalation(step.id, { label: e.target.value })}
                          className="w-full px-3 py-1 bg-card border border-border rounded-lg text-xs focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
                        />
                      </div>
                      <select
                        value={step.action}
                        onChange={(e) => updateEscalation(step.id, { action: e.target.value as any })}
                        className="px-2 py-1 bg-card border border-border rounded-lg text-micro font-bold focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
                      >
                        <option value="notify_manager">Notificar Gerente</option>
                        <option value="redirect_team">Redirecionar Equipe</option>
                        <option value="notify_whatsapp">Alerta WhatsApp</option>
                        <option value="auto_assign_supervisor">Atribuir Supervisor</option>
                      </select>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => removeEscalationStep(step.id)}
                        className="size-7 text-muted-foreground hover:text-status-critical"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </Button>
                    </div>
                  </div>
                ))}

                <Button variant="ghost" size="sm" onClick={addEscalationStep} className="ml-12 mt-2 text-xs font-bold text-primary hover:text-primary">
                  <Plus className="w-3.5 h-3.5" /> Adicionar passo de escalação
                </Button>
              </div>
            </motion.div>
          )}
        </AnimatePresence>
      </div>

      <div className="bg-card rounded-2xl border border-border shadow-sm p-6">
        <h3 className="text-lg font-bold text-foreground mb-1 flex items-center gap-2">
          <Zap className="w-5 h-5 text-primary-600" /> Canais de Notificação Imediata
        </h3>
        <p className="text-sm text-muted-foreground mb-5">Configure como os corretores serão notificados quando receberem um lead.</p>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {notifChannels.map((ch) => {
            const NIcon = NOTIF_ICONS[ch.icon] || Bell;
            return (
              <div
                key={ch.id}
                onClick={() => toggleNotif(ch.id)}
                className={clsx(
                  "flex items-center gap-4 p-4 border rounded-xl cursor-pointer transition-all",
                  ch.enabled
                    ? "border-status-ok/30 bg-status-ok-soft/40 ring-1 ring-emerald-600/10"
                    : "border-border bg-muted opacity-70 hover:opacity-90",
                )}
              >
                <div
                  className={clsx(
                    "w-10 h-10 rounded-full flex items-center justify-center transition-colors",
                    ch.enabled ? "bg-status-ok-soft" : "bg-muted",
                  )}
                >
                  <NIcon className={clsx("w-5 h-5", ch.enabled ? "text-status-ok" : "text-muted-foreground")} />
                </div>
                <div className="flex-1 min-w-0">
                  <span className="block text-sm font-bold text-foreground">{ch.name}</span>
                  <span className="block text-xs text-muted-foreground mt-0.5">{ch.description}</span>
                </div>
                <div className={clsx("w-10 h-6 rounded-full flex items-center px-0.5 transition-colors", ch.enabled ? "bg-status-ok" : "bg-muted-foreground/30")}>
                  <motion.div
                    className="w-5 h-5 bg-card rounded-full shadow-sm"
                    animate={{ x: ch.enabled ? 16 : 0 }}
                    transition={{ type: "spring", stiffness: 500, damping: 30 }}
                  />
                </div>
              </div>
            );
          })}
        </div>
      </div>

      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Button variant="outline" onClick={handleReset} className="font-bold text-muted-foreground">
            <RotateCcw className="w-4 h-4" /> Restaurar Padrão
          </Button>
          {dirty && (
            <span className="flex items-center gap-1.5 text-xs font-bold text-status-warning bg-status-warning-soft px-2.5 py-1 rounded-full border border-status-warning/30">
              <AlertCircle className="w-3 h-3" /> Alterações não salvas
            </span>
          )}
        </div>
        <Button onClick={handleSave} disabled={!dirty}>
          <Save className="w-4 h-4" /> Salvar Configurações
        </Button>
      </div>
    </div>
  );
}

