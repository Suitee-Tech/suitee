import { RotateCcw } from "lucide-react";
import { usePermissionsContext } from "../context/PermissionsContext";
import type { Property } from "../data";
import { useCommissionSettings } from "../hooks/useCommissionSettings";
import {
  formatCommissionPercentage,
  formatCommissionSplit,
  resolvePropertyCommissionSummary,
} from "../lib/propertyCommission";
import { Button } from "../ui/button";

const LABEL_CLASS =
  "text-xs font-bold text-muted-foreground uppercase tracking-wider mb-2";
const HINT_CLASS = "text-xs text-muted-foreground mt-1";
const CARD_CLASS = "rounded-2xl border border-border bg-card p-5";

interface PropertyCommissionCardProps {
  /** Finalidade principal do imóvel (fallback quando `availableFor` é ambíguo). */
  listingType?: Property["listingType"];
  /** "Disponível para": venda/locação/permuta — decide o padrão do card. */
  availableFor?: Property["availableFor"];
}

/**
 * Card de comissão do detalhe do imóvel.
 *
 * O percentual total e o rateio vêm da configuração da imobiliária
 * (Configurações › Comissões e Configurações financeiras), nunca de valores
 * fixos na tela. Quando a leitura falha, o card admite a falha em vez de
 * mostrar um padrão inventado. Perfis sem leitura financeira não disparam a
 * requisição (o servidor responderia 403) e veem a explicação de acesso.
 */
export function PropertyCommissionCard({ listingType, availableFor }: PropertyCommissionCardProps) {
  const { can, isLoading: isLoadingPermissions } = usePermissionsContext();
  const canReadCommission = can("finance:read") || can("commissions:write");

  const { settings, rules, loading, error, fetching, refetch } = useCommissionSettings({
    enabled: canReadCommission,
  });

  if (isLoadingPermissions) {
    return (
      <div className={CARD_CLASS} aria-busy="true">
        <p className={LABEL_CLASS}>Comissão</p>
        <div className="h-4 w-40 rounded bg-muted animate-pulse" />
        <p className={HINT_CLASS}>Verificando acesso à configuração de comissão…</p>
      </div>
    );
  }

  if (!canReadCommission) {
    return (
      <div className={CARD_CLASS}>
        <p className={LABEL_CLASS}>Comissão</p>
        <div className="text-sm font-bold text-foreground">Definida pela imobiliária</div>
        <p className={HINT_CLASS}>
          Os percentuais são configurados pela gestão e visíveis para o financeiro.
        </p>
      </div>
    );
  }

  if (loading) {
    return (
      <div className={CARD_CLASS} aria-busy="true">
        <p className={LABEL_CLASS}>Comissão</p>
        <div className="h-4 w-40 rounded bg-muted animate-pulse" />
        <p className={HINT_CLASS}>Carregando a configuração da imobiliária…</p>
      </div>
    );
  }

  if (error || !settings) {
    return (
      <div className={CARD_CLASS}>
        <p className={LABEL_CLASS}>Comissão</p>
        <div className="text-sm font-bold text-foreground">Não foi possível carregar</div>
        <p className={HINT_CLASS}>A configuração de comissão da imobiliária não foi lida.</p>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          loading={fetching}
          onClick={refetch}
          className="mt-2 -ml-2 h-7 px-2 text-xs"
        >
          <RotateCcw className="w-3.5 h-3.5" />
          Tentar novamente
        </Button>
      </div>
    );
  }

  const summary = resolvePropertyCommissionSummary({
    listingType,
    availableFor,
    settings,
    rules,
  });
  const split = formatCommissionSplit(summary);
  const baseLabel = summary.basis === "rent" ? "do 1º aluguel" : "total";
  const sourceLabel = summary.basis === "rent" ? "locação" : "venda";

  return (
    <div className={CARD_CLASS}>
      <p className={LABEL_CLASS}>
        Comissão ({formatCommissionPercentage(summary.totalPercentage)}% {baseLabel})
      </p>
      {summary.overallocated ? (
        <>
          <div className="text-sm font-bold text-status-warning">Distribuição acima de 100%</div>
          <p className={HINT_CLASS}>
            A soma dos papéis passa de 100%. Revise os percentuais em Configurações › Comissões.
          </p>
        </>
      ) : split ? (
        <>
          <div className="text-sm font-bold text-foreground">{split}</div>
          <p className={HINT_CLASS}>Padrão da imobiliária para {sourceLabel}.</p>
        </>
      ) : (
        <>
          <div className="text-sm font-bold text-foreground">Sem distribuição por papel</div>
          <p className={HINT_CLASS}>
            Defina os percentuais de cada papel em Configurações › Comissões. Enquanto isso, a
            comissão fica consolidada na imobiliária.
          </p>
        </>
      )}
    </div>
  );
}
