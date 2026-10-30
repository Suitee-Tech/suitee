import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { clearAgentsCache, readAgentsCache, writeAgentsCache } from "./agentsCache";
import type { Agent } from "../types/agent";

const SAMPLE: Agent[] = [
  { id: "a1", name: "Alice", role: "corretor", avatar: "https://cdn/img/a1.png" },
  { id: "a2", name: "Bob", role: "gerente" },
];

describe("agentsCache", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    window.sessionStorage.clear();
  });

  it("retorna null quando não há cache para a empresa", () => {
    expect(readAgentsCache("company-1")).toBeNull();
  });

  it("retorna null quando companyId é vazio", () => {
    expect(readAgentsCache("")).toBeNull();
  });

  it("escreve e lê de volta", () => {
    writeAgentsCache("company-1", SAMPLE);
    expect(readAgentsCache("company-1")).toEqual(SAMPLE);
  });

  it("escopa por empresa — leitura de outra empresa não acha", () => {
    writeAgentsCache("company-A", SAMPLE);
    expect(readAgentsCache("company-B")).toBeNull();
    expect(readAgentsCache("company-A")).toEqual(SAMPLE);
  });

  it("invalida após 24h (TTL expirado)", () => {
    const realNow = Date.now;
    const baseTime = 1_700_000_000_000;
    vi.spyOn(Date, "now").mockReturnValue(baseTime);
    writeAgentsCache("company-1", SAMPLE);
    // Avança 25 horas
    vi.spyOn(Date, "now").mockReturnValue(baseTime + 25 * 60 * 60 * 1000);
    expect(readAgentsCache("company-1")).toBeNull();
    Date.now = realNow;
  });

  it("ainda é válido após 23h (dentro do TTL)", () => {
    const realNow = Date.now;
    const baseTime = 1_700_000_000_000;
    vi.spyOn(Date, "now").mockReturnValue(baseTime);
    writeAgentsCache("company-1", SAMPLE);
    vi.spyOn(Date, "now").mockReturnValue(baseTime + 23 * 60 * 60 * 1000);
    expect(readAgentsCache("company-1")).toEqual(SAMPLE);
    Date.now = realNow;
  });

  it("readAgentsCache retorna null se o JSON estiver corrompido", () => {
    window.sessionStorage.setItem("crm:agents:company-1", "{ not valid json");
    expect(readAgentsCache("company-1")).toBeNull();
  });

  it("clearAgentsCache(id) remove só a entrada da empresa específica", () => {
    writeAgentsCache("company-A", SAMPLE);
    writeAgentsCache("company-B", SAMPLE);
    clearAgentsCache("company-A");
    expect(readAgentsCache("company-A")).toBeNull();
    expect(readAgentsCache("company-B")).toEqual(SAMPLE);
  });

  it("clearAgentsCache() sem argumento limpa todas as entradas", () => {
    writeAgentsCache("company-A", SAMPLE);
    writeAgentsCache("company-B", SAMPLE);
    clearAgentsCache();
    expect(readAgentsCache("company-A")).toBeNull();
    expect(readAgentsCache("company-B")).toBeNull();
  });

  it("write/read são no-op se sessionStorage lança erro", () => {
    const setSpy = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("QuotaExceeded");
    });
    // Não deve lançar
    expect(() => writeAgentsCache("company-1", SAMPLE)).not.toThrow();
    setSpy.mockRestore();
  });
});
