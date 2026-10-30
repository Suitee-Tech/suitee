import type { PropsWithChildren } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { DealDTO } from "../services/deal.service";
import { dealQueryKey, allDealsQueryKey } from "../deals/queries";
import { fetchDeal } from "../deals/queries";
import { shareDealRevision } from "../deals/revisions";
import { DealsProvider, useDealsContext } from "./DealsContext";

const mocks = vi.hoisted(() => ({
  getAll: vi.fn(),
  getById: vi.fn(),
  updateBrokerParticipations: vi.fn(),
  notifyApiError: vi.fn(),
}));

vi.mock("../services/deal.service", () => ({
  dealService: {
    getAll: mocks.getAll,
    getById: mocks.getById,
    updateBrokerParticipations: mocks.updateBrokerParticipations,
  },
}));

vi.mock("../services/errorToast", () => ({
  notifyApiError: mocks.notifyApiError,
}));

function dealDto(overrides: Partial<DealDTO> = {}): DealDTO {
  return {
    id: "deal-1",
    type: "SALE",
    status: "NEGOTIATION",
    property_id: "property-1",
    owner_id: "owner-1",
    customer_id: "lead-1",
    lead_id: "lead-1",
    broker_id: "broker-operator",
    broker_participations: [
      { broker_id: "broker-property", role: "captador", percentage: 100 },
    ],
    value: 980_000,
    commission: 6,
    created_at: "2026-08-28T10:00:00Z",
    updated_at: "2026-08-28T10:00:00Z",
    tenant_id: "tenant-a",
    ...overrides,
  };
}

describe("DealsProvider.updateBrokerParticipations", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    mocks.getAll.mockReset().mockResolvedValue({
      data: { data: [dealDto()], meta: { total: 1 } },
    });
    mocks.updateBrokerParticipations.mockReset();
    mocks.getById.mockReset().mockResolvedValue({ data: { data: dealDto() } });
    mocks.notifyApiError.mockReset();
  });

  function wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>
        <DealsProvider
          isAuthenticated
          companyId="tenant-a"
          permissionsLoading={false}
          canReadDeals
        >
          {children}
        </DealsProvider>
      </QueryClientProvider>
    );
  }

  it("publica a resposta persistida no cache de detalhe e lista antes de concluir a mutação", async () => {
    const participations = [
      { broker_id: "broker-property", role: "captador" as const, percentage: 20 },
      { broker_id: "broker-lead", role: "fechamento" as const, percentage: 40 },
      { broker_id: "", role: "imobiliaria" as const, percentage: 40 },
    ];
    const persisted = dealDto({
      broker_participations: participations,
      updated_at: "2026-08-28T11:00:00Z",
    });
    mocks.updateBrokerParticipations.mockResolvedValue({
      data: { data: persisted },
    });

    const { result } = renderHook(() => useDealsContext(), { wrapper });
    await waitFor(() => expect(result.current.dealDTOs).toHaveLength(1));

    await act(async () => {
      await result.current.updateBrokerParticipations(
        "deal-1",
        participations,
      );
    });

    expect(mocks.updateBrokerParticipations).toHaveBeenCalledWith(
      "deal-1",
      participations,
      undefined,
    );
    expect(queryClient.getQueryData(dealQueryKey("deal-1", "tenant-a"))).toEqual(persisted);
    expect(
      queryClient.getQueryData<DealDTO[]>(allDealsQueryKey("tenant-a")),
    ).toEqual([persisted]);
  });

  it("exibe atualização externa após uma mutação local na mesma fonte usada pelos detalhes", async () => {
    const local = dealDto({ commission: 7, updated_at: "2026-08-28T11:00:00Z" });
    const external = dealDto({ commission: 8, status: "CONTRACT", updated_at: "2026-08-28T12:00:00Z" });
    mocks.updateBrokerParticipations.mockResolvedValue({ data: { data: local } });
    const { result } = renderHook(() => ({
      deals: useDealsContext(),
      details: useQuery({ queryKey: dealQueryKey("deal-1", "tenant-a"), queryFn: () => fetchDeal("deal-1"), structuralSharing: shareDealRevision }),
    }), { wrapper });
    await waitFor(() => expect(result.current.details.data?.commission).toBe(6));
    await act(async () => { await result.current.deals.updateBrokerParticipations("deal-1", []); });
    await waitFor(() => expect(result.current.details.data?.commission).toBe(7));
    mocks.getById.mockResolvedValue({ data: { data: external } });
    await act(async () => { await result.current.details.refetch(); });
    await waitFor(() => expect(result.current.details.data).toEqual(external));
    // A stale replica read must not regress the newly confirmed revision.
    mocks.getById.mockResolvedValue({ data: { data: local } });
    await act(async () => { await result.current.details.refetch(); });
    expect(result.current.details.data).toEqual(external);
  });

  it("descarta leitura iniciada antes da confirmação da mutação", async () => {
    const persisted = dealDto({ commission: 7, updated_at: "2026-08-28T11:00:00Z" });
    mocks.updateBrokerParticipations.mockResolvedValue({ data: { data: persisted } });
    const { result } = renderHook(() => useDealsContext(), { wrapper });
    await waitFor(() => expect(result.current.dealDTOs).toHaveLength(1));
    let releaseRead!: (value: { data: { data: DealDTO } }) => void;
    mocks.getById.mockImplementationOnce(() => new Promise((resolve) => { releaseRead = resolve; }));
    const read = result.current.fetchDealById("deal-1").catch(() => undefined);
    await waitFor(() => expect(mocks.getById).toHaveBeenCalledOnce());
    await act(async () => { await result.current.updateBrokerParticipations("deal-1", []); });
    releaseRead({ data: { data: dealDto() } });
    await act(async () => { await read; });
    expect(queryClient.getQueryData(dealQueryKey("deal-1", "tenant-a"))).toEqual(persisted);
  });

  it("preserva a revisão mais nova quando timestamps diferem apenas em nanossegundos", async () => {
    const persisted = dealDto({ commission: 7, updated_at: "2026-08-28T11:00:00.123456789Z" });
    const stale = dealDto({ commission: 6, updated_at: "2026-08-28T11:00:00.123456788Z" });
    mocks.updateBrokerParticipations.mockResolvedValue({ data: { data: persisted } });
    const { result } = renderHook(() => useDealsContext(), { wrapper });
    await waitFor(() => expect(result.current.dealDTOs).toHaveLength(1));
    await act(async () => { await result.current.updateBrokerParticipations("deal-1", []); });
    mocks.getById.mockResolvedValue({ data: { data: stale } });
    await act(async () => { await result.current.fetchDealById("deal-1"); });
    expect(queryClient.getQueryData(dealQueryKey("deal-1", "tenant-a"))).toEqual(persisted);
  });
});
