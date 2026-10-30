import { describe, expect, it } from 'vitest';

import type { Charge } from '../../services/charge.service';
import type { Lancamento } from './types';
import { lancamentoChargeDue, lancamentoSituationLabel, lancamentoSituationVariant } from './chargeDue';

const lancamento = (overrides: Partial<Lancamento> = {}, source: Partial<Charge> = {}): Lancamento =>
  ({
    id: 'charge-1',
    kind: 'charge',
    status: 'vencido',
    amount: 1200,
    currency: 'BRL',
    due_at: '2026-10-01T00:00:00Z',
    counterparty: { kind: 'tenant', id: 'parte-1', label: 'Aluguel 10/2026' },
    origin: { entity_kind: 'contract', entity_id: 'ctr-1' },
    timeline_summary: {},
    permissions_required: ['finance:read'],
    created_at: '2026-10-01T10:00:00Z',
    updated_at: '2026-10-01T10:00:00Z',
    kind_data: { reminders_count: 0 },
    source: { id: 'charge-1', status: 'OVERDUE', due_date: '2026-10-01T00:00:00Z', ...source },
    ...overrides,
  }) as unknown as Lancamento;

describe('badge de situação da cobrança na lista de lançamentos', () => {
  it('usa o days_overdue do backend no texto', () => {
    expect(lancamentoSituationLabel(lancamento({}, { days_overdue: 12 }))).toBe('Venceu há 12 dias');
    expect(lancamentoSituationLabel(lancamento({}, { days_overdue: 1 }))).toBe('Venceu ontem');
  });

  it('vence hoje NÃO é atraso: o tom é de atenção, não o crítico do atraso', () => {
    const hoje = new Date();
    const dueDate = `${new Date(Date.UTC(hoje.getFullYear(), hoje.getMonth(), hoje.getDate())).toISOString().slice(0, 10)}T00:00:00Z`;
    const item = lancamento({ status: 'pendente' }, { status: 'PENDING', due_date: dueDate, days_overdue: 0 });

    expect(lancamentoSituationLabel(item)).toBe('Vence hoje');
    const variant = lancamentoSituationVariant(item);
    expect(variant.bg).toBe('bg-status-warning-soft');
    expect(variant.text).toBe('text-status-warning');
  });

  it('atraso de verdade pinta o badge de crítico', () => {
    const variant = lancamentoSituationVariant(lancamento({}, { days_overdue: 4 }));

    expect(variant.bg).toBe('bg-status-critical-soft');
    expect(variant.text).toBe('text-status-critical');
    expect(variant.iconName).toBe('alert');
  });

  it('payload antigo sem days_overdue cai no vencimento; cobrança paga segue "Pago"', () => {
    const semDias = lancamento({ status: 'vencido', due_at: '2020-01-01T00:00:00Z' }, { due_date: '2020-01-01T00:00:00Z' });
    expect(lancamentoChargeDue(semDias)?.overdue).toBe(true);
    expect(lancamentoSituationLabel(semDias)).toMatch(/^Venceu há \d+ dias$/);

    const paga = lancamento({ status: 'pago' }, { status: 'PAID' });
    expect(lancamentoSituationLabel(paga)).toBe('Pago');
    expect(lancamentoSituationVariant(paga).bg).toBe('bg-status-ok-soft');
  });

  it('não interfere nos outros tipos de lançamento (régua própria)', () => {
    const comissao = lancamento({ kind: 'commission', status: 'aprovado' });

    expect(lancamentoChargeDue(comissao)).toBeNull();
    expect(lancamentoSituationLabel(comissao)).toBe('Aprovado');
  });
});
