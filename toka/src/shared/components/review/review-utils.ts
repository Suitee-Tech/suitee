import type { PartyDTO } from "../../services/deal.service";
import type { ExtractedDocumentDTO, ExtractedFieldDTO } from "../../services/document.service";
import {
  formatCep,
  formatCpfCnpj,
  formatPhoneBR,
  isValidCpfCnpj,
  onlyDigits,
  parseCep,
  parseCpfCnpj,
  parsePhoneBR,
} from "../../utils/masks";

export type FieldFormat = "cpf" | "date" | "phone" | "cep" | "uf" | "text";

/** Formato de apresentação/edição inferido pelo target/nome/rótulo do campo. */
export function fieldFormat(field: ExtractedFieldDTO): FieldFormat {
  const name = (field.name || "").toLowerCase();
  const target = (field.target || "").toLowerCase();
  const label = (field.label || "").toLowerCase();
  const key = `${name} ${label}`;

  if (isCpfCnpjField(field) || /cpf|cnpj/.test(key)) return "cpf";
  if (
    target === "party.birth_date" ||
    name === "birth_date" ||
    /nascimento|birth/.test(key) ||
    /\bdata\b/.test(name)
  ) {
    return "date";
  }
  if (target === "party.phone" || /telefone|celular|whatsapp|phone/.test(key)) return "phone";
  if (/\bcep\b/.test(key)) return "cep";
  if (name === "uf" || name === "estado" || target.endsWith(".uf") || label === "uf") return "uf";
  return "text";
}

function dateParts(value: string): { y: string; m: string; d: string } | null {
  const raw = value.trim();
  const iso = raw.match(/^(\d{4})-(\d{2})-(\d{2})/);
  if (iso) return { y: iso[1], m: iso[2], d: iso[3] };
  const br = raw.match(/^(\d{2})\/(\d{2})\/(\d{4})$/);
  if (br) return { y: br[3], m: br[2], d: br[1] };
  return null;
}

function isRealDate(parts: { y: string; m: string; d: string }): boolean {
  const { y, m, d } = parts;
  const date = new Date(Number(y), Number(m) - 1, Number(d));
  return (
    date.getFullYear() === Number(y) &&
    date.getMonth() === Number(m) - 1 &&
    date.getDate() === Number(d)
  );
}

/** Valor para input type=date (ISO yyyy-mm-dd) ou "" se não for data válida. */
export function toDateInputValue(value: string): string {
  const parts = dateParts(value);
  if (!parts || !isRealDate(parts)) return "";
  return `${parts.y}-${parts.m}-${parts.d}`;
}

function formatDateBR(value: string): string {
  const parts = dateParts(value);
  if (!parts || !isRealDate(parts)) return "";
  return `${parts.d}/${parts.m}/${parts.y}`;
}

/** Apresentação humana do valor (máscaras por tipo de campo). */
export function formatFieldValue(field: ExtractedFieldDTO, value: string): string {
  if (!value.trim()) return "";
  switch (fieldFormat(field)) {
    case "cpf":
      return formatCpfCnpj(value);
    case "date":
      return formatDateBR(value) || value;
    case "phone":
      return formatPhoneBR(value);
    case "cep":
      return formatCep(value);
    case "uf":
      return value.toUpperCase().slice(0, 2);
    default:
      return value;
  }
}

/** Normaliza o que o input entrega para o valor canônico salvo/aplicado. */
export function parseFieldInput(field: ExtractedFieldDTO, raw: string): string {
  switch (fieldFormat(field)) {
    case "cpf":
      return parseCpfCnpj(raw);
    case "date":
      return raw.trim();
    case "phone":
      return parsePhoneBR(raw);
    case "cep":
      return parseCep(raw);
    case "uf":
      return raw.toUpperCase().replace(/[^A-Z]/g, "").slice(0, 2);
    default:
      return raw;
  }
}

/** Valor atual da party para o campo extraído, para destacar divergências. */
export function currentPartyValue(party: PartyDTO | null, field: ExtractedFieldDTO): string {
  if (!party) return "";
  const target = field.target || `party.${field.name}`;
  switch (target) {
    case "party.name":
      return party.name ?? "";
    case "party.email":
      return party.email ?? "";
    case "party.phone":
      return party.phone ?? "";
    case "party.document":
      return party.document ?? "";
    case "party.birth_date":
      return party.birth_date ?? "";
    default: {
      const prefix = "party.custom_fields.";
      if (target.startsWith(prefix)) return party.custom_fields?.[target.slice(prefix.length)] ?? "";
      return party.custom_fields?.[field.name] ?? "";
    }
  }
}

export function isCpfCnpjField(field: ExtractedFieldDTO): boolean {
  return field.name === "document" || field.target === "party.document";
}

function comparableValue(field: ExtractedFieldDTO, value: string): string {
  switch (fieldFormat(field)) {
    case "cpf":
      return parseCpfCnpj(value);
    case "date":
      return toDateInputValue(value) || value.trim();
    case "phone":
      return parsePhoneBR(value);
    case "cep":
      return parseCep(value);
    case "uf":
      return value.toUpperCase().trim();
    default:
      return value.trim();
  }
}

export function fieldDiverges(
  party: PartyDTO | null,
  field: ExtractedFieldDTO,
  value: string,
): boolean {
  const current = currentPartyValue(party, field);
  if (!current.trim() || !value.trim()) return false;
  return comparableValue(field, current) !== comparableValue(field, value);
}

export function isFieldInvalid(field: ExtractedFieldDTO, value: string): boolean {
  if (!value.trim()) return false;
  if (isCpfCnpjField(field) && !isValidCpfCnpj(parseCpfCnpj(value))) return true;
  switch (fieldFormat(field)) {
    case "date":
      if (!toDateInputValue(value)) return true;
      break;
    case "phone": {
      const digits = parsePhoneBR(value);
      if (digits.length !== 10 && digits.length !== 11) return true;
      break;
    }
    case "cep":
      if (onlyDigits(value).length !== 8) return true;
      break;
    case "uf":
      if (!/^[A-Z]{2}$/.test(value.trim().toUpperCase())) return true;
      break;
    default:
      break;
  }
  return field.validation_status === "invalid" && value.trim() === field.value.trim();
}

export type ExtractionSummary = {
  total: number;
  divergences: number;
  low: number;
  high: number;
  missing: number;
  invalid: number;
};

/**
 * Classifica os campos em grupos mutuamente exclusivos para a fila:
 * divergência > baixa confiança > alta confiança; vazios viram "não identificados".
 */
export function summarizeExtraction(
  extraction: ExtractedDocumentDTO | null | undefined,
  party: PartyDTO | null,
): ExtractionSummary {
  const fields = extraction?.fields ?? [];
  let divergences = 0;
  let low = 0;
  let high = 0;
  let missing = 0;
  let invalid = 0;

  for (const field of fields) {
    const value = field.value ?? "";
    if (!value.trim()) {
      missing += 1;
      continue;
    }
    if (fieldDiverges(party, field, value)) {
      divergences += 1;
      continue;
    }
    const fieldInvalid = isFieldInvalid(field, value);
    if (fieldInvalid) invalid += 1;
    if (field.tier === "high" && !fieldInvalid) high += 1;
    else low += 1;
  }

  return { total: fields.length, divergences, low, high, missing, invalid };
}

export type ExtractionErrorInfo = { message: string; detail: string };

/**
 * Traduz erros crus do OCR/IA em uma mensagem acionável para o operador.
 * O detalhe técnico (paths, upstream, stack) fica separado e só é exibido
 * sob demanda — nunca como texto principal do card/fila.
 */
export function describeExtractionError(error?: string | null): ExtractionErrorInfo {
  const detail = (error ?? "").trim();
  if (!detail) {
    return {
      message: "O OCR não encontrou dados legíveis. Revise ou preencha manualmente.",
      detail: "",
    };
  }

  const lower = detail.toLowerCase();
  if (lower.includes("rapidocr artifacts") || lower.includes("artifacts not found")) {
    return {
      message:
        "O modelo de leitura do OCR não carregou. Reprocesse; se persistir, acione o suporte.",
      detail,
    };
  }
  if (lower.includes("upstream returned status 502") || lower.includes(" 502")) {
    return {
      message: "O serviço de OCR está indisponível no momento. Tente novamente em instantes.",
      detail,
    };
  }
  if (lower.includes("timeout") || lower.includes("timed out")) {
    return {
      message: "O OCR demorou mais que o esperado. Tente reprocessar.",
      detail,
    };
  }
  if (lower.includes("401") || lower.includes("unauthorized") || lower.includes("internal key")) {
    return {
      message: "Falha de autenticação interna do OCR. Acione o suporte.",
      detail,
    };
  }
  if (lower.includes("password") || lower.includes("senha") || lower.includes("encrypted")) {
    return {
      message: "O PDF parece protegido por senha. Envie uma versão sem senha.",
      detail,
    };
  }

  return {
    message: "Não foi possível ler este documento. Reprocesse ou envie um arquivo mais nítido.",
    detail,
  };
}
