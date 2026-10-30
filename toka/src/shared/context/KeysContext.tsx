import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { useAuth } from "./AuthContext";
import { usePermissionsContext } from "./PermissionsContext";
import { useDataChangeRefresh } from "../hooks/useDataChangeRefresh";
import { useRequestScopeGuard } from "../hooks/useRequestScopeGuard";
import { unwrapApiData } from "../services/api";
import { keyService, type CreateKeyDTO, type Key } from "../services/key.service";

type KeysContextValue = {
  keys: Key[];
  addKey: (data: CreateKeyDTO) => Promise<void>;
  deleteKey: (id: string) => Promise<void>;
  checkoutKey: (id: string, person: string) => Promise<void>;
  checkinKey: (id: string) => Promise<void>;
};

const KeysContext = createContext<KeysContextValue | null>(null);

export function KeysProvider({ children }: { children: ReactNode }) {
  const { isAuthenticated, user } = useAuth();
  const { can, isLoading: permissionsLoading } = usePermissionsContext();

  const [keys, setKeys] = useState<Key[]>([]);
  const companyId = user?.current_company_id || user?.companies?.[0]?.id;
  const beginScopedRequest = useRequestScopeGuard(`${isAuthenticated}:${companyId ?? ""}`);
  const previousScopeRef = useRef(`${isAuthenticated}:${companyId ?? ""}`);
  const keysMutationRef = useRef<Promise<unknown> | null>(null);
  const keysVersionRef = useRef(0);
  const keysCooldownUntilRef = useRef<number>(0);

  const fetchKeys = useCallback(async () => {
    const pendingMutation = keysMutationRef.current;
    if (pendingMutation) {
      try { await pendingMutation; } catch {}
    }
    const isCurrent = beginScopedRequest();
    const fetchVersion = keysVersionRef.current;
    try {
      const response = await keyService.getAll();
      const data = unwrapApiData<Key[]>(response);
      if (!isCurrent()) return;
      if (keysVersionRef.current !== fetchVersion) return;
      if (Date.now() < keysCooldownUntilRef.current) return;
      if (Array.isArray(data)) setKeys(data);
    } catch (error) {
      if (!isCurrent()) return;
      console.error("Erro ao buscar chaves:", error);
    }
  }, [beginScopedRequest]);

  const addKey = useCallback(
    async (data: CreateKeyDTO) => {
      let releasePlaceholder: () => void = () => {};
      const placeholder = new Promise<void>((r) => { releasePlaceholder = r; });
      keysMutationRef.current = placeholder;
      ++keysVersionRef.current;
      const isCurrent = beginScopedRequest();
      let mutationPromise: Promise<unknown> | undefined;
      try {
        const response = await (mutationPromise = keyService.create(data));
        if (!isCurrent()) { releasePlaceholder(); return; }
        const newKey = unwrapApiData<Key>(response);
        setKeys((prev) => [newKey, ...prev]);
        keysCooldownUntilRef.current = Date.now() + 2000;
        toast.success("Chave cadastrada!");
      } catch (error) {
        if (!isCurrent()) { releasePlaceholder(); return; }
        ++keysVersionRef.current;
        console.error("Erro ao cadastrar chave:", error);
        toast.error("Erro ao cadastrar chave.");
      } finally {
        releasePlaceholder();
        if (mutationPromise && keysMutationRef.current === placeholder || keysMutationRef.current === mutationPromise) {
          keysMutationRef.current = null;
        }
      }
    },
    [beginScopedRequest],
  );

  const deleteKey = useCallback(
    async (id: string) => {
      let releasePlaceholder: () => void = () => {};
      const placeholder = new Promise<void>((r) => { releasePlaceholder = r; });
      keysMutationRef.current = placeholder;
      ++keysVersionRef.current;
      const isCurrent = beginScopedRequest();
      let mutationPromise: Promise<unknown> | undefined;
      try {
        await (mutationPromise = keyService.delete(id));
        if (!isCurrent()) { releasePlaceholder(); return; }
        setKeys((prev) => prev.filter((k) => k.id !== id));
        keysCooldownUntilRef.current = Date.now() + 2000;
        toast.success("Chave removida.");
      } catch (error) {
        if (!isCurrent()) { releasePlaceholder(); return; }
        ++keysVersionRef.current;
        console.error("Erro ao remover chave:", error);
        toast.error("Erro ao remover chave.");
      } finally {
        releasePlaceholder();
        if (mutationPromise && keysMutationRef.current === placeholder || keysMutationRef.current === mutationPromise) {
          keysMutationRef.current = null;
        }
      }
    },
    [beginScopedRequest],
  );

  const checkoutKey = useCallback(
    async (id: string, person: string) => {
      let releasePlaceholder: () => void = () => {};
      const placeholder = new Promise<void>((r) => { releasePlaceholder = r; });
      keysMutationRef.current = placeholder;
      ++keysVersionRef.current;
      const isCurrent = beginScopedRequest();
      let mutationPromise: Promise<unknown> | undefined;
      try {
        const response = await (mutationPromise = keyService.checkout(id, person));
        if (!isCurrent()) { releasePlaceholder(); return; }
        const updated = unwrapApiData<Key>(response);
        setKeys((prev) => prev.map((k) => (k.id === id ? updated : k)));
        keysCooldownUntilRef.current = Date.now() + 2000;
        toast.success("Chave retirada!");
      } catch (error) {
        if (!isCurrent()) { releasePlaceholder(); return; }
        ++keysVersionRef.current;
        console.error("Erro ao retirar chave:", error);
        toast.error("Erro ao retirar chave.");
      } finally {
        releasePlaceholder();
        if (mutationPromise && keysMutationRef.current === placeholder || keysMutationRef.current === mutationPromise) {
          keysMutationRef.current = null;
        }
      }
    },
    [beginScopedRequest],
  );

  const checkinKey = useCallback(
    async (id: string) => {
      let releasePlaceholder: () => void = () => {};
      const placeholder = new Promise<void>((r) => { releasePlaceholder = r; });
      keysMutationRef.current = placeholder;
      ++keysVersionRef.current;
      const isCurrent = beginScopedRequest();
      let mutationPromise: Promise<unknown> | undefined;
      try {
        const response = await (mutationPromise = keyService.checkin(id));
        if (!isCurrent()) { releasePlaceholder(); return; }
        const updated = unwrapApiData<Key>(response);
        setKeys((prev) => prev.map((k) => (k.id === id ? updated : k)));
        keysCooldownUntilRef.current = Date.now() + 2000;
        toast.success("Chave devolvida!");
      } catch (error) {
        if (!isCurrent()) { releasePlaceholder(); return; }
        ++keysVersionRef.current;
        console.error("Erro ao devolver chave:", error);
        toast.error("Erro ao devolver chave.");
      } finally {
        releasePlaceholder();
        if (mutationPromise && keysMutationRef.current === placeholder || keysMutationRef.current === mutationPromise) {
          keysMutationRef.current = null;
        }
      }
    },
    [beginScopedRequest],
  );

  useDataChangeRefresh(["keys"], fetchKeys, {
    enabled:
      isAuthenticated &&
      Boolean(user?.current_company_id || user?.companies?.[0]?.id) &&
      !permissionsLoading &&
      can("rentals:inspect"),
  });

  useEffect(() => {
    const scope = `${isAuthenticated}:${companyId ?? ""}`;
    const scopeChanged = previousScopeRef.current !== scope;
    previousScopeRef.current = scope;

    if (!isAuthenticated) {
      setKeys([]);
      return;
    }

    if (scopeChanged) setKeys([]);
    if (!companyId || permissionsLoading) return;

    if (!can("rentals:inspect")) {
      setKeys([]);
      return;
    }
    void fetchKeys();
  }, [can, companyId, fetchKeys, isAuthenticated, permissionsLoading]);

  const value = useMemo<KeysContextValue>(
    () => ({
      keys,
      addKey,
      deleteKey,
      checkoutKey,
      checkinKey,
    }),
    [addKey, checkinKey, checkoutKey, deleteKey, keys],
  );

  return <KeysContext.Provider value={value}>{children}</KeysContext.Provider>;
}

export function useKeysContext() {
  const ctx = useContext(KeysContext);
  if (!ctx) throw new Error("useKeysContext must be used within KeysProvider");
  return ctx;
}
