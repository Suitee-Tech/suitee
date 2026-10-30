import * as React from "react";
import { AlertCircle, CheckCircle2, Pencil, Sparkles, UserRound } from "lucide-react";

import { cn } from "./utils";
import { Tooltip, TooltipContent, TooltipTrigger } from "./tooltip";

export type ProvenanceSource = "ocr" | "ai" | "manual" | "client" | "validated";

/** Forma mínima do provenance de um campo (espelha FieldProvenanceDTO). */
export interface FieldProvenanceLike {
  source: ProvenanceSource;
  confidence?: number;
  document_id?: string;
  validated_by?: string;
  validated_at?: string;
  updated_at?: string;
}

type Visual = {
  Icon: React.ComponentType<{ className?: string }>;
  color: string;
  label: string;
};

function formatDate(value?: string): string {
  if (!value) return "";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleDateString("pt-BR");
}

function resolveVisual(p: FieldProvenanceLike): Visual {
  const pct =
    typeof p.confidence === "number" ? Math.round(Math.max(0, Math.min(1, p.confidence)) * 100) : null;

  if (p.source === "validated") {
    const who = p.validated_by ? ` por ${p.validated_by}` : "";
    const when = formatDate(p.validated_at);
    return {
      Icon: CheckCircle2,
      color: "text-status-ok",
      label: `Validado${who}${when ? ` · ${when}` : ""}`,
    };
  }

  if (p.source === "client") {
    return { Icon: UserRound, color: "text-status-info", label: "Preenchido pelo cliente no Portal" };
  }

  if (p.source === "manual") {
    return { Icon: Pencil, color: "text-muted-foreground", label: "Preenchido manualmente" };
  }

  // ocr | ai → cor e mensagem por faixa de confiança.
  const conf = pct ?? 0;
  const suffix = pct !== null ? ` · ${pct}%` : "";
  if (conf >= 85) {
    return { Icon: Sparkles, color: "text-status-ok", label: `Preenchido por IA${suffix} · confira` };
  }
  if (conf >= 60) {
    return { Icon: Sparkles, color: "text-status-warning", label: `Preenchido por IA${suffix} · valide` };
  }
  return { Icon: AlertCircle, color: "text-status-critical", label: `IA${suffix} · baixa confiança, revise` };
}

/**
 * Indicador de origem do dado: ícone sufixo + tooltip com label e confiança.
 * Não renderiza nada quando não há proveniência (campo nunca tocado pela IA).
 */
export function FieldProvenanceIndicator({
  provenance,
  className,
}: {
  provenance?: FieldProvenanceLike | null;
  className?: string;
}) {
  if (!provenance) return null;
  const { Icon, color, label } = resolveVisual(provenance);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn("inline-flex items-center", color, className)}
          aria-label={label}
          role="img"
          tabIndex={0}
        >
          <Icon className="w-4 h-4" />
        </span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

/**
 * Versão textual do indicador, para deixar a origem explícita sem depender do
 * tooltip: “IA 92%”, “Validado”, “Manual” ou “Cliente”. Usada no painel da
 * parte, onde o operador precisa distinguir dado validado por OCR/IA de dado
 * editado por ele.
 */
export function FieldProvenanceBadge({
  provenance,
  className,
}: {
  provenance?: FieldProvenanceLike | null;
  className?: string;
}) {
  if (!provenance) return null;
  const { Icon, color, label } = resolveVisual(provenance);
  const pct =
    typeof provenance.confidence === "number"
      ? Math.round(Math.max(0, Math.min(1, provenance.confidence)) * 100)
      : null;
  const short =
    provenance.source === "validated"
      ? "Validado"
      : provenance.source === "manual"
        ? "Manual"
        : provenance.source === "client"
          ? "Cliente"
          : `IA${pct !== null ? ` ${pct}%` : ""}`;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center gap-1 rounded-full border border-border bg-muted/40 px-1.5 py-0.5 text-micro font-bold",
            color,
            className,
          )}
          aria-label={label}
          title={label}
          role="img"
        >
          <Icon className="h-3 w-3" />
          {short}
        </span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

/**
 * Envolve um campo (ex.: <Input/>) posicionando o indicador de origem como
 * sufixo absoluto à direita. O input base (ui/input) não tem slot de adorno, então
 * o consumidor deve adicionar padding à direita (ex.: `pr-9`) para o texto não
 * passar por baixo do ícone.
 */
export function FieldWithProvenance({
  provenance,
  children,
  className,
}: {
  provenance?: FieldProvenanceLike | null;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("relative", className)}>
      {children}
      {provenance ? (
        <span className="absolute right-2 top-1/2 -translate-y-1/2">
          <FieldProvenanceIndicator provenance={provenance} />
        </span>
      ) : null}
    </div>
  );
}
