import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { PropsWithChildren } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { usePortalOverview } from "./usePortalOverview";
const mocks = vi.hoisted(() => ({ getOverview: vi.fn(), handler: null as null | ((type: string, payload?: unknown) => void), onMessage: vi.fn() }));
vi.mock("../services/portal.service", () => ({ portalService: { getOverview: mocks.getOverview } }));
vi.mock("./usePortalWebSocket", () => ({ usePortalWebSocket: () => ({ onMessage: mocks.onMessage }) }));

describe("Portal visit agenda reconciliation", () => {
  beforeEach(() => { vi.clearAllMocks(); mocks.handler = null; mocks.onMessage.mockImplementation((handler) => { mocks.handler = handler; return () => { mocks.handler = null; }; }); });
  afterEach(cleanup);
  it("reloads the authoritative status and slot after an agenda event", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const visit = { date: "2099-10-20", time: "14:00", status: "Aguardando" };
    mocks.getOverview.mockResolvedValue([{ id: "lead", visit_request: visit }]);
    const wrapper = ({ children }: PropsWithChildren) => <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    const { result, unmount } = renderHook(() => usePortalOverview(), { wrapper });
    await waitFor(() => expect(result.current.data?.[0].visit_request?.status).toBe("Aguardando"));
    mocks.getOverview.mockResolvedValue([{ id: "lead", visit_request: { date: "2099-10-21", time: "16:00", status: "Confirmado" } }]);
    act(() => mocks.handler?.("appointment_update"));
    await waitFor(() => expect(result.current.data?.[0].visit_request).toMatchObject({ status: "Confirmado", date: "2099-10-21", time: "16:00" }));
    unmount();
    expect(mocks.handler).toBeNull();
    client.clear();
  });
});
