// Regra canônica de telefone brasileiro no frontend.
//
// Espelha `pkg/validation/phone.go` (NormalizeBRPhone): as duas implementações
// precisam ACEITAR e RECUSAR exatamente as mesmas entradas. Quando elas
// divergem, o formulário deixa passar o que a API recusa e o operador recebe um
// erro genérico do servidor — foi o que aconteceu no cadastro de lead com um
// número de 10 dígitos começando por 9 (celular sem um dígito): o formulário
// aceitava pela contagem de dígitos e a API recusava, porque 10 dígitos só é
// válido para fixo (assinante começando em 2–5).
//
// Aceita: fixo com DDD (10 dígitos, assinante 2–5) e celular com DDD (11
// dígitos, assinante começando com 9), com ou sem +55 e com qualquer máscara.
// Recusa: DDD fora da lista oficial, comprimento diferente de 10/11 e prefixo
// incoerente com o tipo.

const BR_AREA_CODES = new Set([
  "11", "12", "13", "14", "15", "16", "17", "18", "19",
  "21", "22", "24", "27", "28",
  "31", "32", "33", "34", "35", "37", "38",
  "41", "42", "43", "44", "45", "46", "47", "48", "49",
  "51", "53", "54", "55",
  "61", "62", "63", "64", "65", "66", "67", "68", "69",
  "71", "73", "74", "75", "77", "79",
  "81", "82", "83", "84", "85", "86", "87", "88", "89",
  "91", "92", "93", "94", "95", "96", "97", "98", "99",
]);

export type PhoneIssue =
  | "empty"
  | "length"
  | "ddd"
  | "mobile_missing_9"
  | "mobile_incomplete"
  | "landline_invalid_prefix";

export interface NormalizedPhone {
  ok: true;
  value: string;
}

export interface InvalidPhone {
  ok: false;
  reason: PhoneIssue;
}

export type PhoneNormalizationResult = NormalizedPhone | InvalidPhone;

export function normalizeBRPhone(raw: string): PhoneNormalizationResult {
  const trimmed = raw?.trim() ?? "";
  const hasPlus = trimmed.startsWith("+");
  let digits = trimmed.replace(/\D+/g, "");
  if (digits.length === 0) return { ok: false, reason: "empty" };

  if (hasPlus && digits.startsWith("55")) digits = digits.slice(2);
  else if (digits.length > 11 && digits.startsWith("55")) digits = digits.slice(2);

  if (digits.length !== 10 && digits.length !== 11) return { ok: false, reason: "length" };

  const ddd = digits.slice(0, 2);
  if (!BR_AREA_CODES.has(ddd)) return { ok: false, reason: "ddd" };

  if (digits.length === 11 && digits[2] !== "9") {
    return { ok: false, reason: "mobile_missing_9" };
  }
  if (digits.length === 10) {
    const first = digits[2];
    // 10 dígitos começando por 9 é celular sem um dígito — o operador digitou
    // DDD + 8 dígitos, que é a forma antiga do celular. A mensagem precisa
    // dizer isso, não "fixo inválido".
    if (first === "9") return { ok: false, reason: "mobile_incomplete" };
    if (first < "2" || first > "5") return { ok: false, reason: "landline_invalid_prefix" };
  }
  return { ok: true, value: "+55" + digits };
}

const REASON_MESSAGES: Record<PhoneIssue, string> = {
  empty: "Informe o telefone",
  length: "Telefone deve ter DDD + número (10 ou 11 dígitos)",
  ddd: "DDD inválido — confira o código da sua região",
  mobile_missing_9: "Celular deve começar com 9 depois do DDD",
  mobile_incomplete: "Faltou um dígito: celular tem 9 dígitos depois do DDD. Ex.: (88) 99999-9999",
  landline_invalid_prefix: "Telefone fixo deve começar com 2, 3, 4 ou 5 depois do DDD",
};

// phoneErrorMessage é a mensagem que o formulário mostra no campo.
export function phoneErrorMessage(reason: PhoneIssue): string {
  return REASON_MESSAGES[reason];
}

// phoneValidationError devolve a mensagem do primeiro problema do telefone ou
// null quando ele é válido — é o atalho para os formulários que validam antes
// de enviar (e evita o operador só descobrir o problema no erro do servidor).
export function phoneValidationError(raw: string): string | null {
  const result = normalizeBRPhone(raw);
  return result.ok ? null : phoneErrorMessage(result.reason);
}
