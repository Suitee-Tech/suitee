import { describe, expect, it } from "vitest";

import { isPortalNavigationItemActive, PORTAL_BOTTOM_NAV_ITEMS, portalNavItems } from "./navigation";
import { resolvePortalPersona } from "./lib/portalPersona";

const item = (path: string) => PORTAL_BOTTOM_NAV_ITEMS.find((entry) => entry.path === path)!;

describe("portal navigation state", () => {
  it("keeps Início active while the client follows a deal", () => {
    expect(isPortalNavigationItemActive("/portal/deals/deal-1", item("/portal"))).toBe(true);
  });

  it("keeps Imóveis active in search and property details", () => {
    expect(isPortalNavigationItemActive("/portal/buscar", item("/portal/buscar"))).toBe(true);
    expect(isPortalNavigationItemActive("/portal/imoveis/property-1", item("/portal/buscar"))).toBe(true);
  });

  it("does not mark Início active for unrelated portal pages", () => {
    expect(isPortalNavigationItemActive("/portal/mensagens", item("/portal"))).toBe(false);
  });
});

describe("navegação por persona", () => {
  it("quem procura imóvel vai para a busca", () => {
    const items = portalNavItems(resolvePortalPersona({ role: "buyer", dealType: "SALE" }));
    expect(items.find((entry) => entry.label === "Imóveis")?.path).toBe("/portal/buscar");
  });

  it("quem anuncia vai para a própria carteira", () => {
    const items = portalNavItems(resolvePortalPersona({ role: "owner", dealType: "RENT" }));
    expect(items.find((entry) => entry.label === "Meus imóveis")?.path).toBe("/portal/imoveis");
  });

  it("a aba de imóveis continua ativa na busca e no detalhe, em qualquer persona", () => {
    const landlordItems = portalNavItems(resolvePortalPersona({ role: "owner", dealType: "SALE" }));
    const tab = landlordItems.find((entry) => entry.label === "Meus imóveis")!;
    expect(isPortalNavigationItemActive("/portal/buscar", tab)).toBe(true);
    expect(isPortalNavigationItemActive("/portal/imoveis/property-1", tab)).toBe(true);
  });
});
