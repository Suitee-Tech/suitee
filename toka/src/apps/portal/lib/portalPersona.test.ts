import { describe, expect, it } from "vitest";

import {
  portalCopy,
  portalRoleLabel,
  rentalRoleLabel,
  resolvePortalPersona,
} from "./portalPersona";

const persona = (role: string | null, dealType: string | null) =>
  resolvePortalPersona({ role, dealType });

describe("persona do cliente no portal", () => {
  it("comprador: negócio de venda e papel buyer", () => {
    const p = persona("buyer", "SALE");
    expect(p.actor).toBe("buyer");
    expect(p.label).toBe("Comprador");
    expect(p.isSeeker).toBe(true);
    expect(p.isOwnerSide).toBe(false);
    expect(p.isUnidentified).toBe(false);
    expect(p.dealKindLabel).toBe("Compra e venda");
  });

  it("locatário: negócio de locação e papel tenant", () => {
    const p = persona("tenant", "RENT");
    expect(p.actor).toBe("tenant");
    expect(p.label).toBe("Locatário");
    expect(p.isSeeker).toBe(true);
    expect(p.isRent).toBe(true);
  });

  it("vendedor e locador saem do MESMO papel (owner) cruzado com o negócio", () => {
    const seller = persona("owner", "SALE");
    expect(seller.actor).toBe("seller");
    expect(seller.label).toBe("Vendedor");
    expect(seller.isOwnerSide).toBe(true);
    expect(seller.isSeeker).toBe(false);

    const landlord = persona("owner", "RENT");
    expect(landlord.actor).toBe("landlord");
    expect(landlord.label).toBe("Locador");
    expect(landlord.isOwnerSide).toBe(true);
  });

  it("owner sem tipo de negócio não vira vendedor nem locador", () => {
    const p = persona("owner", null);
    expect(p.actor).toBe("owner");
    expect(p.label).toBe("Proprietário");
    expect(p.isOwnerSide).toBe(true);
    expect(p.dealKindLabel).toBeNull();
  });

  it("fiador e testemunha são partes sem lado", () => {
    for (const role of ["guarantor", "witness"]) {
      const p = persona(role, "RENT");
      expect(p.isSeeker).toBe(false);
      expect(p.isOwnerSide).toBe(false);
      expect(p.isUnidentified).toBe(false);
    }
    expect(persona("guarantor", "RENT").label).toBe("Fiador(a)");
  });

  it("papel ausente é parte não identificada (texto neutro)", () => {
    const p = persona(null, null);
    expect(p.actor).toBe("party");
    expect(p.isUnidentified).toBe(true);
    expect(p.isSeeker).toBe(false);
    expect(p.isOwnerSide).toBe(false);
    expect(p.label).toBe("Parte do negócio");
  });

  it("normaliza o tipo do negócio em qualquer caixa", () => {
    expect(persona("owner", "rent").actor).toBe("landlord");
    expect(persona("owner", " sale ").actor).toBe("seller");
    expect(persona("owner", "PERMUTA").actor).toBe("owner");
  });

  it("rótulo de papel desconhecido não vira string vazia", () => {
    expect(portalRoleLabel("attorney")).toBe("Procurador(a)");
    expect(portalRoleLabel("advogado")).toBe("advogado");
    expect(portalRoleLabel("")).toBe("Parte do negócio");
  });
});

describe("UX Writer por persona", () => {
  const copy = (role: string | null, dealType: string | null) => portalCopy(persona(role, dealType));

  it("o lado de quem procura recebe fluxo de busca e visita", () => {
    for (const [role, type] of [["buyer", "SALE"], ["tenant", "RENT"]] as const) {
      const c = copy(role, type);
      expect(c.propertyVisitCta).toBe("Agendar visita");
      expect(c.propertyVisitOwnerHint).toBeNull();
      expect(c.recommendationsTitle).toBe("Selecionados para você");
      expect(c.journeyPrimaryCta.label).toBe("Explorar imóveis");
      expect(c.quickActions.map((a) => a.label)).toContain("Explorar imóveis");
    }
  });

  it("quem anuncia o imóvel NÃO recebe fluxo de procurar nem de agendar visita", () => {
    for (const [role, type] of [["owner", "SALE"], ["owner", "RENT"], ["owner", null]] as const) {
      const c = copy(role, type);
      expect(c.propertyVisitCta).toBeNull();
      expect(c.propertyVisitOwnerHint).toMatch(/imobiliária/);
      expect(c.recommendationsTitle).toBeNull();
      expect(c.journeyPrimaryCta.label).toBe("Falar com meu corretor");
      const labels = c.quickActions.map((a) => a.label);
      expect(labels).not.toContain("Explorar imóveis");
      expect(labels).not.toContain("Selecionados para você");
    }
  });

  it("financeiro muda de nome por lado: boletos, repasses e recebimentos", () => {
    expect(copy("tenant", "RENT").financeEntryLabel).toBe("Meus boletos");
    expect(copy("owner", "RENT").financeEntryLabel).toBe("Meus repasses");
    expect(copy("owner", "SALE").financeEntryLabel).toBe("Meus recebimentos");
    expect(copy(null, null).financeEntryLabel).toBe("Financeiro");
  });

  it("a etapa da esteira é contada do ponto de vista de quem lê", () => {
    expect(copy("buyer", "SALE").stageHeadline.PROPOSAL).toBe("Sua proposta está pronta para revisão.");
    expect(copy("owner", "SALE").stageHeadline.PROPOSAL).toBe("Você recebeu uma proposta para o seu imóvel.");
    expect(copy(null, null).stageHeadline.PROPOSAL).toBe("A proposta do negócio está em revisão.");
  });

  it("a ação principal da etapa não pede para o dono 'ver a proposta dele'", () => {
    expect(copy("buyer", "SALE").primaryAction.PROPOSAL).toBe("Ver proposta");
    expect(copy("owner", "SALE").primaryAction.PROPOSAL).toBe("Ver proposta recebida");
    expect(copy("owner", "RENT").primaryAction.CONVERTED).toBe("Ver contrato de locação");
    expect(copy("buyer", "SALE").primaryAction.CONVERTED).toBe("Ver contrato");
  });

  it("o cabeçalho diferencia quem procura de quem é dono do imóvel", () => {
    expect(copy("buyer", "SALE").overviewEyebrow).toBe("Sua compra");
    expect(copy("tenant", "RENT").overviewEyebrow).toBe("Sua locação");
    expect(copy("owner", "SALE").overviewEyebrow).toBe("Venda do seu imóvel");
    expect(copy("owner", "RENT").overviewEyebrow).toBe("Locação do seu imóvel");
    expect(copy(null, null).overviewEyebrow).toBe("Seu negócio");
  });

  it("locatário e locador falam de locação; comprador e vendedor, de compra", () => {
    expect(copy("tenant", "RENT").journeyEyebrow).toBe("Sua jornada de locação");
    expect(copy("buyer", "SALE").journeyEyebrow).toBe("Sua jornada de compra");
    expect(copy("owner", "RENT").journeyEyebrow).toBe("Acompanhamento da locação");
    expect(copy("owner", "SALE").journeyEyebrow).toBe("Acompanhamento da venda");
  });

  it("lado do dono com locação ganha locações e manutenção; com venda, não", () => {
    const landlord = copy("owner", "RENT").quickActions.map((a) => a.key);
    const seller = copy("owner", "SALE").quickActions.map((a) => a.key);
    expect(landlord).toContain("rentals");
    expect(landlord).toContain("tickets");
    expect(seller).not.toContain("rentals");
    expect(seller).not.toContain("tickets");
  });

  it("a navegação de imóveis aponta para onde a persona realmente tem imóvel", () => {
    expect(copy("buyer", "SALE").propertiesNavPath).toBe("/portal/buscar");
    expect(copy("tenant", "RENT").propertiesNavPath).toBe("/portal/buscar");
    expect(copy("owner", "SALE").propertiesNavPath).toBe("/portal/imoveis");
    expect(copy("owner", "SALE").propertiesNavLabel).toBe("Meus imóveis");
  });

  it("papel não identificado não afirma lado nenhum", () => {
    const c = copy(null, null);
    expect(c.recommendationsTitle).toBe("Sugestões do seu corretor");
    expect(c.quickActions.map((a) => a.label)).not.toContain("Explorar imóveis");
    expect(c.propertyVisitOwnerHint).toBeNull();
    expect(c.stageHeadline.CONVERTED).toBe("O negócio foi concluído.");
  });

  it("só quem procura agenda visita; o dono acompanha as visitas da imobiliária", () => {
    expect(copy("buyer", "SALE").visits.canSchedule).toBe(true);
    expect(copy("tenant", "RENT").visits.canSchedule).toBe(true);

    for (const [role, type] of [["owner", "SALE"], ["owner", "RENT"], ["owner", null]] as const) {
      const visits = copy(role, type).visits;
      expect(visits.canSchedule).toBe(false);
      expect(visits.notScheduled.label).toBe("Acompanhar visitas");
      expect(visits.notScheduled.description).toMatch(/imobiliária/);
      expect(visits.scheduled.description).toMatch(/imobiliária/);
      expect(visits.sectionIntro).toMatch(/imobiliária/);
    }
  });
});

describe("vocabulário das locações financeiras", () => {
  it("usa locatário/locador, não inquilino/proprietário", () => {
    expect(rentalRoleLabel("tenant")).toBe("Você é o locatário");
    expect(rentalRoleLabel("owner")).toBe("Você é o locador");
    expect(rentalRoleLabel("both")).toBe("Você é locatário e locador");
    expect(rentalRoleLabel("")).toBe("Locação vinculada à sua conta");
  });
});
