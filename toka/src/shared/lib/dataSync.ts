export type DataChangeSource = "api" | "websocket" | "reconnect" | "tenant" | "broadcast" | "manual";

export type DataChange = {
  id: string;
  source: DataChangeSource;
  resources: string[];
  timestamp: number;
  tenantId?: string;
  entityId?: string;
  eventType?: string;
  resetCache?: boolean;
};

type DataChangeInput = Omit<DataChange, "id" | "timestamp" | "resources"> & {
  id?: string;
  timestamp?: number;
  resources: readonly string[];
};

type Subscriber = (change: DataChange) => void;

const CHANNEL_NAME = "suitee-data-sync";
const subscribers = new Set<Subscriber>();
const tabId = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
let channel: BroadcastChannel | null = null;

const RESOURCE_RULES: Array<{ pattern: RegExp; resources: string[] }> = [
  { pattern: /\b(suppliers?|fornecedores?|prestadores?)\b/i, resources: ["suppliers"] },
  { pattern: /\b(leads?|prospects?)\b/i, resources: ["leads", "deals"] },
  { pattern: /\b(roulette|roleta)\b/i, resources: ["roulette", "leads"] },
  { pattern: /\b(properties|property|imoveis|imovel)\b/i, resources: ["properties", "deals"] },
  { pattern: /\b(deals?|negocios?)\b/i, resources: ["deals"] },
  { pattern: /\b(proposals?|propostas?)\b/i, resources: ["proposals", "deals"] },
  { pattern: /\b(checklists?)\b/i, resources: ["checklists", "deals"] },
  { pattern: /\b(contracts?|contratos?)\b/i, resources: ["contracts", "deals"] },
  { pattern: /\b(rentals?|leases?|locacoes?|locacao)\b/i, resources: ["rentals", "contracts", "deals"] },
  { pattern: /\b(signatures?|assinaturas?)\b/i, resources: ["signatures", "documents", "contracts", "deals"] },
  { pattern: /\b(documents?|documentos?|files?|arquivos?)\b/i, resources: ["documents", "checklists", "deals"] },
  { pattern: /\b(appointments?|calendar|agenda|eventos?)\b/i, resources: ["calendar"] },
  { pattern: /\b(maintenance|tickets?|manutencao)\b/i, resources: ["maintenance"] },
  { pattern: /\b(keys?|chaves?)\b/i, resources: ["keys"] },
  { pattern: /\b(inspections?|vistorias?)\b/i, resources: ["inspections"] },
  { pattern: /\b(brokers?|agents?|corretores?)\b/i, resources: ["agents"] },
  { pattern: /\b(members?|membros?)\b/i, resources: ["members", "agents", "permissions"] },
  { pattern: /\b(permissions?|roles?|permissoes?|perfis?)\b/i, resources: ["permissions"] },
  { pattern: /\b(partnerships?|shares?|parcerias?|compartilhamentos?)\b/i, resources: ["partnerships", "leads", "properties"] },
  { pattern: /\b(transactions?|transacoes?|lancamentos?)\b/i, resources: ["transactions", "finance"] },
  { pattern: /\b(charges?|cobrancas?)\b/i, resources: ["charges", "finance"] },
  { pattern: /\b(transfers?|repasses?)\b/i, resources: ["transfers", "finance"] },
  { pattern: /\b(collections?|delinquency|inadimplencias?)\b/i, resources: ["collections", "finance"] },
  { pattern: /\b(commissions?|comissoes?)\b/i, resources: ["commissions", "finance"] },
  { pattern: /\b(payouts?|pagamentos?)\b/i, resources: ["payouts", "finance"] },
  { pattern: /\b(receivables?|recebiveis?)\b/i, resources: ["receivables", "finance"] },
  { pattern: /\b(payables?|contas a pagar)\b/i, resources: ["payables", "finance"] },
  { pattern: /\b(billing|invoices?|faturas?|finance)\b/i, resources: ["finance"] },
  { pattern: /\b(notifications?|notificacoes?)\b/i, resources: ["notifications"] },
  { pattern: /\b(companies?|company|empresas?)\b/i, resources: ["companies", "settings", "permissions"] },
  { pattern: /\b(settings?|preferences?|configuracoes?|preferencias?)\b/i, resources: ["settings"] },
  { pattern: /\b(users?|profiles?|usuarios?|perfil)\b/i, resources: ["users", "profile"] },
  { pattern: /\b(channels?|channel|canais?|canal|messages?|messaging)\b/i, resources: ["messages"] },
  { pattern: /\b(read|typing|mark.?read)\b/i, resources: ["messages"] },
];

const DERIVED_QUERY_PATTERN = /\b(dashboard|metrics?|analytics|reports?|resumos?|summary)\b/i;
const CANONICAL_RESOURCES = new Set([
  "all",
  "agents",
  "calendar",
  "charges",
  "checklists",
  "collections",
  "commissions",
  "companies",
  "contracts",
  "deals",
  "documents",
  "finance",
  "inspections",
  "keys",
  "leads",
  "maintenance",
  "members",
  "messages",
  "notifications",
  "partnerships",
  "payables",
  "payouts",
  "permissions",
  "profile",
  "properties",
  "proposals",
  "receivables",
  "rentals",
  "roulette",
  "settings",
  "signatures",
  "suppliers",
  "transactions",
  "transfers",
  "users",
]);
const FINANCE_CHILDREN = new Set([
  "charges",
  "collections",
  "commissions",
  "payables",
  "payouts",
  "receivables",
  "transactions",
  "transfers",
]);

function normalizeText(value: string): string {
  return value
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/[^a-zA-Z0-9]+/g, " ")
    .trim()
    .toLowerCase();
}

function currentTenantId(): string | undefined {
  if (typeof window === "undefined") return undefined;
  try {
    return window.localStorage.getItem("current_company_id") || undefined;
  } catch {
    return undefined;
  }
}

function changeAppliesToCurrentTenant(change: DataChange): boolean {
  const tenantId = currentTenantId();
  return !change.tenantId || !tenantId || change.tenantId === tenantId;
}

function notify(change: DataChange) {
  if (!changeAppliesToCurrentTenant(change)) return;
  subscribers.forEach((subscriber) => subscriber(change));
}

function ensureChannel() {
  if (channel || typeof BroadcastChannel === "undefined") return;
  channel = new BroadcastChannel(CHANNEL_NAME);
  channel.addEventListener("message", (event: MessageEvent<{ tabId?: string; change?: DataChange }>) => {
    if (!event.data?.change || event.data.tabId === tabId) return;
    notify({ ...event.data.change, source: "broadcast" });
  });
}

function makeId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

function queryKeyText(value: unknown): string {
  if (value == null) return "";
  if (Array.isArray(value)) return value.map(queryKeyText).join(" ");
  if (typeof value === "object") return Object.entries(value).map(([key, item]) => `${key} ${queryKeyText(item)}`).join(" ");
  return String(value);
}

export function inferDataResources(value: string): string[] {
  const text = normalizeText(value);
  const resources = new Set<string>();
  for (const rule of RESOURCE_RULES) {
    if (rule.pattern.test(text)) {
      rule.resources.forEach((resource) => resources.add(resource));
    }
  }
  return [...resources];
}

export function normalizeDataResources(resources: readonly string[]): string[] {
  const normalized = new Set<string>();
  for (const resource of resources) {
    const canonical = normalizeText(resource).replace(/\s+/g, "-");
    if (CANONICAL_RESOURCES.has(canonical)) {
      normalized.add(canonical);
      continue;
    }
    const inferred = inferDataResources(resource);
    if (inferred.length > 0) {
      inferred.forEach((item) => normalized.add(item));
    } else {
      const value = normalizeText(resource).replace(/\s+/g, "-");
      if (value) normalized.add(value);
    }
  }
  return [...normalized].sort();
}

function inferQueryResources(value: string): string[] {
  const text = normalizeText(value);
  const resources = new Set<string>();
  for (const rule of RESOURCE_RULES) {
    if (!rule.pattern.test(text)) continue;
    const primary = rule.resources[0];
    resources.add(primary);
    if (FINANCE_CHILDREN.has(primary)) resources.add("finance");
  }
  return [...resources];
}

export function dataChangeMatchesResources(change: Pick<DataChange, "resources">, watchedResources: readonly string[]): boolean {
  const changed = normalizeDataResources(change.resources);
  const watched = normalizeDataResources(watchedResources);
  if (changed.includes("all") || watched.includes("all")) return true;
  const watchedSet = new Set(watched);
  return changed.some((resource) => watchedSet.has(resource));
}

export function queryKeyMatchesResources(queryKey: unknown, resources: readonly string[]): boolean {
  const normalizedResources = normalizeDataResources(resources);
  if (normalizedResources.includes("all")) return true;

  const text = normalizeText(queryKeyText(queryKey));
  if (DERIVED_QUERY_PATTERN.test(text)) return normalizedResources.length > 0;

  const queryResources = inferQueryResources(text);
  return dataChangeMatchesResources({ resources: queryResources }, normalizedResources);
}

export function resourcesForApiMutation(method: string | undefined, url: string | undefined): string[] | null {
  const normalizedMethod = String(method || "get").toLowerCase();
  if (!["post", "put", "patch", "delete"].includes(normalizedMethod)) return null;

  const normalizedUrl = normalizeText(String(url || ""));
  if (!normalizedUrl || /\bauth\b/i.test(normalizedUrl)) return null;

  const resources = inferDataResources(normalizedUrl);
  return resources.length > 0 ? resources : ["all"];
}

export function resourcesForRealtimeEvent(eventType: string): string[] {
  return inferDataResources(eventType);
}

export function publishDataChange(input: DataChangeInput, options: { broadcast?: boolean } = {}): DataChange {
  const change: DataChange = {
    ...input,
    id: input.id || makeId(),
    timestamp: input.timestamp || Date.now(),
    tenantId: input.tenantId ?? currentTenantId(),
    resources: normalizeDataResources(input.resources),
  };

  notify(change);

  if (options.broadcast !== false) {
    ensureChannel();
    channel?.postMessage({ tabId, change });
  }

  return change;
}

export function publishApiMutation(method: string | undefined, url: string | undefined): DataChange | null {
  const resources = resourcesForApiMutation(method, url);
  if (!resources) return null;
  return publishDataChange({ source: "api", resources });
}

export function publishRealtimeChange(eventType: string, payload?: unknown): DataChange | null {
  const resources = resourcesForRealtimeEvent(eventType);
  if (resources.length === 0) return null;

  const candidate = payload && typeof payload === "object" ? (payload as Record<string, unknown>) : {};
  const entityId = candidate.id ?? candidate.lead_id ?? candidate.property_id ?? candidate.deal_id ?? candidate.contract_id;

  return publishDataChange({
    source: "websocket",
    resources,
    eventType,
    entityId: typeof entityId === "string" ? entityId : undefined,
  });
}

export function publishReconnect(): DataChange {
  return publishDataChange({ source: "reconnect", resources: ["all"] }, { broadcast: false });
}

export function publishTenantChange(tenantId: string): DataChange {
  return publishDataChange({
    source: "tenant",
    resources: ["all"],
    tenantId,
    resetCache: true,
  });
}

export function subscribeToDataChanges(subscriber: Subscriber): () => void {
  ensureChannel();
  subscribers.add(subscriber);
  return () => {
    subscribers.delete(subscriber);
  };
}
