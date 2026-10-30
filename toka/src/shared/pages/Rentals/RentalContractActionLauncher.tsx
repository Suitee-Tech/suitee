import { useDeferredValue, useMemo, useState } from "react";
import { Building2, Droplet, FileText, Search, Settings2 } from "lucide-react";
import type { Contract } from "../../contracts/types";
import { EmptyState } from "../../ui/EmptyState";
import { ExpandableSidePanel } from "../../components/ExpandableSidePanel";
import { formatMoneyBR } from "../../utils/format";

export type RentalHeaderAction = "plan" | "account" | "transfer";

interface RentalContractActionLauncherProps {
  action: RentalHeaderAction | null;
  contracts: Contract[];
  onClose: () => void;
  onSelect: (contract: Contract, action: RentalHeaderAction) => void;
}

const ACTION_COPY: Record<
  RentalHeaderAction,
  { title: string; description: string; button: string; icon: typeof Settings2 }
> = {
  plan: {
    title: "Configurar plano financeiro",
    description: "Escolha a locação para revisar calendário, pro-rata, taxas, composição mensal e emissão.",
    button: "Configurar plano",
    icon: Settings2,
  },
  account: {
    title: "Vincular conta à locação",
    description: "Escolha o contrato correto antes de definir responsável e efeito financeiro da conta.",
    button: "Vincular conta",
    icon: Droplet,
  },
  transfer: {
    title: "Preparar repasse da locação",
    description: "O repasse nasce da quitação integral de uma cobrança com composição financeira. Escolha a locação para conferir os recebimentos.",
    button: "Abrir recebimentos",
    icon: FileText,
  },
};

export function RentalContractActionLauncher({
  action,
  contracts,
  onClose,
  onSelect,
}: RentalContractActionLauncherProps) {
  const [search, setSearch] = useState("");
  const deferredSearch = useDeferredValue(search);
  const copy = action ? ACTION_COPY[action] : ACTION_COPY.plan;
  const Icon = copy.icon;

  const filteredContracts = useMemo(() => {
    const term = deferredSearch.trim().toLocaleLowerCase("pt-BR");
    if (!term) return contracts;
    return contracts.filter((contract) => {
      const haystack = `${contract.client ?? ""} ${contract.property_title ?? ""} ${contract.display_id ?? contract.id}`
        .toLocaleLowerCase("pt-BR");
      return haystack.includes(term);
    });
  }, [contracts, deferredSearch]);

  return (
    <ExpandableSidePanel
      open={Boolean(action)}
      onClose={() => {
        setSearch("");
        onClose();
      }}
      defaultSize="2xl"
      expandedSize="4xl"
      bgClass="bg-muted"
      renderHeader={({ expandButton, closeButton }) => (
        <div className="flex items-center justify-between gap-4 border-b border-border bg-card px-6 py-4">
          <div className="flex min-w-0 items-center gap-3">
            <div className="rounded-xl bg-primary-50 p-2 text-primary-700">
              <Icon className="size-5" />
            </div>
            <div className="min-w-0">
              <h2 className="truncate text-lg font-bold text-foreground">{copy.title}</h2>
              <p className="text-sm text-muted-foreground">Selecione uma locação ativa</p>
            </div>
          </div>
          <div className="flex items-center gap-2">{expandButton}{closeButton}</div>
        </div>
      )}
    >
      <div className="flex-1 overflow-y-auto p-6">
        <div className="mb-5 rounded-xl border border-primary-100 bg-primary-50 p-4 text-sm text-primary-800">
          {copy.description}
        </div>

        <div className="relative mb-5">
          <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <input
            autoFocus
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Buscar por inquilino, imóvel ou número do contrato…"
            className="h-11 w-full rounded-xl border border-border bg-card pl-10 pr-4 text-sm outline-none transition focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
          />
        </div>

        {filteredContracts.length === 0 ? (
          <EmptyState
            icon={Building2}
            title="Nenhuma locação encontrada"
            message={search ? "Revise a busca ou escolha outro contrato." : "Não há contratos ativos disponíveis para esta ação."}
          />
        ) : (
          <div className="space-y-3">
            {filteredContracts.map((contract) => (
              <button
                key={contract.id}
                type="button"
                onClick={() => {
                  if (!action) return;
                  setSearch("");
                  onSelect(contract, action);
                }}
                className="group flex w-full items-center justify-between gap-4 rounded-xl border border-border bg-card p-4 text-left shadow-sm transition hover:border-primary-300 hover:bg-primary-50/40 focus:outline-none focus:ring-2 focus:ring-primary-600/20"
              >
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-bold text-foreground">{contract.client || "Inquilino não informado"}</span>
                    <span className="rounded-md bg-muted px-2 py-0.5 text-caption font-bold text-muted-foreground">
                      {contract.display_id ?? contract.id.slice(0, 8)}
                    </span>
                  </div>
                  <p className="mt-1 truncate text-xs text-muted-foreground">{contract.property_title || "Imóvel não informado"}</p>
                </div>
                <div className="flex shrink-0 items-center gap-4">
                  <div className="text-right">
                    <p className="text-caption uppercase tracking-wide text-muted-foreground">Aluguel</p>
                    <p className="text-sm font-bold text-foreground">{formatMoneyBR(contract.value || 0)}</p>
                  </div>
                  <span className="inline-flex h-9 items-center rounded-lg bg-primary px-3 text-xs font-bold text-primary-foreground transition group-hover:bg-primary/90">
                    {copy.button}
                  </span>
                </div>
              </button>
            ))}
          </div>
        )}
      </div>
    </ExpandableSidePanel>
  );
}
