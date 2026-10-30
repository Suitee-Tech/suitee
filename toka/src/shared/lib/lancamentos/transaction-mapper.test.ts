import { describe, it, expect } from "vitest";

import { transactionToLancamento, mapLancamentos } from "./mappers";
import { aggregatePagadores } from "../../components/lancamentos/PagadoresView";
import type { TransactionDTO } from "../../services/transaction.service";

const baseTx: TransactionDTO = {
  id: "tx-1",
  contract_id: "",
  type: "receita",
  description: "Recebimento avulso",
  value: 500,
  date: "2026-06-01T00:00:00Z",
  status: "Pendente",
  payer_name: "Cliente X",
  payer_kind: "external",
  tenant_id: "t1",
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
};

describe("transactionToLancamento", () => {
  it("maps a manual receita to an entrada transaction lancamento", () => {
    const l = transactionToLancamento(baseTx);
    expect(l.kind).toBe("transaction");
    expect(l.kind_data.direction).toBe("entrada");
    expect(l.kind_data.manual).toBe(true);
    expect(l.status).toBe("pendente");
    expect(l.counterparty.label).toBe("Cliente X");
    expect(l.counterparty.kind).toBe("external");
    expect(l.amount).toBe(500);
    expect(l.timeline_summary.divergence_flag).toBe(false);
  });

  it("maps a despesa to a saida and Pago->pago with settled_at", () => {
    const l = transactionToLancamento({
      ...baseTx,
      id: "tx-2",
      type: "despesa",
      status: "Pago",
      payment_date: "2026-06-05T00:00:00Z",
    });
    expect(l.kind_data.direction).toBe("saida");
    expect(l.status).toBe("pago");
    expect(l.settled_at).toBe("2026-06-05T00:00:00Z");
  });

  it("keeps a canonical PAID transaction paid instead of reopening it", () => {
    const l = transactionToLancamento({
      ...baseTx,
      id: "tx-paid-canonical",
      status: "PAID",
      payment_date: "2026-06-05T00:00:00Z",
    });

    expect(l.status).toBe("pago");
    expect(l.settled_at).toBe("2026-06-05T00:00:00Z");
  });

  it("treats a contract-linked tx as non-manual with contract origin", () => {
    const l = transactionToLancamento({ ...baseTx, id: "tx-3", contract_id: "c1" });
    expect(l.kind_data.manual).toBe(false);
    expect(l.origin.entity_kind).toBe("contract");
    expect(l.origin.entity_id).toBe("c1");
  });
});

describe("aggregatePagadores", () => {
  it("buckets receber/pagar/pendente by counterparty", () => {
    const lancs = mapLancamentos({
      transactions: [
        baseTx, // receita 500 pendente, Cliente X
        {
          ...baseTx,
          id: "tx-2",
          type: "despesa",
          value: 200,
          status: "Pago",
          payer_name: "Fornecedor Y",
          payment_date: "2026-06-05T00:00:00Z",
        },
      ],
    });
    const groups = aggregatePagadores(lancs);
    const x = groups.find((g) => g.label === "Cliente X");
    const y = groups.find((g) => g.label === "Fornecedor Y");
    expect(x?.receber).toBe(500);
    expect(x?.pendente).toBe(500);
    expect(y?.pagar).toBe(200);
    expect(y?.pendente).toBe(0); // Pago is settled, not pending
  });
});
