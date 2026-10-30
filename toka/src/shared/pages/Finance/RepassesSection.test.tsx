import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

import type { OwnerTransfer } from "../../services/transfer.service";
import type { Transaction } from "../../types/transaction";
import type { FinanceConfirmOptions } from "./sectionTypes";

vi.mock("../../permissions/Can", () => ({
  Can: ({ children }: { children: React.ReactNode }) => children,
}));

import { RepassesSection } from "./RepassesSection";

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
  admin_fee_amount: 100,
  maintenance_deductions: 0,
  other_deductions: 0,
  net_amount: 900,
  status: "PENDING",
  owner_balance: {
    total_cents: 90_000,
    reserved_cents: 0,
    blocked_cents: 0,
    available_cents: 90_000,
  },
  created_at: "2026-09-01T12:00:00Z",
  updated_at: "2026-09-01T12:00:00Z",
  ...overrides,
});

const legacyTransfer: Transaction = {
  id: "legacy-1",
  contractId: "contract-1",
  type: "repasse",
  description: "Repasse legado",
  value: 900,
  date: "2026-09-05",
  status: "Pendente",
};

function renderSection({
  transfers = [],
  transactions = [],
  openProcessRepasse = vi.fn(),
  processTransfer = vi.fn().mockResolvedValue(undefined),
  openConfirm = vi.fn(),
}: {
  transfers?: OwnerTransfer[];
  transactions?: Transaction[];
  openProcessRepasse?: (transferId: string, transfer: OwnerTransfer) => void;
  processTransfer?: (id: string, evidence?: { proof_url?: string; note?: string }) => Promise<void>;
  openConfirm?: (options: FinanceConfirmOptions) => void;
} = {}) {
  render(
    <RepassesSection
      transfers={transfers}
      contracts={[{ id: "contract-1", client: "Proprietária", property_title: "Apto 101" } as never]}
      transactions={transactions}
      focusMode={false}
      loadingInvoices={false}
      repasseSearch=""
      repasseStatusFilter="Todos"
      repasseStatusOpen={false}
      filtersDispatch={vi.fn()}
      approveTransfer={vi.fn().mockResolvedValue(undefined)}
      processTransfer={processTransfer}
      openProcessRepasse={openProcessRepasse}
      setSelectedTransaction={vi.fn()}
      setExportConfig={vi.fn()}
      setAudit={vi.fn()}
      openConfirm={openConfirm}
      closeConfirm={vi.fn()}
    />,
  );
}

describe("RepassesSection", () => {
  it("mantém repasse sem cobertura como A apurar e somente leitura", () => {
    renderSection({
      transfers: [transfer({ source_charge_id: undefined, owner_balance: undefined })],
    });

    expect(screen.getByText("Sem conciliação · A apurar")).toBeInTheDocument();
    expect(screen.getByText(/permanece.*somente leitura/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Aprovar" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Processar" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /novo repasse/i })).not.toBeInTheDocument();
  });

  it("processa somente um repasse aprovado e conciliado pelo fluxo com comprovante", () => {
    const onProcess = vi.fn();
    const approved = transfer({ status: "APPROVED" });
    renderSection({ transfers: [approved], openProcessRepasse: onProcess });

    fireEvent.click(screen.getByRole("button", { name: "Processar" }));
    expect(onProcess).toHaveBeenCalledWith("transfer-1", approved);
  });

  it("mostra o repasse zero processado como liquidado e sem ação de baixa", () => {
    renderSection({
      transfers: [transfer({
        status: "PROCESSED",
        net_amount: 0,
        processed_at: "2026-09-25T14:17:00Z",
        processed_by: "system",
        owner_balance: { total_cents: 0, reserved_cents: 0, blocked_cents: 0, available_cents: 0 },
      })],
    });

    expect(screen.getByText("Liquidado sem saldo")).toBeInTheDocument();
    expect(screen.getByText("Nada a transferir nesta competência")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Aprovar" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Processar" })).not.toBeInTheDocument();
  });

  it("mostra o repasse zero com o marcador do backend como liquidado mesmo em payload novo", () => {
    renderSection({
      transfers: [transfer({
        status: "PROCESSED",
        net_amount: 0,
        settled_without_balance: true,
        processed_by: "operador-1",
      })],
    });

    expect(screen.getByText("Liquidado sem saldo")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Concluir sem saldo" })).not.toBeInTheDocument();
  });

  it("oferece a conclusão sem saldo para repasse zerado ainda pendente", () => {
    renderSection({
      transfers: [transfer({
        status: "PENDING",
        net_amount: 0,
        owner_balance: { total_cents: 0, reserved_cents: 0, blocked_cents: 0, available_cents: 0 },
      })],
    });

    expect(screen.getByText("Sem saldo")).toBeInTheDocument();
    expect(screen.getByText("Nada a transferir nesta competência")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Concluir sem saldo" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Aprovar" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Processar" })).not.toBeInTheDocument();
  });

  it("conclui o repasse zerado sem comprovante e com confirmação irreversível", async () => {
    const processTransfer = vi.fn().mockResolvedValue(undefined);
    const closeConfirm = vi.fn();
    let confirmOptions: FinanceConfirmOptions | null = null;
    renderSection({
      transfers: [transfer({
        status: "APPROVED",
        net_amount: 0,
        owner_balance: { total_cents: 0, reserved_cents: 0, blocked_cents: 0, available_cents: 0 },
      })],
      processTransfer,
      openConfirm: (options) => { confirmOptions = options; },
    });

    fireEvent.click(screen.getByRole("button", { name: "Concluir sem saldo" }));
    expect(confirmOptions).not.toBeNull();
    const options = confirmOptions as unknown as FinanceConfirmOptions;
    expect(options.title).toBe("Concluir repasse sem saldo");
    expect(options.description).toMatch(/nenhuma movimentação de caixa/i);
    expect(options.description).toMatch(/não pode ser desfeita/i);
    expect(closeConfirm).not.toHaveBeenCalled();
    expect(processTransfer).not.toHaveBeenCalled();

    await options.onConfirm();
    expect(processTransfer).toHaveBeenCalledTimes(1);
    // Sem comprovante: o backend conclui zerado sem exigir evidência.
    expect(processTransfer).toHaveBeenCalledWith("transfer-1", {});
  });

  it("avisa o operador quando a conclusão sem saldo falha", async () => {
    const processTransfer = vi.fn().mockRejectedValue(new Error("409"));
    let confirmOptions: FinanceConfirmOptions | null = null;
    renderSection({
      transfers: [transfer({
        status: "PENDING",
        net_amount: 0,
        owner_balance: { total_cents: 0, reserved_cents: 0, blocked_cents: 0, available_cents: 0 },
      })],
      processTransfer,
      openConfirm: (options) => { confirmOptions = options; },
    });

    fireEvent.click(screen.getByRole("button", { name: "Concluir sem saldo" }));
    const options = confirmOptions as unknown as FinanceConfirmOptions;
    // A exceção continua subindo para o diálogo manter o operador no fluxo.
    await expect(options.onConfirm()).rejects.toThrow("409");
    expect(processTransfer).toHaveBeenCalledWith("transfer-1", {});
  });

  it("não oferece conclusão sem saldo quando há reserva de contas administradas", () => {
    renderSection({
      transfers: [transfer({
        status: "PENDING",
        net_amount: 0,
        owner_balance: { total_cents: 0, reserved_cents: 5_000, blocked_cents: 0, available_cents: 0 },
      })],
    });

    expect(screen.getByText("A apurar")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Concluir sem saldo" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Aprovar" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Processar" })).not.toBeInTheDocument();
  });

  it("mantém aprovação e processamento do repasse com saldo real", () => {
    renderSection({ transfers: [transfer({ status: "PENDING" }), transfer({ id: "transfer-2", status: "APPROVED" })] });

    expect(screen.getByText("Pendente")).toBeInTheDocument();
    expect(screen.getByText("Aprovado")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Aprovar" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Processar" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Concluir sem saldo" })).not.toBeInTheDocument();
  });

  it("não permite baixa nem edição de um espelho legado sem vínculo", () => {
    renderSection({ transactions: [legacyTransfer] });

    expect(screen.getByText("Legado · A apurar")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /dar baixa/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /editar/i })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Detalhes" })).toBeInTheDocument();
  });
});
