import { useQuery } from "@tanstack/react-query";
import { portalService } from "../services/portal.service";

export interface PortalAgencyInfo {
  name: string;
  publicUrl?: string;
  color?: string;
}

function safeColor(value?: string): string | undefined {
  return value && /^#[0-9a-f]{6}$/i.test(value) ? value : undefined;
}

/**
 * Identidade pública da imobiliária de um lead/negócio.
 *
 * O portal é cross-tenant: o JWT do cliente é a conta dele, então o tenant da
 * imobiliária vem sempre explícito do relacionamento. A resposta é cacheada
 * por tenant e compartilhada entre cartões/seletor/cabeçalho.
 */
export function useAgencyInfo(tenantId?: string): {
  agency: PortalAgencyInfo;
  isLoading: boolean;
} {
  const query = useQuery({
    queryKey: ["portal", "agency-site", tenantId],
    queryFn: () => portalService.getSite(tenantId),
    enabled: Boolean(tenantId),
    staleTime: 5 * 60_000,
    retry: false,
  });

  return {
    agency: {
      name: query.data?.name?.trim() || "Imobiliária",
      publicUrl: query.data?.public_url,
      color: safeColor(query.data?.branding?.primary_color),
    },
    isLoading: query.isLoading,
  };
}
