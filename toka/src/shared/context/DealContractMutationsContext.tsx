import { createContext, useCallback, useContext, useMemo, type ReactNode } from "react";
import { toast } from "sonner";
import { contractService } from "../services/contract.service";
import type { DealDTO } from "../services/deal.service";
import type { Property } from "../data";
import type { Contract, CreateContractRequest } from "../contracts/types";

type DealContractMutationsContextValue = {
  createContractFromDeal: (dealId: string, data?: CreateContractRequest) => Promise<Contract>;
};

const DealContractMutationsContext = createContext<DealContractMutationsContextValue | null>(null);

export function DealContractMutationsProvider({
  children,
  fetchDealById,
  updatePropertyStatus,
  moveDeal,
  addContract,
}: {
  children: ReactNode;
  fetchDealById: (dealId: string) => Promise<DealDTO | null | undefined>;
  updatePropertyStatus: (id: string, status: Property["status"]) => Promise<boolean>;
  moveDeal: (dealId: string, stage: any) => Promise<unknown>;
  addContract: (contract: Contract) => void;
}) {
  const createContractFromDeal = useCallback(
    async (dealId: string, data?: CreateContractRequest): Promise<Contract> => {
      const deal = await fetchDealById(dealId);
      if (!deal) throw new Error("Deal not found");

      const signers = deal.signers || [];

      const contractData: CreateContractRequest = data || {
        deal_id: dealId,
        type: deal.type === "SALE" ? "venda" : "locacao",
        signatories: signers.map((s) => ({
          id: s.id,
          name: s.name,
          email: s.email,
          role: String(s.role || "").toLowerCase(),
        })),
        content: `Contrato de ${deal.type === "SALE" ? "Venda e Compra" : "Locação"} referente ao imóvel...`,
        start_date: new Date().toISOString(),
      };

      // Failures from contract creation must reach the caller so it can show
      // the backend validation message instead of a generic success/failure mix.
      const newApiContract = await contractService.create(contractData);
      addContract(newApiContract);

      // These are post-create projections. A failure here must not make the UI
      // report that contract creation failed (or encourage a duplicate retry).
      try {
        await moveDeal(dealId, "Concluído" as any);

        if (deal.property_id) {
          await updatePropertyStatus(deal.property_id, deal.type === "SALE" ? "Vendido" : "Alugado");
        }
      } catch (error) {
        console.error("Contrato criado, mas houve falha ao atualizar a esteira ou o imóvel:", error);
        toast.warning("Contrato criado, mas a etapa ou o imóvel não foi atualizado automaticamente.");
      }

      return newApiContract;
    },
    [addContract, fetchDealById, moveDeal, updatePropertyStatus],
  );

  const value = useMemo<DealContractMutationsContextValue>(() => ({ createContractFromDeal }), [createContractFromDeal]);
  return <DealContractMutationsContext.Provider value={value}>{children}</DealContractMutationsContext.Provider>;
}

export function useDealContractMutationsContext() {
  const ctx = useContext(DealContractMutationsContext);
  if (!ctx) throw new Error("useDealContractMutationsContext must be used within DealContractMutationsProvider");
  return ctx;
}
