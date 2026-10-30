import { useState, useMemo, useCallback, useId, forwardRef, useImperativeHandle } from "react";
import { motion } from "motion/react";
import { Mail, Smartphone, Check, ChevronRight, BellRing, Volume2, VolumeX } from "lucide-react";
import { clsx } from "clsx";
import { useSettingsLoad, SettingsLoadState, UnavailableSetting } from "./useSettingsLoad";
import { Button } from "../../ui/button";
import { ResponsiveTable } from "../../ui/responsive-table";
import { settingsService, type NotificationChannel } from "../../services/settings.service";
import { SectionCard, FieldRow, Toggle, SettingsSelect, Badge, type TabRef } from "./_shared";
import { INITIAL_NOTIFICATIONS } from "./_constants";

export const NotificationsTab = forwardRef<TabRef, { onChange: () => void }>(({ onChange }, ref) => {
  const [channels, setChannels] = useState<NotificationChannel[]>(INITIAL_NOTIFICATIONS);
  const [quietHoursEnabled, setQuietHoursEnabled] = useState(false);
  const [quietStart, setQuietStart] = useState("22:00");
  const [quietEnd, setQuietEnd] = useState("07:00");
  const [digestFrequency, setDigestFrequency] = useState("realtime");
  const [collapsedCategories, setCollapsedCategories] = useState<Set<string>>(() => new Set());
  const categoryId = useId();

  const channelToggle = (ch: NotificationChannel, channel: "email" | "whatsapp" | "inApp") => {
    const label = channel === "inApp" ? "No painel" : channel === "email" ? "E-mail" : "WhatsApp";
    return (
      <button
        type="button"
        aria-label={`${ch.label}: ${label}`}
        aria-pressed={ch[channel]}
        onClick={() => toggleChannel(ch.id, channel)}
        className="mx-auto flex size-9 items-center justify-center rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <div className={clsx("w-5 h-5 rounded-md flex items-center justify-center transition-colors", ch[channel] ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground/60 hover:bg-muted")}>
          {ch[channel] && <Check className="w-3 h-3" />}
        </div>
      </button>
    );
  };

  const load = useSettingsLoad(useCallback(async () => {
    await settingsService
      .getNotifications()
      .then((apiSettings) => {
        if (apiSettings.channels) {
          setChannels(apiSettings.channels);
        }
        if (apiSettings.deliveryFreq) setDigestFrequency(apiSettings.deliveryFreq);
        if (apiSettings.quietHoursStart) setQuietStart(apiSettings.quietHoursStart);
        if (apiSettings.quietHoursEnd) setQuietEnd(apiSettings.quietHoursEnd);
        if (apiSettings.quietHoursStart || apiSettings.quietHoursEnd) {
          setQuietHoursEnabled(true);
        }
      });
  }, []));

  useImperativeHandle(ref, () => ({
    save: async () => {
      load.requireReady();
      await settingsService.updateNotifications({
        channels: channels,
        deliveryFreq: digestFrequency,
        quietHoursStart: quietHoursEnabled ? quietStart : "",
        quietHoursEnd: quietHoursEnabled ? quietEnd : "",
      });
    },
  }));

  const categories = useMemo(() => [...new Set(channels.map((c) => c.category))], [channels]);

  const toggleChannel = (id: string, channel: "email" | "whatsapp" | "inApp") => {
    setChannels((prev) => prev.map((c) => (c.id === id ? { ...c, [channel]: !c[channel] } : c)));
    onChange();
  };

  const toggleGlobal = (channel: "email" | "whatsapp" | "inApp", value: boolean) => {
    setChannels((prev) => prev.map((c) => ({ ...c, [channel]: value })));
    onChange();
  };

  const channelState = (channel: "email" | "whatsapp" | "inApp") => {
    const count = channels.filter(item => item[channel]).length;
    return { enabled: count > 0 && count === channels.length, mixed: count > 0 && count < channels.length };
  };
  if (!load.ready) return <SettingsLoadState error={load.error} retry={load.retry} />;

  return (
    <section aria-label="Preferências de notificação" className="space-y-6">
      <SectionCard title="Canais de Notificação" subtitle="Habilite ou desabilite canais globalmente">
        <div className="grid grid-cols-1 xl:grid-cols-3 gap-4 mb-6">
          {([
            { channel: "email" as const, label: "E-mail", icon: Mail, ...channelState("email"), desc: "Receba no e-mail" },
            { channel: "whatsapp" as const, label: "WhatsApp", icon: Smartphone, ...channelState("whatsapp"), desc: "Alertas no celular" },
            { channel: "inApp" as const, label: "No Painel", icon: BellRing, ...channelState("inApp"), desc: "Notificações no CRM" },
          ]).map(({ channel, label, icon: Icon, enabled, mixed, desc }) => (
            <div
              key={channel}
              className={clsx(
                "p-4 rounded-2xl border transition-colors",
                enabled ? "bg-card border-primary-200" : "bg-muted border-border",
              )}
            >
              <div className="flex items-center justify-between mb-3">
                <div className="flex items-center gap-2">
                  <Icon className={clsx("w-4.5 h-4.5", enabled ? "text-primary-600" : "text-muted-foreground")} />
                  <span className="text-sm font-bold text-foreground">{label}</span>
                </div>
                <Button variant="outline" size="sm" role="checkbox" aria-checked={mixed ? "mixed" : enabled} aria-label={`Todas as notificações por ${label}`} disabled={channels.length === 0} onClick={() => toggleGlobal(channel, !enabled)}>
                  {mixed ? "Parcial" : enabled ? "Ligado" : "Desligado"}
                </Button>
              </div>
              <p className="text-xs text-foreground/80">{desc}</p>
            </div>
          ))}
        </div>

        <UnavailableSetting title="Enviar notificação de teste">O envio de teste ainda não está disponível. Nenhuma mensagem será enviada por esta tela.</UnavailableSetting>
      </SectionCard>

      <SectionCard title="Regras de Notificação" subtitle="Configure gatilhos individuais por canal" delay={0.05}>
        <div className="space-y-3">
          {categories.map((category, index) => {
            const catChannels = channels.filter((c) => c.category === category);
            const expanded = !collapsedCategories.has(category);
            const panelId = `${categoryId}-${index}`;
            return (
              <div key={category} className="border border-border rounded-xl overflow-hidden">
                <button
                  type="button"
                  aria-label={`${category} (${catChannels.length} gatilhos)`}
                  aria-expanded={expanded}
                  aria-controls={panelId}
                  onClick={() => setCollapsedCategories(current => {
                    const next = new Set(current);
                    if (next.has(category)) next.delete(category); else next.add(category);
                    return next;
                  })}
                  className="w-full flex items-center justify-between px-4 py-3 bg-muted hover:bg-muted transition-colors"
                >
                  <div className="flex items-center gap-2">
                    <ChevronRight className={clsx("w-4 h-4 text-muted-foreground transition-transform", expanded && "rotate-90")} />
                    <span className="text-sm font-bold text-foreground">{category}</span>
                    <Badge text={`${catChannels.length}`} />
                  </div>
                </button>
                <div id={panelId} hidden={!expanded} className="overflow-x-auto">
                    <div
                      className="overflow-hidden"
                    >
                      <ResponsiveTable
                        data={catChannels}
                        caption={`Notificações de ${category}`}
                        getRowKey={(ch) => ch.id}
                        columns={[
                          { key: "label", header: "Gatilho", primary: true, cell: (ch) => ch.label },
                          { key: "email", header: "E-mail", className: "text-center", cell: (ch) => channelToggle(ch, "email") },
                          { key: "whatsapp", header: "WhatsApp", className: "text-center", cell: (ch) => channelToggle(ch, "whatsapp") },
                          { key: "inApp", header: "No painel", className: "text-center", cell: (ch) => channelToggle(ch, "inApp") },
                        ]}
                      />
                    </div>
                </div>
              </div>
            );
          })}
        </div>
      </SectionCard>

      <SectionCard title="Frequência & Horário Silencioso" subtitle="Controle quando e como as notificações são entregues" delay={0.1}>
        <div className="divide-y divide-border space-y-0">
          <FieldRow label="Frequência de entrega" description="Como as notificações por e-mail são agrupadas">
            <SettingsSelect
              value={digestFrequency}
              onChange={(v) => {
                setDigestFrequency(v);
                onChange();
              }}
              options={[
                { value: "realtime", label: "Tempo real" },
                { value: "hourly", label: "Resumo por hora" },
                { value: "daily", label: "Resumo diario (8h)" },
                { value: "weekly", label: "Resumo semanal (Seg)" },
              ]}
            />
          </FieldRow>
          <FieldRow label="Horário silencioso" description="Suprime notificações push durante a madrugada">
            <Toggle
              checked={quietHoursEnabled}
              onChange={(v) => {
                setQuietHoursEnabled(v);
                onChange();
              }}
            />
          </FieldRow>
          {quietHoursEnabled && (
            <motion.div initial={{ height: 0, opacity: 0 }} animate={{ height: "auto", opacity: 1 }} className="overflow-hidden">
              <div className="flex flex-wrap items-center gap-4 pt-4">
                <div className="flex flex-wrap items-center gap-2">
                  <VolumeX className="w-4 h-4 text-muted-foreground" />
                  <span className="text-xs text-muted-foreground">De</span>
                  <input
                    type="time"
                    aria-label="Início do horário silencioso"
                    value={quietStart}
                    onChange={(e) => {
                      setQuietStart(e.target.value);
                      onChange();
                    }}
                    className="px-2 py-1.5 border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500/20"
                  />
                  <span className="text-xs text-muted-foreground">ate</span>
                  <input
                    type="time"
                    aria-label="Fim do horário silencioso"
                    value={quietEnd}
                    onChange={(e) => {
                      setQuietEnd(e.target.value);
                      onChange();
                    }}
                    className="px-2 py-1.5 border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500/20"
                  />
                  <Volume2 className="w-4 h-4 text-muted-foreground" />
                </div>
              </div>
            </motion.div>
          )}
        </div>
      </SectionCard>
    </section>
  );
});
