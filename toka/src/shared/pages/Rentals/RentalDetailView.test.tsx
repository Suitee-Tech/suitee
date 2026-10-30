import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";

import { RentalDetailView, type RentalDetailViewProps } from "./RentalDetailView";

const repasse = (overrides: Record<string, unknown> = {}) => ({
  id: "transfer-1",
  month: "2026-09",
  grossRent: 1_000,
  adminFee: 100,
  maintenanceDeductions: 0,
  utilityDeductions: 0,
  netRepasse: 900,
  status: "Pendente",
  action: "approve",
  settlementReady: true,
  approvedAt: null,
  approvedBy: null,
  processedAt: null,
  processedBy: null,
  history: [],
  _source: "transfer",
  ...overrides,
});

const buildProps = (overrides: Partial<RentalDetailViewProps> = {}): RentalDetailViewProps => {
  const currentRepasse = repasse();
  return {
    selectedContract: {
      id: "contract-1",
      status: "ACTIVE",
      client: "Cliente",
      property_title: "Imóvel",
      value: 1_000,
    },
    perms: {
      chargesWrite: true,
      chargesRead: true,
      financeRead: true,
      financeWrite: true,
      financeApproveTransfer: true,
      contractsWrite: true,
    },
    detailTab: "repasses",
    setDetailTab: vi.fn(),
    setView: vi.fn(),
    setActiveModal: vi.fn(),
    setAuditContractId: vi.fn(),
    setAuditLabel: vi.fn(),
    openModal: vi.fn(),
    handleTerminateContract: vi.fn(),
    terminateContractMutation: { isPending: false, mutate: vi.fn() },
    formatCurrency: (value) => `R$ ${value}`,
    chargeStats: { total: 0, paid: 0, overdue: 0, pending: 0 },
    charges: [],
    financialHistory: [],
    utilityAccounts: [],
    utilityStats: { tenantTotalCents: 0, tenantCount: 0, ownerTotalCents: 0, ownerCount: 0, inactiveCount: 0 },
    utilityTypeLabel: (type) => type,
    utilityTypeIcon: () => null,
    handleRemoveUtilityAccount: vi.fn(),
    onLaunchUtilityCharge: vi.fn(),
    deleteUtilityAccountMutation: { isPending: false, mutate: vi.fn() },
    adminFeePct: 10,
    filteredCharges: [],
    hasActiveChargeFilter: false,
    clearChargeFilters: vi.fn(),
    selectedChargeIds: new Set(),
    visibleSelectableIds: [],
    allVisibleSelected: false,
    toggleChargeSelected: vi.fn(),
    toggleSelectAllVisible: vi.fn(),
    clearChargeSelection: vi.fn(),
    handleBulkMarkPaid: vi.fn(),
    handleBulkNotify: vi.fn(),
    chargeFilter: "Todos",
    setChargeFilter: vi.fn(),
    chargeSearch: "",
    setChargeSearch: vi.fn(),
    chargeSortBy: "date_desc",
    setChargeSortBy: vi.fn(),
    chargePeriod: "all",
    setChargePeriod: vi.fn(),
    setAuditChargeId: vi.fn(),
    handleMarkAsPaid: vi.fn(),
    handleViewChargeReceipt: vi.fn(),
    handleSendChargeReminder: vi.fn(),
    realTransfers: [{ id: currentRepasse.id }],
    repasses: [currentRepasse],
    filteredRepasses: [currentRepasse],
    handleDownloadRepasseReceipt: vi.fn(),
    approveTransferMutation: { isPending: false, mutate: vi.fn() },
    processTransferMutation: { isPending: false, mutate: vi.fn() },
    setAuditTransferId: vi.fn(),
    resolveActorName: (actor) => actor ?? "",
    repasseStatusFilter: "Todos",
    setRepasseStatusFilter: vi.fn(),
    repasseSearch: "",
    setRepasseSearch: vi.fn(),
    repasseSortBy: "month_desc",
    setRepasseSortBy: vi.fn(),
    repassePeriod: "all",
    setRepassePeriod: vi.fn(),
    hasActiveRepasseFilter: false,
    clearRepasseFilters: vi.fn(),
    selectedTransferIds: new Set(),
    selectableTransferIds: [currentRepasse.id],
    allVisibleTransfersSelected: false,
    toggleTransferSelected: vi.fn(),
    toggleSelectAllTransfers: vi.fn(),
    clearTransferSelection: vi.fn(),
    handleBulkApproveRepasses: vi.fn(),
    handleApproveRepasse: vi.fn(async () => true),
    handleProcessRepasse: vi.fn(),
    handleSettleRepasseWithoutBalance: vi.fn(),
    ...overrides,
  };
};

const renderView = (props: RentalDetailViewProps) =>
  render(
    <MemoryRouter>
      <RentalDetailView {...props} />
    </MemoryRouter>,
  );

describe("RentalDetailView financial transfer actions", () => {
  it("oferece somente aprovação para repasse pendente", async () => {
    const user = userEvent.setup();
    const props = buildProps();
    renderView(props);

    expect(screen.queryByRole("button", { name: /Processar com comprovante/i })).not.toBeInTheDocument();
    await user.click(screen.getAllByRole("button", { name: /Aprovar repasse|Aprovar$/i })[0]);
    expect(props.handleApproveRepasse).toHaveBeenCalledWith("transfer-1");
    expect(props.handleProcessRepasse).not.toHaveBeenCalled();
  });

  it("oferece processamento com comprovante somente após aprovação", async () => {
    const user = userEvent.setup();
    const approved = repasse({ status: "Aprovado", action: "process" });
    const props = buildProps({ repasses: [approved], filteredRepasses: [approved], selectableTransferIds: [] });
    renderView(props);

    expect(screen.queryByRole("button", { name: /Aprovar repasse|Aprovar$/i })).not.toBeInTheDocument();
    await user.click(screen.getAllByRole("button", { name: /Processar com comprovante|Processar$/i })[0]);
    expect(props.handleProcessRepasse).toHaveBeenCalledWith("transfer-1");
  });

  it("mantém repasse a apurar sem ação mutável", () => {
    const unresolved = repasse({ status: "A apurar", action: null, settlementReady: false });
    const props = buildProps({ repasses: [unresolved], filteredRepasses: [unresolved], selectableTransferIds: [] });
    renderView(props);

    expect(screen.getByRole("link", { name: "Revisar composição" })).toHaveAttribute("href", "/financeiro/repasses");
    expect(screen.queryByRole("button", { name: /Aprovar|Processar/i })).not.toBeInTheDocument();
  });

  it("respeita a permissão canônica de aprovação", () => {
    const props = buildProps({
      perms: {
        ...buildProps().perms,
        financeApproveTransfer: false,
      },
    });
    renderView(props);

    expect(screen.queryByRole("button", { name: /Aprovar repasse|Aprovar$/i })).not.toBeInTheDocument();
    expect(screen.getByLabelText("Selecionar repasse 2026-09")).toBeDisabled();
  });

  it("oferece conclusão sem saldo para repasse zerado e não pede comprovante", async () => {
    const user = userEvent.setup();
    const zero = repasse({ status: "Sem saldo", action: "settle", settlementReady: false, netRepasse: 0 });
    const props = buildProps({ repasses: [zero], filteredRepasses: [zero], selectableTransferIds: [] });
    renderView(props);

    expect(screen.getAllByText("Sem saldo").length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: /Aprovar/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Processar/i })).not.toBeInTheDocument();

    const actions = screen.getAllByRole("button", { name: /Concluir sem saldo/i });
    expect(actions.length).toBeGreaterThan(0);
    await user.click(actions[0]);
    expect(props.handleSettleRepasseWithoutBalance).toHaveBeenCalledWith("transfer-1");
  });

  it("não oferece ação para repasse zerado já liquidado sem saldo", () => {
    const settled = repasse({ status: "Liquidado sem saldo", action: null, settlementReady: false, netRepasse: 0 });
    const props = buildProps({ repasses: [settled], filteredRepasses: [settled], selectableTransferIds: [] });
    renderView(props);

    expect(screen.getAllByText("Liquidado sem saldo").length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: /Concluir sem saldo/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Aprovar|Processar/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Revisar composição" })).not.toBeInTheDocument();
  });
});

describe("RentalDetailView charge settlement actions", () => {
  it("permite completar uma cobrança parcial com conferência individual", async () => {
    const user = userEvent.setup();
    const partial = {
      id: "charge-1",
      description: "Aluguel setembro",
      type: "Aluguel",
      value: 1_000,
      dueDate: "2026-09-10",
      status: "Parcial",
      filterStatus: "Parcial",
      remindersCount: 0,
      _source: "charge",
      _canMarkPaid: true,
      _canNotify: false,
    };
    const props = buildProps({
      detailTab: "cobrancas",
      charges: [partial],
      filteredCharges: [partial],
      visibleSelectableIds: [partial.id],
    });
    renderView(props);

    expect(screen.getByRole("button", { name: "Ver recibo" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Dar baixa" }));
    expect(props.handleMarkAsPaid).toHaveBeenCalledWith("charge-1");
  });

  it("não torna cobrança encerrada selecionável nem baixável", () => {
    const cancelled = {
      id: "charge-cancelled",
      description: "Cobrança cancelada",
      type: "Aluguel",
      value: 1_000,
      dueDate: "2026-09-10",
      status: "Cancelada",
      filterStatus: "Encerrada",
      remindersCount: 0,
      _source: "charge",
      _canMarkPaid: false,
      _canNotify: false,
    };
    const props = buildProps({ detailTab: "cobrancas", charges: [cancelled], filteredCharges: [cancelled] });
    renderView(props);

    expect(screen.getByLabelText("Selecionar cobrança charge-cancelled")).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Dar baixa" })).not.toBeInTheDocument();
  });
});
