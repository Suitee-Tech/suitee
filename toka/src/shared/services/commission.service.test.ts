import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("./api", () => ({
  default: { post: vi.fn() },
}));

import api from "./api";
import { commissionService, type CreateCommissionBatchInput } from "./commission.service";

const apiPost = api.post as unknown as ReturnType<typeof vi.fn>;

const input: CreateCommissionBatchInput = {
  deal_id: "deal-1",
  contract_id: "contract-1",
  splits: [
    {
      broker_id: "broker-1",
      role: "fechamento",
      percentage: 50,
      amount: 1_000,
    },
  ],
};

describe("commissionService.createBatch", () => {
  beforeEach(() => {
    apiPost.mockReset();
  });

  it("envia a chave de idempotência no header esperado pelo backend", async () => {
    apiPost.mockResolvedValueOnce({ data: { data: [] } });

    await commissionService.createBatch(input, "manual-commission-batch:attempt-1");

    expect(apiPost).toHaveBeenCalledWith("/commissions/batch", input, {
      headers: { "X-Idempotency-Key": "manual-commission-batch:attempt-1" },
    });
  });

  it("continua compatível com callers que não informam chave", async () => {
    apiPost.mockResolvedValueOnce({ data: { data: [] } });

    await commissionService.createBatch(input);

    expect(apiPost).toHaveBeenCalledWith("/commissions/batch", input, { headers: {} });
  });
});
