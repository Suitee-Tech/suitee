import { describe, it, expect } from "vitest";

import { formatPercentBR, parsePercentBR } from "./masks";

describe("parsePercentBR", () => {
  it.each([
    ["", undefined],
    ["12", 12],
    ["12,5", 12.5],
    ["12.5%", 12.5],
    ["100", 100],
    ["0,5", 0.5],
    ["  7 % ", 7],
    ["abc", undefined],
    [",", undefined],
  ])("parse(%j) -> %j", (input, expected) => {
    expect(parsePercentBR(input as string)).toBe(expected);
  });
});

describe("formatPercentBR", () => {
  it("usa vírgula e descarta zeros desnecessários", () => {
    expect(formatPercentBR(12.5)).toBe("12,5");
    expect(formatPercentBR(12)).toBe("12");
    expect(formatPercentBR(6)).toBe("6");
  });

  it("vazio para null/undefined", () => {
    expect(formatPercentBR(undefined)).toBe("");
    expect(formatPercentBR(null)).toBe("");
  });

  it("limita casas decimais (default 2)", () => {
    expect(formatPercentBR(33.333)).toBe("33,33");
  });

  it("round-trip parse∘format", () => {
    expect(parsePercentBR(formatPercentBR(12.5))).toBe(12.5);
    expect(parsePercentBR(formatPercentBR(7))).toBe(7);
  });
});
