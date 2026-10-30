import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useMemo, type PropsWithChildren } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DealDTO } from "../../../shared/services/deal.service";
import { newestDeal } from "../../../shared/deals/revisions";
import { portalKeys } from "../hooks/queryKeys";
import { usePortalDealQuery } from "../hooks/useDeals";
import { PortalDealsProvider, usePortalDeals } from "./PortalDealsContext";

const mocks = vi.hoisted(() => ({ getDeals: vi.fn(), getDealById: vi.fn(), connected: false,
  onMessage: vi.fn(), listeners: new Set<(type: string, payload?: unknown) => void>() }));
vi.mock("../services/portal.service", () => ({ portalService: mocks }));
vi.mock("./PortalAuthContext", () => ({ usePortalAuth: () => ({ isAuthenticated: true }) }));
vi.mock("../hooks/usePortalWebSocket", () => ({ usePortalWebSocket: () => ({ onMessage: mocks.onMessage, isConnected: mocks.connected }) }));
const initial: DealDTO = { id: "deal", tenant_id: "agency", status: "NEGOTIATION", type: "RENT", property_id: "property", customer_id: "client", owner_id: "owner", broker_id: "broker", value: 2000, commission: 0, created_at: "2026-09-17T12:00:00Z", updated_at: "2026-09-17T12:00:00.000000001Z" };
const next: DealDTO = { ...initial, status: "ANALYSIS", updated_at: "2026-09-17T12:00:00.000000002Z" };
const clients: QueryClient[] = [];
function mount() {
 const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
 clients.push(client);
 const wrapper = ({ children }: PropsWithChildren) => <QueryClientProvider client={client}><PortalDealsProvider>{children}</PortalDealsProvider></QueryClientProvider>;
 const hook = renderHook(() => {
  const context = usePortalDeals();
  const query = usePortalDealQuery("deal");
  const selected = useMemo(() => context.getDeal("deal"), [context.getDeal]);
  const detail = selected && query.data ? newestDeal(query.data, selected) : query.data ?? selected;
  return { context, detail, query };
 }, { wrapper });
 return { ...hook, client };
}
beforeEach(() => {
 vi.resetAllMocks(); mocks.listeners.clear(); mocks.connected = false; localStorage.clear();
 mocks.getDeals.mockResolvedValue([initial]); mocks.getDealById.mockResolvedValue(initial);
 mocks.onMessage.mockImplementation((fn) => { mocks.listeners.add(fn); return () => mocks.listeners.delete(fn); });
});
afterEach(() => { cleanup(); clients.splice(0).forEach(c => c.clear()); vi.restoreAllMocks(); });
async function emit(type: string, payload?: unknown) {
 await act(async () => { for (const listener of mocks.listeners) listener(type, payload); });
}
describe("estado compartilhado da jornada do cliente", () => {
 it("atualiza etapa no detalhe, painel e seleção pelo evento real do servidor", async () => {
  const view = mount();
  await waitFor(() => expect(view.result.current.detail?.status).toBe("NEGOTIATION"));
  mocks.getDealById.mockResolvedValue(next); mocks.getDeals.mockResolvedValue([next]);
  await emit("deal_status_changed", { deal_id: "deal", new_status: "ANALYSIS" });
  await waitFor(() => expect(view.result.current.detail?.status).toBe("ANALYSIS"));
  expect(view.result.current.context.activeDeal?.status).toBe("ANALYSIS");
  expect(view.result.current.context.deals[0].status).toBe("ANALYSIS");
 });
 it("a lista mais recente supera um detalhe antigo no cache", async () => {
  const view = mount();
  await waitFor(() => expect(view.result.current.detail?.status).toBe("NEGOTIATION"));
  mocks.getDeals.mockResolvedValue([next]);
  await act(async () => { await view.result.current.context.refreshDeals(); });
  expect(view.client.getQueryData<DealDTO>(portalKeys.deal("deal"))?.status).toBe("NEGOTIATION");
  await waitFor(() => expect(view.result.current.detail?.status).toBe("ANALYSIS"));
  expect(view.result.current.context.activeDeal?.status).toBe("ANALYSIS");
 });
 it("mudanças diretas no cache do detalhe também atualizam os consumidores do contexto", async () => {
  const view = mount();
  await waitFor(() => expect(view.result.current.context.deals).toHaveLength(1));
  await act(async () => { view.client.setQueryData(portalKeys.deal("deal"), next); });
  await waitFor(() => expect(view.result.current.context.activeDeal?.status).toBe("ANALYSIS"));
  expect(view.result.current.detail?.status).toBe("ANALYSIS");
 });
 it("não regride para uma etapa anterior quando uma consulta antiga chega por último", async () => {
  const view = mount();
  await waitFor(() => expect(view.result.current.context.deals).toHaveLength(1));
  let resolve!: (deals: DealDTO[]) => void;
  mocks.getDeals.mockImplementationOnce(() => new Promise<DealDTO[]>(r => { resolve = r; }));
  let refresh!: Promise<void>;
  await act(async () => { refresh = view.result.current.context.refreshDeals(); });
  await act(async () => { view.client.setQueryData(portalKeys.deal("deal"), next); });
  await act(async () => { resolve([initial]); await refresh; });
  await waitFor(() => expect(view.result.current.context.activeDeal?.status).toBe("ANALYSIS"));
  expect(view.result.current.detail?.status).toBe("ANALYSIS");
 });
 it("reconcilia eventos perdidos ao reconectar", async () => {
  const view = mount();
  await waitFor(() => expect(view.result.current.detail?.status).toBe("NEGOTIATION"));
  mocks.getDeals.mockResolvedValue([next]); mocks.getDealById.mockResolvedValue(next);
  mocks.connected = true; view.rerender();
  await waitFor(() => expect(view.result.current.detail?.status).toBe("ANALYSIS"));
 });
 it("reconcilia ao voltar à aba e preserva os dados quando a rede falha", async () => {
  const view = mount();
  await waitFor(() => expect(view.result.current.detail?.status).toBe("NEGOTIATION"));
  mocks.getDeals.mockRejectedValue(new Error("offline")); mocks.getDealById.mockRejectedValue(new Error("offline"));
  await act(async () => { document.dispatchEvent(new Event("visibilitychange")); });
  await waitFor(() => expect(view.result.current.query.isError).toBe(true));
  expect(view.result.current.detail?.status).toBe("NEGOTIATION");
  mocks.getDeals.mockResolvedValue([next]); mocks.getDealById.mockResolvedValue(next);
  await act(async () => { document.dispatchEvent(new Event("visibilitychange")); });
  await waitFor(() => expect(view.result.current.detail?.status).toBe("ANALYSIS"));
 });
 it("agrupa eventos repetidos e não consulta novamente para mensagens sem relação com a jornada", async () => {
  const view = mount();
  await waitFor(() => expect(view.result.current.detail?.status).toBe("NEGOTIATION"));
  await emit("new_message", { channel_id: "channel" });
  expect(mocks.getDealById).toHaveBeenCalledTimes(1);
  mocks.getDeals.mockResolvedValue([next]); mocks.getDealById.mockResolvedValue(next);
  await emit("proposal_update", { deal_id: "deal" });
  await emit("deal_status_changed", { deal_id: "deal" });
  await emit("checklist_solicited", { deal_id: "deal" });
  await waitFor(() => expect(view.result.current.detail?.status).toBe("ANALYSIS"));
  expect(mocks.getDealById).toHaveBeenCalledTimes(2);
 });
});
