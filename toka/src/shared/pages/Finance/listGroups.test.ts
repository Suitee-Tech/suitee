import { describe, expect, it } from "vitest";
import { financeListBucket, groupFinanceList } from "./listGroups";

describe("financeListBucket", () => {
  it("classifica status nos três baldes de urgência", () => {
    expect(financeListBucket("Em Atraso")).toBe("overdue");
    expect(financeListBucket("vencido")).toBe("overdue");
    expect(financeListBucket("Pendente")).toBe("upcoming");
    expect(financeListBucket("Parcial")).toBe("upcoming");
    expect(financeListBucket("aprovado")).toBe("upcoming");
    expect(financeListBucket("Pago")).toBe("done");
    expect(financeListBucket("processado")).toBe("done");
    expect(financeListBucket("Liquidado sem saldo")).toBe("done");
    expect(financeListBucket("Cancelada")).toBe("done");
  });
});

describe("groupFinanceList", () => {
  it("agrupa na ordem Em atraso → A vencer → Concluídas, descartando grupos vazios", () => {
    const items = [
      { id: "a", status: "Pago" },
      { id: "b", status: "Em Atraso" },
      { id: "c", status: "Pendente" },
      { id: "d", status: "Cancelada" },
    ];
    const groups = groupFinanceList(items, (i) => i.status);
    expect(groups.map((g) => g.key)).toEqual(["overdue", "upcoming", "done"]);
    expect(groups[0].items.map((i) => i.id)).toEqual(["b"]);
    expect(groups[1].items.map((i) => i.id)).toEqual(["c"]);
    expect(groups[2].items.map((i) => i.id)).toEqual(["a", "d"]);
  });

  it("retorna apenas grupos com itens", () => {
    const groups = groupFinanceList([{ id: "x", status: "Pendente" }], (i) => i.status);
    expect(groups.map((g) => g.key)).toEqual(["upcoming"]);
    expect(groups[0].label).toBe("A vencer");
  });
});
