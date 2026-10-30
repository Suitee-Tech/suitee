import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../lib/runtime-env", () => ({ getEnv: vi.fn(() => "") }));
vi.mock("./api", () => ({
  default: { post: vi.fn() },
}));

import api from "./api";
import { invoiceService } from "./invoice.service";

const apiPost = api.post as unknown as ReturnType<typeof vi.fn>;

describe("invoiceService idempotency headers", () => {
  beforeEach(() => {
    apiPost.mockReset();
    apiPost.mockResolvedValue({ data: { data: { id: "invoice-1" } } });
  });

  it("reutiliza a chave explícita de criação sem incorporá-la ao payload", async () => {
    const command = {
      reference_id: "contract-1",
      charge_id: "charge-1",
      payer_id: "payer-1",
      amount: 40,
      due_date: "2026-08-20T12:00:00Z",
      type: "RENT",
    };

    await invoiceService.create(command, "invoice-create:stable-command");

    expect(apiPost).toHaveBeenCalledWith(
      "/invoices",
      command,
      { headers: { "Idempotency-Key": "invoice-create:stable-command" } },
    );
  });

  it("sempre envia chave na baixa da invoice", async () => {
    await invoiceService.pay("invoice-1");

    expect(apiPost).toHaveBeenCalledTimes(1);
    const [, , config] = apiPost.mock.calls[0];
    expect(config.headers["Idempotency-Key"]).toMatch(/^invoice-payment:/);
  });
});
