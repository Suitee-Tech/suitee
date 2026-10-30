import { describe, expect, it } from "vitest";
import {
  maskAuditValue,
  maskCPF,
  maskCNPJ,
  maskEmail,
  maskName,
  maskPhone,
  maskPixKey,
} from "./pii-mask";

describe("maskEmail", () => {
  it("preserva primeira+última letras + domínio inteiro", () => {
    expect(maskEmail("ana.maria@example.com")).toBe("a***a@example.com");
    expect(maskEmail("contato@suitee.com.br")).toBe("c***o@suitee.com.br");
  });

  it("local muito curto vira só ***@domain", () => {
    expect(maskEmail("a@x.com")).toBe("***@x.com");
    expect(maskEmail("ab@x.com")).toBe("***@x.com");
  });

  it("não-email passa direto", () => {
    expect(maskEmail("não é email")).toBe("não é email");
    expect(maskEmail("")).toBe("");
  });
});

describe("maskCPF", () => {
  it("formato com pontuação", () => {
    expect(maskCPF("123.456.789-00")).toBe("***.***.789-00");
  });

  it("formato só dígitos", () => {
    expect(maskCPF("12345678900")).toBe("***.***.789-00");
  });

  it("não-CPF passa direto", () => {
    expect(maskCPF("xyz")).toBe("xyz");
  });
});

describe("maskCNPJ", () => {
  it("formato pontuado", () => {
    expect(maskCNPJ("12.345.678/0001-99")).toBe("**.***.***/0001-99");
  });

  it("só dígitos", () => {
    expect(maskCNPJ("12345678000199")).toBe("**.***.***/0001-99");
  });
});

describe("maskPhone", () => {
  it("preserva últimos 4 dígitos", () => {
    expect(maskPhone("+55 11 98765-4321")).toBe("(**) ****-4321");
    expect(maskPhone("11987654321")).toBe("(**) ****-4321");
  });

  it("número muito curto passa direto", () => {
    expect(maskPhone("123")).toBe("123");
  });
});

describe("maskPixKey", () => {
  it("preserva últimos 4 chars", () => {
    expect(maskPixKey("ana@suitee.com.br")).toBe("••••m.br");
    expect(maskPixKey("12345678900")).toBe("••••8900");
  });

  it("valor curto fica ****", () => {
    expect(maskPixKey("abc")).toBe("****");
    expect(maskPixKey("")).toBe("");
  });
});

describe("maskName", () => {
  it("preserva inicial de cada palavra", () => {
    expect(maskName("Ana Maria Silva")).toBe("A*** M*** S***");
    expect(maskName("João")).toBe("J***");
  });

  it("nome muito curto passa direto", () => {
    expect(maskName("A")).toBe("A");
    expect(maskName("")).toBe("");
  });
});

describe("maskAuditValue — field-name based", () => {
  it("email → mascara mesmo se field é só 'email'", () => {
    expect(maskAuditValue("email", "ana@x.com")).toBe("a***a@x.com");
  });

  it("cpf field", () => {
    expect(maskAuditValue("cpf", "12345678900")).toBe("***.***.789-00");
  });

  it("phone field", () => {
    expect(maskAuditValue("phone", "11987654321")).toBe("(**) ****-4321");
    expect(maskAuditValue("telefone", "11987654321")).toBe("(**) ****-4321");
  });

  it("pix_key field", () => {
    expect(maskAuditValue("pix_key", "ana@x.com")).toBe("••••.com");
  });

  it("account / agency fields", () => {
    expect(maskAuditValue("account", "123456")).toBe("••••3456");
    expect(maskAuditValue("agencia", "1234")).toBe("****");
  });

  it("name fields", () => {
    expect(maskAuditValue("name", "Ana Maria")).toBe("A*** M***");
    expect(maskAuditValue("firstname", "Ana")).toBe("A***");
    expect(maskAuditValue("fullname", "Maria da Silva")).toBe("M*** d*** S***");
  });
});

describe("maskAuditValue — value-format based fallback", () => {
  it("detecta email pelo formato mesmo quando field genérico", () => {
    expect(maskAuditValue("custom_field", "ana@suitee.com.br")).toBe("a***a@suitee.com.br");
  });

  it("detecta CPF pelo formato", () => {
    expect(maskAuditValue("identifier", "123.456.789-00")).toBe("***.***.789-00");
  });

  it("detecta CNPJ pelo formato", () => {
    expect(maskAuditValue("doc", "12.345.678/0001-99")).toBe("**.***.***/0001-99");
  });

  it("NÃO mascara IDs que parecem dígitos quando field tem 'id' ou 'code'", () => {
    expect(maskAuditValue("user_id", "12345678901")).toBe("12345678901");
    expect(maskAuditValue("code", "12345678901")).toBe("12345678901");
  });
});

describe("maskAuditValue — non-PII passa direto", () => {
  it("status enums", () => {
    expect(maskAuditValue("status", "CONVERTED")).toBe("CONVERTED");
    expect(maskAuditValue("status", "PENDING_ACCEPTANCE")).toBe("PENDING_ACCEPTANCE");
  });

  it("booleans/numbers como string", () => {
    expect(maskAuditValue("active", "true")).toBe("true");
    expect(maskAuditValue("value", "1500.00")).toBe("1500.00");
  });

  it("strings curtas (não atende formato PII)", () => {
    expect(maskAuditValue("action", "create")).toBe("create");
  });

  it("vazio passa direto", () => {
    expect(maskAuditValue("email", "")).toBe("");
  });
});

describe("maskAuditValue — cenários reais de audit", () => {
  it("audit de mudança de banking_updated (#463)", () => {
    // Backend já gravou "last4" mas vamos garantir que valores
    // completos que vazem sejam mascarados. PixKey usa últimos 4 chars.
    expect(maskAuditValue("pix_key", "ana.maria@example.com")).toBe("••••.com");
    expect(maskAuditValue("account", "1234567890")).toBe("••••7890");
  });

  it("audit de consent_changed (#478) — não tem PII", () => {
    expect(maskAuditValue("marketing_emails", "true")).toBe("true");
    expect(maskAuditValue("terms_version", "1.0")).toBe("1.0");
  });

  it("audit de lead.created — name + email expostos", () => {
    expect(maskAuditValue("Name", "Ana Maria Silva")).toBe("A*** M*** S***");
    expect(maskAuditValue("Email", "ana.silva@example.com")).toBe("a***a@example.com");
  });
});
