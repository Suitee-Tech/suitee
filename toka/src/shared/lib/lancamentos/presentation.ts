import type { Contract } from "../../contracts/types";
import { formatCompetencia } from "../../utils/format";
import type { Lead, Property } from "../../data";
import type { Agent } from "../../types/agent";
import type { Charge } from "../../services/charge.service";
import type { Commission } from "../../services/commission.service";
import type { OwnerTransfer } from "../../services/transfer.service";
import type { TransactionDTO } from "../../services/transaction.service";

import type { Lancamento, LancamentoKind, LancamentoStatus } from "./types";

export interface LancamentoPresentationContext {
  contracts?: Contract[];
  properties?: Property[];
  leads?: Lead[];
  agents?: Agent[];
}

export interface EconomicOperationBreakdown {
  flow: "sale" | "rent" | string;
  gross: number;
  ownerPayable: number;
  commissionPayable: number;
  partnerPayable: number;
  agencyRevenue: number;
  agencyNet?: number;
  discounts: number;
  availableToOwner?: number;
  rental?: RentalEconomicSummary;
}

export type RentalAllocationState = "reconciled" | "allocated" | "projected" | "unlinked";

export interface RentalChargeComponent {
  id: string;
  description: string;
  amount: number;
  billing: "included" | "separate" | "withheld" | "agency_expense";
  economicOwner: "owner" | "agency" | "tenant" | "broker" | "reimbursement" | "unknown";
  settlementMode?: string;
  sourceType?: string;
  sourceId?: string;
  isBaseRent: boolean;
}

export interface RentalEconomicSummary {
  allocationState: RentalAllocationState;
  chargedTotal: number;
  receivedTotal: number;
  openTotal: number;
  baseRent: number;
  includedCharges: number;
  separateCharges: number;
  ownerPayable?: number;
  availableToOwner?: number;
  reservedForExpenses?: number;
  agencyRevenue?: number;
  agencyExpenses?: number;
  agencyNet?: number;
  adminFee?: number;
  ownerExpenses: number;
  intermediationRequired: number;
  intermediationFunded: number;
  intermediationOutstanding: number;
  components: RentalChargeComponent[];
}

export interface EconomicOperation {
  key: string;
  title: string;
  subtitle?: string;
  items: Lancamento[];
  primary: Lancamento;
  latestAt: string;
  amount: number;
  openCount: number;
  divergenceCount: number;
  breakdown?: EconomicOperationBreakdown;
}

const OPEN_STATUSES = new Set<LancamentoStatus>(["pendente", "aprovado", "parcial", "vencido"]);

const ROLE_LABELS: Record<string, string> = {
  captador: "Captação",
  co_captador: "Co-captação",
  visitador: "Visitas",
  atendente: "Atendimento",
  fechamento: "Fechamento",
  imobiliaria: "Imobiliária",
};

function clean(value?: string): string | undefined {
  const next = value?.trim();
  return next || undefined;
}

function includesAny(value: string | undefined, needles: string[]): boolean {
  const normalized = value?.toLocaleLowerCase("pt-BR") ?? "";
  return needles.some((needle) => normalized.includes(needle));
}

function contractFlow(contract?: Contract): "sale" | "rent" | undefined {
  if (includesAny(contract?.type, ["sale", "venda", "compra"])) return "sale";
  if (includesAny(contract?.type, ["rent", "loca", "aluguel"])) return "rent";
  return undefined;
}

function contractIdOf(lancamento: Lancamento, contracts: Contract[]): string | undefined {
  if (lancamento.kind === "charge") return (lancamento.source as Charge).contract_id;
  if (lancamento.kind === "commission") {
    const commission = lancamento.source as Commission;
    return commission.contract_id || contracts.find((contract) => contract.deal_id === commission.deal_id)?.id;
  }
  if (lancamento.kind === "transfer") return (lancamento.source as OwnerTransfer).contract_id;
  if (lancamento.kind === "transaction") return (lancamento.source as TransactionDTO).contract_id || undefined;
  return lancamento.origin.entity_kind === "contract" ? lancamento.origin.entity_id : undefined;
}

function propertyIdOf(lancamento: Lancamento, contract?: Contract): string | undefined {
  if (lancamento.kind === "charge") return (lancamento.source as Charge).property_id || contract?.property_id;
  if (lancamento.kind === "transfer") return (lancamento.source as OwnerTransfer).property_id || contract?.property_id;
  return contract?.property_id;
}

function referenceMonthOf(lancamento: Lancamento): string | undefined {
  if (lancamento.kind === "charge") return clean((lancamento.source as Charge).reference_month);
  if (lancamento.kind === "transfer") return clean((lancamento.source as OwnerTransfer).reference_month);
  if (lancamento.kind === "transaction") return clean((lancamento.source as TransactionDTO).month);
  return undefined;
}

function flowOf(lancamento: Lancamento, contract?: Contract): "sale" | "rent" | string | undefined {
  if (lancamento.kind === "transfer") {
    const transfer = lancamento.source as OwnerTransfer;
    return transfer.economic_breakdown?.flow || transfer.settlement_type || contractFlow(contract);
  }
  if (lancamento.kind === "transaction") {
    return (lancamento.source as TransactionDTO).economic_breakdown?.flow || contractFlow(contract);
  }
  return contractFlow(contract);
}

function shortContract(contract?: Contract, fallback?: string): string | undefined {
  const displayId = clean(contract?.display_id);
  if (displayId) return displayId;
  const id = clean(contract?.id) || clean(fallback);
  return id ? `#${id.slice(0, 8)}` : undefined;
}

function partyById(id: string | undefined, leads: Lead[]): Lead | undefined {
  return id ? leads.find((lead) => lead.id === id || lead.accountId === id) : undefined;
}

function partyByName(name: string | undefined, leads: Lead[], agents: Agent[]): Lead | Agent | undefined {
  const normalized = name?.trim().toLocaleLowerCase("pt-BR");
  if (!normalized) return undefined;
  return (
    leads.find((lead) => lead.id === name || lead.name.trim().toLocaleLowerCase("pt-BR") === normalized) ||
    agents.find((agent) => agent.id === name || agent.name.trim().toLocaleLowerCase("pt-BR") === normalized)
  );
}

function operationKey(
  lancamento: Lancamento,
  contractId: string | undefined,
  flow: string | undefined,
  referenceMonth: string | undefined,
): string {
  // Contrato + fluxo representam a origem econômica autoritativa. O
  // operation_id identifica o lote de escrituração e não deve dividir a venda
  // em outro cartão separado das obrigações geradas no mesmo evento.
  if (contractId && flow === "sale") return `contract:${contractId}:sale`;
  if (contractId && flow === "rent" && referenceMonth) return `contract:${contractId}:rent:${referenceMonth}`;
  if (contractId && flow === "rent" && lancamento.kind === "commission") {
    const linkedOperationID = lancamento.kind_data.linked_operation_id?.trim();
    if (linkedOperationID) return `batch:${linkedOperationID}`;
  }
  if (lancamento.kind === "transaction") {
    const transaction = lancamento.source as TransactionDTO;
    if (transaction.operation_id) return `batch:${transaction.operation_id}`;
  }
  if (lancamento.kind === "commission") {
    const commission = lancamento.source as Commission;
    if (contractId) return `contract:${contractId}:commission`;
    if (commission.deal_id) return `deal:${commission.deal_id}:commission`;
  }
  if (contractId && referenceMonth) return `contract:${contractId}:${referenceMonth}`;
  return `${lancamento.kind}:${lancamento.id}`;
}

function operationTitle(flow: string | undefined, propertyTitle: string | undefined, lancamento: Lancamento): string {
  const subject = propertyTitle || lancamento.origin.label || lancamento.counterparty.label;
  if (flow === "sale") return `Venda · ${subject}`;
  if (flow === "rent") return `Locação · ${subject}`;
  if (lancamento.kind === "commission") return `Comissões · ${subject}`;
  if (lancamento.kind === "transaction" && lancamento.kind_data.manual) return lancamento.counterparty.label;
  return subject;
}

/** Resolve nomes e relações já carregados no app, sem alterar a linha de origem. */
export function enrichLancamentos(
  lancamentos: Lancamento[],
  context: LancamentoPresentationContext,
): Lancamento[] {
  const contracts = context.contracts ?? [];
  const properties = context.properties ?? [];
  const leads = context.leads ?? [];
  const agents = context.agents ?? [];

  return lancamentos.map((lancamento) => {
    const contractId = contractIdOf(lancamento, contracts);
    const contract = contracts.find((item) => item.id === contractId);
    const propertyId = propertyIdOf(lancamento, contract);
    const property = properties.find((item) => item.id === propertyId);
    const propertyTitle = clean(property?.title) || clean(contract?.property_title);
    const referenceMonth = referenceMonthOf(lancamento);
    const flow = flowOf(lancamento, contract);
    const contractLabel = shortContract(contract, contractId);
    const subtitle = [contractLabel ? `Contrato ${contractLabel}` : undefined, clean(contract?.client), referenceMonth ? `Competência ${formatCompetencia(referenceMonth)}` : undefined]
      .filter(Boolean)
      .join(" · ") || undefined;

    let counterparty = lancamento.counterparty;
    if (lancamento.kind === "charge") {
      const charge = lancamento.source as Charge;
      const tenant = partyById(charge.tenant_party_id, leads);
      counterparty = {
        ...counterparty,
        id: tenant?.id || charge.tenant_party_id || counterparty.id,
        label: clean(tenant?.name) || clean(contract?.client) || counterparty.label,
      };
    } else if (lancamento.kind === "commission") {
      const commission = lancamento.source as Commission;
      const agent = agents.find((item) => item.id === commission.broker_id);
      const lead = partyById(commission.broker_id, leads);
      const role = commission.role ? ROLE_LABELS[commission.role] || commission.role : undefined;
      counterparty = {
        ...counterparty,
        label: clean(agent?.name) || clean(lead?.name) || (role ? `Corretor · ${role}` : counterparty.label),
      };
    } else if (lancamento.kind === "transfer") {
      const transfer = lancamento.source as OwnerTransfer;
      const owner = partyById(transfer.owner_id, leads);
      counterparty = { ...counterparty, id: owner?.id || transfer.owner_id, label: clean(owner?.name) || counterparty.label };
    } else if (lancamento.kind === "transaction") {
      const transaction = lancamento.source as TransactionDTO;
      const party = partyByName(transaction.payer_name, leads, agents);
      counterparty = {
        ...counterparty,
        id: party?.id,
        label: clean((party as Lead | Agent | undefined)?.name) || counterparty.label,
      };
    }

    const title = operationTitle(flow, propertyTitle, { ...lancamento, counterparty });
    const originLabel = flow === "sale"
      ? `Venda${propertyTitle ? ` · ${propertyTitle}` : ""}`
      : flow === "rent"
        ? `Locação${propertyTitle ? ` · ${propertyTitle}` : ""}${referenceMonth ? ` · ${formatCompetencia(referenceMonth)}` : ""}`
        : propertyTitle
          ? `${propertyTitle}${contractLabel ? ` · Contrato ${contractLabel}` : ""}`
          : lancamento.origin.label;

    return {
      ...lancamento,
      counterparty,
      origin: { ...lancamento.origin, entity_id: contractId || lancamento.origin.entity_id, label: originLabel },
      presentation: {
        operation_key: operationKey(lancamento, contractId, flow, referenceMonth),
        operation_title: title,
        operation_subtitle: subtitle,
        contract_id: contractId,
        property_title: propertyTitle,
        reference_month: referenceMonth,
        flow,
      },
    };
  });
}

function safeTime(value?: string): number {
  const parsed = value ? Date.parse(value) : NaN;
  return Number.isFinite(parsed) ? parsed : 0;
}

function statusPriority(status: LancamentoStatus): number {
  if (status === "vencido") return 0;               // em atraso — mais urgente
  if (status === "pendente") return 1;              // a vencer / aguardando
  if (status === "parcial") return 2;               // pago parcialmente
  if (status === "aprovado") return 3;              // aprovado — aguarda processamento
  if (status === "processado") return 4;
  if (status === "pago") return 5;                  // concluído
  if (status === "revertido") return 6;
  return 7;                                          // cancelado
}

/** Ordenação operacional: urgências primeiro; histórico recente depois. */
export function sortLancamentosForOperations(lancamentos: Lancamento[]): Lancamento[] {
  return [...lancamentos].sort((left, right) => {
    const priority = statusPriority(left.status) - statusPriority(right.status);
    if (priority !== 0) return priority;
    const leftOpen = OPEN_STATUSES.has(left.status);
    const rightOpen = OPEN_STATUSES.has(right.status);
    if (leftOpen || rightOpen) {
      // Itens em aberto: o mais próximo do vencimento (futuro) sobe primeiro;
      // itens sem vencimento (transferências, comissões) vêm em seguida, por
      // recência — mantém o "agora" no topo e separa o histórico.
      const leftDue = left.due_at ? safeTime(left.due_at) : Number.POSITIVE_INFINITY;
      const rightDue = right.due_at ? safeTime(right.due_at) : Number.POSITIVE_INFINITY;
      const due = leftDue - rightDue;
      if (due !== 0) return due;
    }
    const recency = safeTime(right.settled_at || right.updated_at || right.created_at) - safeTime(left.settled_at || left.updated_at || left.created_at);
    return recency || `${left.kind}:${left.id}`.localeCompare(`${right.kind}:${right.id}`);
  });
}

function rentalComponentOwner(
  charge: Charge,
  item: NonNullable<Charge["line_items"]>[number],
): RentalChargeComponent["economicOwner"] {
  if (item.allocation_type === "revenue") return "agency";
  if (item.allocation_type === "commission_payable") return "broker";
  if (item.allocation_type === "owner_payable") return "owner";
  // Custo de terceiro adiantado pela imobiliária: o valor não é do
  // proprietário nem receita própria — é reembolso do que ela desembolsou.
  if (item.allocation_type === "receivable") return "reimbursement";
  if (item.economic_owner_id && item.economic_owner_id === charge.owner_id) return "owner";
  if (item.economic_owner_id && item.economic_owner_id === charge.tenant_id) return "agency";
  if (item.economic_owner_id && item.economic_owner_id === charge.tenant_party_id) return "tenant";
  return "unknown";
}

function isBaseRentItem(item: NonNullable<Charge["line_items"]>[number]): boolean {
  return item.source_type === "rental_base" || item.description.trim().toLocaleLowerCase("pt-BR") === "aluguel";
}

function chargeComponents(charges: Charge[]): RentalChargeComponent[] {
  return charges.flatMap<RentalChargeComponent>((charge): RentalChargeComponent[] => {
    const included = charge.type?.toUpperCase() === "RENT";
    if (!charge.line_items?.length) {
      return [{
        id: `${charge.id}:principal`,
        description: included ? "Aluguel" : charge.description || "Encargo",
        amount: charge.amount,
        billing: included ? "included" as const : "separate" as const,
        economicOwner: included ? "owner" as const : "unknown" as const,
        isBaseRent: included,
      }];
    }
    return charge.line_items.map((item, index) => ({
      id: `${charge.id}:${item.id || item.source_id || index}`,
      description: item.description,
      amount: item.amount_cents / 100,
      billing: included ? "included" as const : "separate" as const,
      economicOwner: rentalComponentOwner(charge, item),
      settlementMode: item.settlement_mode,
      sourceType: item.source_type,
      sourceId: item.source_id,
      isBaseRent: included && isBaseRentItem(item),
    }));
  });
}

function rentalBreakdownOf(items: Lancamento[]): EconomicOperationBreakdown {
  const charges = items.filter((item) => item.kind === "charge").map((item) => item.source as Charge);
  const transfers = items.filter((item) => item.kind === "transfer").map((item) => item.source as OwnerTransfer);
  const transactions = items.filter((item) => item.kind === "transaction");
  const commissions = items.filter((item) => item.kind === "commission");
  const components: RentalChargeComponent[] = [
    ...chargeComponents(charges),
    ...transfers.flatMap((transfer) => (transfer.deduction_details ?? []).map((deduction, index) => ({
      id: `${transfer.id}:deduction:${deduction.reference_id || index}`,
      description: deduction.description,
      amount: deduction.amount,
      billing: "withheld" as const,
      economicOwner: "owner" as const,
      settlementMode: "liquidacao_direta",
      sourceType: deduction.type,
      sourceId: deduction.reference_id,
      isBaseRent: false,
    }))),
    ...transactions
      .filter((item) => item.kind === "transaction" && item.kind_data.transaction_type === "despesa" && item.kind_data.category === "Conta de consumo")
      .map((item) => ({
        id: `${item.id}:agency-expense`,
        description: (item.source as TransactionDTO).description || item.counterparty.label,
        amount: item.amount,
        billing: "agency_expense" as const,
        economicOwner: "agency" as const,
        sourceType: "utility_account",
        isBaseRent: false,
      })),
  ];
  const transferBreakdown = transfers.find((transfer) => transfer.economic_breakdown)?.economic_breakdown;
  const economicReceipt = transactions.find((item) => item.kind_data.direction === "entrada" && item.kind_data.economic_breakdown?.flow === "rent");
  const transactionBreakdown = economicReceipt?.kind_data.economic_breakdown;

  const listedChargeCents = charges.length > 0
    ? charges.reduce((sum, charge) => sum + Math.round((charge.total_amount ?? charge.amount) * 100), 0)
    : transactions
      .filter((item) => item.kind === "transaction" && item.kind_data.direction === "entrada")
      .reduce((sum, item) => sum + Math.round(item.amount * 100), 0);
  const anyTransactionCents = transactions.reduce((sum, item) => sum + Math.round(item.amount * 100), 0);
  const chargedCents = listedChargeCents || transferBreakdown?.gross_cents || anyTransactionCents;

  const chargeReceivedCents = charges.reduce((sum, charge) => {
    const payments = charge.payments ?? [];
    if (payments.length > 0) return sum + payments.reduce((paymentSum, payment) => paymentSum + payment.amount_cents, 0);
    return sum + Math.round((charge.paid_amount ?? 0) * 100);
  }, 0);
  const transactionReceivedCents = transactions
    .filter((item) => item.kind === "transaction" && item.kind_data.direction === "entrada" && (item.status === "pago" || item.status === "processado"))
    .reduce((sum, item) => sum + Math.round(item.amount * 100), 0);
  const receivedCents = Math.max(chargeReceivedCents, transactionReceivedCents);

  let allocatedOwnerCents = 0;
  let allocatedAgencyCents = 0;
  let allocatedCommissionCents = 0;
  let allocatedCommissionFundedCents = 0;
  let allocatedPartnerCents = 0;
  let allocatedReservedCents = 0;
  let allocationCount = 0;
  for (const charge of charges) {
    for (const payment of charge.payments ?? []) {
      for (const allocation of payment.allocations ?? []) {
        allocationCount++;
        const lineItem = charge.line_items?.find((item) => item.id === allocation.charge_item_id);
        if (allocation.allocation_type === "owner_payable") {
          allocatedOwnerCents += allocation.amount_cents;
          if (lineItem?.settlement_mode === "liquidacao_direta") allocatedReservedCents += allocation.amount_cents;
        } else if (allocation.allocation_type === "revenue") {
          allocatedAgencyCents += allocation.amount_cents;
          if (allocation.purpose === "rental_intermediation_broker") allocatedCommissionFundedCents += allocation.amount_cents;
        } else if (allocation.allocation_type === "commission_payable") {
          allocatedCommissionCents += allocation.amount_cents;
        } else if (allocation.allocation_type === "partner_payable" || allocation.allocation_type === "royalty_payable") {
          allocatedPartnerCents += allocation.amount_cents;
        }
      }
    }
  }

  const projectedOwnerCents = Math.round(components
    .filter((component) => component.economicOwner === "owner" && component.billing !== "withheld")
    .reduce((sum, component) => sum + component.amount, 0) * 100);
  const projectedAgencyCents = Math.round(components
    .filter((component) => component.economicOwner === "agency" && component.billing !== "agency_expense")
    .reduce((sum, component) => sum + component.amount, 0) * 100);
  const projectedReservedCents = Math.round(components
    .filter((component) => component.economicOwner === "owner" && (component.settlementMode === "liquidacao_direta" || component.billing === "withheld"))
    .reduce((sum, component) => sum + component.amount, 0) * 100);

  const legacyReceipt = transactions.find((item) => item.kind === "transaction" && item.kind_data.direction === "entrada");
  const legacySource = legacyReceipt?.source as TransactionDTO | undefined;
  const legacyHasComposition = !!legacySource && [legacySource.gross_value, legacySource.admin_fee, legacySource.maintenance_deductions, legacySource.utility_deductions]
    .some((value) => typeof value === "number");
  const legacyGrossCents = Math.round((legacySource?.gross_value ?? legacyReceipt?.amount ?? 0) * 100);
  const legacyAdminCents = Math.round((legacySource?.admin_fee ?? 0) * 100);
  const legacyExpenseCents = Math.round(((legacySource?.maintenance_deductions ?? 0) + (legacySource?.utility_deductions ?? 0)) * 100);

  let allocationState: RentalAllocationState = "unlinked";
  let ownerPayableCents: number | undefined;
  let availableToOwnerCents: number | undefined;
  let reservedCents: number | undefined;
  let agencyRevenueCents: number | undefined;

  if (transferBreakdown) {
    allocationState = "reconciled";
    ownerPayableCents = transferBreakdown.owner_payable_cents;
    availableToOwnerCents = transferBreakdown.available_cents;
    reservedCents = transferBreakdown.reserved_cents;
    agencyRevenueCents = transferBreakdown.agency_revenue_cents || transferBreakdown.agency_retained_cents;
  } else if (transactionBreakdown) {
    allocationState = economicReceipt?.status === "pago" || economicReceipt?.status === "processado" ? "allocated" : "projected";
    ownerPayableCents = transactionBreakdown.owner_payable_cents;
    availableToOwnerCents = transactionBreakdown.owner_payable_cents;
    reservedCents = 0;
    agencyRevenueCents = transactionBreakdown.agency_retained_cents + transactionBreakdown.commission_payable_cents + (transactionBreakdown.admin_fee_cents ?? 0);
  } else if (allocationCount > 0) {
    allocationState = "allocated";
    ownerPayableCents = allocatedOwnerCents;
    reservedCents = allocatedReservedCents;
    availableToOwnerCents = Math.max(0, allocatedOwnerCents - allocatedReservedCents);
    agencyRevenueCents = allocatedAgencyCents;
  } else if (charges.length > 0) {
    allocationState = "projected";
    ownerPayableCents = projectedOwnerCents;
    reservedCents = projectedReservedCents;
    availableToOwnerCents = Math.max(0, projectedOwnerCents - projectedReservedCents);
    agencyRevenueCents = projectedAgencyCents;
  } else if (legacyHasComposition) {
    allocationState = "projected";
    ownerPayableCents = Math.max(0, legacyGrossCents - legacyAdminCents - legacyExpenseCents);
    availableToOwnerCents = ownerPayableCents;
    reservedCents = legacyExpenseCents;
    agencyRevenueCents = legacyAdminCents;
  }

  const intermediationRequiredCents = commissions.reduce((sum, item) => {
    if (item.kind !== "commission") return sum;
    return sum + (item.kind_data.funding_required_cents ?? Math.round(item.amount * 100));
  }, 0);
  const fundedByCommissionCents = commissions.reduce((sum, item) => {
    if (item.kind !== "commission") return sum;
    return sum + (item.kind_data.funded_amount_cents ?? 0);
  }, 0);
  const intermediationFundedCents = Math.max(
    transactionBreakdown?.commission_funded_cents ?? 0,
    transferBreakdown?.commission_funded_cents ?? 0,
    allocatedCommissionFundedCents,
    allocatedCommissionCents,
    fundedByCommissionCents,
  );
  const intermediationOutstandingCents = Math.max(0, intermediationRequiredCents - intermediationFundedCents);

  const componentAdminFeeCents = Math.round(components
    .filter((component) => component.sourceId === "admin_fee")
    .reduce((sum, component) => sum + component.amount, 0) * 100);
  const hasAdminFeeEvidence = transferBreakdown?.admin_fee_cents !== undefined || transactionBreakdown?.admin_fee_cents !== undefined || componentAdminFeeCents > 0 || legacySource?.admin_fee !== undefined;
  const adminFeeCents = transferBreakdown?.admin_fee_cents
    ?? transactionBreakdown?.admin_fee_cents
    ?? (componentAdminFeeCents > 0 ? componentAdminFeeCents : legacyAdminCents);
  const ownerExpenseCents = Math.max(projectedReservedCents, legacyExpenseCents, reservedCents ?? 0);
  const ownExpenseCents = Math.round(components
    .filter((component) => component.billing === "agency_expense")
    .reduce((sum, component) => sum + component.amount, 0) * 100);
  // Only the broker funding attributed to this receipt belongs in its margin.
  // Contract-wide funded balances can include later rents and must not be
  // subtracted from every month again. Legacy unknown splits remain explicit.
  const receiptBrokerExpenseCents = transactionBreakdown?.commission_funding_known
    ? transactionBreakdown.commission_funded_cents ?? 0
    : transferBreakdown?.commission_funded_cents ?? allocatedCommissionFundedCents;
  const agencyExpenseCents = ownExpenseCents + receiptBrokerExpenseCents;
  const unknownBrokerExpense = !!transactionBreakdown && transactionBreakdown.commission_pool_cents > 0 && !transactionBreakdown.commission_funding_known;
  const baseRentCents = Math.round(components.filter((component) => component.isBaseRent).reduce((sum, component) => sum + component.amount, 0) * 100);
  const includedChargesCents = Math.round(components.filter((component) => component.billing === "included" && !component.isBaseRent).reduce((sum, component) => sum + component.amount, 0) * 100);
  const separateChargesCents = Math.round(components.filter((component) => component.billing === "separate").reduce((sum, component) => sum + component.amount, 0) * 100);

  const rental: RentalEconomicSummary = {
    allocationState,
    chargedTotal: chargedCents / 100,
    receivedTotal: receivedCents / 100,
    openTotal: Math.max(0, chargedCents - receivedCents) / 100,
    baseRent: baseRentCents / 100,
    includedCharges: includedChargesCents / 100,
    separateCharges: separateChargesCents / 100,
    ownerPayable: ownerPayableCents === undefined ? undefined : ownerPayableCents / 100,
    availableToOwner: availableToOwnerCents === undefined ? undefined : availableToOwnerCents / 100,
    reservedForExpenses: reservedCents === undefined ? undefined : reservedCents / 100,
    agencyRevenue: agencyRevenueCents === undefined ? undefined : agencyRevenueCents / 100,
    agencyExpenses: unknownBrokerExpense || (allocationState === "unlinked" && agencyExpenseCents === 0) ? undefined : agencyExpenseCents / 100,
    agencyNet: unknownBrokerExpense || agencyRevenueCents === undefined ? undefined : (agencyRevenueCents - agencyExpenseCents) / 100,
    adminFee: hasAdminFeeEvidence ? adminFeeCents / 100 : undefined,
    ownerExpenses: ownerExpenseCents / 100,
    intermediationRequired: intermediationRequiredCents / 100,
    intermediationFunded: intermediationFundedCents / 100,
    intermediationOutstanding: intermediationOutstandingCents / 100,
    components,
  };

  return {
    flow: "rent",
    gross: rental.chargedTotal,
    ownerPayable: rental.ownerPayable ?? 0,
    commissionPayable: (transactionBreakdown?.commission_funding_known
      ? Math.max(transactionBreakdown.commission_payable_cents, intermediationRequiredCents)
      : transferBreakdown?.commission_payable_cents ?? Math.max(allocatedCommissionCents, intermediationRequiredCents)) / 100,
    partnerPayable: (transferBreakdown?.partner_payable_cents ?? allocatedPartnerCents) / 100,
    agencyRevenue: rental.agencyRevenue ?? 0,
    discounts: transactionBreakdown ? transactionBreakdown.discount_cents / 100 : charges.reduce((sum, charge) => sum + (charge.discount || 0), 0),
    availableToOwner: rental.availableToOwner,
    rental,
  };
}

function breakdownOf(items: Lancamento[]): EconomicOperationBreakdown | undefined {
  if (items.some((item) => item.presentation?.flow === "rent")) {
    return rentalBreakdownOf(items);
  }

  const transaction = items.find(
    (item) => item.kind === "transaction" && item.kind_data.economic_breakdown,
  );
  if (transaction?.kind === "transaction" && transaction.kind_data.economic_breakdown) {
    const value = transaction.kind_data.economic_breakdown;
    return {
      flow: value.flow,
      gross: value.gross_cents / 100,
      ownerPayable: value.owner_payable_cents / 100,
      commissionPayable: value.commission_payable_cents / 100,
      partnerPayable: value.partner_payable_cents / 100,
      agencyRevenue: (value.agency_retained_cents + value.commission_payable_cents) / 100,
      agencyNet: value.agency_retained_cents / 100,
      discounts: value.discount_cents / 100,
    };
  }

  const transfer = items.find((item) => item.kind === "transfer" && item.kind_data.economic_breakdown);
  if (transfer?.kind === "transfer" && transfer.kind_data.economic_breakdown) {
    const value = transfer.kind_data.economic_breakdown;
    return {
      flow: value.flow,
      gross: value.gross_cents / 100,
      ownerPayable: value.owner_payable_cents / 100,
      commissionPayable: value.commission_payable_cents / 100,
      partnerPayable: value.partner_payable_cents / 100,
      agencyRevenue: (value.agency_retained_cents + value.commission_payable_cents) / 100,
      agencyNet: value.agency_retained_cents / 100,
      discounts: value.discount_cents / 100,
      availableToOwner: value.available_cents / 100,
    };
  }
  return undefined;
}

export function groupEconomicOperations(lancamentos: Lancamento[]): EconomicOperation[] {
  const groups = new Map<string, Lancamento[]>();
  for (const lancamento of sortLancamentosForOperations(lancamentos)) {
    const key = lancamento.presentation?.operation_key || `${lancamento.kind}:${lancamento.id}`;
    const group = groups.get(key) ?? [];
    group.push(lancamento);
    groups.set(key, group);
  }

  return Array.from(groups, ([key, items]) => {
    const primary = items.find((item) => item.kind === "transaction" && item.kind_data.economic_breakdown) || items[0];
    const breakdown = breakdownOf(items);
    const latestAt = items.reduce((latest, item) => {
      const candidate = item.settled_at || item.updated_at || item.created_at;
      return safeTime(candidate) > safeTime(latest) ? candidate : latest;
    }, primary.updated_at || primary.created_at);
    return {
      key,
      title: primary.presentation?.operation_title || primary.origin.label || primary.counterparty.label,
      subtitle: primary.presentation?.operation_subtitle,
      items,
      primary,
      latestAt,
      amount: breakdown?.gross ?? primary.amount,
      openCount: items.filter((item) => OPEN_STATUSES.has(item.status)).length,
      divergenceCount: items.filter((item) => item.timeline_summary.divergence_flag).length,
      breakdown,
    };
  }).sort((left, right) => {
    const leftUrgent = left.items.some((item) => item.status === "vencido") ? 0 : left.openCount > 0 ? 1 : 2;
    const rightUrgent = right.items.some((item) => item.status === "vencido") ? 0 : right.openCount > 0 ? 1 : 2;
    return leftUrgent - rightUrgent || safeTime(right.latestAt) - safeTime(left.latestAt) || left.key.localeCompare(right.key);
  });
}

export interface MonthlyBucket {
  month: string;
  label: string;
  entrada: number;
  saida: number;
  saldo: number;
  receber: number;
  pagar: number;
  vencidos: number;
  count: number;
  /** Soma por tipo de evento econômico no mês. */
  byKind: Record<LancamentoKind, number>;
  /** Valores ainda em aberto por tipo (obrigações não quitadas). */
  open: Record<LancamentoKind, number>;
}

const MONTH_LABELS = ["Jan", "Fev", "Mar", "Abr", "Mai", "Jun", "Jul", "Ago", "Set", "Out", "Nov", "Dez"];

const monthOf = (l: Lancamento): string => {
  const ref = l.presentation?.reference_month;
  if (ref && ref.length >= 7) return ref.slice(0, 7);
  const date = l.settled_at || l.due_at || l.created_at;
  if (!date) return "sem-data";
  try {
    const d = new Date(date);
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
  } catch {
    return "sem-data";
  }
};

const monthLabel = (month: string): string => {
  if (month === "sem-data") return "Sem competência";
  const [year, m] = month.split("-");
  return `${MONTH_LABELS[Number(m) - 1] ?? m}/${year}`;
};

const isSaida = (l: Lancamento): boolean => {
  if (l.kind === "commission" || l.kind === "transfer" || l.kind === "royalty") return true;
  if (l.kind === "transaction") return l.kind_data.direction === "saida";
  return false;
};

// groupByMonth agrega os lançamentos por competência (mês) — visão mensal
// resumida: entradas (aluguel/caixa recebido) × saídas (comissão/repasse/etc.)
// e o saldo do mês, com o que ainda está em aberto.
export function groupByMonth(lancamentos: Lancamento[]): MonthlyBucket[] {
  const groups = new Map<string, Lancamento[]>();
  for (const l of lancamentos) {
    const key = monthOf(l);
    const list = groups.get(key) ?? [];
    list.push(l);
    groups.set(key, list);
  }

  const emptyKind = (): Record<LancamentoKind, number> => ({
    charge: 0,
    commission: 0,
    transfer: 0,
    royalty: 0,
    transaction: 0,
  });

  return Array.from(groups, ([month, items]) => {
    const byKind = emptyKind();
    const open = emptyKind();
    let entrada = 0;
    let saida = 0;
    let receber = 0;
    let pagar = 0;
    let vencidos = 0;
    for (const l of items) {
      const value = l.net_amount ?? l.amount ?? 0;
      const openish = OPEN_STATUSES.has(l.status);
      byKind[l.kind] += value;
      if (openish) open[l.kind] += value;
      if (isSaida(l)) {
        saida += value;
        if (openish) pagar += value;
      } else {
        entrada += value;
        if (openish) receber += value;
      }
      if (l.status === "vencido") vencidos += value;
    }
    return {
      month,
      label: monthLabel(month),
      entrada,
      saida,
      saldo: entrada - saida,
      receber,
      pagar,
      vencidos,
      count: items.length,
      byKind,
      open,
    };
  }).sort((a, b) => (a.month === "sem-data" ? 1 : b.month === "sem-data" ? -1 : b.month.localeCompare(a.month)));
}

// lancamentoMonth expõe o mês de competência de um lançamento (para os
// filtros da visão mensal).
export const lancamentoMonth = monthOf;
