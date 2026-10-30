import { useCallback, useEffect, useState } from "react";
import { getPortalAccessToken } from "../services/portal-api";
import { getWsUrl } from "../../../shared/services/api";

interface WSMessage {
  type: string;
  payload: any;
}

type MessageHandler = (message: WSMessage) => void;
type StatusHandler = (isConnected: boolean) => void;

const RECONNECT_DELAY_MS = 3000;

class PortalWebSocketManager {
  private socket: WebSocket | null = null;
  private reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
  private consumerCount = 0;
  private isConnected = false;
  private readonly messageHandlers = new Set<MessageHandler>();
  private readonly statusHandlers = new Set<StatusHandler>();

  retain() {
    this.consumerCount += 1;
    this.connect();
  }

  release() {
    if (this.consumerCount === 0) return;

    this.consumerCount -= 1;
    if (this.consumerCount > 0) return;

    this.stop();
    this.messageHandlers.clear();
    this.statusHandlers.clear();
  }

  getConnectionStatus() {
    return this.isConnected;
  }

  subscribeToStatus(handler: StatusHandler) {
    this.statusHandlers.add(handler);
    handler(this.isConnected);

    return () => {
      this.statusHandlers.delete(handler);
    };
  }

  subscribeToMessages(handler: MessageHandler) {
    this.messageHandlers.add(handler);

    return () => {
      this.messageHandlers.delete(handler);
    };
  }

  resetForTests() {
    this.consumerCount = 0;
    this.stop();
    this.messageHandlers.clear();
    this.statusHandlers.clear();
  }

  private connect = () => {
    if (this.consumerCount === 0 || this.reconnectTimeout) return;
    if (
      this.socket?.readyState === WebSocket.CONNECTING ||
      this.socket?.readyState === WebSocket.OPEN
    ) {
      return;
    }

    const token = getPortalAccessToken();
    if (!token) return;

    let socket: WebSocket;
    try {
      socket = new WebSocket(`${getWsUrl()}?token=${encodeURIComponent(token)}`);
    } catch {
      this.scheduleReconnect();
      return;
    }
    this.socket = socket;

    socket.onopen = () => {
      if (!this.isCurrentSocket(socket)) return;
      this.setConnectionStatus(true);
    };

    socket.onmessage = (event) => {
      if (!this.isCurrentSocket(socket) || typeof event.data !== "string") return;

      const chunks = event.data
        .split("\n")
        .map((chunk) => chunk.trim())
        .filter(Boolean);

      for (const chunk of chunks) {
        try {
          const message = JSON.parse(chunk) as WSMessage;
          for (const handler of [...this.messageHandlers]) {
            try {
              handler(message);
            } catch {
              // One consumer must not prevent the others from receiving the event.
            }
          }
        } catch {
          // Ignore malformed chunks while still processing the remaining messages.
        }
      }
    };

    socket.onclose = () => {
      if (!this.isCurrentSocket(socket)) return;

      this.socket = null;
      this.setConnectionStatus(false);
      this.scheduleReconnect();
    };

    socket.onerror = () => {
      if (!this.isCurrentSocket(socket)) return;
      socket.close();
    };
  };

  private scheduleReconnect() {
    if (this.consumerCount === 0 || this.reconnectTimeout) return;

    this.reconnectTimeout = setTimeout(() => {
      this.reconnectTimeout = null;
      this.connect();
    }, RECONNECT_DELAY_MS);
  }

  private stop() {
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }

    const socket = this.socket;
    this.socket = null;
    this.setConnectionStatus(false);

    if (!socket) return;

    socket.onopen = null;
    socket.onmessage = null;
    socket.onclose = null;
    socket.onerror = null;

    if (
      socket.readyState === WebSocket.CONNECTING ||
      socket.readyState === WebSocket.OPEN
    ) {
      socket.close();
    }
  }

  private isCurrentSocket(socket: WebSocket) {
    return this.consumerCount > 0 && this.socket === socket;
  }

  private setConnectionStatus(isConnected: boolean) {
    if (this.isConnected === isConnected) return;

    this.isConnected = isConnected;
    for (const handler of [...this.statusHandlers]) {
      try {
        handler(isConnected);
      } catch {
        // Status updates from other consumers must keep flowing.
      }
    }
  }
}

const portalWebSocketManager = new PortalWebSocketManager();

export function usePortalWebSocket() {
  const [isConnected, setIsConnected] = useState(
    portalWebSocketManager.getConnectionStatus(),
  );

  useEffect(() => {
    portalWebSocketManager.retain();
    const unsubscribe = portalWebSocketManager.subscribeToStatus(setIsConnected);

    return () => {
      unsubscribe();
      portalWebSocketManager.release();
    };
  }, []);

  const onNotification = useCallback((callback: (notification: any) => void) => {
    return portalWebSocketManager.subscribeToMessages((message) => {
      if (message.type !== "notification" && message.type !== "inapp_notification") {
        return;
      }

      const payload =
        message.payload && typeof message.payload === "object"
          ? {
              ...message.payload,
              message: message.payload.message ?? message.payload.content ?? "",
            }
          : message.payload;
      callback(payload);
    });
  }, []);

  const onMessage = useCallback((callback: (type: string, payload: any) => void) => {
    return portalWebSocketManager.subscribeToMessages((message) => {
      callback(message.type, message.payload);
    });
  }, []);

  return { isConnected, onNotification, onMessage };
}

export function resetPortalWebSocketForTests() {
  portalWebSocketManager.resetForTests();
}
