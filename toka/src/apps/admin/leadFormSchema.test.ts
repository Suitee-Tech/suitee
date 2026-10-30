import { describe, expect, it } from "vitest";
import { leadSchema } from "./leadFormSchema";

// O formulário precisa recusar o que a API recusa — e dizer o motivo no campo,
// não deixar o operador descobrir com um erro genérico do servidor. A regra é a
// mesma de `pkg/validation/phone.go` (ver shared/utils/phone.test.ts).
const baseLead = {
  name: "Maria Souza",
  email: "maria@example.com",
  status: "Novo" as const,
  source: "Manual" as const,
};

const phoneError = (phone: string) => {
  const result = leadSchema.safeParse({ ...baseLead, phone });
  if (result.success) return null;
  return result.error.issues.find((issue) => issue.path.includes("phone"))?.message ?? null;
};

describe("apps/admin/leadFormSchema telefone", () => {
  it("aceita celular com DDD e fixo com DDD", () => {
    expect(phoneError("(88) 99999-9999")).toBeNull();
    expect(phoneError("8833334444")).toBeNull();
  });

  it("recusa o celular sem um dígito (DDD + 8 dígitos) com o motivo certo", () => {
    // Caso relatado: o formulário aceitava e a API devolvia erro genérico.
    expect(phoneError("8899999999")).toMatch(/Faltou um dígito/);
    expect(phoneError("(88) 9999-9999")).toMatch(/Faltou um dígito/);
  });

  it("recusa telefone sem DDD e explica o formato esperado", () => {
    expect(phoneError("88999999")).toMatch(/DDD \+ número/);
    expect(phoneError("99999999")).toMatch(/DDD \+ número/);
  });

  it("recusa DDD inexistente e celular sem o 9 inicial", () => {
    expect(phoneError("00999999999")).toMatch(/DDD inválido/);
    expect(phoneError("11888888888")).toMatch(/começar com 9/);
  });

  it("recusa fixo com prefixo que não existe (6 a 9 depois do DDD)", () => {
    expect(phoneError("8862345678")).toMatch(/fixo deve começar com 2, 3, 4 ou 5/);
  });

  it("normaliza a máscara antes de validar", () => {
    const result = leadSchema.safeParse({ ...baseLead, phone: "+55 (88) 9.9999-9999" });
    expect(result.success).toBe(true);
    if (result.success) expect(result.data.phone).toBe("88999999999");
  });
});

// O lead precisa de nome E sobrenome: é o mesmo nome que vira parte do negócio
// na esteira, e sem sobrenome o negócio nem inicia ("Nome do signatário precisa
// incluir sobrenome"). Antes o formulário aceitava só o primeiro nome.
const nameError = (name: string) => {
  const result = leadSchema.safeParse({ ...baseLead, name, phone: "88999999999" });
  if (result.success) return null;
  return result.error.issues.find((issue) => issue.path.includes("name"))?.message ?? null;
};

describe("apps/admin/leadFormSchema nome completo", () => {
  it("exige sobrenome com o mesmo texto da API", () => {
    expect(nameError("Maria")).toBe('Nome precisa incluir sobrenome (ex: "João Silva")');
  });

  it("aceita nome e sobrenome, inclusive composto", () => {
    expect(nameError("Maria Souza")).toBeNull();
    expect(nameError("José da Silva")).toBeNull();
    expect(nameError("  Maria   Souza  ")).toBeNull();
  });

  it("normaliza o nome antes de enviar", () => {
    const result = leadSchema.safeParse({ ...baseLead, name: "  João   Silva ", phone: "88999999999" });
    expect(result.success && result.data.name).toBe("João Silva");
  });

  it("mantém os motivos de vazio, curto e com números", () => {
    expect(nameError("")).toContain("não pode estar vazio");
    expect(nameError("J")).toContain("muito curto");
    expect(nameError("João 2 Silva")).toContain("não pode conter números");
  });
});
