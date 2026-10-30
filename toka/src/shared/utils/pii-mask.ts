const CPF_REGEX = /^(\d{3})\.?(\d{3})\.?(\d{3})-?(\d{2})$/;
const CPF_DIGITS_REGEX = /^\d{11}$/;
const CNPJ_REGEX = /^(\d{2})\.?(\d{3})\.?(\d{3})\/?(\d{4})-?(\d{2})$/;
const CNPJ_DIGITS_REGEX = /^\d{14}$/;
const EMAIL_REGEX = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const PHONE_BR_REGEX = /^\+?55?\s?\(?\d{2}\)?\s?\d{4,5}-?\d{4}$/;
const PHONE_DIGITS_REGEX = /^\d{10,13}$/;
const PIX_KEY_HINT = /^(pix_key|chave_pix|pixKey)$/i;

const PII_FIELD_NAMES = new Set([
  "email",
  "e_mail",
  "e-mail",
  "phone",
  "telefone",
  "celular",
  "cpf",
  "cnpj",
  "rg",
  "passport",
  "name",
  "nome",
  "firstname",
  "lastname",
  "fullname",
  "bio",
  "address",
  "endereco",
  "street",
  "rua",
  "zip_code",
  "cep",
  "account",
  "conta",
  "agency",
  "agencia",
  "pix_key",
  "pix_key_type",
  "holder_name",
  "holder_document",
  "bank_account",
]);

export function maskEmail(v: string): string {
  if (!v || !EMAIL_REGEX.test(v)) return v;
  const [local, domain] = v.split("@");
  if (local.length <= 2) return `***@${domain}`;
  return `${local[0]}***${local[local.length - 1]}@${domain}`;
}

export function maskCPF(v: string): string {
  const m =
    v.match(CPF_REGEX) ??
    (CPF_DIGITS_REGEX.test(v) ? [v, v.slice(0, 3), v.slice(3, 6), v.slice(6, 9), v.slice(9, 11)] : null);
  if (!m) return v;
  return `***.***.${m[3]}-${m[4]}`;
}

export function maskCNPJ(v: string): string {
  const m =
    v.match(CNPJ_REGEX) ??
    (CNPJ_DIGITS_REGEX.test(v)
      ? [v, v.slice(0, 2), v.slice(2, 5), v.slice(5, 8), v.slice(8, 12), v.slice(12, 14)]
      : null);
  if (!m) return v;
  return `**.***.***/${m[4]}-${m[5]}`;
}

export function maskPhone(v: string): string {
  const digits = v.replace(/\D/g, "");
  if (digits.length < 8) return v;
  const last4 = digits.slice(-4);
  return `(**) ****-${last4}`;
}

export function maskPixKey(v: string): string {
  if (!v) return v;
  if (v.length <= 4) return "****";
  return `••••${v.slice(-4)}`;
}

export function maskAccount(v: string): string {
  const digits = v.replace(/\D/g, "");
  if (!digits) return v;
  if (digits.length <= 4) return "****";
  return `••••${digits.slice(-4)}`;
}

export function maskName(v: string): string {
  if (!v || v.length < 2) return v;
  return v
    .split(/\s+/)
    .filter(Boolean)
    .map((part) => (part.length <= 1 ? part : `${part[0]}***`))
    .join(" ");
}

function isNonPIIScalar(value: string): boolean {
  if (value === "true" || value === "false") return true;
  if (/^-?\d+\.\d+$/.test(value)) return true;
  if (/^-?\d{1,4}$/.test(value)) return true;
  if (/^[A-Z][A-Z_]+$/.test(value) && value.length <= 30) return true;
  return false;
}

export function maskAuditValue(field: string, value: string): string {
  if (!value || typeof value !== "string") return value;

  const fname = field.toLowerCase().replace(/[-_]/g, "");

  if (fname === "cpf" || fname.endsWith("cpf")) return maskCPF(value);
  if (fname === "cnpj" || fname.endsWith("cnpj")) return maskCNPJ(value);

  if (PIX_KEY_HINT.test(field) || fname === "pixkey") return maskPixKey(value);

  if (
    fname === "account" ||
    fname === "agency" ||
    fname === "conta" ||
    fname === "agencia" ||
    fname.endsWith("accountnumber") ||
    fname.endsWith("agencynumber")
  ) {
    return maskAccount(value);
  }

  if (fname === "phone" || fname === "telefone" || fname === "celular" || fname.endsWith("phone") || fname.endsWith("telefone")) {
    return maskPhone(value);
  }

  if (fname === "email" || fname.endsWith("email") || fname === "mail") {
    if (isNonPIIScalar(value)) return value;
    if (EMAIL_REGEX.test(value)) return maskEmail(value);
    return value.length > 4 ? `${value[0]}***@***` : "***";
  }

  if (fname === "name" || fname === "nome" || fname === "firstname" || fname === "lastname" || fname === "fullname" || fname === "holdername") {
    return maskName(value);
  }

  if (isNonPIIScalar(value)) return value;

  const looksLikeID = fname === "id" || fname === "code" || fname.endsWith("id") || fname.endsWith("code");

  if (EMAIL_REGEX.test(value)) return maskEmail(value);
  if (!looksLikeID && (CPF_REGEX.test(value) || CPF_DIGITS_REGEX.test(value))) return maskCPF(value);
  if (!looksLikeID && (CNPJ_REGEX.test(value) || CNPJ_DIGITS_REGEX.test(value))) return maskCNPJ(value);
  if (PHONE_BR_REGEX.test(value) || (PHONE_DIGITS_REGEX.test(value) && value.length >= 10 && value.length <= 13)) {
    if (!looksLikeID) return maskPhone(value);
  }

  return value;
}

export { PII_FIELD_NAMES };

