"use client";

import { type ReactNode } from "react";
import { Button } from "./button";
import { clsx } from "clsx";

import { useIsAtLeastMd } from "../hooks/useMediaQuery";

export interface ResponsiveTableColumn<T> {
  key: string;
  header: ReactNode;
  cell: (row: T) => ReactNode;
  className?: string;
  primary?: boolean;
  hideOnMobile?: boolean;
}

export interface ResponsiveTableGroup<T> {
  key: string;
  label?: ReactNode;
  data: T[];
}

export interface ResponsiveTableProps<T> {
  data: T[];
  /** Quando presente, agrupa as linhas/cards com um cabeçalho por grupo. */
  groups?: ResponsiveTableGroup<T>[];
  columns: ResponsiveTableColumn<T>[];
  getRowKey: (row: T) => string;
  mobileCard?: (row: T) => ReactNode;
  empty?: ReactNode;
  onRowClick?: (row: T) => void;
  getRowLabel?: (row: T) => string;
  className?: string;
  forceLayout?: "table" | "cards";
  /** Nome acessível da tabela (caption). */
  caption?: string;
  /** Alternativa ao caption para tabelas sem caption textual. */
  ariaLabel?: string;
}

export function ResponsiveTable<T>({
  data,
  groups,
  columns,
  getRowKey,
  mobileCard,
  empty,
  onRowClick,
  getRowLabel,
  className,
  forceLayout,
  caption,
  ariaLabel,
}: ResponsiveTableProps<T>) {
  const isAtLeastMd = useIsAtLeastMd();
  // Em SSR/jsdom não há matchMedia: a tabela é o fallback semântico. Sem isso,
  // testes e o primeiro render escondem a tabela e mostram cards.
  const hasMediaQuery = typeof window !== "undefined" && typeof window.matchMedia === "function";
  const useTableLayout = forceLayout ? forceLayout === "table" : !hasMediaQuery || isAtLeastMd;

  const action = (row: T) => onRowClick ? (
    <Button variant="ghost" size="sm" aria-label={`Abrir ${getRowLabel?.(row) ?? "registro"}`} onClick={() => onRowClick(row)}>Abrir</Button>
  ) : null;

  const hasGroups = Boolean(groups && groups.length > 0);
  const resolvedGroups = hasGroups
    ? groups!
    : [{ key: "__all", data }];
  const totalRows = resolvedGroups.reduce((total, group) => total + group.data.length, 0);
  const columnCount = columns.length + (onRowClick ? 1 : 0);

  if (totalRows === 0) {
    return (
      <div
        className={clsx(
          "rounded-xl border border-dashed border-border bg-card p-6 text-center text-sm text-muted-foreground",
          className,
        )}
      >
        {empty ?? "Nenhum registro."}
      </div>
    );
  }

  if (useTableLayout) {
    return (
      <div
        className={clsx(
          "rounded-xl border border-border bg-card overflow-hidden",
          className,
        )}
      >
        <div className="overflow-x-auto">
          <table className="w-full text-sm" aria-label={ariaLabel}>
            {caption ? <caption className="sr-only">{caption}</caption> : null}
            <thead>
              <tr className="border-b border-border bg-muted/40">
                {columns.map((c) => (
                  <th
                    key={c.key}
                    scope="col"
                    className={clsx(
                      "text-left px-3 py-2 text-micro font-bold text-muted-foreground uppercase tracking-wide",
                      c.className,
                    )}
                  >
                    {c.header}
                  </th>
                ))}
                {onRowClick && <th scope="col" className="px-3 py-2"><span className="sr-only">Ações</span></th>}
              </tr>
            </thead>
            {resolvedGroups.map((group) => (
              <tbody key={group.key}>
                {group.label != null && (
                  <tr className="border-b border-border bg-muted/30">
                    <th scope="colgroup" colSpan={columnCount} className="px-3 py-2 text-left text-micro font-bold uppercase tracking-wide text-muted-foreground">
                      {group.label}
                    </th>
                  </tr>
                )}
                {group.data.map((row) => (
                  <tr
                    key={getRowKey(row)}
                    className={clsx(
                      "border-b border-border last:border-0",
                      onRowClick &&
                        "cursor-pointer hover:bg-muted/40 transition-colors",
                    )}
                    onClick={onRowClick ? (event) => {
                      if (!(event.target as Element).closest('button, a, input, select, textarea, [role="button"]')) onRowClick(row);
                    } : undefined}
                  >
                    {columns.map((c) => (
                      <td
                        key={c.key}
                        className={clsx(
                          "px-3 py-2 align-middle text-foreground",
                          c.className,
                        )}
                      >
                        {c.cell(row)}
                      </td>
                    ))}
                    {onRowClick && <td className="px-3 py-2">{action(row)}</td>}
                  </tr>
                ))}
              </tbody>
            ))}
          </table>
        </div>
      </div>
    );
  }

  return (
    <div className={clsx("space-y-4", className)}>
      {totalRows === 0 ? (
        <div className="rounded-xl border border-dashed border-border bg-card p-6 text-center text-sm text-muted-foreground">
          {empty ?? "Nenhum registro."}
        </div>
      ) : resolvedGroups.map((group) => (
        <section key={group.key} aria-label={typeof group.label === "string" ? group.label : undefined}>
          {group.label != null && (
            <h3 className="mb-2 text-xs font-bold uppercase tracking-wide text-muted-foreground">{group.label}</h3>
          )}
          <div className="space-y-2">
            {group.data.map((row) => {
              if (mobileCard) {
                const card = mobileCard(row);
                return (
                  <div key={getRowKey(row)}>
                    {card}
                    {action(row)}
                  </div>
                );
              }
              return (
                <AutoMobileCard
                  key={getRowKey(row)}
                  row={row}
                  columns={columns}
                  action={action(row)}
                />
              );
            })}
          </div>
        </section>
      ))}
    </div>
  );
}

function AutoMobileCard<T>({
  row,
  columns,
  action,
}: {
  row: T;
  columns: ResponsiveTableColumn<T>[];
  action?: ReactNode;
}) {
  const visibleColumns = columns.filter((c) => !c.hideOnMobile);
  const primary = visibleColumns.find((c) => c.primary) ?? visibleColumns[0];
  const rest = visibleColumns.filter((c) => c.key !== primary?.key);

  return (
    <article
      className={clsx(
        "rounded-xl border border-border bg-card p-3 flex flex-col gap-2",

      )}
    >
      {primary && (
        <header className="text-title font-bold text-foreground truncate">
          {primary.cell(row)}
        </header>
      )}
      {rest.length > 0 && (
        <dl className="grid grid-cols-2 gap-x-3 gap-y-2">
          {rest.map((c) => (
            <div key={c.key} className="min-w-0">
              <dt className="text-micro font-bold text-muted-foreground uppercase tracking-wide truncate">
                {c.header}
              </dt>
              <dd className="text-caption font-semibold text-foreground/90 truncate">
                {c.cell(row)}
              </dd>
            </div>
          ))}
        </dl>
      )}
      {action}
    </article>
  );
}
