import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useGlobalSearch } from "../useGlobalSearch";
import type { SearchResult } from "../../../shared/api/search";

vi.mock("../../../shared/api/search", () => ({
  searchLeads: vi.fn<(q: string) => Promise<SearchResult[]>>(),
  searchProperties: vi.fn<(q: string) => Promise<SearchResult[]>>(),
  searchBrokers: vi.fn<(q: string) => Promise<SearchResult[]>>(),
  searchDeals: vi.fn<(q: string) => Promise<SearchResult[]>>(),
}));

import * as searchApi from "../../../shared/api/search";

const wrapperFactory = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  // eslint-disable-next-line react/display-name
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const lead = (id: string): SearchResult => ({
  entity: "lead",
  id,
  title: `Lead ${id}`,
  route: `/leads/${id}`,
});

describe("useGlobalSearch", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(searchApi.searchLeads).mockResolvedValue([]);
    vi.mocked(searchApi.searchProperties).mockResolvedValue([]);
    vi.mocked(searchApi.searchBrokers).mockResolvedValue([]);
    vi.mocked(searchApi.searchDeals).mockResolvedValue([]);
  });

  it("does not fetch with empty term", async () => {
    const { result } = renderHook(() => useGlobalSearch(""), { wrapper: wrapperFactory() });
    expect(result.current.enabled).toBe(false);
    expect(searchApi.searchLeads).not.toHaveBeenCalled();
    expect(searchApi.searchProperties).not.toHaveBeenCalled();
    expect(searchApi.searchBrokers).not.toHaveBeenCalled();
    expect(searchApi.searchDeals).not.toHaveBeenCalled();
  });

  it("does not fetch with single character", async () => {
    const { result } = renderHook(() => useGlobalSearch("c"), { wrapper: wrapperFactory() });
    // Wait for debounce to settle
    await new Promise((r) => setTimeout(r, 300));
    expect(result.current.enabled).toBe(false);
    expect(searchApi.searchLeads).not.toHaveBeenCalled();
  });

  it("fans out across 4 entities once debounced term ≥2 chars", async () => {
    vi.mocked(searchApi.searchLeads).mockResolvedValue([lead("L1")]);

    const { result } = renderHook(() => useGlobalSearch("carolina"), { wrapper: wrapperFactory() });

    await waitFor(() => expect(result.current.enabled).toBe(true), { timeout: 1000 });
    await waitFor(() => expect(searchApi.searchLeads).toHaveBeenCalled(), { timeout: 1000 });

    expect(searchApi.searchLeads).toHaveBeenCalledWith("carolina", expect.anything());
    expect(searchApi.searchProperties).toHaveBeenCalledWith("carolina", expect.anything());
    expect(searchApi.searchBrokers).toHaveBeenCalledWith("carolina", expect.anything());
    expect(searchApi.searchDeals).toHaveBeenCalledWith("carolina", expect.anything());

    await waitFor(() => expect(result.current.groups.leads).toEqual([lead("L1")]), {
      timeout: 1000,
    });
  });

  it("isEmpty is true when all groups returned no results", async () => {
    const { result } = renderHook(() => useGlobalSearch("nothing"), { wrapper: wrapperFactory() });
    await waitFor(() => expect(result.current.isEmpty).toBe(true), { timeout: 1500 });
    expect(result.current.groups.leads).toEqual([]);
    expect(result.current.groups.properties).toEqual([]);
    expect(result.current.groups.brokers).toEqual([]);
    expect(result.current.groups.deals).toEqual([]);
  });
});
