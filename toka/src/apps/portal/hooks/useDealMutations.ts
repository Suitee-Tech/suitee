import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  portalService,
  type CreateAppointmentRequest,
  type CreateMaintenanceTicketRequest,
  type CreateTicketRequest,
  type PortalAppointment,
  type PortalMaintenanceTicket,
  type PortalFavoriteIDs,
  type PortalTicket,
  type PortalTicketComment,
} from "../services/portal.service";
import { portalKeys } from "./queryKeys";

interface DealMutationScope {
  dealId?: string;
}

interface TicketMutationScope extends DealMutationScope {
  ticketId?: string;
}

interface FavoriteMutationScope extends DealMutationScope {
  previousFavorites?: PortalFavoriteIDs;
}

function requireId(id: string | undefined, label: string): string {
  if (!id) throw new Error(`missing ${label}`);
  return id;
}

function upsertById<T extends { id: string }>(current: T[] | undefined, value: T): T[] {
  if (!current) return [value];
  const index = current.findIndex((item) => item.id === value.id);
  if (index < 0) return [value, ...current];
  const next = current.slice();
  next[index] = value;
  return next;
}

export function useUploadChecklistItem(dealId: string | undefined) {
  const qc = useQueryClient();
  return useMutation<unknown, Error, { itemId: string; file?: File | null }, DealMutationScope>({
    mutationFn: ({ itemId, file }: { itemId: string; file?: File | null }) =>
      portalService.uploadChecklistItem(requireId(dealId, "deal id"), itemId, file ?? null),
    onMutate: () => ({ dealId }),
    onSuccess: (_data, _variables, scope) => {
      if (!scope?.dealId) return;
      void qc.invalidateQueries({ queryKey: portalKeys.dealChecklist(scope.dealId) });
      void qc.invalidateQueries({ queryKey: portalKeys.deal(scope.dealId) });
      void qc.invalidateQueries({ queryKey: portalKeys.deals() });
    },
  });
}

export function useAcceptProposal(dealId: string | undefined) {
  const qc = useQueryClient();
  return useMutation<unknown, Error, string, DealMutationScope>({
    mutationFn: (proposalId: string) => portalService.acceptProposal(requireId(dealId, "deal id"), proposalId),
    onMutate: () => ({ dealId }),
    onSuccess: (_data, _variables, scope) => {
      if (!scope?.dealId) return;
      void qc.invalidateQueries({ queryKey: portalKeys.deal(scope.dealId) });
      void qc.invalidateQueries({ queryKey: portalKeys.deals() });
    },
  });
}

export function useRejectProposal(dealId: string | undefined) {
  const qc = useQueryClient();
  return useMutation<unknown, Error, { proposalId: string; reason?: string; counterValue?: number }, DealMutationScope>({
    mutationFn: ({ proposalId, reason, counterValue }: { proposalId: string; reason?: string; counterValue?: number }) =>
      portalService.rejectProposal(requireId(dealId, "deal id"), proposalId, reason, counterValue),
    onMutate: () => ({ dealId }),
    onSuccess: (_data, _variables, scope) => {
      if (!scope?.dealId) return;
      void qc.invalidateQueries({ queryKey: portalKeys.deal(scope.dealId) });
      void qc.invalidateQueries({ queryKey: portalKeys.deals() });
    },
  });
}

export function useCreateDealAppointment(dealId: string | undefined) {
  const qc = useQueryClient();
  const mutation = useMutation<
    PortalAppointment,
    Error,
    { request: CreateAppointmentRequest; targetDealId: string },
    DealMutationScope
  >({
    mutationFn: ({ request, targetDealId }) => portalService.createDealAppointment(targetDealId, request),
    onMutate: ({ targetDealId }) => ({ dealId: targetDealId }),
    onSuccess: (appointment, _variables, scope) => {
      if (!scope?.dealId) return;
      qc.setQueryData<PortalAppointment[]>(portalKeys.dealAppointments(scope.dealId), (current) =>
        upsertById(current, appointment),
      );
      void qc.invalidateQueries({ queryKey: portalKeys.dealAppointments(scope.dealId) });
      void qc.invalidateQueries({ queryKey: portalKeys.deal(scope.dealId) });
    },
  });
  return {
    ...mutation,
    mutateAsync: (request: CreateAppointmentRequest) =>
      mutation.mutateAsync({ request, targetDealId: requireId(dealId, "deal id") }),
    mutate: (request: CreateAppointmentRequest) =>
      mutation.mutate({ request, targetDealId: requireId(dealId, "deal id") }),
  };
}

export function useCreateDealMaintenance(dealId: string | undefined) {
  const qc = useQueryClient();
  return useMutation<PortalMaintenanceTicket, Error, CreateMaintenanceTicketRequest, DealMutationScope>({
    mutationFn: (req) => portalService.createDealMaintenance(requireId(dealId, "deal id"), req),
    onMutate: () => ({ dealId }),
    onSuccess: (maintenance, _variables, scope) => {
      if (!scope?.dealId) return;
      qc.setQueryData<PortalMaintenanceTicket[]>(portalKeys.dealMaintenance(scope.dealId), (current) =>
        upsertById(current, maintenance),
      );
      void qc.invalidateQueries({ queryKey: portalKeys.dealMaintenance(scope.dealId) });
      void qc.invalidateQueries({ queryKey: portalKeys.deal(scope.dealId) });
    },
  });
}

export function useCreateDealTicket(dealId: string | undefined) {
  const qc = useQueryClient();
  return useMutation<PortalTicket, Error, CreateTicketRequest, DealMutationScope>({
    mutationFn: (req) => portalService.createDealTicket(requireId(dealId, "deal id"), req),
    onMutate: () => ({ dealId }),
    onSuccess: (ticket, _variables, scope) => {
      if (!scope?.dealId) return;
      qc.setQueryData(portalKeys.ticket(ticket.id), ticket);
      qc.setQueryData<PortalTicket[]>(portalKeys.dealTickets(scope.dealId), (current) => upsertById(current, ticket));
      void qc.invalidateQueries({ queryKey: portalKeys.dealTickets(scope.dealId) });
      void qc.invalidateQueries({ queryKey: portalKeys.deal(scope.dealId) });
    },
  });
}

export function useAddTicketComment(ticketId: string | undefined, dealId?: string) {
  const qc = useQueryClient();
  return useMutation<
    PortalTicketComment,
    Error,
    {
      content: string;
      attachments?: string[];
      clientRequestId?: string;
      targetTicketId?: string;
      targetDealId?: string;
    },
    TicketMutationScope
  >({
    mutationFn: ({ content, attachments, clientRequestId, targetTicketId }) =>
      portalService.addTicketComment(
        requireId(targetTicketId ?? ticketId, "ticket id"),
        content,
        attachments,
        clientRequestId,
      ),
    onMutate: ({ targetTicketId, targetDealId }) => ({
      ticketId: targetTicketId ?? ticketId,
      dealId: targetDealId ?? dealId,
    }),
    onSuccess: (comment, _variables, scope) => {
      if (scope?.ticketId) {
        qc.setQueryData<PortalTicket>(portalKeys.ticket(scope.ticketId), (current) =>
          current ? { ...current, comments: upsertById(current.comments, comment) } : current,
        );
        void qc.invalidateQueries({ queryKey: portalKeys.ticket(scope.ticketId) });
      }
      if (scope?.dealId && scope.ticketId) {
        qc.setQueryData<PortalTicket[]>(portalKeys.dealTickets(scope.dealId), (current) =>
          current?.map((ticket) =>
            ticket.id === scope.ticketId ? { ...ticket, comments: upsertById(ticket.comments, comment) } : ticket,
          ),
        );
        void qc.invalidateQueries({ queryKey: portalKeys.dealTickets(scope.dealId) });
      }
    },
  });
}

export function useToggleDealFavorite(dealId: string | undefined) {
  const qc = useQueryClient();
  return useMutation<void, Error, { propertyId: string; favorite: boolean }, FavoriteMutationScope>({
    mutationFn: async ({ propertyId, favorite }) => {
      const scopedDealId = requireId(dealId, "deal id");
      if (favorite) {
        await portalService.addDealFavorite(scopedDealId, propertyId);
      } else {
        await portalService.removeDealFavorite(scopedDealId, propertyId);
      }
    },
    onMutate: async ({ propertyId, favorite }) => {
      if (!dealId) return { dealId };
      await qc.cancelQueries({ queryKey: portalKeys.dealFavorites(dealId) });
      const previousFavorites = qc.getQueryData<PortalFavoriteIDs>(portalKeys.dealFavorites(dealId));
      qc.setQueryData<PortalFavoriteIDs>(portalKeys.dealFavorites(dealId), (current = []) => {
        const next = new Set(current);
        if (favorite) next.add(propertyId);
        else next.delete(propertyId);
        return Array.from(next);
      });
      return { dealId, previousFavorites };
    },
    onError: (_error, _variables, scope) => {
      if (!scope?.dealId) return;
      qc.setQueryData(portalKeys.dealFavorites(scope.dealId), scope.previousFavorites);
    },
    onSettled: (_data, _error, _variables, scope) => {
      if (!scope?.dealId) return;
      void qc.invalidateQueries({ queryKey: portalKeys.dealFavorites(scope.dealId) });
      void qc.invalidateQueries({ queryKey: portalKeys.deal(scope.dealId) });
    },
  });
}
