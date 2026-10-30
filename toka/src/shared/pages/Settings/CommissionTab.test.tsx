import { act, createRef } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { CommissionTab } from "./CommissionTab";
import type { TabRef } from "./_shared";

const api = vi.hoisted(() => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  getRules: vi.fn(),
  updateRules: vi.fn(),
}));

vi.mock("../../services/tenantFinancialSettings.service", () => ({
  tenantFinancialSettingsService: { get: api.getSettings, update: api.updateSettings },
}));
vi.mock("../../services/commissionRules.service", () => ({
  commissionRulesService: { get: api.getRules, update: api.updateRules },
}));
vi.mock("../../context/AuthContext", () => ({
  useAuth: () => ({ user: { permission: "owner" } }),
}));
vi.mock("../../permissions/Can", () => ({
  Can: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const settings = {
  tenant_id: "c1",
  default_late_fee_pct: 2,
  default_monthly_interest_pct: 1,
  default_due_day: 10,
  default_commission_pct_rent: 100,
  default_commission_pct_sale: 6,
  default_currency: "BRL",
  payout_proof_required: true,
  admin_fee_pct: 10,
  default_lease_term_months: 30,
  default_inss_pct: 0,
  default_iss_pct: 0,
  updated_at: "",
};

beforeEach(() => {
  vi.clearAllMocks();
  api.getSettings.mockResolvedValue(settings);
  api.getRules.mockResolvedValue([]);
  api.updateSettings.mockResolvedValue(settings);
  api.updateRules.mockResolvedValue([]);
});

describe("CommissionTab — configuração de comissão fail-closed", () => {
  it("não mostra defaults editáveis nem grava quando uma leitura falha", async () => {
    api.getSettings.mockRejectedValueOnce(new Error("offline"));
    const ref = createRef<TabRef>();
    render(
      <MemoryRouter>
        <CommissionTab ref={ref} onChange={vi.fn()} />
      </MemoryRouter>,
    );

    expect(await screen.findByRole("alert")).toHaveTextContent("Não foi possível carregar as configurações");
    expect(screen.queryByLabelText("Comissão padrão VENDA (%)")).not.toBeInTheDocument();
    await expect(ref.current!.save()).rejects.toThrow("Carregue");
    expect(api.updateSettings).not.toHaveBeenCalled();
    expect(api.updateRules).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Tentar novamente" }));
    expect(await screen.findByText("Percentual de comissão padrão")).toBeInTheDocument();
    await act(async () => {
      await ref.current!.save();
    });
    expect(api.updateSettings).toHaveBeenCalledWith(expect.objectContaining({ default_commission_pct_sale: 6 }));
    expect(api.updateRules).toHaveBeenCalled();
  });
});
