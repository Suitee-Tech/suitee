import type { CommissionRule } from "../services/commissionRules.service";
import type { TenantFinancialSettings } from "../services/tenantFinancialSettings.service";
import { calculateCommissionDistribution } from "./commissionRules";

/**
 * Base de cálculo da comissão padrão de um imóvel.
 *
 * Venda usa `default_commission_pct_sale` (percentual sobre o valor da venda);
 * locação usa `default_commission_pct_rent` (percentual sobre o primeiro
 * aluguel, a intermediação).
 */
export type CommissionBasis = "sale" | "rent";

export interface PropertyCommissionSummary {
  /** Base usada para o percentual total. */
  basis: CommissionBasis;
  /** Percentual total configurado para a base (ex.: 6 na venda). */
  totalPercentage: number;
  /** Fatia da imobiliária (resto da distribuição por papel). */
  agencyPercentage: number;
  /** Soma das fatias dos corretores (captador, visitador, fechamento...). */
  brokerPercentage: number;
  /** `true` quando existe uma distribuição por papel configurada. */
  hasDistribution: boolean;
  /** `true` quando os papéis passam de 100% (configuração inválida). */
  overallocated: boolean;
}

type CommissionSettingsSlice = Pick<
  TenantFinancialSettings,
  "default_commission_pct_sale" | "default_commission_pct_rent"
>;

/**
 * Descobre a finalidade que define o padrão de comissão.
 *
 * `availableFor` ("Disponível para" no cadastro) é a fonte mais específica:
 * um imóvel marcado só como locação não pode exibir o padrão de venda apenas
 * porque `listing_type` ficou "sale" (default do mapeamento/legado). Quando o
 * imóvel aceita venda e locação, a finalidade principal (`listingType`) decide.
 */
export function commissionBasisForListing(
  listingType?: string | null,
  availableFor?: readonly string[] | null,
): CommissionBasis {
  const purposes = new Set(
    (availableFor ?? [])
      .map((value) => String(value).toLowerCase())
      .filter((value) => value === "sale" || value === "rent"),
  );
  if (purposes.size === 1) {
    return purposes.has("rent") ? "rent" : "sale";
  }
  return listingType === "rent" ? "rent" : "sale";
}

/**
 * Traduz a configuração da imobiliária (percentual padrão + regras por papel)
 * para os números exibidos no detalhe do imóvel.
 *
 * A fonte é sempre o tenant: nada aqui tem valor de negócio embutido. Os
 * percentuais por papel passam pelo mesmo cálculo usado em Configurações ›
 * Comissão, de modo que a tela do imóvel e a tela de configuração concordem.
 */
export function resolvePropertyCommissionSummary(input: {
  listingType?: string | null;
  availableFor?: readonly string[] | null;
  settings?: CommissionSettingsSlice | null;
  rules?: CommissionRule[] | null;
}): PropertyCommissionSummary {
  const basis = commissionBasisForListing(input.listingType, input.availableFor);
  const configuredTotal =
    basis === "rent"
      ? input.settings?.default_commission_pct_rent
      : input.settings?.default_commission_pct_sale;
  const totalPercentage =
    typeof configuredTotal === "number" && Number.isFinite(configuredTotal)
      ? Math.max(0, configuredTotal)
      : 0;

  const distribution = calculateCommissionDistribution(input.rules ?? []);

  return {
    basis,
    totalPercentage,
    agencyPercentage: distribution.agencyPercentage,
    brokerPercentage: distribution.nonAgencyPercentage,
    hasDistribution: distribution.totalPercentage > 0,
    overallocated: distribution.overallocated,
  };
}

const PERCENT_FORMATTER = new Intl.NumberFormat("pt-BR", {
  maximumFractionDigits: 2,
});

/** 40 → "40"; 5.5 → "5,5"; 3.333 → "3,33". */
export function formatCommissionPercentage(value: number): string {
  if (!Number.isFinite(value)) return "0";
  return PERCENT_FORMATTER.format(value);
}

/**
 * Rateio legível para o card do imóvel: "40% imobiliária / 60% corretor".
 * Fatias zeradas são omitidas (ex.: "100% corretor"), e uma configuração
 * inexistente devolve `null` para a UI explicar em vez de exibir 0%/0%.
 */
export function formatCommissionSplit(
  summary: Pick<
    PropertyCommissionSummary,
    "agencyPercentage" | "brokerPercentage" | "hasDistribution" | "overallocated"
  >,
): string | null {
  if (!summary.hasDistribution || summary.overallocated) return null;

  const parts: string[] = [];
  if (summary.agencyPercentage > 0) {
    parts.push(`${formatCommissionPercentage(summary.agencyPercentage)}% imobiliária`);
  }
  if (summary.brokerPercentage > 0) {
    parts.push(`${formatCommissionPercentage(summary.brokerPercentage)}% corretor`);
  }
  return parts.length > 0 ? parts.join(" / ") : null;
}
