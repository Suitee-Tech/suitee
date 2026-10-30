import type { Charge } from '../../services/charge.service';
import type { CreateCollectionDTO } from '../../services/collection.service';
import { chargeDueLabel } from '../../utils/chargeDueLabel';

/**
 * Tradução de uma COBRANÇA vencida (`GET /charges/overdue`) no corpo de
 * `POST /collections` — o "Abrir caso" da faixa de cobranças sem caso aberto.
 *
 * Por que existe: o botão da linha cria dívida FORMAL e o backend valida o que
 * chega (F0.7): `total_debt` tem de bater com a soma das cobranças informadas
 * (±1 centavo) e uma cobrança que já está em caso aberto responde 409. Montar o
 * corpo aqui, a partir da própria linha, mantém uma única conta — o mesmo valor
 * que o operador está vendo na tabela — em vez de espalhar a regra pelo JSX.
 */

const round2 = (value: number): number => Math.round(value * 100) / 100;

/**
 * Saldo em aberto da cobrança — exatamente o número exibido na linha.
 *
 * `total_amount` (multa/juros/desconto já aplicados; `amount` cru como fallback
 * de payload antigo) MENOS `paid_amount` quando houve pagamento parcial: a
 * validação do backend soma `TotalAmount − PaidAmount` de cada cobrança
 * (`collection_service.go`, tolerância de 1 centavo) e `GET /charges/overdue`
 * devolve cobranças `PARTIALLY_PAID` — enviar o valor cheio delas reprovaria a
 * abertura do caso com 400.
 */
export function chargeOutstandingAmount(
  charge: Pick<Charge, 'total_amount' | 'amount' | 'paid_amount'>,
): number {
  const gross = charge.total_amount ?? charge.amount;
  const paid =
    typeof charge.paid_amount === 'number' && Number.isFinite(charge.paid_amount) ? charge.paid_amount : 0;
  const base = typeof gross === 'number' && Number.isFinite(gross) ? gross : 0;
  return Math.max(round2(base - paid), 0);
}

/**
 * O caso só pode nascer com CONTRATO e DEVEDOR: sem qualquer um dos dois o
 * backend não teria a quem cobrar. Nesses itens o botão não aparece (o id da
 * cobrança, sozinho, não identifica o devedor).
 */
export function canOpenCaseFromCharge(charge: Charge): boolean {
  if (!charge.id || !charge.contract_id?.trim() || !charge.tenant_party_id?.trim()) return false;
  // Sem atraso não há caso a abrir: o backend recusa ("cobrança ainda não venceu")
  // e a dívida não está vencida. Cobre o resíduo de cobrança marcada como vencida
  // no próprio dia por uma regra antiga, que o motor devolve para pendente.
  return overdueDaysOf(charge) >= 1;
}

/** Dias de atraso da linha: do backend quando vem, senão derivados do rótulo. */
function overdueDaysOf(charge: Charge): number {
  if (typeof charge.days_overdue === 'number' && Number.isFinite(charge.days_overdue)) {
    return Math.trunc(charge.days_overdue);
  }
  return chargeDueLabel(charge).daysOverdue;
}

/**
 * Corpo de `POST /collections` para a cobrança da linha — `null` quando a linha
 * não tem contrato/devedor (botão escondido).
 *
 * O `days_overdue` vem da mesma régua do rótulo: `days_overdue` do backend
 * quando existe, senão derivado do vencimento; e nunca abaixo de 1 — o caso é
 * criado porque a cobrança está atrasada, então 0 ("vence hoje") não faz sentido
 * no corpo e é normalizado para 1.
 */
export function openCasePayloadFromCharge(charge: Charge): CreateCollectionDTO | null {
  if (!canOpenCaseFromCharge(charge)) return null;

  const daysOverdue = overdueDaysOf(charge);

  const debtorId = charge.tenant_party_id.trim();
  return {
    contract_id: charge.contract_id.trim(),
    debtor_id: debtorId,
    // O nome resolvido pelo CRM pode faltar (a busca falha em silêncio): o id do
    // inquilino é o fallback que já aparece na tela do caso.
    debtor_name: charge.debtor_name?.trim() || debtorId,
    total_debt: chargeOutstandingAmount(charge),
    days_overdue: daysOverdue,
    charge_ids: [charge.id],
  };
}
