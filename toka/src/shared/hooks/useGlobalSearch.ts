import { useQueries } from "@tanstack/react-query";
import { useDebouncedValue } from "./useDebouncedValue";
import {
  searchLeads,
  searchProperties,
  searchBrokers,
  searchDeals,
  type SearchResult,
} from "../api/search";

export type GlobalSearchGroups = {
  leads: SearchResult[];
  properties: SearchResult[];
  brokers: SearchResult[];
  deals: SearchResult[];
};

export type UseGlobalSearchResult = {
  debouncedTerm: string;
  enabled: boolean;
  isFetching: boolean;
  groups: GlobalSearchGroups;
  isEmpty: boolean;
};

const MIN_TERM_LENGTH = 2;
const DEBOUNCE_MS = 250;
const STALE_TIME_MS = 30_000;

export function useGlobalSearch(term: string): UseGlobalSearchResult {
  const debouncedTerm = useDebouncedValue(term.trim(), DEBOUNCE_MS);
  const enabled = debouncedTerm.length >= MIN_TERM_LENGTH;

  const queries = useQueries({
    queries: [
      {
        queryKey: ["search", "leads", debouncedTerm] as const,
        queryFn: ({ signal }: { signal?: AbortSignal }) => searchLeads(debouncedTerm, signal),
        enabled,
        staleTime: STALE_TIME_MS,
      },
      {
        queryKey: ["search", "properties", debouncedTerm] as const,
        queryFn: ({ signal }: { signal?: AbortSignal }) => searchProperties(debouncedTerm, signal),
        enabled,
        staleTime: STALE_TIME_MS,
      },
      {
        queryKey: ["search", "brokers", debouncedTerm] as const,
        queryFn: ({ signal }: { signal?: AbortSignal }) => searchBrokers(debouncedTerm, signal),
        enabled,
        staleTime: STALE_TIME_MS,
      },
      {
        queryKey: ["search", "deals", debouncedTerm] as const,
        queryFn: ({ signal }: { signal?: AbortSignal }) => searchDeals(debouncedTerm, signal),
        enabled,
        staleTime: STALE_TIME_MS,
      },
    ],
  });

  const [leadsQ, propertiesQ, brokersQ, dealsQ] = queries;

  const groups: GlobalSearchGroups = {
    leads: leadsQ.data ?? [],
    properties: propertiesQ.data ?? [],
    brokers: brokersQ.data ?? [],
    deals: dealsQ.data ?? [],
  };

  const isFetching = queries.some((q) => q.isFetching);
  const allDoneAndEmpty =
    enabled &&
    !isFetching &&
    queries.every((q) => q.fetchStatus !== "fetching" && (q.data ?? []).length === 0);

  return {
    debouncedTerm,
    enabled,
    isFetching,
    groups,
    isEmpty: allDoneAndEmpty,
  };
}
