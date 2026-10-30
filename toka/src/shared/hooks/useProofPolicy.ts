import { useQuery } from "@tanstack/react-query";
import { tenantFinancialSettingsService } from "../services/tenantFinancialSettings.service";

/**
 * Política de comprovante das baixas manuais, lida da configuração do tenant.
 *
 * O backend é quem decide (`requireManualProof` nos handlers) — este hook
 * apenas antecipa a mesma regra na interface, para o operador ver a exigência
 * antes de tentar concluir, em vez de receber um 400 depois de enviar o
 * arquivo. A regra de negócio continua sendo do servidor.
 *
 * Em caso de falha na leitura assume `true`, igual ao backend: na dúvida,
 * exigir. Assumir o contrário liberaria visualmente uma confirmação que o
 * servidor vai recusar.
 */
export function useProofPolicy() {
  const query = useQuery({
    queryKey: ["tenant-financial-settings", "proof-policy"],
    queryFn: () => tenantFinancialSettingsService.get(),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  });

  return {
    proofRequired: query.data?.payout_proof_required ?? true,
    loading: query.isLoading,
  };
}
