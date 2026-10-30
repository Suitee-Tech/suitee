import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { PropsWithChildren } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { usePortalDealChecklist } from "./useDealResources";

const mocks = vi.hoisted(() => ({ getDealChecklist: vi.fn() }));
vi.mock("../services/portal.service", () => ({ portalService: mocks }));
vi.mock("../context/PortalAuthContext", () => ({ usePortalAuth: () => ({ isAuthenticated: true }) }));
afterEach(() => { cleanup(); vi.clearAllMocks(); });

describe("recursos por negócio", () => {
  it("cancela a leitura anterior e não mistura documentos ao trocar de negócio", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    let resolve!: (items: unknown[]) => void;
    let previousSignal!: AbortSignal;
    mocks.getDealChecklist.mockImplementation((id, options) => {
      if (id === "first") {
        previousSignal = options.signal;
        return new Promise(r => { resolve = r; });
      }
      return Promise.resolve([{ id: "second-document", label: "Documento do segundo negócio" }]);
    });
    const wrapper = ({ children }: PropsWithChildren) => <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    const view = renderHook(({ id }) => usePortalDealChecklist(id), { initialProps: { id: "first" }, wrapper });
    await waitFor(() => expect(mocks.getDealChecklist).toHaveBeenCalledTimes(1));
    view.rerender({ id: "second" });
    await waitFor(() => expect(view.result.current.data?.[0].id).toBe("second-document"));
    expect(previousSignal.aborted).toBe(true);
    await act(async () => { resolve([{ id: "first-document" }]); });
    expect(view.result.current.data?.[0].id).toBe("second-document");
    view.unmount();
    client.clear();
  });
});
