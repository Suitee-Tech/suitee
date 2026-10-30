import { useMemo } from "react";
import { useDealsContext } from "../../../shared/context/DealsContext";
import { useLeadsContext } from "../../../shared/context/LeadsContext";
import { usePropertiesContext } from "../../../shared/context/PropertiesContext";
import type { EsteiraDeal } from "../../../shared/deals/types";
import { mapDealToEsteiraDeal } from "../utils/mapDealToEsteiraDeal";

function useEsteiraDealsData(): EsteiraDeal[] {
  const { dealDTOs } = useDealsContext();
  const { leads } = useLeadsContext();
  const { properties } = usePropertiesContext();

  return useMemo<EsteiraDeal[]>(
    () => (dealDTOs || []).map((d) => mapDealToEsteiraDeal(d, properties, leads)),
    [dealDTOs, leads, properties],
  );
}

export function useEsteiraDeals() {
  const deals = useEsteiraDealsData();
  const { dealsLoading, dealsError, refreshDeals } = useDealsContext();
  return useMemo(() => ({ deals, dealsLoading, dealsError, refreshDeals }), [deals, dealsLoading, dealsError, refreshDeals]);
}
