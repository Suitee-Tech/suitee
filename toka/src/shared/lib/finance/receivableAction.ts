import type { Transaction } from "../../types/transaction";

/**
 * Linha de recebível como ela chega da tela: o registro de domínio mais os
 * campos de apresentação que a conciliação anexa (origem, cobrança vinculada,
 * valor/ds exibidos e a decomposição do saldo). É um view-model próprio — não
 * estende `Transaction` porque a linha de cobrança não carrega os mesmos campos
 * e a projeção de fatura pode trazer o vínculo nulo.
 */
export interface ReceivableRow {
  id?: string | null;
  /** Tipo do lançamento quando a linha é um registro de domínio. */
  type?: Transaction["type"] | string;
  description?: string;
  value?: number | string | null;
  recordSource?: "transaction" | "invoice";
  linkedChargeID?: string | null;
  /** Cobrança dona da linha, quando a própria linha é a cobrança. */
  _chargeId?: string | null;
  /** Campos de apresentação das tabelas de cobranças/lançamentos. */
  _source?: "charge" | "invoice" | "transaction";
  desc?: string;
  valor?: number | string | null;
  kind_data?: { transaction_type?: string };
}

export type ReceivableAction =
  /** Cobrança é o agregado canônico: baixa de cobrança (valor, destinos,
   *  comprovante). Vale para a linha da cobrança e para a projeção de fatura
   *  vinculada a ela — mesma ação, mesmo painel. */
  | { kind: "charge"; chargeId: string; defaultAmount: number }
  /** Lançamento independente: baixa padronizada (método + data + comprovante). */
  | { kind: "settle"; id: string; type: string; description: string; value: number }
  /** Sem baixa possível — a tela orienta em vez de abrir um formulário recusado. */
  | { kind: "blocked"; reason: "unresolved-invoice" | "read-only" | "missing-id" };

export interface ReceivableActionOptions {
  /** Saldo em aberto da linha (cobrança parcial pré-preenche o restante). */
  defaultAmount?: number;
  /** Contexto somente leitura (ex.: aba de repasses "a apurar"). */
  readOnly?: boolean;
}

/**
 * Decide UMA vez, para todo o módulo financeiro, onde a baixa de um recebível
 * acontece. Antes cada tela decidia por conta própria e a mesma ação aparecia
 * como AlertDialog em uma e como formulário em outra.
 *
 * Invariantes:
 *  - cobrança (ou fatura vinculada a uma) nunca vira baixa de lançamento;
 *  - fatura sem cobrança vinculada é bloqueada: o backend recusa quitar fatura
 *    sem vínculo (`ErrInvoiceChargeLinkRequired`), então pedir dados ao
 *    operador ali seria enganoso;
 *  - nenhuma origem resulta em troca de status por AlertDialog.
 */
export function resolveReceivableAction(
  row: ReceivableRow,
  options: ReceivableActionOptions = {},
): ReceivableAction {
  const id = typeof row.id === "string" ? row.id.trim() : "";
  if (!id) return { kind: "blocked", reason: "missing-id" };

  if (options.readOnly) return { kind: "blocked", reason: "read-only" };

  const rawChargeId = row._chargeId ?? row.linkedChargeID ?? "";
  const chargeId = typeof rawChargeId === "string" ? rawChargeId.trim() : "";
  const value = numberOrZero(row.valor ?? row.value);
  if (chargeId) {
    return { kind: "charge", chargeId, defaultAmount: options.defaultAmount ?? value };
  }

  // Fatura/projeção sem vínculo: a API de fatura só quita com cobrança ligada.
  if (row.recordSource === "invoice" || row._source === "invoice") {
    return { kind: "blocked", reason: "unresolved-invoice" };
  }

  const description = row.desc || row.description || id;
  return {
    kind: "settle",
    id,
    type: row.kind_data?.transaction_type || row.type || "receita",
    description,
    value,
  };
}

const numberOrZero = (value: unknown): number => {
  const parsed = typeof value === "number" ? value : Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
};
