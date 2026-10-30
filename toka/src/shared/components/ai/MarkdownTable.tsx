import React, { type ReactNode } from "react";

function elementName(node: ReactNode): string | undefined {
  return React.isValidElement(node) && typeof node.type === "string" ? node.type : undefined;
}

function childrenOf(node: ReactNode): React.ReactElement[] {
  return React.Children.toArray(node).filter(React.isValidElement) as React.ReactElement[];
}

function textOf(node: ReactNode): string {
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(textOf).join("");
  if (React.isValidElement(node)) return textOf((node.props as { children?: ReactNode }).children);
  return "";
}

function rowsOf(node: ReactNode): React.ReactElement[] {
  return childrenOf(node).filter((child) => elementName(child) === "tr");
}

function cellsOf(row: React.ReactElement): React.ReactElement[] {
  // O ReactMarkdown troca <td>/<th> pelos componentes customizados de
  // MessageContent (funções, não strings). A linha contém apenas células,
  // então preservamos todos os elementos filhos.
  return childrenOf((row.props as { children?: ReactNode }).children);
}

/**
 * Tabela de resposta da IA.
 *
 * Desktop mantém a tabela semântica. No mobile cada linha vira um card com
 * “rótulo: valor”, evitando a coluna cortada e a rolagem horizontal da página.
 * O conteúdo das células (links, botões e ênfases) é preservado nos dois modos.
 */
export function MarkdownTable({ children }: { children?: ReactNode }) {
  const top = childrenOf(children);
  const head = top.find((child) => elementName(child) === "thead");
  const body = top.find((child) => elementName(child) === "tbody");
  const headRows = rowsOf((head?.props as { children?: ReactNode } | undefined)?.children);
  const headerCells = headRows[0] ? cellsOf(headRows[0]) : [];
  const headers = headerCells.map((cell) => textOf((cell.props as { children?: ReactNode }).children).trim());
  const bodyRows = rowsOf((body?.props as { children?: ReactNode } | undefined)?.children);

  return (
    <div className="@container my-3 min-w-0">
      <div className="hidden overflow-x-auto rounded-lg border border-border/60 @[700px]:block">
        <table className="w-full table-fixed border-collapse text-xs">
          <caption className="sr-only">Tabela da resposta</caption>
          {children}
        </table>
      </div>

      <div className="space-y-2 @[700px]:hidden">
        {bodyRows.map((row, rowIndex) => {
          const cells = cellsOf(row);
          return (
            <div key={`ai-table-row-${rowIndex}`} className="rounded-xl border border-border/60 bg-background/70 p-3">
              {cells.map((cell, cellIndex) => (
                <div
                  key={`ai-table-cell-${rowIndex}-${cellIndex}`}
                  className="grid grid-cols-[minmax(84px,32%)_1fr] gap-2 border-b border-border/30 py-1.5 text-xs last:border-0 last:pb-0 first:pt-0"
                >
                  <span className="font-semibold text-muted-foreground">{headers[cellIndex] || `Campo ${cellIndex + 1}`}</span>
                  <span className="min-w-0 break-words text-foreground">
                    {(cell.props as { children?: ReactNode }).children}
                  </span>
                </div>
              ))}
            </div>
          );
        })}
      </div>
    </div>
  );
}
