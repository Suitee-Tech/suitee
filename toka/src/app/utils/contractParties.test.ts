import { describe, expect, it } from "vitest";
import {
  buildRoleLegacy,
  MAX_PARTIES_PER_ROLE,
  PARTY_DEFAULT_PLACEHOLDER,
  toPartyLoopItem,
} from "./contractParties";

describe("toPartyLoopItem", () => {
  it("fills placeholders for missing fields", () => {
    const item = toPartyLoopItem({});
    expect(item.name).toBe(PARTY_DEFAULT_PLACEHOLDER);
    expect(item.email).toBe(PARTY_DEFAULT_PLACEHOLDER);
    expect(item.phone).toBe(PARTY_DEFAULT_PLACEHOLDER);
    expect(item.document).toBe(PARTY_DEFAULT_PLACEHOLDER);
    expect(item.is_primary).toBe(false);
    expect(item.share_pct).toBeUndefined();
  });

  it("preserves provided fields", () => {
    const item = toPartyLoopItem({
      id: "p-1",
      name: "Maria",
      email: "maria@example.com",
      phone: "11999999999",
      document: "123",
      is_primary: true,
      share_pct: 50,
    });
    expect(item.id).toBe("p-1");
    expect(item.name).toBe("Maria");
    expect(item.is_primary).toBe(true);
    expect(item.share_pct).toBe(50);
  });

  it("honors custom placeholder", () => {
    const item = toPartyLoopItem({}, "—");
    expect(item.name).toBe("—");
  });
});

describe("buildRoleLegacy", () => {
  it("emits aggregate fields for an empty role", () => {
    const out = buildRoleLegacy("tenant", "tenants", [], undefined);
    expect(out.tenants_count).toBe("0");
    expect(out.tenants_primary_name).toBe(PARTY_DEFAULT_PLACEHOLDER);
    expect(out.tenants_names_list).toBe(PARTY_DEFAULT_PLACEHOLDER);
    expect(out.tenants_documents_list).toBe(PARTY_DEFAULT_PLACEHOLDER);
  });

  it("emits suffix fields up to MAX_PARTIES_PER_ROLE even when role is empty", () => {
    const out = buildRoleLegacy("tenant", "tenants", [], undefined);
    expect(out.tenant_name).toBe(PARTY_DEFAULT_PLACEHOLDER);
    expect(out.tenant_2_name).toBe(PARTY_DEFAULT_PLACEHOLDER);
    expect(out.tenant_3_name).toBe(PARTY_DEFAULT_PLACEHOLDER);
    expect(out.tenant_4_name).toBe(PARTY_DEFAULT_PLACEHOLDER);
    expect(out.tenant_5_name).toBe(PARTY_DEFAULT_PLACEHOLDER);
    // Beyond MAX_PARTIES_PER_ROLE: must NOT be emitted.
    expect(out.tenant_6_name).toBeUndefined();
  });

  it("fills first slot without numeric suffix", () => {
    const tenants = [{ name: "João", document: "111", email: "j@x.com", phone: "1" }];
    const out = buildRoleLegacy("tenant", "tenants", tenants, tenants[0]);
    expect(out.tenant_name).toBe("João");
    expect(out.tenant_document).toBe("111");
    expect(out.tenant_email).toBe("j@x.com");
    expect(out.tenant_phone).toBe("1");
  });

  it("supports 3 parties with correct suffix indexing", () => {
    const tenants = [
      { name: "João", document: "111", email: "j@x.com", phone: "111" },
      { name: "Maria", document: "222", email: "m@x.com", phone: "222" },
      { name: "Pedro", document: "333", email: "p@x.com", phone: "333" },
    ];
    const out = buildRoleLegacy("tenant", "tenants", tenants, tenants[0]);
    expect(out.tenant_name).toBe("João");
    expect(out.tenant_2_name).toBe("Maria");
    expect(out.tenant_3_name).toBe("Pedro");
    expect(out.tenant_4_name).toBe(PARTY_DEFAULT_PLACEHOLDER);
    expect(out.tenant_5_name).toBe(PARTY_DEFAULT_PLACEHOLDER);
    expect(out.tenants_count).toBe("3");
    expect(out.tenants_names_list).toBe("João; Maria; Pedro");
    expect(out.tenants_documents_list).toBe("111; 222; 333");
  });

  it("uses primary in tenants_primary_name regardless of position", () => {
    const tenants = [
      { name: "Secondary", is_primary: false },
      { name: "Primary", is_primary: true },
    ];
    const primary = tenants[1];
    const out = buildRoleLegacy("tenant", "tenants", tenants, primary);
    expect(out.tenants_primary_name).toBe("Primary");
    // suffix order follows array order, not primary status
    expect(out.tenant_name).toBe("Secondary");
    expect(out.tenant_2_name).toBe("Primary");
  });

  it("falls back to placeholder when individual fields are missing", () => {
    const tenants = [{ name: "João" }]; // no document/email/phone
    const out = buildRoleLegacy("tenant", "tenants", tenants, tenants[0]);
    expect(out.tenant_name).toBe("João");
    expect(out.tenant_document).toBe(PARTY_DEFAULT_PLACEHOLDER);
    expect(out.tenant_email).toBe(PARTY_DEFAULT_PLACEHOLDER);
    expect(out.tenant_phone).toBe(PARTY_DEFAULT_PLACEHOLDER);
  });

  it("truncates names_list to first N (no separator pad) when more than MAX exist", () => {
    // Six tenants — only 5 get suffix slots, but names_list/documents_list show all.
    const tenants = Array.from({ length: MAX_PARTIES_PER_ROLE + 1 }, (_, i) => ({
      name: `T${i + 1}`,
      document: `D${i + 1}`,
    }));
    const out = buildRoleLegacy("tenant", "tenants", tenants, tenants[0]);
    expect(out.tenants_count).toBe(String(MAX_PARTIES_PER_ROLE + 1));
    expect(out.tenants_names_list.split("; ")).toHaveLength(MAX_PARTIES_PER_ROLE + 1);
    // Slot for the 6th tenant is NOT exposed via suffix variables — templates
    // that need >5 parties must use the {{#tenants}} loop instead.
    expect(out.tenant_6_name).toBeUndefined();
  });

  it("honors custom placeholder for empty values", () => {
    const out = buildRoleLegacy("tenant", "tenants", [], undefined, "—");
    expect(out.tenants_primary_name).toBe("—");
    expect(out.tenant_name).toBe("—");
  });

  it("works with non-tenant roles (e.g., guarantor, spouse, attorney)", () => {
    const guarantors = [{ name: "Carlos", document: "abc" }];
    const out = buildRoleLegacy("guarantor", "guarantors", guarantors, guarantors[0]);
    expect(out.guarantor_name).toBe("Carlos");
    expect(out.guarantors_count).toBe("1");

    const spouses = [{ name: "Ana" }];
    const out2 = buildRoleLegacy("spouse", "spouses", spouses, spouses[0]);
    expect(out2.spouse_name).toBe("Ana");
    expect(out2.spouses_count).toBe("1");
  });
});
