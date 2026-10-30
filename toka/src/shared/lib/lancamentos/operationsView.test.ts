import { describe, expect, it } from "vitest";

import type { EconomicOperation } from "./presentation";
import type { Lancamento, LancamentoKind, LancamentoStatus } from "./types";
import {
  collectMonthOptions,
  countQuickFilters,
  currentMonthKey,
  groupOperations,
  monthKeyOf,
  monthLabel,
  sortOperations,
  summarizeOperation,
  summarizeOperations,
} from "./operationsView";

const NOW = new Date("2026-09-18T12:00:00");

function makeItem(overrides: Partial<Lancamento> & { id: string; kind: LancamentoKind }): Lancamento {
  const { id, kind, ...rest } = overrides;
  const base = {
    id,
    kind,
    status: "pendente" as LancamentoStatus,
    amount: 1000,
    currency: "BRL",
    counterparty: { kind: "tenant", label: "Inquilino" },
    origin: { entity_kind: "contract", entity_id: "ct-1", label: "Contrato CTR-1" },
    timeline_summary: {},
    permissions_required: ["finance:read"],
    created_at: "2026-09-05T10:00:00Z",
    updated_at: "2026-09-05T10:00:00Z",
    source: {},
    kind_data: { direction: "entrada" },
  } as unknown as Lancamento;
  return { ...base, ...rest } as Lancamento;
}

function makeOperation(overrides: Partial<EconomicOperation> & { key: string; items: Lancamento[] }): EconomicOperation {
  const { key, items, ...rest } = overrides;
  return {
    key,
    title: `Operação ${key}`,
    items,
    primary: items[0],
    latestAt: items[0]?.updated_at ?? "2026-09-05T10:00:00Z",
    amount: items.reduce((sum, item) => sum + item.amount, 0),
    openCount: items.filter((item) => item.status !== "pago").length,
    divergenceCount: 0,
    ...rest,
  };
}

const chargeSeptember = makeItem({
  id: "charge-1",
  kind: "charge",
  amount: 2500,
  due_at: "2026-09-10T00:00:00Z",
  status: "vencido",
  presentation: {
    operation_key: "contract:ct-1:rent:2026-09",
    operation_title: "Locação · Apartamento dos Ipês",
    contract_id: "ct-1",
    property_title: "Apartamento dos Ipês",
    reference_month: "2026-09",
    flow: "rent",
  },
});

const commissionSeptember = makeItem({
  id: "commission-1",
  kind: "commission",
  amount: 800,
  status: "pendente",
  counterparty: { kind: "broker", label: "Tiago Santini" },
  presentation: {
    operation_key: "contract:ct-1:rent:2026-09",
    operation_title: "Locação · Apartamento dos Ipês",
    contract_id: "ct-1",
    reference_month: "2026-09",
    flow: "rent",
  },
});

const operationSeptember = makeOperation({
  key: "contract:ct-1:rent:2026-09",
  title: "Locação · Apartamento dos Ipês",
  items: [chargeSeptember, commissionSeptember],
});

const chargeOctober = makeItem({
  id: "charge-2",
  kind: "charge",
  amount: 2500,
  due_at: "2026-10-10T00:00:00Z",
  presentation: {
    operation_key: "contract:ct-1:rent:2026-10",
    operation_title: "Locação · Outubro",
    contract_id: "ct-1",
    reference_month: "2026-10",
    flow: "rent",
  },
});

const operationOctober = makeOperation({
  key: "contract:ct-1:rent:2026-10",
  title: "Locação · Outubro",
  items: [chargeOctober],
});

const transferAugust = makeItem({
  id: "transfer-1",
  kind: "transfer",
  amount: 1200,
  status: "pendente",
  counterparty: { kind: "owner", label: "Proprietário João" },
  presentation: {
    operation_key: "contract:ct-1:rent:2026-08",
    operation_title: "Repasse · Agosto",
    contract_id: "ct-1",
    reference_month: "2026-08",
    flow: "rent",
  },
});

const operationAugust = makeOperation({ key: "contract:ct-1:rent:2026-08", title: "Repasse · Agosto", items: [transferAugust] });

const manualSeptember = makeItem({
  id: "tx-1",
  kind: "transaction",
  amount: 300,
  kind_data: { direction: "saida", transaction_type: "despesa" } as never,
  counterparty: { kind: "external", label: "Fornecedor" },
});
const operationManualSeptember = makeOperation({ key: "tx:1", title: "Despesa avulsa", items: [manualSeptember] });

// Sem nenhuma data: cai no balde "Sem competência" da visão mensal.
const undated = {
  ...makeItem({ id: "tx-2", kind: "transaction", amount: 150, kind_data: { direction: "saida" } as never }),
  created_at: "",
  updated_at: "",
  due_at: undefined,
  settled_at: undefined,
} as Lancamento;
const operationUndated = makeOperation({ key: "tx:2", title: "Lançamento sem data", items: [undated] });

describe("chaves de mês", () => {
  it("deriva a competência da data e rotula em pt-BR", () => {
    expect(monthKeyOf("2026-09-10T00:00:00Z")).toBe("2026-09");
    expect(monthKeyOf(undefined)).toBeUndefined();
    expect(monthLabel("2026-09")).toContain("2026");
    expect(monthLabel("sem-data")).toBe("Sem competência");
  });

  it("usa o mês do relógio como mês corrente", () => {
    expect(currentMonthKey(NOW)).toBe("2026-09");
  });
});

describe("summarizeOperation", () => {
  it("separa a receber de a pagar sem somar linhas vinculadas", () => {
    const summary = summarizeOperation(operationSeptember);

    expect(summary.receivable).toBe(2500);
    expect(summary.payable).toBe(800);
    expect(summary.totalLines).toBe(2);
    expect(summary.openLines).toBe(2);
  });

  it("usa a competência predominante e o vencimento em aberto mais próximo", () => {
    const summary = summarizeOperation(operationSeptember);

    expect(summary.competence).toBe("2026-09");
    expect(summary.dueAt).toBe("2026-09-10T00:00:00Z");
    expect(summary.dueMonth).toBe("2026-09");
    expect(summary.overdue).toBe(true);
  });

  it("marca como concluída a operação sem linha em aberto", () => {
    const paid = makeOperation({
      key: "pago",
      items: [makeItem({ id: "c", kind: "charge", status: "pago", amount: 900 })],
    });
    const summary = summarizeOperation(paid);

    expect(summary.closed).toBe(true);
    expect(summary.openLines).toBe(0);
    expect(summary.receivable).toBe(0);
    expect(summary.payable).toBe(0);
  });

  it("trata saída de caixa como obrigação a pagar", () => {
    const summary = summarizeOperation(operationManualSeptember);

    expect(summary.payable).toBe(300);
    expect(summary.receivable).toBe(0);
  });

  it("marca como sem competência a operação sem nenhuma data", () => {
    expect(summarizeOperation(operationUndated).competence).toBe("sem-data");
  });
});

describe("sortOperations", () => {
  const operations = [operationOctober, operationManualSeptember, operationUndated, operationAugust, operationSeptember];

  it("prioriza atraso, mês corrente, passado em aberto e só então o futuro", () => {
    const ordered = sortOperations(operations, "focus", NOW).map((operation) => operation.key);

    expect(ordered).toEqual([
      "contract:ct-1:rent:2026-09", // vencido: prioridade máxima
      "tx:1", // competência do mês corrente (data-chave de setembro)
      "contract:ct-1:rent:2026-08", // competência passada ainda em aberto
      "contract:ct-1:rent:2026-10", // ainda vai vencer
      "tx:2", // sem data
    ]);
  });

  it("ordena por vencimento deixando o sem data no fim", () => {
    const ordered = sortOperations(operations, "due", NOW).map((operation) => operation.key);

    expect(ordered[0]).toBe("contract:ct-1:rent:2026-09");
    // Sem vencimento em aberto: os dois lançamentos de caixa ficam no fim.
    expect(ordered.slice(-2).sort()).toEqual(["tx:1", "tx:2"]);
  });

  it("ordena por maior valor em aberto", () => {
    const ordered = sortOperations(operations, "amount", NOW);

    expect(ordered[0].key).toBe("contract:ct-1:rent:2026-09");
  });
});

describe("groupOperations", () => {
  const operations = [operationOctober, operationManualSeptember, operationUndated, operationAugust, operationSeptember];

  it("agrupa por competência com o mês corrente primeiro e o sem data por último", () => {
    const sections = groupOperations(operations, "competence", "focus", NOW);

    expect(sections.map((section) => section.key)).toEqual(["2026-09", "2026-08", "2026-10", "sem-data"]);
    expect(sections[0].hint).toBe("Mês corrente");
    expect(sections[0].count).toBe(2);
    expect(sections[0].receivable).toBe(2500);
    expect(sections[0].payable).toBe(1100);
    expect(sections[0].overdueCount).toBe(1);
  });

  it("agrupa por vencimento separando o que não tem data em aberto", () => {
    const sections = groupOperations(operations, "due", "focus", NOW);

    expect(sections[0].key).toBe("2026-09");
    expect(sections[sections.length - 1].label).toBe("Sem vencimento em aberto");
  });

  it("agrupa por situação na ordem atraso, a vencer e concluídas", () => {
    const sections = groupOperations(operations, "status", "focus", NOW);

    expect(sections.map((section) => section.label)).toEqual(["Em atraso", "A vencer"]);
  });

  it("agrupa por origem usando o contrato do evento econômico", () => {
    const sections = groupOperations(operations, "origin", "focus", NOW);
    const contrato = sections.find((section) => section.key === "contract:ct-1");

    expect(contrato?.label).toBe("Apartamento dos Ipês");
    expect(contrato?.count).toBe(3);
  });

  it("sem agrupamento devolve uma seção única com o total", () => {
    const sections = groupOperations(operations, "none", "focus", NOW);

    expect(sections).toHaveLength(1);
    expect(sections[0].count).toBe(5);
    expect(sections[0].receivable).toBe(5000);
  });
});

describe("atalhos contextuais", () => {
  const operations = [operationSeptember, operationOctober, operationManualSeptember];

  it("conta cada atalho pelo estado real das operações", () => {
    expect(countQuickFilters(operations)).toEqual({ overdue: 1, receivable: 2, payable: 2, divergent: 0 });
  });

  it("soma o recorte inteiro", () => {
    expect(summarizeOperations(operations)).toEqual({
      count: 3,
      lines: 4,
      receivable: 5000,
      payable: 1100,
      overdueCount: 1,
    });
  });

  it("lista os meses disponíveis em ordem decrescente, sem os sem-data", () => {
    expect(collectMonthOptions(operations)).toEqual({ competence: ["2026-10", "2026-09"], due: ["2026-10", "2026-09"] });
  });
});
