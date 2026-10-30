import { describe, expect, it } from "vitest";
import { act, renderHook } from "@testing-library/react";

import { useFinancialModals, type FinancialConfirmOptions } from "./useFinancialModals";

describe("useFinancialModals", () => {
  it("inicia idle e sem confirm", () => {
    const { result } = renderHook(() => useFinancialModals());
    expect(result.current.state).toEqual({ modal: "idle" });
    expect(result.current.confirm).toBeNull();
  });

  it("abre e fecha o modal de marcar pago preservando payload", () => {
    const { result } = renderHook(() => useFinancialModals());
    act(() => result.current.openMarkPaid("charge-1", 150, { id: "charge-1" } as any));
    expect(result.current.state).toEqual({
      modal: "markPaid",
      chargeId: "charge-1",
      defaultAmount: 150,
      charge: { id: "charge-1" },
    });
    act(() => result.current.close());
    expect(result.current.state).toEqual({ modal: "idle" });
  });

  it("abre o confirm e fecha", () => {
    const { result } = renderHook(() => useFinancialModals());
    const opts: FinancialConfirmOptions = {
      title: "Confirmar",
      description: "Desc.",
      confirmLabel: "OK",
      onConfirm: () => {},
    };
    act(() => result.current.openConfirm(opts));
    expect(result.current.confirm?.title).toBe("Confirmar");
    act(() => result.current.closeConfirm());
    expect(result.current.confirm).toBeNull();
  });

  it("helpers específicos mapeiam para o estado certo", () => {
    const { result } = renderHook(() => useFinancialModals());
    act(() => result.current.openChargeDetail("c1"));
    expect(result.current.state).toEqual({ modal: "chargeDetail", chargeId: "c1" });
    act(() => result.current.openAudit("entity-1", "Conta"));
    expect(result.current.state).toEqual({ modal: "audit", entityId: "entity-1", label: "Conta" });
  });
});
