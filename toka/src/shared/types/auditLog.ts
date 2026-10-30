export interface AuditLogEntry {
  id: string;
  entityType:
    | "transaction"
    | "contract"
    | "property"
    | "document"
    | "ticket"
    | "deal"
    | "lead"
    | "roulette_config"
    | "auth"
    | "setting";
  entityId: string;
  action: "criacao" | "edicao" | "exclusao" | "status_change";
  changes: { field: string; oldValue: string; newValue: string }[];
  timestamp: string;
  user: string;
}

