import { describe, expect, it } from "vitest";

import {
  availableActions,
  normalizeStatus,
  normalizeStatusOrDefault,
  statusBadgeVariant,
  statusLabel,
} from "../status";
import type { ChargeLancamento, CommissionLancamento, Lancamento, TransferLancamento } from "../types";

describe("normalizeStatus — charge", () => {
  it("maps backend UPPER codes", () => {
    expect(normalizeStatus("charge", "PENDING")).toBe("pendente");
    expect(normalizeStatus("charge", "PAID")).toBe("pago");
    expect(normalizeStatus("charge", "OVERDUE")).toBe("vencido");
    expect(normalizeStatus("charge", "CANCELLED")).toBe("cancelado");
    expect(normalizeStatus("charge", "PARTIALLY_PAID")).toBe("parcial");
  });

  it("accepts lowercase and mixed case", () => {
    expect(normalizeStatus("charge", "pending")).toBe("pendente");
    expect(normalizeStatus("charge", "Paid")).toBe("pago");
  });

  it("accepts PT-BR strings from the legacy Transaction layer", () => {
    expect(normalizeStatus("charge", "Pendente")).toBe("pendente");
    expect(normalizeStatus("charge", "Pago")).toBe("pago");
    expect(normalizeStatus("charge", "Em Atraso")).toBe("vencido");
    expect(normalizeStatus("charge", "Concluído")).toBe("pago");
    expect(normalizeStatus("charge", "Estornado")).toBe("revertido");
    expect(normalizeStatus("charge", "Reversada")).toBe("revertido");
  });

  it("returns null when raw is unknown or empty", () => {
    expect(normalizeStatus("charge", "")).toBeNull();
    expect(normalizeStatus("charge", null)).toBeNull();
    expect(normalizeStatus("charge", undefined)).toBeNull();
    expect(normalizeStatus("charge", "BAZINGA")).toBeNull();
  });
});

describe("normalizeStatus — commission", () => {
  it("maps the commission lifecycle", () => {
    expect(normalizeStatus("commission", "PENDING")).toBe("pendente");
    expect(normalizeStatus("commission", "APPROVED")).toBe("aprovado");
    expect(normalizeStatus("commission", "PAID")).toBe("pago");
    expect(normalizeStatus("commission", "CANCELLED")).toBe("cancelado");
  });
});

describe("normalizeStatus — transfer", () => {
  it("maps PROCESSED as the terminal happy state", () => {
    expect(normalizeStatus("transfer", "PROCESSED")).toBe("processado");
    expect(normalizeStatus("transfer", "APPROVED")).toBe("aprovado");
    expect(normalizeStatus("transfer", "PENDING")).toBe("pendente");
    expect(normalizeStatus("transfer", "CANCELLED")).toBe("cancelado");
  });
});

describe("normalizeStatus — transaction", () => {
  it("maps the canonical backend lifecycle without falling back to pendente", () => {
    expect(normalizeStatus("transaction", "PENDING")).toBe("pendente");
    expect(normalizeStatus("transaction", "PAID")).toBe("pago");
    expect(normalizeStatus("transaction", "OVERDUE")).toBe("vencido");
    expect(normalizeStatus("transaction", "CANCELLED")).toBe("cancelado");
    expect(normalizeStatus("transaction", "REVERSED")).toBe("revertido");
  });
});

describe("normalizeStatus — royalty (PayoutOrder)", () => {
  it("collapses DRAFT and PENDING_REVIEW into pendente", () => {
    expect(normalizeStatus("royalty", "DRAFT")).toBe("pendente");
    expect(normalizeStatus("royalty", "PENDING_REVIEW")).toBe("pendente");
  });

  it("treats FAILED as cancelado for UX purposes", () => {
    expect(normalizeStatus("royalty", "FAILED")).toBe("cancelado");
    expect(normalizeStatus("royalty", "CANCELLED")).toBe("cancelado");
  });

  it("maps PAID and APPROVED through", () => {
    expect(normalizeStatus("royalty", "PAID")).toBe("pago");
    expect(normalizeStatus("royalty", "APPROVED")).toBe("aprovado");
  });
});

describe("normalizeStatusOrDefault", () => {
  it("falls back to pendente on unknown", () => {
    expect(normalizeStatusOrDefault("charge", "MARS")).toBe("pendente");
    expect(normalizeStatusOrDefault("charge", null)).toBe("pendente");
  });
});

describe("statusLabel", () => {
  it("returns PT-BR labels", () => {
    expect(statusLabel("pendente")).toBe("Pendente");
    expect(statusLabel("pago")).toBe("Pago");
    expect(statusLabel("vencido")).toBe("Vencido");
    expect(statusLabel("processado")).toBe("Processado");
  });
});

describe("statusBadgeVariant", () => {
  it("groups successful states under the ok token", () => {
    expect(statusBadgeVariant("pago").text).toBe("text-status-ok");
    expect(statusBadgeVariant("processado").text).toBe("text-status-ok");
  });

  it("groups pending-ish states under the warning token", () => {
    expect(statusBadgeVariant("pendente").text).toBe("text-status-warning");
    expect(statusBadgeVariant("aprovado").text).toBe("text-status-warning");
    expect(statusBadgeVariant("parcial").text).toBe("text-status-warning");
  });

  it("flags vencido as critical", () => {
    expect(statusBadgeVariant("vencido").text).toBe("text-status-critical");
  });

  it("dims cancelado", () => {
    expect(statusBadgeVariant("cancelado").text).toBe("text-muted-foreground");
  });
});

// --- availableActions ---

function fakeCharge(status: ChargeLancamento["status"]): ChargeLancamento {
  return {
    id: "c1",
    kind: "charge",
    status,
    amount: 1000,
    currency: "BRL",
    counterparty: { kind: "tenant", label: "Locatário" },
    origin: { entity_kind: "contract" },
    timeline_summary: {},
    permissions_required: ["finance:read"],
    created_at: "",
    updated_at: "",
    kind_data: { reminders_count: 0 },
    source: {},
  };
}

function fakeCommission(
  status: CommissionLancamento["status"],
): CommissionLancamento {
  return {
    id: "co1",
    kind: "commission",
    status,
    amount: 500,
    currency: "BRL",
    counterparty: { kind: "broker", label: "Corretor" },
    origin: { entity_kind: "deal" },
    timeline_summary: {},
    permissions_required: ["finance:read"],
    created_at: "",
    updated_at: "",
    kind_data: {
      deductions_total: 0,
      participant_id: "b1",
      split_origin: "desconhecido",
    },
    source: {},
  };
}

function fakeTransfer(
  status: TransferLancamento["status"],
): TransferLancamento {
  return {
    id: "t1",
    kind: "transfer",
    status,
    amount: 2000,
    currency: "BRL",
    counterparty: { kind: "owner", label: "Proprietário" },
    origin: { entity_kind: "contract" },
    timeline_summary: {},
    permissions_required: ["finance:read"],
    created_at: "",
    updated_at: "",
    kind_data: {
      admin_fee_amount: 0,
      admin_fee_percent: 0,
      maintenance_deductions: 0,
      other_deductions: 0,
      gross_rent: 2000,
      history_count: 0,
    },
    source: {},
  };
}

describe("availableActions — charge", () => {
  it("offers markPaid/notify/cancel for pendente with finance:write", () => {
    const actions = availableActions(
      fakeCharge("pendente"),
      new Set(["finance:write"]),
    );
    expect(actions).toContain("markPaid");
    expect(actions).toContain("notify");
    expect(actions).toContain("cancel");
  });

  it("hides write actions without finance:write", () => {
    const actions = availableActions(fakeCharge("pendente"), new Set());
    expect(actions).not.toContain("markPaid");
    expect(actions).not.toContain("cancel");
    expect(actions).not.toContain("notify");
  });

  it("only offers viewReceipt for pago", () => {
    expect(
      availableActions(fakeCharge("pago"), new Set(["finance:write"])),
    ).toEqual(["viewReceipt"]);
  });
});

describe("availableActions — commission", () => {
  it("approve+editDeductions+cancel for pendente with finance:write", () => {
    const actions = availableActions(
      fakeCommission("pendente"),
      new Set(["finance:write"]),
    );
    expect(actions).toEqual(["approve", "editDeductions", "cancel"]);
  });

  it("markPaid+editDeductions+cancel for aprovado with finance:write", () => {
    const actions = availableActions(
      fakeCommission("aprovado"),
      new Set(["finance:write"]),
    );
    expect(actions).toEqual(["markPaid", "editDeductions", "cancel"]);
  });
});

describe("availableActions — transfer", () => {
  it("requires approve_transfer to approve", () => {
    const without = availableActions(
      fakeTransfer("pendente"),
      new Set(["finance:write"]),
    );
    expect(without).not.toContain("approve");

    const withApprove = availableActions(
      fakeTransfer("pendente"),
      new Set(["finance:write", "finance:approve_transfer"]),
    );
    expect(withApprove).toContain("approve");
    expect(withApprove).toContain("cancel");
  });

  it("processTransfer only after aprovado with approve_transfer", () => {
    const actions = availableActions(
      fakeTransfer("aprovado"),
      new Set(["finance:approve_transfer"]),
    );
    expect(actions).toEqual(["processTransfer"]);
  });
});

describe("availableActions — does not leak across kinds", () => {
  it("commission.aprovado does not get transfer.processTransfer", () => {
    const actions = availableActions(
      fakeCommission("aprovado") as Lancamento,
      new Set(["finance:approve_transfer", "finance:write"]),
    );
    expect(actions).not.toContain("processTransfer");
  });
});
