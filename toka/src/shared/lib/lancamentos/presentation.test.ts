import { describe, expect, it } from "vitest";

import { ContractStatus, type Contract } from "../../contracts/types";
import type { Lead, Property } from "../../data";
import type { Charge } from "../../services/charge.service";
import type { Commission } from "../../services/commission.service";
import type { OwnerTransfer } from "../../services/transfer.service";
import type { TransactionDTO } from "../../services/transaction.service";
import { mapLancamentos } from "./mappers";
import { enrichLancamentos, groupEconomicOperations, sortLancamentosForOperations } from "./presentation";

const contract = {
  id: "contract-sale",
  display_id: "CTR-42",
  deal_id: "deal-1",
  status: ContractStatus.ACTIVE,
  content: "",
  signatories: [],
  start_date: "2026-08-01",
  type: "sale",
  property_id: "property-1",
  client: "Comprador Exemplo",
  created_at: "2026-08-01T10:00:00Z",
  updated_at: "2026-08-01T10:00:00Z",
  tenant_id: "tenant-1",
} as Contract;

const property = { id: "property-1", title: "Casa Jardins" } as Property;
const leads = [{ id: "owner-1", name: "Marisa Silva" }] as Lead[];

const transaction: TransactionDTO = {
  id: "transaction-sale",
  contract_id: contract.id,
  deal_id: "deal-1",
  type: "receita",
  description: "Recebimento da venda",
  value: 980000,
  date: "2026-08-28T10:00:00Z",
  status: "PAID",
  tenant_id: "tenant-1",
  created_at: "2026-08-28T10:00:00Z",
  updated_at: "2026-08-28T10:00:00Z",
  operation_id: "ledger-batch-sale",
  economic_breakdown: {
    flow: "sale",
    gross_cents: 98000000,
    commission_pool_cents: 5880000,
    commission_payable_cents: 3528000,
    partner_payable_cents: 0,
    agency_retained_cents: 2352000,
    owner_payable_cents: 92120000,
    discount_cents: 0,
  },
};

const transfer: OwnerTransfer = {
  id: "transfer-sale",
  tenant_id: "tenant-1",
  contract_id: contract.id,
  owner_id: "owner-1",
  property_id: property.id,
  reference_month: "2026-08",
  settlement_type: "sale",
  gross_rent: 980000,
  admin_fee_percent: 0,
  admin_fee_amount: 0,
  maintenance_deductions: 0,
  other_deductions: 0,
  net_amount: 921200,
  status: "PENDING",
  created_at: "2026-08-28T10:01:00Z",
  updated_at: "2026-08-28T10:01:00Z",
};

const commission: Commission = {
  id: "commission-1",
  deal_id: "deal-1",
  contract_id: contract.id,
  broker_id: "broker-1",
  role: "fechamento",
  amount: 23520,
  currency: "BRL",
  status: "PENDING",
  tenant_id: "tenant-1",
  created_at: "2026-08-28T10:02:00Z",
  updated_at: "2026-08-28T10:02:00Z",
};

const rentalContract = {
  ...contract,
  id: "contract-rent",
  display_id: "LOC-7",
  deal_id: "deal-rent",
  type: "rent",
  property_id: "property-rent",
  client: "Inquilina Exemplo",
} as Contract;

const rentalProperty = { id: "property-rent", title: "Apartamento Ipês" } as Property;

describe("lancamentos presentation", () => {
  it("resolve nomes e reúne venda, comissão e repasse no mesmo evento econômico", () => {
    const enriched = enrichLancamentos(
      mapLancamentos({ transactions: [transaction], transfers: [transfer], commissions: [commission] }),
      { contracts: [contract], properties: [property], leads, agents: [{ id: "broker-1", name: "Bruno Lima", role: "broker" }] },
    );

    expect(enriched.find((item) => item.kind === "transfer")?.counterparty.label).toBe("Marisa Silva");
    expect(enriched.find((item) => item.kind === "commission")?.counterparty.label).toBe("Bruno Lima");
    expect(new Set(enriched.map((item) => item.presentation?.operation_key))).toEqual(
      new Set(["contract:contract-sale:sale"]),
    );

    const operations = groupEconomicOperations(enriched);
    expect(operations).toHaveLength(1);
    expect(operations[0].title).toBe("Venda · Casa Jardins");
    expect(operations[0].items).toHaveLength(3);
    expect(operations[0].breakdown).toMatchObject({
      gross: 980000,
      ownerPayable: 921200,
      commissionPayable: 35280,
      agencyRevenue: 58800,
      agencyNet: 23520,
    });
  });

  it.each([['PAID', 'allocated', 3000], ['PENDING', 'projected', 0]] as const)(
    "preserva administração e intermediação no recebimento manual de locação %s sem supor a margem",
    (status, allocationState, receivedTotal) => {
      const receipt: TransactionDTO = {
        ...transaction,
        contract_id: rentalContract.id,
        deal_id: rentalContract.deal_id!,
        value: 3000,
        status,
        economic_breakdown: {
          flow: 'rent', gross_cents: 300000, commission_pool_cents: 120000,
          commission_payable_cents: 0, partner_payable_cents: 0, agency_retained_cents: 120000,
          owner_payable_cents: 150000, discount_cents: 0, admin_fee_cents: 30000,
        },
      };
      const operations = groupEconomicOperations(enrichLancamentos(
        mapLancamentos({ transactions: [receipt] }),
        { contracts: [rentalContract], properties: [rentalProperty] },
      ));

      expect(operations).toHaveLength(1);
      expect(operations[0].breakdown?.rental).toMatchObject({
        allocationState, receivedTotal, chargedTotal: 3000, adminFee: 300,
        agencyRevenue: 1500, ownerPayable: 1500, availableToOwner: 1500,
      });
      expect(operations[0].breakdown?.rental?.agencyExpenses).toBeUndefined();
      expect(operations[0].breakdown?.rental?.agencyNet).toBeUndefined();
      expect(operations[0].primary.kind === 'transaction' && operations[0].primary.kind_data.economic_breakdown?.admin_fee_cents).toBe(30000);
    },
  );

  it("exibe administração e margem comprovada em aluguel manual sem intermediação", () => {
    const receipt: TransactionDTO = {
      ...transaction, contract_id: rentalContract.id, value: 3000,
      economic_breakdown: {
        flow: 'rent', gross_cents: 300000, commission_pool_cents: 0,
        commission_payable_cents: 0, partner_payable_cents: 0, agency_retained_cents: 0,
        owner_payable_cents: 265000, discount_cents: 5000, admin_fee_cents: 30000,
      },
    };
    const [operation] = groupEconomicOperations(enrichLancamentos(
      mapLancamentos({ transactions: [receipt] }), { contracts: [rentalContract] },
    ));

    expect(operation.breakdown?.rental).toMatchObject({
      allocationState: 'allocated', adminFee: 300, agencyRevenue: 300,
      agencyExpenses: 0, agencyNet: 300, ownerPayable: 2650,
    });
    expect(operation.breakdown?.discounts).toBe(50);
  });

  it("separa a comissão coberta da retenção da imobiliária sem usar o saldo de outros meses", () => {
    const receipt: TransactionDTO = {
      ...transaction, contract_id: rentalContract.id, value: 3000,
      economic_breakdown: {
        flow: 'rent', gross_cents: 300000, commission_pool_cents: 120000,
        commission_payable_cents: 90000, commission_funded_cents: 90000, commission_funding_known: true,
        partner_payable_cents: 0, agency_retained_cents: 30000,
        owner_payable_cents: 150000, discount_cents: 0, admin_fee_cents: 30000,
      },
    };
    const [operation] = groupEconomicOperations(enrichLancamentos(
      mapLancamentos({ transactions: [receipt] }), { contracts: [rentalContract] },
    ));
    expect(operation.breakdown).toMatchObject({ commissionPayable: 900, ownerPayable: 1500 });
    expect(operation.breakdown?.rental).toMatchObject({
      agencyRevenue: 1500, agencyExpenses: 900, agencyNet: 600,
      intermediationFunded: 900, availableToOwner: 1500,
    });
  });

  it("ordena vencidos e pendências antes do histórico concluído", () => {
    const rows = mapLancamentos({
      transactions: [
        { ...transaction, id: "paid", status: "PAID", economic_breakdown: undefined },
        { ...transaction, id: "pending", status: "PENDING", economic_breakdown: undefined },
        { ...transaction, id: "late", status: "OVERDUE", economic_breakdown: undefined },
      ],
    });
    expect(sortLancamentosForOperations(rows).map((item) => item.id)).toEqual(["late", "pending", "paid"]);
  });

  it("ordena por prioridade granular de status (pendente > aprovado > processado)", () => {
    const rows = mapLancamentos({
      transfers: [
        { ...transfer, id: "t-processado", status: "PROCESSED", net_amount: 1 },
        { ...transfer, id: "t-pendente", status: "PENDING", net_amount: 1 },
        { ...transfer, id: "t-aprovado", status: "APPROVED", net_amount: 1 },
      ],
    });
    expect(sortLancamentosForOperations(rows).map((item) => item.id)).toEqual([
      "t-pendente", "t-aprovado", "t-processado",
    ]);
  });

  it("reúne a comissão manual com a entrada de locação sem repetir seu espelho contábil", () => {
    const receipt: TransactionDTO = {
      ...transaction,
      id: "rent-receipt",
      contract_id: rentalContract.id,
      deal_id: rentalContract.deal_id!,
      type: "receita",
      description: "Recebimento - Locacao",
      value: 3200,
      status: "PENDING",
      operation_id: "manual-rent-operation",
      economic_breakdown: undefined,
    };
    const mirror: TransactionDTO = {
      ...receipt,
      id: "742d87cf-85db-5d5f-a165-9e77afbda88a",
      type: "comissao",
      description: "Comissão - atendente (Tiago Santini)",
      broker_id: "broker-rent",
      role: "atendente",
    };
    const rentalCommission: Commission = {
      ...commission,
      id: "rent-commission",
      deal_id: rentalContract.deal_id!,
      contract_id: rentalContract.id,
      broker_id: "broker-rent",
      role: "atendente",
      percentage: 100,
      amount: 3200,
      ledger_transaction_id: mirror.id,
      source_event_id: "284a8ad8-5119-5e6d-bf88-829be78df99b",
    };

    const enriched = enrichLancamentos(
      mapLancamentos({ transactions: [receipt, mirror], commissions: [rentalCommission] }),
      { contracts: [rentalContract], properties: [rentalProperty], agents: [{ id: "broker-rent", name: "Tiago Santini", role: "broker" }] },
    );
    const operations = groupEconomicOperations(enriched);

    expect(operations).toHaveLength(1);
    expect(operations[0].items.map((item) => item.kind).sort()).toEqual(["commission", "transaction"]);
    expect(operations[0].amount).toBe(3200);
    expect(operations[0].breakdown?.rental).toMatchObject({
      allocationState: "unlinked",
      chargedTotal: 3200,
      intermediationRequired: 3200,
      intermediationFunded: 0,
      intermediationOutstanding: 3200,
    });
    expect(operations[0].breakdown?.rental?.availableToOwner).toBeUndefined();
    expect(operations[0].breakdown?.rental?.agencyRevenue).toBeUndefined();
  });

  it("explica aluguel, encargos incluídos ou separados, reserva e receita efetiva", () => {
    const rentCharge: Charge = {
      id: "charge-rent",
      tenant_id: "tenant-1",
      contract_id: rentalContract.id,
      tenant_party_id: "tenant-party",
      owner_id: "owner-1",
      property_id: rentalProperty.id,
      type: "RENT",
      description: "Aluguel + Encargos",
      reference_month: "2026-09",
      amount: 3800,
      late_fee: 0,
      interest: 0,
      discount: 0,
      total_amount: 3800,
      due_date: "2026-09-10T12:00:00Z",
      paid_amount: 3800,
      status: "PAID",
      line_items: [
        { id: "rent", description: "Aluguel", amount_cents: 320000, source_type: "rental_base", economic_owner_id: "owner-1", settlement_mode: "arrecadacao" },
        { id: "condo", description: "Condomínio", amount_cents: 40000, source_type: "rental_bundled_item", source_id: "condominio", economic_owner_id: "owner-1", settlement_mode: "liquidacao_direta" },
        { id: "admin", description: "Taxa de administração", amount_cents: 20000, source_type: "rental_bundled_item", source_id: "admin_fee", economic_owner_id: "tenant-1", settlement_mode: "arrecadacao" },
      ],
      payments: [{
        id: "payment-rent",
        amount_cents: 380000,
        method: "pix",
        paid_at: "2026-09-10T12:00:00Z",
        allocations: [
          { id: "a-rent", payment_id: "payment-rent", charge_item_id: "rent", amount_cents: 320000, allocation_type: "owner_payable", beneficiary_party_id: "owner-1", rule_version_id: "rule-1" },
          { id: "a-condo", payment_id: "payment-rent", charge_item_id: "condo", amount_cents: 40000, allocation_type: "owner_payable", beneficiary_party_id: "owner-1", rule_version_id: "rule-1" },
          { id: "a-admin", payment_id: "payment-rent", charge_item_id: "admin", amount_cents: 20000, allocation_type: "revenue", beneficiary_party_id: "tenant-1", rule_version_id: "rule-1" },
        ],
      }],
      created_at: "2026-09-01T10:00:00Z",
      updated_at: "2026-09-10T12:00:00Z",
    };
    const separateUtility: Charge = {
      ...rentCharge,
      id: "charge-water",
      type: "UTILITY",
      description: "Água por consumo",
      amount: 150,
      total_amount: 150,
      paid_amount: 150,
      line_items: [{ id: "water", description: "Água por consumo", amount_cents: 15000, source_type: "utility_account", economic_owner_id: "owner-1", settlement_mode: "liquidacao_direta" }],
      payments: [{
        id: "payment-water",
        amount_cents: 15000,
        method: "pix",
        paid_at: "2026-09-10T12:00:00Z",
        allocations: [{ id: "a-water", payment_id: "payment-water", charge_item_id: "water", amount_cents: 15000, allocation_type: "owner_payable", beneficiary_party_id: "owner-1", rule_version_id: "rule-1" }],
      }],
    };
    const rentalTransfer: OwnerTransfer = {
      ...transfer,
      id: "transfer-rent",
      contract_id: rentalContract.id,
      property_id: rentalProperty.id,
      reference_month: "2026-09",
      settlement_type: "rent",
      gross_rent: 3950,
      admin_fee_amount: 200,
      other_deductions: 50,
      deduction_details: [{ description: "Reparo emergencial", amount: 50, type: "MAINTENANCE", reference_id: "repair-1" }],
      net_amount: 3150,
      economic_breakdown: {
        flow: "rent",
        gross_cents: 395000,
        owner_payable_cents: 375000,
        reserved_cents: 60000,
        available_cents: 315000,
        agency_revenue_cents: 20000,
        admin_fee_cents: 20000,
        commission_funded_cents: 0,
        commission_payable_cents: 0,
        partner_payable_cents: 0,
        agency_retained_cents: 20000,
        discount_cents: 0,
        deductions_cents: 60000,
      },
    };
    const agencyUtilityExpense: TransactionDTO = {
      ...transaction,
      id: "agency-water-expense",
      contract_id: rentalContract.id,
      type: "despesa",
      category: "Conta de consumo",
      description: "Água assumida pela imobiliária",
      value: 90,
      month: "2026-09",
      status: "PENDING",
      economic_breakdown: undefined,
      operation_id: undefined,
    };

    const operations = groupEconomicOperations(enrichLancamentos(
      mapLancamentos({ charges: [rentCharge, separateUtility], transfers: [rentalTransfer], transactions: [agencyUtilityExpense] }),
      { contracts: [rentalContract], properties: [rentalProperty], leads },
    ));

    expect(operations).toHaveLength(1);
    expect(operations[0].breakdown?.rental).toMatchObject({
      allocationState: "reconciled",
      chargedTotal: 3950,
      receivedTotal: 3950,
      baseRent: 3200,
      includedCharges: 600,
      separateCharges: 150,
      ownerPayable: 3750,
      availableToOwner: 3150,
      reservedForExpenses: 600,
      agencyRevenue: 200,
      agencyExpenses: 90,
      agencyNet: 110,
      adminFee: 200,
    });
    expect(operations[0].breakdown?.rental?.components).toEqual(expect.arrayContaining([
      expect.objectContaining({ description: "Condomínio", billing: "included", economicOwner: "owner" }),
      expect.objectContaining({ description: "Água por consumo", billing: "separate", economicOwner: "owner" }),
      expect.objectContaining({ description: "Reparo emergencial", billing: "withheld", economicOwner: "owner" }),
      expect.objectContaining({ description: "Água assumida pela imobiliária", billing: "agency_expense", economicOwner: "agency" }),
    ]));
  });
});
