import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useDataChangeRefresh } from "../hooks/useDataChangeRefresh";
import { useRequestScopeGuard } from "../hooks/useRequestScopeGuard";
import { contractService } from "../services/contract.service";
import type { Contract } from "../contracts/types";

type ContractsContextValue = {
  contracts: Contract[];
  refreshContracts: () => Promise<void>;
  addContract: (contract: Contract) => void;
};

const ContractsContext = createContext<ContractsContextValue | null>(null);

export function ContractsProvider({
  children,
  isAuthenticated,
  companyId,
  canReadContracts,
  permissionsLoading,
}: {
  children: ReactNode;
  isAuthenticated: boolean;
  companyId?: string;
  canReadContracts: boolean;
  permissionsLoading: boolean;
}) {
  const [contracts, setContracts] = useState<Contract[]>([]);
  const beginScopedRequest = useRequestScopeGuard(`${isAuthenticated}:${companyId ?? ""}`);
  const previousScopeRef = useRef(`${isAuthenticated}:${companyId ?? ""}`);
  const contractsMutationRef = useRef<Promise<unknown> | null>(null);
  const contractsVersionRef = useRef(0);
  const contractsCooldownUntilRef = useRef<number>(0);

  const refreshContracts = useCallback(async () => {
    const pendingMutation = contractsMutationRef.current;
    if (pendingMutation) {
      try { await pendingMutation; } catch {}
    }
    const isCurrent = beginScopedRequest();
    const fetchVersion = contractsVersionRef.current;
    try {
      const apiContracts = await contractService.getAll();
      if (!isCurrent()) return;
      if (contractsVersionRef.current !== fetchVersion) return;
      if (Date.now() < contractsCooldownUntilRef.current) return;
      if (Array.isArray(apiContracts)) {
        setContracts(apiContracts);
      } else {
        setContracts([]);
      }
    } catch (error) {
      if (!isCurrent()) return;
      console.error("Erro ao buscar contratos:", error);
      setContracts([]);
    }
  }, [beginScopedRequest]);

  const addContract = useCallback((contract: Contract) => {
    setContracts((prev) => {
      const idx = prev.findIndex((c) => c.id === contract.id);
      if (idx >= 0) {
        return prev.map((c) => (c.id === contract.id ? contract : c));
      }
      return [...prev, contract];
    });
    contractsCooldownUntilRef.current = Date.now() + 2000;
  }, []);

  useDataChangeRefresh(["contracts"], refreshContracts, {
    enabled: isAuthenticated && Boolean(companyId) && !permissionsLoading && canReadContracts,
  });

  useEffect(() => {
    const scope = `${isAuthenticated}:${companyId ?? ""}`;
    const scopeChanged = previousScopeRef.current !== scope;
    previousScopeRef.current = scope;

    if (!isAuthenticated) {
      setContracts([]);
      return;
    }
    if (scopeChanged) setContracts([]);
    if (!companyId || permissionsLoading) return;

    if (!canReadContracts) {
      setContracts([]);
      return;
    }
    void refreshContracts();
  }, [canReadContracts, companyId, isAuthenticated, permissionsLoading, refreshContracts]);

  const value = useMemo<ContractsContextValue>(
    () => ({
      contracts,
      refreshContracts,
      addContract,
    }),
    [addContract, contracts, refreshContracts],
  );

  return <ContractsContext.Provider value={value}>{children}</ContractsContext.Provider>;
}

export function useContractsContext() {
  const ctx = useContext(ContractsContext);
  if (!ctx) throw new Error("useContractsContext must be used within ContractsProvider");
  return ctx;
}
