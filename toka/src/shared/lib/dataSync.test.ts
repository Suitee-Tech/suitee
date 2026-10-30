import { describe, expect, it } from "vitest";
import {
  dataChangeMatchesResources,
  inferDataResources,
  queryKeyMatchesResources,
  resourcesForApiMutation,
  resourcesForRealtimeEvent,
} from "./dataSync";

describe("dataSync resource routing", () => {
  it("maps successful API mutations to the affected domains", () => {
    expect(resourcesForApiMutation("PATCH", "/properties/P-1/media/images")).toEqual(
      expect.arrayContaining(["properties", "deals"]),
    );
    expect(resourcesForApiMutation("post", "/charges/C-1/mark-paid")).toEqual([
      "charges",
      "finance",
    ]);
    expect(resourcesForApiMutation("delete", "/contracts/C-1/documents/D-1")).toEqual(
      expect.arrayContaining(["contracts", "documents", "checklists", "deals"]),
    );
  });

  it("does not publish reads or authentication lifecycle requests", () => {
    expect(resourcesForApiMutation("GET", "/leads")).toBeNull();
    expect(resourcesForApiMutation("POST", "/auth/switch-context")).toBeNull();
    expect(resourcesForApiMutation("POST", "/auth/logout-all")).toBeNull();
  });

  it("falls back to a safe full reconciliation for an unknown mutation", () => {
    expect(resourcesForApiMutation("POST", "/custom-command/run")).toEqual(["all"]);
  });

  it("maps websocket names written with dots or underscores", () => {
    expect(resourcesForRealtimeEvent("deal.status_changed")).toContain("deals");
    expect(resourcesForRealtimeEvent("lead_assigned")).toContain("leads");
    expect(resourcesForRealtimeEvent("checklist.solicited")).toEqual([
      "checklists",
      "deals",
    ]);
  });

  it("normalizes Portuguese and English resource aliases", () => {
    expect(inferDataResources("vistorias imóveis cobranças")).toEqual(
      expect.arrayContaining(["inspections", "properties", "charges", "finance"]),
    );
  });
});

describe("dataSync cache matching", () => {
  it("matches filtered and fallback query variants by canonical domain", () => {
    expect(queryKeyMatchesResources(["property-fallback", "P-1"], ["properties"])).toBe(true);
    expect(queryKeyMatchesResources(["leads", { status: "new" }], ["leads"])).toBe(true);
    expect(queryKeyMatchesResources(["lancamentos", { month: "2026-07" }], ["finance"])).toBe(true);
    expect(queryKeyMatchesResources(["documents", "D-1"], ["properties"])).toBe(false);
  });

  it("refreshes derived dashboards and respects a full reconciliation", () => {
    expect(queryKeyMatchesResources(["dashboard", "summary"], ["deals"])).toBe(true);
    expect(queryKeyMatchesResources(["anything"], ["all"])).toBe(true);
  });

  it("matches relationship-expanded resources", () => {
    expect(dataChangeMatchesResources({ resources: ["charges", "finance"] }, ["finance"])).toBe(true);
    expect(dataChangeMatchesResources({ resources: ["signatures", "documents", "contracts"] }, ["documents"])).toBe(true);
    expect(dataChangeMatchesResources({ resources: ["calendar"] }, ["leads"])).toBe(false);
  });
});

it("cadastrar profissional não recarrega permissões nem desmonta o agendamento", () => {
  expect(resourcesForApiMutation("post", "/suppliers")).toEqual(["suppliers"]);
  expect(queryKeyMatchesResources(["suppliers", "list"], ["suppliers"])).toBe(true);
  expect(queryKeyMatchesResources(["permissions"], ["suppliers"])).toBe(false);
});

it("atualiza parcerias e dados compartilhados após mudança do acordo", () => {
  expect(resourcesForRealtimeEvent("partnership_changed")).toEqual(expect.arrayContaining(["partnerships", "properties", "leads"]));
});
