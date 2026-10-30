import { describe, expect, it } from "vitest";

import { applyFinanceFilters } from "./financeFilters";
import type { Charge } from "../../services/charge.service";
import type { Commission } from "../../services/commission.service";
import type { OwnerTransfer } from "../../services/transfer.service";
import type { Contract } from "../../contracts/types";
import type { Property } from "../../data";

const properties = [
  { id: "prop-apto", type: "Apartamento", title: "Apê" },
  { id: "prop-casa", type: "Casa", title: "Casa" },
] as unknown as Property[];

const contracts = [
  { id: "c-apto", property_id: "prop-apto", type: "locacao" },
  { id: "c-casa", property_id: "prop-casa", type: "venda" },
] as unknown as Contract[];

const charge = (id: string, propertyId: string): Charge =>
  ({ id, property_id: propertyId, total_amount: 100, amount: 100, status: "PAID" }) as Charge;

const transfer = (id: string, propertyId: string): OwnerTransfer =>
  ({ id, property_id: propertyId, net_amount: 90, status: "PROCESSED" }) as OwnerTransfer;

const commission = (id: string, brokerId: string, contractId: string): Commission =>
  ({ id, broker_id: brokerId, contract_id: contractId, amount: 50, status: "PAID" }) as Commission;

const noFilters = { propertyTypes: [], agentIds: [] };

/**
 * Os endpoints de agregação do servidor aceitam só período. Sem esta camada,
 * escolher um imóvel deixava os cartões financeiros mostrando a carteira
 * inteira como se fosse do imóvel selecionado.
 */
describe("applyFinanceFilters", () => {
  const charges = [charge("ch-apto", "prop-apto"), charge("ch-casa", "prop-casa")];
  const transfers = [transfer("tr-apto", "prop-apto"), transfer("tr-casa", "prop-casa")];
  const commissions = [
    commission("co-apto", "broker-1", "c-apto"),
    commission("co-casa", "broker-2", "c-casa"),
  ];

  it("sem filtro devolve as listas intactas e não marca filtro ativo", () => {
    const result = applyFinanceFilters(charges, transfers, commissions, properties, contracts, noFilters);
    expect(result.active).toBe(false);
    expect(result.charges).toHaveLength(2);
    expect(result.transfers).toHaveLength(2);
    expect(result.commissions).toHaveLength(2);
  });

  it("filtra cobranças e repasses pelo tipo do imóvel", () => {
    const result = applyFinanceFilters(charges, transfers, commissions, properties, contracts, {
      propertyTypes: ["Casa"],
      agentIds: [],
    });
    expect(result.active).toBe(true);
    expect(result.charges.map((c) => c.id)).toEqual(["ch-casa"]);
    expect(result.transfers.map((t) => t.id)).toEqual(["tr-casa"]);
    // A comissão liga-se ao imóvel pelo contrato.
    expect(result.commissions.map((c) => c.id)).toEqual(["co-casa"]);
  });

  it("filtra comissões pelo corretor selecionado", () => {
    const result = applyFinanceFilters(charges, transfers, commissions, properties, contracts, {
      propertyTypes: [],
      agentIds: ["broker-1"],
    });
    expect(result.commissions.map((c) => c.id)).toEqual(["co-apto"]);
    // Cobrança não tem corretor próprio, então a dimensão não a filtra.
    expect(result.charges).toHaveLength(2);
  });

  it("combina imóvel e corretor", () => {
    const result = applyFinanceFilters(charges, transfers, commissions, properties, contracts, {
      propertyTypes: ["Apartamento"],
      agentIds: ["broker-2"],
    });
    expect(result.charges.map((c) => c.id)).toEqual(["ch-apto"]);
    // broker-2 está no contrato da casa, excluído pelo filtro de imóvel.
    expect(result.commissions).toHaveLength(0);
  });

  it("não exclui linha sem vínculo cadastrado", () => {
    // Dado ausente não é evidência de exclusão: filtrar por falta de cadastro
    // esvaziaria a tela sem que o usuário tivesse pedido isso.
    const orphan = charge("ch-orfa", "");
    const result = applyFinanceFilters([orphan], [], [], properties, contracts, {
      propertyTypes: ["Casa"],
      agentIds: [],
    });
    expect(result.charges.map((c) => c.id)).toEqual(["ch-orfa"]);
  });

  it("não exclui cobrança cujo imóvel não está na lista de propriedades", () => {
    const unknown = charge("ch-desconhecido", "prop-inexistente");
    const result = applyFinanceFilters([unknown], [], [], properties, contracts, {
      propertyTypes: ["Casa"],
      agentIds: [],
    });
    expect(result.charges.map((c) => c.id)).toEqual(["ch-desconhecido"]);
  });
});
