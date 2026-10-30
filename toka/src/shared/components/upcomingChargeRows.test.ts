import { describe, it, expect } from 'vitest';
import { upcomingChargeRows } from './ContractRentalTab';
import type { Charge } from '../services/charge.service';

/**
 * A regressão relatada na tela de contrato: a MESMA cobrança aparecia como
 * "Em Atraso · Vencimento 09/10" na lista de próximas cobranças e como
 * "Pendente · Vencimento 10/10" na tabela do plano financeiro.
 *
 * Duas causas, as duas travadas aqui:
 *  1. data gravada como meia-noite UTC formatada no fuso do navegador (UTC-3 no
 *     Brasil mostra o dia anterior);
 *  2. estado decidido por comparação de INSTANTES, que marca atraso na cobrança
 *     que vence no próprio dia.
 */
const cobranca = (over: Partial<Charge>): Charge =>
  ({
    id: 'ch-1',
    tenant_id: 't-1',
    contract_id: 'ctr-1',
    tenant_party_id: 'parte-1',
    owner_id: 'dono-1',
    property_id: 'im-1',
    type: 'RENT',
    description: 'Aluguel + Encargos',
    reference_month: '2026-10',
    amount: 2846.77,
    total_amount: 2846.77,
    status: 'PENDING',
    due_date: '2026-10-10T00:00:00Z',
    created_at: '2026-10-01T00:00:00Z',
    updated_at: '2026-10-01T00:00:00Z',
    ...over,
  }) as Charge;

/** Data no calendário local, no formato que a API usa (meia-noite UTC). */
const localISO = (d: Date): string =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
const vencimentoEmDias = (dias: number): string => {
  const d = new Date();
  d.setDate(d.getDate() + dias);
  return `${localISO(d)}T00:00:00Z`;
};

describe('próximas cobranças do contrato', () => {
  it('mostra o dia do vencimento como gravado, não deslocado pelo fuso', () => {
    // Cobrança que vence HOJE: o dia gravado é hoje, e é hoje que deve aparecer.
    const hoje = localISO(new Date());
    const [linha] = upcomingChargeRows([cobranca({ due_date: `${hoje}T00:00:00Z` })]);

    const [ano, mes, dia] = hoje.split('-');
    expect(linha.dueDateLabel).toBe(`${dia}/${mes}/${ano}`);
  });

  it('cobrança que vence hoje não é atraso', () => {
    const [linha] = upcomingChargeRows([cobranca({ due_date: vencimentoEmDias(0) })]);

    expect(linha.label).toBe('Vence hoje');
    // Nem pelo rótulo antigo nem pelo estado calculado por instante.
    expect(linha).not.toHaveProperty('overdue', true);
  });

  it('cobrança de ontem aparece como vencida, com o dia certo', () => {
    const [linha] = upcomingChargeRows([cobranca({ due_date: vencimentoEmDias(-1) })]);

    expect(linha.label).toBe('Venceu ontem');
    const ontem = new Date();
    ontem.setDate(ontem.getDate() - 1);
    const [ano, mes, dia] = localISO(ontem).split('-');
    expect(linha.dueDateLabel).toBe(`${dia}/${mes}/${ano}`);
  });

  it('respeita o atraso informado pelo backend e ordena por vencimento', () => {
    const linhas = upcomingChargeRows([
      cobranca({ id: 'ch-2', due_date: vencimentoEmDias(5) }),
      cobranca({ id: 'ch-1', due_date: vencimentoEmDias(-3), status: 'OVERDUE', days_overdue: 3 }),
    ]);

    expect(linhas.map((l) => l.id)).toEqual(['ch-1', 'ch-2']);
    expect(linhas[0].label).toBe('Venceu há 3 dias');
    expect(linhas[1].label).toMatch(/^Vence em 5 dias$/);
  });

  it('ignora cobranças fechadas e limita a lista', () => {
    const muitas = Array.from({ length: 9 }, (_, i) =>
      cobranca({ id: `ch-${i}`, due_date: vencimentoEmDias(i + 1) }),
    );
    muitas.push(cobranca({ id: 'ch-paga', status: 'PAID', due_date: vencimentoEmDias(-10) }));

    const linhas = upcomingChargeRows(muitas, 6);

    expect(linhas).toHaveLength(6);
    expect(linhas.some((l) => l.id === 'ch-paga')).toBe(false);
  });
});
