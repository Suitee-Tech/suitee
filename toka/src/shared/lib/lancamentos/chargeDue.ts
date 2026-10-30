import type { Charge } from '../../services/charge.service';
import {
  CHARGE_DUE_TONE_STYLE,
  chargeDueLabel,
  type ChargeDueLabel,
} from '../../utils/chargeDueLabel';
import { statusBadgeVariant, statusLabel, type StatusBadgeVariant } from './status';
import type { Lancamento } from './types';

/**
 * Vencimento de uma linha de COBRANÇA na lista de lançamentos do financeiro.
 *
 * O lançamento guarda o `Charge` original em `source` (ver
 * `lib/lancamentos/mappers.ts`): é dele que sai o `days_overdue` calculado pelo
 * backend. O `due_at` do lançamento é a data que a linha mostra, e serve de
 * fallback quando o payload não trouxe o campo. Devolve `null` para os outros
 * tipos de lançamento (comissão, repasse, parceria, caixa) — eles têm régua
 * própria e não devem passar pela régua da cobrança.
 */
export function lancamentoChargeDue(lancamento: Lancamento): ChargeDueLabel | null {
  if (lancamento.kind !== 'charge') return null;
  const source = lancamento.source as Charge | null | undefined;
  return chargeDueLabel({
    due_date: source?.due_date ?? lancamento.due_at,
    status: source?.status ?? lancamento.status,
    days_overdue: source?.days_overdue,
  });
}

/**
 * Texto do badge de situação. Cobrança passa pela régua de vencimento ("Vence
 * hoje" / "Venceu há N dias"); os demais lançamentos seguem o rótulo canônico.
 */
export function lancamentoSituationLabel(lancamento: Lancamento): string {
  return lancamentoChargeDue(lancamento)?.label ?? statusLabel(lancamento.status);
}

/**
 * Estilo do badge. Fora do atraso vale o visual canônico do status — só duas
 * situações o substituem: atraso de verdade (crítico) e o dia do vencimento
 * (atenção, sem tratar como atraso).
 */
export function lancamentoSituationVariant(lancamento: Lancamento): StatusBadgeVariant {
  const canonical = statusBadgeVariant(lancamento.status);
  const due = lancamentoChargeDue(lancamento);
  if (!due || (due.tone !== 'overdue' && due.tone !== 'today')) return canonical;
  const style = CHARGE_DUE_TONE_STYLE[due.tone];
  return {
    bg: style.bg,
    text: style.text,
    border: style.border,
    iconName: due.tone === 'overdue' ? 'alert' : 'clock',
  };
}
