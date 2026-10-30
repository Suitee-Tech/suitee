import { useEffect, useState } from "react";
import { Bell, Mail, Smartphone, MessageCircle } from "lucide-react";
import { clsx } from "clsx";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
} from "../ui/sheet";
import { Button } from "../ui/button";
import type {
  CreateReminderRuleDTO,
  ReminderChannel,
  ReminderRule,
  ReminderTriggerType,
  UpdateReminderRuleDTO,
} from "../services/reminderRule.service";
import { reminderRuleService } from "../services/reminderRule.service";

interface ReminderRuleModalProps {
  rule: ReminderRule | null;
  onClose: () => void;
}

const CHANNEL_OPTIONS: { value: ReminderChannel; label: string; icon: React.ReactNode }[] = [
  { value: "email", label: "Email", icon: <Mail className="w-4 h-4" /> },
  { value: "in_app", label: "In-app", icon: <Smartphone className="w-4 h-4" /> },
  { value: "whatsapp", label: "WhatsApp", icon: <MessageCircle className="w-4 h-4" /> },
];

const TEMPLATE_HINT =
  "Use {tenant_name}, {amount}, {due_date}, {total_amount} para inserir variáveis dinâmicas.";

export function ReminderRuleModal({ rule, onClose }: ReminderRuleModalProps) {
  const [form, setForm] = useState({
    name: "",
    description: "",
    triggerType: "BEFORE_DUE" as ReminderTriggerType,
    offsetDays: "3",
    channels: ["email"] as ReminderChannel[],
    template:
      "Olá {tenant_name}, sua cobrança de {amount} vence em {due_date}. Por favor, realize o pagamento para evitar multa e juros.",
    enabled: true,
  });
  const [error, setError] = useState<string | null>(null);
  const [isPending, setIsPending] = useState(false);

  useEffect(() => {
    if (rule) {
      setForm({
        name: rule.name,
        description: rule.description ?? "",
        triggerType: rule.trigger_type,
        offsetDays: String(rule.offset_days),
        channels: rule.channels,
        template: rule.template,
        enabled: rule.enabled,
      });
    } else {
      setForm({
        name: "",
        description: "",
        triggerType: "BEFORE_DUE",
        offsetDays: "3",
        channels: ["email"],
        template:
          "Olá {tenant_name}, sua cobrança de {amount} vence em {due_date}. Por favor, realize o pagamento para evitar multa e juros.",
        enabled: true,
      });
    }
    setError(null);
  }, [rule?.id]);

  const toggleChannel = (c: ReminderChannel) => {
    setForm((f) =>
      f.channels.includes(c)
        ? { ...f, channels: f.channels.filter((x) => x !== c) }
        : { ...f, channels: [...f.channels, c] },
    );
  };

  const handleSubmit = async () => {
    setError(null);
    const offset = parseInt(form.offsetDays, 10);
    if (form.triggerType !== "ON_DUE" && (isNaN(offset) || offset < 1)) {
      setError("Para BEFORE_DUE / AFTER_DUE o offset precisa ser >= 1.");
      return;
    }
    if (form.triggerType === "ON_DUE" && offset !== 0) {
      setError("Para ON_DUE o offset deve ser 0.");
      return;
    }
    if (form.channels.length === 0) {
      setError("Selecione pelo menos um canal.");
      return;
    }
    if (!form.name.trim() || !form.template.trim()) {
      setError("Nome e template são obrigatórios.");
      return;
    }
    const payload: CreateReminderRuleDTO | UpdateReminderRuleDTO = {
      name: form.name.trim(),
      description: form.description.trim() || undefined,
      trigger_type: form.triggerType,
      offset_days: form.triggerType === "ON_DUE" ? 0 : offset,
      channels: form.channels,
      template: form.template,
      enabled: form.enabled,
    };
    setIsPending(true);
    try {
      if (rule) {
        await reminderRuleService.update(rule.id, payload as UpdateReminderRuleDTO);
      } else {
        await reminderRuleService.create(payload as CreateReminderRuleDTO);
      }
      onClose();
    } finally {
      setIsPending(false);
    }
  };

  return (
    <Sheet open onOpenChange={(next) => { if (!next) onClose(); }}>
      <SheetContent side="right" className="w-full gap-0 p-0 sm:max-w-xl">
        <div className="bg-card px-6 py-4 border-b border-border flex items-center gap-3 flex-shrink-0 pr-12">
          <div className="p-2 bg-primary-50 text-primary-700 rounded-lg">
            <Bell className="w-5 h-5" />
          </div>
          <div>
            <SheetTitle className="text-lg font-bold text-foreground">
              {rule ? "Editar Regra" : "Nova Regra de Lembrete"}
            </SheetTitle>
            <SheetDescription className="text-sm">Régua de Comunicação</SheetDescription>
          </div>
        </div>

        <div className="flex-1 overflow-y-auto p-6 space-y-5">
          <div>
            <label className="block text-xs font-medium text-foreground mb-1">Nome da Regra *</label>
            <input
              type="text"
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              placeholder="Ex: 3 dias antes do vencimento"
              className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none transition-all"
            />
          </div>

          <div>
            <label className="block text-xs font-medium text-foreground mb-1">Descrição</label>
            <textarea
              value={form.description}
              onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
              rows={2}
              placeholder="Opcional — contexto interno da regra"
              className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none resize-none transition-all"
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-medium text-foreground mb-1">Disparo *</label>
              <select
                value={form.triggerType}
                onChange={(e) =>
                  setForm((f) => {
                    const t = e.target.value as ReminderTriggerType;
                    return {
                      ...f,
                      triggerType: t,
                      offsetDays: t === "ON_DUE" ? "0" : f.offsetDays === "0" ? "3" : f.offsetDays,
                    };
                  })
                }
                className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none transition-all"
              >
                <option value="BEFORE_DUE">Antes do vencimento</option>
                <option value="ON_DUE">No dia do vencimento</option>
                <option value="AFTER_DUE">Após o vencimento</option>
              </select>
            </div>
            <div>
              <label className="block text-xs font-medium text-foreground mb-1">Dias de Offset *</label>
              <input
                type="number"
                min={0}
                max={365}
                value={form.offsetDays}
                onChange={(e) => setForm((f) => ({ ...f, offsetDays: e.target.value }))}
                disabled={form.triggerType === "ON_DUE"}
                className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none transition-all disabled:opacity-60 disabled:cursor-not-allowed"
              />
            </div>
          </div>

          <div>
            <label className="block text-xs font-medium text-foreground mb-2">Canais *</label>
            <div className="grid grid-cols-3 gap-2">
              {CHANNEL_OPTIONS.map((opt) => (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => toggleChannel(opt.value)}
                  className={clsx(
                    "flex items-center justify-center gap-2 px-2 py-2 rounded-xl border text-xs font-medium transition-all",
                    form.channels.includes(opt.value)
                      ? "bg-primary-50 border-primary-300 text-primary-700 ring-2 ring-primary-600/20"
                      : "bg-card border-border text-muted-foreground hover:bg-muted",
                  )}
                >
                  <span
                    className={
                      form.channels.includes(opt.value)
                        ? "text-primary-600"
                        : "text-muted-foreground"
                    }
                  >
                    {opt.icon}
                  </span>
                  {opt.label}
                </button>
              ))}
            </div>
          </div>

          <div>
            <label className="block text-xs font-medium text-foreground mb-1">Mensagem *</label>
            <textarea
              value={form.template}
              onChange={(e) => setForm((f) => ({ ...f, template: e.target.value }))}
              rows={4}
              className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none resize-none transition-all font-mono"
            />
            <p className="text-caption text-muted-foreground mt-1">{TEMPLATE_HINT}</p>
          </div>

          <label className="flex items-center gap-2 text-sm text-foreground">
            <input
              type="checkbox"
              checked={form.enabled}
              onChange={(e) => setForm((f) => ({ ...f, enabled: e.target.checked }))}
              className="rounded border-border focus:ring-2 focus:ring-primary-600/20"
            />
            Regra ativa (será avaliada diariamente)
          </label>

          {error && <p className="text-xs text-status-critical">{error}</p>}
        </div>

        <div className="px-6 py-4 border-t border-border flex justify-end gap-3 bg-card flex-shrink-0">
          <Button variant="outline" onClick={onClose}>
            Cancelar
          </Button>
          <Button onClick={handleSubmit} loading={isPending} loadingText="Salvando…">
            {rule ? "Salvar Alterações" : "Criar Regra"}
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}

