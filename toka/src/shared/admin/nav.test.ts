import { describe, expect, it } from "vitest";

import { adminNavGroups, getVisibleAdminNavGroups } from "./nav";

const finance = adminNavGroups.find((group) => group.id === "finance");
const rentalGroup = adminNavGroups.find((group) => group.title === "Gestão de Locação");

describe("admin nav — rótulos e agrupamento", () => {
  it("alinha os rótulos com o destino real", () => {
    expect(rentalGroup?.items.find((item) => item.to === "/locacao")?.label).toBe("Contratos de locação");
    expect(finance?.items.find((item) => item.to === "/financeiro/lancamentos")?.label).toBe("Lançamentos");
    expect(finance?.items.find((item) => item.to === "/inadimplencia")?.label).toBe("Inadimplência");
  });

  it("torna o Financeiro colapsável e mantém os 12 destinos", () => {
    expect(finance?.collapsible).toBe(true);
    expect(finance?.items).toHaveLength(12);
  });

  it("mantém os filtros de permissão ao recolher grupos", () => {
    const visible = getVisibleAdminNavGroups({
      user: { permission: "user", functional_role: "corretor" },
      can: (permission) => permission === "finance:read" || permission === "module:finance",
    });
    const visibleFinance = visible.find((group) => group.id === "finance");
    expect(visibleFinance?.items.every((item) => item.permission === "finance:read")).toBe(true);
  });
});
