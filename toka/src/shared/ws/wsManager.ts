import { getAccessToken, getWsUrl } from "../services/api";
import { publishRealtimeChange, publishReconnect } from "../lib/dataSync";

export type WSEvent = { type: string; payload: any };

type Handler = (event: WSEvent) => void;
type StatusHandler = (isConnected: boolean) => void;

const subscribers = new Map<string, Map<string, Handler>>();
const statusSubscribers = new Map<string, StatusHandler>();

let ws: WebSocket | null = null;
let currentToken: string | null = null;
let isConnected = false;
let reconnectTimeout: number | null = null;
let reconnectAttempt = 0;
let subIdSeq = 0;
let reconcileOnNextOpen = false;

const dedup = new Map<string, number>();

function notifyStatus(next: boolean) {
  if (isConnected === next) return;
  isConnected = next;
  statusSubscribers.forEach((h) => h(next));
}

function parseEvents(data: any): WSEvent[] {
  const str = typeof data === "string" ? data : "";
  const chunks = str
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);
  const out: WSEvent[] = [];
  for (const chunk of chunks) {
    try {
      const parsed = JSON.parse(chunk);
      if (parsed && typeof parsed.type === "string") out.push(parsed);
    } catch {
      // A malformed line must not discard valid events from the same frame.
    }
  }
  return out;
}

function maybeDedup(event: WSEvent): boolean {
  const id = event?.payload?.id ?? event?.payload?.message_id;
  if (!id) return false;
  const key = `${event.type}:${String(id)}`;
  const now = Date.now();
  const prev = dedup.get(key);
  if (prev && now - prev < 60_000) return true;
  dedup.set(key, now);
  if (dedup.size > 2000) {
    const cutoff = now - 60_000;
    for (const [k, ts] of dedup.entries()) {
      if (ts < cutoff) dedup.delete(k);
    }
  }
  return false;
}

function dispatch(event: WSEvent) {
  if (maybeDedup(event)) return;

  publishRealtimeChange(event.type, event.payload);

  const byType = subscribers.get(event.type);
  if (byType) {
    byType.forEach((h) => h(event));
  }
  const any = subscribers.get("*");
  if (any) {
    any.forEach((h) => h(event));
  }
}

function clearReconnect() {
  if (reconnectTimeout != null) {
    window.clearTimeout(reconnectTimeout);
    reconnectTimeout = null;
  }
}

function scheduleReconnect() {
  if (reconnectTimeout != null) return;

  reconnectAttempt += 1;
  const base = Math.min(30_000, 1000 * 2 ** Math.min(10, reconnectAttempt));
  const jitter = base * (0.8 + Math.random() * 0.4);
  reconnectTimeout = window.setTimeout(() => {
    reconnectTimeout = null;
    connect();
  }, jitter);
}

function connect() {
  const token = getAccessToken();
  const url = getWsUrl();
  if (!token || !url) {
    disconnect();
    return;
  }

  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) && currentToken === token) {
    return;
  }

  currentToken = token;
  clearReconnect();

  const previousSocket = ws;
  ws = null;
  if (previousSocket) {
    try {
      previousSocket.close();
    } catch {
    }
  }

  const wsUrl = `${url}?token=${token}`;
  const socket = new WebSocket(wsUrl);
  ws = socket;

  socket.onopen = () => {
    if (ws !== socket) return;
    reconnectAttempt = 0;
    notifyStatus(true);
    if (reconcileOnNextOpen) {
      reconcileOnNextOpen = false;
      publishReconnect();
    }
  };

  socket.onmessage = (evt) => {
    if (ws !== socket) return;
    try {
      const events = parseEvents(evt.data);
      events.forEach(dispatch);
    } catch {
    }
  };

  socket.onclose = () => {
    if (ws !== socket) return;
    notifyStatus(false);
    ws = null;
    reconcileOnNextOpen = true;
    scheduleReconnect();
  };

  socket.onerror = () => {
    if (ws !== socket) return;
    try {
      socket.close();
    } catch {
    }
  };
}

function disconnect() {
  clearReconnect();
  reconnectAttempt = 0;
  reconcileOnNextOpen = false;
  currentToken = null;
  const socket = ws;
  ws = null;
  notifyStatus(false);
  if (socket) {
    try {
      socket.close();
    } catch {
    }
  }
}

export const wsManager = {
  getIsConnected: () => isConnected,
  ensureConnected: () => connect(),
  reconnect: () => {
    currentToken = null;
    const socket = ws;
    ws = null;
    if (socket) {
      try {
        socket.close();
      } catch {
      }
    }
    connect();
  },
  disconnect,
  subscribe: (eventType: string, handler: Handler) => {
    connect();

    const id = `${Date.now()}-${subIdSeq++}`;
    if (!subscribers.has(eventType)) subscribers.set(eventType, new Map());
    subscribers.get(eventType)!.set(id, handler);
    return id;
  },
  unsubscribe: (eventType: string, id: string) => {
    const map = subscribers.get(eventType);
    if (!map) return;
    map.delete(id);
    if (map.size === 0) subscribers.delete(eventType);
  },
  subscribeStatus: (handler: StatusHandler) => {
    connect();
    const id = `${Date.now()}-${subIdSeq++}`;
    statusSubscribers.set(id, handler);
    handler(isConnected);
    return id;
  },
  unsubscribeStatus: (id: string) => {
    statusSubscribers.delete(id);
  },
};
