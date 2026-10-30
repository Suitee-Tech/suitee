import type { OperationViewMode } from "../components/lancamentos/OperationsExplorer";

export type { OperationViewMode };

// A planilha é o modo PADRÃO de /financeiro/lancamentos: é onde o operador
// confere linha a linha, ordena por coluna e vê os totais. O cartão continua
// disponível explicitamente em `?view=cards`.
export function parseOperationView(value: string | null): OperationViewMode {
  return value === "cards" ? "cards" : "sheet";
}
