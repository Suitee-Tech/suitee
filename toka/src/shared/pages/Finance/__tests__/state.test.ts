import { describe, it, expect } from "vitest";
import { initialFinanceFilters, lancamentoDraftReducer } from "../state";
import { initialLancamentoDraft, type LancamentoDraft } from "../schema";

describe("lancamentoDraftReducer", () => {
  const defaultPct = 6;

  describe("SET_COMMISSION_LOADING", () => {
    it("deve ativar o estado de carregamento e limpar erro anterior", () => {
      const state: LancamentoDraft = {
        ...initialLancamentoDraft(defaultPct),
        commissionDataError: "erro anterior",
      };
      const next = lancamentoDraftReducer(state, {
        type: "SET_COMMISSION_LOADING",
        loading: true,
      });
      expect(next.loadingCommission).toBe(true);
      expect(next.commissionDataError).toBeUndefined();
    });

    it("deve desativar o estado de carregamento mantendo erro existente", () => {
      const state: LancamentoDraft = {
        ...initialLancamentoDraft(defaultPct),
        loadingCommission: true,
        commissionDataError: "erro existente",
      };
      const next = lancamentoDraftReducer(state, {
        type: "SET_COMMISSION_LOADING",
        loading: false,
      });
      expect(next.loadingCommission).toBe(false);
      expect(next.commissionDataError).toBe("erro existente");
    });
  });

  describe("SET_COMMISSION_DATA", () => {
    it("deve definir participantes e limpar estado de carregamento/erro", () => {
      const state: LancamentoDraft = {
        ...initialLancamentoDraft(defaultPct),
        loadingCommission: true,
        commissionDataError: "erro",
      };
      const participants = [
        { id: 1, role: "captador", name: "João", percent: 30, brokerId: "b1" },
        { id: 2, role: "fechamento", name: "Maria", percent: 20, brokerId: "b2" },
      ];
      const next = lancamentoDraftReducer(state, {
        type: "SET_COMMISSION_DATA",
        participants,
        baseValue: 500000,
        commissionPercent: 6,
      });
      expect(next.loadingCommission).toBe(false);
      expect(next.commissionDataLoaded).toBe(true);
      expect(next.commissionDataError).toBeUndefined();
      expect(next.participants).toEqual(participants);
      expect(next.baseValue).toBe(500000);
      expect(next.commissionPercent).toBe(6);
    });

    it("deve lidar com lista vazia de participantes", () => {
      const state: LancamentoDraft = {
        ...initialLancamentoDraft(defaultPct),
        loadingCommission: true,
      };
      const next = lancamentoDraftReducer(state, {
        type: "SET_COMMISSION_DATA",
        participants: [],
        baseValue: 1000,
        commissionPercent: 10,
      });
      expect(next.participants).toEqual([]);
      expect(next.commissionDataLoaded).toBe(true);
      expect(next.loadingCommission).toBe(false);
    });

    it("deve suportar múltiplos tipos de comissionamento simultâneos", () => {
      const state = initialLancamentoDraft(defaultPct);
      const participants = [
        { id: 1, role: "captador", name: "João", percent: 30, brokerId: "b1", amount: 15000, commissionType: "captador" },
        { id: 2, role: "visitador", name: "Ana", percent: 20, brokerId: "b3", amount: 10000, commissionType: "visitador" },
        { id: 3, role: "fechamento", name: "Maria", percent: 50, brokerId: "b2", amount: 25000, commissionType: "fechamento" },
        { id: 4, role: "royalty", name: "Parceiro XPTO", percent: 10, brokerId: "b4", amount: 5000, commissionType: "royalty" },
      ];
      const next = lancamentoDraftReducer(state, {
        type: "SET_COMMISSION_DATA",
        participants,
        baseValue: 500000,
        commissionPercent: 11,
      });
      expect(next.participants.length).toBe(4);
      expect(next.participants.map((p) => p.role)).toEqual([
        "captador",
        "visitador",
        "fechamento",
        "royalty",
      ]);
    });
  });

  describe("SET_COMMISSION_ERROR", () => {
    it("deve definir mensagem de erro e limpar carregamento", () => {
      const state: LancamentoDraft = {
        ...initialLancamentoDraft(defaultPct),
        loadingCommission: true,
      };
      const next = lancamentoDraftReducer(state, {
        type: "SET_COMMISSION_ERROR",
        error: "Falha na API",
      });
      expect(next.loadingCommission).toBe(false);
      expect(next.commissionDataError).toBe("Falha na API");
      expect(next.commissionDataLoaded).toBe(false);
    });

    it("deve sobrescrever erro anterior", () => {
      const state: LancamentoDraft = {
        ...initialLancamentoDraft(defaultPct),
        commissionDataError: "Erro antigo",
      };
      const next = lancamentoDraftReducer(state, {
        type: "SET_COMMISSION_ERROR",
        error: "Erro novo",
      });
      expect(next.commissionDataError).toBe("Erro novo");
    });
  });

  it("permite selecionar o corretor nas linhas automáticas", () => {
    const state: LancamentoDraft = {
      ...initialLancamentoDraft(defaultPct),
      participants: [{ id: 1, role: "captador", name: "", percent: 30 }],
    };

    const next = lancamentoDraftReducer(state, {
      type: "UPDATE_PARTICIPANT_BROKER",
      id: 1,
      brokerId: "broker-1",
      name: "João",
    });

    expect(next.participants[0]).toMatchObject({
      brokerId: "broker-1",
      name: "João",
      role: "captador",
      percent: 30,
    });
  });

  it("permite definir o papel financeiro de uma parte adicionada", () => {
    const state: LancamentoDraft = {
      ...initialLancamentoDraft(defaultPct),
      participants: [{ id: 1, role: "fechamento", name: "", percent: 20 }],
    };

    const next = lancamentoDraftReducer(state, {
      type: "UPDATE_PARTICIPANT_ROLE",
      id: 1,
      role: "visitador",
    });

    expect(next.participants[0]?.role).toBe("visitador");
  });

  describe("transições de estado completas", () => {
    it("loading -> data -> sucesso (fluxo feliz)", () => {
      const state = initialLancamentoDraft(defaultPct);

      const s1 = lancamentoDraftReducer(state, {
        type: "SET_COMMISSION_LOADING",
        loading: true,
      });
      expect(s1.loadingCommission).toBe(true);

      const s2 = lancamentoDraftReducer(s1, {
        type: "SET_COMMISSION_DATA",
        participants: [{ id: 1, role: "captador", name: "João", percent: 100 }],
        baseValue: 1000,
        commissionPercent: 6,
      });
      expect(s2.loadingCommission).toBe(false);
      expect(s2.commissionDataLoaded).toBe(true);
      expect(s2.commissionDataError).toBeUndefined();
      expect(s2.participants.length).toBe(1);
    });

    it("loading -> erro (fluxo de falha)", () => {
      const state = initialLancamentoDraft(defaultPct);

      const s1 = lancamentoDraftReducer(state, {
        type: "SET_COMMISSION_LOADING",
        loading: true,
      });
      expect(s1.loadingCommission).toBe(true);

      const s2 = lancamentoDraftReducer(s1, {
        type: "SET_COMMISSION_ERROR",
        error: "Contrato sem dados",
      });
      expect(s2.loadingCommission).toBe(false);
      expect(s2.commissionDataLoaded).toBe(false);
      expect(s2.commissionDataError).toBe("Contrato sem dados");
    });
  });

  describe("SET_CONTRACT com commission reset", () => {
    it("deve limpar participantes ao limpar contrato", () => {
      const state: LancamentoDraft = {
        ...initialLancamentoDraft(defaultPct),
        selectedContractId: "c1",
        participants: [{ id: 1, role: "captador", name: "João", percent: 30 }],
      };
      const next = lancamentoDraftReducer(state, {
        type: "SET_CONTRACT",
        contractId: "",
      });
      expect(next.selectedContractId).toBe("");
      expect(next.participants).toEqual([]);
      expect(next.baseValue).toBe(0);
    });
  });
});

describe("initialFinanceFilters", () => {
  it("abre o Financeiro pela visão geral", () => {
    expect(initialFinanceFilters.module).toBe("overview");
  });
});
