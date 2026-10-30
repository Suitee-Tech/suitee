import { describe, expect, it } from 'vitest';
import {
  formatContractDateValue,
  isContractDateVariable,
  parseContractDateVariable,
  resolveContractDateVariable,
} from './contractDateVariables';

describe('formatContractDateValue', () => {
  it('formats the same calendar date as BR, ISO and written Portuguese', () => {
    expect(formatContractDateValue('2029-03-18', 'br')).toBe('18/03/2029');
    expect(formatContractDateValue('2029-03-18', 'written')).toBe('18 de março de 2029');
    expect(formatContractDateValue('18/03/2029', 'iso')).toBe('2029-03-18');
    expect(formatContractDateValue('18/03/2029', 'br')).toBe('18/03/2029');
  });

  it('preserves the explicit timestamp date irrespective of its time offset', () => {
    expect(formatContractDateValue('2029-03-18T00:00:00.000Z', 'br')).toBe('18/03/2029');
    expect(formatContractDateValue('2029-03-18T23:59:59-03:00', 'written')).toBe('18 de março de 2029');
    expect(formatContractDateValue('2029-03-18T00:00:00+09:00', 'iso')).toBe('2029-03-18');
  });

  it('accepts valid Date objects using their existing calendar semantics', () => {
    expect(formatContractDateValue(new Date(2029, 2, 18), 'br')).toBe('18/03/2029');
    expect(formatContractDateValue(new Date(Number.NaN), 'br')).toBeUndefined();
  });

  it.each(['2029-02-29', '31/04/2029', '2029-03-18Tinvalid', '18 de março de 2029', 'Data acordada', '', null, undefined, 0, false, {}])(
    'does not invent a date for invalid or missing value %s', (value) => {
      expect(formatContractDateValue(value, 'br')).toBeUndefined();
      expect(formatContractDateValue(value, 'written')).toBeUndefined();
    },
  );
});

describe('contract date variable grammar', () => {
  it('recognizes base dates, formats and bounded English operations', () => {
    expect(parseContractDateVariable('current_date_plus_30_days')).toEqual({
      baseKey: 'current_date', direction: 'plus', amount: 30, unit: 'days', format: 'br',
    });
    expect(parseContractDateVariable('contract_start_date_minus_2_months_iso')).toEqual({
      baseKey: 'contract_start_date', direction: 'minus', amount: 2, unit: 'months', format: 'iso',
    });
    expect(parseContractDateVariable('rent_first_charge_date_written')).toEqual({
      baseKey: 'rent_first_charge_date', direction: null, amount: 0, unit: null, format: 'written',
    });
    expect(isContractDateVariable('contract_end_date')).toBe(true);
  });

  it.each([
    'current_date_plus_-1_days',
    'current_date_plus_1.5_days',
    'current_date_plus_01_days',
    'current_date_plus_36601_days',
    'current_date_plus_26001_business_days',
    'current_date_plus_1201_months',
    'current_date_plus_101_years',
    'current_date_plus_9007199254740992_days',
    'current_date_plus_3_weeks',
    'current_date_plus_3_days_us',
    'current_date_plus_3_days_plus_2_months',
    'unknown_date_plus_1_days',
    'current_date + 3',
    '__proto___plus_1_days',
  ])('rejects invalid or unbounded expression %s', (key) => {
    expect(isContractDateVariable(key)).toBe(false);
    expect(resolveContractDateVariable({ current_date: '2026-09-17' }, key)).toBeUndefined();
  });
});

describe('resolveContractDateVariable', () => {
  it('adds/subtracts calendar days across years and leap days', () => {
    expect(resolveContractDateVariable({ current_date: '2026-12-31' }, 'current_date_plus_1_days')).toBe('01/01/2027');
    expect(resolveContractDateVariable({ current_date: '01/03/2024' }, 'current_date_minus_1_days_iso')).toBe('2024-02-29');
    expect(resolveContractDateVariable({ current_date: '2026-09-17' }, 'current_date_plus_30_days')).toBe('17/10/2026');
  });

  it('clamps months and years to the last valid day instead of spilling over', () => {
    expect(resolveContractDateVariable({ contract_start_date: '2024-01-31' }, 'contract_start_date_plus_1_months')).toBe('29/02/2024');
    expect(resolveContractDateVariable({ contract_start_date: '2025-03-31' }, 'contract_start_date_minus_1_months')).toBe('28/02/2025');
    expect(resolveContractDateVariable({ contract_start_date: '2024-02-29' }, 'contract_start_date_plus_1_years_iso')).toBe('2025-02-28');
    expect(resolveContractDateVariable({ contract_start_date: '2024-01-31' }, 'contract_start_date_plus_2_months_iso')).toBe('2024-03-31');
  });

  it('skips weekends in both directions and counts from the following day', () => {
    expect(resolveContractDateVariable({ current_date: '2026-09-18' }, 'current_date_plus_1_business_days')).toBe('21/09/2026');
    expect(resolveContractDateVariable({ current_date: '2026-09-21' }, 'current_date_minus_1_business_days')).toBe('18/09/2026');
    expect(resolveContractDateVariable({ current_date: '2026-09-19' }, 'current_date_plus_5_business_days')).toBe('25/09/2026');
    expect(resolveContractDateVariable({ current_date: '2026-09-19' }, 'current_date_plus_0_business_days')).toBe('19/09/2026');
  });

  it('uses a date timestamp calendar component without converting its timezone', () => {
    expect(resolveContractDateVariable({ current_date: '2026-09-17T00:00:00Z' }, 'current_date_plus_1_days_iso')).toBe('2026-09-18');
    expect(resolveContractDateVariable({ current_date: '2026-09-17T23:00:00-03:00' }, 'current_date_plus_1_days_iso')).toBe('2026-09-18');
    expect(resolveContractDateVariable({ current_date: '2026-03-07T23:00:00-05:00' }, 'current_date_plus_2_days_iso')).toBe('2026-03-09');
    expect(resolveContractDateVariable({ current_date: new Date('2026-09-17T00:00:00Z') }, 'current_date_plus_1_days_iso')).toBe('2026-09-18');
  });

  it('renders readable Portuguese dates with English keys', () => {
    expect(resolveContractDateVariable({ current_date: '2026-09-17' }, 'current_date_written')).toBe('17 de setembro de 2026');
    expect(resolveContractDateVariable({ rent_first_charge_date: '2026-09-30' }, 'rent_first_charge_date_plus_1_months_written')).toBe('30 de outubro de 2026');
  });

  it('prefers an edited base date over a stale ISO alias', () => {
    const source = { contract_start_date: '31/03/2026', contract_start_date_iso: '2026-01-31' };
    expect(resolveContractDateVariable(source, 'contract_start_date_plus_1_months_iso')).toBe('2026-04-30');
    expect(resolveContractDateVariable({ contract_start_date_iso: '2026-01-31' }, 'contract_start_date_plus_1_months_iso')).toBe('2026-02-28');
    expect(resolveContractDateVariable({ ...source, contract_start_date: '' }, 'contract_start_date_plus_1_months')).toBeUndefined();
  });

  it.each(['2026-02-29', '31/04/2026', '2026-13-01', '0000-01-01', '99999-01-01', '17/09/26', '2026-09-17Tgarbage', '—', '', 0, null, undefined])(
    'keeps invalid or unavailable base %s unresolved', (value) => {
      expect(resolveContractDateVariable({ current_date: value }, 'current_date_plus_30_days')).toBeUndefined();
    },
  );

  it('rejects absent or inherited bases and results outside the supported calendar', () => {
    expect(resolveContractDateVariable({}, 'current_date_plus_1_days')).toBeUndefined();
    expect(resolveContractDateVariable(Object.create({ current_date: '2026-09-17' }), 'current_date_plus_1_days')).toBeUndefined();
    expect(resolveContractDateVariable({ current_date: '9999-12-31' }, 'current_date_plus_1_days')).toBeUndefined();
    expect(resolveContractDateVariable({ current_date: '0001-01-01' }, 'current_date_minus_1_days')).toBeUndefined();
  });
});
