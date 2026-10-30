import type { ReactNode } from "react";

import { Tooltip, TooltipContent, TooltipTrigger } from "./tooltip";
import { cn } from "./utils";

interface TooltipTextProps {
  /** Texto completo — o motivo do tooltip existir quando a célula trunca. */
  text: string;
  /** Conteúdo já renderizado; por padrão, o próprio texto. */
  children?: ReactNode;
  className?: string;
  /** Lado preferido do balão, para não cobrir a coluna vizinha. */
  side?: "top" | "right" | "bottom" | "left";
}

/**
 * Texto truncado com o conteúdo completo acessível por ponteiro E por foco.
 *
 * O atributo `title` nativo não serve para isso: não padroniza atraso nem
 * aparência, não aparece no foco do teclado e não existe em toque. O gatilho é
 * focável de propósito — sem `tabIndex` o tooltip continuaria inalcançável para
 * quem navega por teclado, que é justamente o caso que o `title` não cobria.
 *
 * Use apenas quando a informação for essencial. Texto decorativo ou já visível
 * por inteiro não precisa de tooltip.
 */
export function TooltipText({ text, children, className, side = "top" }: TooltipTextProps) {
  if (!text) return <>{children ?? null}</>;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={0} className={cn("cursor-help outline-none focus-visible:ring-2 focus-visible:ring-ring rounded-sm", className)}>
          {children ?? text}
        </span>
      </TooltipTrigger>
      <TooltipContent side={side}>
        <span className="break-all">{text}</span>
      </TooltipContent>
    </Tooltip>
  );
}
