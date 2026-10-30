export const onlyDigits = (value: string) => value.replace(/\D/g, "");

export const parseCnpj = (value: string) => onlyDigits(value).slice(0, 14);

// Valida dígitos verificadores do CNPJ (algoritmo da Receita Federal).
// Compatível com o validador `cnpj_valid` do backend Go (pkg/validation/documents.go).
export const isValidCnpj = (value: string): boolean => {
  const digits = onlyDigits(value);
  if (digits.length !== 14) return false;
  if (/^(\d)\1{13}$/.test(digits)) return false;

  const calc = (base: string, weights: number[]): number => {
    let sum = 0;
    for (let i = 0; i < base.length; i++) {
      sum += parseInt(base[i], 10) * weights[i];
    }
    const r = sum % 11;
    return r < 2 ? 0 : 11 - r;
  };

  const d1 = calc(digits.slice(0, 12), [5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2]);
  if (digits[12] !== String(d1)) return false;
  const d2 = calc(digits.slice(0, 13), [6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2]);
  if (digits[13] !== String(d2)) return false;

  return true;
};

export const formatCnpj = (value: string) => {
  const digits = parseCnpj(value);
  const p1 = digits.slice(0, 2);
  const p2 = digits.slice(2, 5);
  const p3 = digits.slice(5, 8);
  const p4 = digits.slice(8, 12);
  const p5 = digits.slice(12, 14);

  let out = p1;
  if (digits.length > 2) out += "." + p2;
  if (digits.length > 5) out += "." + p3;
  if (digits.length > 8) out += "/" + p4;
  if (digits.length > 12) out += "-" + p5;
  return out;
};

export const parseCpf = (value: string) => onlyDigits(value).slice(0, 11);

// Valida dígitos verificadores do CPF (algoritmo da Receita Federal).
// Compatível com IsValidCPF do backend (pkg/validation/documents.go).
// Nunca relaxar — same-digit (e.g. 11111111111) é rejeitado de propósito.
export const isValidCpf = (value: string): boolean => {
  const digits = onlyDigits(value);
  if (digits.length !== 11) return false;
  if (/^(\d)\1{10}$/.test(digits)) return false;

  const calc = (base: string, weightStart: number): number => {
    let sum = 0;
    let w = weightStart;
    for (let i = 0; i < base.length; i++) {
      sum += parseInt(base[i], 10) * w;
      w--;
    }
    const r = (sum * 10) % 11;
    return r === 10 ? 0 : r;
  };

  const d1 = calc(digits.slice(0, 9), 10);
  if (digits[9] !== String(d1)) return false;
  const d2 = calc(digits.slice(0, 9) + String(d1), 11);
  if (digits[10] !== String(d2)) return false;

  return true;
};

export const formatCpf = (value: string) => {
  const digits = parseCpf(value);
  const p1 = digits.slice(0, 3);
  const p2 = digits.slice(3, 6);
  const p3 = digits.slice(6, 9);
  const p4 = digits.slice(9, 11);

  let out = p1;
  if (digits.length > 3) out += "." + p2;
  if (digits.length > 6) out += "." + p3;
  if (digits.length > 9) out += "-" + p4;
  return out;
};

// Documento combinado CPF/CNPJ — para campos de "documento" de uma
// pessoa que pode ser física (CPF, ≤11 dígitos) ou jurídica (CNPJ, 14
// dígitos), como as partes de um negócio. Limita a 14 dígitos.
export const parseCpfCnpj = (value: string) => onlyDigits(value).slice(0, 14);

// Aplica a máscara de CPF até 11 dígitos e a de CNPJ a partir daí, para
// que o usuário possa digitar qualquer um dos dois no mesmo campo.
export const formatCpfCnpj = (value: string) => {
  const digits = parseCpfCnpj(value);
  return digits.length <= 11 ? formatCpf(digits) : formatCnpj(digits);
};

// Valida o documento como CPF OU CNPJ (≤11 dígitos → CPF, senão CNPJ).
// Reaproveita isValidCpf/isValidCnpj — same-digit e DV inválido são
// rejeitados. String vazia é inválida; o caller decide obrigatoriedade.
export const isValidCpfCnpj = (value: string): boolean => {
  const digits = onlyDigits(value);
  return digits.length <= 11 ? isValidCpf(digits) : isValidCnpj(digits);
};

// Extracts the 10-11 dígitos canônicos (DDD + número) de qualquer formato
// brasileiro de telefone. Tolerante a country code: "+5511999999999" e
// "5511999999999" são reduzidos a "11999999999" antes do slice — evita
// que `+55...` armazenado no backend (pkg/validation/phone.go) reentre
// como "55119999999" e falhe na próxima normalização (DDD inválido).
export const parsePhoneBR = (value: string) => {
  const digits = onlyDigits(value);
  // Strip do country code "55" quando o resultado tem mais que 11 dígitos.
  // Não strip se o número original já tem ≤11 dígitos: pode coincidir com
  // DDD 55 (Rio Grande do Sul) seguido de mobile válido.
  if (digits.length > 11 && digits.startsWith("55")) {
    return digits.slice(2, 13);
  }
  return digits.slice(0, 11);
};

export const formatPhoneBR = (value: string) => {
  const digits = parsePhoneBR(value);
  const ddd = digits.slice(0, 2);
  const isMobile = digits.length > 10;
  const p1 = isMobile ? digits.slice(2, 7) : digits.slice(2, 6);
  const p2 = isMobile ? digits.slice(7, 11) : digits.slice(6, 10);

  if (!digits) return "";
  if (digits.length <= 2) return `(${ddd}`;
  if (digits.length <= (isMobile ? 7 : 6)) return `(${ddd}) ${p1}`;
  return `(${ddd}) ${p1}-${p2}`;
};

export const parseCep = (value: string) => onlyDigits(value).slice(0, 8);

export const formatCep = (value: string) => {
  const digits = parseCep(value);
  const p1 = digits.slice(0, 5);
  const p2 = digits.slice(5, 8);
  if (digits.length <= 5) return p1;
  return `${p1}-${p2}`;
};

/** Converte uma data ISO ou dd/mm/aaaa para o valor de <input type="date">. */
export const toDateInputValue = (value: string) => {
  const raw = (value ?? "").trim();
  const iso = raw.match(/^(\d{4})-(\d{2})-(\d{2})/);
  if (iso) return `${iso[1]}-${iso[2]}-${iso[3]}`;
  const br = raw.match(/^(\d{2})\/(\d{2})\/(\d{4})$/);
  if (br) return `${br[3]}-${br[2]}-${br[1]}`;
  return "";
};

/** Apresenta uma data ISO/dd-mm-aaaa como dd/mm/aaaa. */
export const formatDateBR = (value: string) => {
  const iso = toDateInputValue(value);
  if (!iso) return "";
  const [year, month, day] = iso.split("-");
  return `${day}/${month}/${year}`;
};

// Percentual como NÚMERO HUMANO (50 = 50%, não a fração 0,5). Todos os
// campos de % do app usam 0–100 (comissão, juros, multa), então não há
// divisão por 100 aqui — o clamp de faixa fica no zod.
//
// `parsePercentBR` aceita vírgula (pt-BR) ou ponto como separador decimal
// e ignora "%", espaços e lixo. O primeiro separador é o decimal; demais
// são descartados. String sem dígito -> `undefined` (distingue vazio de 0).
export const parsePercentBR = (value: string): number | undefined => {
  const cleaned = (value ?? "").replace(/[^\d.,]/g, "");
  if (!cleaned) return undefined;
  const firstSep = cleaned.search(/[.,]/);
  const intPart = firstSep === -1 ? cleaned : cleaned.slice(0, firstSep);
  const fracPart =
    firstSep === -1 ? "" : cleaned.slice(firstSep + 1).replace(/[.,]/g, "");
  const normalized = fracPart ? `${intPart}.${fracPart}` : intPart;
  const n = Number.parseFloat(normalized);
  return Number.isFinite(n) ? n : undefined;
};

export const formatPercentBR = (
  value: number | null | undefined,
  options?: { fractionDigits?: number },
): string => {
  if (value === null || value === undefined || !Number.isFinite(value)) return "";
  return new Intl.NumberFormat("pt-BR", {
    minimumFractionDigits: 0,
    maximumFractionDigits: options?.fractionDigits ?? 2,
  }).format(value);
};
