import { useCallback, useMemo, useState } from "react";

import type { Transaction } from "../types/transaction";
import type { Charge } from "../services/charge.service";
import type { OwnerTransfer } from "../services/transfer.service";
import type { ExportConfig } from "../components/ExportReportModal";

/**
 * Estado dos modais compartilhados do Financeiro, centralizado num único
 * hook tipado. Cada área (Recebimentos, Repasses, Geração) que for extraída
 * do mega-componente `Finance` passa a usar este hook em vez de manter os
 * seus próprios `useState` de modal — é a estrutura preparada para o passo
 * seguinte (sem mover os componentes de modal ainda).
 */

export type EditTxMode = "cobranca" | "repasse";

export type FinancialModalState =
  | { modal: "idle" }
  | { modal: "transaction"; transaction: Transaction }
  | { modal: "receipt"; chargeId: string }
  | { modal: "chargeDetail"; chargeId: string }
  | { modal: "editCharge"; chargeId: string }
  | { modal: "cancelCharge"; chargeId: string }
  | { modal: "markPaid"; chargeId: string; defaultAmount: number; charge?: Charge }
  | { modal: "novaCobranca" }
  | { modal: "processRepasse"; transferId: string; transfer: OwnerTransfer }
  | { modal: "editTransaction"; tx: Transaction; mode: EditTxMode }
  | { modal: "export"; config: ExportConfig }
  | { modal: "audit"; entityId: string; label: string };

export const FINANCIAL_MODAL_IDLE: FinancialModalState = { modal: "idle" };

export interface FinancialConfirmOptions {
  title: string;
  description: string;
  confirmLabel?: string;
  variant?: "danger" | "warning" | "default";
  doubleConfirm?: boolean;
  doubleConfirmLabel?: string;
  onConfirm: (value?: string) => void | Promise<void>;
  input?: { label: string; placeholder?: string; required?: boolean; minLength?: number };
}

export interface UseFinancialModals {
  state: FinancialModalState;
  /** Abre um estado de modal arbitrário (avançado). */
  open: (state: FinancialModalState) => void;
  /** Fecha qualquer modal aberto. */
  close: () => void;

  openTransaction: (transaction: Transaction) => void;
  openReceipt: (chargeId: string) => void;
  openChargeDetail: (chargeId: string) => void;
  openEditCharge: (chargeId: string) => void;
  openCancelCharge: (chargeId: string) => void;
  openMarkPaid: (chargeId: string, defaultAmount: number, charge?: Charge) => void;
  openNovaCobranca: () => void;
  openProcessRepasse: (transferId: string, transfer: OwnerTransfer) => void;
  openEditTransaction: (tx: Transaction, mode: EditTxMode) => void;
  openExport: (config: ExportConfig) => void;
  openAudit: (entityId: string, label: string) => void;

  /** Confirm dialog (ações destrutivas que pedem confirmação). */
  confirm: FinancialConfirmOptions | null;
  openConfirm: (options: FinancialConfirmOptions) => void;
  closeConfirm: () => void;
}

export function useFinancialModals(): UseFinancialModals {
  const [state, setState] = useState<FinancialModalState>(FINANCIAL_MODAL_IDLE);
  const [confirm, setConfirm] = useState<FinancialConfirmOptions | null>(null);

  const open = useCallback((next: FinancialModalState) => setState(next), []);
  const close = useCallback(() => setState(FINANCIAL_MODAL_IDLE), []);

  const openTransaction = useCallback((transaction: Transaction) => setState({ modal: "transaction", transaction }), []);
  const openReceipt = useCallback((chargeId: string) => setState({ modal: "receipt", chargeId }), []);
  const openChargeDetail = useCallback((chargeId: string) => setState({ modal: "chargeDetail", chargeId }), []);
  const openEditCharge = useCallback((chargeId: string) => setState({ modal: "editCharge", chargeId }), []);
  const openCancelCharge = useCallback((chargeId: string) => setState({ modal: "cancelCharge", chargeId }), []);
  const openMarkPaid = useCallback(
    (chargeId: string, defaultAmount: number, charge?: Charge) => setState({ modal: "markPaid", chargeId, defaultAmount, charge }),
    [],
  );
  const openNovaCobranca = useCallback(() => setState({ modal: "novaCobranca" }), []);
  const openProcessRepasse = useCallback((transferId: string, transfer: OwnerTransfer) => setState({ modal: "processRepasse", transferId, transfer }), []);
  const openEditTransaction = useCallback((tx: Transaction, mode: EditTxMode) => setState({ modal: "editTransaction", tx, mode }), []);
  const openExport = useCallback((config: ExportConfig) => setState({ modal: "export", config }), []);
  const openAudit = useCallback((entityId: string, label: string) => setState({ modal: "audit", entityId, label }), []);

  const openConfirm = useCallback((options: FinancialConfirmOptions) => setConfirm(options), []);
  const closeConfirm = useCallback(() => setConfirm(null), []);

  return useMemo(() => ({
    state, open, close,
    openTransaction, openReceipt, openChargeDetail, openEditCharge, openCancelCharge,
    openMarkPaid, openNovaCobranca, openProcessRepasse, openEditTransaction, openExport, openAudit,
    confirm, openConfirm, closeConfirm,
  }), [
    state, open, close,
    openTransaction, openReceipt, openChargeDetail, openEditCharge, openCancelCharge,
    openMarkPaid, openNovaCobranca, openProcessRepasse, openEditTransaction, openExport, openAudit,
    confirm, openConfirm, closeConfirm,
  ]);
}
