import { describe, expect, it } from "vitest";
import {
  DOCUMENT_CATEGORIES,
  DOCUMENT_STATUSES,
  documentCategoryFromChecklistCategory,
  getCategoryLabel,
  getCategoryMeta,
  getSafeDocumentFallbackUrl,
  getStatusMeta,
  normalizeBackendStatus,
} from "./documents";

// Parity guard with toca-deal/internal/domain/document/document.go
// (`AllDocumentStatuses` and `AllCategories`). If either side changes,
// update both lists, `docs/domain/documents-status-mapping.md`, and the
// matching Go tests in `mapping_test.go`. Order matters for the docs
// table.
describe("status / category parity with backend", () => {
  it("DOCUMENT_STATUSES matches the Go AllDocumentStatuses slice", () => {
    expect(DOCUMENT_STATUSES).toEqual([
      "rascunho",
      "pendente",
      "pendente_assinatura",
      "parcialmente_assinado",
      "assinado",
      "expirado",
      "cancelado",
    ]);
  });

  it("DOCUMENT_CATEGORIES matches the Go AllCategories slice", () => {
    expect(DOCUMENT_CATEGORIES).toEqual([
      "contrato",
      "aditivo",
      "laudo",
      "vistoria",
      "procuracao",
      "recibo",
      "notificacao",
      "financeiro",
      "juridico",
      "fiscal",
      "identidade",
      "comprovante",
      "foto",
      "outros",
    ]);
  });
});

describe("getCategoryMeta", () => {
  it("returns a meta entry for every canonical category", () => {
    for (const cat of DOCUMENT_CATEGORIES) {
      const meta = getCategoryMeta(cat);
      expect(meta.key).toBe(cat);
      expect(meta.label).toBeTruthy();
      expect(meta.badgeClass).toBeTruthy();
    }
  });

  it("falls back to 'outros' for unknown categories", () => {
    // Cast — exercising the fallback branch a future backend value
    // would hit before we add the matching meta.
    const meta = getCategoryMeta("future_new_category" as never);
    expect(meta.key).toBe("outros");
  });
});

describe("getStatusMeta", () => {
  it("returns a meta entry for every canonical status", () => {
    for (const status of DOCUMENT_STATUSES) {
      const meta = getStatusMeta(status);
      expect(meta.key).toBe(status);
      expect(meta.label).toBeTruthy();
      expect(meta.badgeClass).toBeTruthy();
      expect(meta.icon).toBeDefined();
    }
  });
});

describe("getCategoryLabel", () => {
  it("returns Portuguese label for known categories", () => {
    // Rótulos descrevem o que o operador encontra na pasta (RG/CPF/CNH, notas
    // fiscais), não o nome técnico do enum.
    expect(getCategoryLabel("identidade")).toBe("Documento pessoal");
    expect(getCategoryLabel("fiscal")).toBe("Documento fiscal");
    expect(getCategoryLabel("financeiro")).toBe("Financeiro");
    expect(getCategoryLabel("contrato")).toBe("Contrato");
  });
});

describe("normalizeBackendStatus", () => {
  it("preserva 'pendente' — não é sinônimo de aguardando assinatura", () => {
    // O backend grava `pendente` em todo upload genérico (document.New).
    // `pendente_assinatura` só aparece quando o documento entrou em um fluxo
    // de assinatura; colapsar os dois fazia foto e comprovante aparecerem
    // como pendências de assinatura.
    expect(normalizeBackendStatus("pendente")).toBe("pendente");
    expect(normalizeBackendStatus("pendente_assinatura")).toBe("pendente_assinatura");
    expect(normalizeBackendStatus("pendente")).not.toBe("pendente_assinatura");
  });

  it("passes through canonical statuses unchanged", () => {
    for (const status of DOCUMENT_STATUSES) {
      expect(normalizeBackendStatus(status)).toBe(status);
    }
  });

  it("falls back to 'rascunho' for unknown / empty values", () => {
    expect(normalizeBackendStatus("future_value")).toBe("rascunho");
    expect(normalizeBackendStatus("")).toBe("rascunho");
    expect(normalizeBackendStatus(null)).toBe("rascunho");
    expect(normalizeBackendStatus(undefined)).toBe("rascunho");
  });
});

describe("getSafeDocumentFallbackUrl", () => {
  it.each([
    "https://cdn.example.com/legacy/document.pdf",
    "https://project.supabase.co/storage/v1/object/public/toca-documents/document.pdf",
    "https://project.supabase.co/storage/v1/object/sign/toca-documents/document.pdf?token=signed",
    "/uploads/toca-documents/legacy-document.pdf",
  ])("preserves a public or legacy document URL: %s", (url) => {
    expect(getSafeDocumentFallbackUrl(url)).toBe(url);
  });

  it.each([
    "https://project.supabase.co/storage/v1/object/authenticated/toca-documents/document.pdf",
    "http://host.docker.internal:54321/storage/v1/object/authenticated/toca-documents/document.pdf",
    "/storage/v1/object/authenticated/toca-documents/document.pdf",
    "https://project.supabase.co/storage%2Fv1%2Fobject%2Fauthenticated%2Ftoca-documents%2Fdocument.pdf",
  ])("rejects an unauthenticated private Storage fallback: %s", (url) => {
    expect(getSafeDocumentFallbackUrl(url)).toBeNull();
  });

  it("rejects empty, malformed and unsafe-protocol URLs", () => {
    expect(getSafeDocumentFallbackUrl(null)).toBeNull();
    expect(getSafeDocumentFallbackUrl("")).toBeNull();
    expect(getSafeDocumentFallbackUrl("http://[invalid")).toBeNull();
    expect(getSafeDocumentFallbackUrl("javascript:alert(1)")).toBeNull();
  });
});

describe("documentCategoryFromChecklistCategory", () => {
  // This table must stay in lockstep with
  // toca-deal/internal/domain/document/mapping.go::CategoryFromChecklistCategory.
  // If you change one side, change the other and update both tests.
  it.each([
    ["pessoal", "identidade"],
    ["financeiro", "financeiro"],
    ["imovel", "juridico"],
    ["juridico", "juridico"],
    ["outros", "outros"],
  ])("maps checklist category %s to document category %s", (input, expected) => {
    expect(documentCategoryFromChecklistCategory(input)).toBe(expected);
  });

  it("falls back to 'outros' for unknown / empty input", () => {
    expect(documentCategoryFromChecklistCategory("nova_categoria")).toBe("outros");
    expect(documentCategoryFromChecklistCategory("")).toBe("outros");
    expect(documentCategoryFromChecklistCategory(null)).toBe("outros");
    expect(documentCategoryFromChecklistCategory(undefined)).toBe("outros");
  });
});
