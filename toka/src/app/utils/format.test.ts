import { describe, expect, it } from "vitest";
import { currencyToWordsBRL, formatCurrencyBRL, formatISODateBR, numberToWordsPTBR } from "./format";

describe("format utils", () => {
	it("formatCurrencyBRL formats numbers in pt-BR", () => {
		expect(formatCurrencyBRL(0)).toBe("R$ 0,00");
	});

	it("formatISODateBR formats ISO dates in UTC", () => {
		expect(formatISODateBR("2026-01-02")).toBe("02/01/2026");
	});

	it("numberToWordsPTBR converts numbers to words", () => {
		expect(numberToWordsPTBR(21)).toBe("vinte e um");
	});

	it("currencyToWordsBRL converts currency to words", () => {
		expect(currencyToWordsBRL(10.5)).toBe("Dez reais e cinquenta centavos");
	});
});

