import { listAllCharges, listAllTransfers, listAllTransactions } from "../services/financialLists";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { toast } from "sonner";
import type { Transaction } from "../types/transaction";
import { coerceMoneyBR } from "../utils/format";
import { useAuth } from "./AuthContext";
import { usePermissionsContext } from "./PermissionsContext";
import { useAuditLogContext } from "./AuditLogContext";
import { retry } from "../lib/retry";
import { serverMessage } from "../lib/serverMessage";
import { unwrapApiData } from "../services/api";
import { chargeService, type Charge, type CreateChargeDTO, type MarkPaidDTO, type UpdateChargeDTO } from "../services/charge.service";
import { transferService, type OwnerTransfer, type CreateTransferDTO } from "../services/transfer.service";
import { collectionService, type Collection, type CreateCollectionDTO, type UpdateCollectionDTO, type AddActionDTO } from "../services/collection.service";
import { transactionService, type TransactionDTO, type CreateTransactionDTO, type CreateTransactionBatchItem, type ManualCommissionBatchItem, type ManualCommissionRetention, type UpdateTransactionDTO } from "../services/transaction.service";
import { useDataChangeRefresh } from "../hooks/useDataChangeRefresh";
import { useRequestScopeGuard } from "../hooks/useRequestScopeGuard";

type FinanceLoading = {
  transactions: boolean;
  charges: boolean;
  overdueCharges: boolean;
  transfers: boolean;
  collections: boolean;
};

type FinanceErrors = {
  transactions: unknown | null;
  charges: unknown | null;
  overdueCharges: unknown | null;
  transfers: unknown | null;
  collections: unknown | null;
};

type FinanceContextValue = {
  transactions: Transaction[];
  addTransaction: (t: Transaction) => Promise<void>;
  addTransactionBatch: (items: CreateTransactionBatchItem[], operationID: string, commissions?: ManualCommissionBatchItem[], commissionRetention?: ManualCommissionRetention) => Promise<Transaction[]>;
  updateTransaction: (id: string, updates: Partial<Transaction>) => Promise<boolean>;
  deleteTransaction: (id: string) => Promise<void>;

  charges: Charge[];
  overdueCharges: Charge[];
  addCharge: (data: CreateChargeDTO) => Promise<void>;
  markChargePaid: (id: string, data: MarkPaidDTO) => Promise<void>;
  updateCharge: (id: string, data: UpdateChargeDTO) => Promise<void>;
  cancelCharge: (id: string, reason: string) => Promise<void>;
  notifyCharge: (id: string) => Promise<void>;

  transfers: OwnerTransfer[];
  addTransfer: (data: CreateTransferDTO) => Promise<void>;
  approveTransfer: (id: string) => Promise<void>;
  processTransfer: (id: string, evidence?: { proof_url?: string; note?: string }) => Promise<void>;

  collections: Collection[];
  addCollection: (data: CreateCollectionDTO) => Promise<void>;
  updateCollection: (id: string, data: UpdateCollectionDTO) => Promise<void>;
  addCollectionAction: (id: string, data: AddActionDTO) => Promise<void>;
  processDelinquency: () => Promise<any>;

  loading: FinanceLoading;
  errors: FinanceErrors;
  refetchTransactions: () => Promise<void>;
  refetchCharges: () => Promise<void>;
  refetchOverdueCharges: () => Promise<void>;
  refetchTransfers: () => Promise<void>;
  refetchCollections: () => Promise<void>;
};

const FinanceContext = createContext<FinanceContextValue | null>(null);

const FIELD_LABELS: Record<string, string> = {
  status: "Status",
  value: "Valor",
  description: "Descrição",
  date: "Data",
  dueDate: "Data de Vencimento",
  paymentDate: "Data de Pagamento",
  paymentMethod: "Método de Pagamento",
  category: "Categoria",
  grossValue: "Valor Bruto",
  adminFee: "Taxa Administrativa",
  maintenanceDeductions: "Deduções de Manutenção",
  utilityDeductions: "Deduções de Consumo",
  type: "Tipo",
  month: "Referência",
};

const normalizeTransactionStatus = (status: TransactionDTO["status"]): Transaction["status"] => {
  switch (status.trim().toUpperCase()) {
    case "PAID":
    case "PAGO":
      return "Pago";
    case "OVERDUE":
    case "EM ATRASO":
      return "Em Atraso";
    case "REVERSED":
    case "ESTORNADO":
      return "Estornado";
    case "CANCELLED":
    case "CANCELADO":
      return "Cancelado";
    case "PENDING":
    case "PENDENTE":
    default:
      return "Pendente";
  }
};

export function FinanceProvider({ children }: { children: ReactNode }) {
  const { isAuthenticated, user } = useAuth();
  const { can, isLoading: permissionsLoading } = usePermissionsContext();
  const { appendAuditLogEntry } = useAuditLogContext();
  const beginScopedRequest = useRequestScopeGuard(
    `${isAuthenticated}:${user?.current_company_id ?? ""}`,
  );

  const [transactions, setTransactions] = useState<Transaction[]>([]);
  const [charges, setCharges] = useState<Charge[]>([]);
  const [overdueCharges, setOverdueCharges] = useState<Charge[]>([]);
  const [transfers, setTransfers] = useState<OwnerTransfer[]>([]);
  const [collections, setCollections] = useState<Collection[]>([]);

  const [loading, setLoading] = useState<FinanceLoading>({
    transactions: false,
    charges: false,
    overdueCharges: false,
    transfers: false,
    collections: false,
  });

  const [errors, setErrors] = useState<FinanceErrors>({
    transactions: null,
    charges: null,
    overdueCharges: null,
    transfers: null,
    collections: null,
  });

  // WS reconciliation protection for optimistic updates
  const financeMutationRef = useRef<Promise<unknown> | null>(null);
  const financeVersionRef = useRef(0);
  const financeCooldownUntilRef = useRef<number>(0);

  const runIdRef = useRef(0);
  const mutationSequenceRef = useRef(new Map<string, number>());
  const beginFinanceMutation = useCallback((entityKey?: string) => {
    const isCurrentScope = beginScopedRequest();
    const runId = runIdRef.current;
    const sequence = entityKey
      ? (mutationSequenceRef.current.get(entityKey) ?? 0) + 1
      : 0;
    if (entityKey) mutationSequenceRef.current.set(entityKey, sequence);
    return () =>
      isCurrentScope() &&
      runId === runIdRef.current &&
      (!entityKey || mutationSequenceRef.current.get(entityKey) === sequence);
  }, [beginScopedRequest]);

  const inflightRef = useRef<
    Record<keyof FinanceLoading, { runId: number; promise: Promise<void> } | null>
  >({
    transactions: null,
    charges: null,
    overdueCharges: null,
    transfers: null,
    collections: null,
  });

  const dedupe = useCallback(<K extends keyof FinanceLoading>(key: K, runId: number, fn: () => Promise<void>) => {
    const existing = inflightRef.current[key];
    if (existing?.runId === runId) return existing.promise;

    let promise: Promise<void>;
    promise = fn().finally(() => {
      if (inflightRef.current[key]?.promise === promise) {
        inflightRef.current[key] = null;
      }
    });
    inflightRef.current[key] = { runId, promise };
    return promise;
  }, []);

  const mapTransactionDtoToTransaction = useCallback(
    (dto: TransactionDTO): Transaction => ({
      id: dto.id,
      contractId: dto.contract_id,
      type: dto.type,
      description: dto.description,
      value: dto.value,
      date: dto.date,
      status: normalizeTransactionStatus(dto.status),
      dueDate: dto.due_date,
      paymentDate: dto.payment_date,
      paymentMethod: dto.payment_method,
      category: dto.category,
      month: dto.month,
      grossValue: dto.gross_value,
      adminFee: dto.admin_fee,
      maintenanceDeductions: dto.maintenance_deductions,
      utilityDeductions: dto.utility_deductions,
      sourceEventID: dto.source_event_id,
      operationID: dto.operation_id,
      postingID: dto.posting_id,
      recordSource: "transaction",
    }),
    [],
  );

  const fetchTransactions = useCallback(
    async (runId: number) => {
      const fetchVersion = financeVersionRef.current;
      await dedupe("transactions", runId, async () => {
        setLoading((p) => ({ ...p, transactions: true }));
        setErrors((p) => ({ ...p, transactions: null }));
        try {
          const response = await retry(async () => ({ data: await listAllTransactions() }), { attempts: 2, baseDelayMs: 300 });
          if (runId !== runIdRef.current) return;
          if (financeVersionRef.current !== fetchVersion) return;
          const data = unwrapApiData<TransactionDTO[]>(response);
          if (Array.isArray(data)) {
            setTransactions(data.map(mapTransactionDtoToTransaction));
          } else {
            setTransactions([]);
          }
        } catch (error) {
          if (runId !== runIdRef.current) return;
          if (financeVersionRef.current !== fetchVersion) return;
          setErrors((p) => ({ ...p, transactions: error }));
          toast.error("Não foi possível carregar transações.");
        } finally {
          if (runId !== runIdRef.current) return;
          setLoading((p) => ({ ...p, transactions: false }));
        }
      });
    },
    [dedupe, mapTransactionDtoToTransaction],
  );

  const fetchCharges = useCallback(
    async (runId: number) => {
      await dedupe("charges", runId, async () => {
        setLoading((p) => ({ ...p, charges: true }));
        setErrors((p) => ({ ...p, charges: null }));
        try {
          const response = await retry(async () => ({ data: await listAllCharges() }), { attempts: 2, baseDelayMs: 300 });
          if (runId !== runIdRef.current) return;
          const data = unwrapApiData<Charge[]>(response);
          if (Array.isArray(data)) setCharges(data);
        } catch (error) {
          if (runId !== runIdRef.current) return;
          setErrors((p) => ({ ...p, charges: error }));
          toast.error("Financeiro indisponível (cobranças).");
        } finally {
          if (runId !== runIdRef.current) return;
          setLoading((p) => ({ ...p, charges: false }));
        }
      });
    },
    [dedupe],
  );

  const fetchOverdueCharges = useCallback(
    async (runId: number) => {
      await dedupe("overdueCharges", runId, async () => {
        setLoading((p) => ({ ...p, overdueCharges: true }));
        setErrors((p) => ({ ...p, overdueCharges: null }));
        try {
          const response = await retry(() => chargeService.getOverdue(), { attempts: 2, baseDelayMs: 300 });
          if (runId !== runIdRef.current) return;
          const data = unwrapApiData<Charge[]>(response);
          if (Array.isArray(data)) setOverdueCharges(data);
        } catch (error) {
          if (runId !== runIdRef.current) return;
          setErrors((p) => ({ ...p, overdueCharges: error }));
          toast.error("Financeiro indisponível (cobranças em atraso).");
        } finally {
          if (runId !== runIdRef.current) return;
          setLoading((p) => ({ ...p, overdueCharges: false }));
        }
      });
    },
    [dedupe],
  );

  const fetchTransfers = useCallback(
    async (runId: number) => {
      await dedupe("transfers", runId, async () => {
        setLoading((p) => ({ ...p, transfers: true }));
        setErrors((p) => ({ ...p, transfers: null }));
        try {
          const response = await retry(async () => ({ data: await listAllTransfers() }), { attempts: 2, baseDelayMs: 300 });
          if (runId !== runIdRef.current) return;
          const data = unwrapApiData<OwnerTransfer[]>(response);
          if (Array.isArray(data)) setTransfers(data);
        } catch (error) {
          if (runId !== runIdRef.current) return;
          setErrors((p) => ({ ...p, transfers: error }));
          toast.error("Não foi possível carregar repasses.");
        } finally {
          if (runId !== runIdRef.current) return;
          setLoading((p) => ({ ...p, transfers: false }));
        }
      });
    },
    [dedupe],
  );

  const fetchCollections = useCallback(
    async (runId: number) => {
      await dedupe("collections", runId, async () => {
        setLoading((p) => ({ ...p, collections: true }));
        setErrors((p) => ({ ...p, collections: null }));
        try {
          const response = await retry(() => collectionService.getAll(), { attempts: 2, baseDelayMs: 300 });
          if (runId !== runIdRef.current) return;
          const data = unwrapApiData<Collection[]>(response);
          if (Array.isArray(data)) setCollections(data);
        } catch (error) {
          if (runId !== runIdRef.current) return;
          setErrors((p) => ({ ...p, collections: error }));
          toast.error("Não foi possível carregar inadimplências.");
        } finally {
          if (runId !== runIdRef.current) return;
          setLoading((p) => ({ ...p, collections: false }));
        }
      });
    },
    [dedupe],
  );

  const reconcileFinance = useCallback(async () => {
    // Wait for any in-flight finance mutation to settle.
    const pendingMutation = financeMutationRef.current;
    if (pendingMutation) {
      try { await pendingMutation; } catch {}
    }

    const fetchVersion = financeVersionRef.current;
    const runId = runIdRef.current;
    const requests: Promise<void>[] = [];
    if (can("finance:read")) {
      requests.push(fetchTransactions(runId), fetchTransfers(runId));
    }
    if (can("charges:read")) {
      requests.push(fetchCharges(runId), fetchOverdueCharges(runId), fetchCollections(runId));
    }
    await Promise.all(requests);

    // After all fetches complete, check if a newer mutation started
    // or we're within the post-mutation cooldown window. We don't
    // block the fetches themselves — just don't apply stale results.
    if (financeVersionRef.current !== fetchVersion) return;
    if (Date.now() < financeCooldownUntilRef.current) return;
  }, [can, fetchCharges, fetchCollections, fetchOverdueCharges, fetchTransactions, fetchTransfers]);

  useDataChangeRefresh(
    ["finance", "transactions", "charges", "transfers", "collections", "commissions", "payouts", "receivables", "payables"],
    reconcileFinance,
    {
      enabled: isAuthenticated && Boolean(user?.current_company_id) && !permissionsLoading,
    },
  );

  useEffect(() => {
    if (!isAuthenticated || !user?.current_company_id || permissionsLoading) {
      runIdRef.current += 1;
      mutationSequenceRef.current.clear();
      setTransactions([]);
      setCharges([]);
      setOverdueCharges([]);
      setTransfers([]);
      setCollections([]);
      setErrors({
        transactions: null,
        charges: null,
        overdueCharges: null,
        transfers: null,
        collections: null,
      });
      return;
    }

    const runId = runIdRef.current + 1;
    runIdRef.current = runId;
    mutationSequenceRef.current.clear();

    setTransactions([]);
    setCharges([]);
    setOverdueCharges([]);
    setTransfers([]);
    setCollections([]);

    const load = async () => {
      if (can("finance:read")) {
        await fetchTransactions(runId);
      }
      if (can("charges:read")) {
        await fetchCharges(runId);
        await fetchOverdueCharges(runId);
        await fetchCollections(runId);
      }
      if (can("finance:read")) {
        await fetchTransfers(runId);
      }
    };

    load().catch(() => {});
  }, [
    can,
    fetchCharges,
    fetchCollections,
    fetchOverdueCharges,
    fetchTransactions,
    fetchTransfers,
    isAuthenticated,
    permissionsLoading,
    user?.current_company_id,
  ]);

  const addTransaction = useCallback(
    async (t: Transaction) => {
      const isCurrent = beginFinanceMutation(`transaction:${t.id}`);
      try {
        const dto: CreateTransactionDTO = {
          contract_id: t.contractId,
          type: t.type,
          description: t.description,
          value: coerceMoneyBR((t as any).value),
          date: new Date(t.date).toISOString(),
          status: t.status,
          due_date: t.dueDate ? new Date(t.dueDate).toISOString() : undefined,
          payment_date: t.paymentDate ? new Date(t.paymentDate).toISOString() : undefined,
          payment_method: t.paymentMethod || undefined,
          category: t.category,
          month: t.month,
          gross_value: t.grossValue !== undefined ? coerceMoneyBR((t as any).grossValue) : undefined,
          admin_fee: t.adminFee !== undefined ? coerceMoneyBR((t as any).adminFee) : undefined,
          maintenance_deductions: t.maintenanceDeductions !== undefined ? coerceMoneyBR((t as any).maintenanceDeductions) : undefined,
          utility_deductions: t.utilityDeductions !== undefined ? coerceMoneyBR((t as any).utilityDeductions) : undefined,
          source_event_id: t.sourceEventID,
        };
        const response = await transactionService.create(dto);
        if (!isCurrent()) return;
        const created = unwrapApiData<TransactionDTO>(response);
        const mapped = mapTransactionDtoToTransaction(created);
        setTransactions((prev) => [...prev, mapped]);
        appendAuditLogEntry({
          id: `AUD-${Date.now()}`,
          entityType: "transaction",
          entityId: mapped.id,
          action: "criacao",
          changes: [{ field: "status", oldValue: "", newValue: mapped.status }],
          timestamp: new Date().toISOString(),
          user: "Usuário Atual",
        });
      } catch (error) {
        if (!isCurrent()) return;
        console.error("Erro ao criar transação:", error);
        toast.error("Erro ao criar transação.");
      }
    },
    [appendAuditLogEntry, beginFinanceMutation, mapTransactionDtoToTransaction],
  );

  const addTransactionBatch = useCallback(
    async (items: CreateTransactionBatchItem[], operationID: string, commissions: ManualCommissionBatchItem[] = [], commissionRetention?: ManualCommissionRetention): Promise<Transaction[]> => {
      const isCurrent = beginFinanceMutation(`transaction-batch:${operationID}`);
      ++financeVersionRef.current;

      const mutationPromise = (async () => {
        try {
          const response = await transactionService.createBatch(items, operationID, commissions, commissionRetention);
          const results = unwrapApiData<Array<{ transaction: TransactionDTO; index: number; error?: string }>>(response);
          if (!Array.isArray(results) || results.length !== items.length) {
            throw new Error(`incomplete transaction batch response: expected ${items.length}, got ${Array.isArray(results) ? results.length : "invalid"}`);
          }
          const seenIndexes = new Set<number>();
          let canonicalOperationID = "";
          for (const result of results) {
            if (!Number.isInteger(result.index) || result.index < 0 || result.index >= items.length || seenIndexes.has(result.index)) {
              throw new Error("invalid or duplicate transaction batch index");
            }
            seenIndexes.add(result.index);
            if (result.error) throw new Error(`transaction batch item ${result.index} failed: ${result.error}`);
            const persisted = result.transaction;
            if (!persisted?.id || persisted.posting_id !== persisted.id || !persisted.source_event_id || !persisted.operation_id) {
              throw new Error(`transaction batch item ${result.index} has no canonical posting identity`);
            }
            if (canonicalOperationID && persisted.operation_id !== canonicalOperationID) {
              throw new Error("transaction batch response contains more than one operation_id");
            }
            canonicalOperationID = persisted.operation_id;
          }

          const created = results
            .slice()
            .sort((a, b) => a.index - b.index)
            .map((result) => mapTransactionDtoToTransaction(result.transaction));
          if (isCurrent()) {
            setTransactions((prev) => {
              const createdByID = new Map(created.map((tx) => [tx.id, tx]));
              return [...created, ...prev.filter((tx) => !createdByID.has(tx.id))];
            });
            financeCooldownUntilRef.current = Date.now() + 2000;
            ++financeVersionRef.current;
          }
          return created;
        } catch (error) {
          if (isCurrent()) {
            console.error("Erro ao criar lote de transações:", error);
            toast.error("Erro ao criar lançamentos.");
          }
          throw error;
        }
      })();

      financeMutationRef.current = mutationPromise;
      const clearMutation = () => {
        if (financeMutationRef.current === mutationPromise) {
          financeMutationRef.current = null;
        }
      };
      void mutationPromise.then(clearMutation, clearMutation);
      return mutationPromise;
    },
    [beginFinanceMutation, mapTransactionDtoToTransaction],
  );

  const updateTransaction = useCallback(
    async (id: string, updates: Partial<Transaction>) => {
      const isCurrent = beginFinanceMutation(`transaction:${id}`);
      const runId = runIdRef.current;
      const oldTx = transactions.find((t) => t.id === id);

      // Block WS reconciliation before optimistic update
      let releasePlaceholder!: () => void;
      financeMutationRef.current = new Promise<void>((r) => { releasePlaceholder = r; });
      ++financeVersionRef.current;

      setTransactions((prev) => prev.map((t) => (t.id === id ? { ...t, ...updates } : t)));

      const mutationPromise = (async () => {
        try {
          const dto: UpdateTransactionDTO = {};
          if (updates.type !== undefined) dto.type = updates.type;
          if (updates.description !== undefined) dto.description = updates.description;
          if (updates.value !== undefined) dto.value = coerceMoneyBR((updates as any).value);
          if (updates.date !== undefined) dto.date = updates.date;
          if (updates.status !== undefined) dto.status = updates.status;
          if (updates.dueDate !== undefined) dto.due_date = updates.dueDate;
          if (updates.paymentDate !== undefined) dto.payment_date = updates.paymentDate;
          if (updates.paymentMethod !== undefined) dto.payment_method = updates.paymentMethod;
          if (updates.category !== undefined) dto.category = updates.category;
          if (updates.month !== undefined) dto.month = updates.month;
          if (updates.grossValue !== undefined) dto.gross_value = coerceMoneyBR((updates as any).grossValue);
          if (updates.adminFee !== undefined) dto.admin_fee = coerceMoneyBR((updates as any).adminFee);
          if (updates.maintenanceDeductions !== undefined) dto.maintenance_deductions = coerceMoneyBR((updates as any).maintenanceDeductions);
          if (updates.utilityDeductions !== undefined) dto.utility_deductions = coerceMoneyBR((updates as any).utilityDeductions);

          // Baixa (pagamento) é uma operação de ciclo de vida — usa o endpoint
          // dedicado /settle (que completa o ciclo de comissão/ledger quando a
          // transação é source-linked). Só roteamos para /settle quando a
          // alteração é EXCLUSIVAMENTE a baixa (apenas `status: "Pago"`);
          // edições de descrição/valor/vencimento/etc. de uma transação já
          // paga continuam no PUT genérico (senão as edições seriam descartadas
          // porque /settle só aceita payment_date/payment_method).
          const isPureBaixa = updates.status === "Pago" && Object.keys(updates).length === 1;
          const response = isPureBaixa
              ? await transactionService.settle(id, {
                  payment_date: (updates as any).paymentDate,
                  payment_method: (updates as any).paymentMethod,
                })
              : await transactionService.update(id, dto);
          if (!isCurrent()) return false;
          const updated = unwrapApiData<TransactionDTO>(response);
          if (updated?.id) {
            const reconciled = mapTransactionDtoToTransaction(updated);
            setTransactions((prev) => prev.map((t) => (t.id === id ? reconciled : t)));
            financeCooldownUntilRef.current = Date.now() + 2000;
          }

          if (oldTx) {
            const changes: { field: string; oldValue: string; newValue: string }[] = [];
            const isStatusChange = "status" in updates && updates.status !== oldTx.status;
            for (const key of Object.keys(updates) as (keyof Transaction)[]) {
              const oldVal = oldTx[key];
              const newVal = updates[key];
              if (String(oldVal ?? "") !== String(newVal ?? "")) {
                const label = FIELD_LABELS[key] || String(key);
                changes.push({
                  field: label,
                  oldValue: oldVal != null ? String(oldVal) : "",
                  newValue: newVal != null ? String(newVal) : "",
                });
              }
            }
            if (changes.length > 0) {
              appendAuditLogEntry({
                id: `AUD-${Date.now()}-${crypto.getRandomValues(new Uint32Array(1))[0].toString(36).substring(0, 6)}`,
                entityType: "transaction",
                entityId: id,
                action: isStatusChange ? "status_change" : "edicao",
                changes,
                timestamp: new Date().toISOString(),
                user: "Usuário Atual",
              });
            }
          }
          return true;
        } catch (error) {
          if (!isCurrent()) return false;
          ++financeVersionRef.current;
          console.error("Erro ao atualizar transação:", error);
          if (oldTx) {
            setTransactions((prev) => prev.map((t) => (t.id === id ? oldTx : t)));
          }
          await fetchTransactions(runId);
          if ((error as { response?: { status?: number } })?.response?.status === 409) {
            toast.warning(
              "Este lançamento já foi pago/revertido ou foi alterado por outra operação. A lista foi atualizada — confira o status antes de tentar novamente.",
            );
          } else {
            toast.error("Erro ao atualizar transação.");
          }
          throw error;
        }
      })();

      financeMutationRef.current = mutationPromise;
      releasePlaceholder();
      const clearMutation = () => {
        if (financeMutationRef.current === mutationPromise) {
          financeMutationRef.current = null;
        }
      };
      void mutationPromise.then(clearMutation, clearMutation);

      return mutationPromise;
    },
    [appendAuditLogEntry, beginFinanceMutation, fetchTransactions, mapTransactionDtoToTransaction, transactions],
  );

  const deleteTransaction = useCallback(
    async (id: string) => {
      const isCurrent = beginFinanceMutation(`transaction:${id}`);
      const runId = runIdRef.current;
      const oldTx = transactions.find((t) => t.id === id);

      // Block WS reconciliation before optimistic delete
      let releasePlaceholder!: () => void;
      financeMutationRef.current = new Promise<void>((r) => { releasePlaceholder = r; });
      ++financeVersionRef.current;

      setTransactions((prev) => prev.filter((t) => t.id !== id));

      const mutationPromise = (async () => {
        try {
          await transactionService.delete(id);
          if (!isCurrent()) return;
          financeCooldownUntilRef.current = Date.now() + 2000;
          appendAuditLogEntry({
            id: `AUD-${Date.now()}-${crypto.getRandomValues(new Uint32Array(1))[0].toString(36).substring(0, 6)}`,
            entityType: "transaction",
            entityId: id,
            action: "exclusao",
            changes: [{ field: "Transacao", oldValue: id, newValue: "Removida" }],
            timestamp: new Date().toISOString(),
            user: "Usuário Atual",
          });
        } catch (error) {
          if (!isCurrent()) return;
          ++financeVersionRef.current;
          console.error("Erro ao excluir transação:", error);
          if (oldTx) {
            setTransactions((prev) => {
              const exists = prev.some((t) => t.id === id);
              return exists ? prev : [...prev, oldTx];
            });
          }
          await fetchTransactions(runId);
          toast.error("Erro ao excluir transação.");
          throw error;
        }
      })();

      financeMutationRef.current = mutationPromise;
      releasePlaceholder();
      const clearMutation = () => {
        if (financeMutationRef.current === mutationPromise) {
          financeMutationRef.current = null;
        }
      };
      void mutationPromise.then(clearMutation, clearMutation);
      return mutationPromise;
    },
    [appendAuditLogEntry, beginFinanceMutation, fetchTransactions, transactions],
  );

  const addCharge = useCallback(async (data: CreateChargeDTO) => {
    const isCurrent = beginFinanceMutation();
    try {
      const normalized: CreateChargeDTO = {
        ...data,
        amount: coerceMoneyBR((data as any).amount),
        discount: data.discount !== undefined ? coerceMoneyBR((data as any).discount) : undefined,
      };
      const response = await chargeService.create(normalized);
      if (!isCurrent()) return;
      const newCharge = unwrapApiData<Charge>(response);
      setCharges((prev) => [newCharge, ...prev]);
      toast.success("Cobrança criada com sucesso!");
    } catch (error) {
      if (!isCurrent()) return;
      console.error("Erro ao criar cobrança:", error);
      toast.error("Erro ao criar cobrança.");
      throw error;
    }
  }, [beginFinanceMutation]);

  const markChargePaid = useCallback(async (id: string, data: MarkPaidDTO) => {
    const isCurrent = beginFinanceMutation(`charge:${id}`);
    try {
      const normalized: MarkPaidDTO = { ...data, paid_amount: coerceMoneyBR((data as any).paid_amount) };
      const response = await chargeService.markPaid(id, normalized);
      if (!isCurrent()) return;
      const updated = unwrapApiData<Charge>(response);
      setCharges((prev) => prev.map((c) => (c.id === id ? updated : c)));
      setOverdueCharges((prev) => prev.filter((c) => c.id !== id));
      toast.success("Pagamento registrado!");
    } catch (error) {
      if (!isCurrent()) return;
      console.error("Erro ao registrar pagamento:", error);
      toast.error("Erro ao registrar pagamento.");
      throw error;
    }
  }, [beginFinanceMutation]);

  const updateCharge = useCallback(async (id: string, data: UpdateChargeDTO) => {
    const isCurrent = beginFinanceMutation(`charge:${id}`);
    try {
      const normalized: UpdateChargeDTO = {
        ...data,
        amount: data.amount !== undefined ? coerceMoneyBR((data as any).amount) : undefined,
        discount: data.discount !== undefined ? coerceMoneyBR((data as any).discount) : undefined,
        late_fee: data.late_fee !== undefined ? coerceMoneyBR((data as any).late_fee) : undefined,
        interest: data.interest !== undefined ? coerceMoneyBR((data as any).interest) : undefined,
      };
      const response = await chargeService.update(id, normalized);
      if (!isCurrent()) return;
      const updated = unwrapApiData<Charge>(response);
      setCharges((prev) => prev.map((c) => (c.id === id ? updated : c)));
      toast.success("Cobrança atualizada!");
    } catch (error) {
      if (!isCurrent()) return;
      console.error("Erro ao atualizar cobrança:", error);
      toast.error("Erro ao atualizar cobrança.");
      throw error;
    }
  }, [beginFinanceMutation]);

  const cancelCharge = useCallback(async (id: string, reason: string) => {
    const isCurrent = beginFinanceMutation(`charge:${id}`);
    try {
      const response = await chargeService.cancel(id, reason);
      if (!isCurrent()) return;
      const updated = unwrapApiData<Charge>(response);
      setCharges((prev) => prev.map((c) => (c.id === id ? updated : c)));
      setOverdueCharges((prev) => prev.filter((c) => c.id !== id));
      toast.success("Cobrança cancelada.");
    } catch (error: any) {
      if (!isCurrent()) return;
      console.error("Erro ao cancelar cobrança:", error);
      const status = error?.response?.status;
      const message = error?.response?.data?.message || "Erro ao cancelar cobrança.";
      if (status === 409) {
        toast.error(message);
      } else {
        toast.error(message);
      }
      throw error;
    }
  }, [beginFinanceMutation]);

  const notifyCharge = useCallback(async (id: string) => {
    const isCurrent = beginFinanceMutation();
    try {
      const response = await chargeService.notify(id);
      if (!isCurrent()) return;
      const result = unwrapApiData<{ channels_sent?: string[] }>(response);
      const channels = result?.channels_sent?.join(", ") || "lembrete";
      toast.success(`Lembrete enviado (${channels}).`);
    } catch (error) {
      if (!isCurrent()) return;
      console.error("Erro ao notificar cobrança:", error);
      toast.error("Erro ao enviar lembrete.");
      throw error;
    }
  }, [beginFinanceMutation]);

  const addTransfer = useCallback(async (data: CreateTransferDTO) => {
    const isCurrent = beginFinanceMutation();
    try {
      const normalized: CreateTransferDTO = {
        ...data,
        gross_rent: coerceMoneyBR((data as any).gross_rent),
        admin_fee_percent: Number((data as any).admin_fee_percent) || 0,
        maintenance_deductions: data.maintenance_deductions !== undefined ? coerceMoneyBR((data as any).maintenance_deductions) : undefined,
        other_deductions: data.other_deductions !== undefined ? coerceMoneyBR((data as any).other_deductions) : undefined,
        deduction_details: data.deduction_details?.map((d) => ({ ...d, amount: coerceMoneyBR((d as any).amount) })),
      };
      const response = await transferService.create(normalized);
      if (!isCurrent()) return;
      const newTransfer = unwrapApiData<OwnerTransfer>(response);
      setTransfers((prev) => [newTransfer, ...prev]);
      toast.success("Repasse criado com sucesso!");
    } catch (error) {
      if (!isCurrent()) return;
      console.error("Erro ao criar repasse:", error);
      toast.error("Erro ao criar repasse.");
      throw error;
    }
  }, [beginFinanceMutation]);

  const approveTransfer = useCallback(async (id: string) => {
    const isCurrent = beginFinanceMutation(`transfer:${id}`);
    try {
      const response = await transferService.approve(id);
      if (!isCurrent()) return;
      let updated = unwrapApiData<OwnerTransfer | null>(response);
      if (!updated?.id) {
        const refreshed = await transferService.getById(id);
        if (!isCurrent()) return;
        updated = unwrapApiData<OwnerTransfer | null>(refreshed);
      }
      if (!updated?.id) throw new Error("Resposta inválida ao aprovar repasse.");
      setTransfers((prev) => prev.map((t) => (t.id === id ? updated : t)));
      toast.success("Repasse aprovado!");
    } catch (error) {
      if (!isCurrent()) return;
      console.error("Erro ao aprovar repasse:", error);
      toast.error("Erro ao aprovar repasse.");
      throw error;
    }
  }, [beginFinanceMutation]);

  const processTransfer = useCallback(async (id: string, evidence?: { proof_url?: string; note?: string }) => {
    const isCurrent = beginFinanceMutation(`transfer:${id}`);
    try {
      const response = await transferService.process(id, evidence);
      if (!isCurrent()) return;
      let updated = unwrapApiData<OwnerTransfer | null>(response);
      if (!updated?.id) {
        const refreshed = await transferService.getById(id);
        if (!isCurrent()) return;
        updated = unwrapApiData<OwnerTransfer | null>(refreshed);
      }
      if (!updated?.id) throw new Error("Resposta inválida ao processar repasse.");
      setTransfers((prev) => prev.map((t) => (t.id === id ? updated : t)));
      toast.success("Repasse processado!");
    } catch (error) {
      if (!isCurrent()) return;
      console.error("Erro ao processar repasse:", error);
      toast.error("Erro ao processar repasse.");
      throw error;
    }
  }, [beginFinanceMutation]);

  const addCollection = useCallback(async (data: CreateCollectionDTO) => {
    const isCurrent = beginFinanceMutation();
    try {
      const normalized: CreateCollectionDTO = {
        ...data,
        total_debt: coerceMoneyBR((data as any).total_debt),
      };
      const response = await collectionService.create(normalized);
      if (!isCurrent()) return;
      const newCollection = unwrapApiData<Collection>(response);
      setCollections((prev) => [newCollection, ...prev]);
      toast.success("Caso de inadimplência criado!");
    } catch (error) {
      if (!isCurrent()) return;
      console.error("Erro ao criar inadimplência:", error);
      toast.error("Erro ao criar caso de inadimplência.");
    }
  }, [beginFinanceMutation]);

  const updateCollection = useCallback(async (id: string, data: UpdateCollectionDTO) => {
    const isCurrent = beginFinanceMutation(`collection:${id}`);
    try {
      const normalized: UpdateCollectionDTO = {
        ...data,
        total_debt: data.total_debt !== undefined ? coerceMoneyBR((data as any).total_debt) : undefined,
        negotiated_amount: data.negotiated_amount !== undefined ? coerceMoneyBR((data as any).negotiated_amount) : undefined,
      };
      const updated = await collectionService.update(id, normalized);
      if (!isCurrent()) return;
      // Só substitui a linha quando o servidor devolveu o caso; um payload
      // vazio não pode virar `null` no meio da lista.
      if (updated?.id) setCollections((prev) => prev.map((c) => (c.id === id ? updated : c)));
      toast.success("Inadimplência atualizada!");
    } catch (error) {
      if (!isCurrent()) return;
      console.error("Erro ao atualizar inadimplência:", error);
      toast.error(`Erro ao atualizar inadimplência: ${serverMessage(error, "falha na atualização.")}`);
    }
  }, [beginFinanceMutation]);

  const addCollectionAction = useCallback(async (id: string, data: AddActionDTO) => {
    const isCurrent = beginFinanceMutation(`collection:${id}`);
    try {
      const returned = await collectionService.addAction(id, data);
      if (!isCurrent()) return;
      // POST /actions devolve o caso atualizado; quando a resposta não traz o
      // caso, relê antes de mexer na lista — nunca inserir `null`.
      const updated = returned?.id ? returned : await collectionService.getById(id);
      if (!isCurrent()) return;
      if (updated?.id) setCollections((prev) => prev.map((c) => (c.id === id ? updated : c)));
      toast.success("Ação registrada!");
    } catch (error) {
      if (!isCurrent()) return;
      console.error("Erro ao adicionar ação:", error);
      toast.error(`Erro ao registrar ação: ${serverMessage(error, "falha ao registrar a ação.")}`);
    }
  }, [beginFinanceMutation]);

  const processDelinquency = useCallback(async () => {
    const isCurrent = beginFinanceMutation();
    try {
      const response = await collectionService.processDelinquency();
      if (!isCurrent()) return null;
      const result = unwrapApiData<any>(response);
      if (result && (result.collections_created > 0 || result.collections_updated > 0)) {
        await fetchCollections(runIdRef.current);
      }
      return isCurrent() ? result : null;
    } catch (error) {
      if (!isCurrent()) return null;
      console.error("Erro ao processar inadimplência:", error);
      throw error;
    }
  }, [beginFinanceMutation, fetchCollections]);

  const refetchTransactions = useCallback(() => fetchTransactions(runIdRef.current), [fetchTransactions]);
  const refetchCharges = useCallback(() => fetchCharges(runIdRef.current), [fetchCharges]);
  const refetchOverdueCharges = useCallback(() => fetchOverdueCharges(runIdRef.current), [fetchOverdueCharges]);
  const refetchTransfers = useCallback(() => fetchTransfers(runIdRef.current), [fetchTransfers]);
  const refetchCollections = useCallback(() => fetchCollections(runIdRef.current), [fetchCollections]);

  const value = useMemo<FinanceContextValue>(
    () => ({
      transactions,
      addTransaction,
      addTransactionBatch,
      updateTransaction,
      deleteTransaction,
      charges,
      overdueCharges,
      addCharge,
      markChargePaid,
      updateCharge,
      cancelCharge,
      notifyCharge,
      transfers,
      addTransfer,
      approveTransfer,
      processTransfer,
      collections,
      addCollection,
      updateCollection,
      addCollectionAction,
      processDelinquency,
      loading,
      errors,
      refetchTransactions,
      refetchCharges,
      refetchOverdueCharges,
      refetchTransfers,
      refetchCollections,
    }),
    [
      addCharge,
      addCollection,
      addCollectionAction,
      addTransaction,
      addTransactionBatch,
      addTransfer,
      approveTransfer,
      cancelCharge,
      charges,
      collections,
      deleteTransaction,
      markChargePaid,
      notifyCharge,
      overdueCharges,
      errors,
      loading,
      processDelinquency,
      refetchCharges,
      refetchCollections,
      refetchOverdueCharges,
      refetchTransactions,
      refetchTransfers,
      processTransfer,
      transactions,
      transfers,
      updateCharge,
      updateCollection,
      updateTransaction,
    ],
  );

  return <FinanceContext.Provider value={value}>{children}</FinanceContext.Provider>;
}

export function useFinanceContext() {
  const ctx = useContext(FinanceContext);
  if (!ctx) throw new Error("useFinanceContext must be used within FinanceProvider");
  return ctx;
}
