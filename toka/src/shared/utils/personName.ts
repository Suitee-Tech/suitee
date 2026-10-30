/**
 * Regra de nome de pessoa — espelho de `pkg/validation/person_name.go`.
 *
 * O formulário precisa recusar o que a API recusa, com o MESMO texto: quando as
 * duas divergem, o operador preenche, o formulário aceita e o cadastro volta com
 * erro genérico do servidor (foi assim com o telefone). O lead alimenta a parte
 * do negócio, então vale a regra de nome completo — sem sobrenome o negócio nem
 * inicia na esteira.
 */

/** Espaços das pontas fora e sequências internas viram um espaço simples. */
export function sanitizePersonName(name: string): string {
  return name.trim().replace(/\s+/g, " ");
}

/** Motivo pelo qual o nome seria recusado; `null` quando está válido. */
export function personNameError(name: string, label = "Nome"): string | null {
  const clean = sanitizePersonName(name ?? "");
  if (!clean) return `${label} não pode estar vazio`;
  if ([...clean].length < 2) return `${label} muito curto (mínimo 2 caracteres)`;
  if (/\d/.test(clean)) return `${label} não pode conter números (recebido: "${clean}")`;
  if (clean.split(" ").length < 2) return `${label} precisa incluir sobrenome (ex: "João Silva")`;
  return null;
}
