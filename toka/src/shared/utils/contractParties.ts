import { resolveContractDateVariable } from "../contracts/contractDateVariables";

export interface PartyLike {
  id?: string;
  lead_id?: string;
  account_id?: string;
  name?: string;
  email?: string;
  phone?: string;
  document?: string;
  birth_date?: string;
  role?: string;
  kind?: string;
  custom_fields?: Record<string, unknown>;
  qualification?: Record<string, unknown>;
  is_primary?: boolean;
  share_pct?: number;
  signature_required?: boolean;
  spouse_of_party_id?: string;
  no_spouse_declared?: boolean;
  representation?: {
    type: string;
    representative_party_id: string;
    requires_principal_signature: boolean;
    allow_representative_proposal: boolean;
  };
  [key: string]: unknown;
}

export const PARTY_QUALIFICATION_FIELDS = [
  "birth_date", "birth_date_iso", "identity_document", "nationality", "marital_status", "occupation",
  "address", "address_street", "address_number", "address_complement", "address_neighborhood",
  "address_city", "address_state", "address_postal_code", "company_legal_name", "company_trade_name",
  "company_state_registration", "spouse_name", "spouse_document", "spouse_email", "representative_name",
  "representative_document", "representative_email", "representation_type", "represented_party_names_list",
] as const;

export interface PartyLoopItem extends Record<typeof PARTY_QUALIFICATION_FIELDS[number], string> {
  id?: string;
  lead_id: string;
  account_id: string;
  name: string;
  email: string;
  phone: string;
  document: string;
  role: string;
  kind: string;
  is_primary: boolean;
  share_pct?: number;
  signature_required: boolean;
  spouse_of_party_id: string;
  no_spouse_declared: boolean;
  representative_party_id: string;
  requires_principal_signature: boolean;
  allow_representative_proposal: boolean;
}

export const PARTY_DEFAULT_PLACEHOLDER = "______________________";
export const MAX_PARTIES_PER_ROLE = 5;

// Qualification extracted by the platform is stored in custom_fields using
// Portuguese names. Templates always use the canonical English field names.
function partyText(p: PartyLike, ...keys: string[]): string {
  for (const source of [p, p.custom_fields, p.qualification]) {
    for (const key of keys) {
      const value = source?.[key];
      if (typeof value === "string" && value.trim()) return value;
      if (typeof value === "number" && Number.isFinite(value)) return String(value);
    }
  }
  return "";
}

export function toPartyLoopItem(p: PartyLike, placeholder = PARTY_DEFAULT_PLACEHOLDER): PartyLoopItem {
  const addressStreet = partyText(p, "address_street", "street", "logradouro");
  const addressNumber = partyText(p, "address_number", "number", "numero");
  const addressComplement = partyText(p, "address_complement", "complement", "complemento");
  const addressNeighborhood = partyText(p, "address_neighborhood", "neighborhood", "bairro");
  const addressCity = partyText(p, "address_city", "city", "cidade");
  const addressState = partyText(p, "address_state", "state", "uf");
  const addressPostalCode = partyText(p, "address_postal_code", "postal_code", "postalCode", "cep");
  const address = partyText(p, "address", "formatted_address", "endereco") || [
    [addressStreet, addressNumber].filter(Boolean).join(", "),
    addressComplement,
    addressNeighborhood,
    [addressCity, addressState].filter(Boolean).join(" - "),
    addressPostalCode ? `CEP ${addressPostalCode}` : "",
  ].filter(Boolean).join(", ");
  const birthDate = partyText(p, "birth_date", "birthday", "data_nascimento");
  const birthDateIso = resolveContractDateVariable({ current_date: birthDate }, "current_date_iso") || "";

  return {
    id: p.id,
    lead_id: p.lead_id || "",
    account_id: p.account_id || "",
    name: p.name || placeholder,
    email: p.email || placeholder,
    phone: p.phone || placeholder,
    document: p.document || placeholder,
    role: p.role || "",
    kind: p.kind || "",
    is_primary: !!p.is_primary,
    share_pct: typeof p.share_pct === "number" ? p.share_pct : undefined,
    signature_required: p.signature_required ?? !!p.is_primary,
    birth_date: birthDateIso ? resolveContractDateVariable({ current_date: birthDateIso }, "current_date") || "" : "",
    birth_date_iso: birthDateIso,
    identity_document: partyText(p, "identity_document", "rg"),
    nationality: partyText(p, "nationality", "nacionalidade"),
    marital_status: partyText(p, "marital_status", "estado_civil"),
    occupation: partyText(p, "occupation", "profissao"),
    address,
    address_street: addressStreet,
    address_number: addressNumber,
    address_complement: addressComplement,
    address_neighborhood: addressNeighborhood,
    address_city: addressCity,
    address_state: addressState,
    address_postal_code: addressPostalCode,
    company_legal_name: partyText(p, "company_legal_name", "razao_social"),
    company_trade_name: partyText(p, "company_trade_name", "nome_fantasia"),
    company_state_registration: partyText(p, "company_state_registration", "inscricao_estadual"),
    spouse_of_party_id: p.spouse_of_party_id || "",
    no_spouse_declared: !!p.no_spouse_declared,
    spouse_name: partyText(p, "spouse_name"),
    spouse_document: partyText(p, "spouse_document"),
    spouse_email: partyText(p, "spouse_email"),
    representation_type: p.representation?.type || partyText(p, "representation_type"),
    representative_party_id: p.representation?.representative_party_id || "",
    representative_name: partyText(p, "representative_name"),
    representative_document: partyText(p, "representative_document"),
    representative_email: partyText(p, "representative_email"),
    represented_party_names_list: partyText(p, "represented_party_names_list"),
    requires_principal_signature: p.representation?.requires_principal_signature ?? false,
    allow_representative_proposal: p.representation?.allow_representative_proposal ?? false,
  };
}

// Indexed legacy slots preserve their original array order. Qualification
// aliases refer to the primary party; full qualifications for every party are
// available through the loop objects without imposing a party-count limit.
export function buildRoleLegacy(
  singular: string,
  plural: string,
  arr: PartyLike[],
  primary: PartyLike | undefined,
  placeholder = PARTY_DEFAULT_PLACEHOLDER,
): Record<string, string> {
  const out: Record<string, string> = {};
  out[`${plural}_count`] = String(arr.length);
  out[`${plural}_primary_name`] = primary?.name || placeholder;
  out[`${plural}_names_list`] = arr.map((p) => p.name || placeholder).join("; ") || placeholder;
  out[`${plural}_documents_list`] = arr.map((p) => p.document || placeholder).join("; ") || placeholder;

  const qualified = toPartyLoopItem(primary || {}, placeholder);
  for (const field of PARTY_QUALIFICATION_FIELDS) out[`${singular}_${field}`] = qualified[field];

  for (let i = 0; i < MAX_PARTIES_PER_ROLE; i++) {
    const p = arr[i];
    const suffix = i === 0 ? "" : `_${i + 1}`;
    for (const field of ["name", "document", "email", "phone"] as const) {
      out[`${singular}${suffix}_${field}`] = p?.[field] || placeholder;
    }
  }
  return out;
}
