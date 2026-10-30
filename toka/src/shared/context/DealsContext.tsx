import { createContext, useCallback, useContext, useEffect, useMemo, useRef, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { unwrapApiData } from "../services/api";
import { notifyApiError } from "../services/errorToast";
import {
  dealService,
  type AddChecklistItemDTO,
  type AddProposalDTO,
  type AddSignerDTO,
  type ChecklistItemDTO,
  type ChecklistRequestedFieldDTO,
  type CreateDealDTO,
  type CreatePartyDTO,
  type DealDTO,
  type BrokerParticipation,
  type PartyRole,
  type UpdateChecklistItemStatusDTO,
  type UpdateDealStatusDTO,
  type UpdateDueDiligenceDTO,
  type UpdatePartyDTO,
  type UpdateSignerDTO,
  type UpdateSignerStatusDTO,
} from "../services/deal.service";
import { dealQueryKey, allDealsQueryKey, fetchDeal, fetchAllDeals } from "../deals/queries";
import { newestDeal, shareDealRevision, shareDealsRevisions } from "../deals/revisions";

type DealsContextValue = {
  dealDTOs: DealDTO[];
  dealsLoading: boolean;
  dealsError: string | null;
  refreshDeals: () => Promise<void>;
  fetchDealById: (id: string) => Promise<DealDTO>;
  createDeal: (dto: CreateDealDTO, idempotencyKey?: string) => Promise<DealDTO>;
  deleteDeal: (id: string) => Promise<void>;
  updateDealStatus: (id: string, dto: UpdateDealStatusDTO) => Promise<DealDTO | null>;
  addProposal: (dealId: string, dto: AddProposalDTO) => Promise<DealDTO | null>;
  acceptCounterOffer: (dealId: string, proposalId: string) => Promise<DealDTO | null>;
  updateDueDiligence: (dealId: string, dto: UpdateDueDiligenceDTO) => Promise<DealDTO | null>;
  updatePartyDueDiligence: (dealId: string, partyId: string, dto: UpdateDueDiligenceDTO) => Promise<DealDTO | null>;
  addSigner: (dealId: string, dto: AddSignerDTO) => Promise<DealDTO | null>;
  updateSignerStatus: (dealId: string, signerId: string, dto: UpdateSignerStatusDTO) => Promise<DealDTO | null>;
  updateSigner: (dealId: string, signerId: string, dto: UpdateSignerDTO) => Promise<DealDTO | null>;
  inviteSigner: (dealId: string, signerId: string) => Promise<void>;
  addParty: (dealId: string, dto: CreatePartyDTO) => Promise<DealDTO | null>;
  removeParty: (dealId: string, partyId: string) => Promise<DealDTO | null>;
  setPartyPrimary: (dealId: string, partyId: string) => Promise<DealDTO | null>;
  updatePartyRole: (dealId: string, partyId: string, role: PartyRole) => Promise<DealDTO | null>;
  updateParty: (dealId: string, partyId: string, dto: UpdatePartyDTO) => Promise<DealDTO | null>;
  updateBrokerParticipations: (
    dealId: string,
    participations: BrokerParticipation[],
    commissionPct?: number,
  ) => Promise<DealDTO>;
  initChecklist: (
    dealId: string,
    dto: { items: Array<{ name: string; category: ChecklistItemDTO["category"]; required: boolean; description?: string }> },
  ) => Promise<DealDTO | null>;
  addChecklistItem: (dealId: string, dto: AddChecklistItemDTO) => Promise<DealDTO | null>;
  removeChecklistItem: (dealId: string, itemId: string) => Promise<DealDTO | null>;
  updateChecklistItemStatus: (dealId: string, itemId: string, dto: UpdateChecklistItemStatusDTO) => Promise<DealDTO | null>;
  solicitChecklistItem: (
    dealId: string,
    itemId: string,
    partyId?: string,
    requestedFields?: ChecklistRequestedFieldDTO[],
  ) => Promise<DealDTO | null>;
};

const DealsContext = createContext<DealsContextValue | null>(null);

export function DealsProvider({
  children,
  isAuthenticated,
  companyId,
  permissionsLoading,
  canReadDeals,
}: {
  children: ReactNode;
  isAuthenticated: boolean;
  companyId?: string;
  permissionsLoading: boolean;
  canReadDeals: boolean;
}) {
  const queryClient = useQueryClient();

  const dealsQuery = useQuery({
    queryKey: allDealsQueryKey(companyId),
    queryFn: ({ signal }) => fetchAllDeals(signal),
    structuralSharing: shareDealsRevisions,
    enabled: isAuthenticated && !!companyId && !permissionsLoading && canReadDeals,
  });

  const dealDTOs = useMemo(() => dealsQuery.data ?? [], [dealsQuery.data]);
  const dealsLoading = dealsQuery.isLoading;
  const dealsError = dealsQuery.isError ? "Não foi possível carregar todos os negócios. A lista pode estar incompleta ou desatualizada." : null;
  const refreshDeals = useCallback(async () => { await dealsQuery.refetch(); }, [dealsQuery.refetch]);

  const fetchDealById = useCallback(
    async (id: string) => {
      return queryClient.fetchQuery({
        queryKey: dealQueryKey(id, companyId),
        queryFn: () => fetchDeal(id),
        structuralSharing: shareDealRevision,
      });
    },
    [companyId, queryClient],
  );

  useEffect(() => {
    if (dealsQuery.isError && dealsQuery.error) {
      notifyApiError(dealsQuery.error, { context: "Carregar negócios" });
    }
  }, [dealsQuery.error, dealsQuery.isError]);

  const previousCompanyId = useRef<string | undefined>(companyId);
  useEffect(() => {
    const prev = previousCompanyId.current;
    if (prev && companyId && prev !== companyId) {
      queryClient.removeQueries({ queryKey: ["deals"] });
      queryClient.removeQueries({ queryKey: ["deal"] });
    }
    previousCompanyId.current = companyId;
  }, [companyId, queryClient]);

  useEffect(() => {
    if (isAuthenticated) return;
    queryClient.removeQueries({ queryKey: ["deals"] });
    queryClient.removeQueries({ queryKey: ["deal"] });
  }, [isAuthenticated, queryClient]);

  const upsertDeal = useCallback(
    async (next: DealDTO) => {
      const detailKey = dealQueryKey(next.id, companyId);
      const listPrefix = ["deals", companyId ?? "no-tenant"];
      // A read started before the mutation ACK must not replace its response.
      await Promise.all([
        queryClient.cancelQueries({ queryKey: detailKey, exact: true }),
        queryClient.cancelQueries({ queryKey: listPrefix }),
      ]);
      queryClient.setQueryData<DealDTO>(detailKey, (previous) => newestDeal(previous, next));
      queryClient.setQueriesData<DealDTO[]>({ queryKey: ["deals", companyId ?? "no-tenant"] }, (prev) => {
        if (!Array.isArray(prev)) return prev;
        const idx = prev.findIndex((d) => d.id === next.id);
        if (idx === -1) return [next, ...prev];
        const copy = prev.slice();
        copy[idx] = newestDeal(copy[idx], next);
        return copy;
      });
    },
    [companyId, queryClient],
  );

  const createDeal = useCallback(
    async (dto: CreateDealDTO, idempotencyKey?: string) => {
      const res = await dealService.create(dto, idempotencyKey);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const deleteDeal = useCallback(
    async (id: string) => {
      await dealService.delete(id);
      queryClient.removeQueries({ queryKey: dealQueryKey(id, companyId) });
      queryClient.setQueriesData<DealDTO[]>({ queryKey: ["deals", companyId ?? "no-tenant"] }, (prev) => {
        if (!Array.isArray(prev)) return prev;
        return prev.filter((d) => d.id !== id);
      });
    },
    [companyId, queryClient],
  );

  const updateDealStatus = useCallback(
    async (id: string, dto: UpdateDealStatusDTO) => {
      const res = await dealService.updateStatus(id, dto);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const addProposal = useCallback(
    async (dealId: string, dto: AddProposalDTO) => {
      const res = await dealService.addProposal(dealId, dto);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const acceptCounterOffer = useCallback(
    async (dealId: string, proposalId: string) => {
      const res = await dealService.acceptCounterOffer(dealId, proposalId);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const updateDueDiligence = useCallback(
    async (dealId: string, dto: UpdateDueDiligenceDTO) => {
      const res = await dealService.updateDueDiligence(dealId, dto);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const updatePartyDueDiligence = useCallback(
    async (dealId: string, partyId: string, dto: UpdateDueDiligenceDTO) => {
      const res = await dealService.updatePartyDueDiligence(dealId, partyId, dto);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const addSigner = useCallback(
    async (dealId: string, dto: AddSignerDTO) => {
      const res = await dealService.addSigner(dealId, dto);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const updateSignerStatus = useCallback(
    async (dealId: string, signerId: string, dto: UpdateSignerStatusDTO) => {
      const res = await dealService.updateSignerStatus(dealId, signerId, dto);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const updateSigner = useCallback(
    async (dealId: string, signerId: string, dto: UpdateSignerDTO) => {
      const res = await dealService.updateSigner(dealId, signerId, dto);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const inviteSigner = useCallback(async (dealId: string, signerId: string) => {
    await dealService.inviteSigner(dealId, signerId);
  }, []);

  const addParty = useCallback(
    async (dealId: string, dto: CreatePartyDTO) => {
      const res = await dealService.addParty(dealId, dto);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const removeParty = useCallback(
    async (dealId: string, partyId: string) => {
      const res = await dealService.removeParty(dealId, partyId);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const setPartyPrimary = useCallback(
    async (dealId: string, partyId: string) => {
      const res = await dealService.setPartyPrimary(dealId, partyId);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const updatePartyRole = useCallback(
    async (dealId: string, partyId: string, role: PartyRole) => {
      const res = await dealService.updatePartyRole(dealId, partyId, { role });
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const updateParty = useCallback(
    async (dealId: string, partyId: string, dto: UpdatePartyDTO) => {
      const res = await dealService.updateParty(dealId, partyId, dto);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const updateBrokerParticipations = useCallback(
    async (
      dealId: string,
      participations: BrokerParticipation[],
      commissionPct?: number,
    ) => {
      const res = await dealService.updateBrokerParticipations(
        dealId,
        participations,
        commissionPct,
      );
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const initChecklist = useCallback(
    async (
      dealId: string,
      dto: { items: Array<{ name: string; category: ChecklistItemDTO["category"]; required: boolean; description?: string }> },
    ) => {
      const res = await dealService.initChecklist(dealId, dto);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const addChecklistItem = useCallback(
    async (dealId: string, dto: AddChecklistItemDTO) => {
      const res = await dealService.addChecklistItem(dealId, dto);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const removeChecklistItem = useCallback(
    async (dealId: string, itemId: string) => {
      const res = await dealService.removeChecklistItem(dealId, itemId);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const updateChecklistItemStatus = useCallback(
    async (dealId: string, itemId: string, dto: UpdateChecklistItemStatusDTO) => {
      const res = await dealService.updateChecklistItemStatus(dealId, itemId, dto);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const solicitChecklistItem = useCallback(
    async (dealId: string, itemId: string, partyId?: string, requestedFields?: ChecklistRequestedFieldDTO[]) => {
      const res = await dealService.solicitChecklistItem(dealId, itemId, partyId, requestedFields);
      const deal = unwrapApiData<DealDTO>(res);
      await upsertDeal(deal);
      return deal;
    },
    [upsertDeal],
  );

  const value = useMemo<DealsContextValue>(
    () => ({
      dealDTOs,
      dealsLoading,
      dealsError,
      refreshDeals,
      fetchDealById,
      createDeal,
      deleteDeal,
      updateDealStatus,
      addProposal,
      acceptCounterOffer,
      updateDueDiligence,
      updatePartyDueDiligence,
      addSigner,
      updateSignerStatus,
      updateSigner,
      inviteSigner,
      addParty,
      removeParty,
      setPartyPrimary,
      updatePartyRole,
      updateParty,
      updateBrokerParticipations,
      initChecklist,
      addChecklistItem,
      removeChecklistItem,
      updateChecklistItemStatus,
      solicitChecklistItem,
    }),
    [
      addChecklistItem,
      addParty,
      addProposal,
      acceptCounterOffer,
      addSigner,
      createDeal,
      dealDTOs,
      dealsLoading,
      dealsError,
      refreshDeals,
      deleteDeal,
      fetchDealById,
      initChecklist,
      removeChecklistItem,
      removeParty,
      setPartyPrimary,
      solicitChecklistItem,
      updateChecklistItemStatus,
      updateDealStatus,
      updateDueDiligence,
      updateParty,
      updateBrokerParticipations,
      updatePartyDueDiligence,
      updatePartyRole,
      updateSignerStatus,
      updateSigner,
      inviteSigner,
    ],
  );

  return <DealsContext.Provider value={value}>{children}</DealsContext.Provider>;
}

export function useDealsContext() {
  const ctx = useContext(DealsContext);
  if (!ctx) {
    throw new Error("useDealsContext must be used within DealsProvider");
  }
  return ctx;
}
