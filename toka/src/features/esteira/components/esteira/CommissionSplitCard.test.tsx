import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { DealDTO } from "../../../../shared/services/deal.service";
import { CommissionSplitCard } from "./CommissionSplitCard";

const mocks = vi.hoisted(() => ({
  commissionRulesGet: vi.fn(),
  financialSettingsGet: vi.fn(),
  propertyGetById: vi.fn(),
  leadGetById: vi.fn(),
  brokerGetAll: vi.fn(),
  brokerGetById: vi.fn(),
  updateBrokerParticipations: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
  allowedPermissions: new Set<string>(["deals:write", "commissions:write", "finance:read"]),
}));

vi.mock("../../../../shared/services/commissionRules.service", () => ({
  commissionRulesService: { get: mocks.commissionRulesGet },
}));

vi.mock("../../../../shared/services/tenantFinancialSettings.service", () => ({
  tenantFinancialSettingsService: { get: mocks.financialSettingsGet },
}));

vi.mock("../../../../shared/services/property.service", () => ({
  propertyService: { getById: mocks.propertyGetById },
}));

vi.mock("../../../../shared/services/lead.service", () => ({
  leadService: { getById: mocks.leadGetById },
}));

vi.mock("../../../../shared/services/broker.service", () => ({
  brokerService: {
    getAll: mocks.brokerGetAll,
    getById: mocks.brokerGetById,
  },
}));

vi.mock("../../../../shared/context/DealsContext", () => ({
  useDealsContext: () => ({
    updateBrokerParticipations: mocks.updateBrokerParticipations,
  }),
}));

vi.mock("../../../../shared/permissions/usePermission", () => ({
  useCan: () => (permission: string) => mocks.allowedPermissions.has(permission),
}));

vi.mock("sonner", () => ({
  toast: {
    success: mocks.toastSuccess,
    error: mocks.toastError,
  },
}));

vi.mock("../../../../shared/ui/dialog", () => ({
  Dialog: ({ open, children }: { open: boolean; children: React.ReactNode }) =>
    open ? <div role="dialog">{children}</div> : null,
  DialogContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogHeader: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: React.ReactNode }) => <h2>{children}</h2>,
  DialogDescription: ({ children }: { children: React.ReactNode }) => <p>{children}</p>,
}));

vi.mock("../../../../shared/ui/select", () => ({
  Select: ({
    value,
    onValueChange,
    disabled,
    children,
  }: {
    value: string;
    onValueChange: (value: string) => void;
    disabled?: boolean;
    children: React.ReactNode;
  }) => (
    <select
      value={value}
      onChange={(event) => onValueChange(event.target.value)}
      disabled={disabled}
    >
      {children}
    </select>
  ),
  SelectTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  SelectValue: () => null,
  SelectContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  SelectItem: ({ value, children }: { value: string; children: React.ReactNode }) => (
    <option value={value}>{children}</option>
  ),
}));

vi.mock("../../../../shared/components/ConfirmDialog", () => ({
  ConfirmDialog: ({
    open,
    onConfirm,
    confirmLabel,
  }: {
    open: boolean;
    onConfirm: () => void;
    confirmLabel: string;
  }) => open ? <button onClick={onConfirm}>{confirmLabel}</button> : null,
}));

function dealDto(): DealDTO {
  return {
    id: "deal-1",
    type: "SALE",
    status: "NEGOTIATION",
    property_id: "property-1",
    owner_id: "owner-1",
    customer_id: "client-converted",
    lead_id: "lead-original",
    broker_id: "broker-operator",
    broker_participations: [],
    value: 980_000,
    commission: 0,
    created_at: "2026-08-28T10:00:00Z",
    updated_at: "2026-08-28T10:00:00Z",
    tenant_id: "tenant-a",
  };
}

function roleRow(label: string): HTMLElement {
  const labelNode = screen.getByText(label, { selector: "p" });
  const row = labelNode.closest(".grid");
  if (!row) throw new Error(`row not found for ${label}`);
  return row as HTMLElement;
}

describe("CommissionSplitCard", () => {
  beforeEach(() => {
    mocks.commissionRulesGet.mockReset().mockResolvedValue([
      { role: "captador", default_percentage: 30 },
      { role: "co_captador", default_percentage: 0 },
      { role: "visitador", default_percentage: 0 },
      { role: "atendente", default_percentage: 0 },
      { role: "fechamento", default_percentage: 50 },
      { role: "imobiliaria", default_percentage: 20 },
    ]);
    mocks.financialSettingsGet.mockReset().mockResolvedValue({
      default_commission_pct_sale: 6,
      default_commission_pct_rent: 100,
    });
    mocks.propertyGetById.mockReset().mockResolvedValue({
      data: {
        data: {
          id: "property-1",
          capturing_brokers: [
            {
              id: "broker-property",
              role: "captador",
              commission_pct: 99,
            },
          ],
        },
      },
    });
    mocks.leadGetById.mockReset().mockResolvedValue({
      id: "lead-original",
      assigned_broker_id: "broker-lead",
    });
    mocks.brokerGetAll.mockReset().mockResolvedValue({
      data: {
        data: [
          { id: "broker-property", name: "Captador do imóvel", active: true },
          { id: "broker-lead", name: "Atendente do lead", active: true },
        ],
      },
    });
    mocks.brokerGetById.mockReset();
    mocks.updateBrokerParticipations.mockReset().mockResolvedValue({
      ...dealDto(),
      broker_participations: [
        { broker_id: "broker-property", role: "captador", percentage: 30 },
        { broker_id: "broker-lead", role: "fechamento", percentage: 50 },
        { broker_id: "", role: "imobiliaria", percentage: 20 },
      ],
    });
    mocks.toastSuccess.mockReset();
    mocks.toastError.mockReset();
    mocks.allowedPermissions = new Set(["deals:write", "commissions:write", "finance:read"]);
  });

  it("usa as fontes canônicas e salva o plano configurado sem depender de reload", async () => {
    const onUpdated = vi.fn();
    render(
      <CommissionSplitCard
        deal={dealDto()}
        dealValue={980_000}
        onUpdated={onUpdated}
      />,
    );

    expect(await screen.findByText("Comissão total (6%)")).toBeInTheDocument();
    const editButton = await screen.findByRole("button", { name: "Editar" });
    await waitFor(() => expect(editButton).toBeEnabled());
    fireEvent.click(editButton);

    await screen.findByRole("dialog");
    await waitFor(() => expect(mocks.brokerGetAll).toHaveBeenCalled());

    const captador = roleRow("Captador");
    expect(within(captador).getByRole("combobox")).toHaveValue("broker-property");
    expect(within(captador).getByRole("textbox")).toHaveValue("30");

    const fechamento = roleRow("Fechamento");
    expect(within(fechamento).getByRole("combobox")).toHaveValue("broker-lead");
    expect(within(fechamento).getByRole("textbox")).toHaveValue("50");

    const imobiliaria = roleRow("Imobiliária");
    expect(within(imobiliaria).getByRole("textbox")).toHaveValue("20");
    expect(mocks.leadGetById).toHaveBeenCalledWith("lead-original");
    expect(mocks.leadGetById).not.toHaveBeenCalledWith("client-converted");
    expect(mocks.propertyGetById).toHaveBeenCalledWith("property-1");

    fireEvent.click(screen.getByRole("button", { name: "Salvar" }));
    fireEvent.click(screen.getByRole("button", { name: "Salvar alterações" }));

    await waitFor(() => {
      expect(mocks.updateBrokerParticipations).toHaveBeenCalledWith(
        "deal-1",
        [
          { broker_id: "broker-property", role: "captador", percentage: 30 },
          { broker_id: "broker-lead", role: "fechamento", percentage: 50 },
          { broker_id: "", role: "imobiliaria", percentage: 20 },
        ],
        undefined,
      );
    });
    expect(onUpdated).toHaveBeenCalledTimes(1);
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Distribuição de comissão atualizada");
  });

  it("corretor (sem commissions:write) não vê o editor de comissão", async () => {
    // O corretor tem deals:write — era por esse gate que ele conseguia editar o
    // próprio percentual. A comissão da negociação exige commissions:write
    // (admin, gestor, operações e financeiro).
    mocks.allowedPermissions = new Set(["deals:write", "finance:read"]);

    render(<CommissionSplitCard deal={dealDto()} dealValue={980_000} />);

    expect(await screen.findByText(/Comissão total/)).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Editar" })).toBeNull();
    });
    expect(mocks.updateBrokerParticipations).not.toHaveBeenCalled();
  });
});
