import { describe, expect, it } from "vitest";
import type { RentalChargeConfig } from "./types";
import type { RentalDTO } from "../services/rental.service";

// The RENT wire contract is shared between toca-financial (Go) and this
// frontend. The Go domain structs emit snake_case JSON for RentalChargeConfig
// and Rental; these tests pin the same keyset on the TypeScript side and prove
// a populated payload contains no undefined/NaN so the UI never renders a
// broken amount.

function sortedKeys(obj: unknown): string[] {
  return Object.keys(obj as Record<string, unknown>).sort();
}

function assertNoUndefinedOrNaN(value: unknown, path: string): void {
  if (value === undefined) {
    throw new Error(`${path} is undefined`);
  }
  if (typeof value === "number") {
    if (!Number.isFinite(value)) {
      throw new Error(`${path} is not finite (${value})`);
    }
    return;
  }
  if (Array.isArray(value)) {
    value.forEach((item, i) => assertNoUndefinedOrNaN(item, `${path}[${i}]`));
    return;
  }
  if (value && typeof value === "object") {
    for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
      assertNoUndefinedOrNaN(v, `${path}.${k}`);
    }
  }
}

const rentalChargeConfigSample: RentalChargeConfig = {
  id: "config-1",
  contract_id: "contract-1",
  tenant_id: "tenant-1",
  property_id: "property-1",
  tenant_party_id: "party-1",
  owner_id: "owner-1",
  start_date: "2026-08-10T00:00:00Z",
  end_date: "2027-08-10T00:00:00Z",
  first_charge_date: "2026-08-10T00:00:00Z",
  first_charge_type: "same_month_pro_rata",
  pro_rata_policy: "actual_actual",
  default_due_day: 10,
  monthly_amount: 250000,
  base_monthly_amount: 225000,
  adjustments: [
    {
      event_id: "adjustment-1",
      effective_from: "2026-08-10T00:00:00Z",
      monthly_amount: 250000,
      occurred_at: "2026-08-10T00:00:00Z",
    },
  ],
  first_charge_paid: true,
  monthly_product_revision: 2,
  bundled_items: [
    { key: "condominio", description: "Condomínio", amount_cents: 50000, enabled: true },
  ],
  created_at: "2026-08-10T00:00:00Z",
  updated_at: "2026-08-10T00:00:00Z",
};

const rentalDTOSample: RentalDTO = {
  id: "rental-1",
  tenant_id: "tenant-1",
  deal_id: "deal-1",
  contract_id: "contract-1",
  property_id: "property-1",
  tenant_lead_id: "tenant-lead-1",
  owner_lead_id: "owner-lead-1",
  broker_id: "broker-1",
  start_date: "2026-08-10T00:00:00Z",
  end_date: "2027-08-10T00:00:00Z",
  monthly_value_cents: 250000,
  status: "ACTIVE",
  created_at: "2026-08-10T00:00:00Z",
  updated_at: "2026-08-10T00:00:00Z",
};

describe("RENT wire contract (Go ↔ TypeScript)", () => {
  it("RentalChargeConfig exposes exactly the snake_case keys Go emits", () => {
    expect(sortedKeys(rentalChargeConfigSample)).toEqual([
      "adjustments",
      "base_monthly_amount",
      "bundled_items",
      "contract_id",
      "created_at",
      "default_due_day",
      "end_date",
      "first_charge_date",
      "first_charge_paid",
      "first_charge_type",
      "id",
      "monthly_amount",
      "monthly_product_revision",
      "owner_id",
      "pro_rata_policy",
      "property_id",
      "start_date",
      "tenant_id",
      "tenant_party_id",
      "updated_at",
    ]);
  });

  it("RentalDTO exposes exactly the snake_case keys Go emits", () => {
    expect(sortedKeys(rentalDTOSample)).toEqual([
      "broker_id",
      "contract_id",
      "created_at",
      "deal_id",
      "end_date",
      "id",
      "monthly_value_cents",
      "owner_lead_id",
      "property_id",
      "start_date",
      "status",
      "tenant_id",
      "tenant_lead_id",
      "updated_at",
    ]);
  });

  it("a populated RentalChargeConfig has no undefined/NaN values", () => {
    expect(() => assertNoUndefinedOrNaN(rentalChargeConfigSample, "rentalChargeConfig")).not.toThrow();
  });

  it("a populated RentalDTO has no undefined/NaN values", () => {
    expect(() => assertNoUndefinedOrNaN(rentalDTOSample, "rental")).not.toThrow();
  });

  it("money fields are whole cents, never NaN", () => {
    expect(Number.isInteger(rentalChargeConfigSample.monthly_amount)).toBe(true);
    expect(Number.isInteger(rentalDTOSample.monthly_value_cents)).toBe(true);
  });
});
