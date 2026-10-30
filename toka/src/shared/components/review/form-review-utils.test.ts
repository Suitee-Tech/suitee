import { describe, expect, it } from "vitest";
import type { DocumentChecklistItem } from "../../data";
import type { PartyDTO } from "../../services/deal.service";
import {
  buildFormReviewFields,
  currentPartyValueForKey,
  formCorrections,
  formFieldTarget,
  missingRequiredCorrections,
  sameFormValue,
} from "./form-review-utils";

function party(overrides: Partial<PartyDTO> = {}): PartyDTO {
  return {
    id: "party-1",
    name: "Fernando Pessoa",
    email: "fernando@example.com",
    document: "12345678900",
    phone: "11999999999",
    custom_fields: { profissao: "Engenheiro" },
    role: "buyer",
    is_primary: true,
    created_at: "2026-10-19T00:00:00Z",
    updated_at: "2026-10-19T00:00:00Z",
    ...overrides,
  } as PartyDTO;
}

function item(overrides: Partial<DocumentChecklistItem> = {}): DocumentChecklistItem {
  return {
    id: "item-1",
    name: "Dados pessoais",
    category: "pessoal",
    status: "uploaded",
    required: true,
    requestMode: "form",
    requestedFields: [
      { key: "full_name", label: "Nome completo", type: "text", required: true },
      { key: "cpf", label: "CPF", type: "text", required: true },
      { key: "profissao", label: "Profissão", type: "text", required: false },
    ],
    formSubmission: {
      values: {
        full_name: "Fernando P.",
        cpf: "123.456.789-00",
        profissao: "Engenheiro",
      },
      requestedFields: [
        { key: "full_name", label: "Nome completo", type: "text", required: true },
        { key: "cpf", label: "CPF", type: "text", required: true },
        { key: "profissao", label: "Profissão", type: "text", required: false },
      ],
      submittedBy: "fernando@example.com",
      submittedAt: "2026-10-19T23:00:00Z",
    },
    ...overrides,
  };
}

describe("form-review-utils", () => {
  it("mapeia chaves para targets da party", () => {
    expect(formFieldTarget("full_name")).toBe("party.name");
    expect(formFieldTarget("cpf")).toBe("party.document");
    expect(formFieldTarget("profissao")).toBe("party.custom_fields.profissao");
    expect(currentPartyValueForKey(party(), "profissao")).toBe("Engenheiro");
  });

  it("compara CPF por dígitos e texto por trim", () => {
    expect(sameFormValue("cpf", "123.456.789-00", "12345678900")).toBe(true);
    expect(sameFormValue("cpf", "123.456.789-00", "123.456.789-01")).toBe(false);
    expect(sameFormValue("phone", " 11999999999 ", "11999999999")).toBe(true);
  });

  it("constrói campos a partir do snapshot, não do valor atual", () => {
    const fields = buildFormReviewFields(item(), party());
    expect(fields).toHaveLength(3);
    expect(fields[0]).toMatchObject({ key: "full_name", submitted: "Fernando P.", current: "Fernando Pessoa" });
    expect(fields[1]).toMatchObject({ key: "cpf", submitted: "123.456.789-00", current: "12345678900" });
  });

  it("gera correções apenas para valores alterados", () => {
    const fields = buildFormReviewFields(item(), party());
    const corrections = {
      full_name: "Fernando P.",
      cpf: "12345678900",
      profissao: "Engenheiro",
    };
    const payload = formCorrections(fields, corrections);
    expect(payload).toEqual([
      { name: "full_name", target: "party.name", value: "Fernando P.", confidence: 1 },
    ]);
  });

  it("bloqueia obrigatório vazio", () => {
    const fields = buildFormReviewFields(item(), party());
    const missing = missingRequiredCorrections(fields, { full_name: " ", cpf: "" });
    expect(missing.map((field) => field.key)).toEqual(["full_name", "cpf"]);
  });
});
