// Mapa de siglas UF ↔ nomes de estados para consistência entre formulários
// (o backend exige `state` + `stateCode` no endereço; o usuário normalmente
// preenche só a sigla).
export const UF_STATE_NAMES: Record<string, string> = {
  AC: "Acre",
  AL: "Alagoas",
  AP: "Amapá",
  AM: "Amazonas",
  BA: "Bahia",
  CE: "Ceará",
  DF: "Distrito Federal",
  ES: "Espírito Santo",
  GO: "Goiás",
  MA: "Maranhão",
  MT: "Mato Grosso",
  MS: "Mato Grosso do Sul",
  MG: "Minas Gerais",
  PA: "Pará",
  PB: "Paraíba",
  PR: "Paraná",
  PE: "Pernambuco",
  PI: "Piauí",
  RJ: "Rio de Janeiro",
  RN: "Rio Grande do Norte",
  RS: "Rio Grande do Sul",
  RO: "Rondônia",
  RR: "Roraima",
  SC: "Santa Catarina",
  SP: "São Paulo",
  SE: "Sergipe",
  TO: "Tocantins",
};

const STATE_NAME_TO_CODE: Record<string, string> = Object.fromEntries(
  Object.entries(UF_STATE_NAMES).map(([code, name]) => [name.toLowerCase(), code]),
);

/** Retorna o nome completo do estado a partir da sigla (fallback: a própria sigla). */
export const stateNameFromCode = (code: string): string => {
  const norm = String(code || "").trim().toUpperCase();
  return UF_STATE_NAMES[norm] || norm;
};

/** Retorna a sigla (UF) a partir do nome do estado; "" quando não reconhecido. */
export const stateCodeFromName = (name: string): string => {
  const norm = String(name || "").trim().toLowerCase();
  return STATE_NAME_TO_CODE[norm] || "";
};
