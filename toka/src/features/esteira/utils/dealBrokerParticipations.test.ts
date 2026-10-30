import { describe, expect, it } from "vitest";

import { buildDealBrokerParticipations } from "./dealBrokerParticipations";

// Regras do tenant do caso real (Comissões por papel): 20 / 0 / 0 / 0 / 40 / 40.
const TENANT_RULES = { captador: 20, co_captador: 0, visitador: 0, atendente: 0, fechamento: 40, imobiliaria: 40 };
// Padrão de fábrica: 30 / 0 / 20 / 0 / 50 / 0.
const DEFAULT_RULES = { captador: 30, co_captador: 0, visitador: 20, atendente: 0, fechamento: 50, imobiliaria: 0 };

const JAQUELINE = "28056d8a-ce8f-497e-8915-a16377b3f1a3";

describe("buildDealBrokerParticipations", () => {
  it("distribui a comissão da VENDA pelas regras do tenant em vez de dar 100% ao captador", () => {
    const { participations, warning } = buildDealBrokerParticipations({
      dealType: "SALE",
      capturingBrokers: [{ id: JAQUELINE, role: "captador" }],
      closingBrokerId: JAQUELINE,
      responsibleBrokerId: JAQUELINE,
      rolePercentages: TENANT_RULES,
    });

    expect(warning).toBeUndefined();
    expect(participations).toEqual([
      { broker_id: JAQUELINE, role: "captador", percentage: 20 },
      { broker_id: JAQUELINE, role: "fechamento", percentage: 40 },
      { broker_id: "", role: "imobiliaria", percentage: 40 },
    ]);
    expect(participations.reduce((sum, row) => sum + row.percentage, 0)).toBe(100);
  });

  it("não deixa percentual de papel sem corretor ir para outro corretor: fica com a imobiliária", () => {
    const { participations } = buildDealBrokerParticipations({
      dealType: "SALE",
      capturingBrokers: [{ id: JAQUELINE, role: "captador" }],
      closingBrokerId: JAQUELINE,
      rolePercentages: DEFAULT_RULES,
    });

    // captador 30 + fechamento 50; os 20% de visitador (sem corretor) viram
    // retenção da imobiliária em vez de inflar o pagamento de alguém.
    expect(participations).toEqual([
      { broker_id: JAQUELINE, role: "captador", percentage: 30 },
      { broker_id: JAQUELINE, role: "fechamento", percentage: 50 },
      { broker_id: "", role: "imobiliaria", percentage: 20 },
    ]);
  });

  it("divide o percentual do papel entre os corretores do mesmo papel", () => {
    const { participations } = buildDealBrokerParticipations({
      dealType: "SALE",
      capturingBrokers: [
        { id: JAQUELINE, role: "captador" },
        { id: "outro-corretor", role: "captador" },
      ],
      closingBrokerId: JAQUELINE,
      rolePercentages: TENANT_RULES,
    });

    expect(participations).toEqual([
      { broker_id: JAQUELINE, role: "captador", percentage: 10 },
      { broker_id: "outro-corretor", role: "captador", percentage: 10 },
      { broker_id: JAQUELINE, role: "fechamento", percentage: 40 },
      { broker_id: "", role: "imobiliaria", percentage: 40 },
    ]);
  });

  it("sem configuração de papéis, retém a comissão na imobiliária em vez de nascer sem rateio", () => {
    const { participations, warning } = buildDealBrokerParticipations({
      dealType: "SALE",
      capturingBrokers: [{ id: JAQUELINE, role: "captador" }],
      closingBrokerId: JAQUELINE,
      rolePercentages: {},
    });

    expect(participations).toEqual([{ broker_id: "", role: "imobiliaria", percentage: 100 }]);
    expect(warning).toMatch(/Configurações › Comissões/);
  });

  it("usa o responsável pelo negócio como fechamento quando o lead não tem corretor", () => {
    const { participations } = buildDealBrokerParticipations({
      dealType: "SALE",
      capturingBrokers: [{ id: JAQUELINE, role: "captador" }],
      responsibleBrokerId: "responsavel",
      rolePercentages: TENANT_RULES,
    });

    expect(participations).toContainEqual({ broker_id: "responsavel", role: "fechamento", percentage: 40 });
  });

  it("segura o que ninguém ocupa como retenção quando não há corretor algum", () => {
    const { participations } = buildDealBrokerParticipations({
      dealType: "SALE",
      capturingBrokers: [],
      rolePercentages: TENANT_RULES,
    });

    // Nenhum corretor pago; a comissão fica retida na imobiliária e o operador
    // ajusta o rateio na esteira antes de assinar.
    expect(participations).toEqual([{ broker_id: "", role: "imobiliaria", percentage: 100 }]);
  });

  it("com soma de papéis acima de 100%, retém tudo na imobiliária e avisa", () => {
    const { participations, warning } = buildDealBrokerParticipations({
      dealType: "SALE",
      capturingBrokers: [{ id: JAQUELINE, role: "captador" }],
      closingBrokerId: JAQUELINE,
      rolePercentages: { ...TENANT_RULES, captador: 80, fechamento: 60 },
    });

    expect(participations).toEqual([{ broker_id: "", role: "imobiliaria", percentage: 100 }]);
    expect(warning).toMatch(/passa de 100%/);
  });

  it("mantém o rateio da LOCAÇÃO entre os captadores do imóvel (sem imobiliária)", () => {
    const { participations } = buildDealBrokerParticipations({
      dealType: "RENT",
      capturingBrokers: [{ id: JAQUELINE, role: "captador", commissionPct: 0 }],
      rolePercentages: TENANT_RULES,
    });

    expect(participations).toEqual([{ broker_id: JAQUELINE, role: "captador", percentage: 100 }]);
  });

  it("na locação com vários captadores, rateia proporcionalmente ao percentual informado", () => {
    const { participations } = buildDealBrokerParticipations({
      dealType: "RENT",
      capturingBrokers: [
        { id: "a", role: "captador", commissionPct: 30 },
        { id: "b", role: "captador", commissionPct: 10 },
      ],
      rolePercentages: TENANT_RULES,
    });

    expect(participations).toEqual([
      { broker_id: "a", role: "captador", percentage: 75 },
      { broker_id: "b", role: "captador", percentage: 25 },
    ]);
  });

  it("locação sem captador cadastrado também nasce com rateio (retenção da imobiliária)", () => {
    const { participations, warning } = buildDealBrokerParticipations({
      dealType: "RENT",
      capturingBrokers: [],
      rolePercentages: TENANT_RULES,
    });

    expect(participations).toEqual([{ broker_id: "", role: "imobiliaria", percentage: 100 }]);
    expect(warning).toMatch(/captação/i);
  });

  it("fecha a soma em 100% quando o rateio não divide exato", () => {
    const { participations } = buildDealBrokerParticipations({
      dealType: "RENT",
      capturingBrokers: [
        { id: "a", role: "captador" },
        { id: "b", role: "captador" },
        { id: "c", role: "captador" },
      ],
      rolePercentages: TENANT_RULES,
    });

    expect(participations.reduce((sum, row) => sum + row.percentage, 0)).toBe(100);
  });
});
