import type { Transaction } from "../../types/transaction";

export type FinanceEditSelection = {
 tx: Transaction;
 mode: "cobranca" | "repasse";
};

export type FinanceAuditSelection = {
 entityId: string;
 label: string;
};

export type FinanceConfirmOptions = {
 title: string;
 description: string;
 confirmLabel?: string;
 variant?: "danger" | "warning" | "default";
 doubleConfirm?: boolean;
 doubleConfirmLabel?: string;
 onConfirm: (value?: string) => void | Promise<void>;
 input?: { label: string; placeholder?: string; required?: boolean; minLength?: number };
};
