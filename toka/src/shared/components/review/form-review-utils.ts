import type { DocumentChecklistItem } from "../../data";
import type { PartyDTO } from "../../services/deal.service";
import { parseCpfCnpj } from "../../utils/masks";

export type FormReviewField = {
  key: string;
  label: string;
  type: string;
  required: boolean;
  target: string;
  submitted: string;
  current: string;
};

export type FormCorrection = {
  name: string;
  target: string;
  value: string;
  confidence: number;
};

const CORE_TARGETS: Record<string, string> = {
  full_name: "party.name",
  cpf: "party.document",
  birth_date: "party.birth_date",
  phone: "party.phone",
};

export function formFieldTarget(key: string): string {
  return CORE_TARGETS[key] ?? `party.custom_fields.${key}`;
}

export function currentPartyValueForKey(party: PartyDTO | null, key: string): string {
  if (!party) return "";
  switch (key) {
    case "full_name":
      return party.name ?? "";
    case "cpf":
      return party.document ?? "";
    case "birth_date":
      return party.birth_date ?? "";
    case "phone":
      return party.phone ?? "";
    default:
      return party.custom_fields?.[key] ?? "";
  }
}

/** Compara CPF/CNPJ por dígitos; os demais campos por texto aparado. */
export function sameFormValue(key: string, a: string, b: string): boolean {
  if (key === "cpf") return parseCpfCnpj(a) === parseCpfCnpj(b);
  return a.trim() === b.trim();
}

export function buildFormReviewFields(
  item: DocumentChecklistItem,
  party: PartyDTO | null,
): FormReviewField[] {
  const submission = item.formSubmission;
  const requested = submission?.requestedFields?.length
    ? submission.requestedFields
    : item.requestedFields ?? [];
  const values = submission?.values ?? {};
  const fields: FormReviewField[] = requested
    .filter((field) => (field.key ?? "").trim() !== "")
    .map((field) => ({
      key: field.key.trim(),
      label: (field.label ?? field.key).trim() || field.key,
      type: field.type || "text",
      required: !!field.required,
      target: formFieldTarget(field.key.trim()),
      submitted: values[field.key.trim()] ?? "",
      current: currentPartyValueForKey(party, field.key.trim()),
    }));

  const known = new Set(fields.map((field) => field.key));
  for (const key of Object.keys(values)) {
    if (known.has(key)) continue;
    fields.push({
      key,
      label: key,
      type: "text",
      required: false,
      target: formFieldTarget(key),
      submitted: values[key] ?? "",
      current: currentPartyValueForKey(party, key),
    });
  }
  return fields;
}

/** Campos cujo valor corrigido difere do valor atual da party. */
export function formCorrections(
  fields: FormReviewField[],
  corrections: Record<string, string>,
): FormCorrection[] {
  return fields
    .filter((field) => {
      const value = corrections[field.key] ?? field.current;
      return !sameFormValue(field.key, value, field.current);
    })
    .map((field) => ({
      name: field.key,
      target: field.target,
      value: (corrections[field.key] ?? field.current).trim(),
      confidence: 1,
    }));
}

/** Correções obrigatórias que ficaram vazias bloqueiam a aprovação. */
export function missingRequiredCorrections(
  fields: FormReviewField[],
  corrections: Record<string, string>,
): FormReviewField[] {
  return fields.filter((field) => {
    if (!field.required) return false;
    const value = (corrections[field.key] ?? field.current).trim();
    return value === "";
  });
}
