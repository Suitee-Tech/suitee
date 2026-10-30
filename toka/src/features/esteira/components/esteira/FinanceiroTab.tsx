import { useMemo } from "react";
import { ArrowRight, CheckCircle2, Info, Loader2 } from "lucide-react";
import { DealDTO } from "../../../../shared/services/deal.service";
import { FinancialAccessRestricted, FinancialPreviewCard } from "./FinancialPreviewCard";
import { useContractsContext } from "../../../../shared/context/ContractsContext";
import { usePermissionsContext } from "../../../../shared/context/PermissionsContext";
import { Button } from "../../../../shared/ui/button";

interface FinanceiroTabProps {
  dealDetails: DealDTO | null;
  onRefresh: () => void;
  onGoToContract?: () => void;
}

/**
 * FinanceiroTab — visão financeira do deal no modelo manual de
 * liquidação (os recebíveis nascem PENDING e não possuem gateway).
 *
 * Mostra a projeção financeira do deal — comissões de corretor, repasse
 * ao proprietário e payouts — via FinancialPreviewCard. Ao assinar o
 * contrato, recebíveis e obrigações são CRIADOS (status PENDENTE) a partir
 * do plano aprovado; a liquidação é registrada manualmente na plataforma.
 */
export function FinanceiroTab({ dealDetails, onGoToContract }: FinanceiroTabProps) {
  const isConverted = dealDetails?.status === "CONVERTED";
  const { contracts: crmContracts } = useContractsContext();
  const { can, isLoading: isLoadingPermissions } = usePermissionsContext();

  // Resolve o contract id a partir do CRM context (indexa por dealId).
  const contractId = useMemo(() => {
    if (!dealDetails) return undefined;
    const found = crmContracts.find(
      (c: any) => c.deal_id === dealDetails.id || c.dealId === dealDetails.id,
    );
    return found?.id;
  }, [dealDetails, crmContracts]);

  if (!dealDetails) {
    return <div className="p-6 text-sm text-muted-foreground">Carregando deal...</div>;
  }

  if (isLoadingPermissions) {
    return (
      <div className="flex items-center gap-2 rounded-2xl border border-border bg-card p-6 text-sm text-muted-foreground">
        <Loader2 className="size-4 animate-spin" aria-hidden="true" />
        Verificando acesso à área financeira...
      </div>
    );
  }

  // A guia permanece visível para explicar a restrição, mas não dispara uma
  // requisição que o servidor já recusará para perfis sem finance:read.
  if (!can("finance:read")) {
    return <FinancialAccessRestricted />;
  }

  return (
    <div className="space-y-4">
      {isConverted ? (
        <div className="flex items-start gap-2 rounded-2xl border border-border bg-card p-4 text-sm">
          <CheckCircle2 className="w-4 h-4 text-status-ok mt-0.5 flex-shrink-0" />
          <div>
            <p className="font-medium text-foreground">Provisionamentos criados na assinatura</p>
            <p className="text-xs text-muted-foreground mt-1">
              Recebíveis, comissões e repasses foram registrados como <span className="font-medium">pendentes</span>.
              O pagamento e a conciliação são registrados manualmente na plataforma, sem checkout de gateway.
            </p>
          </div>
        </div>
      ) : (
        <div className="flex items-start gap-2 rounded-2xl border border-border bg-card p-4 text-sm">
          <Info className="w-4 h-4 text-primary-500 mt-0.5 flex-shrink-0" />
          <div>
            <p className="font-medium text-foreground">Prévia financeira e classificação contábil</p>
            <p className="text-xs text-muted-foreground mt-1">
              A prévia separa o que o cliente deve, a receita própria e cada obrigação com terceiros. Os registros só
              serão materializados na assinatura e começarão pendentes para liquidação manual.
            </p>
            {onGoToContract && (
              <Button variant="link" size="sm" onClick={onGoToContract} className="mt-2 h-auto gap-1 p-0 text-xs font-bold">
                Revisar plano antes da minuta <ArrowRight className="size-3.5" />
              </Button>
            )}
          </div>
        </div>
      )}

      <FinancialPreviewCard dealId={dealDetails.id} contractId={contractId} />
    </div>
  );
}
