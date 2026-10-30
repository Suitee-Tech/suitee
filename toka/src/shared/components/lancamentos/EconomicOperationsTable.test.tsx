import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { EconomicOperation } from "../../lib/lancamentos/presentation";
import type { Lancamento } from "../../lib/lancamentos/types";
import { EconomicOperationsTable } from "./EconomicOperationsTable";

const base = {
  status: "pendente",
  amount: 3200,
  currency: "BRL",
  counterparty: { kind: "tenant", label: "Inquilino" },
  origin: { entity_kind: "contract", entity_id: "contract-rent", label: "Locação" },
  timeline_summary: {},
  presentation: {
    operation_key: "batch:rent-1",
    operation_title: "Locação · Apartamento Ipês",
    operation_subtitle: "Contrato #contract · Competência 2026-09",
    contract_id: "contract-rent",
    flow: "rent",
  },
  permissions_required: ["finance:read"],
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-01T10:00:00Z",
} as const;

const receipt = {
  ...base,
  id: "receipt-1",
  kind: "transaction",
  kind_data: {
    transaction_type: "receita",
    direction: "entrada",
    manual: false,
  },
  source: {},
} as unknown as Lancamento;

const commission = {
  ...base,
  id: "commission-1",
  kind: "commission",
  counterparty: { kind: "broker", label: "Tiago Santini" },
  kind_data: {
    deductions_total: 0,
    participant_id: "broker-1",
    split_origin: "desconhecido",
    funding_required_cents: 320000,
    funded_amount_cents: 0,
    funding_outstanding_cents: 320000,
  },
  source: {},
} as unknown as Lancamento;

/** Linha de cobrança com o `Charge` original em `source` (de onde sai o atraso). */
const chargeLancamento = ({ days_overdue, due_at }: { days_overdue?: number; due_at?: string }) =>
  ({
    ...base,
    id: "charge-1",
    kind: "charge",
    status: "vencido",
    amount: 1200,
    due_at: due_at ?? "2026-10-01T00:00:00Z",
    kind_data: { reminders_count: 0 },
    source: {
      id: "charge-1",
      status: "OVERDUE",
      due_date: due_at ?? "2026-10-01T00:00:00Z",
      days_overdue,
      amount: 1200,
      total_amount: 1200,
    },
  }) as unknown as Lancamento;

const operation: EconomicOperation = {
  key: "batch:rent-1",
  title: "Locação · Apartamento Ipês",
  subtitle: "Contrato #contract · Competência 2026-09",
  items: [receipt, commission],
  primary: receipt,
  latestAt: receipt.updated_at,
  amount: 3200,
  openCount: 2,
  divergenceCount: 0,
  breakdown: {
    flow: "rent",
    gross: 3200,
    ownerPayable: 0,
    commissionPayable: 3200,
    partnerPayable: 0,
    agencyRevenue: 0,
    discounts: 0,
    rental: {
      allocationState: "unlinked",
      chargedTotal: 3200,
      receivedTotal: 0,
      openTotal: 3200,
      baseRent: 0,
      includedCharges: 0,
      separateCharges: 0,
      adminFee: undefined,
      agencyExpenses: undefined,
      agencyNet: undefined,
      ownerExpenses: 0,
      intermediationRequired: 3200,
      intermediationFunded: 0,
      intermediationOutstanding: 3200,
      components: [],
    },
  },
};

/** Cenário do filtro "a pagar": a comissão é a única linha e nada foi recebido. */
const firstInstallmentOperation: EconomicOperation = {
  key: "batch:rent-first",
  title: "Locação · Apartamento dos Ipês",
  subtitle: "Contrato #contract-rent · Vanessa Rodrigues Melo",
  items: [commission],
  primary: commission,
  latestAt: commission.updated_at,
  amount: 0,
  openCount: 1,
  divergenceCount: 0,
  breakdown: {
    flow: "rent",
    gross: 0,
    ownerPayable: 0,
    commissionPayable: 3200,
    partnerPayable: 0,
    agencyRevenue: 0,
    discounts: 0,
    rental: {
      allocationState: "unlinked",
      chargedTotal: 0,
      receivedTotal: 0,
      openTotal: 0,
      baseRent: 0,
      includedCharges: 0,
      separateCharges: 0,
      adminFee: undefined,
      agencyExpenses: undefined,
      agencyNet: undefined,
      ownerExpenses: 0,
      intermediationRequired: 3200,
      intermediationFunded: 0,
      intermediationOutstanding: 3200,
      components: [],
    },
  },
};

describe("EconomicOperationsTable rental UX", () => {
  it("separa valor informado, obrigação de intermediação e saldos ainda não apurados", () => {
    const onOpenRentalPlan = vi.fn();
    render(
      <EconomicOperationsTable
        operations={[operation]}
        onSelect={vi.fn()}
        onOpenRentalPlan={onOpenRentalPlan}
      />,
    );

    expect(screen.getByText("Destinos do valor")).toBeInTheDocument();
    expect(screen.getByText("Intermediação é uma obrigação vinculada — não é uma segunda entrada")).toBeInTheDocument();
    expect(screen.getAllByText("A apurar").length).toBeGreaterThan(1);
    expect(screen.getByText("Recebimento sem composição mensal reconciliada")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Locação · Apartamento Ipês/ }));
    expect(screen.getByText("Obrigação contratual · não somar como nova saída")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Revisar plano da locação/ }));
    expect(onOpenRentalPlan).toHaveBeenCalledWith("contract-rent");
  });

  it("explica a comissão sem recebimento em vez de deixar um valor zerado solto", () => {
    render(<EconomicOperationsTable operations={[firstInstallmentOperation]} onSelect={vi.fn()} />);

    // O cartão não pode resumir a operação a R$ 0,00 quando há R$ 3.200,00
    // de comissão contratada aguardando os aluguéis.
    expect(screen.getByText("Sem aluguel nesta competência")).toBeInTheDocument();
    expect(screen.getByText("+ R$ 3.200,00 de comissão a liberar")).toBeInTheDocument();

    // A cobertura da comissão vive em um bloco próprio, com o motivo do zero.
    expect(screen.getByText("Cobertura pelos aluguéis recebidos")).toBeInTheDocument();
    expect(screen.getByText("Nada a pagar ainda")).toBeInTheDocument();
    expect(screen.getByText(/Nenhum aluguel recebido até agora/)).toBeInTheDocument();
    expect(screen.getByText("Comissão contratual")).toBeInTheDocument();
    expect(screen.getByText("Coberta por recebimentos")).toBeInTheDocument();
    expect(screen.getByText("Nenhum aluguel recebido")).toBeInTheDocument();

    // Nenhum aluguel cobrado/recebido: sem três zeros sem explicação.
    expect(screen.getByText("Nenhum aluguel cobrado ou recebido nesta competência")).toBeInTheDocument();
    expect(screen.queryByText("Valor lançado")).not.toBeInTheDocument();
    expect(screen.queryByText("Recebido")).not.toBeInTheDocument();

    // A obrigação que trouxe a operação para o filtro "a pagar" fica visível.
    expect(screen.getByText("Obrigações desta operação")).toBeInTheDocument();
    expect(screen.getByText("Tiago Santini")).toBeInTheDocument();
    expect(screen.getByText("Nada liberado ainda · depende dos aluguéis recebidos")).toBeInTheDocument();

    // "Intermediação coberta" saiu da grade de destinos do aluguel.
    expect(screen.queryByText("Intermediação coberta")).not.toBeInTheDocument();
  });

  it("não mostra cobertura de comissão quando a operação não tem intermediação", () => {
    const operationWithoutCommission: EconomicOperation = {
      ...operation,
      items: [receipt],
      openCount: 0,
      breakdown: {
        ...operation.breakdown!,
        rental: { ...operation.breakdown!.rental!, intermediationRequired: 0, intermediationFunded: 0, intermediationOutstanding: 0 },
      },
    };

    render(<EconomicOperationsTable operations={[operationWithoutCommission]} onSelect={vi.fn()} />);

    expect(screen.queryByText("Cobertura pelos aluguéis recebidos")).not.toBeInTheDocument();
    expect(screen.queryByText("Obrigações desta operação")).not.toBeInTheDocument();
    expect(screen.queryByText(/comissão a liberar/)).not.toBeInTheDocument();
  });

  it("troca o rótulo genérico 'Vencido' pela régua do vencimento na linha da cobrança", () => {
    // Primeiro dia do atraso: "Venceu ontem" (o dia do vencimento é "Vence hoje").
    const overdue = chargeLancamento({ days_overdue: 1 });
    render(
      <EconomicOperationsTable
        operations={[{ ...operation, items: [overdue, commission], primary: overdue, amount: 1200 }]}
        onSelect={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /Locação · Apartamento Ipês/ }));

    expect(screen.getByText("Venceu ontem")).toBeInTheDocument();
    expect(screen.queryByText("Vencido")).not.toBeInTheDocument();
  });
});
