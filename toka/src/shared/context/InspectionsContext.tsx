import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { useAuth } from "./AuthContext";
import { usePermissionsContext } from "./PermissionsContext";
import { useDataChangeRefresh } from "../hooks/useDataChangeRefresh";
import { useRequestScopeGuard } from "../hooks/useRequestScopeGuard";
import { unwrapApiData } from "../services/api";
import { inspectionService, type CreateInspectionDTO, type Inspection, type UpdateInspectionDTO } from "../services/inspection.service";

type InspectionsContextValue = {
  inspections: Inspection[];
  addInspection: (data: CreateInspectionDTO) => Promise<void>;
  updateInspection: (id: string, data: UpdateInspectionDTO) => Promise<void>;
  deleteInspection: (id: string) => Promise<void>;
};

const InspectionsContext = createContext<InspectionsContextValue | null>(null);

export function InspectionsProvider({ children }: { children: ReactNode }) {
  const { isAuthenticated, user } = useAuth();
  const { can, isLoading: permissionsLoading } = usePermissionsContext();

  const [inspections, setInspections] = useState<Inspection[]>([]);
  const companyId = user?.current_company_id || user?.companies?.[0]?.id;
  const beginScopedRequest = useRequestScopeGuard(`${isAuthenticated}:${companyId ?? ""}`);
  const previousScopeRef = useRef(`${isAuthenticated}:${companyId ?? ""}`);
  const inspectionsMutationRef = useRef<Promise<unknown> | null>(null);
  const inspectionsVersionRef = useRef(0);
  const inspectionsCooldownUntilRef = useRef<number>(0);

  const fetchInspections = useCallback(async () => {
    const pendingMutation = inspectionsMutationRef.current;
    if (pendingMutation) {
      try { await pendingMutation; } catch {}
    }
    const isCurrent = beginScopedRequest();
    const fetchVersion = inspectionsVersionRef.current;
    try {
      const response = await inspectionService.getAll();
      const data = unwrapApiData<Inspection[]>(response);
      if (!isCurrent()) return;
      if (inspectionsVersionRef.current !== fetchVersion) return;
      if (Date.now() < inspectionsCooldownUntilRef.current) return;
      if (Array.isArray(data)) setInspections(data);
    } catch (error) {
      if (!isCurrent()) return;
      console.error("Erro ao buscar vistorias:", error);
    }
  }, [beginScopedRequest]);

  const addInspection = useCallback(
    async (data: CreateInspectionDTO) => {
      let releasePlaceholder: () => void = () => {};
      const placeholder = new Promise<void>((r) => { releasePlaceholder = r; });
      inspectionsMutationRef.current = placeholder;
      ++inspectionsVersionRef.current;
      const isCurrent = beginScopedRequest();
      let mutationPromise: Promise<unknown> | undefined;
      try {
        const response = await (mutationPromise = inspectionService.create(data));
        if (!isCurrent()) { releasePlaceholder(); return; }
        const newInspection = unwrapApiData<Inspection>(response);
        setInspections((prev) => [newInspection, ...prev]);
        inspectionsCooldownUntilRef.current = Date.now() + 2000;
        toast.success("Vistoria criada!");
      } catch (error) {
        if (!isCurrent()) { releasePlaceholder(); return; }
        ++inspectionsVersionRef.current;
        console.error("Erro ao criar vistoria:", error);
        toast.error("Erro ao criar vistoria.");
      } finally {
        releasePlaceholder();
        if (mutationPromise && inspectionsMutationRef.current === placeholder || inspectionsMutationRef.current === mutationPromise) {
          inspectionsMutationRef.current = null;
        }
      }
    },
    [beginScopedRequest],
  );

  const updateInspection = useCallback(
    async (id: string, data: UpdateInspectionDTO) => {
      let releasePlaceholder: () => void = () => {};
      const placeholder = new Promise<void>((r) => { releasePlaceholder = r; });
      inspectionsMutationRef.current = placeholder;
      ++inspectionsVersionRef.current;
      const isCurrent = beginScopedRequest();
      let mutationPromise: Promise<unknown> | undefined;
      try {
        const response = await (mutationPromise = inspectionService.update(id, data));
        if (!isCurrent()) { releasePlaceholder(); return; }
        const updated = unwrapApiData<Inspection>(response);
        setInspections((prev) => prev.map((i) => (i.id === id ? updated : i)));
        inspectionsCooldownUntilRef.current = Date.now() + 2000;
        toast.success("Vistoria atualizada!");
      } catch (error) {
        if (!isCurrent()) { releasePlaceholder(); return; }
        ++inspectionsVersionRef.current;
        console.error("Erro ao atualizar vistoria:", error);
        toast.error("Erro ao atualizar vistoria.");
      } finally {
        releasePlaceholder();
        if (mutationPromise && inspectionsMutationRef.current === placeholder || inspectionsMutationRef.current === mutationPromise) {
          inspectionsMutationRef.current = null;
        }
      }
    },
    [beginScopedRequest],
  );

  const deleteInspection = useCallback(
    async (id: string) => {
      let releasePlaceholder: () => void = () => {};
      const placeholder = new Promise<void>((r) => { releasePlaceholder = r; });
      inspectionsMutationRef.current = placeholder;
      ++inspectionsVersionRef.current;
      const isCurrent = beginScopedRequest();
      let mutationPromise: Promise<unknown> | undefined;
      try {
        await (mutationPromise = inspectionService.delete(id));
        if (!isCurrent()) { releasePlaceholder(); return; }
        setInspections((prev) => prev.filter((i) => i.id !== id));
        inspectionsCooldownUntilRef.current = Date.now() + 2000;
        toast.success("Vistoria removida.");
      } catch (error) {
        if (!isCurrent()) { releasePlaceholder(); return; }
        ++inspectionsVersionRef.current;
        console.error("Erro ao remover vistoria:", error);
        toast.error("Erro ao remover vistoria.");
      } finally {
        releasePlaceholder();
        if (mutationPromise && inspectionsMutationRef.current === placeholder || inspectionsMutationRef.current === mutationPromise) {
          inspectionsMutationRef.current = null;
        }
      }
    },
    [beginScopedRequest],
  );

  useDataChangeRefresh(["inspections"], fetchInspections, {
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
      setInspections([]);
      return;
    }

    if (scopeChanged) setInspections([]);
    if (!companyId || permissionsLoading) return;

    if (!can("rentals:inspect")) {
      setInspections([]);
      return;
    }
    void fetchInspections();
  }, [can, companyId, fetchInspections, isAuthenticated, permissionsLoading]);

  const value = useMemo<InspectionsContextValue>(
    () => ({
      inspections,
      addInspection,
      updateInspection,
      deleteInspection,
    }),
    [addInspection, deleteInspection, inspections, updateInspection],
  );

  return <InspectionsContext.Provider value={value}>{children}</InspectionsContext.Provider>;
}

export function useInspectionsContext() {
  const ctx = useContext(InspectionsContext);
  if (!ctx) throw new Error("useInspectionsContext must be used within InspectionsProvider");
  return ctx;
}
