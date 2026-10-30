import type { ReactNode } from "react";

import { cn } from "./utils";

type PageHeading = "h1" | "h2";

export interface PageHeaderProps {
  title: ReactNode;
  description?: ReactNode;
  /** Ícone decorativo antes do título (aria-hidden). */
  icon?: ReactNode;
  /** Ação primária e ações secundárias da tela. */
  children?: ReactNode;
  /** `h2` para cabeçalhos embutidos em outra página. */
  as?: PageHeading;
  className?: string;
  actionsClassName?: string;
}

/**
 * Cabeçalho de página do admin.
 *
 * Antes cada tela repetia a própria combinação de `text-lg/xl/2xl/3xl`, com e
 * sem `tracking-tight`, e um wrapper de ações diferente. O contrato aqui é:
 * título + descrição à esquerda, ações à direita, quebrando para coluna no
 * mobile. Não renderiza botão de voltar por conta própria: telas que precisam
 * disso continuam controlando a navegação.
 */
export function PageHeader({
  title,
  description,
  icon,
  children,
  as: Heading = "h1",
  className,
  actionsClassName,
}: PageHeaderProps) {
  return (
    <header
      className={cn(
        "flex flex-col items-start justify-between gap-4 sm:flex-row sm:items-center",
        className,
      )}
    >
      <div className="flex min-w-0 items-start gap-3">
        {icon ? (
          <span aria-hidden="true" className="mt-0.5 shrink-0 text-primary-600">
            {icon}
          </span>
        ) : null}
        <div className="min-w-0">
          <Heading className="text-2xl font-bold tracking-tight text-foreground">{title}</Heading>
          {description ? (
            <p className="mt-1 text-sm text-muted-foreground">{description}</p>
          ) : null}
        </div>
      </div>
      {children ? (
        <div
          className={cn(
            "flex w-full flex-wrap items-center gap-2 sm:w-auto sm:gap-3",
            actionsClassName,
          )}
        >
          {children}
        </div>
      ) : null}
    </header>
  );
}
