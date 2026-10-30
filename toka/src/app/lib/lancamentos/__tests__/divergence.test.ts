import { describe, expect, it } from "vitest";

import {
  computeDivergence,
  describeDivergence,
  hasDivergence,
} from "../divergence";
import type {
  ChargeLancamento,
  CommissionLancamento,
  TransferLancamento,
} from "../types";

function chargeLanc(
  over: Partial<ChargeLancamento> & {
    kind_data?: Partial<ChargeLancamento["kind_data"]>;
  } = {},
): ChargeLancamento {
  return {
    id: "ch-1",
    kind: "charge",
    status: over.status ?? "pago",
    amount: over.amount ?? 1000,
    currency: "BRL",
    counterparty: { kind: "tenant", label: "Locatário" },
    origin: { entity_kind: "contract" },
    timeline_summary: {},
    permissions_required: ["finance:read"],
    created_at: "",
    updated_at: "",
    kind_data: {
      reminders_count: 0,
      paid_amount: 1000,
      ...(over.kind_data ?? {}),
    },
    source: {},
    ...over,
  };
}

describe("hasDivergence — charge", () => {
  it("returns false when paid_amount matches amount", () => {
    expect(
      hasDivergence(chargeLanc({ amount: 1000, kind_data: { reminders_count: 0, paid_amount: 1000 } })),
    ).toBe(false);
  });

  it("tolerates rounding noise under 1 centavo", () => {
    expect(
      hasDivergence(
        chargeLanc({ amount: 1000, kind_data: { reminders_count: 0, paid_amount: 1000.009 } }),
      ),
    ).toBe(false);
  });

  it("flags overpayment beyond tolerance", () => {
    expect(
      hasDivergence(
        chargeLanc({ amount: 1000, kind_data: { reminders_count: 0, paid_amount: 1050 } }),
      ),
    ).toBe(true);
  });

  it("flags underpayment beyond tolerance", () => {
    expect(
      hasDivergence(
        chargeLanc({ amount: 1000, kind_data: { reminders_count: 0, paid_amount: 980 } }),
      ),
    ).toBe(true);
  });

  it("never flags charges that haven't settled yet", () => {
    expect(
      hasDivergence(
        chargeLanc({
          status: "pendente",
          amount: 1000,
          kind_data: { reminders_count: 0, paid_amount: 500 },
        }),
      ),
    ).toBe(false);
    expect(
      hasDivergence(
        chargeLanc({
          status: "vencido",
          amount: 1000,
          kind_data: { reminders_count: 0, paid_amount: 0 },
        }),
      ),
    ).toBe(false);
  });

  it("never flags when paid_amount is absent (incomplete data)", () => {
    expect(
      hasDivergence(
        chargeLanc({
          amount: 1000,
          kind_data: { reminders_count: 0, paid_amount: undefined },
        }),
      ),
    ).toBe(false);
  });
});

function commLanc(
  over: Partial<CommissionLancamento> & {
    kind_data?: Partial<CommissionLancamento["kind_data"]>;
  } = {},
): CommissionLancamento {
  return {
    id: "co-1",
    kind: "commission",
    status: over.status ?? "pago",
    amount: over.amount ?? 500,
    net_amount: over.net_amount,
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
      ...(over.kind_data ?? {}),
    },
    source: {},
    ...over,
  };
}

describe("hasDivergence — commission", () => {
  it("flags net != gross - deductions beyond tolerance", () => {
    const l = commLanc({
      net_amount: 380,
      kind_data: {
        deductions_total: 100,
        gross_amount: 500,
        participant_id: "b1",
        split_origin: "desconhecido",
      },
    });
    // expected = 500 - 100 = 400; net = 380 → delta -20
    expect(hasDivergence(l)).toBe(true);
    expect(computeDivergence(l)?.reason).toBe("amount_mismatch");
  });

  it("tolerates rounding noise in net calculation", () => {
    const l = commLanc({
      // status aprovado evita a regra missing_proof e isola o net check
      status: "aprovado",
      net_amount: 400.009,
      kind_data: {
        deductions_total: 100,
        gross_amount: 500,
        participant_id: "b1",
        split_origin: "desconhecido",
      },
    });
    expect(hasDivergence(l)).toBe(false);
  });

  it("skips amount check when gross or net is absent", () => {
    const l = commLanc({
      status: "aprovado",
      net_amount: 380,
      kind_data: {
        deductions_total: 100,
        // no gross_amount
        participant_id: "b1",
        split_origin: "desconhecido",
      },
    });
    expect(hasDivergence(l)).toBe(false);
  });

  it("flags PAID without proof_url", () => {
    const l = commLanc({
      status: "pago",
      kind_data: {
        deductions_total: 0,
        participant_id: "b1",
        split_origin: "desconhecido",
        // no proof_url
      },
    });
    expect(hasDivergence(l)).toBe(true);
    expect(computeDivergence(l)?.reason).toBe("missing_proof");
  });

  it("does NOT flag PAID with proof_url", () => {
    const l = commLanc({
      status: "pago",
      kind_data: {
        deductions_total: 0,
        participant_id: "b1",
        split_origin: "desconhecido",
        proof_url: "https://example.com/proof.pdf",
      },
    });
    expect(hasDivergence(l)).toBe(false);
  });

  it("does NOT flag missing proof when status is not PAID", () => {
    const l = commLanc({
      status: "aprovado",
      kind_data: {
        deductions_total: 0,
        participant_id: "b1",
        split_origin: "desconhecido",
      },
    });
    expect(hasDivergence(l)).toBe(false);
  });

  it("amount_mismatch takes precedence over missing_proof", () => {
    const l = commLanc({
      net_amount: 380,
      status: "pago",
      kind_data: {
        gross_amount: 500,
        deductions_total: 100,
        participant_id: "b1",
        split_origin: "desconhecido",
        // no proof_url either — both rules would fire; amount wins
      },
    });
    expect(computeDivergence(l)?.reason).toBe("amount_mismatch");
  });
});

function trLanc(
  over: Partial<TransferLancamento> & {
    kind_data?: Partial<TransferLancamento["kind_data"]>;
  } = {},
): TransferLancamento {
  return {
    id: "tr-1",
    kind: "transfer",
    status: over.status ?? "processado",
    amount: over.amount ?? 2000,
    net_amount: over.net_amount,
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
      processed_at: "2026-05-10T00:00:00Z",
      ...(over.kind_data ?? {}),
    },
    source: {},
    ...over,
  };
}

describe("hasDivergence — transfer", () => {
  it("flags net != gross - admin - manutencao - outras", () => {
    const l = trLanc({
      net_amount: 1500, // expected = 2000 - 200 - 100 - 50 = 1650
      kind_data: {
        admin_fee_amount: 200,
        admin_fee_percent: 10,
        maintenance_deductions: 100,
        other_deductions: 50,
        gross_rent: 2000,
        history_count: 0,
        processed_at: "2026-05-10T00:00:00Z",
      },
    });
    expect(hasDivergence(l)).toBe(true);
    expect(computeDivergence(l)?.reason).toBe("amount_mismatch");
  });

  it("does NOT flag when net = gross - admin - manutencao - outras", () => {
    const l = trLanc({
      net_amount: 1650,
      kind_data: {
        admin_fee_amount: 200,
        admin_fee_percent: 10,
        maintenance_deductions: 100,
        other_deductions: 50,
        gross_rent: 2000,
        history_count: 0,
        processed_at: "2026-05-10T00:00:00Z",
      },
    });
    expect(hasDivergence(l)).toBe(false);
  });

  it("uses the allocation-aware owner balance for a sale transfer", () => {
    const l = trLanc({
      status: "pendente",
      amount: 980_000,
      net_amount: 921_200,
      kind_data: {
        admin_fee_amount: 0,
        admin_fee_percent: 0,
        maintenance_deductions: 0,
        other_deductions: 0,
        gross_rent: 980_000,
        history_count: 1,
        settlement_type: "sale",
        economic_breakdown: {
          flow: "sale",
          gross_cents: 98_000_000,
          owner_payable_cents: 92_120_000,
          reserved_cents: 0,
          available_cents: 92_120_000,
          agency_revenue_cents: 2_352_000,
          commission_payable_cents: 3_528_000,
          partner_payable_cents: 0,
          agency_retained_cents: 2_352_000,
          discount_cents: 0,
          deductions_cents: 0,
        },
      },
    });

    expect(computeDivergence(l)).toBeNull();
  });

  it("compares an allocation-aware rental transfer with the available owner balance", () => {
    const l = trLanc({
      status: "aprovado",
      amount: 6_000,
      net_amount: 4_600,
      kind_data: {
        admin_fee_amount: 400,
        admin_fee_percent: 8,
        maintenance_deductions: 0,
        other_deductions: 0,
        gross_rent: 6_000,
        history_count: 2,
        settlement_type: "rent",
        economic_breakdown: {
          flow: "rent",
          gross_cents: 600_000,
          owner_payable_cents: 560_000,
          reserved_cents: 100_000,
          available_cents: 460_000,
          agency_revenue_cents: 40_000,
          commission_payable_cents: 0,
          partner_payable_cents: 0,
          agency_retained_cents: 40_000,
          discount_cents: 0,
          deductions_cents: 100_000,
        },
      },
    });

    expect(computeDivergence(l)).toBeNull();
  });

  it("flags PROCESSED without processed_at timestamp", () => {
    const l = trLanc({
      status: "processado",
      net_amount: 2000,
      kind_data: {
        admin_fee_amount: 0,
        admin_fee_percent: 0,
        maintenance_deductions: 0,
        other_deductions: 0,
        gross_rent: 2000,
        history_count: 0,
        // no processed_at
      },
    });
    expect(hasDivergence(l)).toBe(true);
    expect(computeDivergence(l)?.reason).toBe("missing_timestamp");
  });

  it("does NOT flag non-PROCESSED missing timestamp", () => {
    const l = trLanc({
      status: "aprovado",
      net_amount: 2000,
      kind_data: {
        admin_fee_amount: 0,
        admin_fee_percent: 0,
        maintenance_deductions: 0,
        other_deductions: 0,
        gross_rent: 2000,
        history_count: 0,
      },
    });
    expect(hasDivergence(l)).toBe(false);
  });
});

describe("hasDivergence — royalty", () => {
  it("flags PAID without proof_url", () => {
    const l: import("../types").RoyaltyLancamento = {
      id: "rl-1",
      kind: "royalty",
      status: "pago",
      amount: 750,
      currency: "BRL",
      counterparty: { kind: "partner", label: "Franqueador X" },
      origin: { entity_kind: "partnership" },
      timeline_summary: {},
      permissions_required: ["finance:read", "partnerships:read"],
      created_at: "",
      updated_at: "",
      kind_data: {},
      source: {},
    };
    expect(hasDivergence(l)).toBe(true);
    expect(computeDivergence(l)?.reason).toBe("missing_proof");
  });

  it("does NOT flag PAID with proof_url", () => {
    const l: import("../types").RoyaltyLancamento = {
      id: "rl-2",
      kind: "royalty",
      status: "pago",
      amount: 750,
      currency: "BRL",
      counterparty: { kind: "partner", label: "Franqueador X" },
      origin: { entity_kind: "partnership" },
      timeline_summary: {},
      permissions_required: ["finance:read", "partnerships:read"],
      created_at: "",
      updated_at: "",
      kind_data: { proof_url: "https://example.com/proof.pdf" },
      source: {},
    };
    expect(hasDivergence(l)).toBe(false);
  });

  it("does NOT flag non-PAID royalty without proof_url", () => {
    const l: import("../types").RoyaltyLancamento = {
      id: "rl-3",
      kind: "royalty",
      status: "aprovado",
      amount: 750,
      currency: "BRL",
      counterparty: { kind: "partner", label: "Franqueador X" },
      origin: { entity_kind: "partnership" },
      timeline_summary: {},
      permissions_required: ["finance:read", "partnerships:read"],
      created_at: "",
      updated_at: "",
      kind_data: {},
      source: {},
    };
    expect(hasDivergence(l)).toBe(false);
  });
});

describe("computeDivergence", () => {
  it("returns full info with positive delta on overpayment", () => {
    const info = computeDivergence(
      chargeLanc({ amount: 1000, kind_data: { reminders_count: 0, paid_amount: 1050 } }),
    );
    expect(info).toEqual({
      reason: "amount_mismatch",
      delta: 50,
      expected: 1000,
      received: 1050,
    });
  });

  it("returns full info with negative delta on underpayment", () => {
    const info = computeDivergence(
      chargeLanc({ amount: 1000, kind_data: { reminders_count: 0, paid_amount: 980 } }),
    );
    expect(info).toEqual({
      reason: "amount_mismatch",
      delta: -20,
      expected: 1000,
      received: 980,
    });
  });

  it("returns null when no divergence", () => {
    expect(
      computeDivergence(
        chargeLanc({ amount: 1000, kind_data: { reminders_count: 0, paid_amount: 1000 } }),
      ),
    ).toBeNull();
  });
});

describe("describeDivergence", () => {
  it("formats overpayment with + sign and BRL", () => {
    expect(
      describeDivergence({
        reason: "amount_mismatch",
        delta: 50,
        expected: 1000,
        received: 1050,
      }),
    ).toMatch(/Diverg[êe]ncia \+R\$\s?50,00/);
  });

  it("formats underpayment with minus sign", () => {
    expect(
      describeDivergence({
        reason: "amount_mismatch",
        delta: -20,
        expected: 1000,
        received: 980,
      }),
    ).toMatch(/Diverg[êe]ncia .R\$\s?20,00/);
  });

  it("describes missing_proof in PT-BR", () => {
    expect(describeDivergence({ reason: "missing_proof" })).toBe(
      "Pago sem comprovante anexado",
    );
  });

  it("describes missing_timestamp in PT-BR", () => {
    expect(describeDivergence({ reason: "missing_timestamp" })).toBe(
      "Status terminal sem timestamp",
    );
  });
});
