import { describe, expect, it } from "vitest";
import { personNameError, sanitizePersonName } from "./personName";

// Os casos espelham `pkg/validation/person_name_test.go`: se um lado mudar sem o
// outro, o formulário volta a aceitar o que a API recusa.
describe("sanitizePersonName", () => {
  it("tira espaços das pontas e normaliza o meio", () => {
    expect(sanitizePersonName("  João \t Silva  ")).toBe("João Silva");
    expect(sanitizePersonName("Maria   da    Silva")).toBe("Maria da Silva");
  });
});

describe("personNameError", () => {
  it("aceita nome e sobrenome (inclusive nomes compostos)", () => {
    expect(personNameError("João Silva")).toBeNull();
    expect(personNameError("José da Silva")).toBeNull();
    expect(personNameError("  Maria   Souza ")).toBeNull();
  });

  it("recusa monônimo com o mesmo texto da API", () => {
    expect(personNameError("João")).toBe('Nome precisa incluir sobrenome (ex: "João Silva")');
    expect(personNameError("Maria")).toContain("precisa incluir sobrenome");
  });

  it("recusa vazio, curto e com números na ordem da API", () => {
    expect(personNameError("")).toBe("Nome não pode estar vazio");
    expect(personNameError("   ")).toBe("Nome não pode estar vazio");
    expect(personNameError("J")).toContain("muito curto");
    expect(personNameError("João 2 Silva")).toContain("não pode conter números");
    expect(personNameError("Maria 3ª")).toContain("não pode conter números");
  });

  it("usa o rótulo do campo na mensagem", () => {
    expect(personNameError("João", "Nome do proprietário")).toBe(
      'Nome do proprietário precisa incluir sobrenome (ex: "João Silva")',
    );
  });
});
