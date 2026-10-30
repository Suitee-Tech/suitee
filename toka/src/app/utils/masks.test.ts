import { describe, expect, it } from "vitest";
import { isValidCnpj, formatCnpj, parseCnpj, formatPhoneBR, parsePhoneBR, formatCep, parseCep, isValidCpf, formatCpfCnpj, parseCpfCnpj, isValidCpfCnpj } from "./masks";

describe("isValidCnpj", () => {
  it("aceita CNPJs válidos com dígitos verificadores corretos", () => {
    // Exemplos conhecidos com check digits válidos.
    expect(isValidCnpj("11222333000181")).toBe(true);
    expect(isValidCnpj("11.222.333/0001-81")).toBe(true);
    expect(isValidCnpj("19131243000197")).toBe(true);
  });

  it("rejeita CNPJ com check digits inválidos", () => {
    // 14 dígitos mas check digits não batem (este é o CNPJ placeholder do bug que reportamos).
    expect(isValidCnpj("12345678000190")).toBe(false);
    expect(isValidCnpj("12345678000100")).toBe(false);
  });

  it("rejeita CNPJ com comprimento errado", () => {
    expect(isValidCnpj("")).toBe(false);
    expect(isValidCnpj("1234")).toBe(false);
    expect(isValidCnpj("123456789012345")).toBe(false);
  });

  it("rejeita CNPJ com todos os dígitos iguais", () => {
    expect(isValidCnpj("00000000000000")).toBe(false);
    expect(isValidCnpj("11111111111111")).toBe(false);
    expect(isValidCnpj("99999999999999")).toBe(false);
  });

  it("ignora caracteres não-numéricos da entrada", () => {
    expect(isValidCnpj("11.222.333/0001-81")).toBe(true);
    expect(isValidCnpj("11 222 333 / 0001 - 81")).toBe(true);
  });
});

describe("formatCnpj / parseCnpj", () => {
  it("formata progressivamente conforme o usuário digita", () => {
    expect(formatCnpj("")).toBe("");
    expect(formatCnpj("11")).toBe("11");
    expect(formatCnpj("11222")).toBe("11.222");
    expect(formatCnpj("11222333")).toBe("11.222.333");
    expect(formatCnpj("112223330001")).toBe("11.222.333/0001");
    expect(formatCnpj("11222333000181")).toBe("11.222.333/0001-81");
  });

  it("parseCnpj strip-a tudo que não é dígito e limita em 14", () => {
    expect(parseCnpj("11.222.333/0001-81")).toBe("11222333000181");
    expect(parseCnpj("abc 11.222.333/0001-81 def")).toBe("11222333000181");
    expect(parseCnpj("1122233300018199")).toBe("11222333000181");
  });
});

describe("isValidCpf", () => {
  it("aceita CPFs com check digits corretos", () => {
    expect(isValidCpf("529.982.247-25")).toBe(true);
    expect(isValidCpf("52998224725")).toBe(true);
    expect(isValidCpf("111.444.777-35")).toBe(true);
  });

  it("rejeita comprimento errado", () => {
    expect(isValidCpf("")).toBe(false);
    expect(isValidCpf("1234")).toBe(false);
    expect(isValidCpf("1234567890")).toBe(false);
  });

  it("rejeita todos os dígitos iguais (nunca relaxar)", () => {
    expect(isValidCpf("00000000000")).toBe(false);
    expect(isValidCpf("11111111111")).toBe(false);
    expect(isValidCpf("99999999999")).toBe(false);
  });

  it("rejeita check digits inválidos", () => {
    expect(isValidCpf("529.982.247-26")).toBe(false);
    expect(isValidCpf("111.444.777-30")).toBe(false);
  });
});

describe("formatCpfCnpj / parseCpfCnpj / isValidCpfCnpj", () => {
  it("aplica máscara de CPF até 11 dígitos e de CNPJ a partir daí", () => {
    expect(formatCpfCnpj("111444777")).toBe("111.444.777");
    expect(formatCpfCnpj("11144477735")).toBe("111.444.777-35");
    expect(formatCpfCnpj("112223330001")).toBe("11.222.333/0001");
    expect(formatCpfCnpj("11222333000181")).toBe("11.222.333/0001-81");
  });

  it("parseCpfCnpj strip-a não-dígitos e limita em 14", () => {
    expect(parseCpfCnpj("111.444.777-35")).toBe("11144477735");
    expect(parseCpfCnpj("11.222.333/0001-81")).toBe("11222333000181");
    expect(parseCpfCnpj("1122233300018199")).toBe("11222333000181");
  });

  it("valida como CPF (≤11 dígitos) ou CNPJ (>11) — nunca relaxa", () => {
    expect(isValidCpfCnpj("111.444.777-35")).toBe(true);
    expect(isValidCpfCnpj("11.222.333/0001-81")).toBe(true);
    expect(isValidCpfCnpj("")).toBe(false);
    expect(isValidCpfCnpj("111.444.777-30")).toBe(false); // CPF DV errado
    expect(isValidCpfCnpj("12345678000190")).toBe(false); // CNPJ DV errado
    expect(isValidCpfCnpj("11111111111")).toBe(false); // same-digit
  });
});

describe("formatPhoneBR / parsePhoneBR", () => {
  it("formata como celular (11 dígitos) com 5 dígitos no primeiro bloco", () => {
    expect(formatPhoneBR("11999998888")).toBe("(11) 99999-8888");
  });

  it("formata como fixo (10 dígitos) com 4 dígitos no primeiro bloco", () => {
    expect(formatPhoneBR("1133334444")).toBe("(11) 3333-4444");
  });

  it("parsePhoneBR strip-a não-dígitos e limita em 11", () => {
    expect(parsePhoneBR("(11) 99999-8888")).toBe("11999998888");
  });

  it("parsePhoneBR remove country code +55 antes do slice", () => {
    // Bug pré-fix: o backend salva como "+5511999998888"; o FE pegava
    // os 13 dígitos e fazia slice(0,11), retornando "55119999988" —
    // DDD=55 (válido, mas sub começa com '1' em vez de '9') que falha
    // na revalidação do backend (`invalid phone`). Agora strip do "55"
    // antes do slice preserva o número canônico.
    expect(parsePhoneBR("+5511999998888")).toBe("11999998888");
    expect(parsePhoneBR("5511999998888")).toBe("11999998888");
    expect(parsePhoneBR("+55 (11) 99999-8888")).toBe("11999998888");
  });

  it("parsePhoneBR preserva DDD 55 quando o número original já cabe em 11", () => {
    // Caso real: DDD 55 (Rio Grande do Sul). Sem country code prefix.
    expect(parsePhoneBR("(55) 99999-8888")).toBe("55999998888");
  });
});

describe("formatCep / parseCep", () => {
  it("formata CEP", () => {
    expect(formatCep("01310100")).toBe("01310-100");
    expect(formatCep("01310")).toBe("01310");
  });

  it("parseCep limita em 8 dígitos", () => {
    expect(parseCep("01310-100")).toBe("01310100");
    expect(parseCep("013101009999")).toBe("01310100");
  });
});
