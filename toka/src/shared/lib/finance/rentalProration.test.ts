import { describe, expect, it } from "vitest";

import {
  addMonthsClamped,
  billingMonths,
  daysBetweenInclusive,
  DEFAULT_LEASE_TERM_MONTHS,
  endOfMonthISO,
  firstChargeCents,
  firstMonthProRata,
  lastMonthResidual,
  wholeMonthsBetween,
} from "./rentalProration";

/**
 * Os números batem com os testes do domínio no backend
 * (`rental_charge_config_test.go`, política actual/actual): se um lado mudar sem
 * o outro, o operador veria no plano um valor diferente do que o financeiro
 * cobra.
 */
describe("rentalProration — espelha o domínio do backend", () => {
  const monthly = 100_000; // R$ 1.000,00 em centavos

  it("pró-rata do primeiro mês (actual/actual)", () => {
    expect(firstMonthProRata("2026-04-10", monthly).cents).toBe(70_000);
    expect(firstMonthProRata("2026-02-01", monthly).cents).toBe(100_000);
    expect(firstMonthProRata("2026-02-15", monthly).cents).toBe(50_000);
    expect(firstMonthProRata("2028-02-15", monthly).cents).toBe(51_724);
    expect(firstMonthProRata("2026-03-01", monthly).cents).toBe(100_000);
  });

  it("residual do último mês (actual/actual)", () => {
    expect(lastMonthResidual("2026-05-10", monthly).cents).toBe(32_258);
    expect(lastMonthResidual("2026-02-15", monthly).cents).toBe(53_571);
    expect(lastMonthResidual("2026-02-28", monthly).cents).toBe(100_000);
    expect(lastMonthResidual("2026-02-27", monthly).cents).toBe(96_428);
  });

  it("explica a base em dias usada no cálculo", () => {
    expect(firstMonthProRata("2026-04-10", monthly)).toEqual({ cents: 70_000, days: 21, daysInMonth: 30 });
    expect(lastMonthResidual("2026-05-10", monthly)).toEqual({ cents: 32_258, days: 10, daysInMonth: 31 });
    expect(lastMonthResidual("2028-02-29", monthly)).toEqual({ cents: 100_000, days: 29, daysInMonth: 29 });
  });

  it("primeira cobrança conforme o tratamento escolhido", () => {
    // Pró-rata do mês inicial, isolado.
    expect(firstChargeCents("2026-04-10", "2029-10-10", monthly, "same_month_pro_rata")).toBe(70_000);
    // Acumulado: pró-rata + mês seguinte integral.
    expect(firstChargeCents("2026-04-10", "2029-10-10", monthly, "next_month_accumulated")).toBe(170_000);
    // Acumulado com término dentro do mês seguinte: o mês seguinte vem residual.
    expect(firstChargeCents("2026-04-10", "2026-05-20", monthly, "next_month_accumulated")).toBe(70_000 + 64_516);
  });

  it("soma meses ancorando no último dia do mês alvo", () => {
    expect(addMonthsClamped("2026-10-02", 30)).toBe("2029-04-02");
    expect(addMonthsClamped("2026-01-31", 1)).toBe("2026-02-28");
    expect(addMonthsClamped("2028-01-31", 1)).toBe("2028-02-29");
    // O caso que o `setUTCMonth` estourava: 31/08 + 30 meses vazava para 03/03.
    expect(addMonthsClamped("2026-08-31", 30)).toBe("2029-02-28");
    expect(addMonthsClamped("2026-03-15", 12)).toBe("2027-03-15");
  });

  it("conta meses inteiros entre as datas para manter prazo e término coerentes", () => {
    expect(wholeMonthsBetween("2026-10-02", addMonthsClamped("2026-10-02", 30))).toBe(30);
    expect(wholeMonthsBetween("2026-10-02", "2029-03-31")).toBe(29);
    expect(wholeMonthsBetween("2026-01-31", "2026-02-28")).toBe(1);
    expect(wholeMonthsBetween("2026-10-02", "2026-09-30")).toBe(0);
  });

  it("conta meses de cobrança e dias do contrato", () => {
    expect(billingMonths("2026-10-02", "2029-04-02")).toBe(31);
    expect(billingMonths("2026-10-02", "2026-10-02")).toBe(1);
    expect(daysBetweenInclusive("2026-10-02", "2026-10-02")).toBe(1);
    expect(daysBetweenInclusive("2026-10-02", "2026-10-31")).toBe(30);
    expect(daysBetweenInclusive("2026-10-02", "2026-10-01")).toBe(0);
  });

  it("rejeita data inválida sem quebrar a tela", () => {
    expect(addMonthsClamped("2026-02-30", 1)).toBe("");
    expect(addMonthsClamped("", 1)).toBe("");
    expect(firstMonthProRata("", monthly).cents).toBe(0);
    expect(endOfMonthISO("2026-02-01")).toBe("2026-02-28");
    expect(endOfMonthISO("2028-02-01")).toBe("2028-02-29");
  });

  it("prazo padrão do contrato é 30 meses", () => {
    expect(DEFAULT_LEASE_TERM_MONTHS).toBe(30);
    expect(addMonthsClamped("2026-10-02", DEFAULT_LEASE_TERM_MONTHS)).toBe("2029-04-02");
  });
});
