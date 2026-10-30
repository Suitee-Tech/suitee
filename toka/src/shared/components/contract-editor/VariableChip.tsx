import { memo, useState, useCallback } from "react";
import { GripVertical, CheckCircle, AlertTriangle, Copy, ArrowRightLeft, Star, Search } from "lucide-react";

import { cn } from "../../ui/utils";
import { Tooltip, TooltipTrigger, TooltipContent } from "../../ui/tooltip";
import type { ContractVariable } from "../../contracts/contractVariables";
import { VariableStructureHelp } from "./VariableStructureHelp";

export type VariableChipStatus = "system" | "identified" | "unidentified";

interface VariableChipProps {
  variableKey: string;
  label: string;
  status: VariableChipStatus;
  category?: string;
  description?: string;
  inDocument?: boolean;
  onInsert?: (key: string) => void;
  onMap?: (key: string) => void;
  onRemap?: (key: string) => void;
  onLocate?: (key: string) => void;
  searchQuery?: string;
  variable?: ContractVariable;
  pinned?: boolean;
  onTogglePin?: (key: string) => void;
}

const statusConfig: Record<
  VariableChipStatus,
  { bg: string; text: string; border: string; hover: string; icon: React.ReactNode }
> = {
  system: {
    bg: "bg-card",
    text: "text-foreground",
    border: "border-border",
    hover: "hover:border-primary-300 hover:shadow-md hover:bg-primary-50/50",
    icon: null,
  },
  identified: {
    bg: "bg-status-ok-soft/50",
    text: "text-status-ok",
    border: "border-status-ok/30",
    hover: "hover:bg-status-ok-soft hover:border-status-ok/40 hover:shadow-sm",
    icon: <CheckCircle className="w-3.5 h-3.5 text-status-ok shrink-0" />,
  },
  unidentified: {
    bg: "bg-status-warning-soft/50",
    text: "text-status-warning",
    border: "border-status-warning/30",
    hover: "hover:bg-status-warning-soft hover:border-status-warning/40 hover:shadow-sm",
    icon: <AlertTriangle className="w-3.5 h-3.5 text-status-warning shrink-0" />,
  },
};

const highlightMatches = (text: string, query?: string) => {
  const trimmed = query?.trim();
  if (!trimmed) return text;
  const escaped = trimmed.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const parts = text.split(new RegExp(`(${escaped})`, "ig"));
  return parts.map((part, index) =>
    part.toLowerCase() === trimmed.toLowerCase() ? (
      <mark key={`${part}-${index}`} className="rounded-sm bg-primary-100 px-0.5 text-inherit">
        {part}
      </mark>
    ) : (
      part
    ),
  );
};

export const VariableChip = memo(function VariableChip({
  variableKey,
  label,
  status,
  description,
  inDocument,
  onInsert,
  onMap,
  onRemap,
  onLocate,
  searchQuery,
  variable,
  pinned = false,
  onTogglePin,
}: VariableChipProps) {
  const [isDragging, setIsDragging] = useState(false);

  const handleDragStart = useCallback(
    (e: React.DragEvent) => {
      const text = `{{${variableKey}}}`;
      e.dataTransfer.setData("text/plain", text);
      e.dataTransfer.setData("text/html", `<span>${text}</span>`);
      e.dataTransfer.effectAllowed = "copy";
      setIsDragging(true);
    },
    [variableKey],
  );

  const handleDragEnd = useCallback(() => {
    setIsDragging(false);
  }, []);

  const config = statusConfig[status];

  const handleClick = () => {
    if (status === "unidentified" && onMap) {
      onMap(variableKey);
    } else if (status === "identified" && onRemap) {
      onRemap(variableKey);
    } else if (onInsert) {
      onInsert(variableKey);
    }
  };

  const handleSecondaryAction = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (status === "identified" && onRemap) {
      onRemap(variableKey);
    } else if (status === "system" && onInsert) {
      onInsert(variableKey);
    } else if (status === "unidentified" && onMap) {
      onMap(variableKey);
    }
  };

  const primaryLabel =
    status === "system"
      ? `Copiar {{${variableKey}}}`
      : status === "unidentified"
        ? `Mapear {{${variableKey}}}`
        : `Alterar {{${variableKey}}}`;

  return (
    <div className="w-full min-w-0 space-y-1.5">
      <Tooltip>
        <TooltipTrigger asChild>
          <div
            draggable
            onDragStart={handleDragStart}
            onDragEnd={handleDragEnd}
            className={cn(
              "group flex w-full items-start gap-1.5 rounded-lg border px-2.5 py-2 text-xs font-medium shadow-sm transition-all",
              config.bg,
              config.text,
              config.border,
              config.hover,
              isDragging && "opacity-40 scale-95",
              inDocument &&
                status === "system" &&
                "ring-1 ring-primary-300 bg-primary-50 border-primary-200 shadow-none",
            )}
          >
            <GripVertical className="mt-0.5 w-3.5 h-3.5 opacity-30 group-hover:opacity-60 group-hover:text-primary-600 shrink-0 cursor-grab transition-colors" />
            <button
              type="button"
              onClick={handleClick}
              onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault();
                  handleClick();
                }
              }}
              aria-label={primaryLabel}
              className="flex min-w-0 flex-1 items-start gap-2 rounded-md text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-300"
            >
              {config.icon}
              <span className="min-w-0 flex-1 space-y-1">
                <code className="block break-all font-mono text-xs leading-relaxed">{highlightMatches(`{{${variableKey}}}`, searchQuery)}</code>
                {label !== variableKey && <span className="block text-caption leading-relaxed text-muted-foreground">{highlightMatches(label, searchQuery)}</span>}
                {description && <span className="block text-caption leading-relaxed text-muted-foreground">{highlightMatches(description, searchQuery)}</span>}
                {inDocument && status === "system" && <span className="block text-caption text-primary-700">Em uso no documento</span>}
              </span>
            </button>

            {onTogglePin && (
              <button
                type="button"
                onClick={(event) => {
                  event.stopPropagation();
                  onTogglePin(variableKey);
                }}
                aria-label={pinned ? `Desafixar {{${variableKey}}}` : `Fixar {{${variableKey}}}`}
                className={cn(
                  "shrink-0 rounded-md p-1 text-muted-foreground/60 transition-colors hover:bg-black/5 hover:text-primary-700",
                  pinned && "text-primary-700",
                )}
                title={pinned ? "Desafixar variável" : "Fixar variável"}
              >
                <Star className={cn("size-3.5", pinned && "fill-current")} />
              </button>
            )}

            {status === "system" && onInsert && (
              <button
                type="button"
                onClick={handleSecondaryAction}
                aria-label={`Copiar código {{${variableKey}}}`}
                className="shrink-0 rounded-md p-1 text-muted-foreground/60 transition-colors hover:bg-primary-50 hover:text-primary-700"
                title="Copiar código"
              >
                <Copy className="size-3.5" />
              </button>
            )}

            {status === "identified" && onLocate && (
              <button
                type="button"
                aria-label={`Localizar {{${variableKey}}} no documento`}
                onClick={(event) => {
                  event.stopPropagation();
                  onLocate(variableKey);
                }}
                className="shrink-0 rounded-md p-1 text-current transition-colors hover:bg-black/5"
                title="Localizar no documento"
              >
                <Search className="w-3.5 h-3.5" />
              </button>
            )}

            {(status === "identified" || status === "unidentified") && (
              <button
                type="button"
                aria-label={`${status === "unidentified" ? "Mapear" : "Alterar"} variável ${variableKey}`}
                onClick={handleSecondaryAction}
                className="shrink-0 rounded-md p-1 text-current transition-colors hover:bg-black/5"
              >
                <ArrowRightLeft className="w-3.5 h-3.5" />
              </button>
            )}
          </div>
        </TooltipTrigger>
        <TooltipContent side="right" sideOffset={8}>
          <div className="text-xs max-w-[200px]">
            <p className="font-semibold">{label}</p>
            {description && <p className="opacity-80 mt-0.5">{description}</p>}
            <p className="mt-1 opacity-60">
              {status === "unidentified" && "Clique para mapear a uma variável do sistema"}
              {status === "system" && "Arraste para o documento ou clique para copiar"}
              {status === "identified" && "Identificada no documento"}
            </p>
          </div>
        </TooltipContent>
      </Tooltip>
      {variable && <VariableStructureHelp variable={variable} />}
    </div>
  );
});
