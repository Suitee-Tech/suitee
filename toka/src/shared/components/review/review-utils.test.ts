import { describe, expect, it } from "vitest";
import type { PartyDTO } from "../../services/deal.service";
import type { ExtractedDocumentDTO, ExtractedFieldDTO } from "../../services/document.service";
import {
  currentPartyValue,
  describeExtractionError,
  fieldDiverges,
  fieldFormat,
  formatFieldValue,
  isFieldInvalid,
  parseFieldInput,
  summarizeExtraction,
  toDateInputValue,
} from "./review-utils";

function party(overrides: Partial<PartyDTO> = {}): PartyDTO {
  return {
    id: "party-1",
    name: "Fernando Pessoa",
    email: "fernando@example.com",
    document: "12345678900",
    phone: "11999999999",
    role: "buyer",
    is_primary: true,
    created_at: "2026-10-19T00:00:00Z",
    updated_at: "2026-10-19T00:00:00Z",
    ...overrides,
  } as PartyDTO;
}

function field(overrides: Partial<ExtractedFieldDTO> = {}): ExtractedFieldDTO {
  return {
    name: "name",
    label: "Nome",
    target: "party.name",
    value: "Fernando Pessoa",
    confidence: 0.98,
    tier: "high",
    source: "ocr",
    validation_status: "valid",
    ...overrides,
  };
}

describe("review-utils", () => {
  it("resolve valor atual de campo padrão e customizado", () => {
    const p = party({ custom_fields: { profissao: "Engenheiro" } });
    expect(currentPartyValue(p, field())).toBe("Fernando Pessoa");
    expect(currentPartyValue(p, field({ name: "profissao", target: "party.custom_fields.profissao" }))).toBe("Engenheiro");
  });

  it("compara CPF sem falso positivo de formatação", () => {
    const p = party({ document: "123.456.789-00" });
    const cpf = field({ name: "document", target: "party.document", value: "12345678900" });
    expect(fieldDiverges(p, cpf, cpf.value)).toBe(false);
    expect(fieldDiverges(p, cpf, "12345678901")).toBe(true);
  });

  it("detecta CPF inválido e validação de backend", () => {
    const cpf = field({ name: "document", target: "party.document", value: "111.111.111-11" });
    expect(isFieldInvalid(cpf, cpf.value)).toBe(true);
    const invalid = field({ name: "birth_date", validation_status: "invalid", value: "31/02/1990" });
    expect(isFieldInvalid(invalid, invalid.value)).toBe(true);
    expect(isFieldInvalid(invalid, "12/04/1988")).toBe(false);
  });

  it("formata e normaliza data, telefone, CEP e UF", () => {
    const birth = field({ name: "birth_date", target: "party.birth_date", value: "1993-04-06" });
    expect(fieldFormat(birth)).toBe("date");
    expect(toDateInputValue("1993-04-06")).toBe("1993-04-06");
    expect(formatFieldValue(birth, "1993-04-06")).toBe("06/04/1993");
    expect(toDateInputValue("06/04/1993")).toBe("1993-04-06");

    const phone = field({ name: "phone", target: "party.phone", value: "11999998888" });
    expect(formatFieldValue(phone, "11999998888")).toBe("(11) 99999-8888");
    expect(parseFieldInput(phone, "(11) 99999-8888")).toBe("11999998888");

    const cep = field({ name: "cep", target: "party.custom_fields.cep", value: "01310100" });
    expect(formatFieldValue(cep, "01310100")).toBe("01310-100");
    expect(parseFieldInput(cep, "01310-100")).toBe("01310100");

    const uf = field({ name: "uf", target: "party.custom_fields.uf", value: "sp" });
    expect(formatFieldValue(uf, "sp")).toBe("SP");
    expect(parseFieldInput(uf, "s1p")).toBe("SP");
  });

  it("valida formato de data, telefone e CEP", () => {
    expect(isFieldInvalid(field({ name: "birth_date", target: "party.birth_date" }), "31/02/1990")).toBe(true);
    expect(isFieldInvalid(field({ name: "birth_date", target: "party.birth_date" }), "06/04/1993")).toBe(false);
    expect(isFieldInvalid(field({ name: "phone", target: "party.phone" }), "11999")).toBe(true);
    expect(isFieldInvalid(field({ name: "phone", target: "party.phone" }), "11999998888")).toBe(false);
    expect(isFieldInvalid(field({ name: "cep", target: "party.custom_fields.cep" }), "0131")).toBe(true);
  });

  it("resume divergência, baixa confiança, alta confiança e ausentes", () => {
    const p = party();
    const extraction = {
      status: "completed",
      overall_confidence: 0.9,
      extracted_at: "2026-10-19T00:00:00Z",
      fields: [
        field(),
        field({ name: "document", target: "party.document", value: "12345678901", tier: "high" }),
        field({ name: "birth_date", target: "party.birth_date", value: "12/04/1988", tier: "low" }),
        field({ name: "phone", target: "party.phone", value: "", tier: "high" }),
      ],
    } as ExtractedDocumentDTO;

    const summary = summarizeExtraction(extraction, p);
    expect(summary).toMatchObject({ total: 4, divergences: 1, low: 1, high: 1, missing: 1 });
  });

  it("traduz falha de OCR em mensagem acionável e preserva detalhe técnico", () => {
    const info = describeExtractionError(
      "falha no OCR: RapidOCR artifacts not found in /root/.cache/docling/RapidOcr",
    );
    expect(info.message).toContain("modelo de leitura");
    expect(info.message).not.toContain("/root/.cache");
    expect(info.detail).toContain("RapidOCR");

    expect(describeExtractionError("upstream returned status 502").message).toContain(
      "indisponível",
    );
    expect(describeExtractionError("").message).toContain("dados legíveis");
  });
});
