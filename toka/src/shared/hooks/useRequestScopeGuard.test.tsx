import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useRequestScopeGuard } from "./useRequestScopeGuard";

describe("useRequestScopeGuard", () => {
  it("invalidates in-flight work when the tenant scope changes", () => {
    const { result, rerender } = renderHook(
      ({ companyId }) => useRequestScopeGuard(companyId),
      { initialProps: { companyId: "company-a" } },
    );

    const requestFromCompanyA = result.current();
    expect(requestFromCompanyA()).toBe(true);

    rerender({ companyId: "company-b" });

    expect(requestFromCompanyA()).toBe(false);
    expect(result.current()()).toBe(true);
  });

  it("does not let a callback retained from the previous scope start valid work", () => {
    const { result, rerender } = renderHook(
      ({ companyId }) => useRequestScopeGuard(companyId),
      { initialProps: { companyId: "company-a" } },
    );

    const beginFromCompanyA = result.current;
    rerender({ companyId: "company-b" });

    expect(beginFromCompanyA()()).toBe(false);
    expect(result.current()()).toBe(true);
  });

  it("invalidates work when its provider unmounts", () => {
    const { result, unmount } = renderHook(() => useRequestScopeGuard("company-a"));
    const request = result.current();

    unmount();

    expect(request()).toBe(false);
  });
});
