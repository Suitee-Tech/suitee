import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  auth: { isAuthenticated: true, user: { current_company_id: "tenant-1" } },
  can: vi.fn(() => true),
  create: vi.fn(),
  getAll: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("./AuthContext", () => ({ useAuth: () => mocks.auth }));
vi.mock("./PermissionsContext", () => ({
  usePermissionsContext: () => ({ can: mocks.can, isLoading: false }),
}));
vi.mock("../hooks/useDataChangeRefresh", () => ({ useDataChangeRefresh: () => undefined }));
// O hook devolve `beginScopedRequest`, que por sua vez devolve `isCurrent`.
vi.mock("../hooks/useRequestScopeGuard", () => ({
  useRequestScopeGuard: () => () => () => true,
}));
vi.mock("sonner", () => ({ toast: { success: mocks.toastSuccess, error: mocks.toastError } }));
vi.mock("../services/maintenance.service", () => ({
  maintenanceService: {
    create: mocks.create,
    getAll: mocks.getAll,
    update: vi.fn(),
    changeStatus: vi.fn(),
    delete: vi.fn(),
  },
}));

import { MaintenanceProvider, useMaintenanceContext } from "./MaintenanceContext";

const wrapper = ({ children }: { children: ReactNode }) => <MaintenanceProvider>{children}</MaintenanceProvider>;

beforeEach(() => {
  vi.clearAllMocks();
  mocks.create.mockResolvedValue({ data: { id: "ticket-1" } });
});

describe("MaintenanceContext.addTicket", () => {
  // A montagem campo a campo do DTO já descartou `requester_lead_id` e
  // `rental_id` — sem eles o toca-financial não sabe quem cobrar.
  it("repassa contrato, parte responsável e custo para a API", async () => {
    const { result } = renderHook(() => useMaintenanceContext(), { wrapper });
    await waitFor(() => expect(result.current).toBeTruthy());

    act(() => {
      void result.current.addTicket({
        type: "Manutenção",
        category: "Elétrica",
        property_id: "property-1",
        contract_id: "contract-1",
        rental_id: "deal-1",
        requester_lead_id: "lead-tenant",
        property_title: "IM-101 - Apartamento dos Ipês",
        requester: "Vanessa Rodrigues Melo",
        profile: "Inquilino",
        priority: "Média",
        description: "Tomada queimada",
        estimated_cost: 130,
        payer_role: "tenant",
      } as never);
    });

    await waitFor(() => expect(mocks.create).toHaveBeenCalledTimes(1));
    expect(mocks.create).toHaveBeenCalledWith(
      expect.objectContaining({
        contract_id: "contract-1",
        rental_id: "deal-1",
        requester_lead_id: "lead-tenant",
        property_id: "property-1",
        estimated_cost: 130,
        payer_role: "tenant",
      }),
    );
  });
});
