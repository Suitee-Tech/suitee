import { describe, expect, it } from "vitest";
import { formatCompetencia } from "./format";

describe("formatCompetencia", () => {
  it("converte competência ISO YYYY-MM para o padrão brasileiro mmm/aaaa", () => {
    expect(formatCompetencia("2026-09")).toBe("Set/2026");
    expect(formatCompetencia("2026-10")).toBe("Out/2026");
    expect(formatCompetencia("2027-01")).toBe("Jan/2027");
  });

  it("devolve a entrada quando não é uma competência ISO válida", () => {
    expect(formatCompetencia("")).toBe("—");
    expect(formatCompetencia("sem-data")).toBe("sem-data");
    expect(formatCompetencia(undefined)).toBe("—");
  });
});
