import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("./api", () => ({
  default: { get: vi.fn() },
}));

import api from "./api";
import { leadService } from "./lead.service";

const apiGet = api.get as unknown as ReturnType<typeof vi.fn>;

describe("leadService.getStats", () => {
  beforeEach(() => {
    apiGet.mockReset();
  });

  it("desembrulha o envelope e devolve os stats reais (não 0%)", async () => {
    // O backend responde com `{ success, message, statusCode, data: {...} }`.
    // A regressão era devolver o envelope inteiro: o Dashboard lia
    // `stats.conversion_rate` (undefined) e exibia "0%".
    apiGet.mockResolvedValueOnce({
      data: {
        success: true,
        message: "ok",
        statusCode: 200,
        data: { total: 42, by_status: { Novo: 30, Convertido: 12 }, conversion_rate: 0.2857 },
      },
    });

    const stats = await leadService.getStats();

    expect(apiGet).toHaveBeenCalledWith("/leads/stats");
    expect(stats.total).toBe(42);
    expect(stats.conversion_rate).toBe(0.2857);
  });
});
