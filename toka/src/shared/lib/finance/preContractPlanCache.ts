// Cache do plano pré-contrato de locação.
//
// O plano (início, prazo, término, primeiro vencimento, tratamento do primeiro
// aluguel) é um rascunho do operador que vive no cache do React Query enquanto
// a minuta não existe. Duas telas o consomem:
//
//   - a aba Assinatura (ContractTab) — onde ele é editado;
//   - a projeção da aba Financeiro (FinancialPreviewCard) — que precisa das
//     datas para não projetar 30 meses fixos quando o operador escolheu outro
//     prazo.
//
// A chave e o formato ficam aqui para as duas pontas lerem o mesmo rascunho sem
// uma importar a outra.

import type { FirstChargeType } from "../../rentals/types";

export interface CachedRentalPreContractPlan {
  startDate: string;
  endDate: string;
  firstChargeType: FirstChargeType;
  firstChargeDate: string;
  defaultDueDay: number;
  /** Prazo do contrato em meses — dirige o término (padrão 30). */
  termMonths: number;
}

/** Chave do rascunho do plano de locação para um negócio. */
export function rentalPlanCacheKey(dealId?: string) {
  return ["pre-contract-rental-plan", dealId || "unknown"] as const;
}
