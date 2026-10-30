import { describe, expect, it } from "vitest";

import { resolveReceivableAction, type ReceivableRow } from "./receivableAction";

// A decisão "onde a baixa acontece" é única para todo o módulo: estes testes
// travam a regra que padronizou o fluxo (painel lateral de cobrança, painel de
// lançamento ou orientação de conciliação — nunca um AlertDialog de status).
describe("resolveReceivableAction", () => {
  const chargeRow: ReceivableRow = {
    id: "charge-1",
    _chargeId: "charge-1",
    _source: "charge",
    valor: 2500,
    desc: "Aluguel de setembro",
  };

  it("manda a linha de cobrança para a baixa de cobrança", () => {
    expect(resolveReceivableAction(chargeRow, { defaultAmount: 1800 })).toEqual({
      kind: "charge",
      chargeId: "charge-1",
      defaultAmount: 1800,
    });
  });

  it("usa o valor da linha quando não há saldo em aberto informado", () => {
    expect(resolveReceivableAction(chargeRow)).toEqual({
      kind: "charge",
      chargeId: "charge-1",
      defaultAmount: 2500,
    });
  });

  it("trata a projeção de fatura vinculada como a mesma ação da cobrança", () => {
    const invoiceRow: ReceivableRow = {
      id: "invoice-9",
      recordSource: "invoice",
      linkedChargeID: "charge-7",
      value: 990.5,
      description: "Fatura RENT - 9f3a1c",
    };

    expect(resolveReceivableAction(invoiceRow)).toEqual({
      kind: "charge",
      chargeId: "charge-7",
      defaultAmount: 990.5,
    });
  });

  it("bloqueia fatura sem cobrança vinculada em vez de pedir dados que o backend recusa", () => {
    const orphanInvoice: ReceivableRow = {
      id: "invoice-9",
      recordSource: "invoice",
      linkedChargeID: null,
      value: 500,
    };

    expect(resolveReceivableAction(orphanInvoice)).toEqual({
      kind: "blocked",
      reason: "unresolved-invoice",
    });
  });

  it("manda o lançamento independente para a baixa padronizada", () => {
    const transactionRow: ReceivableRow = {
      id: "tx-1",
      _source: "transaction",
      desc: "Serviço de manutenção",
      valor: 320,
      kind_data: { transaction_type: "despesa" },
    };

    expect(resolveReceivableAction(transactionRow)).toEqual({
      kind: "settle",
      id: "tx-1",
      type: "despesa",
      description: "Serviço de manutenção",
      value: 320,
    });
  });

  it("cai no rótulo de receita e na descrição do domínio quando a linha não os traz", () => {
    expect(resolveReceivableAction({ id: "tx-2", value: 100 })).toEqual({
      kind: "settle",
      id: "tx-2",
      type: "receita",
      description: "tx-2",
      value: 100,
    });
  });

  it("respeita o contexto somente leitura", () => {
    expect(resolveReceivableAction(chargeRow, { readOnly: true })).toEqual({
      kind: "blocked",
      reason: "read-only",
    });
  });

  it("bloqueia linha sem identificador", () => {
    expect(resolveReceivableAction({ value: 10 })).toEqual({ kind: "blocked", reason: "missing-id" });
  });
});
