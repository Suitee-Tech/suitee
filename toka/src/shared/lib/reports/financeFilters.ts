import type { Charge } from "../../services/charge.service";
import type { Commission } from "../../services/commission.service";
import type { OwnerTransfer } from "../../services/transfer.service";
import type { Contract } from "../../contracts/types";
import type { Property } from "../../data";

/**
 * Filtro de imóvel e corretor sobre as listas financeiras.
 *
 * Os endpoints de agregação do servidor (DRE, cashflow, summary) aceitam apenas
 * período — não imóvel nem corretor. Sem esta camada, mudar o filtro de imóvel
 * deixava os cartões financeiros intactos, mostrando em silêncio o total da
 * carteira como se fosse do imóvel selecionado.
 *
 * A ligação de cada fonte ao imóvel e ao corretor:
 * - cobrança  → `property_id`; o corretor vem do contrato (não há campo próprio)
 * - repasse   → `property_id`
 * - comissão  → corretor por `broker_id`; imóvel pelo contrato
 */

export interface FinanceFilterInput {
  propertyTypes: string[];
  agentIds: string[];
}

/** Resultado do filtro: listas reduzidas para o cálculo local. */
export interface FinanceFilteredLists {
  charges: Charge[];
  transfers: OwnerTransfer[];
  commissions: Commission[];
  /** Verdadeiro quando algum filtro que o servidor não conhece está ativo. */
  active: boolean;
}

/**
 * Reduz as listas financeiras conforme imóvel e corretor.
 *
 * Um imóvel só é excluído quando existe e seu tipo não está entre os
 * selecionados: dado ausente não pode remover a linha, senão o filtro esvazia a
 * tela por falta de cadastro em vez de por critério.
 */
export function applyFinanceFilters(
  charges: Charge[],
  transfers: OwnerTransfer[],
  commissions: Commission[],
  properties: Property[],
  contracts: Contract[],
  filter: FinanceFilterInput,
): FinanceFilteredLists {
  const byType = filter.propertyTypes.length > 0;
  const byAgent = filter.agentIds.length > 0;
  const active = byType || byAgent;

  if (!active) {
    return { charges, transfers, commissions, active: false };
  }

  const propertyTypeById = new Map(properties.map((p) => [p.id, p.type]));
  const propertyTypeByContract = new Map(
    contracts.filter((c) => c.property_id).map((c) => [c.id, propertyTypeById.get(c.property_id!)]),
  );

  const typeMatches = (propertyId: string | undefined): boolean => {
    if (!byType) return true;
    if (!propertyId) return true; // sem vínculo não é evidência de exclusão
    const type = propertyTypeById.get(propertyId);
    if (!type) return true;
    return filter.propertyTypes.includes(type);
  };

  const typeMatchesContract = (contractId: string | undefined): boolean => {
    if (!byType) return true;
    if (!contractId) return true;
    const type = propertyTypeByContract.get(contractId);
    if (!type) return true;
    return filter.propertyTypes.includes(type);
  };

  const agentSet = new Set(filter.agentIds);

  return {
    charges: charges.filter((c) => typeMatches(c.property_id)),
    transfers: transfers.filter((t) => typeMatches(t.property_id)),
    commissions: commissions.filter((c) => {
      if (byAgent && c.broker_id && !agentSet.has(c.broker_id)) return false;
      return typeMatchesContract(c.contract_id);
    }),
    active: true,
  };
}
