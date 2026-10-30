import { stateNameFromCode } from "../utils/uf";

export interface CepLookupResult {
  cep: string;
  street: string;
  neighborhood: string;
  city: string;
  stateCode: string;
  state: string;
}

interface ViaCepResponse {
  cep?: string;
  logradouro?: string;
  complemento?: string;
  bairro?: string;
  localidade?: string;
  uf?: string;
  erro?: boolean;
}

const DIGITS = (cep: string) => cep.replace(/\D/g, "");

/**
 * Consulta o ViaCEP para enriquecer o endereço a partir do CEP. Retorna null
 * quando o CEP é inválido, não existe ou o provedor está indisponível — nesses
 * casos o operador continua preenchendo manualmente.
 */
export async function lookupCep(cep: string): Promise<CepLookupResult | null> {
  const digits = DIGITS(cep);
  if (digits.length !== 8) return null;
  try {
    const res = await fetch(`https://viacep.com.br/ws/${digits}/json/`, {
      headers: { Accept: "application/json" },
    });
    if (!res.ok) return null;
    const data: ViaCepResponse = await res.json();
    if (data.erro || (!data.localidade && !data.logradouro)) return null;
    const stateCode = (data.uf || "").toUpperCase();
    return {
      cep: data.cep || cep,
      street: data.logradouro || "",
      neighborhood: data.bairro || "",
      city: data.localidade || "",
      stateCode,
      state: stateCode ? stateNameFromCode(stateCode) : "",
    };
  } catch {
    return null;
  }
}
