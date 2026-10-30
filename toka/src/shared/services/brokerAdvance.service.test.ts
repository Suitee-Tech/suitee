import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("./api", async () => {
  // O unwrap real é usado pelo serviço; só o cliente HTTP é dublado, para o
  // teste travar o parsing de `{ data, meta }` que o backend devolve.
  const { unwrapData } = await vi.importActual<typeof import("./httpClient")>("./httpClient");
  return {
    default: { get: vi.fn(), post: vi.fn() },
    unwrapApiData: unwrapData,
  };
});

import api from "./api";
import {
  advanceOutstandingCents,
  brokerAdvanceService,
  totalOutstandingCents,
  totalSettledCents,
  type BrokerAdvance,
  type BrokerBalance,
} from "./brokerAdvance.service";

const apiGet = api.get as unknown as ReturnType<typeof vi.fn>;
const apiPost = api.post as unknown as ReturnType<typeof vi.fn>;

const advance: BrokerAdvance = {
  id: "adv-1",
  tenant_id: "tenant-1",
  broker_id: "broker-1",
  amount_cents: 200000,
  settled_cents: 50000,
  status: "OPEN",
  description: "Adiantamento de comissão",
  granted_at: "2026-09-10T00:00:00Z",
  created_at: "2026-09-10T10:00:00Z",
  updated_at: "2026-09-10T10:00:00Z",
};

const balances: BrokerBalance[] = [
  { broker_id: "broker-1", open_count: 1, outstanding_cents: 150000, settled_cents: 50000 },
  { broker_id: "broker-2", open_count: 0, outstanding_cents: 0, settled_cents: 30000 },
];

beforeEach(() => {
  apiGet.mockReset();
  apiPost.mockReset();
});

describe("brokerAdvanceService.list", () => {
  it("sem filtro, chama a rota pública limpa (o prefixo /api/v1 é do cliente)", async () => {
    apiGet.mockResolvedValueOnce({ data: { data: [], meta: { balances: [] } } });

    const result = await brokerAdvanceService.list();

    expect(apiGet).toHaveBeenCalledWith("/broker-advances");
    expect(result).toEqual({ items: [], balances: [] });
  });

  it("serializa corretor, status em lista, somente abertos e limite", async () => {
    apiGet.mockResolvedValueOnce({ data: { data: [advance], meta: { balances } } });

    const result = await brokerAdvanceService.list({
      broker_id: "broker-1",
      status: ["OPEN", "SETTLED"],
      open: true,
      limit: 100,
    });

    expect(apiGet).toHaveBeenCalledWith(
      "/broker-advances?broker_id=broker-1&status=OPEN%2CSETTLED&open=true&limit=100",
    );
    expect(result.items).toEqual([advance]);
    expect(result.balances).toEqual(balances);
  });

  it("omite o status quando o filtro está vazio", async () => {
    apiGet.mockResolvedValueOnce({ data: { data: [], meta: {} } });

    await brokerAdvanceService.list({ status: '' });

    expect(apiGet).toHaveBeenCalledWith("/broker-advances");
  });

  it("tolera resposta sem meta.balances sem quebrar a lista", async () => {
    apiGet.mockResolvedValueOnce({ data: { data: [advance] } });

    const result = await brokerAdvanceService.list();

    expect(result.items).toEqual([advance]);
    expect(result.balances).toEqual([]);
  });
});

describe("brokerAdvanceService.create", () => {
  it("envia corretor, valor em reais, data e descrição e desembrulha { data }", async () => {
    apiPost.mockResolvedValueOnce({ data: { data: advance } });

    const created = await brokerAdvanceService.create({
      broker_id: "broker-1",
      amount: 2000,
      description: "Adiantamento de comissão",
      granted_at: "2026-09-10",
    });

    expect(apiPost).toHaveBeenCalledWith("/broker-advances", {
      broker_id: "broker-1",
      amount: 2000,
      description: "Adiantamento de comissão",
      granted_at: "2026-09-10",
    });
    expect(created).toEqual(advance);
  });
});

describe("brokerAdvanceService.cancel", () => {
  it("envia o motivo do cancelamento pelo caminho do recurso", async () => {
    const cancelled = { ...advance, status: "CANCELLED" as const };
    apiPost.mockResolvedValueOnce({ data: { data: cancelled } });

    const result = await brokerAdvanceService.cancel("adv-1", "lançado em duplicidade");

    expect(apiPost).toHaveBeenCalledWith("/broker-advances/adv-1/cancel", { reason: "lançado em duplicidade" });
    expect(result.status).toBe("CANCELLED");
  });

  it("aceita cancelamento sem motivo", async () => {
    apiPost.mockResolvedValueOnce({ data: { data: { ...advance, status: "CANCELLED" } } });

    await brokerAdvanceService.cancel("adv-1");

    expect(apiPost).toHaveBeenCalledWith("/broker-advances/adv-1/cancel", { reason: undefined });
  });
});

describe("brokerAdvanceService.outstandingForBroker", () => {
  it("soma o saldo dos adiantamentos abertos do corretor", async () => {
    apiGet.mockResolvedValueOnce({
      data: {
        data: [
          advance,
          { ...advance, id: "adv-2", amount_cents: 80000, settled_cents: 30000 },
        ],
        meta: { balances },
      },
    });

    const result = await brokerAdvanceService.outstandingForBroker("broker-1");

    expect(apiGet).toHaveBeenLastCalledWith("/broker-advances?broker_id=broker-1&open=true&limit=100");
    // (200000 - 50000) + (80000 - 30000) = 200000
    expect(result).toEqual({ outstanding_cents: 200000, open_count: 2 });
  });

  it("devolve zero quando o corretor não tem adiantamento aberto", async () => {
    apiGet.mockResolvedValueOnce({ data: { data: [], meta: { balances: [] } } });

    const result = await brokerAdvanceService.outstandingForBroker("broker-9");

    expect(result).toEqual({ outstanding_cents: 0, open_count: 0 });
  });
});

describe("helpers de saldo", () => {
  it("saldo em aberto = valor - abatido, sem negativo", () => {
    expect(advanceOutstandingCents({ amount_cents: 200000, settled_cents: 50000 })).toBe(150000);
    expect(advanceOutstandingCents({ amount_cents: 100, settled_cents: 100 })).toBe(0);
    expect(advanceOutstandingCents({ amount_cents: 100, settled_cents: 300 })).toBe(0);
  });

  it("soma o retrato por corretor dos dois lados", () => {
    expect(totalOutstandingCents(balances)).toBe(150000);
    expect(totalSettledCents(balances)).toBe(80000);
    expect(totalOutstandingCents(undefined)).toBe(0);
    expect(totalSettledCents(undefined)).toBe(0);
  });
});
