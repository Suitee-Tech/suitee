import { act, render, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { CreateTransactionBatchItem, TransactionDTO } from "../services/transaction.service";
import type { OwnerTransfer } from "../services/transfer.service";
import { FinanceProvider, useFinanceContext } from "./FinanceContext";

const mocks = vi.hoisted(() => ({
  transactionGetAll: vi.fn(),
  transactionCreateBatch: vi.fn(),
  transactionUpdate: vi.fn(),
  transactionDelete: vi.fn(),
  transferGetAll: vi.fn(),
  transferGetById: vi.fn(),
  transferApprove: vi.fn(),
  appendAuditLogEntry: vi.fn(),
  toastError: vi.fn(),
  can: vi.fn((permission: string) => permission === "finance:read"),
}));

vi.mock("./AuthContext", () => ({
  useAuth: () => ({
    isAuthenticated: true,
    user: { current_company_id: "company-1" },
  }),
}));

vi.mock("./PermissionsContext", () => ({
  usePermissionsContext: () => ({
    can: mocks.can,
    isLoading: false,
  }),
}));

vi.mock("./AuditLogContext", () => ({
  useAuditLogContext: () => ({
    appendAuditLogEntry: mocks.appendAuditLogEntry,
  }),
}));

vi.mock("../hooks/useDataChangeRefresh", () => ({
  useDataChangeRefresh: vi.fn(),
}));

vi.mock("../services/transaction.service", () => ({
  transactionService: {
    getAll: mocks.transactionGetAll,
    createBatch: mocks.transactionCreateBatch,
    update: mocks.transactionUpdate,
    delete: mocks.transactionDelete,
  },
}));

vi.mock("../services/transfer.service", () => ({
  transferService: {
    getAll: mocks.transferGetAll,
    getById: mocks.transferGetById,
    approve: mocks.transferApprove,
  },
}));

vi.mock("sonner", () => ({
  toast: {
    error: mocks.toastError,
    success: vi.fn(),
  },
}));

const transactionDto = (
  overrides: Partial<TransactionDTO> = {},
): TransactionDTO => ({
  id: "tx-1",
  contract_id: "contract-1",
  type: "receita",
  description: "Aluguel",
  value: 1200,
  date: "2026-07-01",
  status: "Pendente",
  tenant_id: "company-1",
  created_at: "2026-07-01T00:00:00Z",
  updated_at: "2026-07-01T00:00:00Z",
  ...overrides,
});

const ownerTransfer = (overrides: Partial<OwnerTransfer> = {}): OwnerTransfer => ({
  id: "transfer-1",
  tenant_id: "company-1",
  contract_id: "contract-1",
  owner_id: "owner-1",
  property_id: "property-1",
  reference_month: "2026-07",
  gross_rent: 1200,
  admin_fee_percent: 10,
  admin_fee_amount: 120,
  maintenance_deductions: 0,
  other_deductions: 0,
  net_amount: 1080,
  status: "PENDING",
  created_at: "2026-07-01T00:00:00Z",
  updated_at: "2026-07-01T00:00:00Z",
  ...overrides,
});

let finance: ReturnType<typeof useFinanceContext>;

function CaptureFinanceContext({ children }: { children?: ReactNode }) {
  finance = useFinanceContext();
  return children ?? null;
}

describe("FinanceContext.updateTransaction", () => {
  beforeEach(() => {
    mocks.transactionGetAll.mockReset();
    mocks.transactionCreateBatch.mockReset();
    mocks.transactionUpdate.mockReset();
    mocks.transactionDelete.mockReset();
    mocks.transferGetAll.mockReset();
    mocks.transferGetById.mockReset();
    mocks.transferApprove.mockReset();
    mocks.appendAuditLogEntry.mockReset();
    mocks.toastError.mockReset();
    mocks.can.mockClear();

    // Envelope paginado como o backend devolve (response.Paginated): sem
    // `meta.limit` a listagem não tem como provar que veio completa.
    mocks.transactionGetAll.mockResolvedValue({ data: [transactionDto()], meta: { total: 1, limit: 100, page: 1, total_pages: 1 } });
    mocks.transactionDelete.mockResolvedValue(undefined);
    mocks.transferGetAll.mockResolvedValue({ data: [], meta: { total: 0, limit: 100, page: 1, total_pages: 0 } });
  });

  it("reconcilia o estado otimista com a resposta persistida", async () => {
    mocks.transactionUpdate.mockResolvedValue({
      data: transactionDto({
        description: "Aluguel confirmado pelo servidor",
        value: 1250,
      }),
    });

    render(
      <FinanceProvider>
        <CaptureFinanceContext />
      </FinanceProvider>,
    );

    await waitFor(() => expect(finance.transactions).toHaveLength(1));

    await act(async () => {
      await finance.updateTransaction("tx-1", {
        description: "Alteração local",
        value: 1300,
      });
    });

    expect(finance.transactions[0]).toMatchObject({
      description: "Aluguel confirmado pelo servidor",
      value: 1250,
    });
    expect(mocks.appendAuditLogEntry).toHaveBeenCalledTimes(1);
  });

  it("normaliza status canônicos da API para os labels em português", async () => {
    mocks.transactionGetAll.mockResolvedValue({
      data: [
        transactionDto({ id: "tx-pending", status: "PENDING" }),
        transactionDto({ id: "tx-paid", status: "PAID" }),
        transactionDto({ id: "tx-overdue", status: "OVERDUE" }),
        transactionDto({ id: "tx-reversed", status: "REVERSED" }),
        transactionDto({ id: "tx-cancelled", status: "CANCELLED" }),
      ],
      meta: { total: 5, limit: 100, page: 1, total_pages: 1 },
    });

    render(
      <FinanceProvider>
        <CaptureFinanceContext />
      </FinanceProvider>,
    );

    await waitFor(() => expect(finance.transactions).toHaveLength(5));
    expect(finance.transactions.map((tx) => tx.status)).toEqual([
      "Pendente",
      "Pago",
      "Em Atraso",
      "Estornado",
      "Cancelado",
    ]);
  });

  it("aguarda o lote e publica operation_id/posting_id canônicos juntos", async () => {
    let resolveBatch!: (value: unknown) => void;
    mocks.transactionCreateBatch.mockImplementationOnce(
      () => new Promise((resolve) => { resolveBatch = resolve; }),
    );
    const items: CreateTransactionBatchItem[] = [{
      contract_id: "contract-1",
      type: "receita",
      description: "Receita em lote",
      value: 300,
      date: "2026-08-13",
    }];

    render(
      <FinanceProvider>
        <CaptureFinanceContext />
      </FinanceProvider>,
    );
    await waitFor(() => expect(finance.transactions).toHaveLength(1));

    let batchPromise!: Promise<unknown>;
    act(() => {
      batchPromise = finance.addTransactionBatch(items, "manual-posting:operation-1");
    });
    expect(finance.transactions).toHaveLength(1);

    await act(async () => {
      resolveBatch({
        data: [{
          index: 0,
          transaction: transactionDto({
            id: "posting-2",
            description: "Receita em lote",
            operation_id: "operation-1",
            posting_id: "posting-2",
            source_event_id: "source-line-1",
          }),
        }],
      });
      await batchPromise;
    });

    expect(mocks.transactionCreateBatch).toHaveBeenCalledWith(items, "manual-posting:operation-1", [], undefined);
    expect(finance.transactions).toHaveLength(2);
    expect(finance.transactions[0]).toMatchObject({
      id: "posting-2",
      operationID: "operation-1",
      postingID: "posting-2",
      sourceEventID: "source-line-1",
    });
  });

  it("rejeita resposta parcial sem publicar sucesso local", async () => {
    mocks.transactionCreateBatch.mockResolvedValueOnce({ data: [] });
    const items: CreateTransactionBatchItem[] = [{
      contract_id: "contract-1",
      type: "receita",
      description: "Receita em lote",
      value: 300,
      date: "2026-08-13",
    }];

    render(
      <FinanceProvider>
        <CaptureFinanceContext />
      </FinanceProvider>,
    );
    await waitFor(() => expect(finance.transactions).toHaveLength(1));

    await act(async () => {
      await expect(finance.addTransactionBatch(items, "manual-posting:partial")).rejects.toThrow(
        "incomplete transaction batch response",
      );
    });

    expect(finance.transactions).toHaveLength(1);
    expect(mocks.toastError).toHaveBeenCalledWith("Erro ao criar lançamentos.");
  });

  it("não deixa um refresh iniciado antes do lote apagar postings confirmados", async () => {
    let resolveStaleFetch!: (value: unknown) => void;
    mocks.transactionCreateBatch.mockResolvedValueOnce({
      data: [{
        index: 0,
        transaction: transactionDto({
          id: "posting-new",
          operation_id: "operation-new",
          posting_id: "posting-new",
          source_event_id: "source-new",
        }),
      }],
    });

    render(
      <FinanceProvider>
        <CaptureFinanceContext />
      </FinanceProvider>,
    );
    await waitFor(() => expect(finance.transactions).toHaveLength(1));
    await waitFor(() => expect(finance.loading.transactions).toBe(false));

    mocks.transactionGetAll.mockImplementationOnce(
      () => new Promise((resolve) => { resolveStaleFetch = resolve; }),
    );
    let staleRefresh!: Promise<void>;
    act(() => {
      staleRefresh = finance.refetchTransactions();
    });
    await waitFor(() => expect(mocks.transactionGetAll).toHaveBeenCalledTimes(2));

    await act(async () => {
      await finance.addTransactionBatch([{
        contract_id: "contract-1",
        type: "receita",
        description: "Nova receita",
        value: 300,
        date: "2026-08-13",
      }], "manual-posting:stale-refresh");
    });
    expect(finance.transactions[0]?.id).toBe("posting-new");

    await act(async () => {
      resolveStaleFetch({ data: [transactionDto()] });
      await staleRefresh;
    });
    expect(finance.transactions.map((transaction) => transaction.id)).toContain("posting-new");
  });

  it("recarrega o repasse quando uma API legada responde data nula ao aprovar", async () => {
    mocks.transferGetAll.mockResolvedValue({ data: [ownerTransfer()], meta: { total: 1, limit: 100, page: 1, total_pages: 1 } });
    mocks.transferApprove.mockResolvedValue({ data: null });
    mocks.transferGetById.mockResolvedValue({
      data: ownerTransfer({ status: "APPROVED", approved_by: "operator-1" }),
    });

    render(
      <FinanceProvider>
        <CaptureFinanceContext />
      </FinanceProvider>,
    );

    await waitFor(() => expect(finance.transfers).toHaveLength(1));
    await act(async () => {
      await finance.approveTransfer("transfer-1");
    });

    expect(mocks.transferGetById).toHaveBeenCalledWith("transfer-1");
    expect(finance.transfers[0]).toMatchObject({
      id: "transfer-1",
      status: "APPROVED",
    });
  });

  it("propaga a falha, desfaz o estado otimista e reconcilia com o servidor", async () => {
    let rejectUpdate!: (error: Error) => void;
    mocks.transactionUpdate.mockImplementationOnce(
      () =>
        new Promise((_resolve, reject) => {
          rejectUpdate = reject;
        }),
    );

    render(
      <FinanceProvider>
        <CaptureFinanceContext />
      </FinanceProvider>,
    );

    await waitFor(() => expect(finance.transactions[0]?.description).toBe("Aluguel"));
    await waitFor(() => expect(finance.loading.transactions).toBe(false));

    let updatePromise!: Promise<boolean>;
    act(() => {
      updatePromise = finance.updateTransaction("tx-1", {
        description: "Alteração otimista",
      });
    });

    await waitFor(() =>
      expect(finance.transactions[0]?.description).toBe("Alteração otimista"),
    );

    const failure = new Error("backend indisponível");
    await act(async () => {
      rejectUpdate(failure);
      await expect(updatePromise).rejects.toBe(failure);
    });

    await waitFor(() => expect(finance.transactions[0]?.description).toBe("Aluguel"));
    expect(mocks.transactionGetAll).toHaveBeenCalledTimes(2);
    expect(mocks.toastError).toHaveBeenCalledWith("Erro ao atualizar transação.");
    expect(mocks.appendAuditLogEntry).not.toHaveBeenCalled();
  });

  it("aguarda a exclusão e restaura a transação quando o backend falha", async () => {
    let rejectDelete!: (error: Error) => void;
    mocks.transactionDelete.mockImplementationOnce(
      () => new Promise((_resolve, reject) => { rejectDelete = reject; }),
    );

    render(
      <FinanceProvider>
        <CaptureFinanceContext />
      </FinanceProvider>,
    );
    await waitFor(() => expect(finance.transactions).toHaveLength(1));
    await waitFor(() => expect(finance.loading.transactions).toBe(false));

    let deletePromise!: Promise<void>;
    act(() => {
      deletePromise = finance.deleteTransaction("tx-1");
    });
    await waitFor(() => expect(finance.transactions).toHaveLength(0));

    const failure = new Error("delete indisponível");
    await act(async () => {
      rejectDelete(failure);
      await expect(deletePromise).rejects.toBe(failure);
    });

    await waitFor(() => expect(finance.transactions).toHaveLength(1));
    expect(mocks.transactionGetAll).toHaveBeenCalledTimes(2);
    expect(mocks.toastError).toHaveBeenCalledWith("Erro ao excluir transação.");
    expect(mocks.appendAuditLogEntry).not.toHaveBeenCalled();
  });
});
