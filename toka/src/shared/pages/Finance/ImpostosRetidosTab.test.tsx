import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";

import { proofService } from "../../services/proof.service";
import {
  taxObligationService,
  type TaxObligation,
  type TaxObligationTotals,
} from "../../services/taxObligation.service";
import { ImpostosRetidosTab } from "./ImpostosRetidosTab";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn(), warning: vi.fn() } }));

vi.mock("../../services/taxObligation.service", () => ({
  taxObligationService: {
    list: vi.fn(),
    getById: vi.fn(),
    settle: vi.fn(),
    recompute: vi.fn(),
  },
}));

const list = vi.mocked(taxObligationService.list);
const settle = vi.mocked(taxObligationService.settle);
const recompute = vi.mocked(taxObligationService.recompute);

beforeAll(() => {
  // jsdom não implementa as APIs de ponteiro/scroll que o Radix Dialog usa.
  const proto = Element.prototype as unknown as Record<string, unknown>;
  proto.scrollIntoView ??= vi.fn();
  proto.hasPointerCapture ??= vi.fn(() => false);
  proto.setPointerCapture ??= vi.fn();
  proto.releasePointerCapture ??= vi.fn();
  (globalThis as Record<string, unknown>).ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});

const DAY_MS = 24 * 60 * 60 * 1000;
const isoIn = (days: number) => new Date(Date.now() + days * DAY_MS).toISOString();

// Vencimentos relativos a "agora": a marcação de vencida é comparada com o
// relógio, e datas fixas deixariam o teste verde hoje e vermelho no mês que vem.
const overdueObligation: TaxObligation = {
  id: "tax-1",
  tenant_id: "tenant-1",
  kind: "IRRF",
  competence: "2026-09",
  amount_cents: 108000,
  status: "OPEN",
  due_date: isoIn(-5),
  created_at: isoIn(-30),
  updated_at: isoIn(-5),
};

const openObligation: TaxObligation = {
  id: "tax-2",
  tenant_id: "tenant-1",
  kind: "ISS",
  competence: "2026-10",
  amount_cents: 45000,
  status: "OPEN",
  due_date: isoIn(10),
  created_at: isoIn(-3),
  updated_at: isoIn(-3),
};

const paidObligation: TaxObligation = {
  id: "tax-3",
  tenant_id: "tenant-1",
  kind: "INSS",
  competence: "2026-08",
  amount_cents: 25000,
  status: "PAID",
  due_date: isoIn(-40),
  paid_at: isoIn(-35),
  proof_url: "https://files.example/guia.pdf",
  created_at: isoIn(-60),
  updated_at: isoIn(-35),
};

const totals: TaxObligationTotals = {
  open_count: 2,
  open_amount_cents: 153000,
  overdue_count: 1,
  overdue_amount_cents: 108000,
  paid_count: 1,
  paid_amount_cents: 25000,
  by_kind: {
    IRRF: { open_count: 1, open_amount_cents: 108000, paid_amount_cents: 0 },
    ISS: { open_count: 1, open_amount_cents: 45000, paid_amount_cents: 0 },
    INSS: { open_count: 0, open_amount_cents: 0, paid_amount_cents: 25000 },
  },
};

const emptyTotals: TaxObligationTotals = {
  open_count: 0,
  open_amount_cents: 0,
  overdue_count: 0,
  overdue_amount_cents: 0,
  paid_count: 0,
  paid_amount_cents: 0,
  by_kind: {},
};

beforeEach(() => {
  vi.restoreAllMocks();
  list.mockReset();
  settle.mockReset();
  recompute.mockReset();
  list.mockResolvedValue({
    items: [overdueObligation, openObligation, paidObligation],
    totals,
    overdue_in_page: 1,
  });
});

const renderTab = () =>
  render(
    <MemoryRouter>
      <ImpostosRetidosTab />
    </MemoryRouter>,
  );

describe("ImpostosRetidosTab · lista e totais", () => {
  it("lista as obrigações com tipo, competência (MM/AAAA), valor, vencimento e status", async () => {
    renderTab();

    // A tela abre em "A recolher" — o passivo do dia é o que ainda não foi pago.
    expect(await screen.findByText("09/2026")).toBeInTheDocument();
    expect(screen.getByText("10/2026")).toBeInTheDocument();
    expect(screen.getByText("08/2026")).toBeInTheDocument();
    const table = screen.getByRole("table");
    const irrf = within(table).getByText("IRRF").closest("tr");
    expect(irrf).not.toBeNull();
    expect(within(irrf as HTMLElement).getByText("R$ 1.080,00")).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: /Dar baixa no recolhimento de IRRF/i })).toBeInTheDocument();
    // A recolhida não oferece baixa.
    expect(screen.queryByRole("button", { name: /Dar baixa no recolhimento de INSS/i })).not.toBeInTheDocument();
  });

  it("busca com os filtros padrão e envia status/tipo/competência/atrasadas escolhidos", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    renderTab();
    await screen.findByText("09/2026");

    expect(list).toHaveBeenCalledWith(
      expect.objectContaining({ status: "OPEN", kind: undefined, competence: undefined, overdue: undefined }),
    );

    await user.selectOptions(screen.getByLabelText("Tipo"), "ISS");
    await waitFor(() => expect(list).toHaveBeenLastCalledWith(expect.objectContaining({ kind: "ISS" })));

    await user.selectOptions(screen.getByLabelText("Status"), "ALL");
    await waitFor(() => expect(list).toHaveBeenLastCalledWith(expect.objectContaining({ status: undefined })));

    await user.type(screen.getByLabelText("Competência"), "2026-09");
    await waitFor(() => expect(list).toHaveBeenLastCalledWith(expect.objectContaining({ competence: "2026-09" })));

    await user.click(screen.getByRole("button", { name: /Atrasadas/i }));
    await waitFor(() => expect(list).toHaveBeenLastCalledWith(expect.objectContaining({ overdue: true })));
  });

  it("mostra os totais do servidor, o vencido e o retrato por tipo", async () => {
    renderTab();

    const openCard = (await screen.findByText("2 obrigações abertas")).closest("div.rounded-xl") as HTMLElement;
    expect(within(openCard).getByText("R$ 1.530,00")).toBeInTheDocument();

    const overdueCard = screen.getByText("1 vencidas").closest("div.rounded-xl") as HTMLElement;
    expect(within(overdueCard).getByText("R$ 1.080,00")).toBeInTheDocument();

    const paidCard = screen.getByText("1 obrigações recolhidas").closest("div.rounded-xl") as HTMLElement;
    expect(within(paidCard).getByText("R$ 250,00")).toBeInTheDocument();

    // Retrato por tipo: o IRRF aberto do filtro e o INSS já recolhido.
    const byKind = screen.getByRole("heading", { name: "Retrato por tipo" }).closest("div.rounded-xl") as HTMLElement;
    expect(within(byKind).getByText("IRRF")).toBeInTheDocument();
    expect(within(byKind).getByText("R$ 1.080,00")).toBeInTheDocument();
    expect(within(byKind).getByText("R$ 250,00 recolhido")).toBeInTheDocument();
  });

  it("marca visualmente o vencimento vencido", async () => {
    renderTab();

    const table = await screen.findByRole("table");
    const row = within(table).getByText("IRRF").closest("tr") as HTMLElement;
    expect(within(row).getByText("Vencida")).toBeInTheDocument();
    const futureRow = within(table).getByText("ISS").closest("tr") as HTMLElement;
    expect(within(futureRow).queryByText("Vencida")).not.toBeInTheDocument();
  });
});

describe("ImpostosRetidosTab · baixa com comprovante", () => {
  const uploadedProof = {
    url: "https://files.example/tax-proof.pdf",
    filename: "tax-proof.pdf",
    size_bytes: 2048,
  };

  it("dá baixa enviando o comprovante anexado para o serviço", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const upload = vi.spyOn(proofService, "upload").mockResolvedValue(uploadedProof);
    settle.mockResolvedValue({ ...overdueObligation, status: "PAID" });
    renderTab();

    await user.click(await screen.findByRole("button", { name: /Dar baixa no recolhimento de IRRF/i }));

    const panel = await screen.findByRole("dialog", { name: "Baixar recolhimento" });
    expect(within(panel).getByText("IRRF — Imposto de Renda Retido na Fonte")).toBeInTheDocument();
    expect(within(panel).getByText("R$ 1.080,00")).toBeInTheDocument();

    await user.upload(
      within(panel).getByLabelText(/selecionar PDF/i),
      new File(["guia"], "guia-darf.pdf", { type: "application/pdf" }),
    );
    await user.type(within(panel).getByLabelText(/observação/i), "DARF pago no dia 20");
    await user.click(within(panel).getByRole("button", { name: /confirmar baixa/i }));

    await waitFor(() => expect(settle).toHaveBeenCalledTimes(1));
    // O comprovante é o mesmo upload dos outros fluxos, no purpose de comissão.
    expect(upload).toHaveBeenCalledWith(expect.any(File), "commission");
    expect(settle).toHaveBeenCalledWith("tax-1", {
      proof_url: uploadedProof.url,
      proof_filename: "guia-darf.pdf",
      note: "DARF pago no dia 20",
    });
    // A lista recarrega depois da baixa para os totais acompanharem.
    await waitFor(() => expect(list.mock.calls.length).toBeGreaterThan(1));
  });

  it("não envia a baixa sem comprovante e explica a exigência", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    renderTab();

    await user.click(await screen.findByRole("button", { name: /Dar baixa no recolhimento de IRRF/i }));
    const panel = await screen.findByRole("dialog", { name: "Baixar recolhimento" });
    await user.click(within(panel).getByRole("button", { name: /confirmar baixa/i }));

    expect(settle).not.toHaveBeenCalled();
    expect(
      await within(panel).findByText(/Anexe o comprovante do recolhimento \(guia\/DARF\) antes de confirmar/i),
    ).toBeInTheDocument();
  });
});

describe("ImpostosRetidosTab · estado vazio", () => {
  it("explica que a lista nasce do desconto lançado na comissão", async () => {
    list.mockResolvedValue({ items: [], totals: emptyTotals, overdue_in_page: 0 });
    renderTab();

    expect(await screen.findByText(/Nenhum tributo retido a recolher/i)).toBeInTheDocument();
    expect(screen.getByText(/desconto de IRRF, INSS, ISS ou Outros/i)).toBeInTheDocument();
    const link = screen.getByRole("link", { name: "Abrir comissões" });
    expect(link).toHaveAttribute("href", "/financeiro/comissoes");
  });

  it("reconcilia as obrigações com as comissões pela ferramenta de reparo", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    list.mockResolvedValue({ items: [], totals: emptyTotals, overdue_in_page: 0 });
    recompute.mockResolvedValue({ commissions_processed: 3 });
    renderTab();

    await user.click(await screen.findByRole("button", { name: /Reconciliar com as comissões/i }));

    await waitFor(() => expect(recompute).toHaveBeenCalledWith());
    await waitFor(() => expect(list.mock.calls.length).toBeGreaterThan(1));
  });
});
