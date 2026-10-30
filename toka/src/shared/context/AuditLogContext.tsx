import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import type { AuditLogEntry } from "../types/auditLog";

type AuditLogContextValue = {
  auditLog: AuditLogEntry[];
  appendAuditLogEntry: (entry: AuditLogEntry) => void;
  getAuditLog: (entityId?: string, entityType?: AuditLogEntry["entityType"]) => AuditLogEntry[];
};

const AuditLogContext = createContext<AuditLogContextValue | null>(null);

export function AuditLogProvider({ children }: { children: ReactNode }) {
  const [auditLog, setAuditLog] = useState<AuditLogEntry[]>([]);

  const appendAuditLogEntry = useCallback((entry: AuditLogEntry) => {
    setAuditLog((prev) => [...prev, entry]);
  }, []);

  const getAuditLog = useCallback(
    (entityId?: string, entityType?: AuditLogEntry["entityType"]): AuditLogEntry[] => {
      if (!entityId && !entityType) {
        return [...auditLog].sort((a, b) => b.timestamp.localeCompare(a.timestamp));
      }
      return auditLog
        .filter((e) => (!entityId || e.entityId === entityId) && (!entityType || e.entityType === entityType))
        .sort((a, b) => b.timestamp.localeCompare(a.timestamp));
    },
    [auditLog],
  );

  const value = useMemo<AuditLogContextValue>(
    () => ({
      auditLog,
      appendAuditLogEntry,
      getAuditLog,
    }),
    [appendAuditLogEntry, auditLog, getAuditLog],
  );

  return <AuditLogContext.Provider value={value}>{children}</AuditLogContext.Provider>;
}

export function useAuditLogContext() {
  const ctx = useContext(AuditLogContext);
  if (!ctx) throw new Error("useAuditLogContext must be used within AuditLogProvider");
  return ctx;
}

