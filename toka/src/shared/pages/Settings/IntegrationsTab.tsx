import { useState, useMemo, useEffect, forwardRef, useImperativeHandle } from "react";
import { motion, AnimatePresence } from "motion/react";
import {
  Building2,
  Globe,
  Mail,
  Smartphone,
  AlertTriangle,
  Plus,
  Trash2,
  Copy,
  Eye,
  EyeOff,
  RefreshCw,
  Webhook,
  Zap,
  Database,
  Settings2,
  CalendarDays,
  Link2,
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { ConfirmDialog } from "../../components/ConfirmDialog";
import { Button } from "../../ui/button";
import { settingsService, type WebhookEndpoint } from "../../services/settings.service";
import { SectionCard, SettingsInput, Toggle, Badge, type TabRef } from "./_shared";
import { STATIC_INTEGRATIONS, PORTAL_DEFINITIONS, WEBHOOK_EVENT_OPTIONS, type Integration } from "./_constants";
import { PortalConfigModal } from "./PortalConfigModal";
import { notifyApiError } from "../../services/errorToast";

export const IntegrationsTab = forwardRef<TabRef, { onChange: () => void }>(({ onChange }, ref) => {
  const [integrations, setIntegrations] = useState<Integration[]>(STATIC_INTEGRATIONS);
  const [portalIntegrations, setPortalIntegrations] = useState<any[]>([]);
  const [webhooks, setWebhooks] = useState<WebhookEndpoint[]>([]);
  const [showApiKey, setShowApiKey] = useState(false);
  const [apiKeys, setApiKeys] = useState<any[]>([]);
  const [showNewWebhook, setShowNewWebhook] = useState(false);
  const [newWebhookUrl, setNewWebhookUrl] = useState("");
  const [newWebhookEvents, setNewWebhookEvents] = useState<string[]>([]);
  const [deleteWebhook, setDeleteWebhook] = useState<WebhookEndpoint | null>(null);
  const [connectingId, setConnectingId] = useState<string | null>(null);
  const [syncingId, setSyncingId] = useState<string | null>(null);
  const [configPortal, setConfigPortal] = useState<{
    portalDef: typeof PORTAL_DEFINITIONS[0];
    backendData: any | null;
  } | null>(null);

  useEffect(() => {
    const loadPortalIntegrations = async () => {
      try {
        const { portalIntegrationService: portalService } = await import("../../services/portal-integration.service");
        const backendIntegrations = await portalService.listIntegrations();
        setPortalIntegrations(Array.isArray(backendIntegrations) ? backendIntegrations : []);
      } catch {
        setPortalIntegrations([]);
      }
    };
    loadPortalIntegrations();

    settingsService.listWebhooks().then(setWebhooks).catch(console.error);
    settingsService.listAPIKeys().then(setApiKeys).catch(console.error);
  }, []);

  useImperativeHandle(ref, () => ({
    save: async () => {
    },
  }));

  const allIntegrations = useMemo(() => {
    const safePortalIntegrations = Array.isArray(portalIntegrations) ? portalIntegrations : [];
    const portalItems: Integration[] = PORTAL_DEFINITIONS.map((def) => {
      const backendInteg = safePortalIntegrations.find((bi: any) => bi.portal === def.portal);
      if (backendInteg) {
        return {
          id: backendInteg.id,
          name: def.name,
          description: def.description,
          status:
            backendInteg.status === "active"
              ? ("connected" as const)
              : backendInteg.status === "error"
                ? ("error" as const)
                : ("disconnected" as const),
          icon: def.icon,
          category: "Portais",
          lastSync: backendInteg.last_sync_at ? new Date(backendInteg.last_sync_at).toLocaleString("pt-BR") : null,
          syncFrequency: backendInteg.sync_frequency || "12h",
        };
      }
      return {
        id: `portal-${def.portal}`,
        name: def.name,
        description: def.description,
        status: "disconnected" as const,
        icon: def.icon,
        category: "Portais",
        lastSync: null,
        syncFrequency: "12h",
      };
    });
    return [...portalItems, ...integrations];
  }, [portalIntegrations, integrations]);

  const handleToggleIntegration = async (id: string) => {
    const safePortalIntegrations = Array.isArray(portalIntegrations) ? portalIntegrations : [];

    const portalDef = PORTAL_DEFINITIONS.find((def) => id === `portal-${def.portal}`);
    const existingPortal = safePortalIntegrations.find((bi: any) => bi.id === id);

    if (portalDef && !existingPortal) {
      setConfigPortal({ portalDef, backendData: null });
      return;
    }

    setConnectingId(id);

    try {
      const { portalIntegrationService: portalService } = await import("../../services/portal-integration.service");

      if (existingPortal) {
        await portalService.deleteIntegration(existingPortal.id);
        setPortalIntegrations((prev) =>
          Array.isArray(prev) ? prev.filter((p: any) => p.id !== existingPortal.id) : [],
        );
        toast.success(
          `${PORTAL_DEFINITIONS.find((d) => d.portal === existingPortal.portal)?.name || "Portal"} desconectado.`,
        );
      } else {
        setIntegrations((prev) =>
          prev.map((int) =>
            int.id === id
              ? {
                  ...int,
                  status: int.status === "connected" ? "disconnected" : "connected",
                  lastSync: int.status === "disconnected" ? new Date().toLocaleString("pt-BR") : null,
                }
              : int,
          ),
        );
        const integration = integrations.find((i) => i.id === id);
        if (integration) {
          toast.success(integration.status === "connected" ? `${integration.name} desconectado.` : `${integration.name} conectado com sucesso!`);
        }
      }
      onChange();
    } catch (err: any) {
      notifyApiError(err, { context: "Alterar integração" });
    } finally {
      setConnectingId(null);
    }
  };

  const handleTriggerSync = async (id: string) => {
    setSyncingId(id);
    try {
      const { portalIntegrationService: portalService } = await import("../../services/portal-integration.service");
      await portalService.triggerSync(id);
      toast.success("Sincronização iniciada!");
      const updated = await portalService.listIntegrations();
      setPortalIntegrations(Array.isArray(updated) ? updated : []);
    } catch {
      toast.error("Erro ao iniciar sincronização.");
    } finally {
      setSyncingId(null);
    }
  };

  const handleOpenPortalConfig = (integrationId: string) => {
    const safePortalIntegrations = Array.isArray(portalIntegrations) ? portalIntegrations : [];
    const backendData = safePortalIntegrations.find((bi: any) => bi.id === integrationId);
    if (!backendData) return;
    const portalDef = PORTAL_DEFINITIONS.find((d) => d.portal === backendData.portal);
    if (!portalDef) return;
    setConfigPortal({ portalDef, backendData });
  };

  const handleCopyApiKey = () => {
    navigator.clipboard.writeText(apiKeys[0]?.keyPrefix || "").catch(() => {});
    toast.success("Chave API copiada!");
  };

  const handleRegenerateKey = async () => {
    try {
      if (apiKeys.length > 0 && apiKeys[0].id) {
        await settingsService.revokeAPIKey(apiKeys[0].id);
      }
      const newKey = await settingsService.createAPIKey({ name: "Default Key", scopes: ["all"] } as any);
      const updatedKeys = await settingsService.listAPIKeys();
      setApiKeys(updatedKeys);
      toast.success("Nova chave API gerada: " + newKey.raw_key);
      onChange();
    } catch (e: any) {
      notifyApiError(e, { context: "Gerar chave de API" });
    }
  };

  const handleAddWebhook = async () => {
    if (!newWebhookUrl.trim() || newWebhookEvents.length === 0) return;
    try {
      const wh = await settingsService.createWebhook({
        url: newWebhookUrl.trim(),
        events: newWebhookEvents,
        active: true,
      });
      setWebhooks((prev) => [...prev, wh]);
      setNewWebhookUrl("");
      setNewWebhookEvents([]);
      setShowNewWebhook(false);
      onChange();
      toast.success("Webhook adicionado!");
    } catch (e: any) {
      notifyApiError(e, { context: "Criar webhook" });
    }
  };

  const handleDeleteWebhook = async () => {
    if (!deleteWebhook || !deleteWebhook.id) return;
    try {
      await settingsService.deleteWebhook(deleteWebhook.id);
      setWebhooks((prev) => prev.filter((w) => w.id !== deleteWebhook.id));
      setDeleteWebhook(null);
      onChange();
      toast.success("Webhook removido.");
    } catch (e: any) {
      notifyApiError(e, { context: "Remover webhook" });
    }
  };

  const toggleWebhookActive = async (id: string) => {
    const wh = webhooks.find((w) => w.id === id);
    if (!wh) return;
    try {
      await settingsService.updateWebhook(id, { active: !wh.active });
      setWebhooks((prev) => prev.map((w) => (w.id === id ? { ...w, active: !w.active } : w)));
      onChange();
    } catch (e: any) {
      notifyApiError(e, { context: "Atualizar webhook" });
    }
  };

  const toggleWebhookEvent = (event: string) => {
    setNewWebhookEvents((prev) => (prev.includes(event) ? prev.filter((e) => e !== event) : [...prev, event]));
  };

  const integrationsByCategory = useMemo(() => {
    const map: Record<string, Integration[]> = {};
    allIntegrations.forEach((int) => {
      if (!map[int.category]) map[int.category] = [];
      map[int.category].push(int);
    });
    return map;
  }, [allIntegrations]);

  const getIntIcon = (icon: string) => {
    switch (icon) {
      case "zap":
        return Zap;
      case "globe":
        return Globe;
      case "home":
        return Building2;
      case "mail":
        return Mail;
      case "calendar":
        return CalendarDays;
      case "phone":
        return Smartphone;
      case "database":
        return Database;
      default:
        return Link2;
    }
  };

  return (
    <div className="space-y-6">
      <SectionCard title="Chave de API do suitee" subtitle="Use esta chave para autenticar integrações externas">
        <div className="space-y-4">
          <div className="flex items-center gap-3">
            <div className="flex-1 relative">
              <input
                type={showApiKey ? "text" : "password"}
                value={apiKeys.length > 0 ? apiKeys[0]?.keyPrefix + "********************" : "Nenhuma chave ativa"}
                readOnly
                className="w-full px-3 py-2.5 bg-muted border border-border rounded-xl text-sm font-mono text-foreground pr-20"
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={() => setShowApiKey(!showApiKey)}
                className="absolute right-2 top-1/2 -translate-y-1/2 size-8 text-muted-foreground"
              >
                {showApiKey ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
              </Button>
            </div>
            <Button variant="outline" size="icon" onClick={handleCopyApiKey} className="text-muted-foreground hover:text-primary-600" title="Copiar">
              <Copy className="w-4 h-4" />
            </Button>
            <Button variant="outline" size="icon" onClick={handleRegenerateKey} className="text-muted-foreground hover:text-status-warning" title="Regenerar chave">
              <RefreshCw className="w-4 h-4" />
            </Button>
          </div>
          <div className="flex items-start gap-2 bg-status-warning-soft text-status-warning p-3 rounded-xl border border-status-warning/30">
            <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5" />
            <p className="text-xs">Nunca compartilhe sua chave API publicamente. Ao regenerar, todas as integrações existentes precisarao ser atualizadas.</p>
          </div>
        </div>
      </SectionCard>

      {Object.entries(integrationsByCategory).map(([category, ints], catIdx) => (
        <SectionCard key={category} title={category} subtitle={`${ints.length} integrações disponíveis`} delay={0.05 * (catIdx + 1)}>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {ints.map((int) => {
              const Icon = getIntIcon(int.icon);
              const connecting = connectingId === int.id;
              return (
                <div key={int.id} className={clsx("border rounded-2xl p-5 flex flex-col gap-3 transition-colors", int.status === "connected" ? "border-border bg-card" : "border-border bg-muted")}>
                  <div className="flex items-start justify-between">
                    <div className="flex items-center gap-3">
                      <div className={clsx("w-10 h-10 rounded-xl flex items-center justify-center", int.status === "connected" ? "bg-primary-50 text-primary-600" : "bg-muted text-muted-foreground")}>
                        <Icon className="w-5 h-5" />
                      </div>
                      <div>
                        <h4 className="text-sm font-bold text-foreground">{int.name}</h4>
                        <p className="text-[11px] text-muted-foreground">{int.category}</p>
                      </div>
                    </div>
                    <Badge text={int.status === "connected" ? "Ativo" : int.status === "error" ? "Erro" : "Inativo"} variant={int.status === "connected" ? "success" : int.status === "error" ? "danger" : "default"} />
                  </div>
                  <p className="text-xs text-muted-foreground line-clamp-2">{int.description}</p>
                  {int.lastSync && (
                    <p className="text-[11px] text-muted-foreground flex items-center gap-1">
                      <RefreshCw className="w-3 h-3" />
                      Sincronizado: {int.lastSync}
                    </p>
                  )}
                  <div className="mt-auto pt-2 flex gap-2">
                    <Button onClick={() => handleToggleIntegration(int.id)} loading={connecting} variant={int.status === "connected" ? "outline" : "default"} className="flex-1">
                      {int.status === "connected" ? "Desconectar" : "Conectar"}
                    </Button>
                    {int.status === "connected" && int.category === "Portais" && (
                      <>
                        <Button variant="outline" size="icon" onClick={() => handleTriggerSync(int.id)} loading={syncingId === int.id} className="text-muted-foreground" title="Sincronizar agora">
                          <RefreshCw className="w-4 h-4" />
                        </Button>
                        <Button variant="outline" size="icon" onClick={() => handleOpenPortalConfig(int.id)} className="text-muted-foreground" title="Configurar portal">
                          <Settings2 className="w-4 h-4" />
                        </Button>
                      </>
                    )}
                    {int.status === "connected" && int.category !== "Portais" && (
                      <Button variant="outline" size="icon" className="text-muted-foreground">
                        <Settings2 className="w-4 h-4" />
                      </Button>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </SectionCard>
      ))}

      <SectionCard
        title="Webhooks Personalizados"
        subtitle="Dispare eventos em tempo real para URLs externas"
        delay={0.2}
        actions={
          <Button onClick={() => setShowNewWebhook(true)} variant="outline" size="sm" className="border-primary/30 bg-primary/10 text-primary hover:bg-primary/15 hover:text-primary">
            <Plus className="w-3.5 h-3.5" />
            Endpoint
          </Button>
        }
      >
        <div className="space-y-3">
          {webhooks.map((wh) => (
            <div key={wh.id} className="flex flex-col gap-3 p-4 bg-muted rounded-xl border border-border">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3 min-w-0 flex-1">
                  <Webhook className={clsx("w-4 h-4 shrink-0", wh.active ? "text-primary-600" : "text-muted-foreground")} />
                  <code className="text-xs font-mono text-foreground truncate">{wh.url}</code>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <Toggle checked={wh.active} onChange={() => wh.id && toggleWebhookActive(wh.id)} size="sm" />
                  <Button variant="ghost" size="icon" onClick={() => setDeleteWebhook(wh)} className="size-8 text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft">
                    <Trash2 className="w-3.5 h-3.5" />
                  </Button>
                </div>
              </div>
              <div className="flex flex-wrap gap-1.5">
                {wh.events.map((e) => (
                  <Badge key={e} text={e} variant="info" />
                ))}
              </div>
              {wh.lastTriggered && <p className="text-[11px] text-muted-foreground">Último disparo: {wh.lastTriggered}</p>}
            </div>
          ))}
          {webhooks.length === 0 && <p className="text-sm text-muted-foreground text-center py-6">Nenhum webhook configurado.</p>}
        </div>

        <AnimatePresence>
          {showNewWebhook && (
            <motion.div initial={{ height: 0, opacity: 0 }} animate={{ height: "auto", opacity: 1 }} exit={{ height: 0, opacity: 0 }} className="overflow-hidden">
              <div className="mt-4 p-5 bg-primary-50 rounded-xl border border-primary-200 space-y-4">
                <p className="text-sm font-bold text-primary-900">Novo Webhook Endpoint</p>
                <div className="space-y-1.5">
                  <label className="text-xs font-bold text-primary-800">URL de Destino</label>
                  <SettingsInput value={newWebhookUrl} onChange={setNewWebhookUrl} placeholder="https://api.seusite.com/webhook" icon={<Globe className="w-4 h-4" />} />
                </div>
                <div className="space-y-2">
                  <label className="text-xs font-bold text-primary-800">Eventos (selecione ao menos 1)</label>
                  <div className="flex flex-wrap gap-2">
                    {WEBHOOK_EVENT_OPTIONS.map((event) => (
                      <button
                        key={event}
                        onClick={() => toggleWebhookEvent(event)}
                        className={clsx(
                          "px-2.5 py-1.5 rounded-lg text-[11px] font-bold border transition-colors",
                          newWebhookEvents.includes(event) ? "bg-primary text-primary-foreground border-primary" : "bg-card text-muted-foreground border-border hover:border-border",
                        )}
                      >
                        {event}
                      </button>
                    ))}
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Button onClick={handleAddWebhook} disabled={!newWebhookUrl.trim() || newWebhookEvents.length === 0}>
                    Criar Webhook
                  </Button>
                  <Button variant="outline" onClick={() => { setShowNewWebhook(false); setNewWebhookUrl(""); setNewWebhookEvents([]); }}>
                    Cancelar
                  </Button>
                </div>
              </div>
            </motion.div>
          )}
        </AnimatePresence>
      </SectionCard>

      <ConfirmDialog
        open={!!deleteWebhook}
        title="Remover webhook"
        description={`Tem certeza que deseja remover o endpoint "${deleteWebhook?.url}"? Eventos nao serao mais enviados para esta URL.`}
        confirmLabel="Remover Webhook"
        variant="warning"
        onConfirm={handleDeleteWebhook}
        onCancel={() => setDeleteWebhook(null)}
      />

      <AnimatePresence>
        {configPortal && (
          <PortalConfigModal
            portalDef={configPortal.portalDef}
            backendData={configPortal.backendData}
            onClose={() => setConfigPortal(null)}
            onSaved={(integration) => {
              setPortalIntegrations((prev) => {
                const arr = Array.isArray(prev) ? prev : [];
                const idx = arr.findIndex((p: any) => p.id === integration.id);
                if (idx >= 0) {
                  const updated = [...arr];
                  updated[idx] = integration;
                  return updated;
                }
                return [...arr, integration];
              });
              setConfigPortal(null);
              onChange();
            }}
          />
        )}
      </AnimatePresence>
    </div>
  );
});
