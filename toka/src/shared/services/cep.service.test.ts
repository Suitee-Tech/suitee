import { afterEach, describe, expect, it, vi } from "vitest";
import { lookupCep } from "./cep.service";

describe("lookupCep", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("retorna o endereço enriquecido quando o ViaCEP responde", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ cep: "01310-100", logradouro: "Avenida Paulista", bairro: "Bela Vista", localidade: "São Paulo", uf: "SP" }),
    }));
    const result = await lookupCep("01310100");
    expect(result).toEqual({
      cep: "01310-100",
      street: "Avenida Paulista",
      neighborhood: "Bela Vista",
      city: "São Paulo",
      stateCode: "SP",
      state: "São Paulo",
    });
  });

  it("retorna null para CEP inválido (menos de 8 dígitos)", async () => {
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
    expect(await lookupCep("01310")).toBeNull();
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("retorna null quando o ViaCEP indica erro ou CEP inexistente", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ erro: true }),
    }));
    expect(await lookupCep("99999999")).toBeNull();
  });
});
