import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Transaction } from "../../types/transaction";
import type { OwnerTransfer } from "../../services/transfer.service";
import { ConciliacaoSection } from "./ConciliacaoSection";

const transaction = (id: string, status: Transaction["status"]): Transaction => ({
  id,
  contractId: "contract-1",
  type: "receita",
  description: `Recebível ${id}`,
  value: 500,
  date: "2026-09-01",
  dueDate: "2026-09-10",
  status,
});

const transfer = (overrides: Partial<OwnerTransfer> = {}): OwnerTransfer => ({
  id: "transfer-1",
  tenant_id: "tenant-1",
  source_charge_id: "charge-1",
  contract_id: "contract-1",
  owner_id: "owner-1",
  property_id: "property-1",
  reference_month: "2026-09",
  gross_rent: 1_000,
  admin_fee_percent: 10,
  admin_fee_amount: 1_000,
  maintenance_deductions: 0,
  other_deductions: 0,
  net_amount: 0,
  status: "PENDING",
  owner_balance: { total_cents: 0, reserved_cents: 0, blocked_cents: 0, available_cents: 0 },
  created_at: "2026-09-01T12:00:00Z",
  updated_at: "2026-09-01T12:00:00Z",
  ...overrides,
});

function renderSection({
  transactions = [],
  transfers = [],
}: {
  transactions?: Transaction[];
  transfers?: OwnerTransfer[];
} = {}) {
  render(
    <ConciliacaoSection
      contracts={[]}
      transactions={transactions}
      charges={[]}
      transfers={transfers}
      focusMode={false}
      conciliacaoSearch=""
      conciliacaoStatusFilter="Todos"
      conciliacaoStatusOpen={false}
      filtersDispatch={vi.fn()}
      setExportConfig={vi.fn()}
    />,
  );
}

/** Valor do card de estatística (o mesmo rótulo também aparece nas linhas). */
function statValue(label: string): string | undefined {
  const card = screen
    .getAllByText(label)
    .map((node) => node.closest("div"))
    .find((node) => node?.querySelector("h3"));
  return card?.querySelector("h3")?.textContent ?? undefined;
}

describe("ConciliacaoSection", () => {
  it("diferencia baixa de conciliação bancária e não altera o lançamento", () => {
    renderSection({
      transactions: [
        transaction("paid", "Pago"),
        transaction("pending", "Pendente"),
        transaction("late", "Em Atraso"),
        transaction("reversed", "Estornado"),
      ],
    });

    expect(screen.getByText("Conferência manual de baixas")).toBeInTheDocument();
    expect(screen.getAllByText("Baixado").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Aguardando baixa").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Vencido").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Encerrado").length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: /Conciliar/i })).not.toBeInTheDocument();
    expect(screen.getByText(/não importa nem cruza extrato bancário/i)).toBeInTheDocument();
  });

  it("não conta repasse zerado como pendente e mostra a categoria Sem saldo", () => {
    renderSection({ transfers: [transfer()] });

    // A linha aparece com a categoria própria, com a subdescrição curta.
    expect(screen.getAllByText("Sem saldo").length).toBeGreaterThan(0);
    expect(screen.getByText("Nada a repassar nesta competência")).toBeInTheDocument();
    // Os cards continuam contando o repasse sem saldo fora de pendentes.
    expect(statValue("Total de Movimentações")).toBe("1");
    expect(statValue("Aguardando baixa")).toBe("0");
    expect(statValue("Sem saldo")).toBe("1");
    expect(statValue("Encerradas")).toBe("0");
    expect(statValue("Baixadas")).toBe("0");
  });

  it("mantém o repasse com valor a repassar aguardando baixa", () => {
    renderSection({ transfers: [transfer({ net_amount: 900, owner_balance: { total_cents: 90_000, reserved_cents: 0, blocked_cents: 0, available_cents: 90_000 } })] });

    expect(statValue("Aguardando baixa")).toBe("1");
    expect(statValue("Sem saldo")).toBe("0");
    expect(screen.queryByText("Nada a repassar nesta competência")).not.toBeInTheDocument();
  });

  it("considera o repasse zerado concluído como baixado", () => {
    renderSection({
      transfers: [transfer({ status: "PROCESSED", settled_without_balance: true })],
    });

    expect(statValue("Baixadas")).toBe("1");
    expect(statValue("Sem saldo")).toBe("0");
    expect(screen.queryByText("Nada a repassar nesta competência")).not.toBeInTheDocument();
  });

  it("mantém a reserva de contas administradas como pendente", () => {
    renderSection({
      transfers: [
        transfer({
          owner_balance: { total_cents: 0, reserved_cents: 5_000, blocked_cents: 0, available_cents: 0 },
        }),
      ],
    });

    expect(statValue("Aguardando baixa")).toBe("1");
    expect(statValue("Sem saldo")).toBe("0");
    expect(screen.queryByText("Nada a repassar nesta competência")).not.toBeInTheDocument();
  });
});
