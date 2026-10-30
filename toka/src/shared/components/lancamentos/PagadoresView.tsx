import { useMemo } from "react";
import { Users } from "lucide-react";

import { formatMoneyBR } from "../../utils/format";
import type { Lancamento } from "../../lib/lancamentos/types";

type Flow = "receber" | "pagar";

// Money the agency expects to receive vs. money it owes out. Charges and manual
// entradas are receivables; commissions, transfers, royalties and manual saídas
// are payables.
function flowOf(l: Lancamento): Flow {
  if (l.kind === "charge") return "receber";
  if (l.kind === "transaction") return l.kind_data.direction === "entrada" ? "receber" : "pagar";
  return "pagar";
}

const SETTLED = new Set(["pago", "processado", "cancelado", "revertido"]);

export interface PagadorGroup {
  key: string;
  id?: string;
  label: string;
  kind: string;
  receber: number;
  pagar: number;
  pendente: number;
  count: number;
}

export function aggregatePagadores(lancamentos: Lancamento[]): PagadorGroup[] {
  const map = new Map<string, PagadorGroup>();
  for (const l of lancamentos) {
    const idOrLabel = l.counterparty.id ?? l.counterparty.label;
    const key = `${l.counterparty.kind}:${idOrLabel}`;
    let g = map.get(key);
    if (!g) {
      g = { key, id: l.counterparty.id, label: l.counterparty.label, kind: l.counterparty.kind, receber: 0, pagar: 0, pendente: 0, count: 0 };
      map.set(key, g);
    }
    const amount = l.amount || 0;
    if (flowOf(l) === "receber") g.receber += amount;
    else g.pagar += amount;
    if (!SETTLED.has(l.status)) g.pendente += amount;
    g.count += 1;
  }
  return Array.from(map.values()).sort((a, b) => b.receber + b.pagar - (a.receber + a.pagar));
}

const KIND_LABEL: Record<string, string> = {
  tenant: "Inquilino",
  owner: "Proprietário",
  broker: "Corretor",
  partner: "Parceiro",
  external: "Externo",
  desconhecido: "Sem identificação",
};

interface PagadoresViewProps {
  lancamentos: Lancamento[];
  onSelect?: (group: PagadorGroup) => void;
}

export function PagadoresView({ lancamentos, onSelect }: PagadoresViewProps) {
  const groups = useMemo(() => aggregatePagadores(lancamentos), [lancamentos]);

  if (groups.length === 0) {
    return (
      <div className="rounded-xl border border-dashed border-border bg-card p-12 text-center">
        <p className="text-sm font-bold text-foreground">Nenhuma parte relacionada encontrada</p>
        <p className="mt-1 text-xs text-muted-foreground">Pagadores e beneficiários aparecem conforme os lançamentos são criados.</p>
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded-xl border border-border bg-card">
      <table className="w-full text-sm">
        <thead className="bg-muted/40 text-xs text-muted-foreground">
          <tr>
            <th className="px-4 py-3 text-left font-bold">Parte relacionada</th>
            <th className="px-4 py-3 text-right font-bold">A receber</th>
            <th className="px-4 py-3 text-right font-bold">A pagar</th>
            <th className="px-4 py-3 text-right font-bold">Em aberto</th>
            <th className="px-4 py-3 text-right font-bold">Itens</th>
          </tr>
        </thead>
        <tbody>
          {groups.map((g) => (
            <tr
              key={g.key}
              className="cursor-pointer border-t border-border hover:bg-muted/30"
              onClick={() => onSelect?.(g)}
            >
              <td className="px-4 py-3">
                <div className="flex items-center gap-2">
                  <Users className="h-4 w-4 text-muted-foreground" />
                  <div className="min-w-0">
                    <div className="truncate font-bold text-foreground">{g.label}</div>
                    <div className="text-xs text-muted-foreground">{KIND_LABEL[g.kind] ?? g.kind}</div>
                  </div>
                </div>
              </td>
              <td className="px-4 py-3 text-right text-status-ok">{g.receber > 0 ? formatMoneyBR(g.receber, { withSymbol: true }) : "—"}</td>
              <td className="px-4 py-3 text-right text-status-info">{g.pagar > 0 ? formatMoneyBR(g.pagar, { withSymbol: true }) : "—"}</td>
              <td className="px-4 py-3 text-right text-status-warning">{g.pendente > 0 ? formatMoneyBR(g.pendente, { withSymbol: true }) : "—"}</td>
              <td className="px-4 py-3 text-right text-muted-foreground">{g.count}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
