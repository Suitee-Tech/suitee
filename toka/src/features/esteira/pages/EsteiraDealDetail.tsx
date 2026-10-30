import { useEffect, useMemo } from "react";
import { useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { Loader2 } from "lucide-react";
import { EsteiraDealDetails } from "../components/EsteiraDealDetails";
import { Button } from "../../../shared/ui/button";
import type { DealStage, EsteiraDeal } from "../../../shared/deals/types";
import { useEsteiraDeals } from "../hooks/useEsteiraDeals";
import { useDealQuery } from "../../../shared/deals/hooks";
import { useLeadQuery } from "../../../shared/leads/hooks";
import { usePropertyQuery } from "../../../shared/properties/hooks";

const dealStatusToStage: Record<string, DealStage> = {
  NEGOTIATION: "Em Negociação",
  PROPOSAL: "Proposta",
  ANALYSIS: "Análise",
  CONTRACT: "Contrato",
  CONVERTED: "Concluído",
  CANCELLED: "Perdido",
};

export function EsteiraDealDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { deals } = useEsteiraDeals();

  const { data: dealDto, isLoading: dealLoading } = useDealQuery(id);
  const { data: lead } = useLeadQuery(dealDto?.customer_id);
  const { data: property, isError: propertyError } = usePropertyQuery(dealDto?.property_id);

  const dealFromContext = useMemo(() => deals.find((d) => d.id === id) ?? null, [deals, id]);

  useEffect(() => {
    if (!propertyError) return;
    toast.error("Não foi possível carregar o imóvel do negócio.");
  }, [propertyError]);

  const contract = useMemo<EsteiraDeal | null>(() => {
    if (dealFromContext) return dealFromContext;
    if (!dealDto) return null;

    return {
      id: dealDto.id,
      displayId: dealDto.display_id,
      client: lead?.name ?? "Cliente Desconhecido",
      client_email: lead?.email ?? "",
      client_phone: lead?.phone ?? "",
      client_doc: lead?.document ?? "",
      clientId: dealDto.customer_id,
      property: property?.title ?? "Imóvel Desconhecido",
      propertyId: dealDto.property_id,
      type: dealDto.type === "SALE" ? "Venda" : "Locação",
      value: dealDto.value,
      stage: dealStatusToStage[dealDto.status] ?? "Em Negociação",
      date: new Date(dealDto.updated_at).toLocaleDateString("pt-BR"),
      created_at: dealDto.created_at,
      alerts: [],
      avatarUrl: lead?.avatar ?? "",
      brokerId: dealDto.broker_id,
    };
  }, [dealFromContext, dealDto, lead, property]);

  if (dealLoading && !contract) {
    return (
      <div className="p-8 flex items-center justify-center text-muted-foreground">
        <Loader2 className="w-4 h-4 animate-spin mr-2" />
        Carregando negócio...
      </div>
    );
  }

  if (!id || (!dealLoading && !contract)) {
    return (
      <div className="p-8 flex flex-col items-center text-center text-muted-foreground">
        <p>Negócio não encontrado.</p>
        <Button onClick={() => navigate("/esteira")} className="mt-4">
          Voltar
        </Button>
      </div>
    );
  }

  return (
    <div className="h-full">
      {contract && <EsteiraDealDetails contract={contract} onClose={() => navigate("/esteira")} />}
    </div>
  );
}
