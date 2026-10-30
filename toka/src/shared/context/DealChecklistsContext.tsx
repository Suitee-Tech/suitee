import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { toast } from "sonner";
import {
  DEFAULT_CHECKLIST_TEMPLATES,
  type ChecklistItemStatus,
  type ChecklistTemplateId,
  type DocumentChecklistItem,
} from "../data";
import type {
  ChecklistItemDTO,
  ChecklistRequestedFieldDTO,
  DealDTO,
  InitChecklistDTO,
  AddChecklistItemDTO,
  UpdateChecklistItemStatusDTO,
} from "../services/deal.service";

type DealChecklistsContextValue = {
  dealChecklists: Record<string, DocumentChecklistItem[]>;
  updateChecklistFromDeal: (dealId: string, dealData: DealDTO | null | undefined) => void;
  initDealChecklist: (dealId: string, templateId: ChecklistTemplateId) => Promise<void>;
  addChecklistItem: (
    dealId: string,
    item: {
      name: string;
      category: DocumentChecklistItem["category"];
      required: boolean;
      description?: string;
      partyId?: string;
      requestMode?: "upload" | "form";
      requestedFields?: ChecklistRequestedFieldDTO[];
    },
  ) => Promise<void>;
  removeChecklistItem: (dealId: string, itemId: string) => Promise<void>;
  updateChecklistItemStatus: (dealId: string, itemId: string, status: ChecklistItemStatus, reason?: string) => Promise<void>;
  solicitChecklistItem: (
    dealId: string,
    itemId: string,
    partyId?: string,
    requestedFields?: ChecklistRequestedFieldDTO[],
    requestMode?: "upload" | "form",
  ) => Promise<void>;
  solicitAllPending: (dealId: string, partyId?: string) => Promise<void>;
};

const DealChecklistsContext = createContext<DealChecklistsContextValue | null>(null);

export function DealChecklistsProvider({
  children,
  isAuthenticated,
  companyId,
  canReadDeals,
  permissionsLoading,
  dealDTOs,
  fetchDealById,
  initChecklist,
  addChecklistItem: addChecklistItemDTO,
  removeChecklistItem: removeChecklistItemDTO,
  updateChecklistItemStatus: updateChecklistItemStatusDTO,
  solicitChecklistItem: solicitChecklistItemDTO,
}: {
  children: ReactNode;
  isAuthenticated: boolean;
  companyId?: string;
  canReadDeals: boolean;
  permissionsLoading: boolean;
  dealDTOs: DealDTO[];
  fetchDealById: (dealId: string) => Promise<DealDTO | null | undefined>;
  initChecklist: (dealId: string, data: InitChecklistDTO) => Promise<DealDTO | null | undefined>;
  addChecklistItem: (dealId: string, data: AddChecklistItemDTO) => Promise<DealDTO | null | undefined>;
  removeChecklistItem: (dealId: string, itemId: string) => Promise<DealDTO | null | undefined>;
  updateChecklistItemStatus: (
    dealId: string,
    itemId: string,
    data: UpdateChecklistItemStatusDTO,
  ) => Promise<DealDTO | null | undefined>;
  solicitChecklistItem: (
    dealId: string,
    itemId: string,
    partyId?: string,
    requestedFields?: ChecklistRequestedFieldDTO[],
    requestMode?: "upload" | "form",
  ) => Promise<DealDTO | null | undefined>;
}) {
  const [dealChecklists, setDealChecklists] = useState<Record<string, DocumentChecklistItem[]>>({});

  const mapChecklistDtoToLocal = useCallback(
    (dto: ChecklistItemDTO): DocumentChecklistItem => ({
      id: dto.id,
      name: dto.name,
      category: dto.category as DocumentChecklistItem["category"],
      partyId: dto.party_id,
      status: dto.status.toLowerCase() as ChecklistItemStatus,
      required: dto.required,
      description: dto.description,
      requestMode: dto.request_mode,
      requestedFields: dto.requested_fields,
      rejectionReason: dto.rejection_reason,
      uploadedAt: dto.uploaded_at,
      reviewedAt: dto.reviewed_at,
      solicited: dto.solicited,
      solicitedAt: dto.solicited_at,
      fileUrl: dto.file_url,
      fileName: dto.file_name,
      formSubmission: dto.form_submission
        ? {
            values: dto.form_submission.values ?? {},
            requestedFields: dto.form_submission.requested_fields,
            submittedBy: dto.form_submission.submitted_by,
            submittedAt: dto.form_submission.submitted_at,
          }
        : undefined,
    }),
    [],
  );

  const updateChecklistFromDeal = useCallback(
    (dealId: string, dealData: DealDTO | null | undefined) => {
      if (!dealData?.checklist) return;
      setDealChecklists((prev) => ({
        ...prev,
        [dealId]: (dealData.checklist as ChecklistItemDTO[]).map(mapChecklistDtoToLocal),
      }));
    },
    [mapChecklistDtoToLocal],
  );

  const initDealChecklist = useCallback(
    async (dealId: string, templateId: ChecklistTemplateId) => {
      const template = DEFAULT_CHECKLIST_TEMPLATES.find((t) => t.id === templateId);
      if (!template) return;

      try {
        const deal = await initChecklist(dealId, {
          items: template.items.map((item) => ({
            name: item.name,
            category: item.category,
            required: item.required,
            description: item.description,
            request_mode: item.requestMode,
            requested_fields: item.requestedFields,
          })),
        });
        if (deal) updateChecklistFromDeal(dealId, deal);
        toast.success(`Checklist "${template.label}" aplicado com ${template.items.length} documentos.`);
      } catch (error) {
        console.error("Erro ao inicializar checklist:", error);
        const items: DocumentChecklistItem[] = template.items.map((item, i) => ({
          id: `chk-${dealId}-${Date.now()}-${i}`,
          name: item.name,
          category: item.category,
          status: "pending",
          required: item.required,
          description: item.description,
          requestMode: item.requestMode,
          requestedFields: item.requestedFields,
        }));
        setDealChecklists((prev) => ({ ...prev, [dealId]: items }));
        toast.success(`Checklist "${template.label}" aplicado localmente (${items.length} documentos).`);
      }
    },
    [initChecklist, updateChecklistFromDeal],
  );

  const addChecklistItem = useCallback(
    async (
      dealId: string,
      item: {
        name: string;
        category: DocumentChecklistItem["category"];
        required: boolean;
        description?: string;
        partyId?: string;
        requestMode?: "upload" | "form";
        requestedFields?: ChecklistRequestedFieldDTO[];
      },
    ) => {
      try {
        const deal = await addChecklistItemDTO(dealId, {
          name: item.name,
          category: item.category,
          party_id: item.partyId,
          required: item.required,
          description: item.description,
          request_mode: item.requestMode,
          requested_fields: item.requestedFields,
        });
        if (deal) updateChecklistFromDeal(dealId, deal);
        toast.success(`"${item.name}" adicionado ao checklist.`);
      } catch (error) {
        console.error("Erro ao adicionar item:", error);
        const newItem: DocumentChecklistItem = {
          id: `chk-${dealId}-${Date.now()}`,
          name: item.name,
          category: item.category,
          partyId: item.partyId,
          status: "pending",
          required: item.required,
          description: item.description,
          requestMode: item.requestMode,
          requestedFields: item.requestedFields,
        };
        setDealChecklists((prev) => ({
          ...prev,
          [dealId]: [...(prev[dealId] || []), newItem],
        }));
        toast.success(`"${item.name}" adicionado localmente.`);
      }
    },
    [addChecklistItemDTO, updateChecklistFromDeal],
  );

  const removeChecklistItem = useCallback(
    async (dealId: string, itemId: string) => {
      // Remoção otimista: o item sai da lista imediatamente (fila e cards
      // reagem na hora), sem esperar o round-trip da API/refresh do deal.
      const removedItem = (dealChecklists[dealId] ?? []).find((i) => i.id === itemId);
      setDealChecklists((prev) => ({
        ...prev,
        [dealId]: (prev[dealId] || []).filter((i) => i.id !== itemId),
      }));

      try {
        const deal = await removeChecklistItemDTO(dealId, itemId);
        if (deal) {
          // O endpoint devolve o deal inteiro; blindamos a lista contra um
          // payload que ainda traga o item (ID divergente/legado), senão o
          // update otimista era desfeito na mesma hora.
          updateChecklistFromDeal(dealId, {
            ...deal,
            checklist: (deal.checklist ?? []).filter((i) => i.id !== itemId),
          });
        }
      } catch (error) {
        console.error("Erro ao remover item:", error);
        if (removedItem) {
          setDealChecklists((prev) => ({
            ...prev,
            [dealId]: [
              removedItem,
              ...(prev[dealId] || []).filter((i) => i.id !== itemId),
            ],
          }));
        }
      }
    },
    [dealChecklists, removeChecklistItemDTO, updateChecklistFromDeal],
  );

  const updateChecklistItemStatus = useCallback(
    async (dealId: string, itemId: string, status: ChecklistItemStatus, reason?: string) => {
      const statusMap: Record<string, string> = {
        pending: "PENDING",
        uploaded: "UPLOADED",
        approved: "APPROVED",
        rejected: "REJECTED",
      };
      try {
        const deal = await updateChecklistItemStatusDTO(dealId, itemId, {
          status: (statusMap[status] || status.toUpperCase()) as any,
          rejection_reason: reason,
        });
        if (deal) updateChecklistFromDeal(dealId, deal);
      } catch (error) {
        console.error("Erro ao atualizar status:", error);
        setDealChecklists((prev) => ({
          ...prev,
          [dealId]: (prev[dealId] || []).map((i) =>
            i.id === itemId
              ? {
                  ...i,
                  status,
                  rejectionReason: status === "rejected" ? reason : undefined,
                  reviewedAt:
                    status === "approved" || status === "rejected" ? new Date().toISOString() : i.reviewedAt,
                  uploadedAt: status === "uploaded" ? new Date().toISOString() : i.uploadedAt,
                }
              : i,
          ),
        }));
      }
    },
    [updateChecklistItemStatusDTO, updateChecklistFromDeal],
  );

  const solicitChecklistItem = useCallback(
    async (
      dealId: string,
      itemId: string,
      partyId?: string,
      requestedFields?: ChecklistRequestedFieldDTO[],
      requestMode?: "upload" | "form",
    ) => {
      try {
        const deal = await solicitChecklistItemDTO(dealId, itemId, partyId, requestedFields, requestMode);
        if (deal) updateChecklistFromDeal(dealId, deal);
        toast.success("Solicitação enviada ao cliente via Portal.");
      } catch (error) {
        console.error("Erro ao solicitar item:", error);
        setDealChecklists((prev) => ({
          ...prev,
          [dealId]: (prev[dealId] || []).map((i) =>
            i.id === itemId
              ? {
                  ...i,
                  partyId: partyId ?? i.partyId,
                  requestedFields: requestedFields ?? i.requestedFields,
                  requestMode: requestMode ?? i.requestMode,
                  solicited: true,
                  solicitedAt: new Date().toISOString(),
                }
              : i,
          ),
        }));
        toast.success("Solicitação registrada localmente.");
      }
    },
    [solicitChecklistItemDTO, updateChecklistFromDeal],
  );

  const solicitAllPending = useCallback(
    async (dealId: string, partyId?: string) => {
      const items = dealChecklists[dealId] || [];
      const pendingItems = items.filter((i) => {
        if (i.status !== "pending" || i.solicited) return false;
        // Solicitações form-only exigem parte-alvo (backend rejeita sem ela).
        if (i.requestMode === "form" && !i.partyId) return false;
        if (partyId === undefined) return true;
        if (partyId === "") return !i.partyId;
        return i.partyId === partyId;
      });

      let successCount = 0;
      for (const item of pendingItems) {
        try {
          // Consistência kit x individual: a solicitação em lote carrega a
          // MESMA configuração do item (campos + modo), preservando o que o
          // operador definiu item a item — inclusive form-only.
          await solicitChecklistItemDTO(dealId, item.id, item.partyId, item.requestedFields, item.requestMode);
          successCount++;
        } catch {
          //
        }
      }

      try {
        const deal = await fetchDealById(dealId);
        updateChecklistFromDeal(dealId, deal);
      } catch {
        const now = new Date().toISOString();
        setDealChecklists((prev) => ({
          ...prev,
          [dealId]: (prev[dealId] || []).map((i) => {
            const matches =
              i.status === "pending" &&
              !i.solicited &&
              (partyId === undefined ? true : partyId === "" ? !i.partyId : i.partyId === partyId);
            if (matches) return { ...i, solicited: true, solicitedAt: now };
            return i;
          }),
        }));
      }

      toast.success(`${successCount || "Todas as"} solicitações enviadas ao cliente.`);
    },
    [dealChecklists, fetchDealById, solicitChecklistItemDTO, updateChecklistFromDeal],
  );

  useEffect(() => {
    if (!isAuthenticated || !companyId || permissionsLoading) {
      setDealChecklists({});
      return;
    }
    setDealChecklists({});
  }, [companyId, isAuthenticated, permissionsLoading]);

  useEffect(() => {
    if (!isAuthenticated || !companyId || permissionsLoading) return;
    if (!canReadDeals) return;

    const next: Record<string, DocumentChecklistItem[]> = {};
    for (const d of dealDTOs) {
      if (d.checklist && Array.isArray(d.checklist) && d.checklist.length > 0) {
        next[d.id] = (d.checklist as ChecklistItemDTO[]).map(mapChecklistDtoToLocal);
      }
    }
    if (Object.keys(next).length === 0) return;

    setDealChecklists((prev) => ({ ...prev, ...next }));
  }, [canReadDeals, companyId, dealDTOs, isAuthenticated, mapChecklistDtoToLocal, permissionsLoading]);

  const value = useMemo<DealChecklistsContextValue>(
    () => ({
      dealChecklists,
      updateChecklistFromDeal,
      initDealChecklist,
      addChecklistItem,
      removeChecklistItem,
      updateChecklistItemStatus,
      solicitChecklistItem,
      solicitAllPending,
    }),
    [
      addChecklistItem,
      dealChecklists,
      initDealChecklist,
      removeChecklistItem,
      solicitAllPending,
      solicitChecklistItem,
      updateChecklistFromDeal,
      updateChecklistItemStatus,
    ],
  );

  return <DealChecklistsContext.Provider value={value}>{children}</DealChecklistsContext.Provider>;
}

export function useDealChecklistsContext() {
  const ctx = useContext(DealChecklistsContext);
  if (!ctx) throw new Error("useDealChecklistsContext must be used within DealChecklistsProvider");
  return ctx;
}
