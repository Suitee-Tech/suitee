import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("./api", () => ({
  default: { post: vi.fn() },
}));

import api from "./api";
import { transactionService, type CreateTransactionBatchItem } from "./transaction.service";

const apiPost = api.post as unknown as ReturnType<typeof vi.fn>;

const items: CreateTransactionBatchItem[] = [
  {
    contract_id: "contract-1",
    type: "receita",
    description: "Recebimento manual",
    value: 1_000,
    date: "2026-08-13",
  },
  {
    contract_id: "contract-1",
    type: "despesa",
    description: "Despesa manual",
    value: 100,
    date: "2026-08-13",
  },
];

describe("transactionService.createBatch", () => {
  beforeEach(() => {
    apiPost.mockReset();
  });

  it("envia todas as linhas em uma chamada com uma operação comum", async () => {
    apiPost.mockResolvedValueOnce({ data: { data: [] } });

    await transactionService.createBatch(items, "manual-posting:operation-1");

    expect(apiPost).toHaveBeenCalledTimes(1);
    expect(apiPost).toHaveBeenCalledWith(
      "/transactions/batch",
      { items },
      { headers: { "Idempotency-Key": "manual-posting:operation-1" } },
    );
  });

  it("inclui commissions relacionadas no mesmo comando atômico", async () => {
    apiPost.mockResolvedValueOnce({ data: { data: [] } });
    const commissions = [{
      deal_id: "deal-1",
      contract_id: "contract-1",
      broker_id: "broker-1",
      role: "fechamento",
      percentage: 100,
      amount: 100,
    }];

    const commissionItems: CreateTransactionBatchItem[] = [items[0], {
      contract_id: "contract-1",
      type: "comissao",
      description: "Comissão manual",
      value: 100,
      date: "2026-08-13",
      broker_id: "broker-1",
      role: "fechamento",
    }];

    await transactionService.createBatch(commissionItems, "manual-posting:operation-2", commissions);

    expect(apiPost).toHaveBeenCalledTimes(1);
    expect(apiPost).toHaveBeenCalledWith(
      "/transactions/batch",
      { items: commissionItems, commissions },
      { headers: { "Idempotency-Key": "manual-posting:operation-2" } },
    );
  });

  it("envia o restante da imobiliária separado das comissões de corretores", async () => {
    apiPost.mockResolvedValueOnce({ data: { data: [] } });
    const commissions = [{
      deal_id: "deal-1",
      contract_id: "contract-1",
      broker_id: "broker-1",
      role: "captador",
      percentage: 80,
      amount: 80,
    }];
    const retention = { percentage: 20, amount: 20 };

    await transactionService.createBatch(
      items,
      "manual-posting:operation-retained",
      commissions,
      retention,
    );

    expect(apiPost).toHaveBeenCalledWith(
      "/transactions/batch",
      {
        items,
        commissions,
        commission_retention: retention,
      },
      { headers: { "Idempotency-Key": "manual-posting:operation-retained" } },
    );
  });

  it("preserva retenção percentual mesmo quando o arredondamento resulta em zero centavo", async () => {
    apiPost.mockResolvedValueOnce({ data: { data: [] } });
    const retention = { percentage: 20, amount: 0 };

    await transactionService.createBatch(
      [items[0]],
      "manual-posting:operation-minimum-retained",
      [{
        deal_id: "deal-1",
        contract_id: "contract-1",
        broker_id: "broker-1",
        role: "captador",
        percentage: 80,
        amount: 0.01,
      }],
      retention,
    );

    expect(apiPost).toHaveBeenCalledWith(
      "/transactions/batch",
      expect.objectContaining({ commission_retention: retention }),
      { headers: { "Idempotency-Key": "manual-posting:operation-minimum-retained" } },
    );
  });
});
