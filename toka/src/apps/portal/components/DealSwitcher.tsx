import { ChevronDown, Check } from "lucide-react";
import { useMemo, useState } from "react";
import type { DealDTO, DealStatus } from "../../../shared/services/deal.service";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "../../../shared/ui/dialog";
import { portalRoleLabel } from "../lib/portalPersona";

const STAGE_LABELS: Record<DealStatus, string> = {
  NEGOTIATION: "Em Negociação",
  PROPOSAL: "Proposta",
  ANALYSIS: "Análise",
  CONTRACT: "Contrato",
  CONVERTED: "Concluído",
  CANCELLED: "Perdido",
};

export function DealSwitcher({
  deals,
  activeDealId,
  onSelect,
  title = "Selecionar transação",
}: {
  deals: DealDTO[];
  activeDealId: string | null;
  onSelect: (id: string) => void;
  title?: string;
}) {
  const [open, setOpen] = useState(false);

  const activeDeal = useMemo(() => {
    if (!activeDealId) return null;
    return deals.find((d) => d.id === activeDealId) || null;
  }, [activeDealId, deals]);

  const label = useMemo(() => {
    if (!activeDeal) return "Selecionar transação";
    const id = activeDeal.display_id ? `#${activeDeal.display_id}` : `#${activeDeal.id.slice(0, 6)}`;
    return `Transação ${id}`;
  }, [activeDeal]);

  return (
    <>
      <button
        onClick={() => setOpen(true)}
        className="inline-flex items-center gap-2 h-10 px-4 rounded-full bg-card border border-border text-foreground/80 text-sm font-semibold shadow-sm"
      >
        <span className="truncate max-w-[220px]">{label}</span>
        {(activeDeal as any)?.my_role && (
          <span className="text-micro font-bold px-2 py-0.5 rounded-full bg-muted text-foreground/80 border border-border">
            {portalRoleLabel((activeDeal as any).my_role)}
          </span>
        )}
        <ChevronDown className="w-4 h-4 text-muted-foreground/70" />
      </button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-2 py-2">
            {deals.map((d) => {
              const selected = d.id === activeDealId;
              const id = d.display_id ? `#${d.display_id}` : `#${d.id.slice(0, 6)}`;
              const stage = STAGE_LABELS[d.status] || d.status;
              return (
                <button
                  key={d.id}
                  onClick={() => {
                    onSelect(d.id);
                    setOpen(false);
                  }}
                  className={`w-full text-left rounded-2xl border px-4 py-3 transition-colors ${
                    selected ? "border-status-ok/30 bg-status-ok-soft/40" : "border-border bg-card hover:bg-muted/50"
                  }`}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2 min-w-0">
                        <div className="text-sm font-bold text-foreground truncate">Transação {id}</div>
                        {(d as any).my_role && (
                          <span className="text-micro font-bold px-2 py-0.5 rounded-full bg-muted text-foreground/80 border border-border whitespace-nowrap">
                            {portalRoleLabel((d as any).my_role)}
                          </span>
                        )}
                      </div>
                      <div className="text-[12px] text-muted-foreground mt-0.5">{stage}</div>
                    </div>
                    {selected && <Check className="w-4 h-4 text-status-ok mt-0.5 flex-shrink-0" />}
                  </div>
                </button>
              );
            })}
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}

