import type { ReactNode } from "react";
import { DealsBootstrapProvider } from "../../shared/context/DealsBootstrapContext";
import { EsteiraDealPipelineProvider } from "../../shared/context/EsteiraDealPipelineContext";
import { useEsteiraDealPipelineContext } from "../../shared/context/EsteiraDealPipelineContext";
import { DealContractMutationsProvider as SharedDealContractMutationsProvider } from "../../shared/context/DealContractMutationsContext";
import { useContractsContext } from "../../shared/context/ContractsContext";
import { useDealsContext } from "../../shared/context/DealsContext";
import { usePropertiesContext } from "../../shared/context/PropertiesContext";

export function EsteiraRouteProviders({ children }: { children: ReactNode }) {
  return (
    <DealsBootstrapProvider>
      <EsteiraDealPipelineProvider>
        <DealContractMutationsBridge>{children}</DealContractMutationsBridge>
      </EsteiraDealPipelineProvider>
    </DealsBootstrapProvider>
  );
}

function DealContractMutationsBridge({ children }: { children: ReactNode }) {
  const { fetchDealById } = useDealsContext();
  const { updatePropertyStatus } = usePropertiesContext();
  const { moveDeal } = useEsteiraDealPipelineContext();
  const { addContract } = useContractsContext();

  return (
    <SharedDealContractMutationsProvider
      fetchDealById={fetchDealById}
      updatePropertyStatus={updatePropertyStatus}
      moveDeal={moveDeal}
      addContract={addContract}
    >
      {children}
    </SharedDealContractMutationsProvider>
  );
}
