import { useQuery } from "@tanstack/react-query";
import { commissionRulesService, type CommissionRule } from "../services/commissionRules.service";
import {
  tenantFinancialSettingsService,
  type TenantFinancialSettings,
} from "../services/tenantFinancialSettings.service";

export interface CommissionSettingsSnapshot {
  settings: TenantFinancialSettings | null;
  rules: CommissionRule[];
  loading: boolean;
  /** `true` quando a leitura falhou: a UI não deve inventar percentuais. */
  error: boolean;
  fetching: boolean;
  refetch: () => void;
}

/**
 * Configuração de comissão do tenant (percentual padrão + distribuição por
 * papel), lida de `/financial-settings` e `/commission-rules`.
 *
 * As duas leituras são feitas juntas porque a tela de comissão só faz sentido
 * com o conjunto completo — o percentual total vem das configurações
 * financeiras e o rateio vem das regras de comissão.
 *
 * `enabled: false` evita disparar as requisições para perfis sem permissão de
 * leitura financeira (o backend responderia 403).
 */
export function useCommissionSettings(
  options: { enabled?: boolean } = {},
): CommissionSettingsSnapshot {
  const enabled = options.enabled ?? true;

  const query = useQuery({
    queryKey: ["tenant-financial-settings", "commission"],
    queryFn: async () => {
      const [settings, rules] = await Promise.all([
        tenantFinancialSettingsService.get(),
        commissionRulesService.get(),
      ]);
      return { settings, rules: Array.isArray(rules) ? rules : [] };
    },
    enabled,
    staleTime: 5 * 60 * 1000,
    retry: 1,
  });

  return {
    settings: query.data?.settings ?? null,
    rules: query.data?.rules ?? [],
    loading: enabled && query.isLoading,
    error: enabled && query.isError,
    fetching: query.isFetching,
    refetch: () => {
      void query.refetch();
    },
  };
}
