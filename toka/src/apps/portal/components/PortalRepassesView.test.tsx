import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { PortalRepassesView } from "./PortalRepassesView";
import { portalService, type PortalTransfer } from "../services/portal.service";

vi.mock("../services/portal.service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../services/portal.service")>();
  return {
    ...actual,
    portalService: {
      ...actual.portalService,
      getMyTransfers: vi.fn(),
      getMyTransferReceiptUrl: vi.fn(() => "http://api/receipt.pdf"),
    },
  };
});

const transfer: PortalTransfer = {
  id: "transfer-1",
  tenant_id: "tenant-1",
  contract_id: "contract-1",
  owner_id: "owner-1",
  property_id: "property-1",
  reference_month: "2026-10",
  gross_rent: 2800,
  admin_fee_percent: 10,
  admin_fee_amount: 280,
  maintenance_deductions: 0,
  other_deductions: 0,
  net_amount: 2061.28,
  status: "PENDING",
  created_at: "2026-10-05T12:00:00Z",
  updated_at: "2026-10-05T12:00:00Z",
  economic_breakdown: {
    flow: "rent",
    gross_cents: 280000,
    owner_payable_cents: 206128,
    admin_fee_cents: 28000,
    intermediation_revenue_cents: 45744,
    commission_funded_cents: 36128,
  },
};

function renderView() {
  return render(
    <MemoryRouter>
      <PortalRepassesView />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.mocked(portalService.getMyTransfers).mockResolvedValue([transfer]);
});

/**
 * O detalhe do repasse no portal mostrava só "aluguel bruto − taxa adm": o
 * proprietário não fechava a conta porque a comissão do primeiro aluguel
 * (intermediação) também sai do aluguel nos primeiros meses.
 */
describe("PortalRepassesView — intermediação", () => {
  it("detalha o desconto da intermediação e o destino das comissões", async () => {
    renderView();

    await waitFor(() => expect(screen.getByText("Intermediação (comissão do 1º aluguel)")).toBeInTheDocument());
    // O valor vem com espaço não separável depois de "R$".
    expect(screen.getByText(/457,44/)).toBeInTheDocument();
    expect(screen.getByText(/R\$ 361,28 foram para comissões de corretores/)).toBeInTheDocument();
    expect(screen.getByText(/descontada do próprio aluguel nos primeiros meses/)).toBeInTheDocument();
  });

  it("sem intermediação no repasse, não mostra a linha", async () => {
    vi.mocked(portalService.getMyTransfers).mockResolvedValue([
      { ...transfer, economic_breakdown: { ...transfer.economic_breakdown!, intermediation_revenue_cents: 0, commission_funded_cents: 0 } },
    ]);
    renderView();

    await waitFor(() => expect(screen.getByText("Aluguel bruto")).toBeInTheDocument());
    expect(screen.queryByText("Intermediação (comissão do 1º aluguel)")).not.toBeInTheDocument();
  });
});
