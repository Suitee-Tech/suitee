import { act, cleanup, renderHook } from "@testing-library/react";
import { useEffect } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  resetPortalWebSocketForTests,
  usePortalWebSocket,
} from "./usePortalWebSocket";

vi.mock("../services/portal-api", () => ({
  getPortalAccessToken: () => "portal-token",
}));

vi.mock("../../../shared/services/api", () => ({
  getWsUrl: () => "ws://localhost:8080/api/v1/ws",
}));

class MockWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  static instances: MockWebSocket[] = [];

  readonly url: string;
  readyState = MockWebSocket.CONNECTING;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  readonly close = vi.fn(() => {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.(new Event("close") as CloseEvent);
  });

  constructor(url: string | URL) {
    this.url = String(url);
    MockWebSocket.instances.push(this);
  }

  open() {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.(new Event("open"));
  }

  receive(data: string) {
    this.onmessage?.(new MessageEvent("message", { data }));
  }

  closeFromServer() {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.(new Event("close") as CloseEvent);
  }
}

function useConsumer(handler: (type: string, payload: any) => void) {
  const connection = usePortalWebSocket();

  useEffect(() => connection.onMessage(handler), [connection.onMessage, handler]);

  return connection;
}

describe("usePortalWebSocket", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    MockWebSocket.instances = [];
    vi.stubGlobal("WebSocket", MockWebSocket);
  });

  afterEach(() => {
    cleanup();
    resetPortalWebSocketForTests();
    vi.clearAllTimers();
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("shares one connection between multiple consumers", () => {
    const firstHandler = vi.fn();
    const secondHandler = vi.fn();
    const first = renderHook(() => useConsumer(firstHandler));
    const second = renderHook(() => useConsumer(secondHandler));

    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0].url).toBe(
      "ws://localhost:8080/api/v1/ws?token=portal-token",
    );

    act(() => {
      MockWebSocket.instances[0].open();
      MockWebSocket.instances[0].receive(
        JSON.stringify({ type: "deal_updated", payload: { deal_id: "deal-1" } }),
      );
    });

    expect(first.result.current.isConnected).toBe(true);
    expect(second.result.current.isConnected).toBe(true);
    expect(firstHandler).toHaveBeenCalledWith("deal_updated", { deal_id: "deal-1" });
    expect(secondHandler).toHaveBeenCalledWith("deal_updated", { deal_id: "deal-1" });

    first.unmount();
    expect(MockWebSocket.instances[0].close).not.toHaveBeenCalled();

    second.unmount();
    expect(MockWebSocket.instances[0].close).toHaveBeenCalledTimes(1);
  });

  it("reconnects once while at least one consumer remains mounted", async () => {
    const view = renderHook(() => usePortalWebSocket());
    const firstSocket = MockWebSocket.instances[0];

    act(() => {
      firstSocket.open();
      firstSocket.closeFromServer();
    });
    expect(view.result.current.isConnected).toBe(false);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2999);
    });
    expect(MockWebSocket.instances).toHaveLength(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(MockWebSocket.instances).toHaveLength(2);

    act(() => {
      MockWebSocket.instances[1].open();
    });
    expect(view.result.current.isConnected).toBe(true);
  });

  it("does not reconnect after the last consumer unmounts", async () => {
    const view = renderHook(() => usePortalWebSocket());
    const socket = MockWebSocket.instances[0];
    const delayedCloseHandler = socket.onclose;

    act(() => {
      socket.open();
      socket.closeFromServer();
    });
    expect(vi.getTimerCount()).toBe(1);

    view.unmount();

    act(() => {
      delayedCloseHandler?.(new Event("close") as CloseEvent);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });

    expect(MockWebSocket.instances).toHaveLength(1);
    expect(vi.getTimerCount()).toBe(0);
  });
});
