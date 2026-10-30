import { describe, expect, it } from 'vitest';

import { CHARGE_DUE_TONE_CLASS, chargeDueLabel, chargeDueLabelText } from './chargeDueLabel';

/** Data de vencimento como o backend grava: o DIA em UTC, à meia-noite. */
const dueInDays = (days: number, now: Date) => {
  const day = new Date(Date.UTC(now.getFullYear(), now.getMonth(), now.getDate()) + days * 86_400_000);
  return `${day.toISOString().slice(0, 10)}T00:00:00Z`;
};

/** Meio-dia local: evita que o próprio teste caia na virada do dia. */
const NOW = new Date(2026, 9, 10, 12, 0, 0);

describe('chargeDueLabel — a régua "vence no dia, atrasa no dia seguinte"', () => {
  it('vence hoje não é atraso: o rótulo é "Vence hoje" e o tom é de atenção, não de crítico', () => {
    const result = chargeDueLabel({ due_date: dueInDays(0, NOW), status: 'PENDING', now: NOW });

    expect(result.label).toBe('Vence hoje');
    expect(result.tone).toBe('today');
    expect(result.overdue).toBe(false);
    expect(result.daysOverdue).toBe(0);
    // O backend devolve 0 para "vence hoje" — o rótulo não pode virar "Venceu".
    expect(chargeDueLabel({ due_date: dueInDays(0, NOW), status: 'OVERDUE', days_overdue: 0, now: NOW }).label).toBe(
      'Vence hoje',
    );
  });

  it('venceu ontem é o primeiro dia de atraso', () => {
    const result = chargeDueLabel({ due_date: dueInDays(-1, NOW), status: 'OVERDUE', now: NOW });

    expect(result.label).toBe('Venceu ontem');
    expect(result.tone).toBe('overdue');
    expect(result.overdue).toBe(true);
    expect(result.daysOverdue).toBe(1);
  });

  it('dez dias de atraso viram "Venceu há 10 dias"', () => {
    const result = chargeDueLabel({ due_date: dueInDays(-10, NOW), status: 'OVERDUE', now: NOW });

    expect(result.label).toBe('Venceu há 10 dias');
    expect(result.daysOverdue).toBe(10);
    expect(result.overdue).toBe(true);
  });

  it('usa o days_overdue do backend quando ele vem (mesmo sem data no payload)', () => {
    const result = chargeDueLabel({ status: 'OVERDUE', days_overdue: 12 });

    expect(result.label).toBe('Venceu há 12 dias');
    expect(result.tone).toBe('overdue');
    expect(result.daysOverdue).toBe(12);
  });

  it('o campo do backend manda no atraso: days_overdue 0 não vira atrasado por fuso do navegador', () => {
    // Navegador um dia à frente do produto (UTC-3 x UTC): a data "já passou"
    // para o cliente, mas o servidor diz que a cobrança ainda está no dia dela.
    const result = chargeDueLabel({ due_date: dueInDays(-1, NOW), status: 'OVERDUE', days_overdue: 0, now: NOW });

    expect(result.label).toBe('Vence hoje');
    expect(result.overdue).toBe(false);
  });

  it('vencimento futuro conta os dias à frente, com "amanhã" para o dia seguinte', () => {
    expect(chargeDueLabelText({ due_date: dueInDays(3, NOW), status: 'PENDING', now: NOW })).toBe('Vence em 3 dias');
    expect(chargeDueLabelText({ due_date: dueInDays(1, NOW), status: 'PENDING', now: NOW })).toBe('Vence amanhã');
    expect(chargeDueLabelText({ due_date: dueInDays(30, NOW), status: 'PENDING', now: NOW })).toBe('Vence em 30 dias');
  });

  it('status fechado nunca é atrasado, mesmo com data no passado', () => {
    const pago = chargeDueLabel({ due_date: dueInDays(-9, NOW), status: 'PAID', days_overdue: 9, now: NOW });
    expect(pago.label).toBe('Pago');
    expect(pago.tone).toBe('settled');
    expect(pago.overdue).toBe(false);

    const cancelada = chargeDueLabel({ due_date: dueInDays(-9, NOW), status: 'CANCELLED', now: NOW });
    expect(cancelada.label).toBe('Cancelada');
    expect(cancelada.tone).toBe('settled');
    expect(cancelada.overdue).toBe(false);

    const reversada = chargeDueLabel({ due_date: dueInDays(-9, NOW), status: 'REVERSED', now: NOW });
    expect(reversada.label).toBe('Reversada');
    expect(reversada.tone).toBe('settled');
    expect(reversada.overdue).toBe(false);
  });

  it('aceita o status já normalizado pelo frontend', () => {
    expect(chargeDueLabel({ due_date: dueInDays(-2, NOW), status: 'vencido', now: NOW }).label).toBe('Venceu há 2 dias');
    expect(chargeDueLabel({ due_date: dueInDays(-2, NOW), status: 'pago', now: NOW }).label).toBe('Pago');
  });

  it('payload antigo sem days_overdue e sem data não quebra: cai no rótulo do status', () => {
    const semData = chargeDueLabel({ status: 'OVERDUE' });
    expect(semData.label).toBe('Vencido');
    expect(semData.tone).toBe('unknown');
    expect(semData.overdue).toBe(false);

    expect(chargeDueLabel({}).label).toBe('Pendente');
    expect(chargeDueLabel({ due_date: 'data-invalida', status: 'PENDING' }).label).toBe('Pendente');
  });

  it('aceita data sem hora ("2026-10-10") com o mesmo dia civil', () => {
    expect(chargeDueLabelText({ due_date: '2026-10-10', status: 'PENDING', now: NOW })).toBe('Vence hoje');
    expect(chargeDueLabelText({ due_date: '2026-10-09', status: 'OVERDUE', now: NOW })).toBe('Venceu ontem');
  });

  it('todo tom tem classe de badge em token do design system', () => {
    expect(CHARGE_DUE_TONE_CLASS.overdue).toContain('status-critical');
    expect(CHARGE_DUE_TONE_CLASS.today).toContain('status-warning');
    expect(CHARGE_DUE_TONE_CLASS.upcoming).toContain('status-info');
    expect(CHARGE_DUE_TONE_CLASS.settled).toContain('status-ok');
  });
});
