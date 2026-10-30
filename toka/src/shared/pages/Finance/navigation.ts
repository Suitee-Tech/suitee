// Navegação da área Financeira (extraído de Finance.tsx — P1-04/T-07):
// cada sub-módulo vive em rota própria; o mapeamento rota↔módulo fica
// centralizado aqui para o router e o estado de filtros usarem a mesma fonte.
import type { FinanceModule } from "./state";

/** Abas da área Financeira — id, rótulo e rota canônica. */
export const FINANCE_AREA_TABS: Array<{ id: FinanceModule; label: string; path: string }> = [
  { id: "overview", label: "Visão geral", path: "/financeiro" },
  { id: "lancamentos", label: "Operações", path: "/financeiro/lancamentos" },
  { id: "cobrancas", label: "Recebimentos", path: "/financeiro/cobrancas" },
  { id: "comissoes", label: "Comissões", path: "/financeiro/comissoes" },
  { id: "repasses", label: "Repasses", path: "/financeiro/repasses" },
  { id: "contas", label: "Contas", path: "/financeiro/contas" },
  { id: "conciliacao", label: "Conferência", path: "/financeiro/conciliacao" },
];

/** Mapa rota(slug) → módulo (deep-links legados). */
export const FINANCE_AREA_BY_PATH: Record<string, FinanceModule> = {
  geracao: "lancamentos",
  cobrancas: "cobrancas",
  comissoes: "comissoes",
  repasses: "repasses",
  contas: "contas",
  conciliacao: "conciliacao",
};

const FINANCE_MODULE_IDS = new Set<FinanceModule>([
  "overview",
  "cobrancas",
  "repasses",
  "contas",
  "conciliacao",
  "lancamentos",
  "split_payouts",
  "comissoes",
]);

/** Type guard de módulo financeiro (query string e rota). */
export function isFinanceModule(value: string | null): value is FinanceModule {
  return Boolean(value && FINANCE_MODULE_IDS.has(value as FinanceModule));
}
