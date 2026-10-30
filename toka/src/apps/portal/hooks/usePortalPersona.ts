import { useMemo } from "react";

import { usePortalDeals } from "../context/PortalDealsContext";
import { portalCopy, resolvePortalPersona, type PortalCopy, type PortalPersona } from "../lib/portalPersona";

/**
 * Persona + dicionário de textos do cliente no portal.
 *
 * A verdade vem do negócio ativo (`my_role` + `type`); antes disso — cliente
 * convidado que ainda não tem negócio na esteira — vale o papel do lead no
 * atendimento (`PortalOverviewLead.role`), que a tela informa.
 */
export function usePortalPersona(options?: { leadRole?: string | null }): {
  persona: PortalPersona;
  copy: PortalCopy;
} {
  const { activeDeal } = usePortalDeals();
  const role = (activeDeal as { my_role?: string } | null)?.my_role || options?.leadRole || null;
  const dealType = (activeDeal as { type?: string } | null)?.type ?? null;

  return useMemo(() => {
    const persona = resolvePortalPersona({ role, dealType });
    return { persona, copy: portalCopy(persona) };
  }, [role, dealType]);
}
