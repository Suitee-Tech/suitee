import { clsx } from "clsx";

import type { SettlementEffectRow, SettlementEffectsData } from "../lib/lancamentos/settlement";

const accentClass = (accent?: SettlementEffectRow["accent"]) => {
  switch (accent) {
    case "ok":
      return "text-status-ok";
    case "critical":
      return "text-status-critical";
    case "primary":
      return "text-primary-600";
    case "muted":
    default:
      return "text-foreground";
  }
};

/**
 * Painel de "Efeitos da baixa" — mostra, de forma simples e clara, o que está
 * sendo feito (título), os destinos financeiros do valor e o efeito contábil
 * (partidas/lançamentos) antes de confirmar uma baixa/liquidação manual.
 */
export function SettlementEffects({ data }: { data: SettlementEffectsData }) {
  return (
    <div className="space-y-3 rounded-xl border border-border bg-muted/30 p-3">
      {data.effects.length > 0 && (
        <div className="space-y-1.5">
          <p className="text-xs font-bold uppercase tracking-wider text-muted-foreground">Efeitos financeiros</p>
          {data.effects.map((row) => (
            <div key={row.label} className="flex items-center justify-between gap-3">
              <span className="text-xs font-semibold text-muted-foreground">{row.label}</span>
              <span className={clsx("text-sm font-bold", accentClass(row.accent))}>{row.value}</span>
            </div>
          ))}
        </div>
      )}

      {data.accounting.length > 0 && (
        <div className="space-y-1.5">
          <p className="text-xs font-bold uppercase tracking-wider text-muted-foreground">Efeito contábil</p>
          <ul className="space-y-1 text-xs leading-relaxed text-foreground/80">
            {data.accounting.map((line) => (
              <li key={line} className="flex gap-1.5">
                <span className="text-muted-foreground">•</span>
                <span>{line}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {data.impactNote && (
        <p className="rounded-lg border border-status-info/25 bg-status-info-soft px-3 py-2 text-xs leading-relaxed text-status-info">
          {data.impactNote}
        </p>
      )}
    </div>
  );
}
