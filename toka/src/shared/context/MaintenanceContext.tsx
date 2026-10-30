import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { useAuth } from "./AuthContext";
import { usePermissionsContext } from "./PermissionsContext";
import { useDataChangeRefresh } from "../hooks/useDataChangeRefresh";
import { useRequestScopeGuard } from "../hooks/useRequestScopeGuard";
import { unwrapApiData } from "../services/api";
import {
  maintenanceService,
  type CreateMaintenanceTicketDTO,
  type MaintenanceTicket,
  type UpdateMaintenanceTicketDTO,
} from "../services/maintenance.service";

type MaintenanceContextValue = {
  tickets: MaintenanceTicket[];
  isTicketsLoading: boolean;
  ticketsError: string | null;
  reloadTickets: () => Promise<void>;
  addTicket: (ticket: Omit<MaintenanceTicket, "id" | "date">) => Promise<boolean>;
  updateTicket: (id: string, updates: Partial<MaintenanceTicket>) => Promise<boolean>;
};

const MaintenanceContext = createContext<MaintenanceContextValue | null>(null);

export function MaintenanceProvider({ children }: { children: ReactNode }) {
  const { isAuthenticated, user } = useAuth();
  const { can, isLoading: permissionsLoading } = usePermissionsContext();

  const [tickets, setTickets] = useState<MaintenanceTicket[]>([]);
  const [isTicketsLoading, setIsTicketsLoading] = useState(false);
  const [ticketsError, setTicketsError] = useState<string | null>(null);
  const companyId = user?.current_company_id || user?.companies?.[0]?.id;
  const beginScopedRequest = useRequestScopeGuard(`${isAuthenticated}:${companyId ?? ""}`);
  const previousScopeRef = useRef(`${isAuthenticated}:${companyId ?? ""}`);
  const ticketMutationGenerationRef = useRef<Map<string, number>>(new Map());
  const maintenanceMutationRef = useRef<Promise<unknown> | null>(null);

  const beginTicketMutation = useCallback((ticketId: string) => {
    const generation = (ticketMutationGenerationRef.current.get(ticketId) ?? 0) + 1;
    ticketMutationGenerationRef.current.set(ticketId, generation);
    return generation;
  }, []);

  const isLatestTicketMutation = useCallback(
    (ticketId: string, generation: number) =>
      ticketMutationGenerationRef.current.get(ticketId) === generation,
    [],
  );

  const fetchTickets = useCallback(async () => {
    // Wait for any in-flight ticket mutation to settle before refetching.
    const pendingMutation = maintenanceMutationRef.current;
    if (pendingMutation) {
      try { await pendingMutation; } catch {}
    }

    const isCurrent = beginScopedRequest();
    setIsTicketsLoading(true);
    setTicketsError(null);
    try {
      const response = await maintenanceService.getAll();
      const ticketsData = unwrapApiData<MaintenanceTicket[]>(response);
      if (!isCurrent()) return;
      if (Array.isArray(ticketsData)) setTickets(ticketsData);
    } catch {
      if (!isCurrent()) return;
      setTicketsError("Não foi possível carregar as solicitações de manutenção.");
    } finally {
      if (isCurrent()) setIsTicketsLoading(false);
    }
  }, [beginScopedRequest]);

  const addTicket = useCallback(
    async (ticket: Omit<MaintenanceTicket, "id" | "date">) => {
      const isCurrent = beginScopedRequest();
      try {
        // Repassa o chamado inteiro: `requester_lead_id` é a parte que o
        // financeiro cobra (ou desconta do repasse) e `contract_id` é o
        // vínculo usado para lançar. Montar o DTO campo a campo já descartou
        // esses dois e o lançamento saía sem contraparte.
        const dto: CreateMaintenanceTicketDTO = {
          type: ticket.type,
          ticket_type: ticket.ticket_type,
          category: ticket.category,
          property_id: ticket.property_id,
          contract_id: ticket.contract_id,
          rental_id: ticket.rental_id,
          requester_lead_id: ticket.requester_lead_id,
          property_title: ticket.property_title,
          requester: ticket.requester,
          profile: ticket.profile,
          priority: ticket.priority,
          description: ticket.description,
          assigned_to: ticket.assigned_to,
          estimated_cost: ticket.estimated_cost,
          payer_role: ticket.payer_role,
        };

        const response = await maintenanceService.create(dto);
        if (!isCurrent()) return false;
        const newTicket = unwrapApiData<MaintenanceTicket>(response);
        if (!newTicket?.id) throw new Error("A API não retornou o ticket criado.");

        setTickets((prev) => {
          const exists = prev.some((item) => item.id === newTicket.id);
          return exists
            ? prev.map((item) => (item.id === newTicket.id ? newTicket : item))
            : [newTicket, ...prev];
        });
        toast.success("Ticket criado com sucesso!");
        return true;
      } catch (error) {
        if (!isCurrent()) return false;
        console.error("Erro ao criar ticket:", error);
        toast.error("Erro ao criar ticket.");
        throw error;
      }
    },
    [beginScopedRequest],
  );

  const updateTicket = useCallback(
    async (id: string, updates: Partial<MaintenanceTicket>) => {
      if (Object.keys(updates).length === 0) return true;
      const isCurrent = beginScopedRequest();
      const generation = beginTicketMutation(id);
      const previousTicket = tickets.find((ticket) => ticket.id === id);

      // Block WS reconciliation before optimistic update
      let releasePlaceholder: () => void = () => {};
      maintenanceMutationRef.current = new Promise<void>((r) => { releasePlaceholder = r; });

      setTickets((prev) => prev.map((t) => (t.id === id ? { ...t, ...updates } : t)));

      const mutationPromise = (async () => {
        try {
          const { status, ...rest } = updates as Partial<MaintenanceTicket> & { status?: string };
          let response: unknown;
          if (status) {
            response = await maintenanceService.changeStatus(id, status);
            if (!isCurrent()) return false;
          }
          if (Object.keys(rest).length > 0) {
            const dto: UpdateMaintenanceTicketDTO = { ...rest };
            response = await maintenanceService.update(id, dto);
          }
          if (!isCurrent() || !isLatestTicketMutation(id, generation)) return false;
          const updatedTicket = unwrapApiData<MaintenanceTicket>(response);
          if (!updatedTicket?.id) throw new Error("A API não retornou o ticket atualizado.");
          setTickets((prev) =>
            prev.map((ticket) => (ticket.id === id ? updatedTicket : ticket)),
          );
          return true;
        } catch (error) {
          if (!isCurrent()) return false;
          if (isLatestTicketMutation(id, generation)) {
            setTickets((prev) =>
              prev.map((ticket) => {
                if (ticket.id !== id || !previousTicket) return ticket;
                const stillShowsOptimisticUpdate = Object.entries(updates).every(
                  ([key, value]) => ticket[key as keyof MaintenanceTicket] === value,
                );
                return stillShowsOptimisticUpdate ? previousTicket : ticket;
              }),
            );
            await fetchTickets();
          }
          console.error("Erro ao atualizar ticket:", error);
          toast.error("Erro ao atualizar ticket.");
          throw error;
        }
      })();

      maintenanceMutationRef.current = mutationPromise;
      releasePlaceholder();
      mutationPromise.finally(() => {
        if (maintenanceMutationRef.current === mutationPromise) {
          maintenanceMutationRef.current = null;
        }
      });

      return mutationPromise;
    },
    [
      beginScopedRequest,
      beginTicketMutation,
      isLatestTicketMutation,
      fetchTickets,
      tickets,
    ],
  );

  useDataChangeRefresh(["maintenance"], fetchTickets, {
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
      setTickets([]);
      setIsTicketsLoading(false);
      setTicketsError(null);
      ticketMutationGenerationRef.current.clear();
      return;
    }

    if (scopeChanged) {
      setTickets([]);
      setIsTicketsLoading(false);
      setTicketsError(null);
      ticketMutationGenerationRef.current.clear();
    }
    if (!companyId || permissionsLoading) return;

    if (!can("rentals:inspect")) {
      setTickets([]);
      setIsTicketsLoading(false);
      setTicketsError(null);
      return;
    }
    void fetchTickets();
  }, [can, companyId, fetchTickets, isAuthenticated, permissionsLoading]);

  const value = useMemo<MaintenanceContextValue>(
    () => ({
      tickets,
      isTicketsLoading,
      ticketsError,
      reloadTickets: fetchTickets,
      addTicket,
      updateTicket,
    }),
    [addTicket, fetchTickets, isTicketsLoading, tickets, ticketsError, updateTicket],
  );

  return <MaintenanceContext.Provider value={value}>{children}</MaintenanceContext.Provider>;
}

export function useMaintenanceContext() {
  const ctx = useContext(MaintenanceContext);
  if (!ctx) throw new Error("useMaintenanceContext must be used within MaintenanceProvider");
  return ctx;
}
