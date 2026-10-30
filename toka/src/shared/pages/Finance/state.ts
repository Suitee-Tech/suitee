import { useReducer } from "react";
import {
  LancamentoDraft,
  Participant,
  initialLancamentoDraft,
} from "./schema";

// ---------------------------------------------------------------------------
// Filtros — consolida search/status/dropdown/dateRange dos 4 sub-módulos
// (cobrancas, repasses, conciliacao, contas) + a tab principal do
// Financeiro. Substitui ~14 useState soltos por um reducer único.
// ---------------------------------------------------------------------------

export type FinanceModule =
  | "overview"
  | "cobrancas"
  | "repasses"
  | "contas"
  | "conciliacao"
  | "lancamentos"
  | "split_payouts"
  | "comissoes";

export type FilteredTab = "cobranca" | "repasse" | "conciliacao";

export type TabFilter = { search: string; status: string; statusOpen: boolean };

export type FinanceFilters = {
  module: FinanceModule;
  // Sub-tab da view de cobranças ('todas' x 'cobrancas' [manutenções])
  cobrancaSubTab: "todas" | "cobrancas";
  cobranca: TabFilter;
  repasse: TabFilter;
  conciliacao: TabFilter;
  contas: { search: string };
  lancDate: { from: string; to: string };
};

export const initialFinanceFilters: FinanceFilters = {
  module: "overview",
  cobrancaSubTab: "todas",
  cobranca: { search: "", status: "Todos", statusOpen: false },
  repasse: { search: "", status: "Todos", statusOpen: false },
  conciliacao: { search: "", status: "Todos", statusOpen: false },
  contas: { search: "" },
  lancDate: { from: "", to: "" },
};

export type FilterAction =
  | { type: "SET_MODULE"; module: FinanceModule }
  | { type: "SET_COBRANCA_SUB_TAB"; tab: "todas" | "cobrancas" }
  | { type: "SET_TAB_SEARCH"; tab: FilteredTab | "contas"; search: string }
  | { type: "SET_TAB_STATUS"; tab: FilteredTab; status: string }
  | { type: "TOGGLE_TAB_STATUS_OPEN"; tab: FilteredTab; open?: boolean }
  | { type: "SET_LANC_DATE_FROM"; value: string }
  | { type: "SET_LANC_DATE_TO"; value: string }
  | { type: "CLEAR_LANC_DATE" };

export function financeFiltersReducer(
  state: FinanceFilters,
  action: FilterAction,
): FinanceFilters {
  switch (action.type) {
    case "SET_MODULE":
      return { ...state, module: action.module };
    case "SET_COBRANCA_SUB_TAB":
      return { ...state, cobrancaSubTab: action.tab };
    case "SET_TAB_SEARCH":
      if (action.tab === "contas") {
        return { ...state, contas: { search: action.search } };
      }
      return {
        ...state,
        [action.tab]: { ...state[action.tab], search: action.search },
      };
    case "SET_TAB_STATUS":
      return {
        ...state,
        [action.tab]: {
          ...state[action.tab],
          status: action.status,
          statusOpen: false,
        },
      };
    case "TOGGLE_TAB_STATUS_OPEN": {
      const current = state[action.tab];
      const next = action.open ?? !current.statusOpen;
      return { ...state, [action.tab]: { ...current, statusOpen: next } };
    }
    case "SET_LANC_DATE_FROM":
      return { ...state, lancDate: { ...state.lancDate, from: action.value } };
    case "SET_LANC_DATE_TO":
      return { ...state, lancDate: { ...state.lancDate, to: action.value } };
    case "CLEAR_LANC_DATE":
      return { ...state, lancDate: { from: "", to: "" } };
    default:
      return state;
  }
}

export function useFinanceFilters() {
  return useReducer(financeFiltersReducer, initialFinanceFilters);
}

// ---------------------------------------------------------------------------
// Draft do Lançamento — formulário da aba "Lançamentos & Comissões".
// Substitui 7 useState soltos por um reducer com ações explícitas. Os
// fields aqui batem 1:1 com `LancamentoSchema` em schema.ts (= payload
// do envelope persistido em localStorage).
// ---------------------------------------------------------------------------

export type DraftAction =
  | { type: "SET_TIPO"; tipo: "recebimento" | "comissao" }
  | {
    type: "SET_ORIGEM";
    origem: "venda" | "locacao";
    commissionPercent: number;
  }
  | { type: "SET_DISTRIBUTE"; value: boolean }
  | {
    type: "SET_CONTRACT";
    contractId: string;
    baseValue?: number;
    commissionPercent?: number;
    participants?: Participant[];
    paymentPlanLabel?: string;
    firstDueDate?: string;
    discountSummary?: string;
  }
  | { type: "SET_BENEFICIARY"; beneficiaryId: string; beneficiaryName: string }
  | { type: "SET_BASE_VALUE"; value: number }
  | { type: "SET_COMMISSION_PERCENT"; value: number }
  | { type: "ADD_PARTICIPANT"; participant: Participant }
  | { type: "REMOVE_PARTICIPANT"; id: number }
  | { type: "UPDATE_PARTICIPANT_PERCENT"; id: number; percent: number }
  | { type: "UPDATE_PARTICIPANT_BROKER"; id: number; brokerId: string; name: string }
  | { type: "UPDATE_PARTICIPANT_ROLE"; id: number; role: string }
  | { type: "CLEAR"; commissionPercent: number }
  | { type: "RESTORE"; draft: LancamentoDraft }
  | { type: "SET_COMMISSION_LOADING"; loading: boolean }
  | {
    type: "SET_COMMISSION_DATA";
    participants: Participant[];
    baseValue: number;
    commissionPercent: number;
    paymentPlanLabel?: string;
    firstDueDate?: string;
    discountSummary?: string;
  }
  | { type: "SET_PAYMENT_PLAN"; label: string }
  | { type: "SET_FIRST_DUE_DATE"; value: string }
  | { type: "SET_COMMISSION_ERROR"; error: string };

export function lancamentoDraftReducer(
  state: LancamentoDraft,
  action: DraftAction,
): LancamentoDraft {
  switch (action.type) {
    case "SET_TIPO":
      return { ...state, lancamentoTipo: action.tipo };
    case "SET_ORIGEM":
      return {
        ...state,
        lancamentoOrigem: action.origem,
        selectedContractId: "",
        baseValue: 0,
        commissionPercent: action.commissionPercent,
        participants: [],
      };
    case "SET_DISTRIBUTE":
      return { ...state, distributeCommission: action.value };
    case "SET_CONTRACT": {
      if (!action.contractId) {
        return {
          ...state,
          selectedContractId: "",
          selectedBeneficiaryId: "",
          selectedBeneficiaryName: "",
          baseValue: 0,
          participants: [],
          paymentPlanLabel: undefined,
          firstDueDate: undefined,
          discountSummary: undefined,
        };
      }
      return {
        ...state,
        selectedContractId: action.contractId,
        selectedBeneficiaryId: "",
        selectedBeneficiaryName: "",
        baseValue: action.baseValue ?? state.baseValue,
        commissionPercent:
          action.commissionPercent ?? state.commissionPercent,
        participants: action.participants ?? state.participants,
        paymentPlanLabel: action.paymentPlanLabel ?? state.paymentPlanLabel,
        firstDueDate: action.firstDueDate ?? state.firstDueDate,
        discountSummary: action.discountSummary ?? state.discountSummary,
      };
    }
    case "SET_BASE_VALUE":
      return { ...state, baseValue: action.value };
    case "SET_BENEFICIARY":
      return {
        ...state,
        selectedBeneficiaryId: action.beneficiaryId,
        selectedBeneficiaryName: action.beneficiaryName,
      };
    case "SET_COMMISSION_PERCENT":
      return { ...state, commissionPercent: action.value };
    case "ADD_PARTICIPANT":
      return { ...state, participants: [...state.participants, action.participant] };
    case "REMOVE_PARTICIPANT":
      return {
        ...state,
        participants: state.participants.filter((p) => p.id !== action.id),
      };
    case "UPDATE_PARTICIPANT_PERCENT":
      return {
        ...state,
        participants: state.participants.map((p) =>
          p.id === action.id ? { ...p, percent: action.percent } : p,
        ),
      };
    case "UPDATE_PARTICIPANT_BROKER":
      return {
        ...state,
        participants: state.participants.map((p) =>
          p.id === action.id
            ? { ...p, brokerId: action.brokerId || undefined, name: action.name }
            : p,
        ),
      };
    case "UPDATE_PARTICIPANT_ROLE":
      return {
        ...state,
        participants: state.participants.map((p) =>
          p.id === action.id ? { ...p, role: action.role } : p,
        ),
      };
    case "CLEAR":
      return initialLancamentoDraft(action.commissionPercent);
    case "RESTORE":
      return action.draft;
    case "SET_COMMISSION_LOADING":
      return {
        ...state,
        loadingCommission: action.loading,
        commissionDataError: action.loading ? undefined : state.commissionDataError,
      };
    case "SET_COMMISSION_DATA":
      return {
        ...state,
        loadingCommission: false,
        commissionDataLoaded: true,
        commissionDataError: undefined,
        participants: action.participants,
        baseValue: action.baseValue,
        commissionPercent: action.commissionPercent,
        paymentPlanLabel: action.paymentPlanLabel ?? state.paymentPlanLabel,
        firstDueDate: action.firstDueDate ?? state.firstDueDate,
        discountSummary: action.discountSummary ?? state.discountSummary,
      };
    case "SET_PAYMENT_PLAN":
      return { ...state, paymentPlanLabel: action.label };
    case "SET_FIRST_DUE_DATE":
      return { ...state, firstDueDate: action.value };
    case "SET_COMMISSION_ERROR":
      return {
        ...state,
        loadingCommission: false,
        commissionDataError: action.error,
        commissionDataLoaded: false,
      };
    default:
      return state;
  }
}

export function useLancamentoDraft(defaultCommissionPercent: number) {
  return useReducer(
    lancamentoDraftReducer,
    initialLancamentoDraft(defaultCommissionPercent),
  );
}
