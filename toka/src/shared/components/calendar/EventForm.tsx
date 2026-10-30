import { useEffect, useId, useMemo, useState } from "react";
import { clsx } from "clsx";
import { Check, ChevronDown, ChevronsUpDown, Loader2, UserPlus, X } from "lucide-react";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandSeparator } from "../../ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "../../ui/popover";
import { PropertyPicker } from "../PropertyPicker";
import { dealOptionLabel } from "../../lib/deals/dealOptionLabel";
import { leadService, type ApiLead } from "../../services/lead.service";
import { dealService, type DealDTO } from "../../services/deal.service";
import { propertyService, type PropertyDTO } from "../../services/property.service";
import { formatPhoneBR } from "../../utils/masks";
import { ALL_TYPES, PRIORITY_CONFIG, STATUS_CONFIG, TYPE_CONFIG } from "./appointmentOptions";
import type { Appointment, AppointmentStatus } from "../../data";
import {
  END_BEFORE_START,
  appointmentFieldErrors,
  earliestAppointmentDate,
  type AppointmentFieldErrors,
  type AppointmentFormValues,
} from "./appointmentForm";

/**
 * Formulário de compromisso da agenda — extraído de `pages/Calendar` para ser o
 * MESMO formulário em todo lugar que agenda (a agenda completa e o detalhe do
 * lead). Campos, combos de cliente/imóvel/negócio, validação e payload são os
 * mesmos: quem agenda pelo lead não vê uma versão reduzida.
 */

export function EventForm({
  form,
  updateField,
  agents,
  properties,
  leads,
  editing,
}: {
  form: AppointmentFormValues;
  updateField: (f: string, v: any) => void;
  agents: any[];
  properties: any[];
  leads: any[];
  /** Compromisso em edição: reagendar um que já começou é permitido. */
  editing?: Appointment | null;
}) {
  const [searchLeadTerm, setSearchLeadTerm] = useState("");
  const [suggestedLeads, setSuggestedLeads] = useState<ApiLead[]>([]);
  const [searchingLeads, setSearchingLeads] = useState(false);
  const [manualEntryMode, setManualEntryMode] = useState(false);
  const [selectedLead, setSelectedLead] = useState<ApiLead | any | null>(null);
  const fieldId = useId();
  const [leadComboOpen, setLeadComboOpen] = useState(false);

  const [selectedProperty, setSelectedProperty] = useState<PropertyDTO | null>(null);

  const [searchDealTerm, setSearchDealTerm] = useState("");
  const [suggestedDeals, setSuggestedDeals] = useState<DealDTO[]>([]);
  const [searchingDeals, setSearchingDeals] = useState(false);
  const [dealComboOpen, setDealComboOpen] = useState(false);
  const [selectedDeal, setSelectedDeal] = useState<DealDTO | null>(null);

  const propertyOptions = useMemo(
    () =>
      (Array.isArray(properties) ? properties : []).map((p: any) => ({
        id: p.id,
        title: p.title,
        displayId: p.displayId || p.display_id,
        address: p.address,
      })),
    [properties],
  );

  useEffect(() => {
    if (form.propertyId && !selectedProperty) {
      propertyService
        .getById(form.propertyId)
        .then((p) => {
          if (p.data) setSelectedProperty(p.data as any);
        })
        .catch((err) => console.error("Error loading property:", err));
    }
    if (form.contractId && !selectedDeal) {
      dealService
        .getById(form.contractId)
        .then((d) => {
          if (d.data) setSelectedDeal(d.data as any);
        })
        .catch((err) => console.error("Error loading deal:", err));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- carrega o imóvel/negócio vinculados uma única vez, na abertura
  }, []);

  useEffect(() => {
    if (form.client && !selectedLead && !manualEntryMode) {
      setManualEntryMode(true);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- só o cliente decide entrar em modo manual
  }, [form.client]);

  useEffect(() => {
    if (!leadComboOpen) return;

    if (!searchLeadTerm || searchLeadTerm.length < 2) {
      if (suggestedLeads.length === 0) {
        setSearchingLeads(true);
        leadService
          .getAll({ limit: 10 })
          .then((data) => setSuggestedLeads(data.data || []))
          .catch((err) => console.error("Error loading leads:", err))
          .finally(() => setSearchingLeads(false));
      }
      return;
    }

    const timer = setTimeout(() => {
      setSearchingLeads(true);
      leadService
        .getAll({ search: searchLeadTerm, limit: 10 })
        .then((data) => setSuggestedLeads(data.data || []))
        .catch((err) => console.error("Error searching leads:", err))
        .finally(() => setSearchingLeads(false));
    }, 300);

    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- busca no servidor por termo; o comprimento é detalhe interno
  }, [searchLeadTerm, leadComboOpen]);

  useEffect(() => {
    if (!dealComboOpen) return;

    if (!searchDealTerm || searchDealTerm.length < 2) {
      if (suggestedDeals.length === 0) {
        setSearchingDeals(true);
        dealService
          .getAll()
          .then((data) => setSuggestedDeals(data.data.data || []))
          .catch((err) => console.error("Error loading deals:", err))
          .finally(() => setSearchingDeals(false));
      }
      return;
    }

    const timer = setTimeout(() => {
      setSearchingDeals(true);
      dealService
        .getAll()
        .then((data) => {
          const deals: DealDTO[] = data.data.data || [];
          const term = searchDealTerm.toLowerCase();
          const filtered = deals.filter((d) => dealLabelFor(d).searchText.includes(term));
          setSuggestedDeals(filtered);
        })
        .catch((err) => console.error("Error searching deals:", err))
        .finally(() => setSearchingDeals(false));
    }, 300);

    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- busca no servidor por termo; o comprimento é detalhe interno
  }, [searchDealTerm, dealComboOpen]);

  const handleSelectLead = (lead: ApiLead) => {
    setSelectedLead(lead);
    updateField("client", lead.name);
    updateField("leadId", lead.id);
    setLeadComboOpen(false);
  };

  const handleClearSelection = () => {
    setSelectedLead(null);
    updateField("client", "");
    updateField("leadId", "");
    setManualEntryMode(false);
  };

  const handleEnterManualMode = () => {
    setManualEntryMode(true);
    setSelectedLead(null);
    updateField("leadId", "");
    setLeadComboOpen(false);
  };

  const handleClearProperty = () => {
    setSelectedProperty(null);
    updateField("propertyId", "");
  };

  const handleSelectDeal = (deal: DealDTO) => {
    setSelectedDeal(deal);
    updateField("contractId", deal.id);
    updateField("propertyTitle", deal.property_id || deal.display_id);
    setDealComboOpen(false);
  };

  const handleClearDeal = () => {
    setSelectedDeal(null);
    updateField("contractId", "");
  };

  const leadRoleToBadgeColor = (role?: string): string => {
    switch (role) {
      case "owner":
        return "bg-stage-contract/10 text-stage-contract";
      case "tenant":
        return "bg-status-info-soft text-status-info";
      case "buyer":
        return "bg-status-ok-soft text-status-ok";
      default:
        return "bg-muted text-muted-foreground";
    }
  };

  // O rótulo do negócio é montado com dados identificáveis (imóvel, cliente,
  // tipo, valor, etapa) — antes o dropdown mostrava `display_id || id`, ou seja
  // um UUID, quando o negócio não tinha display_id.
  const dealLabelContext = (deal: DealDTO) => {
    const property = properties.find((p: any) => p.id === deal.property_id);
    const customer = leads.find(
      (l: any) => l.id === deal.customer_id || (deal.lead_id && l.id === deal.lead_id),
    );
    const owner = leads.find((l: any) => l.id === deal.owner_id);
    return {
      propertyTitle: (property as any)?.title || "",
      customerName: (customer as any)?.name || "",
      ownerName: (owner as any)?.name || "",
    };
  };

  const dealLabelFor = (deal: DealDTO) => dealOptionLabel({ deal, ...dealLabelContext(deal) });

  const selectedLeadLabel = selectedLead?.name || form.client || "";

  const selectedDealLabel = selectedDeal
    ? dealLabelFor(selectedDeal).full
    : form.contractId
      ? form.contractId
      : "";

  const selectedPropertyLabel = selectedProperty
    ? selectedProperty.display_id
      ? `${selectedProperty.display_id} - ${selectedProperty.title}`
      : selectedProperty.title
    : form.propertyId
      ? form.propertyId
      : "";

  const selectedAgent = agents?.find((a: any) => a.id === form.agentId);

  // Validação campo a campo (mesma regra do salvamento). O erro só aparece
  // depois que o operador passou pelo campo — abrir o formulário já "vermelho"
  // é ruído, e o operador ainda não fez nada de errado.
  const [touched, setTouched] = useState<Record<string, boolean>>({});
  const fieldErrors = useMemo(() => appointmentFieldErrors(form, { editing: editing ?? null }), [editing, form]);
  const errorFor = (field: keyof AppointmentFieldErrors) => (touched[field] ? fieldErrors[field] : undefined);
  const markTouched = (field: string) => setTouched((prev) => (prev[field] ? prev : { ...prev, [field]: true }));
  const minDate = useMemo(() => earliestAppointmentDate({ editing: editing ?? null }), [editing]);

  const fieldClass = (field: keyof AppointmentFieldErrors, extra = "") =>
    clsx(
      "w-full px-4 py-3 bg-muted border rounded-xl text-sm focus:bg-card focus:ring-2 outline-none",
      errorFor(field) ? "border-destructive focus:ring-destructive/20" : "border-border focus:ring-primary-500",
      extra,
    );

  return (
    <div className="px-6 py-5 space-y-6">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div className="sm:col-span-2">
          <label htmlFor={`${fieldId}-title`} className="block text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">
            Título *
          </label>
          <input
            id={`${fieldId}-title`}
            value={form.title}
            onChange={(e) => updateField("title", e.target.value)}
            onBlur={() => markTouched("title")}
            aria-invalid={errorFor("title") ? true : undefined}
            aria-describedby={errorFor("title") ? `${fieldId}-title-error` : undefined}
            className={fieldClass("title")}
            placeholder="Ex: Visita ao imóvel"
          />
          {errorFor("title") && (
            <p id={`${fieldId}-title-error`} role="alert" className="mt-1 text-xs text-destructive">
              {errorFor("title")}
            </p>
          )}
        </div>

        <label className="sm:col-span-2 flex items-start gap-3 rounded-xl border border-border bg-muted/40 p-4 cursor-pointer">
          <input
            type="checkbox"
            aria-label="Compromisso particular"
            checked={form.visibility === "private"}
            onChange={(event) => updateField("visibility", event.target.checked ? "private" : "team")}
            className="mt-0.5 h-4 w-4 rounded border-border text-primary-600"
          />
          <span>
            <span className="block text-sm font-semibold text-foreground">Compromisso particular</span>
            <span className="block text-xs text-muted-foreground">Visível somente para o corretor responsável, inclusive para gestores.</span>
          </span>
        </label>

        <div>
          <label htmlFor={`${fieldId}-date`} className="block text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">
            Data *
          </label>
          <input
            id={`${fieldId}-date`}
            type="date"
            min={minDate}
            value={form.date}
            onChange={(e) => updateField("date", e.target.value)}
            onBlur={() => {
              markTouched("date");
              // O fim é revalidado junto: a data pode ter acabado de tornar o
              // horário inválido (ou voltar a ser válido).
              markTouched("time");
              markTouched("endTime");
            }}
            aria-invalid={errorFor("date") ? true : undefined}
            aria-describedby={errorFor("date") ? `${fieldId}-date-error` : undefined}
            className={fieldClass("date")}
          />
          {errorFor("date") && (
            <p id={`${fieldId}-date-error`} role="alert" className="mt-1 text-xs text-destructive">
              {errorFor("date")}
            </p>
          )}
        </div>

        <div className="grid grid-cols-2 gap-2">
          <div>
            <label htmlFor={`${fieldId}-time`} className="block text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">
              Início *
            </label>
            <input
              id={`${fieldId}-time`}
              type="time"
              value={form.time}
              onChange={(e) => {
                const newTime = e.target.value;
                updateField("time", newTime);
                // Auto-fill fim = início + 1h como padrão
                if (newTime) {
                  const [h, m] = newTime.split(":").map(Number);
                  const endH = (h + 1) % 24;
                  updateField("endTime", `${String(endH).padStart(2, "0")}:${String(m).padStart(2, "0")}`);
                }
              }}
              onBlur={() => {
                markTouched("time");
                markTouched("endTime");
              }}
              aria-invalid={errorFor("time") ? true : undefined}
              aria-describedby={errorFor("time") ? `${fieldId}-time-error` : undefined}
              className={fieldClass("time")}
            />
            {errorFor("time") && (
              <p id={`${fieldId}-time-error`} role="alert" className="mt-1 text-xs text-destructive">
                {errorFor("time")}
              </p>
            )}
          </div>
          <div>
            <label htmlFor={`${fieldId}-endTime`} className="block text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">
              Fim
            </label>
            <input
              id={`${fieldId}-endTime`}
              type="time"
              value={form.endTime}
              onChange={(e) => updateField("endTime", e.target.value)}
              onBlur={() => markTouched("endTime")}
              aria-invalid={errorFor("endTime") ? true : undefined}
              aria-describedby={errorFor("endTime") ? `${fieldId}-endTime-error` : undefined}
              className={clsx(
                "w-full px-4 py-3 bg-muted border rounded-xl text-sm focus:bg-card focus:ring-2 outline-none",
                // Avisa enquanto digita (sem esperar o blur): o operador vê o
                // conflito na hora em que escolhe o horário.
                errorFor("endTime") || (form.time && form.endTime && form.time >= form.endTime)
                  ? "border-destructive focus:ring-destructive/20"
                  : "border-border focus:ring-primary-500",
              )}
            />
            {(errorFor("endTime") || (touched.endTime && form.time && form.endTime && form.time >= form.endTime)) && (
              <p id={`${fieldId}-endTime-error`} role="alert" className="mt-1 text-xs text-destructive">
                {END_BEFORE_START}
              </p>
            )}
          </div>
        </div>

        <div className="sm:col-span-2">
          <label htmlFor={`${fieldId}-client`} className="block text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">
            Cliente{form.visibility === "private" ? " (opcional)" : " *"}
          </label>
          <div className="space-y-2">
            <Popover open={leadComboOpen} onOpenChange={setLeadComboOpen}>
              <PopoverTrigger asChild>
                <button
                  id={`${fieldId}-client`}
                  type="button"
                  className="w-full px-4 py-3 bg-muted border border-border rounded-xl text-sm flex items-center justify-between focus:bg-card focus:ring-2 focus:ring-primary-500 outline-none"
                >
                  <span className={clsx("truncate", !selectedLeadLabel && "text-muted-foreground")}>
                    {selectedLeadLabel || "Selecione um lead/cliente"}
                  </span>
                  <ChevronsUpDown className="w-4 h-4 text-muted-foreground opacity-60" />
                </button>
              </PopoverTrigger>
              <PopoverContent
                className="w-[var(--radix-popover-trigger-width)] p-0 bg-card border border-border shadow-xl rounded-lg z-50"
                align="start"
              >
                <Command>
                  <CommandInput
                    placeholder="Buscar lead/cliente..."
                    value={searchLeadTerm}
                    onValueChange={setSearchLeadTerm}
                  />
                  <CommandList className="max-h-[240px] overflow-y-auto">
                    <CommandEmpty className="py-3 text-sm text-muted-foreground text-center">
                      Nenhum lead encontrado.
                    </CommandEmpty>
                    <CommandGroup heading="Sugestões">
                      {searchingLeads ? (
                        <div className="p-3 text-sm text-muted-foreground flex items-center gap-2">
                          <Loader2 className="w-4 h-4 animate-spin" /> Buscando...
                        </div>
                      ) : (
                        suggestedLeads.map((lead) => (
                          <CommandItem
                            key={lead.id}
                            value={lead.name}
                            onSelect={() => handleSelectLead(lead)}
                            className="cursor-pointer flex items-center justify-between"
                          >
                            <span className="truncate">{lead.name}</span>
                            <span
                              className={clsx(
                                "text-micro font-bold uppercase tracking-wider px-2 py-0.5 rounded",
                                leadRoleToBadgeColor((lead as any).role),
                              )}
                            >
                              {(lead as any).role || "Lead"}
                            </span>
                          </CommandItem>
                        ))
                      )}
                    </CommandGroup>
                    <CommandSeparator />
                    <CommandGroup heading="Ações">
                      <CommandItem onSelect={handleEnterManualMode} className="cursor-pointer">
                        <UserPlus className="w-4 h-4 mr-2" /> Digitar manualmente
                      </CommandItem>
                      {selectedLeadLabel ? (
                        <CommandItem onSelect={handleClearSelection} className="cursor-pointer">
                          <X className="w-4 h-4 mr-2" /> Limpar seleção
                        </CommandItem>
                      ) : null}
                    </CommandGroup>
                  </CommandList>
                </Command>
              </PopoverContent>
            </Popover>

            {manualEntryMode ? (
              <input
                aria-label="Nome do cliente (entrada manual)"
                value={form.client}
                onChange={(e) => updateField("client", e.target.value)}
                className="w-full px-4 py-3 bg-muted border border-border rounded-xl text-sm focus:bg-card focus:ring-2 focus:ring-primary-500 outline-none"
                placeholder="Nome do cliente"
              />
            ) : null}
          </div>
        </div>

        <div>
          <label htmlFor={`${fieldId}-type`} className="block text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">
            Tipo
          </label>
          <select
            id={`${fieldId}-type`}
            value={form.type}
            onChange={(e) => updateField("type", e.target.value)}
            className="w-full px-4 py-3 bg-muted border border-border rounded-xl text-sm focus:bg-card focus:ring-2 focus:ring-primary-500 outline-none"
          >
            {!ALL_TYPES.includes(form.type) && <option value={form.type}>{form.type}</option>}
            {ALL_TYPES.map((t) => (
              <option key={t} value={t}>
                {TYPE_CONFIG[t]?.label ?? t}
              </option>
            ))}
          </select>
        </div>

        <div>
          <label htmlFor={`${fieldId}-status`} className="block text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">
            Status
          </label>
          <select
            id={`${fieldId}-status`}
            value={form.status}
            onChange={(e) => updateField("status", e.target.value)}
            className="w-full px-4 py-3 bg-muted border border-border rounded-xl text-sm focus:bg-card focus:ring-2 focus:ring-primary-500 outline-none"
          >
            {!(form.status in STATUS_CONFIG) && <option value={form.status}>{form.status}</option>}
            {Object.keys(STATUS_CONFIG).map((s) => (
              <option key={s} value={s}>
                {STATUS_CONFIG[s as AppointmentStatus].label}
              </option>
            ))}
          </select>
        </div>

        <div className="sm:col-span-2">
          <label htmlFor={`${fieldId}-agent`} className="block text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">
            Corretor
          </label>
          <Popover>
            <PopoverTrigger asChild>
              <button
                id={`${fieldId}-agent`}
                type="button"
                className="w-full px-4 py-3 bg-muted border border-border rounded-xl text-sm flex items-center justify-between focus:bg-card focus:ring-2 focus:ring-primary-500 outline-none"
              >
                <span className={clsx("truncate", !selectedAgent && "text-muted-foreground")}>
                  {selectedAgent ? selectedAgent.name || selectedAgent.fullname || selectedAgent.email : "Selecione um corretor"}
                </span>
                <ChevronDown className="w-4 h-4 text-muted-foreground opacity-60" />
              </button>
            </PopoverTrigger>
            <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0 bg-card border border-border shadow-xl rounded-lg z-50" align="start">
              <Command>
                <CommandInput placeholder="Buscar corretor..." />
                <CommandList className="max-h-[240px] overflow-y-auto">
                  <CommandEmpty className="py-3 text-sm text-muted-foreground text-center">
                    Nenhum corretor encontrado.
                  </CommandEmpty>
                  <CommandGroup heading="Corretores">
                    {(agents || []).map((a: any) => (
                      <CommandItem
                        key={a.id}
                        value={a.name || a.fullname || a.email}
                        onSelect={() => updateField("agentId", a.id)}
                        className="cursor-pointer"
                      >
                        <span className="truncate">{a.name || a.fullname || a.email}</span>
                      </CommandItem>
                    ))}
                  </CommandGroup>
                </CommandList>
              </Command>
            </PopoverContent>
          </Popover>
        </div>

        <div className="sm:col-span-2">
          <label htmlFor={`${fieldId}-property`} className="block text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">
            Imóvel (opcional)
          </label>
          <div className="flex items-center gap-2">
            {/*
              `min-w-0` é obrigatório: sem ele um item flex não encolhe abaixo do
              conteúdo (min-width: auto), o `truncate` do seletor nunca age e o
              rótulo longo do imóvel empurra o botão de remover para fora do
              painel — o corpo rolável ganhava scroll horizontal no mobile.
            */}
            <div className="flex-1 min-w-0">
              <PropertyPicker
                id={`${fieldId}-property`}
                properties={propertyOptions}
                value={form.propertyId}
                includeNone
                placeholder="Selecione um imóvel..."
                inputPlaceholder="Buscar por código ou título..."
                onChange={(id) => updateField("propertyId", id)}
              />
            </div>
            {selectedPropertyLabel ? (
              <button
                type="button"
                aria-label="Remover imóvel vinculado"
                onClick={handleClearProperty}
                className="p-2 rounded-lg border border-border bg-card text-muted-foreground hover:bg-muted"
              >
                <X className="w-4 h-4" />
              </button>
            ) : null}
          </div>
        </div>

        <div className="sm:col-span-2">
          <label htmlFor={`${fieldId}-deal`} className="block text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">
            Negócio (opcional)
          </label>
          <Popover open={dealComboOpen} onOpenChange={setDealComboOpen}>
            <PopoverTrigger asChild>
              <button
                id={`${fieldId}-deal`}
                type="button"
                className="w-full px-4 py-3 bg-muted border border-border rounded-xl text-sm flex items-center justify-between focus:bg-card focus:ring-2 focus:ring-primary-500 outline-none"
              >
                <span className={clsx("truncate", !selectedDealLabel && "text-muted-foreground")}>
                  {selectedDealLabel || "Selecione um negócio"}
                </span>
                <ChevronsUpDown className="w-4 h-4 text-muted-foreground opacity-60" />
              </button>
            </PopoverTrigger>
            <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0 bg-card border border-border shadow-xl rounded-lg z-50" align="start">
              <Command>
                <CommandInput
                  placeholder="Buscar negócio..."
                  value={searchDealTerm}
                  onValueChange={setSearchDealTerm}
                />
                <CommandList className="max-h-[240px] overflow-y-auto">
                  <CommandEmpty className="py-3 text-sm text-muted-foreground text-center">
                    Nenhum negócio encontrado.
                  </CommandEmpty>
                  <CommandGroup heading="Sugestões">
                    {searchingDeals ? (
                      <div className="p-3 text-sm text-muted-foreground flex items-center gap-2">
                        <Loader2 className="w-4 h-4 animate-spin" /> Buscando...
                      </div>
                    ) : (
                      suggestedDeals.map((d) => {
                        const label = dealLabelFor(d);
                        return (
                          <CommandItem
                            key={d.id}
                            value={label.full}
                            onSelect={() => handleSelectDeal(d)}
                            className="cursor-pointer"
                          >
                            <div className="flex min-w-0 flex-col">
                              <span className="truncate font-medium">{label.title}</span>
                              <span className="truncate text-xs text-muted-foreground">
                                {label.detail}
                              </span>
                            </div>
                          </CommandItem>
                        );
                      })
                    )}
                  </CommandGroup>
                  {selectedDealLabel ? (
                    <>
                      <CommandSeparator />
                      <CommandGroup heading="Ações">
                        <CommandItem onSelect={handleClearDeal} className="cursor-pointer">
                          <X className="w-4 h-4 mr-2" /> Limpar seleção
                        </CommandItem>
                      </CommandGroup>
                    </>
                  ) : null}
                </CommandList>
              </Command>
            </PopoverContent>
          </Popover>
        </div>

        {/* Local e anotações já existiam no compromisso e apareciam na lista e
            nos detalhes, mas nenhum formulário os editava — nem a agenda. */}
        <div className="sm:col-span-2">
          <label htmlFor={`${fieldId}-location`} className="block text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">
            Local
          </label>
          <input
            id={`${fieldId}-location`}
            value={form.location}
            onChange={(e) => updateField("location", e.target.value)}
            placeholder="Ex: Rua das Flores, 100 — ou vídeo-chamada"
            className="w-full px-4 py-3 bg-muted border border-border rounded-xl text-sm focus:bg-card focus:ring-2 focus:ring-primary-500 outline-none"
          />
        </div>

        <div className="sm:col-span-2">
          <label htmlFor={`${fieldId}-notes`} className="block text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">
            Anotações
          </label>
          <textarea
            id={`${fieldId}-notes`}
            rows={3}
            value={form.notes}
            onChange={(e) => updateField("notes", e.target.value)}
            placeholder="O que combinar com o cliente nesta conversa"
            className="w-full px-4 py-3 bg-muted border border-border rounded-xl text-sm focus:bg-card focus:ring-2 focus:ring-primary-500 outline-none resize-none"
          />
        </div>
      </div>
    </div>
  );
}
