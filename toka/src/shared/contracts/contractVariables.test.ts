import { describe, expect, it } from "vitest";
import { CONTRACT_VARIABLES, CONTRACT_PARTY_FIELDS, getVariableByKey, isSystemVariable } from "./contractVariables";
import { isContractDateVariable } from "./contractDateVariables";
import { buildContractVariableData } from "../utils/contractVariableData";
import { toPartyLoopItem } from "../utils/contractParties";

const data = buildContractVariableData({
  dealDetails: { id: "deal", type: "RENT", value: 1000, parties: [] } as never,
  property: null, signers: [], buyerLead: null, sellerLead: null, brokersById: {}, ownerLeadsById: {},
  rentalStartDate: "2026-09-17", rentalEndDate: "2027-09-17", computedEndDate: "", firstChargeDate: "2026-09-17",
  firstChargeType: "same_month_pro_rata", defaultDueDay: 10, contract: null,
});

describe("contract variable vocabulary", () => {
  it("has unique keys and every mapped field is discoverable", () => {
    const keys = CONTRACT_VARIABLES.map((variable) => variable.key);
    expect(new Set(keys).size).toBe(keys.length);
    expect(Object.keys(data).filter((key) => !isSystemVariable(key))).toEqual([]);
    expect(keys.filter((key) => !(key in data) && !isContractDateVariable(key))).toEqual([]);
  });
  it("describes every field actually exposed in party objects and lists", () => {
    expect(CONTRACT_PARTY_FIELDS.map((field) => field.key).sort()).toEqual(Object.keys(toPartyLoopItem({})).sort());
    for (const key of ["buyer", "seller", "tenant", "landlord", "parties", "tenants", "owners", "spouses", "attorneys", "legal_representatives", "guardians", "curators"]) {
      expect(getVariableByKey(key)?.fields).toEqual(CONTRACT_PARTY_FIELDS);
      expect(getVariableByKey(key)?.example).toContain("{{");
    }
  });
  it("recognizes only cataloged dotted fields and bounded English date expressions", () => {
    expect(getVariableByKey("buyer.nationality")?.kind).toBe("text");
    expect(getVariableByKey("tenants.is_primary")?.kind).toBe("boolean");
    expect(isSystemVariable("current_date_plus_45_days_iso")).toBe(true);
    expect(isSystemVariable("contract_start_date_minus_1_years_written")).toBe(true);
    for (const key of ["buyer.unknown", "buyer.__proto__", "buyer.name.extra", "deal_value.unknown", "current_date_plus_999999_days", "current_date_plus_2_weeks", "current_date + 30"]) expect(isSystemVariable(key), key).toBe(false);
  });
  it("recognizes cataloged fields through one safe list index", () => {
    expect(getVariableByKey("buyers.0.name")?.kind).toBe("text");
    expect(getVariableByKey("tenants.3.nationality")?.kind).toBe("text");
    expect(getVariableByKey("parties.12.is_primary")?.kind).toBe("boolean");
    for (const key of ["buyers.0.unknown", "buyers.0.__proto__", "buyers.__proto__.name", "buyers.-1.name", "buyers.01.name", "buyers.1.0.name", "buyers.9007199254740992.name", "buyer.0.name", "seller.3.nationality", "buyers.0.name.extra"]) {
      expect(isSystemVariable(key), key).toBe(false);
    }
  });
  it("expõe listas e objetos para todos os papéis, inclusive vendedor e locador", () => {
    for (const key of ["sellers", "landlords", "signers", "guarantor", "witness", "spouse", "attorney", "legal_representative", "guardian", "curator", "owner"]) {
      expect(getVariableByKey(key), key).toBeDefined();
    }
    expect(getVariableByKey("sellers")?.kind).toBe("list");
    expect(getVariableByKey("signers")?.fields?.map((field) => field.key)).toContain("status");
    expect(getVariableByKey("guarantor.name")?.label).toContain("Fiador");
    expect(data.signers).toEqual([]);
  });

  it("mantém o comprador escalar na categoria Pessoas", () => {
    expect(getVariableByKey("buyer_name")?.category).toBe("Pessoas");
    expect(getVariableByKey("seller_name")?.category).toBe("Pessoas");
  });

  it("preserves the existing broker keys and exposes English replacements", () => {
    expect(getVariableByKey("broker_captador_name")?.legacyAliasFor).toBe("broker_listing_name");
    expect(getVariableByKey("broker_visitador_name")?.legacyAliasFor).toBe("broker_visiting_name");
    expect(getVariableByKey("broker_fechamento_name")?.legacyAliasFor).toBe("broker_closing_name");
  });
});
