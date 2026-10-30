import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { DealDTO } from "../../../../shared/services/deal.service";
import { financialPreviewService } from "../../../../shared/services/financialPreview.service";
import { FinanceiroTab } from "./FinanceiroTab";

const permissionMocks = vi.hoisted(() => ({
  can: vi.fn(),
  isLoading: false,
}));

vi.mock("../../../../shared/context/PermissionsContext", () => ({
  usePermissionsContext: () => permissionMocks,
}));

vi.mock("../../../../shared/context/ContractsContext", () => ({
  useContractsContext: () => ({ contracts: [] }),
}));

afterEach(() => {
  vi.restoreAllMocks();
  permissionMocks.can.mockReset();
  permissionMocks.isLoading = false;
});

describe("FinanceiroTab", () => {
  it("explica a restrição e não consulta a projeção sem finance:read", () => {
    permissionMocks.can.mockReturnValue(false);
    const request = vi.spyOn(financialPreviewService, "getByDeal");

    render(
      <FinanceiroTab
        dealDetails={{ id: "deal-broker", status: "CONVERTED" } as DealDTO}
        onRefresh={vi.fn()}
      />,
    );

    expect(permissionMocks.can).toHaveBeenCalledWith("finance:read");
    expect(screen.getByText("Área financeira restrita")).toBeInTheDocument();
    expect(screen.getByText(/solicite acesso a um gestor/)).toBeInTheDocument();
    expect(screen.queryByText("Provisionamentos criados na assinatura")).not.toBeInTheDocument();
    expect(request).not.toHaveBeenCalled();
  });
});
