import { describe, expect, it } from "vitest";
import { normalizeBRPhone, phoneErrorMessage, phoneValidationError } from "./phone";

// Fixtures espelham pkg/validation/phone_test.go: as duas implementações
// precisam aceitar e recusar exatamente as mesmas entradas.
describe("shared/utils/phone.normalizeBRPhone", () => {
  it.each([
    ["mobile spaces", "11 99999 9999", "+5511999999999"],
    ["mobile parens dash", "(11) 99999-9999", "+5511999999999"],
    ["mobile dots", "11.99999.9999", "+5511999999999"],
    ["mobile with country code", "5511999999999", "+5511999999999"],
    ["mobile with +55", "+55 11 99999-9999", "+5511999999999"],
    ["landline 10 digits", "1133334444", "+551133334444"],
    ["landline with parens", "(11) 3333-4444", "+551133334444"],
    ["landline with country code", "551133334444", "+551133334444"],
    ["landline starting with 5", "1155554444", "+551155554444"],
    ["mobile rio 21", "21 98765 4321", "+5521987654321"],
  ])("normalizes %s", (_name, raw, want) => {
    const got = normalizeBRPhone(raw);
    expect(got.ok).toBe(true);
    if (got.ok) expect(got.value).toBe(want);
  });

  it.each([
    ["empty", "", "empty"],
    ["only spaces", "   ", "empty"],
    ["garbage letters", "abc", "empty"],
    ["too short (8 digits, no DDD)", "88999999", "length"],
    ["too short (4 digits)", "1199", "length"],
    ["too long", "115512345678901", "length"],
    ["invalid DDD 00", "00999999999", "ddd"],
    ["invalid DDD 10", "10999999999", "ddd"],
    ["invalid DDD 23", "23999999999", "ddd"],
    ["mobile missing 9", "11888888888", "mobile_missing_9"],
    // O caso do formulário: DDD + 8 dígitos (celular sem um dígito). A API
    // recusa; o formulário precisa recusar antes, com esta mensagem.
    ["mobile with 8 digits after DDD", "8899999999", "mobile_incomplete"],
    ["landline starts with 1", "1112345678", "landline_invalid_prefix"],
    ["landline starts with 0", "1102345678", "landline_invalid_prefix"],
    ["landline starts with 6", "1162345678", "landline_invalid_prefix"],
  ])("rejects %s", (_name, raw, reason) => {
    const got = normalizeBRPhone(raw);
    expect(got.ok).toBe(false);
    if (!got.ok) expect(got.reason).toBe(reason);
  });
});

describe("shared/utils/phone.phoneValidationError", () => {
  it("não devolve erro para telefone válido", () => {
    expect(phoneValidationError("(88) 99999-9999")).toBeNull();
    expect(phoneValidationError("8833334444")).toBeNull();
  });

  it("explica o problema de cada entrada inválida", () => {
    expect(phoneValidationError("88999999")).toMatch(/DDD \+ número/);
    expect(phoneValidationError("8899999999")).toMatch(/Faltou um dígito/);
    expect(phoneValidationError("11888888888")).toMatch(/começar com 9/);
    expect(phoneValidationError("1123456789")).toBeNull(); // fixo válido: DDD 11 + 2345-6789
    expect(phoneValidationError("1112345678")).toMatch(/fixo deve começar com 2, 3, 4 ou 5/);
    expect(phoneValidationError("00999999999")).toMatch(/DDD inválido/);
  });

  it("mantém as mensagens distintas entre si", () => {
    const messages = new Set(
      (["empty", "length", "ddd", "mobile_missing_9", "mobile_incomplete", "landline_invalid_prefix"] as const).map(phoneErrorMessage),
    );
    expect(messages.size).toBe(6);
  });
});
