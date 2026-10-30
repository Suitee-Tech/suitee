import { describe, it, expect } from "vitest";
import type { Finding } from "../../services/inspection.service";
import { diffInspections, rankOf, summarize, groupByRoom } from "./inspectionDiff";

// Helper para construir Finding nos testes. Reduz ruído quando os
// testes só precisam de room+description+condition (campos relevantes
// para o pairing).
function f(room: string, description: string, condition: string = "", overrides: Partial<Finding> = {}): Finding {
  return {
    id: `${room}-${description}`,
    room,
    description,
    condition,
    notes: "",
    photos: [],
    ...overrides,
  };
}

describe("rankOf", () => {
  it("orders conditions Ótimo > Bom > Regular > Ruim > Danificado > empty", () => {
    expect(rankOf("Ótimo")).toBe(5);
    expect(rankOf("Bom")).toBe(4);
    expect(rankOf("Regular")).toBe(3);
    expect(rankOf("Ruim")).toBe(2);
    expect(rankOf("Danificado")).toBe(1);
    expect(rankOf("")).toBe(0);
    expect(rankOf(undefined)).toBe(0);
  });
  it("returns 0 for unknown conditions (defensive)", () => {
    expect(rankOf("alien")).toBe(0);
  });
});

describe("diffInspections — pairing", () => {
  it("matches by (room, description) case-insensitive + trimmed", () => {
    const entry = [f("Sala", "Piso", "Ótimo")];
    const exit = [f("sala", " Piso ", "Bom")];
    const pairs = diffInspections(entry, exit);
    expect(pairs).toHaveLength(1);
    expect(pairs[0].entry).toBeDefined();
    expect(pairs[0].exit).toBeDefined();
    expect(pairs[0].status).toBe("worsened");
  });

  it("classifies worsened/unchanged/improved by condition rank", () => {
    const entry = [
      f("Sala", "Piso", "Ótimo"),
      f("Sala", "Parede", "Bom"),
      f("Cozinha", "Pia", "Regular"),
    ];
    const exit = [
      f("Sala", "Piso", "Regular"), // 5 → 3 = worsened
      f("Sala", "Parede", "Bom"), // 4 = unchanged
      f("Cozinha", "Pia", "Ótimo"), // 3 → 5 = improved (raro)
    ];
    const pairs = diffInspections(entry, exit);
    const byKey = Object.fromEntries(pairs.map((p) => [p.description.toLowerCase(), p.status]));
    expect(byKey["piso"]).toBe("worsened");
    expect(byKey["parede"]).toBe("unchanged");
    expect(byKey["pia"]).toBe("improved");
  });

  it("flags missing_in_exit when ENTRY item has no EXIT pair", () => {
    const entry = [f("Sala", "Tomada", "Bom")];
    const exit: Finding[] = [];
    const pairs = diffInspections(entry, exit);
    expect(pairs).toHaveLength(1);
    expect(pairs[0].status).toBe("missing_in_exit");
    expect(pairs[0].entry).toBeDefined();
    expect(pairs[0].exit).toBeUndefined();
  });

  it("flags added_in_exit when EXIT item has no ENTRY pair", () => {
    const entry: Finding[] = [];
    const exit = [f("Quarto", "Cortina", "Ruim")];
    const pairs = diffInspections(entry, exit);
    expect(pairs).toHaveLength(1);
    expect(pairs[0].status).toBe("added_in_exit");
    expect(pairs[0].exit).toBeDefined();
    expect(pairs[0].entry).toBeUndefined();
  });

  it("handles empty inputs", () => {
    expect(diffInspections([], [])).toEqual([]);
  });

  it("worsens when entry has condition and exit is empty (omitted)", () => {
    // Exit sem condição declarada conta como rank 0 < entry. Reflete
    // o caso real: inspetor da saída esqueceu de marcar, então o
    // resultado conservador é "piorou" (operador revisita).
    const entry = [f("Sala", "Piso", "Bom")];
    const exit = [f("Sala", "Piso", "")];
    const pairs = diffInspections(entry, exit);
    expect(pairs[0].status).toBe("worsened");
  });

  it("preserves entry-order within a room when sorting result", () => {
    const entry = [
      f("Cozinha", "Pia", "Bom"),
      f("Cozinha", "Azulejo", "Bom"),
      f("Cozinha", "Armário", "Bom"),
    ];
    const exit = [
      f("Cozinha", "Armário", "Regular"),
      f("Cozinha", "Pia", "Regular"),
      f("Cozinha", "Azulejo", "Regular"),
    ];
    const pairs = diffInspections(entry, exit);
    expect(pairs.map((p) => p.description)).toEqual(["Pia", "Azulejo", "Armário"]);
  });

  it("orders by room alphabetically (pt-BR collation)", () => {
    const entry = [
      f("Quarto", "Piso", "Bom"),
      f("Banheiro", "Pia", "Bom"),
      f("Cozinha", "Pia", "Bom"),
    ];
    const exit = entry.map((x) => ({ ...x, condition: "Regular" }));
    const pairs = diffInspections(entry, exit);
    const rooms = pairs.map((p) => p.room);
    expect(rooms).toEqual(["Banheiro", "Cozinha", "Quarto"]);
  });

  it("deduplicates entry findings with same (room, description) keeping first", () => {
    // Edge case: dois findings de ENTRY com a mesma key — o primeiro
    // vence (segundo é ignorado na hora de casar). Garante que não
    // duplicamos pairs.
    const entry = [
      f("Sala", "Piso", "Ótimo"),
      f("Sala", "Piso", "Ruim"),
    ];
    const exit = [f("Sala", "Piso", "Regular")];
    const pairs = diffInspections(entry, exit);
    expect(pairs).toHaveLength(1);
    expect(pairs[0].entry?.condition).toBe("Ótimo"); // primeiro vence
    expect(pairs[0].status).toBe("worsened"); // 5 → 3
  });
});

describe("summarize", () => {
  it("counts pairs by status correctly", () => {
    const entry = [
      f("Sala", "Piso", "Ótimo"),
      f("Sala", "Parede", "Bom"),
      f("Sala", "Janela", "Bom"),
      f("Cozinha", "Pia", "Regular"),
      f("Cozinha", "Removido", "Bom"),
    ];
    const exit = [
      f("Sala", "Piso", "Regular"), // worsened
      f("Sala", "Parede", "Bom"), // unchanged
      f("Sala", "Janela", "Ótimo"), // improved
      f("Cozinha", "Pia", "Regular"), // unchanged
      f("Cozinha", "Novo Item", "Ruim"), // added
      // "Removido" só na ENTRY → missing
    ];
    const pairs = diffInspections(entry, exit);
    const s = summarize(pairs);
    expect(s.total).toBe(6);
    expect(s.worsened).toBe(1);
    expect(s.unchanged).toBe(2);
    expect(s.improved).toBe(1);
    expect(s.missingInExit).toBe(1);
    expect(s.addedInExit).toBe(1);
  });

  it("zero counts on empty pairs", () => {
    const s = summarize([]);
    expect(s).toEqual({
      worsened: 0,
      unchanged: 0,
      improved: 0,
      missingInExit: 0,
      addedInExit: 0,
      total: 0,
    });
  });
});

describe("groupByRoom", () => {
  it("groups pairs by room preserving order", () => {
    const entry = [
      f("Sala", "Piso", "Bom"),
      f("Cozinha", "Pia", "Bom"),
      f("Sala", "Parede", "Bom"),
    ];
    const exit = entry.map((x) => ({ ...x, condition: "Regular" }));
    const pairs = diffInspections(entry, exit);
    const groups = groupByRoom(pairs);
    expect(groups.map((g) => g.room)).toEqual(["Cozinha", "Sala"]);
    const sala = groups.find((g) => g.room === "Sala")!;
    expect(sala.pairs).toHaveLength(2);
  });

  it("places empty-room items under '(sem cômodo)'", () => {
    const entry = [f("", "Item solto", "Bom")];
    const exit = entry.map((x) => ({ ...x, condition: "Ruim" }));
    const pairs = diffInspections(entry, exit);
    const groups = groupByRoom(pairs);
    expect(groups[0].room).toBe("(sem cômodo)");
  });
});
