import { useEffect, useMemo, useRef, useState } from "react";
import { useForm, Controller } from "react-hook-form";
import { Share2, X, ChevronsUpDown, Check, UserPlus } from "lucide-react";
import { toast } from "sonner";
import type { DealStage, EsteiraDeal } from "../../../shared/deals/types";
import { useEsteiraDeals } from "../hooks/useEsteiraDeals";
import { useDealsContext } from "../../../shared/context/DealsContext";
import { useLeadsContext } from "../../../shared/context/LeadsContext";
import { usePropertiesContext } from "../../../shared/context/PropertiesContext";
import { useCurrentUser } from "../../../shared/context/CurrentUserContext";
import { useAuth } from "../../../shared/context/AuthContext";
import { getActiveCompanyId } from "../../../shared/utils/tenant";
import { useRequestScopeGuard } from "../../../shared/hooks/useRequestScopeGuard";
import { flags } from "../../../shared/config/flags";
import { ExpandableSidePanel } from "../../../shared/components/ExpandableSidePanel";
import { LAYER } from "../../../shared/ui/layers";
import { ConfirmDialog } from "../../../shared/components/ConfirmDialog";
import { Button } from "../../../shared/ui/button";
import { Checkbox } from "../../../shared/ui/checkbox";
import { Input } from "../../../shared/ui/input";
import { Label } from "../../../shared/ui/label";
import { Command, CommandEmpty, CommandInput, CommandItem, CommandList } from "../../../shared/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "../../../shared/ui/popover";
import { MoneyInput } from "../../../shared/ui/money-input";
import { PropertyPicker } from "../../../shared/components/PropertyPicker";
import { PropertyModal } from "../../../shared/components/PropertyModal";
import { partnershipService, type Partnership } from "../../../shared/services/partnership.service";
import { getStageMetaByLabel } from "../../../shared/deals/dealStages";
import { useQueryClient } from "@tanstack/react-query";
import { dealQueryKey } from "../../../shared/deals/queries";
import { moveDealErrorMessage } from "../utils/moveDealError";
import { buildDealBrokerParticipations } from "../utils/dealBrokerParticipations";
import { commissionRulesService } from "../../../shared/services/commissionRules.service";
import type { CreateDealDTO, CreatePartyDTO, DealDTO } from "../../../shared/services/deal.service";

interface EsteiraDealModalProps {
 open: boolean;
 onClose: () => void;
 onSuccess?: (data: EsteiraDealFormData) => void | Promise<void>;
 dealToEdit?: EsteiraDeal | null;
 initialLeadId?: string;
 initialPropertyTitle?: string;
 /**
  * Onda 9 — id do imóvel pré-selecionado. Tem precedência sobre
  * `initialPropertyTitle` (mais confiável: títulos podem colidir).
  * Usado pelo fluxo "Iniciar negócio" de um imóvel compartilhado.
  */
 initialPropertyId?: string;
}

export interface EsteiraDealFormData {
 client: string;
 property: string;
 type: 'Venda' | 'Locação';
 value: number;
 stage: DealStage;
 avatarUrl: string;
}

/**
 * Normaliza um trecho da chave de idempotência para ASCII: remove diacríticos
 * (NFD), descarta símbolos e limita o tamanho. Navegadores enviam headers em
 * Latin-1; caracteres acentuados em nomes de cliente/imóvel chegavam como
 * bytes não-UTF8 e o Postgres rejeitava com SQLSTATE 22021 na criação do
 * negócio. "Criação" → "Criacao" mantém a mesma intenção de forma estável.
 */
export function asciiKeyPart(value: string): string {
  return value
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .replace(/[^A-Za-z0-9._:-]/g, "")
    .slice(0, 80);
}

/** Monta a chave de intenção de criação de negócio (ASCII, determinística). */
export function buildCreateIntentKey(company: string, data: EsteiraDealFormData): string {
  const value = Number(data.value ?? 0).toFixed(2);
  return `deal-create:${asciiKeyPart(company)}:${asciiKeyPart(data.property)}:${asciiKeyPart(data.client)}:${asciiKeyPart(data.type)}:${value}`;
}

export function buildEsteiraDealFormDefaults({
 dealToEdit,
 initialLeadId,
 resolvedInitialPropertyId,
}: {
 dealToEdit?: EsteiraDeal | null;
 initialLeadId?: string;
 resolvedInitialPropertyId?: string;
}): EsteiraDealFormData {
 if (dealToEdit) {
  return {
   client: dealToEdit.clientId || dealToEdit.client,
   property: dealToEdit.propertyId || dealToEdit.property,
   type: dealToEdit.type,
   value: dealToEdit.value,
   stage: dealToEdit.stage,
   avatarUrl: dealToEdit.avatarUrl,
  };
 }
 return {
  client: initialLeadId || '',
  property: resolvedInitialPropertyId || '',
  type: 'Venda',
  value: 0,
  stage: 'Em Negociação',
  avatarUrl: '',
 };
}

const ACTIVE_DEAL_STAGES: ReadonlySet<DealStage> = new Set<DealStage>([
 'Em Negociação',
 'Proposta',
 'Análise',
 'Contrato',
]);

export function EsteiraDealModal({ open, onClose, onSuccess, dealToEdit, initialLeadId, initialPropertyTitle, initialPropertyId }: EsteiraDealModalProps) {
 const { userId } = useCurrentUser();
 const { user } = useAuth();
 const tenantId = getActiveCompanyId(user);
 const beginScopedRequest = useRequestScopeGuard(tenantId ?? "");
 const { createDeal, updateDealStatus } = useDealsContext();
 const queryClient = useQueryClient();
 const { leads } = useLeadsContext();
 const { properties } = usePropertiesContext();
 // Sem imóvel cadastrado não há negócio: o seletor abre o MESMO cadastro do
 // menu Imóveis (em vez de mandar o operador procurar a tela) e o imóvel
 // recém-criado já entra selecionado.
 const [isCreatingProperty, setIsCreatingProperty] = useState(false);
 const { deals } = useEsteiraDeals();

 // Resolve the pre-selected property id. An explicit `initialPropertyId`
 // wins; otherwise fall back to matching `initialPropertyTitle`.
 const resolvedInitialPropertyId =
 initialPropertyId ||
 (initialPropertyTitle
 ? properties.find(p => p.title.toLowerCase() === initialPropertyTitle.toLowerCase())?.id || ''
 : '');

 const formDefaults = useMemo(
  () => buildEsteiraDealFormDefaults({ dealToEdit, initialLeadId, resolvedInitialPropertyId }),
  [dealToEdit, initialLeadId, resolvedInitialPropertyId],
 );
 const {
  register,
  handleSubmit,
  setValue,
  watch,
  control,
  reset,
  formState: { errors, isDirty, isSubmitting },
 } = useForm<EsteiraDealFormData>({
  defaultValues: formDefaults,
 });

 const watchType = watch("type");

 const watchClient = watch("client");
 const watchProperty = watch("property");

 // Regras de comissão por papel do tenant: definem o rateio inicial de uma
 // VENDA (captador / fechamento / imobiliária). Sem elas, o negócio nasce sem
 // rateio e o pipeline financeiro adia a materialização, em vez de dar 100% da
 // comissão ao captador — o defeito que originou este fluxo.
 const [rolePercentages, setRolePercentages] = useState<Record<string, number>>({});
 useEffect(() => {
  if (!open || dealToEdit) return;
  let cancelled = false;
  commissionRulesService
   .get()
   .then((rules) => {
    if (cancelled) return;
    const map: Record<string, number> = {};
    (rules || []).forEach((rule) => {
     map[rule.role] = rule.default_percentage;
    });
    setRolePercentages(map);
   })
   .catch(() => {
    // Leitura que falhou não vira percentual inventado: o negócio é criado sem
    // rateio e o operador é avisado para configurá-lo antes de assinar.
    if (!cancelled) setRolePercentages({});
   });
  return () => {
   cancelled = true;
  };
 }, [open, dealToEdit]);

 // Detecta deal ativo duplicado (mesmo lead — como customer OU owner — para a
 // mesma propriedade). Não bloqueia o submit; apenas avisa.
 const duplicateDeal = useMemo(() => {
  if (dealToEdit) return null;
  if (!watchClient || !watchProperty) return null;
  return deals.find((d) =>
   (d.clientId === watchClient || d.ownerId === watchClient) &&
   (d.propertyId === watchProperty || d.property === watchProperty) &&
   ACTIVE_DEAL_STAGES.has(d.stage)
  ) || null;
 }, [deals, watchClient, watchProperty, dealToEdit]);

 // Duplo envio (duplo clique/Enter) criava dois negócios: o RHF não serializa
 // chamadas concorrentes de handleSubmit. A ref trava o submit em andamento e
 // o botão desabilita com isSubmitting; a CHAVE de idempotência cobre o resto
 // (retry de rede com o mesmo formulário devolve o negócio já criado).
 const submittingRef = useRef<boolean>(false);
 // Confirmação explícita quando já existe negócio ativo para o mesmo lead e
 // imóvel — a deduplicação visível, além da guarda silenciosa do backend.
 const [duplicateConfirmOpen, setDuplicateConfirmOpen] = useState(false);
 // Depois da confirmação explícita, o reenvio passa direto pela checagem de
 // duplicata (senão a confirmação abriria em loop).
 const duplicateOverrideRef = useRef<boolean>(false);

 const createIntentKey = (data: EsteiraDealFormData): string => {
  const company = getActiveCompanyId(user) ?? "tenant";
  return buildCreateIntentKey(company, data);
 };

 // Onda 9 — co-corretagem. Se o imóvel OU o lead selecionado foi
 // compartilhado por um parceiro, o negócio é um deal de co-corretagem:
 // exibimos a divisão de comissão do acordo e avisamos que a imobiliária
 // dona será notificada. A procedência (`sharedFrom`) é a mesma parceria
 // tanto para imóvel quanto para lead.
 const selectedSharedProperty = useMemo(
  () => properties.find((p) => p.id === watchProperty)?.sharedFrom ?? null,
  [properties, watchProperty],
 );
 const selectedSharedLead = useMemo(
  () => leads.find((l) => l.id === watchClient)?.sharedFrom ?? null,
  [leads, watchClient],
 );
 const sharedFrom = selectedSharedProperty ?? selectedSharedLead;

 // Acordo da parceria que originou o recurso compartilhado — buscado sob
 // demanda para exibir a divisão de comissão (captadora/fechamento %).
 const [coBrokeringPartnership, setCoBrokeringPartnership] = useState<Partnership | null>(null);
 useEffect(() => {
  const partnershipId = sharedFrom?.partnershipId;
  if (!partnershipId) {
   setCoBrokeringPartnership(null);
   return;
  }
  let cancelled = false;
  partnershipService
   .getById(partnershipId)
   .then((p) => { if (!cancelled) setCoBrokeringPartnership(p); })
   .catch(() => { if (!cancelled) setCoBrokeringPartnership(null); });
  return () => { cancelled = true; };
 }, [sharedFrom?.partnershipId]);

 const commissionModel = coBrokeringPartnership?.financial_config?.commission_model;


 const partyRoleOptions = useMemo(() => {
  if (watchType === 'Venda') return ['buyer', 'owner', 'guarantor', 'witness'] as const;
  return ['tenant', 'owner', 'guarantor', 'witness'] as const;
 }, [watchType]);

 const defaultCustomerRole = watchType === 'Venda' ? 'buyer' : 'tenant';
 const [selectedParties, setSelectedParties] = useState<Array<{ leadId: string; role: typeof partyRoleOptions[number]; isPrimary: boolean }>>([]);
 const [partyPickerOpen, setPartyPickerOpen] = useState(false);
 const wasOpenRef = useRef(false);
 const lastFormIdentityRef = useRef("");
 const lastServerSnapshotRef = useRef("");
 const formIdentity = dealToEdit
  ? `edit:${dealToEdit.id}`
  : `create:${initialLeadId || ""}:${initialPropertyId || ""}:${resolvedInitialPropertyId}`;
 const serverSnapshot = JSON.stringify(formDefaults);

 useEffect(() => {
  if (!open) {
   wasOpenRef.current = false;
   return;
  }

  const reopening = !wasOpenRef.current;
  const identityChanged = lastFormIdentityRef.current !== formIdentity;
  const serverChanged = lastServerSnapshotRef.current !== serverSnapshot;
  if (reopening || identityChanged || (!isDirty && serverChanged)) {
   reset(formDefaults);
   if (!dealToEdit) setSelectedParties([]);
   lastFormIdentityRef.current = formIdentity;
   lastServerSnapshotRef.current = serverSnapshot;
  }
  wasOpenRef.current = true;
 }, [dealToEdit, formDefaults, formIdentity, isDirty, open, reset, serverSnapshot]);

 const roleLabel: Record<string, string> = {
  tenant: "Inquilino",
  buyer: "Comprador",
  owner: "Proprietário",
  guarantor: "Fiador/Avalista",
  witness: "Testemunha",
 };

 // Quando o Tipo do negócio muda, papéis específicos de um tipo (Comprador ↔
 // Inquilino) deixam de existir nas opções do novo tipo. Normaliza qualquer
 // parte com papel agora inválido para o papel padrão do tipo atual, evitando
 // que o <select> da parte fique preso num valor fora das opções.
 useEffect(() => {
  if (!flags.multiPartyUI || dealToEdit) return;
  setSelectedParties((prev) => {
   const validRoles = new Set<string>(partyRoleOptions);
   if (prev.every((p) => validRoles.has(p.role))) return prev;
   const next = prev.map((p) =>
    validRoles.has(p.role) ? p : { ...p, role: defaultCustomerRole as typeof partyRoleOptions[number] }
   );
   // Mantém a invariante de haver um primário para o papel padrão.
   if (!next.some((p) => p.role === defaultCustomerRole && p.isPrimary)) {
    const idx = next.findIndex((p) => p.role === defaultCustomerRole);
    if (idx !== -1) next[idx] = { ...next[idx], isPrimary: true };
   }
   return next;
  });
 }, [partyRoleOptions, defaultCustomerRole, dealToEdit]);

 useEffect(() => {
  if (!flags.multiPartyUI || dealToEdit) return;
  if (selectedParties.length === 0) {
   setValue('client', '');
   return;
  }
  const primary =
   selectedParties.find((p) => p.role === defaultCustomerRole && p.isPrimary) ||
   selectedParties.find((p) => p.role === defaultCustomerRole) ||
   selectedParties[0];
  setValue('client', primary?.leadId ?? '');
 }, [selectedParties, defaultCustomerRole, setValue, dealToEdit]);

 // Effect to set initial values if editing, trying to match names to IDs if possible
 // But since we store names in EsteiraDeal, and selects will use IDs, we might have a mismatch if we don't handle it.
 // Actually, let's keep it simple: The select will output the ID (or Name).
 // If we want to strictly select from DB, we should use IDs.
 
 const onSubmit = async (data: EsteiraDealFormData) => {
  // Duplo envio (duplo clique/Enter) não pode disparar duas criações; o
  // backend também tem a chave de idempotência e a guarda de janela.
  if (submittingRef.current) return;
  const isCurrent = beginScopedRequest();
  if (!dealToEdit && duplicateDeal && !duplicateConfirmOpen && !duplicateOverrideRef.current) {
   // Deduplicação visível: já existe negócio ativo para este lead e imóvel.
   // Só cria outro com confirmação explícita — o caso típico é um duplo envio.
   setDuplicateConfirmOpen(true);
   return;
  }
  duplicateOverrideRef.current = false;
  if (dealToEdit) {
   const status = getStageMetaByLabel(data.stage)?.key;
   if (status) {
    try {
     await updateDealStatus(dealToEdit.id, { status });
     if (!isCurrent()) return;
    } catch (error: any) {
     if (!isCurrent()) return;
     const raw =
      error.response?.data?.message ||
      error.response?.data?.error ||
      error.message ||
      "";
     const cachedDeal = queryClient.getQueryData<DealDTO>(dealQueryKey(dealToEdit.id, tenantId));
     toast.error(moveDealErrorMessage(raw, cachedDeal, status));
     return;
    }
   }
  } else {
   const property = properties.find((p) => p.id === data.property || p.title === data.property);
   const client = leads.find((l) => l.id === data.client || l.name === data.client);
   if (!property || !client) {
    toast.error("Imóvel ou Cliente não encontrado para criar o negócio.");
    return;
   }

   const isCoBrokering = !!property.sharedFrom || !!client.sharedFrom;
   const ownerId = property.ownerId || property.owners?.[0]?.id;
   if (!ownerId && !isCoBrokering) {
    toast.error("Imóvel sem proprietário vinculado. Cadastre um proprietário antes de criar o negócio.");
    return;
   }
   if (!data.value || data.value <= 0) {
    toast.error("O valor do negócio precisa ser maior que zero.");
    return;
   }
   const ownerLead = ownerId ? leads.find((l) => l.id === ownerId) : undefined;
   // Rateio inicial do negócio. VENDA segue as regras por papel do tenant
   // (captador / fechamento / imobiliária); LOCAÇÃO mantém o rateio entre os
   // captadores do imóvel. Um imóvel com um único captador sem percentual
   // informado NÃO pode mais gerar `captador 100%` numa venda: a comissão
   // inteira ia para um corretor, sem retenção, e ficava congelada assim na
   // assinatura. Ver features/esteira/utils/dealBrokerParticipations.ts.
   const { participations: brokerParticipations, warning: participationWarning } =
    buildDealBrokerParticipations({
     dealType: data.type === "Venda" ? "SALE" : "RENT",
     capturingBrokers: property.capturingBrokers,
     closingBrokerId: (client as { assigned_broker_id?: string }).assigned_broker_id,
     responsibleBrokerId: userId,
     rolePercentages,
    });
   const dto: CreateDealDTO = {
    type: data.type === "Venda" ? "SALE" : "RENT",
    property_id: property.id,
    owner_id: ownerId || "",
    customer_id: client.id,
    broker_id: userId || "unknown",
    value: data.value,
    customer_name: client.name,
    customer_email: client.email,
    customer_account_id: client.accountId,
    customer_role: client.role === "buyer" ? "comprador" : client.role === "tenant" ? "locatario" : undefined,
    owner_name: ownerLead?.name || undefined,
    owner_email: ownerLead?.email || undefined,
    owner_phone: ownerLead?.phone || undefined,
    owner_account_id: ownerLead?.accountId || undefined,
    broker_participations: brokerParticipations.length > 0 ? brokerParticipations : undefined,
   };

   const parties = flags.multiPartyUI
    ? selectedParties.map((sp) => {
      const lead = leads.find((l) => l.id === sp.leadId);
      return {
       lead_id: sp.leadId,
       account_id: lead?.accountId,
       name: lead?.name,
       email: lead?.email,
       phone: lead?.phone,
       role: sp.role,
       is_primary: sp.isPrimary,
      };
     })
    : undefined;
   if (parties && parties.length > 0) {
    dto.parties = parties as unknown as CreatePartyDTO[];
   }
   submittingRef.current = true;
   try {
    // Mesma intenção = mesma chave: um retry de rede devolve o negócio já
    // criado (o backend responde com o registro existente em vez de duplicar).
    await createDeal(dto, createIntentKey(data));
    if (!isCurrent()) return;
    toast.success("Negócio criado com sucesso!");
    if (participationWarning) {
     // O negócio SEMPRE nasce com rateio (fecha 100%), mas quando a
     // configuração de comissão não pôde ser aplicada a comissão fica retida na
     // imobiliária: o operador precisa revisar antes de assinar.
     toast.warning("Revise o rateio de comissão deste negócio", { description: participationWarning });
    }
   } catch (error: any) {
    if (!isCurrent()) return;
    console.error("Erro ao criar negócio:", error);
    const raw =
     error.response?.data?.message ||
     error.response?.data?.error ||
     error.message ||
     "Erro ao criar negócio.";
    if (error.response?.status === 409) {
     // Guarda de janela do backend: um envio simultâneo criou o mesmo negócio.
     const existingId = error.response?.data?.data?.existing_deal_id as string | undefined;
     toast.error("Este negócio já foi criado agora mesmo", {
      description: existingId
       ? `As mesmas partes, imóvel e valor já têm um negócio ativo (#${existingId.substring(0, 8)}). O cadastro não foi duplicado.`
       : "As mesmas partes, imóvel e valor já têm um negócio ativo. O cadastro não foi duplicado.",
     });
     return;
    }
    if (error.response?.status === 403) {
     toast.error(
      "Você não tem permissão para iniciar este negócio. A parceria de co-corretagem com a imobiliária dona não está ativa.",
     );
     return;
    }
    if (typeof raw === "string" && raw.includes("CreateDealRequest.Value") && raw.includes("required")) {
     toast.error("Informe o valor do negócio para criar a proposta.");
     return;
    }
    if (typeof raw === "string" && raw.includes("CreateDealRequest.Value") && raw.includes("gt=0")) {
     toast.error("O valor do negócio precisa ser maior que zero.");
     return;
    }
    toast.error(raw);
    return;
   } finally {
    submittingRef.current = false;
   }
  }
  if (!isCurrent()) return;
  if (onSuccess) {
   await onSuccess(data);
   if (!isCurrent()) return;
  }
  onClose();
 };

 if (!open) return null;

 return (
  <>
   <ExpandableSidePanel
   open={open}
   onClose={onClose}
   defaultSize="lg"
   expandedSize="2xl"
   zIndex={60}
   renderHeader={({ expandButton, closeButton }) => (
    <div className="flex-none px-6 py-5 border-b border-border bg-card">
     <div className="flex items-start justify-between gap-3">
      <div className="min-w-0">
       <h2 className="text-lg font-bold text-foreground">{dealToEdit ? "Editar negócio" : "Nova proposta"}</h2>
       <p className="text-sm text-muted-foreground mt-0.5">
        {dealToEdit ? "Atualize os dados do negócio." : "Crie um negócio e configure as partes e papéis."}
       </p>
      </div>
      <div className="flex items-center gap-2 shrink-0">
       {expandButton}
       {closeButton}
      </div>
     </div>
    </div>
   )}
  >
   <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col flex-1 overflow-hidden">
    <div className="flex-1 overflow-y-auto px-6 py-5 space-y-6">
     <div className="space-y-1">
      <Label>Tipo</Label>
      <select {...register("type")} className="h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm">
       <option value="Venda">Venda</option>
       <option value="Locação">Locação</option>
      </select>
     </div>

     <div className="space-y-2">
      <Label>{flags.multiPartyUI && !dealToEdit ? "Partes" : "Cliente / Lead"}</Label>
      {flags.multiPartyUI && !dealToEdit ? (
       <>
        <input type="hidden" {...register("client", { required: "Cliente é obrigatório" })} />

        {/* Trigger com chips das partes selecionadas */}
        <Popover open={partyPickerOpen} onOpenChange={setPartyPickerOpen}>
         <PopoverTrigger asChild>
          <button
           type="button"
           className="w-full min-h-[42px] px-4 py-2.5 bg-background border border-border rounded-xl text-sm flex items-center gap-2 hover:bg-muted/50 transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500"
          >
           {selectedParties.length === 0 ? (
            <span className="text-muted-foreground">Selecionar partes...</span>
           ) : (
            <div className="flex flex-wrap gap-1.5 items-center flex-1 min-w-0">
             {selectedParties.map((p) => {
              const lead = leads.find((l) => l.id === p.leadId);
              return (
               <span
                key={p.leadId}
                className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md bg-primary-50 border border-primary-200 text-xs font-medium text-primary-700 max-w-[180px]"
               >
                <span className="truncate">{lead?.name ?? p.leadId}</span>
                <button
                 type="button"
                 onClick={(e) => {
                  e.stopPropagation();
                  setSelectedParties((prev) => prev.filter((x) => x.leadId !== p.leadId));
                 }}
                 className="shrink-0 p-0.5 rounded-sm hover:bg-primary-200/50 text-primary-500 hover:text-primary-700"
                >
                 <X className="w-3 h-3" />
                </button>
               </span>
              );
             })}
             <span className="text-xs text-muted-foreground ml-1">{selectedParties.length} selecionada(s)</span>
            </div>
           )}
           <ChevronsUpDown className="w-4 h-4 text-muted-foreground shrink-0 ml-auto" />
          </button>
         </PopoverTrigger>
         <PopoverContent
          className="w-[var(--radix-popover-trigger-width)] p-0 bg-card border border-border shadow-xl rounded-lg z-50"
          align="start"
         >
          <Command>
           <CommandInput placeholder="Buscar pessoa por nome ou email..." />
           <CommandList className="max-h-64">
            <CommandEmpty>Nenhum lead encontrado</CommandEmpty>
            {leads.map((lead) => {
             const isSelected = selectedParties.some((p) => p.leadId === lead.id);
             return (
              <CommandItem
               key={lead.id}
               value={`${lead.name ?? ""} ${lead.email ?? ""}`.trim()}
               onSelect={() => {
                if (isSelected) {
                 setSelectedParties((prev) => prev.filter((p) => p.leadId !== lead.id));
                } else {
                 setSelectedParties((prev) => {
                  if (prev.some((p) => p.leadId === lead.id)) return prev;
                  const next = [...prev, { leadId: lead.id, role: defaultCustomerRole as any, isPrimary: false }];
                  if (!next.some((p) => p.role === defaultCustomerRole && p.isPrimary)) {
                   const idx = next.findIndex((p) => p.role === defaultCustomerRole);
                   if (idx !== -1) next[idx] = { ...next[idx], isPrimary: true };
                  }
                  return next;
                 });
                }
                // Mantém aberto para multi-seleção
               }}
              >
               <div className="flex items-center gap-2 w-full min-w-0">
                {isSelected ? (
                 <Check className="w-4 h-4 text-primary-600 shrink-0" />
                ) : (
                 <div className="w-4 h-4 shrink-0" />
                )}
                <div className="flex flex-col min-w-0 flex-1">
                 <span className="text-sm font-medium truncate">
                  {lead.name} {lead.isClient ? "(Cliente)" : "(Lead)"}
                 </span>
                 <span className="text-xs text-muted-foreground truncate">{lead.email}</span>
                </div>
               </div>
              </CommandItem>
             );
            })}
           </CommandList>
          </Command>
          <div className="border-t border-border p-2 flex items-center justify-between">
           <span className="text-xs text-muted-foreground px-2">
            {selectedParties.length} de {leads.length} selecionada(s)
           </span>
           <button
            type="button"
            onClick={() => setPartyPickerOpen(false)}
            className="px-3 py-1.5 text-xs font-medium bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors"
           >
            Concluir
           </button>
          </div>
         </PopoverContent>
        </Popover>

        {selectedParties.length > 0 ? (
         <div className="space-y-2">
          {selectedParties.map((p) => {
           const lead = leads.find((l) => l.id === p.leadId);
           return (
            <div key={p.leadId} className="p-3 rounded-xl border border-border bg-muted space-y-2">
             <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
               <div className="text-sm font-semibold text-foreground truncate">{lead?.name ?? p.leadId}</div>
               <div className="text-xs text-muted-foreground truncate">{lead?.email}</div>
              </div>
              <Button
               type="button"
               variant="ghost"
               size="sm"
               className="text-destructive hover:text-destructive"
               onClick={() => setSelectedParties((prev) => prev.filter((x) => x.leadId !== p.leadId))}
              >
               Remover
              </Button>
             </div>

             <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <div className="space-y-1">
               <div className="text-xs font-semibold text-muted-foreground">Tipo de parte</div>
               <select
                value={p.role}
                onChange={(ev) => {
                 const role = ev.target.value as any;
                 setSelectedParties((prev) => {
                  const next = prev.map((x) => (x.leadId === p.leadId ? { ...x, role } : x));
                  if (!next.some((x) => x.role === defaultCustomerRole && x.isPrimary)) {
                   const idx = next.findIndex((x) => x.role === defaultCustomerRole);
                   if (idx !== -1) next[idx] = { ...next[idx], isPrimary: true };
                  }
                  return next;
                 });
                }}
                className="h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
               >
                {partyRoleOptions.map((r) => (
                 <option key={r} value={r}>
                  {roleLabel[r] ?? r}
                 </option>
                ))}
               </select>
              </div>

              <div className="space-y-1 sm:col-span-2">
               <div className="text-xs font-semibold text-muted-foreground">Primário</div>
               <div className="h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm flex items-center gap-2">
                <Checkbox
                 checked={p.isPrimary}
                 onCheckedChange={(v) => {
                  const checked = Boolean(v);
                  setSelectedParties((prev) =>
                   prev.map((x) =>
                    x.role === p.role ? { ...x, isPrimary: x.leadId === p.leadId ? checked : false } : x
                   )
                  );
                 }}
                />
                <span>Definir como primário para este papel</span>
               </div>
              </div>
             </div>
            </div>
           );
          })}
         </div>
        ) : (
         <div className="p-4 border border-border rounded-2xl bg-card text-sm text-muted-foreground">
          Adicione pelo menos uma parte para criar o negócio.
         </div>
        )}
       </>
      ) : (
       <select
        {...register("client", { required: "Cliente é obrigatório" })}
        className="h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
       >
        <option value="">Selecione um cliente</option>
        {leads.map((lead) => (
         <option key={lead.id} value={lead.id}>
          {lead.name} {lead.isClient ? "(Cliente)" : "(Lead)"}
         </option>
        ))}
       </select>
      )}
      {errors.client && <div className="text-xs text-destructive">{errors.client.message}</div>}
     </div>

     <div className="space-y-1">
      <Label>Imóvel</Label>
      <input type="hidden" {...register("property", { required: "Imóvel é obrigatório" })} />
      <PropertyPicker
       properties={properties}
       value={watch("property")}
       includeNone={false}
       placeholder="Selecione um imóvel"
       onChange={(next) => setValue("property", next, { shouldDirty: true, shouldValidate: true })}
       buttonClassName="h-10 bg-background border-input"
       onCreateNew={() => setIsCreatingProperty(true)}
      />
      {errors.property && <div className="text-xs text-destructive">{errors.property.message}</div>}
     </div>

     <div className="space-y-1">
      <Label>Valor (R$)</Label>
      <Controller
        name="value"
        control={control}
        rules={{ required: "Valor é obrigatório", min: { value: 1, message: "Informe um valor maior que zero." } }}
        render={({ field }) => (
          <MoneyInput
            ref={field.ref}
            name={field.name}
            onBlur={field.onBlur}
            fractionDigits={0}
            withSymbol
            value={typeof field.value === "number" ? field.value : undefined}
            onChange={(v) => field.onChange(v ?? 0)}
            className="flex h-9 w-full min-w-0 rounded-md border border-input bg-input-background px-3 py-1 text-base outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px] md:text-sm placeholder:text-muted-foreground"
          />
        )}
      />
      {errors.value && <div className="text-xs text-destructive">{errors.value.message}</div>}
     </div>

     <div className="space-y-1">
      <Label>Estágio</Label>
      <select {...register("stage")} className="h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm">
       <option value="Em Negociação">Em Negociação</option>
       <option value="Proposta">Proposta</option>
       <option value="Análise">Análise (Doc)</option>
       <option value="Contrato">Contrato</option>
       <option value="Concluído">Concluído</option>
      </select>
     </div>

     {sharedFrom && (
      <div className="rounded-xl border border-stage-contract/30 bg-stage-contract/5 px-4 py-3 text-sm text-foreground">
       <div className="flex items-center gap-2 font-bold text-stage-contract mb-1">
        <Share2 className="w-4 h-4" />
        Negócio de co-corretagem
       </div>
       <p className="text-xs text-muted-foreground">
        {selectedSharedProperty && selectedSharedLead
         ? "O imóvel e o lead foram compartilhados por "
         : selectedSharedProperty
          ? "Este imóvel foi compartilhado por "
          : "Este lead foi compartilhado por "}
        <span className="font-semibold text-foreground">{sharedFrom.companyName}</span>.
       </p>
       {commissionModel && (commissionModel.owner_company_pct || commissionModel.closing_company_pct) ? (
        <p className="text-xs text-muted-foreground mt-1">
         Divisão de comissão do acordo —{" "}
         <span className="font-semibold text-foreground">
          captadora {commissionModel.owner_company_pct}%
         </span>{" "}
         /{" "}
         <span className="font-semibold text-foreground">
          fechamento {commissionModel.closing_company_pct}%
         </span>
         .
        </p>
       ) : (
        <p className="text-xs text-muted-foreground mt-1">
         A divisão de comissão segue o acordo da parceria.
        </p>
       )}
       <p className="text-xs text-muted-foreground mt-1">
        Ao criar, a imobiliária dona será notificada.
       </p>
      </div>
     )}

     {duplicateDeal && (
      <div className="rounded-xl border border-status-warning/40 bg-status-warning-soft px-4 py-3 text-sm text-status-warning">
       <div className="font-bold mb-0.5">Este lead já possui um negócio ativo para esta propriedade.</div>
       <div className="text-xs">
        Negócio existente: <span className="font-semibold">#{duplicateDeal.displayId || duplicateDeal.id.substring(0, 8)}</span>
        {" — "}{duplicateDeal.stage}. Você pode continuar mesmo assim.
       </div>
      </div>
     )}
    </div>

    <div className="flex-none px-6 py-4 border-t border-border bg-card">
     <div className="flex justify-end gap-2">
      <Button type="button" variant="outline" onClick={onClose}>
       Cancelar
      </Button>
      <Button
       type="submit"
       loading={isSubmitting}
       loadingText={dealToEdit ? "Salvando…" : "Criando…"}
       disabled={
        isSubmitting ||
        submittingRef.current ||
        (flags.multiPartyUI && !dealToEdit && selectedParties.length === 0)
       }
      >
       {dealToEdit ? "Salvar alterações" : "Criar negócio"}
      </Button>
     </div>
    </div>
   </form>
  </ExpandableSidePanel>

  {/* O cadastro de imóvel abre por cima do painel do negócio: o formulário do
      negócio (partes, valor, estágio) continua preenchido atrás.

      `PropertyModal` é só o CONTEÚDO do formulário: fundo, posicionamento e o
      botão de fechar vêm do `ExpandableSidePanel`. Sem esse invólucro o
      formulário nascia solto DENTRO do painel do negócio (mesmo z-index, sem
      sobreposição) e o clique em "Cadastrar novo imóvel" parecia não fazer
      nada. A camada é a da escala (`LAYER.panelTop`): acima do painel do
      negócio (60) e abaixo das confirmações (300) e menus flutuantes (400). */}
  <ExpandableSidePanel
   open={isCreatingProperty}
   onClose={() => setIsCreatingProperty(false)}
   defaultSize="4xl"
   expandedSize="full"
   zIndex={LAYER.panelTop}
   ariaLabel="Novo imóvel"
  >
   <PropertyModal
    isOpen={isCreatingProperty}
    onClose={() => setIsCreatingProperty(false)}
    onCreated={(created) => {
     setValue("property", created.id, { shouldDirty: true, shouldValidate: true });
     toast.success(`Imóvel "${created.title || created.id}" cadastrado e vinculado ao negócio.`);
    }}
   />
  </ExpandableSidePanel>

  <ConfirmDialog
   open={duplicateConfirmOpen}
   title="Já existe um negócio ativo para este lead e imóvel"
   description={
    <>
     {duplicateDeal ? (
      <span>
       Negócio existente: <span className="font-semibold">#{duplicateDeal.displayId || duplicateDeal.id.substring(0, 8)}</span>
       {" — "}
       {duplicateDeal.stage}. Pode ser o envio duplicado desta mesma tela.
      </span>
     ) : (
      "Já existe um negócio ativo com as mesmas partes, imóvel e valor."
     )}
     <br />
     Criar outro negócio mesmo assim?
    </>
   }
   variant="warning"
   confirmLabel="Criar outro mesmo assim"
   cancelLabel="Não criar"
   onConfirm={() => {
    setDuplicateConfirmOpen(false);
    duplicateOverrideRef.current = true;
    void handleSubmit(onSubmit)();
   }}
   onCancel={() => {
    setDuplicateConfirmOpen(false);
   }}
  />
  </>
 );
}
