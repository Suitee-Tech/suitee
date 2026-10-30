import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("./api", async () => {
  // O unwrap real é usado pelo serviço; só o cliente HTTP é dublado, para o
  // teste travar o parsing de `{ data: ... }` que o backend devolve.
  const { unwrapData } = await vi.importActual<typeof import("./httpClient")>("./httpClient");
  return {
    default: { get: vi.fn(), post: vi.fn() },
    unwrapApiData: unwrapData,
  };
});

import api from "./api";
import { taxObligationService, type TaxObligation } from "./taxObligation.service";

const apiGet = api.get as unknown as ReturnType<typeof vi.fn>;
const apiPost = api.post as unknown as ReturnType<typeof vi.fn>;

const obligation: TaxObligation = {
  id: "tax-1",
  tenant_id: "tenant-1",
  kind: "IRRF",
  competence: "2026-09",
  amount_cents: 108000,
  status: "OPEN",
  due_date: "2026-10-20T00:00:00Z",
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-01T10:00:00Z",
};

const totals = {
  open_count: 1,
  open_amount_cents: 108000,
  overdue_count: 0,
  overdue_amount_cents: 0,
  paid_count: 0,
  paid_amount_cents: 0,
  by_kind: { IRRF: { open_count: 1, open_amount_cents: 108000, paid_amount_cents: 0 } },
};

beforeEach(() => {
  apiGet.mockReset();
  apiPost.mockReset();
});

describe("taxObligationService.list", () => {
  it("sem filtro, chama a rota pública limpa (o prefixo /api/v1 é do cliente)", async () => {
    apiGet.mockResolvedValueOnce({ data: { data: [], meta: { totals, overdue_in_page: 0 } } });

    const result = await taxObligationService.list();

    expect(apiGet).toHaveBeenCalledWith("/tax-obligations");
    expect(result).toEqual({ items: [], totals, overdue_in_page: 0 });
  });

  it("serializa status/tipo em lista, competência, atrasadas e paginação", async () => {
    apiGet.mockResolvedValueOnce({ data: { data: [obligation], meta: { totals, overdue_in_page: 1 } } });

    const result = await taxObligationService.list({
      status: ["OPEN", "PAID"],
      kind: "IRRF",
      competence: "2026-09",
      overdue: true,
      limit: 100,
      offset: 20,
    });

    expect(apiGet).toHaveBeenCalledWith(
      "/tax-obligations?status=OPEN%2CPAID&kind=IRRF&competence=2026-09&overdue=true&limit=100&offset=20",
    );
    expect(result.items).toEqual([obligation]);
    expect(result.totals.by_kind.IRRF?.open_amount_cents).toBe(108000);
    expect(result.overdue_in_page).toBe(1);
  });

  it("omite filtros vazios e devolve totais zerados quando o meta não vem", async () => {
    apiGet.mockResolvedValueOnce({ data: { data: [] } });

    const result = await taxObligationService.list({ status: "", kind: [], overdue: false, competence: "" });

    expect(apiGet).toHaveBeenCalledWith("/tax-obligations");
    expect(result.totals).toMatchObject({
      open_count: 0,
      open_amount_cents: 0,
      overdue_amount_cents: 0,
      paid_amount_cents: 0,
      by_kind: {},
    });
  });
});

describe("taxObligationService.getById", () => {
  it("desembrulha o envelope { data: { data } }", async () => {
    apiGet.mockResolvedValueOnce({ data: { data: obligation } });

    await expect(taxObligationService.getById("tax-1")).resolves.toEqual(obligation);
    expect(apiGet).toHaveBeenCalledWith("/tax-obligations/tax-1");
  });
});

describe("taxObligationService.settle", () => {
  it("envia o comprovante obrigatório e o nome do arquivo", async () => {
    const settled = { ...obligation, status: "PAID" as const, proof_url: "https://files.example/guia.pdf" };
    apiPost.mockResolvedValueOnce({ data: { data: settled } });

    const result = await taxObligationService.settle("tax-1", {
      proof_url: "https://files.example/guia.pdf",
      proof_filename: "guia.pdf",
      note: "DARF pago",
    });

    expect(apiPost).toHaveBeenCalledWith("/tax-obligations/tax-1/settle", {
      proof_url: "https://files.example/guia.pdf",
      proof_filename: "guia.pdf",
      note: "DARF pago",
    });
    expect(result.status).toBe("PAID");
  });
});

describe("taxObligationService.recompute", () => {
  it("reprocessa as comissões recentes quando não recebe comissão", async () => {
    apiPost.mockResolvedValueOnce({ data: { data: { commissions_processed: 4 } } });

    await expect(taxObligationService.recompute()).resolves.toEqual({ commissions_processed: 4 });
    expect(apiPost).toHaveBeenCalledWith("/tax-obligations/recompute", {});
  });

  it("reprocessa uma comissão específica quando o id é informado", async () => {
    apiPost.mockResolvedValueOnce({ data: { data: { commissions_processed: 1 } } });

    await expect(taxObligationService.recompute("commission-1")).resolves.toEqual({ commissions_processed: 1 });
    expect(apiPost).toHaveBeenCalledWith("/tax-obligations/recompute", { commission_id: "commission-1" });
  });

  it("tolera resposta sem o contador", async () => {
    apiPost.mockResolvedValueOnce({ data: { data: {} } });

    await expect(taxObligationService.recompute()).resolves.toEqual({ commissions_processed: 0 });
  });
});
