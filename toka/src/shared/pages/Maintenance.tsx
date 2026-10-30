import React, { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import {
 Wrench,
 ClipboardCheck,
 MessageSquare,
 DollarSign,
 Calendar,
 AlertCircle,
 CheckCircle2,
 Clock,
 RefreshCw,
 Search,
 X,
 Maximize2,
 Minimize2,
 FileText,
 User,
 Paperclip,
 Send,
 ChevronDown,
 Plus,
 History,
 Check
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { motion, AnimatePresence } from "motion/react";
import { EmptyState } from "../ui/EmptyState";
import { Button } from "../ui/button";
import { PageHeader } from "../ui/page-header";
import { Skeleton } from "../ui/skeleton";
import { useIsMobile } from "../ui/use-mobile";
import { useContractsContext } from "../context/ContractsContext";
import { useLeadsContext } from "../context/LeadsContext";
import { useMaintenanceContext } from "../context/MaintenanceContext";
import { usePropertiesContext } from "../context/PropertiesContext";
import type { AdvancedBy, BillingTarget, MaintenanceTicket, PayerRole } from "../services/maintenance.service";
import { maintenanceService } from "../services/maintenance.service";
import { chargeService, type Charge } from "../services/charge.service";
import { transferService, type OwnerTransfer } from "../services/transfer.service";
import { contractLabel } from "../contracts/contractLabel";
import { SupplierPicker } from "../components/suppliers/SupplierPicker";
import type { Supplier } from "../services/supplier.service";
import { useQuery } from "@tanstack/react-query";
import { formatCompetencia, formatISODateBR, formatMoneyBR } from "../utils/format";
import { AuditTrailPanel } from "../components/AuditTrailPanel";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "../ui/command";
import { PropertyPicker } from "../components/PropertyPicker";
import { useFocusMode } from "../context/FocusModeContext";
import { MoneyInput } from "../ui/money-input";
import { MediaImage } from "../components/MediaImage";
import { getMediaUrl } from "../media";

type ModalState = 'none' | 'chat' | 'cobranca' | 'agendamento' | 'nova';

/**
 * Rascunho do painel aberto. Qualquer mutação na aplicação dispara o refresh
 * global de dados, que remonta a árvore da rota e zera o estado local — o
 * operador perdia o painel e o que já tinha preenchido. Guardar o rascunho na
 * sessão devolve o painel como estava.
 */
const PANEL_DRAFT_KEY = 'maintenance:open-panel';

interface PanelDraft {
 activeModal: ModalState;
 ticket: MaintenanceTicket | null;
 schedule: { supplierId: string; date: string; time: string; instructions: string };
}

function readPanelDraft(): PanelDraft | null {
 if (typeof window === 'undefined') return null;
 try {
  const raw = window.sessionStorage.getItem(PANEL_DRAFT_KEY);
  if (!raw) return null;
  const parsed = JSON.parse(raw) as PanelDraft;
  if (!parsed || parsed.activeModal === 'none') return null;
  return parsed;
 } catch {
  return null;
 }
}

function writePanelDraft(draft: PanelDraft | null) {
 if (typeof window === 'undefined') return;
 try {
  if (!draft) window.sessionStorage.removeItem(PANEL_DRAFT_KEY);
  else window.sessionStorage.setItem(PANEL_DRAFT_KEY, JSON.stringify(draft));
 } catch {
  // Sessão indisponível (modo privado): o painel segue só em memória.
 }
}



export function Maintenance() {
 const { tickets, isTicketsLoading, ticketsError, reloadTickets, addTicket, updateTicket } = useMaintenanceContext();
 const { properties } = usePropertiesContext();
 const { contracts } = useContractsContext();
 const { leads } = useLeadsContext();
 const { focusMode } = useFocusMode();
 const isMobile = useIsMobile();
 const navigate = useNavigate();
 const [searchParams] = useSearchParams();
 const openedContractRef = useRef<string | null>(null);
 const [activeModal, setActiveModal] = useState<ModalState>(() => readPanelDraft()?.activeModal ?? 'none');
 const [selectedItemId, setSelectedItemId] = useState<string | null>(() => readPanelDraft()?.ticket?.id ?? null);
 // Qualquer mutação na aplicação recarrega a lista de chamados. Enquanto ela
 // recarrega, o chamado some da lista e o painel aberto era desmontado no meio
 // do preenchimento — então guardamos o último estado conhecido e usamos a
 // lista só para atualizá-lo quando ela tem o registro.
 const [selectedItemSnapshot, setSelectedItemSnapshot] = useState<MaintenanceTicket | null>(() => readPanelDraft()?.ticket ?? null);
 const selectedItem = useMemo(() => {
  if (!selectedItemId) return null;
  return tickets.find((ticket) => ticket.id === selectedItemId) ?? selectedItemSnapshot;
 }, [selectedItemId, selectedItemSnapshot, tickets]);
 const setSelectedItem = (item: MaintenanceTicket | null) => {
  setSelectedItemId(item?.id ?? null);
  setSelectedItemSnapshot(item);
 };
 const closeTimerRef = useRef<number | null>(null);
 const [activeTab, setActiveTab] = useState<'Todas' | 'Manutenção' | 'Vistoria'>('Todas');
 const [searchTerm, setSearchTerm] = useState('');
 const [auditEntityId, setAuditEntityId] = useState<string | null>(null);
 const [auditEntityLabel, setAuditEntityLabel] = useState('');

 const [isExpanded, setIsExpanded] = useState(false);
 const toggleExpand = () => setIsExpanded(prev => !prev);

 // Form states for Nova Solicitação
 const [selectedPropertyId, setSelectedPropertyId] = useState<string>('');
 const [selectedContractId, setSelectedContractId] = useState<string>('');
 const [selectedRequesterId, setSelectedRequesterId] = useState<string>('');
 const describeContract = (contract: (typeof contracts)[number]) => contractLabel(contract, leads.find((lead) => lead.id === contract.customer_id)?.name, properties.find((property) => property.id === contract.property_id)?.title);
 const contractText = (id?: string) => { const contract = contracts.find((item) => item.id === id); return contract ? describeContract(contract) : "Contrato não disponível"; };
 const [contractSearchOpen, setContractSearchOpen] = useState(false);
 const [requesterSearchOpen, setRequesterSearchOpen] = useState(false);
 const [newTicketSubmitting, setNewTicketSubmitting] = useState(false);
 const [schedulingTicketId, setSchedulingTicketId] = useState<string | null>(null);
 const [requesterProfile, setRequesterProfile] = useState<string>('Inquilino');

 // ── Cascata do formulário ────────────────────────────────────────────────
 // Contrato define o imóvel e as partes; imóvel define os proprietários. Sem
 // isso era possível abrir um chamado apontando para imóvel/parte de outro
 // contrato — e o financeiro lançava a cobrança no lugar errado.
 const selectedContract = useMemo(
  () => contracts.find((item) => item.id === selectedContractId),
  [contracts, selectedContractId],
 );

 useEffect(() => {
  const contractId = searchParams.get("contrato");
  if (!contractId || openedContractRef.current === contractId || !contracts.some((contract) => contract.id === contractId)) return;
  openedContractRef.current = contractId;
  setSelectedItemId(null);
  setSelectedItemSnapshot(null);
  setSelectedContractId(contractId);
  setActiveModal('nova');
 }, [searchParams, contracts]);

 const contractProperty = useMemo(() => {
  const propertyId = selectedContract?.property_id;
  return propertyId ? properties.find((item) => item.id === propertyId) : undefined;
 }, [properties, selectedContract]);

 const propertyOwners = useMemo(() => {
  const property = properties.find((item) => item.id === selectedPropertyId);
  if (!property) return [];
  const ids = new Set<string>();
  for (const owner of property.owners ?? []) if (owner.id) ids.add(owner.id);
  if (property.ownerId) ids.add(property.ownerId);
  return leads.filter((lead) => ids.has(lead.id));
 }, [leads, properties, selectedPropertyId]);

 const contractTenant = useMemo(() => {
  if (!selectedContract) return undefined;
  return (
   leads.find((lead) => lead.id === selectedContract.customer_id) ??
   leads.find((lead) => lead.name?.trim() === selectedContract.client?.trim())
  );
 }, [leads, selectedContract]);

 /** Partes elegíveis: as do contrato quando ele existe, senão os clientes. */
 const requesterOptions = useMemo(() => {
  const fromContract = [contractTenant, ...propertyOwners].filter(
   (lead): lead is NonNullable<typeof lead> => Boolean(lead),
  );
  const base = fromContract.length > 0 ? fromContract : leads.filter((lead) => lead.isClient);
  const unique = base.filter((lead, index) => base.findIndex((other) => other.id === lead.id) === index);

  if (requesterProfile === 'Proprietário') {
   const owners = unique.filter((lead) => propertyOwners.some((owner) => owner.id === lead.id));
   return owners.length > 0 ? owners : unique;
  }
  if (requesterProfile === 'Inquilino' && contractTenant) {
   return [contractTenant];
  }
  return unique;
 }, [contractTenant, leads, propertyOwners, requesterProfile]);

 // Ao escolher o contrato, o imóvel e a parte passam a ser os dele.
 useEffect(() => {
  if (!selectedContract) return;
  const contractPropertyId = selectedContract.property_id;
  setSelectedPropertyId((current) =>
   contractPropertyId && current !== contractPropertyId ? contractPropertyId : current,
  );
 }, [selectedContract]);

 // Trocar de imóvel/parte invalida uma seleção que não pertence mais ao contexto.
 useEffect(() => {
  if (selectedRequesterId && !requesterOptions.some((lead) => lead.id === selectedRequesterId)) {
   setSelectedRequesterId('');
  }
 }, [requesterOptions, selectedRequesterId]);

 // Form states for Cobrança
 const [cobrancaValor, setCobrancaValor] = useState<number | null>(null);
 const [cobrancaDesc, setCobrancaDesc] = useState('');
 // PayerRole maps to the radio buttons. Default to "tenant" since that's
 // the radio pre-checked in the UI. "shared" is the avulsa flow — the
 // backend creates the charge but doesn't attach it to the rental boleto.
 const [cobrancaPayerRole, setCobrancaPayerRole] = useState<PayerRole>('tenant');
 const [cobrancaTarget, setCobrancaTarget] = useState<BillingTarget>('next_charge');
 const [cobrancaAdvancedBy, setCobrancaAdvancedBy] = useState<AdvancedBy>('imobiliaria');
 // Agendamento com o prestador: o chamado guarda a referência do cadastro,
 // não apenas o nome digitado.
 const [scheduleSupplierId, setScheduleSupplierId] = useState(() => readPanelDraft()?.schedule.supplierId ?? '');
 const [scheduleSupplier, setScheduleSupplier] = useState<Supplier | undefined>(undefined);
 const [scheduleDate, setScheduleDate] = useState(() => readPanelDraft()?.schedule.date ?? '');
 const [scheduleTime, setScheduleTime] = useState(() => readPanelDraft()?.schedule.time ?? '');
 const [scheduleInstructions, setScheduleInstructions] = useState(() => readPanelDraft()?.schedule.instructions ?? '');
 const [cobrancaSubmitting, setCobrancaSubmitting] = useState(false);
 const modalMutationPending =
  newTicketSubmitting || cobrancaSubmitting || schedulingTicketId !== null;

 const openModal = (type: ModalState, item?: MaintenanceTicket) => {
 if (closeTimerRef.current != null) {
  window.clearTimeout(closeTimerRef.current);
  closeTimerRef.current = null;
 }
 if (item) setSelectedItem(item);
 if (type === 'nova') setSelectedItem(null);
 if (type === 'agendamento') {
  setScheduleSupplierId(item?.supplier_id ?? '');
  setScheduleSupplier(undefined);
  setScheduleDate(item?.scheduled_at ? item.scheduled_at.slice(0, 10) : '');
  setScheduleTime(item?.scheduled_at ? item.scheduled_at.slice(11, 16) : '');
  setScheduleInstructions(item?.instructions ?? '');
 }
 if (type === 'cobranca') {
  setCobrancaValor(null);
  setCobrancaDesc('');
  setCobrancaPayerRole('tenant');
  setCobrancaTarget('next_charge');
  setCobrancaAdvancedBy('imobiliaria');
 }
 setActiveModal(type);
 };

 useEffect(() => {
  if (activeModal === 'none') {
   writePanelDraft(null);
   return;
  }
  writePanelDraft({
   activeModal,
   ticket: selectedItemSnapshot,
   schedule: {
    supplierId: scheduleSupplierId,
    date: scheduleDate,
    time: scheduleTime,
    instructions: scheduleInstructions,
   },
  });
 }, [activeModal, selectedItemSnapshot, scheduleSupplierId, scheduleDate, scheduleTime, scheduleInstructions]);

 const closeModal = () => {
 writePanelDraft(null);
 setActiveModal('none');
 closeTimerRef.current = window.setTimeout(() => {
  setSelectedItem(null);
  closeTimerRef.current = null;
 }, 300);
 };

 useEffect(() => {
  return () => {
   if (closeTimerRef.current != null) window.clearTimeout(closeTimerRef.current);
  };
 }, []);


 // O chamado guarda o id do contrato; chamados antigos podem ter o display id.
 const billingContractLabel = useMemo(() => {
  if (!selectedItem) return '—';
  const contract = contracts.find((item) => item.id === selectedItem.contract_id);
  if (!contract) return 'Contrato não disponível';
  return contractLabel(contract);
 }, [contracts, selectedItem]);

 const billingPartyLabel = useMemo(() => {
  if (!selectedItem) return '—';
  const lead =
   leads.find((item) => item.id === selectedItem.requester_lead_id) ??
   leads.find((item) => item.name?.trim() === selectedItem.requester?.trim());
  return lead?.name || selectedItem.requester || 'não informada';
 }, [leads, selectedItem]);

 // Onde o custo vai entrar: próxima cobrança em aberto do contrato (inquilino)
 // ou próximo repasse pendente (proprietário). Lido só quando a janela de
 // lançamento está aberta, para o operador ver o destino antes de confirmar.
 const billingDestinationQuery = useQuery({
  queryKey: ["maintenance-billing-destination", selectedItem?.contract_id ?? "none"],
  queryFn: async () => {
   const contractId = selectedItem?.contract_id as string;
   const [chargesResponse, transfersResponse] = await Promise.all([
    chargeService.getAll({ contract_id: contractId, status: "PENDING", limit: 100 }),
    transferService.getAll({ contract_id: contractId, status: "PENDING", limit: 100 }),
   ]);
   return {
    charges: ((chargesResponse as { data?: Charge[] })?.data ?? []) as Charge[],
    transfers: ((transfersResponse as { data?: OwnerTransfer[] })?.data ?? []) as OwnerTransfer[],
   };
  },
  enabled: activeModal === 'cobranca' && Boolean(selectedItem?.contract_id),
  staleTime: 30_000,
 });

 const nextOpenCharge = useMemo(() => {
  const pending = [...(billingDestinationQuery.data?.charges ?? [])].sort((a, b) =>
   (a.due_date ?? "").localeCompare(b.due_date ?? ""),
  );
  if (pending.length === 0) return undefined;
  const today = new Date().toISOString().slice(0, 10);
  return pending.find((charge) => (charge.due_date ?? "") >= today) ?? pending[0];
 }, [billingDestinationQuery.data]);

 const nextPendingTransfer = useMemo(() => {
  const pending = [...(billingDestinationQuery.data?.transfers ?? [])].sort((a, b) =>
   (a.reference_month ?? "").localeCompare(b.reference_month ?? ""),
  );
  if (pending.length === 0) return undefined;
  const currentMonth = new Date().toISOString().slice(0, 7);
  return pending.find((transfer) => (transfer.reference_month ?? "") >= currentMonth) ?? pending[0];
 }, [billingDestinationQuery.data]);

 // A natureza do valor decide quanto do lançamento é receita da imobiliária:
 // nada. Custo de terceiro é reembolso do que a imobiliária adiantou — e
 // quando quem pagou o fornecedor é quem assume o custo, não há o que lançar.
 const billingNature = useMemo(() => {
  const advanced: string = cobrancaAdvancedBy;
  // A responsabilidade financeira usa tenant/owner/agency; quem pagou usa
  // inquilino/proprietário/imobiliária. Sem traduzir, a mesma parte apareceria
  // como duas.
  const bears: string =
   cobrancaPayerRole === 'tenant'
    ? 'inquilino'
    : cobrancaPayerRole === 'owner' || cobrancaPayerRole === 'shared'
      ? 'proprietario'
      : 'imobiliaria';
  if (advanced === bears) {
   return {
    movement: false,
    message:
     advanced === 'proprietario'
      ? 'O proprietário pagou o serviço e assume o custo: nada é cobrado nem descontado.'
      : 'O inquilino pagou o serviço e assume o custo: nada é cobrado nem descontado.',
   };
  }
  if (advanced === 'imobiliaria') {
   return {
    movement: true,
    message: 'A imobiliária adiantou o pagamento: o valor entra como reembolso do custo, sem virar receita própria.',
   };
  }
  return {
   movement: true,
   message:
    advanced === 'proprietario'
     ? 'O proprietário adiantou: o reembolso pago pelo inquilino pertence a ele.'
     : 'O inquilino adiantou: o reembolso pago pelo proprietário pertence a ele.',
  };
 }, [cobrancaAdvancedBy, cobrancaPayerRole]);

 // O que o contrato permite hoje: sem cobrança em aberto não há onde somar;
 // sem repasse pendente (locação encerrada, venda já repassada) não há onde
 // descontar. Nesses casos a cobrança avulsa é o caminho válido.
 // Só entra em cobrança que já tem composição: somar numa cobrança sem
 // composição exigiria inventar o dono econômico do valor que já estava lá.
 const nextOpenChargeIsComposed = (nextOpenCharge?.line_items?.length ?? 0) > 0;
 const canUseNextCharge = Boolean(nextOpenCharge) && nextOpenChargeIsComposed;

 // Quanto o repasse absorve antes de truncar: com subledger o desconto
 // bloqueia o disponível, cujo teto é total − reservado; sem ele, o líquido.
 const transferCapacity = useMemo(() => {
  if (!nextPendingTransfer) return 0;
  const balance = nextPendingTransfer.owner_balance;
  if (balance) return Math.max(0, (balance.total_cents - balance.reserved_cents) / 100);
  return Math.max(0, nextPendingTransfer.net_amount ?? 0);
 }, [nextPendingTransfer]);

 const transferAbsorbsValue = cobrancaValor === null || cobrancaValor <= transferCapacity;
 const canUseNextTransfer = Boolean(nextPendingTransfer) && transferAbsorbsValue;
 const destinationsLoading = billingDestinationQuery.isLoading;

 /** Destinos oferecidos para a responsabilidade escolhida. */
 const destinationOptions = useMemo(() => {
  if (cobrancaPayerRole === 'owner') {
   return [
    {
     id: 'next_transfer' as BillingTarget,
     label: 'Descontar do próximo repasse',
     available: canUseNextTransfer,
     hint: !nextPendingTransfer
      ? 'O contrato não tem repasse pendente — encerrado ou já repassado ao proprietário.'
      : !transferAbsorbsValue
       ? `O repasse de ${formatCompetencia(nextPendingTransfer.reference_month)} absorve até ${formatMoneyBR(transferCapacity, { withSymbol: true })}; o excedente não é retido. Use a cobrança avulsa.`
       : `Abate no repasse de ${formatCompetencia(nextPendingTransfer.reference_month)} (até ${formatMoneyBR(transferCapacity, { withSymbol: true })}).`,
    },
    {
     id: 'standalone' as BillingTarget,
     label: 'Cobrança avulsa ao proprietário',
     available: true,
     hint: 'Cobrança de manutenção com vencimento em 30 dias, fora do repasse.',
    },
   ];
  }
  return [
   {
    id: 'next_charge' as BillingTarget,
    label: 'Somar na próxima cobrança do inquilino',
    available: canUseNextCharge,
    hint: !nextOpenCharge
     ? 'O contrato não tem cobrança em aberto para receber o valor.'
     : !nextOpenChargeIsComposed
      ? 'A cobrança em aberto não tem composição definida; use a cobrança avulsa.'
      : `Entra na cobrança de ${formatISODateBR(nextOpenCharge.due_date)}.`,
   },
   {
    id: 'standalone' as BillingTarget,
    label: 'Cobrança avulsa ao inquilino',
    available: true,
    hint: 'Cobrança de manutenção com vencimento em 30 dias, fora do aluguel.',
   },
  ];
 }, [
  canUseNextCharge,
  canUseNextTransfer,
  cobrancaPayerRole,
  nextOpenCharge,
  nextOpenChargeIsComposed,
  nextPendingTransfer,
  transferAbsorbsValue,
  transferCapacity,
 ]);

 // Escolha inválida (contrato encerrado, repasse já feito) cai para a avulsa
 // em vez de deixar o operador tentar um lançamento que vai falhar.
 useEffect(() => {
  if (activeModal !== 'cobranca' || destinationsLoading) return;
  const current = destinationOptions.find((option) => option.id === cobrancaTarget);
  if (current?.available) return;
  const fallback = destinationOptions.find((option) => option.available);
  if (fallback) setCobrancaTarget(fallback.id);
 }, [activeModal, cobrancaTarget, destinationOptions, destinationsLoading]);

 const selectedDestination = destinationOptions.find((option) => option.id === cobrancaTarget);
 const destinationBlocked = !destinationsLoading && selectedDestination !== undefined && !selectedDestination.available;

 const billingPreview = useMemo(() => {
  if (cobrancaValor === null) return null;
  if (cobrancaTarget === 'standalone') {
   return `Cobrança avulsa de ${formatMoneyBR(cobrancaValor, { withSymbol: true })} com vencimento em 30 dias.`;
  }
  if (cobrancaTarget === 'next_transfer') {
   if (destinationsLoading) return 'Buscando o próximo repasse pendente…';
   if (!nextPendingTransfer) return null;
   return `Entra no repasse de ${formatCompetencia(nextPendingTransfer.reference_month)} como desconto de manutenção.`;
  }
  if (destinationsLoading) return 'Buscando a próxima cobrança em aberto…';
  if (!nextOpenCharge) return null;
  const current = nextOpenCharge.total_amount ?? nextOpenCharge.amount ?? 0;
  const due = nextOpenCharge.due_date ? formatISODateBR(nextOpenCharge.due_date) : "sem vencimento";
  return `Entra na cobrança de ${due}: ${formatMoneyBR(current, { withSymbol: true })} → ${formatMoneyBR(
   current + cobrancaValor,
   { withSymbol: true },
  )} (item "Manutenção: ${selectedItem?.category ?? "serviço"}").`;
 }, [cobrancaTarget, cobrancaValor, destinationsLoading, nextOpenCharge, nextPendingTransfer, selectedItem?.category]);

 const handleCreateCobranca = async () => {
 if (!selectedItem || cobrancaValor === null) return;
 setCobrancaSubmitting(true);

 try {
 // PUT grava a política financeira (payor + custo) e PATCH conclui o
 // chamado. A conclusão é o gatilho do toca-financial: ele cria a cobrança
 // do inquilino ou aplica o desconto no repasse do proprietário. Não criar
 // lançamento manual aqui — isso duplicaria o mesmo custo no financeiro.
 await maintenanceService.update(selectedItem.id, {
 payer_role: cobrancaPayerRole,
 billing_target: cobrancaTarget,
 advanced_by: cobrancaAdvancedBy,
 estimated_cost: cobrancaValor,
 actual_cost: cobrancaValor,
 });
 await maintenanceService.changeStatus(selectedItem.id, 'Concluído');

 await reloadTickets();
 toast.success(
 cobrancaTarget === 'next_transfer'
  ? 'Custo lançado: será descontado no próximo repasse ao proprietário.'
  : cobrancaTarget === 'next_charge'
   ? 'Custo somado na próxima cobrança do inquilino.'
   : 'Cobrança avulsa de manutenção emitida no financeiro.',
 );
 closeModal();
 } catch (err) {
 console.error('Failed to record maintenance billing', err);
 const message =
  (err as { response?: { data?: { message?: string } } })?.response?.data?.message ||
  'Não foi possível lançar o custo no financeiro.';
 toast.error(message);
 } finally {
 setCobrancaSubmitting(false);
 }
 };

 const handleCreateTicket = async (event: React.FormEvent<HTMLFormElement>) => {
  event.preventDefault();
  if (newTicketSubmitting) return;
  const fd = new FormData(event.currentTarget);

  const prop = properties.find((item) => item.id === selectedPropertyId);
  const propTitle = prop
   ? prop.displayId
    ? `${prop.displayId} - ${prop.title}`
    : prop.title
   : 'Imóvel não especificado';
  const requesterName =
   leads.find((item) => item.id === selectedRequesterId)?.name || 'Desconhecido';

  if (!selectedPropertyId) {
   toast.error('Selecione o imóvel do chamado.');
   return;
  }
  if (!selectedRequesterId) {
   toast.error('Selecione a parte responsável pelo chamado.');
   return;
  }

  setNewTicketSubmitting(true);
  try {
   const applied = await addTicket({
    type: fd.get('type') as string,
    category: fd.get('category') as string,
    property_id: selectedPropertyId,
    property_title: propTitle,
    // O financeiro usa o id do contrato para vincular a cobrança e o repasse;
    // o display id vai só no título legível.
    contract_id: selectedContractId || undefined,
    rental_id: selectedContract?.deal_id || undefined,
    // `requester_lead_id` é a parte que paga: é ele que o toca-financial usa
    // para criar a cobrança ou descontar do repasse.
    requester_lead_id: selectedRequesterId || undefined,
    requester: requesterName,
    profile: fd.get('profile') as string,
    status: 'Em Análise',
    priority: fd.get('priority') as string,
    description: fd.get('description') as string,
   } as any);
   if (!applied) return;

   setSelectedPropertyId('');
   setSelectedContractId('');
   setSelectedRequesterId('');
   setRequesterProfile('Inquilino');
   closeModal();
  } catch {
   // MaintenanceContext already surfaced the error; keep the form open for retry.
  } finally {
   setNewTicketSubmitting(false);
  }
 };

 const handleConfirmSchedule = async () => {
  if (!selectedItem || schedulingTicketId) return;
  if (!scheduleSupplierId) {
   toast.error('Selecione o profissional responsável pelo serviço.');
   return;
  }
  if (!scheduleDate) {
   toast.error('Informe a data agendada.');
   return;
  }

  setSchedulingTicketId(selectedItem.id);
  try {
   // O prestador vem do cadastro: o chamado guarda a referência e o nome
   // usado até aqui, para o histórico continuar legível.
   const scheduledAt = new Date(
    `${scheduleDate}T${scheduleTime || '09:00'}:00`,
   ).toISOString();
   const applied = await updateTicket(selectedItem.id, {
    status: 'Em Andamento',
    supplier_id: scheduleSupplierId,
    assigned_to: scheduleSupplier?.name ?? selectedItem.assigned_to,
    instructions: scheduleInstructions,
    scheduled_at: scheduledAt,
   });
   if (applied) closeModal();
  } catch {
   // Keep the scheduling drawer open after rollback/reconciliation.
  } finally {
   setSchedulingTicketId(null);
  }
 };

 const getStatusStyle = (status: string) => {
 switch (status) {
 case 'Em Análise': return 'bg-status-warning-soft text-status-warning border border-status-warning/30';
 case 'Em Andamento': return 'bg-primary-100 text-primary-800 border border-primary-200';
 case 'Concluído': return 'bg-status-ok-soft text-status-ok border border-status-ok/30';
 case 'Cancelado': return 'bg-status-critical-soft text-status-critical border border-status-critical/30';
 default: return 'bg-muted text-muted-foreground border border-border';
 }
 };

 const getPriorityIcon = (prioridade: string) => {
 switch (prioridade) {
 case 'Alta': return <AlertCircle className="w-4 h-4 text-status-critical" />;
 case 'Média': return <Clock className="w-4 h-4 text-status-warning" />;
 case 'Baixa': return <CheckCircle2 className="w-4 h-4 text-status-ok" />;
 default: return null;
 }
 };

 const renderModalHeader = (title: string, icon: React.ReactNode) => (
 <div className="px-6 py-4 border-b border-border flex items-start justify-between bg-card z-10">
 <div className="flex items-center gap-3">
 <div className="w-10 h-10 rounded-full bg-primary-50 flex items-center justify-center text-primary-600">
 {icon}
 </div>
 <div>
 <h2 className="text-xl font-bold text-foreground">{title}</h2>
 {selectedItem && (
 <p className="text-xs text-muted-foreground mt-0.5">{selectedItem.id} - {selectedItem.property_title}</p>
 )}
 </div>
 </div>
 <div className="flex items-center gap-2">
 <Button
 variant="outline"
 size="sm"
 onClick={toggleExpand}
 className={clsx(
 isExpanded && "text-primary-600 bg-primary-50 border-primary-200 hover:bg-primary-100 hover:text-primary-600"
 )}
 title={isExpanded ? "Recolher painel" : "Expandir painel"}
 >
 {isExpanded ? (
 <Minimize2 className="w-4 h-4" />
 ) : (
 <Maximize2 className="w-4 h-4" />
 )}
 <span className="hidden sm:inline">
 {isExpanded ? "Recolher" : "Expandir"}
 </span>
 </Button>
 <Button
 variant="ghost"
 size="icon"
 onClick={closeModal}
 disabled={modalMutationPending}
 className="size-8"
 >
 <X className="w-5 h-5" />
 </Button>
 </div>
 </div>
 );

 const filteredTickets = tickets.filter(t => {
 const matchesTab = activeTab === 'Todas' || t.type === activeTab;
 const matchesSearch = searchTerm === '' ||
 t.id.toLowerCase().includes(searchTerm.toLowerCase()) ||
 t.property_title.toLowerCase().includes(searchTerm.toLowerCase()) ||
 t.requester.toLowerCase().includes(searchTerm.toLowerCase());
 return matchesTab && matchesSearch;
 });

 const kpis = {
 total: tickets.length,
 emAnalise: tickets.filter(t => t.status === 'Em Análise').length,
 emAndamento: tickets.filter(t => t.status === 'Em Andamento').length,
 concluidas: tickets.filter(t => t.status === 'Concluído').length
 };

 return (
 <div className="flex flex-col h-full bg-muted/50 font-sans relative overflow-hidden">
 {/* Header */}
 <div className="flex-none px-6 lg:px-8 py-6 border-b border-border bg-card sticky top-0 z-10">
 <PageHeader
 title="Manutenção"
 description="Gerencie ordens de serviço, integre com atendimento e lance cobranças no financeiro"
 >
 <div className="flex items-center gap-3">
 <Button
 variant="outline"
 onClick={reloadTickets}
 disabled={isTicketsLoading}
 >
 <RefreshCw className={clsx("w-4 h-4", isTicketsLoading && "animate-spin")} /> Atualizar
 </Button>
 <Button
 variant="outline"
 onClick={() => { setAuditEntityId('__all_tickets__'); setAuditEntityLabel(''); }}
 >
 <History className="w-4 h-4" /> Histórico
 </Button>
 <Button onClick={() => openModal('nova')}>
 <Plus className="w-4 h-4" /> Nova Solicitação
 </Button>
 </div>
 </PageHeader>
 </div>

 <div className="flex-1 overflow-y-auto p-6 lg:p-8 space-y-8">
 {/* KPIs */}
 <AnimatePresence>
 {!focusMode && (
 <motion.div
 initial={{ opacity: 0, height: 0 }}
 animate={{ opacity: 1, height: "auto" }}
 exit={{ opacity: 0, height: 0 }}
 transition={{ duration: 0.3, ease: "easeInOut" }}
 >
 <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
 <div className="bg-card p-6 rounded-2xl border border-border shadow-sm flex flex-col gap-4">
 <div className="flex items-center gap-3">
 <div className="w-8 h-8 rounded-full bg-muted flex items-center justify-center">
 <FileText className="w-4 h-4 text-muted-foreground" />
 </div>
 <span className="text-sm font-medium text-muted-foreground">Total de Solicitações</span>
 </div>
 <h2 className="text-3xl font-bold text-foreground">{kpis.total}</h2>
 </div>

 <div className="bg-card p-6 rounded-2xl border border-border shadow-sm flex flex-col gap-4">
 <div className="flex items-center gap-3">
 <div className="w-8 h-8 rounded-full bg-status-warning-soft flex items-center justify-center">
 <Clock className="w-4 h-4 text-status-warning" />
 </div>
 <span className="text-sm font-medium text-muted-foreground">Em Análise</span>
 </div>
 <h2 className="text-3xl font-bold text-foreground">{kpis.emAnalise}</h2>
 </div>

 <div className="bg-card p-6 rounded-2xl border border-border shadow-sm flex flex-col gap-4">
 <div className="flex items-center gap-3">
 <div className="w-8 h-8 rounded-full bg-primary-50 flex items-center justify-center">
 <AlertCircle className="w-4 h-4 text-primary-600" />
 </div>
 <span className="text-sm font-medium text-muted-foreground">Em Andamento</span>
 </div>
 <h2 className="text-3xl font-bold text-foreground">{kpis.emAndamento}</h2>
 </div>

 <div className="bg-card p-6 rounded-2xl border border-border shadow-sm flex flex-col gap-4">
 <div className="flex items-center gap-3">
 <div className="w-8 h-8 rounded-full bg-status-ok-soft flex items-center justify-center">
 <CheckCircle2 className="w-4 h-4 text-status-ok" />
 </div>
 <span className="text-sm font-medium text-muted-foreground">Concluídas</span>
 </div>
 <h2 className="text-3xl font-bold text-foreground">{kpis.concluidas}</h2>
 </div>
 </div>
 </motion.div>
 )}
 </AnimatePresence>

 {/* Main Content Area */}
 <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden flex flex-col">
 <div className="p-4 border-b border-border bg-muted/50 flex items-center gap-6 overflow-x-auto">
 <button
 onClick={() => setActiveTab('Todas')}
 className={clsx(
 "flex items-center gap-2 px-4 py-2 text-sm font-bold rounded-lg shadow-sm border transition-colors whitespace-nowrap",
 activeTab === 'Todas' ? "bg-card text-foreground border-border" : "bg-transparent text-muted-foreground border-transparent hover:text-foreground"
 )}
 >
 <Wrench className="w-4 h-4" /> Todas
 </button>
 <button
 onClick={() => setActiveTab('Manutenção')}
 className={clsx(
 "flex items-center gap-2 px-4 py-2 text-sm font-bold rounded-lg shadow-sm border transition-colors whitespace-nowrap",
 activeTab === 'Manutenção' ? "bg-card text-foreground border-border" : "bg-transparent text-muted-foreground border-transparent hover:text-foreground"
 )}
 >
 <Wrench className="w-4 h-4" /> Manutenções
 </button>
 <button
 onClick={() => setActiveTab('Vistoria')}
 className={clsx(
 "flex items-center gap-2 px-4 py-2 text-sm font-bold rounded-lg shadow-sm border transition-colors whitespace-nowrap",
 activeTab === 'Vistoria' ? "bg-card text-foreground border-border" : "bg-transparent text-muted-foreground border-transparent hover:text-foreground"
 )}
 >
 <ClipboardCheck className="w-4 h-4" /> Vistorias
 </button>
 </div>

 <div className="p-4 border-b border-border">
 <div className="relative max-w-full">
 <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
 <input
 type="text"
 aria-label="Buscar manutenção por ID, imóvel ou solicitante"
 placeholder="Buscar por ID, imóvel ou solicitante..."
 value={searchTerm}
 onChange={(e) => setSearchTerm(e.target.value)}
 className="w-full pl-9 pr-4 py-2 bg-muted border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 transition-all"
 />
 </div>
 </div>

 <div className="overflow-x-auto">
 {ticketsError && (
 <div className="p-6">
 <EmptyState
 icon={AlertCircle}
 title="Não foi possível carregar"
 message={ticketsError}
 action={
 <Button onClick={reloadTickets}>
 Tentar novamente
 </Button>
 }
 />
 </div>
 )}

 {!ticketsError && isTicketsLoading && tickets.length === 0 && (
 <div className="p-6 space-y-3">
 <Skeleton className="h-14 w-full rounded-2xl" />
 <Skeleton className="h-14 w-full rounded-2xl" />
 <Skeleton className="h-14 w-full rounded-2xl" />
 </div>
 )}

 {!ticketsError && !(isTicketsLoading && tickets.length === 0) && (
 <>
 {isMobile ? (
 <div className="divide-y divide-border">
 {filteredTickets.map((item) => (
 <div key={item.id} className="p-4">
 <div className="flex items-start justify-between gap-3">
 <div className="min-w-0">
 <div className="flex items-center gap-2 min-w-0">
 {getPriorityIcon(item.priority)}
 <span className="text-sm font-bold text-foreground">{item.id}</span>
 <span className={clsx("px-2.5 py-1 text-micro font-bold rounded-full whitespace-nowrap", getStatusStyle(item.status))}>
 {item.status}
 </span>
 </div>
 <div className="text-xs text-muted-foreground mt-1">{item.type} • {item.category}</div>
 <div className="mt-2">
 <div className="text-sm font-medium text-foreground truncate" title={item.property_title}>{item.property_title}</div>
 <div className="text-xs text-primary-600 font-bold mt-0.5">{contractText(item.contract_id)}</div>
 </div>
 <div className="mt-2 text-xs text-muted-foreground">{item.requester} • {item.profile}</div>
 <div className="text-xs text-muted-foreground mt-0.5">{item.created_at ? new Date(item.created_at).toLocaleDateString('pt-BR') : ''}</div>
 </div>
 </div>
 <div className="mt-3 flex items-center justify-end gap-2">
 <Button variant="ghost" size="icon" onClick={() => openModal('chat', item)} className="text-muted-foreground hover:text-primary-600 hover:bg-primary-50" title="Atendimento / Mensagens">
 <MessageSquare className="w-4 h-4" />
 </Button>
 <Button variant="ghost" size="icon" onClick={() => openModal('agendamento', item)} className="text-muted-foreground hover:text-primary-600 hover:bg-primary-50" title="Gestão da Locação / Agendar">
 <Calendar className="w-4 h-4" />
 </Button>
 <Button variant="ghost" size="icon" onClick={() => openModal('cobranca', item)} className="text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft" title="Financeiro / Gerar Cobrança">
 <DollarSign className="w-4 h-4" />
 </Button>
 <Button
 variant="ghost"
 size="icon"
 onClick={() => { setAuditEntityId(item.id); setAuditEntityLabel(`${item.id} - ${item.category}`); }}
 className="text-muted-foreground"
 title="Histórico de alterações"
 >
 <History className="w-4 h-4" />
 </Button>
 </div>
 </div>
 ))}
 {filteredTickets.length === 0 && (
 <EmptyState
 icon={Search}
 message="Nenhuma solicitação encontrada."
 className="border-none bg-transparent my-6"
 />
 )}
 </div>
 ) : (
 <>
 <table className="w-full text-left border-collapse min-w-[1000px]">
 <thead>
 <tr className="border-b border-border text-muted-foreground text-xs font-medium tracking-wide">
 <th className="px-6 py-4">Ticket</th>
 <th className="px-6 py-4">Tipo/Categoria</th>
 <th className="px-6 py-4">Imóvel/Contrato</th>
 <th className="px-6 py-4">Solicitante</th>
 <th className="px-6 py-4">Status</th>
 <th className="px-6 py-4 text-right">Ações</th>
 </tr>
 </thead>
 <tbody className="divide-y divide-border">
 {filteredTickets.map((item) => (
 <tr key={item.id} className="hover:bg-muted/80 transition-colors group">
 <td className="px-6 py-4">
 <div className="flex items-center gap-2">
 {getPriorityIcon(item.priority)}
 <span className="text-sm font-bold text-foreground">{item.id}</span>
 </div>
 <div className="text-xs text-muted-foreground mt-0.5">{item.created_at ? new Date(item.created_at).toLocaleDateString('pt-BR') : ''}</div>
 </td>
 <td className="px-6 py-4">
 <div className="flex items-center gap-2">
 {item.type === 'Vistoria' ? <ClipboardCheck className="w-4 h-4 text-muted-foreground" /> : <Wrench className="w-4 h-4 text-muted-foreground" />}
 <span className="text-sm font-bold text-foreground">{item.type}</span>
 </div>
 <div className="text-xs text-muted-foreground mt-0.5">{item.category}</div>
 </td>
 <td className="px-6 py-4">
 <div className="text-sm font-medium text-foreground truncate max-w-[200px]" title={item.property_title}>{item.property_title}</div>
 <div className="text-xs text-primary-600 font-bold mt-0.5">{contractText(item.contract_id)}</div>
 </td>
 <td className="px-6 py-4">
 <div className="text-sm font-bold text-foreground">{item.requester}</div>
 <div className="text-micro font-bold uppercase tracking-wider text-muted-foreground mt-0.5">{item.profile}</div>
 </td>
 <td className="px-6 py-4">
 <span className={clsx("px-3 py-1 text-caption font-bold rounded-full whitespace-nowrap", getStatusStyle(item.status))}>
 {item.status}
 </span>
 </td>
 <td className="px-6 py-4 text-right">
 <div className="flex items-center justify-end gap-1">
 <Button variant="ghost" size="icon" onClick={() => openModal('chat', item)} className="size-9 text-muted-foreground hover:text-primary-600 hover:bg-primary-50" title="Atendimento / Mensagens">
 <MessageSquare className="w-4 h-4" />
 </Button>
 <Button variant="ghost" size="icon" onClick={() => openModal('agendamento', item)} className="size-9 text-muted-foreground hover:text-primary-600 hover:bg-primary-50" title="Gestão da Locação / Agendar">
 <Calendar className="w-4 h-4" />
 </Button>
 <Button variant="ghost" size="icon" onClick={() => openModal('cobranca', item)} className="size-9 text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft" title="Financeiro / Gerar Cobrança">
 <DollarSign className="w-4 h-4" />
 </Button>
 <Button
 variant="ghost"
 size="icon"
 onClick={() => { setAuditEntityId(item.id); setAuditEntityLabel(`${item.id} - ${item.category}`); }}
 className="size-9 text-muted-foreground"
 title="Histórico de alterações"
 >
 <History className="w-4 h-4" />
 </Button>
 </div>
 </td>
 </tr>
 ))}
 </tbody>
 </table>
 {filteredTickets.length === 0 && (
 <EmptyState
 icon={Search}
 message="Nenhuma solicitação encontrada."
 className="border-none bg-transparent my-6"
 />
 )}
 </>
 )}
 </>
 )}
 </div>
 </div>


 {/* Slide-over Modals */}
 <AnimatePresence>
 {activeModal !== 'none' && (
 <>
 <motion.div
 initial={{ opacity: 0 }}
 animate={{ opacity: 1 }}
 exit={{ opacity: 0 }}
 onClick={() => {
 if (!modalMutationPending) closeModal();
 }}
 className="fixed inset-0 bg-foreground/40 backdrop-blur-sm z-40"
 />
 <motion.div
 initial={{ x: '100%' }}
 animate={{ x: 0 }}
 exit={{ x: '100%' }}
 transition={{ type: "spring", damping: 25, stiffness: 200 }}
 className={clsx(
 "fixed inset-y-0 right-0 bg-card shadow-2xl z-50 flex flex-col border-l border-border transition-[max-width] duration-300 ease-in-out",
 isExpanded ? "w-full max-w-full" : (activeModal === 'chat' ? "w-full max-w-lg" : "w-full max-w-md")
 )}
 >

 {/* MODAL: NOVA SOLICITAÇÃO */}
 {activeModal === 'nova' && (
 <form
 className="flex flex-col h-full"
 onSubmit={handleCreateTicket}
 >
 <div className="px-6 py-4 border-b border-border flex items-start justify-between bg-card z-10">
 <div className="flex items-center gap-3">
 <div className="w-10 h-10 rounded-full bg-primary-50 flex items-center justify-center text-primary-600">
 <Plus className="w-5 h-5" />
 </div>
 <div>
 <h2 className="text-xl font-bold text-foreground">Nova Solicitação</h2>
 <p className="text-xs text-muted-foreground mt-0.5">Criar OS ou agendar Vistoria</p>
 </div>
 </div>
 <Button
 type="button"
 variant="ghost"
 size="icon"
 onClick={closeModal}
 disabled={newTicketSubmitting}
 className="size-8"
 >
 <X className="w-5 h-5" />
 </Button>
 </div>

 <div className="flex-1 overflow-y-auto p-6 space-y-6">
 <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
 <div>
 <label className="block text-sm font-bold text-foreground mb-1.5">Tipo *</label>
 <select name="type" className="w-full px-4 py-2.5 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none appearance-none" required>
 <option value="Manutenção">Manutenção</option>
 <option value="Vistoria">Vistoria</option>
 </select>
 </div>
 <div>
 <label className="block text-sm font-bold text-foreground mb-1.5">Prioridade *</label>
 <select name="priority" className="w-full px-4 py-2.5 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none appearance-none" required>
 <option value="Baixa">Baixa</option>
 <option value="Média">Média</option>
 <option value="Alta">Alta</option>
 </select>
 </div>
 </div>

 <div>
 <label className="block text-sm font-bold text-foreground mb-1.5">Categoria / Serviço *</label>
 <input type="text" name="category" placeholder="Ex: Elétrica, Hidráulica, Pintura..." className="w-full px-4 py-2.5 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none" required />
 </div>

 <div>
 <label className="block text-sm font-bold text-foreground mb-1.5">Imóvel Referência *</label>
 {contractProperty ? (
  <div className="px-4 py-2.5 bg-muted border border-border rounded-xl text-sm flex items-center justify-between gap-3">
   <span className="truncate text-foreground">
    {contractProperty.displayId ? `${contractProperty.displayId} - ${contractProperty.title}` : contractProperty.title}
   </span>
   <span className="shrink-0 text-micro font-bold uppercase tracking-wide text-muted-foreground">
    do contrato
   </span>
  </div>
 ) : (
  <PropertyPicker
   properties={properties}
   value={selectedPropertyId}
   includeNone={false}
   placeholder="Selecione um imóvel..."
   inputPlaceholder="Buscar por código ou título..."
   onChange={setSelectedPropertyId}
   buttonClassName="px-4 py-2.5 rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
  />
 )}
 {!contractProperty && propertyOwners.length > 0 && (
  <p className="mt-1.5 text-caption text-muted-foreground">
   Proprietário{propertyOwners.length > 1 ? "s" : ""}: {propertyOwners.map((owner) => owner.name).join(", ")}
  </p>
 )}
 </div>

 <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
 <div>
 <label className="block text-sm font-bold text-foreground mb-1.5">Contrato Referência</label>
 <Popover open={contractSearchOpen} onOpenChange={setContractSearchOpen}>
 <PopoverTrigger asChild>
 <button
 type="button"
 className="w-full px-4 py-2.5 bg-card border border-border rounded-xl text-sm flex items-center justify-between focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
 >
 {selectedContractId ? (
 <span className="truncate">
 {contractText(selectedContractId)}
 </span>
 ) : (
 <span className="text-muted-foreground">Opcional</span>
 )}
 <ChevronDown className="w-4 h-4 text-muted-foreground" />
 </button>
 </PopoverTrigger>
 <PopoverContent className="w-(--radix-popover-trigger-width) p-0 bg-card border border-border shadow-xl rounded-lg z-50" align="start">
 <Command className="w-full">
 <CommandInput placeholder="Buscar por cliente, imóvel ou código..." className="h-10 text-sm border-b-0 focus:ring-0" />
 <CommandList className="max-h-[200px] overflow-y-auto">
 <CommandEmpty className="py-3 text-sm text-muted-foreground text-center">Nenhum contrato encontrado.</CommandEmpty>
 <CommandGroup>
 {contracts.map((ctr) => (
 <CommandItem
 key={ctr.id}
 value={ctr.id}
 keywords={[describeContract(ctr)]}
 onSelect={() => {
 setSelectedContractId(ctr.id);
 setContractSearchOpen(false);
 }}
 className="px-3 py-2 text-sm text-foreground hover:bg-primary-50 hover:text-primary-700 cursor-pointer flex items-center justify-between group"
 >
 <div className="min-w-0">
 <p className="font-medium text-foreground truncate">{describeContract(ctr)}</p>
 <p className="text-micro text-muted-foreground truncate">{ctr.type || "Contrato"}</p>
 </div>
 <Check className={clsx("w-4 h-4 shrink-0", selectedContractId === ctr.id ? "opacity-100 text-primary-600" : "opacity-0")} />
 </CommandItem>
 ))}
 </CommandGroup>
 </CommandList>
 </Command>
 </PopoverContent>
 </Popover>
 </div>
 <div>
 <label className="block text-sm font-bold text-foreground mb-1.5">Perfil Solicitante *</label>
 <select
 name="profile"
 value={requesterProfile}
 onChange={(event) => setRequesterProfile(event.target.value)}
 className="w-full px-4 py-2.5 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none appearance-none"
 required
 >
 <option value="Inquilino">Inquilino</option>
 <option value="Proprietário">Proprietário</option>
 <option value="Sistema">Sistema</option>
 </select>
 </div>
 </div>

 <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
 <div className="col-span-2">
 <label className="block text-sm font-bold text-foreground mb-1.5">Nome do Solicitante *</label>
 <Popover open={requesterSearchOpen} onOpenChange={setRequesterSearchOpen}>
 <PopoverTrigger asChild>
 <button
 type="button"
 className="w-full px-4 py-2.5 bg-card border border-border rounded-xl text-sm flex items-center justify-between focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
 >
 {selectedRequesterId ? (
 <span className="truncate">
 {leads.find(l => l.id === selectedRequesterId)?.name}
 </span>
 ) : (
 <span className="text-muted-foreground">
  {selectedContractId ? 'Selecione a parte do contrato...' : 'Selecione um solicitante (Cliente)...'}
 </span>
 )}
 <ChevronDown className="w-4 h-4 text-muted-foreground" />
 </button>
 </PopoverTrigger>
 <PopoverContent className="w-(--radix-popover-trigger-width) p-0 bg-card border border-border shadow-xl rounded-lg z-50" align="start">
 <Command className="w-full">
 <CommandInput placeholder="Buscar por nome ou email..." className="h-10 text-sm border-b-0 focus:ring-0" />
 <CommandList className="max-h-[200px] overflow-y-auto">
 <CommandEmpty className="py-3 text-sm text-muted-foreground text-center">
  {selectedContractId ? 'Nenhuma parte vinculada a este contrato.' : 'Nenhum cliente encontrado.'}
 </CommandEmpty>
 <CommandGroup>
 {requesterOptions.map((lead) => (
 <CommandItem
 key={lead.id}
 value={lead.name}
 onSelect={() => {
 setSelectedRequesterId(lead.id);
 setRequesterSearchOpen(false);
 }}
 className="px-3 py-2 text-sm text-foreground hover:bg-primary-50 hover:text-primary-700 cursor-pointer flex items-center justify-between group"
 >
 <div className="min-w-0">
 <p className="font-medium text-foreground truncate">{lead.name}</p>
 <p className="text-micro text-muted-foreground truncate">{lead.email}</p>
 </div>
 <Check className={clsx("w-4 h-4 shrink-0", selectedRequesterId === lead.id ? "opacity-100 text-primary-600" : "opacity-0")} />
 </CommandItem>
 ))}
 </CommandGroup>
 </CommandList>
 </Command>
 </PopoverContent>
 </Popover>
 </div>
 </div>

 <div>
 <label className="block text-sm font-bold text-foreground mb-1.5">Descrição do Problema</label>
 <textarea name="description" rows={4} placeholder="Descreva os detalhes..." className="w-full px-4 py-3 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none resize-none"></textarea>
 </div>
 </div>

 <div className="px-6 py-4 border-t border-border flex gap-3 bg-muted">
 <Button
 type="button"
 variant="outline"
 onClick={closeModal}
 disabled={newTicketSubmitting}
 className="flex-1"
 >
 Cancelar
 </Button>
 <Button
 type="submit"
 className="flex-2"
 loading={newTicketSubmitting}
 loadingText="Criando..."
 disabled={newTicketSubmitting}
 >
 <Plus className="w-4 h-4" /> Criar Solicitação
 </Button>
 </div>
 </form>
 )}

 {/* MODAL: MENSAGENS / ATENDIMENTO */}
 {activeModal === 'chat' && selectedItem && (
 <>
 {renderModalHeader('Atendimento Integrado', <MessageSquare className="w-5 h-5" />)}
 <div className="flex-1 overflow-y-auto p-4 space-y-4 bg-muted/50">
 <div className="flex items-center justify-center">
 <span className="px-3 py-1 bg-muted text-muted-foreground text-micro font-bold rounded-full uppercase tracking-wider">Histórico de Mensagens</span>
 </div>

  {/* Atendimento — sem mensagens registradas (sem conversa simulada) */}
  <div className="flex flex-col items-center justify-center gap-2 rounded-2xl border border-dashed border-border bg-card p-8 text-center">
  <MessageSquare className="h-8 w-8 text-muted-foreground" />
  <p className="text-sm font-bold text-foreground">Nenhuma mensagem registrada</p>
  <p className="text-xs text-muted-foreground">Este atendimento ainda não possui histórico de conversas. Acompanhe o status e os anexos da solicitação.</p>
  {selectedItem.status && (
  <span className="mt-2 inline-flex items-center gap-1.5 rounded-full bg-status-warning-soft border border-status-warning/30 px-3 py-1 text-micro font-bold text-status-warning">
  <Wrench className="w-3 h-3" /> Status: {selectedItem.status}
  </span>
  )}
  </div>

 {selectedItem.attachments && selectedItem.attachments.length > 0 && (
 <div className="rounded-2xl border border-border bg-card p-4">
 <div className="flex items-center gap-2 text-sm font-bold text-foreground">
 <Paperclip className="w-4 h-4 text-primary-600" />
 Arquivos enviados pelo cliente ({selectedItem.attachments.length})
 </div>
 <div className="mt-3 grid grid-cols-2 gap-3">
 {selectedItem.attachments.map((attachment, index) => {
 const url = getMediaUrl(attachment);
 if (!url) return null;
 return (
 <a key={attachment} href={url} target="_blank" rel="noreferrer" className="overflow-hidden rounded-xl border border-border bg-card">
 <MediaImage src={url} alt={`Arquivo enviado ${index + 1}`} className="h-32 w-full object-cover" fallback={<div className="flex h-32 items-center justify-center text-xs text-muted-foreground">Arquivo indisponível</div>} />
 </a>
 );
 })}
 </div>
 </div>
 )}
 </div>

 {/* Chat Input */}
 <div className="px-4 py-4 border-t border-border bg-card flex items-center gap-2">
 <Button variant="ghost" size="icon" className="rounded-full text-muted-foreground hover:text-primary-600 hover:bg-primary-50">
 <Paperclip className="w-5 h-5" />
 </Button>
 <input
 type="text"
 placeholder="Digite sua mensagem..."
 className="flex-1 px-4 py-2.5 bg-muted border border-border rounded-full text-sm focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
 />
 <Button size="icon" className="rounded-full">
 <Send className="w-4 h-4" />
 </Button>
 </div>
 </>
 )}

 {/* MODAL: INTEGRAÇÃO FINANCEIRA / COBRANÇA */}
 {activeModal === 'cobranca' && selectedItem && (
 <>
 {renderModalHeader('Integração Financeira', <DollarSign className="w-5 h-5" />)}
 <div className="flex-1 overflow-y-auto p-6 space-y-6">

 <div className="bg-primary-50 border border-primary-100 p-4 rounded-xl">
 <h3 className="text-sm font-bold text-primary-900 mb-1">Lançamento de Cobrança</h3>
 <p className="text-xs text-primary-700">
 Defina quem será o responsável pelo custo deste serviço. O sistema "Automático" irá gerar a cobrança no módulo financeiro.
 </p>
 </div>

 <div>
 <label className="block text-sm font-bold text-foreground mb-1.5">Valor do Serviço *</label>
 <div className="relative">
 <span className="absolute left-4 top-1/2 -translate-y-1/2 text-muted-foreground font-bold">R$</span>
 <MoneyInput
 placeholder="0,00"
 value={cobrancaValor ?? undefined}
 onChange={(v) => setCobrancaValor(v ?? null)}
 className="w-full pl-10 pr-4 py-2.5 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
 />
 </div>
 </div>

 {/* Contexto do lançamento: contrato, imóvel e parte que o financeiro vai
     usar. Evita lançar em outro lugar por engano. */}
 <div className="rounded-xl border border-border bg-muted p-4 text-xs space-y-1">
 <p className="text-micro font-bold uppercase tracking-wider text-muted-foreground">Destino do lançamento</p>
 <p className="text-foreground">
 <span className="font-bold">Contrato:</span> {billingContractLabel}
 </p>
 <p className="text-foreground">
 <span className="font-bold">Imóvel:</span> {selectedItem.property_title || "não informado"}
 </p>
 <p className="text-foreground">
 <span className="font-bold">Parte responsável:</span> {billingPartyLabel}
 </p>
 </div>

 <div className="space-y-3">
 <label className="block text-sm font-bold text-foreground mb-1.5">Responsabilidade Financeira *</label>

 <label className="flex items-start gap-3 p-4 bg-card border border-border rounded-xl cursor-pointer hover:border-primary-600 transition-colors group">
 <input
 type="radio"
 name="charge_target"
 value="tenant"
 checked={cobrancaPayerRole === 'tenant'}
 onChange={() => setCobrancaPayerRole('tenant')}
 className="mt-0.5 text-primary-600 focus:ring-primary-600 border-border"
 />
 <div className="min-w-0">
 <span className="block text-sm font-bold text-foreground group-hover:text-primary-700">Cobrar do Inquilino</span>
 <span className="block text-xs text-muted-foreground mt-1">
 Soma o valor na próxima cobrança em aberto de {billingPartyLabel}, discriminado na composição.
 </span>

 </div>
 </label>

 <label className="flex items-start gap-3 p-4 bg-card border border-border rounded-xl cursor-pointer hover:border-primary-600 transition-colors group">
 <input
 type="radio"
 name="charge_target"
 value="owner"
 checked={cobrancaPayerRole === 'owner'}
 onChange={() => setCobrancaPayerRole('owner')}
 className="mt-0.5 text-primary-600 focus:ring-primary-600 border-border"
 />
 <div className="min-w-0">
 <span className="block text-sm font-bold text-foreground group-hover:text-primary-700">Descontar do Proprietário</span>
 <span className="block text-xs text-muted-foreground mt-1">
 Abate o valor no próximo repasse pendente ao proprietário do imóvel. Não gera cobrança ao inquilino.
 </span>

 </div>
 </label>
 </div>

 <div className="space-y-3">
 <label className="block text-sm font-bold text-foreground mb-1.5">Quem pagou o serviço *</label>
 <div className="grid grid-cols-3 gap-2">
 {([
  { id: 'imobiliaria' as AdvancedBy, label: 'Imobiliária' },
  { id: 'proprietario' as AdvancedBy, label: 'Proprietário' },
  { id: 'inquilino' as AdvancedBy, label: 'Inquilino' },
 ]).map((option) => (
  <label
   key={option.id}
   className={clsx(
    'flex cursor-pointer items-center justify-center gap-2 rounded-lg border px-3 py-2 text-label transition-colors',
    cobrancaAdvancedBy === option.id
     ? 'border-primary-600 bg-primary-50 font-bold text-primary-700'
     : 'border-border bg-card text-muted-foreground hover:border-primary-600',
   )}
  >
   <input
    type="radio"
    name="advanced_by"
    value={option.id}
    checked={cobrancaAdvancedBy === option.id}
    onChange={() => setCobrancaAdvancedBy(option.id)}
    className="sr-only"
   />
   {option.label}
  </label>
 ))}
 </div>
 <p className={clsx('text-caption', billingNature.movement ? 'text-muted-foreground' : 'font-bold text-status-warning')}>
  {billingNature.message}
 </p>
 </div>

 {/* Onde o custo entra: só aparecem os destinos que o contrato permite. */}
 <div className="space-y-3">
 <label className="block text-sm font-bold text-foreground mb-1.5">Como lançar no financeiro *</label>
 {destinationOptions.map((option) => {
  const disabled = !option.available || destinationsLoading;
  return (
   <label
    key={option.id}
    aria-disabled={disabled}
    className={clsx(
     "flex items-start gap-3 p-4 bg-card border rounded-xl transition-colors",
     disabled ? "border-border opacity-60 cursor-not-allowed" : "border-border cursor-pointer hover:border-primary-600 group",
     !disabled && cobrancaTarget === option.id && "border-primary-600 ring-1 ring-primary-600/30",
    )}
   >
    <input
     type="radio"
     name="billing_target"
     value={option.id}
     checked={cobrancaTarget === option.id}
     disabled={disabled}
     onChange={() => setCobrancaTarget(option.id)}
     className="mt-0.5 text-primary-600 focus:ring-primary-600 border-border disabled:cursor-not-allowed"
    />
    <div className="min-w-0">
     <span className={clsx("block text-sm font-bold", disabled ? "text-muted-foreground" : "text-foreground group-hover:text-primary-700")}>
      {option.label}
     </span>
     <span className="block text-xs text-muted-foreground mt-1">{option.hint}</span>
     {!disabled && cobrancaTarget === option.id && billingPreview && (
      <span className="mt-1.5 block text-xs font-medium text-primary-700">{billingPreview}</span>
     )}
    </div>
   </label>
  );
 })}
 {destinationsLoading && <p className="text-caption text-muted-foreground">Verificando o que o contrato permite…</p>}
 <p className="text-caption text-muted-foreground">
  {cobrancaPayerRole === 'owner'
   ? 'Se o custo final for menor que o retido, a diferença volta no próximo repasse pendente — sem repasse seguinte, é paga ao proprietário como crédito avulso.'
   : 'Se o custo final for menor que o cobrado, a diferença vira desconto na próxima cobrança em aberto — sem cobrança seguinte, é devolvida ao inquilino como crédito avulso.'}
 </p>
 {destinationBlocked && (
  <p className="text-caption font-bold text-status-warning">
   O contrato não permite este lançamento agora. A cobrança avulsa continua válida.
  </p>
 )}
 </div>

 <div>
 <label className="block text-sm font-bold text-foreground mb-1.5">Descrição na Fatura</label>
 <input
 type="text"
 value={cobrancaDesc}
 onChange={(e) => setCobrancaDesc(e.target.value)}
 placeholder={`Serviço de ${selectedItem.category} - OS ${selectedItem.id}`}
 className="w-full px-4 py-2.5 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
 />
 </div>
 </div>

 <div className="px-6 py-4 border-t border-border flex gap-3 bg-muted">
 <Button
 variant="outline"
 onClick={closeModal}
 disabled={schedulingTicketId === selectedItem.id}
 className="flex-1"
 >
 Cancelar
 </Button>
 <Button
 onClick={handleCreateCobranca}
 loading={cobrancaSubmitting}
 loadingText="Lançando..."
 disabled={cobrancaValor === null || destinationBlocked}
 className="flex-2"
 >
 <DollarSign className="w-4 h-4" /> Lançar no Financeiro
 </Button>
 </div>
 </>
 )}

 {/* MODAL: INTEGRAÇÃO LOCAÇÃO / AGENDAMENTO */}
 {activeModal === 'agendamento' && selectedItem && (
 <>
 {renderModalHeader('Gestão da Locação', <Calendar className="w-5 h-5" />)}
 <div className="flex-1 overflow-y-auto p-6 space-y-6">

 <div className="bg-muted border border-border p-4 rounded-xl flex items-center justify-between">
 <div>
 <p className="text-xs text-muted-foreground font-bold uppercase tracking-wider">Contrato Vinculado</p>
 <p className="text-sm font-bold text-primary-600 mt-1">{contractText(selectedItem.contract_id)}</p>
 </div>
 {(() => {
 const linkedContract = contracts.find(c => c.id === selectedItem.contract_id || c.display_id === selectedItem.contract_id);
 return (
 <Button
 variant="outline"
 size="sm"
 disabled={!linkedContract}
 onClick={() => {
 if (!linkedContract) return;
 navigate(`/contratos/${linkedContract.id}`);
 closeModal();
 }}
 >
 Ver Contrato
 </Button>
 );
 })()}
 </div>

 <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
 <div className="col-span-2">
 <SupplierPicker
 value={scheduleSupplierId}
 onChange={setScheduleSupplierId}
 onSupplierSelected={setScheduleSupplier}
 required
 />
 </div>

 {/* Campos nativos: o indicador do próprio controle já é o ícone. Ícone
     decorativo por cima duplicava a affordance. */}
 <div>
 <label htmlFor="schedule-date" className="block text-sm font-bold text-foreground mb-1.5">Data Agendada *</label>
 <input
 id="schedule-date"
 type="date"
 value={scheduleDate}
 onChange={(event) => setScheduleDate(event.target.value)}
 className="w-full px-4 py-2.5 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
 />
 </div>

 <div>
 <label htmlFor="schedule-time" className="block text-sm font-bold text-foreground mb-1.5">Horário *</label>
 <input
 id="schedule-time"
 type="time"
 value={scheduleTime}
 onChange={(event) => setScheduleTime(event.target.value)}
 className="w-full px-4 py-2.5 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
 />
 </div>
 </div>

 <div>
 <label className="block text-sm font-bold text-foreground mb-1.5">Instruções para o Fornecedor/Vistoriador</label>
 <textarea
 rows={4}
 value={scheduleInstructions}
 onChange={(event) => setScheduleInstructions(event.target.value)}
 placeholder="Descreva os detalhes do serviço a ser realizado ou itens específicos para a vistoria..."
 className="w-full px-4 py-3 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none resize-none"
 />
 </div>

 <div className="flex items-center gap-3 p-4 bg-status-ok-soft border border-status-ok/30 rounded-xl">
 <input type="checkbox" className="w-4 h-4 text-status-ok rounded border-status-ok/40 focus:ring-emerald-600" defaultChecked />
 <span className="text-sm font-medium text-status-ok">Notificar o {selectedItem.profile.toLowerCase()} automaticamente por WhatsApp sobre o agendamento.</span>
 </div>

 </div>

 <div className="px-6 py-4 border-t border-border flex gap-3 bg-muted">
 <Button
 variant="outline"
 onClick={closeModal}
 disabled={schedulingTicketId === selectedItem.id}
 className="flex-1"
 >
 Cancelar
 </Button>
 <Button
 onClick={handleConfirmSchedule}
 loading={schedulingTicketId === selectedItem.id}
 loadingText="Confirmando..."
 disabled={schedulingTicketId === selectedItem.id}
 className="flex-2"
 >
 <CheckCircle2 className="w-4 h-4" /> Confirmar Agendamento
 </Button>
 </div>
 </>
 )}

 </motion.div>
 </>
 )}
 </AnimatePresence>

 {/* Audit Trail Panel */}
 {auditEntityId && (
 <AuditTrailPanel
 entityId={auditEntityId === '__all_tickets__' ? undefined : auditEntityId}
 entityType={auditEntityId === '__all_tickets__' ? 'ticket' : undefined}
 entityLabel={auditEntityLabel}
 onClose={() => { setAuditEntityId(null); setAuditEntityLabel(''); }}
 />
 )}
 </div>
 </div>
 );
}
