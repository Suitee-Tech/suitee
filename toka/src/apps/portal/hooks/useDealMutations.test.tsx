import type { PropsWithChildren } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
  CreateAppointmentRequest,
  PortalAppointment,
  PortalTicket,
  PortalTicketComment,
} from "../services/portal.service";
import { portalKeys } from "./queryKeys";
import {
  useAddTicketComment,
  useCreateDealAppointment,
  useToggleDealFavorite,
} from "./useDealMutations";

const mocks = vi.hoisted(() => ({
  createDealAppointment: vi.fn(),
  addTicketComment: vi.fn(),
  addDealFavorite: vi.fn(),
  removeDealFavorite: vi.fn(),
}));

vi.mock("../services/portal.service", () => ({
  portalService: {
    createDealAppointment: mocks.createDealAppointment,
    addTicketComment: mocks.addTicketComment,
    addDealFavorite: mocks.addDealFavorite,
    removeDealFavorite: mocks.removeDealFavorite,
  },
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

function appointment(id: string): PortalAppointment {
  return {
    id,
    title: "Visita",
    date: "2026-07-30",
    time: "10:00",
    end_time: "10:30",
    type: "VISIT",
    status: "PENDING",
    created_at: "2026-07-28T18:00:00Z",
  };
}

function ticket(id: string, comments: PortalTicketComment[] = []): PortalTicket {
  return {
    id,
    tenant_id: "tenant-1",
    deal_id: "deal-a",
    type: "support",
    category: "general",
    subject: "Ajuda",
    description: "Preciso de ajuda",
    status: "open",
    priority: "medium",
    requester_account_id: "account-1",
    comments,
    created_at: "2026-07-28T18:00:00Z",
    updated_at: "2026-07-28T18:00:00Z",
  };
}

describe("portal deal mutations", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    mocks.createDealAppointment.mockReset();
    mocks.addTicketComment.mockReset();
    mocks.addDealFavorite.mockReset();
    mocks.removeDealFavorite.mockReset();
  });

  function wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  }

  it("reconciles an appointment into the deal that started the request after the active deal changes", async () => {
    const pending = deferred<PortalAppointment>();
    mocks.createDealAppointment.mockReturnValueOnce(pending.promise);
    const request: CreateAppointmentRequest = {
      date: "2026-07-30",
      time: "10:00",
      end_time: "10:30",
    };
    const created = appointment("appointment-1");

    const { result, rerender } = renderHook(
      ({ dealId }) => useCreateDealAppointment(dealId),
      { initialProps: { dealId: "deal-a" }, wrapper },
    );

    let mutation!: Promise<PortalAppointment>;
    act(() => {
      mutation = result.current.mutateAsync(request);
    });
    rerender({ dealId: "deal-b" });

    await act(async () => {
      pending.resolve(created);
      await mutation;
    });

    expect(mocks.createDealAppointment).toHaveBeenCalledWith("deal-a", request);
    expect(queryClient.getQueryData(portalKeys.dealAppointments("deal-a"))).toEqual([created]);
    expect(queryClient.getQueryData(portalKeys.dealAppointments("deal-b"))).toBeUndefined();
  });

  it("adds the returned comment immediately to detail and list caches", async () => {
    const original = ticket("ticket-1");
    queryClient.setQueryData(portalKeys.ticket(original.id), original);
    queryClient.setQueryData(portalKeys.dealTickets("deal-a"), [original]);
    const comment: PortalTicketComment = {
      id: "comment-1",
      author_id: "account-1",
      author_type: "client",
      content: "Novo comentário",
      created_at: "2026-07-28T18:05:00Z",
    };
    mocks.addTicketComment.mockResolvedValueOnce(comment);

    const { result } = renderHook(() => useAddTicketComment("ticket-1", "deal-a"), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({ content: comment.content });
    });

    expect(queryClient.getQueryData<PortalTicket>(portalKeys.ticket("ticket-1"))?.comments).toEqual([comment]);
    expect(
      queryClient.getQueryData<PortalTicket[]>(portalKeys.dealTickets("deal-a"))?.[0].comments,
    ).toEqual([comment]);
  });

  it("rolls an optimistic favorite back in the original deal cache when the request fails", async () => {
    const pending = deferred<void>();
    mocks.addDealFavorite.mockReturnValueOnce(pending.promise);
    queryClient.setQueryData(portalKeys.dealFavorites("deal-a"), ["property-1"]);
    queryClient.setQueryData(portalKeys.dealFavorites("deal-b"), ["property-b"]);

    const { result, rerender } = renderHook(
      ({ dealId }) => useToggleDealFavorite(dealId),
      { initialProps: { dealId: "deal-a" }, wrapper },
    );

    let mutation!: Promise<void>;
    act(() => {
      mutation = result.current.mutateAsync({ propertyId: "property-2", favorite: true });
    });

    await waitFor(() => {
      expect(queryClient.getQueryData(portalKeys.dealFavorites("deal-a"))).toEqual([
        "property-1",
        "property-2",
      ]);
    });
    rerender({ dealId: "deal-b" });

    await act(async () => {
      pending.reject(new Error("favorite failed"));
      await mutation.catch(() => undefined);
    });

    expect(queryClient.getQueryData(portalKeys.dealFavorites("deal-a"))).toEqual(["property-1"]);
    expect(queryClient.getQueryData(portalKeys.dealFavorites("deal-b"))).toEqual(["property-b"]);
  });
});
