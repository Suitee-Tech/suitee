import React, { useState, useEffect, useId } from "react";
import {
  Building2,
  Globe,
  Mail,
  AlertTriangle,
  Phone,
  Copy,
  RefreshCw,
  Zap,
  Settings2,
  Save,
  Filter,
  User,
  History,
  Link as LinkIcon,
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import type { PortalIntegration, FeedPropertyFilter, PortalSync, PortalLead } from "../../services/portal-integration.service";
import { Button } from "../../ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "../../ui/dialog";
import { formatPhoneBR, parsePhoneBR } from "../../utils/masks";
import { MoneyInput } from "../../ui/money-input";
import { SettingsInput, SettingsSelect, Badge } from "./_shared";
import { PORTAL_DEFINITIONS, PORTAL_PROPERTY_STATUSES, PORTAL_PROPERTY_TYPES, PORTAL_LISTING_TYPES } from "./_constants";
import { notifyApiError } from "../../services/errorToast";

export function PortalConfigModal({
  portalDef,
  backendData,
  onClose,
  onSaved,
}: {
  portalDef: typeof PORTAL_DEFINITIONS[0];
  backendData: PortalIntegration | null;
  onClose: () => void;
  onSaved: (integration: PortalIntegration) => void;
}) {
  const contactNameId = useId();
  const contactEmailId = useId();
  const contactPhoneId = useId();
  const syncFrequencyId = useId();
  const publicationTypeId = useId();
  const minPriceId = useId();
  const maxPriceId = useId();
  const feedUrlId = useId();
  const webhookUrlId = useId();
  const isEditing = !!backendData;
  const [activeTab, setActiveTab] = useState<"config" | "syncs" | "leads">("config");
  const [saving, setSaving] = useState(false);

  const [contactName, setContactName] = useState(backendData?.contact_name || "");
  const [contactEmail, setContactEmail] = useState(backendData?.contact_email || "");
  const [contactPhone, setContactPhone] = useState(parsePhoneBR(backendData?.contact_phone || ""));
  const [syncFrequency, setSyncFrequency] = useState(backendData?.sync_frequency || "12h");
  const [publicationType, setPublicationType] = useState(backendData?.publication_type || "STANDARD");

  const [filterStatuses, setFilterStatuses] = useState<string[]>(backendData?.property_filter?.statuses || []);
  const [filterPropertyTypes, setFilterPropertyTypes] = useState<string[]>(
    backendData?.property_filter?.property_types || [],
  );
  const [filterListingTypes, setFilterListingTypes] = useState<string[]>(
    backendData?.property_filter?.listing_types || [],
  );
  const [minPrice, setMinPrice] = useState(backendData?.property_filter?.min_price?.toString() || "");
  const [maxPrice, setMaxPrice] = useState(backendData?.property_filter?.max_price?.toString() || "");

  const [syncHistory, setSyncHistory] = useState<PortalSync[]>([]);
  const [syncHistoryTotal, setSyncHistoryTotal] = useState(0);
  const [leads, setLeads] = useState<PortalLead[]>([]);
  const [leadsTotal, setLeadsTotal] = useState(0);
  const [loadingSyncs, setLoadingSyncs] = useState(false);
  const [loadingLeads, setLoadingLeads] = useState(false);

  useEffect(() => {
    if (!backendData) return;
    if (activeTab === "syncs" && syncHistory.length === 0 && !loadingSyncs) {
      loadSyncHistory();
    } else if (activeTab === "leads" && leads.length === 0 && !loadingLeads) {
      loadLeads();
    }
  }, [activeTab]);

  const loadSyncHistory = async () => {
    if (!backendData) return;
    setLoadingSyncs(true);
    try {
      const { portalIntegrationService: portalService } = await import("../../services/portal-integration.service");
      const result = await portalService.getSyncHistory(backendData.id, 10, 0);
      setSyncHistory(result.data || []);
      setSyncHistoryTotal(result.total || 0);
    } catch {
      toast.error("Erro ao carregar histórico de sincronização.");
    } finally {
      setLoadingSyncs(false);
    }
  };

  const loadLeads = async () => {
    if (!backendData) return;
    setLoadingLeads(true);
    try {
      const { portalIntegrationService: portalService } = await import("../../services/portal-integration.service");
      const result = await portalService.getLeadsByIntegration(backendData.id, 10, 0);
      setLeads(result.data || []);
      setLeadsTotal(result.total || 0);
    } catch {
      toast.error("Erro ao carregar leads.");
    } finally {
      setLoadingLeads(false);
    }
  };

  const toggleChip = (arr: string[], setArr: React.Dispatch<React.SetStateAction<string[]>>, value: string) => {
    setArr((prev) => (prev.includes(value) ? prev.filter((v) => v !== value) : [...prev, value]));
  };

  const handleSave = async () => {
    if (!contactName.trim() || !contactEmail.trim() || !contactPhone.trim()) {
      toast.error("Preencha todos os campos de contato.");
      return;
    }
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(contactEmail)) {
      toast.error("Informe um e-mail valido.");
      return;
    }
    setSaving(true);
    try {
      const { portalIntegrationService: portalService } = await import("../../services/portal-integration.service");
      const propertyFilter: FeedPropertyFilter = {};
      if (filterStatuses.length > 0) propertyFilter.statuses = filterStatuses;
      if (filterPropertyTypes.length > 0) propertyFilter.property_types = filterPropertyTypes;
      if (filterListingTypes.length > 0) propertyFilter.listing_types = filterListingTypes;
      if (minPrice) propertyFilter.min_price = Number(minPrice);
      if (maxPrice) propertyFilter.max_price = Number(maxPrice);

      let result: PortalIntegration;
      if (isEditing && backendData) {
        result = await portalService.updateIntegration(backendData.id, {
          contact_name: contactName,
          contact_email: contactEmail,
          contact_phone: contactPhone,
          sync_frequency: syncFrequency,
          publication_type: publicationType,
          property_filter: propertyFilter,
        });
        toast.success("Configuração atualizada!");
      } else {
        result = await portalService.createIntegration({
          portal: portalDef.portal,
          contact_name: contactName,
          contact_email: contactEmail,
          contact_phone: contactPhone,
          sync_frequency: syncFrequency,
          publication_type: publicationType,
          property_filter: propertyFilter,
        });
        toast.success(`${portalDef.name} conectado com sucesso!`);
      }
      onSaved(result);
    } catch (err: any) {
      notifyApiError(err, { context: "Salvar integração de portal" });
    } finally {
      setSaving(false);
    }
  };

  const handleCopyUrl = (url: string, label: string) => {
    navigator.clipboard.writeText(url).catch(() => {});
    toast.success(`${label} copiada!`);
  };

  const syncStatusLabel = (status: string) => {
    switch (status) {
      case "completed":
        return "Concluido";
      case "failed":
        return "Falhou";
      case "in_progress":
        return "Em andamento";
      case "pending":
        return "Pendente";
      default:
        return status;
    }
  };

  const syncStatusVariant = (status: string): "success" | "danger" | "warning" | "info" | "default" => {
    switch (status) {
      case "completed":
        return "success";
      case "failed":
        return "danger";
      case "in_progress":
        return "info";
      case "pending":
        return "warning";
      default:
        return "default";
    }
  };

  const leadStatusLabel = (status: string) => {
    switch (status) {
      case "processed":
        return "Processado";
      case "received":
        return "Recebido";
      case "failed":
        return "Falhou";
      case "duplicate":
        return "Duplicado";
      default:
        return status;
    }
  };

  const leadStatusVariant = (status: string): "success" | "danger" | "warning" | "info" | "default" => {
    switch (status) {
      case "processed":
        return "success";
      case "received":
        return "info";
      case "failed":
        return "danger";
      case "duplicate":
        return "warning";
      default:
        return "default";
    }
  };

  const formatDate = (dateStr: string | null | undefined) => {
    if (!dateStr) return "-";
    return new Date(dateStr).toLocaleString("pt-BR");
  };

  const getPortalIcon = (icon: string) => {
    switch (icon) {
      case "zap":
        return Zap;
      case "globe":
        return Globe;
      case "home":
        return Building2;
      default:
        return LinkIcon;
    }
  };

  const PortalIcon = getPortalIcon(portalDef.icon);

  return (
    <Dialog open onOpenChange={(next) => { if (!next) onClose(); }}>
      <DialogContent className="flex max-h-[90vh] flex-col gap-0 overflow-hidden p-0 sm:max-w-2xl">
        <div className="flex items-center gap-3 px-6 py-4 border-b border-border pr-12">
          <div className="w-10 h-10 rounded-xl bg-primary-50 text-primary-600 flex items-center justify-center">
            <PortalIcon className="w-5 h-5" />
          </div>
          <div>
            <DialogTitle className="text-base font-bold text-foreground">{portalDef.name}</DialogTitle>
            <DialogDescription className="text-xs">
              {isEditing ? "Configuração da integração" : "Nova integração"}
            </DialogDescription>
          </div>
        </div>

        {isEditing && (
          <div className="flex gap-1 px-6 pt-3 border-b border-border">
            {([
              { key: "config" as const, label: "Configuração", icon: Settings2 },
              { key: "syncs" as const, label: "Histórico", icon: History },
              { key: "leads" as const, label: "Leads", icon: User },
            ]).map(({ key, label, icon: TabIcon }) => (
              <button
                key={key}
                onClick={() => setActiveTab(key)}
                className={clsx(
                  "flex items-center gap-1.5 px-4 py-2.5 text-xs font-bold rounded-t-xl transition-colors -mb-px border-b-2",
                  activeTab === key
                    ? "border-primary-600 text-primary-700 bg-primary-50/50"
                    : "border-transparent text-muted-foreground hover:text-foreground hover:bg-muted",
                )}
              >
                <TabIcon className="w-3.5 h-3.5" />
                {label}
              </button>
            ))}
          </div>
        )}

        <div className="flex-1 overflow-y-auto px-6 py-5">
          {activeTab === "config" && (
            <div className="space-y-6">
              {backendData?.status === "error" && (
                <div className="flex items-start gap-2 bg-status-critical-soft text-status-critical p-3 rounded-xl border border-status-critical/30">
                  <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5" />
                  <div>
                    <p className="text-xs font-bold">
                      Erro na integração ({backendData.consecutive_fails} falha
                      {backendData.consecutive_fails !== 1 ? "s" : ""} consecutiva
                      {backendData.consecutive_fails !== 1 ? "s" : ""})
                    </p>
                    {backendData.error_message && <p className="text-xs mt-1">{backendData.error_message}</p>}
                  </div>
                </div>
              )}

              <div className="space-y-4">
                <h4 className="text-xs font-bold text-muted-foreground uppercase tracking-wider flex items-center gap-1.5">
                  <User className="w-3.5 h-3.5" />
                  Dados de Contato
                </h4>
                <div className="space-y-3">
                  <div className="space-y-1.5">
                    <label htmlFor={contactNameId} className="text-xs font-bold text-foreground">Nome do contato</label>
                    <SettingsInput id={contactNameId} ariaLabel="Nome do contato" value={contactName} onChange={setContactName} placeholder="Nome completo" icon={<User className="w-4 h-4" />} />
                  </div>
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    <div className="space-y-1.5">
                      <label htmlFor={contactEmailId} className="text-xs font-bold text-foreground">E-mail</label>
                      <SettingsInput id={contactEmailId} ariaLabel="E-mail" value={contactEmail} onChange={setContactEmail} placeholder="contato@empresa.com" type="email" icon={<Mail className="w-4 h-4" />} />
                    </div>
                    <div className="space-y-1.5">
                      <label htmlFor={contactPhoneId} className="text-xs font-bold text-foreground">Telefone</label>
                      <SettingsInput id={contactPhoneId} ariaLabel="Telefone" value={formatPhoneBR(contactPhone)} onChange={(v) => setContactPhone(parsePhoneBR(v))} placeholder="(11) 99999-9999" icon={<Phone className="w-4 h-4" />} />
                    </div>
                  </div>
                </div>
              </div>

              <div className="space-y-4">
                <h4 className="text-xs font-bold text-muted-foreground uppercase tracking-wider flex items-center gap-1.5">
                  <RefreshCw className="w-3.5 h-3.5" />
                  Sincronização & Publicação
                </h4>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <div className="space-y-1.5">
                    <label htmlFor={syncFrequencyId} className="text-xs font-bold text-foreground">Frequência de sincronização</label>
                    <SettingsSelect
                      id={syncFrequencyId}
                      ariaLabel="Frequência de sincronização"
                      value={syncFrequency}
                      onChange={(v) => setSyncFrequency(v as typeof syncFrequency)}
                      options={[
                        { value: "6h", label: "A cada 6 horas" },
                        { value: "12h", label: "A cada 12 horas" },
                        { value: "24h", label: "A cada 24 horas" },
                      ]}
                    />
                  </div>
                  <div className="space-y-1.5">
                    <label htmlFor={publicationTypeId} className="text-xs font-bold text-foreground">Tipo de publicação</label>
                    <SettingsSelect
                      id={publicationTypeId}
                      ariaLabel="Tipo de publicação"
                      value={publicationType}
                      onChange={(v) => setPublicationType(v as typeof publicationType)}
                      options={[
                        { value: "STANDARD", label: "Padrão" },
                        { value: "PREMIUM", label: "Premium" },
                        { value: "SUPER_PREMIUM", label: "Super Premium" },
                      ]}
                    />
                  </div>
                </div>
              </div>

              <div className="space-y-4">
                <h4 className="text-xs font-bold text-muted-foreground uppercase tracking-wider flex items-center gap-1.5">
                  <Filter className="w-3.5 h-3.5" />
                  Filtro de Imóveis
                </h4>

                <div className="space-y-3">
                  <div className="space-y-2">
                    <label className="text-xs font-bold text-foreground">Status do imóvel</label>
                    <div role="group" aria-label="Status do imóvel" className="flex flex-wrap gap-2">
                      {PORTAL_PROPERTY_STATUSES.map((opt) => (
                        <button
                          key={opt.value}
                          onClick={() => toggleChip(filterStatuses, setFilterStatuses, opt.value)}
                          className={clsx(
                            "px-2.5 py-1.5 rounded-lg text-[11px] font-bold border transition-colors",
                            filterStatuses.includes(opt.value)
                              ? "bg-primary text-primary-foreground border-primary"
                              : "bg-card text-muted-foreground border-border hover:border-border",
                          )}
                        >
                          {opt.label}
                        </button>
                      ))}
                    </div>
                  </div>

                  <div className="space-y-2">
                    <label className="text-xs font-bold text-foreground">Tipo de imóvel</label>
                    <div role="group" aria-label="Tipo de imóvel" className="flex flex-wrap gap-2">
                      {PORTAL_PROPERTY_TYPES.map((opt) => (
                        <button
                          key={opt.value}
                          onClick={() => toggleChip(filterPropertyTypes, setFilterPropertyTypes, opt.value)}
                          className={clsx(
                            "px-2.5 py-1.5 rounded-lg text-[11px] font-bold border transition-colors",
                            filterPropertyTypes.includes(opt.value)
                              ? "bg-primary text-primary-foreground border-primary"
                              : "bg-card text-muted-foreground border-border hover:border-border",
                          )}
                        >
                          {opt.label}
                        </button>
                      ))}
                    </div>
                  </div>

                  <div className="space-y-2">
                    <label className="text-xs font-bold text-foreground">Tipo de transação</label>
                    <div role="group" aria-label="Tipo de transação" className="flex flex-wrap gap-2">
                      {PORTAL_LISTING_TYPES.map((opt) => (
                        <button
                          key={opt.value}
                          onClick={() => toggleChip(filterListingTypes, setFilterListingTypes, opt.value)}
                          className={clsx(
                            "px-2.5 py-1.5 rounded-lg text-[11px] font-bold border transition-colors",
                            filterListingTypes.includes(opt.value)
                              ? "bg-primary text-primary-foreground border-primary"
                              : "bg-card text-muted-foreground border-border hover:border-border",
                          )}
                        >
                          {opt.label}
                        </button>
                      ))}
                    </div>
                  </div>

                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    <div className="space-y-1.5">
                      <label htmlFor={minPriceId} className="text-xs font-bold text-foreground">Preço mínimo (R$)</label>
                      <div className="relative">
                        <div className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground">
                          <span className="text-xs font-bold">R$</span>
                        </div>
                        <MoneyInput
                          id={minPriceId}
                          aria-label="Preço mínimo"
                          fractionDigits={0}
                          value={minPrice ? Number(minPrice) : undefined}
                          onChange={(v) => setMinPrice(v === undefined ? "" : String(v))}
                          placeholder="0"
                          className="w-full px-3 py-2.5 pl-9 bg-card border border-border rounded-xl text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-400 transition-colors"
                        />
                      </div>
                    </div>
                    <div className="space-y-1.5">
                      <label htmlFor={maxPriceId} className="text-xs font-bold text-foreground">Preço máximo (R$)</label>
                      <div className="relative">
                        <div className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground">
                          <span className="text-xs font-bold">R$</span>
                        </div>
                        <MoneyInput
                          id={maxPriceId}
                          aria-label="Preço máximo"
                          fractionDigits={0}
                          value={maxPrice ? Number(maxPrice) : undefined}
                          onChange={(v) => setMaxPrice(v === undefined ? "" : String(v))}
                          placeholder="Sem limite"
                          className="w-full px-3 py-2.5 pl-9 bg-card border border-border rounded-xl text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-400 transition-colors"
                        />
                      </div>
                    </div>
                  </div>
                </div>
              </div>

              {backendData && (
                <div className="space-y-4">
                  <h4 className="text-xs font-bold text-muted-foreground uppercase tracking-wider flex items-center gap-1.5">
                    <LinkIcon className="w-3.5 h-3.5" />
                    URLs de Integração
                  </h4>
                  <div className="space-y-3">
                    <div className="space-y-1.5">
                      <label htmlFor={feedUrlId} className="text-xs font-bold text-foreground">URL do Feed XML</label>
                      <div className="flex items-center gap-2">
                        <input id={feedUrlId} readOnly value={backendData.feed_url} className="flex-1 px-3 py-2.5 bg-muted border border-border rounded-xl text-xs font-mono text-muted-foreground truncate" />
                        <Button variant="outline" size="icon" aria-label="Copiar URL do feed" onClick={() => handleCopyUrl(backendData.feed_url, "URL do feed")} className="shrink-0 text-muted-foreground hover:text-primary-600">
                          <Copy className="w-4 h-4" />
                        </Button>
                      </div>
                    </div>
                    <div className="space-y-1.5">
                      <label htmlFor={webhookUrlId} className="text-xs font-bold text-foreground">URL do Webhook</label>
                      <div className="flex items-center gap-2">
                        <input id={webhookUrlId} readOnly value={backendData.webhook_url} className="flex-1 px-3 py-2.5 bg-muted border border-border rounded-xl text-xs font-mono text-muted-foreground truncate" />
                        <Button variant="outline" size="icon" aria-label="Copiar URL do webhook" onClick={() => handleCopyUrl(backendData.webhook_url, "URL do webhook")} className="shrink-0 text-muted-foreground hover:text-primary-600">
                          <Copy className="w-4 h-4" />
                        </Button>
                      </div>
                    </div>
                  </div>
                </div>
              )}
            </div>
          )}

          {activeTab === "syncs" && (
            <div className="space-y-4">
              {loadingSyncs ? (
                <div className="flex items-center justify-center py-12">
                  <span className="w-6 h-6 border-2 border-primary-200 border-t-primary-600 rounded-full animate-spin" />
                </div>
              ) : syncHistory.length === 0 ? (
                <div className="text-center py-12">
                  <History className="w-8 h-8 text-muted-foreground/60 mx-auto mb-2" />
                  <p className="text-sm text-muted-foreground">Nenhuma sincronização realizada.</p>
                </div>
              ) : (
                <>
                  <p className="text-xs text-muted-foreground">
                    {syncHistoryTotal} sincronização{syncHistoryTotal !== 1 ? "es" : ""} no total
                  </p>
                  <div className="overflow-x-auto rounded-xl border border-border">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="bg-muted text-left">
                          <th className="px-4 py-2.5 text-[11px] font-bold text-muted-foreground uppercase">Status</th>
                          <th className="px-4 py-2.5 text-[11px] font-bold text-muted-foreground uppercase">Iniciado em</th>
                          <th className="px-4 py-2.5 text-[11px] font-bold text-muted-foreground uppercase">Concluido em</th>
                          <th className="px-4 py-2.5 text-[11px] font-bold text-muted-foreground uppercase text-right">Sincronizados</th>
                          <th className="px-4 py-2.5 text-[11px] font-bold text-muted-foreground uppercase text-right">Falhas</th>
                        </tr>
                      </thead>
                      <tbody>
                        {syncHistory.map((sync) => (
                          <tr key={sync.id} className="border-t border-border">
                            <td className="px-4 py-2.5">
                              <Badge text={syncStatusLabel(sync.status)} variant={syncStatusVariant(sync.status)} />
                            </td>
                            <td className="px-4 py-2.5 text-xs text-muted-foreground">{formatDate(sync.started_at)}</td>
                            <td className="px-4 py-2.5 text-xs text-muted-foreground">{formatDate(sync.completed_at)}</td>
                            <td className="px-4 py-2.5 text-xs text-foreground font-bold text-right">{sync.properties_synced}</td>
                            <td className="px-4 py-2.5 text-xs text-right">
                              <span className={sync.properties_failed > 0 ? "text-status-critical font-bold" : "text-muted-foreground"}>
                                {sync.properties_failed}
                              </span>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </>
              )}
            </div>
          )}

          {activeTab === "leads" && (
            <div className="space-y-4">
              {loadingLeads ? (
                <div className="flex items-center justify-center py-12">
                  <span className="w-6 h-6 border-2 border-primary-200 border-t-primary-600 rounded-full animate-spin" />
                </div>
              ) : leads.length === 0 ? (
                <div className="text-center py-12">
                  <User className="w-8 h-8 text-muted-foreground/60 mx-auto mb-2" />
                  <p className="text-sm text-muted-foreground">Nenhum lead recebido deste portal.</p>
                </div>
              ) : (
                <>
                  <p className="text-xs text-muted-foreground">
                    {leadsTotal} lead{leadsTotal !== 1 ? "s" : ""} no total
                  </p>
                  <div className="overflow-x-auto rounded-xl border border-border">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="bg-muted text-left">
                          <th className="px-4 py-2.5 text-[11px] font-bold text-muted-foreground uppercase">Nome</th>
                          <th className="px-4 py-2.5 text-[11px] font-bold text-muted-foreground uppercase">E-mail</th>
                          <th className="px-4 py-2.5 text-[11px] font-bold text-muted-foreground uppercase">Telefone</th>
                          <th className="px-4 py-2.5 text-[11px] font-bold text-muted-foreground uppercase">Status</th>
                          <th className="px-4 py-2.5 text-[11px] font-bold text-muted-foreground uppercase">Recebido em</th>
                        </tr>
                      </thead>
                      <tbody>
                        {leads.map((lead) => (
                          <tr key={lead.id} className="border-t border-border">
                            <td className="px-4 py-2.5 text-xs text-foreground font-bold">{lead.name}</td>
                            <td className="px-4 py-2.5 text-xs text-muted-foreground">{lead.email}</td>
                            <td className="px-4 py-2.5 text-xs text-muted-foreground">{formatPhoneBR(lead.phone)}</td>
                            <td className="px-4 py-2.5">
                              <Badge text={leadStatusLabel(lead.status)} variant={leadStatusVariant(lead.status)} />
                            </td>
                            <td className="px-4 py-2.5 text-xs text-muted-foreground">{formatDate(lead.received_at)}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </>
              )}
            </div>
          )}
        </div>

        <div className="flex items-center justify-end gap-2 px-6 py-4 border-t border-border bg-muted/50">
          <Button variant="outline" onClick={onClose}>
            Cancelar
          </Button>
          {activeTab === "config" && (
            <Button onClick={handleSave} loading={saving} loadingText="Salvando...">
              <Save className="w-4 h-4" />
              {isEditing ? "Salvar Alterações" : "Conectar Portal"}
            </Button>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
