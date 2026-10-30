import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { useSystemNotifications } from "./useSystemNotifications";

class FakeNotification {
  static permission: NotificationPermission = "default";
  static requestPermission = vi.fn(async (): Promise<NotificationPermission> => "granted");
}

beforeEach(() => {
  (window as any).Notification = FakeNotification;
  FakeNotification.permission = "default";
  FakeNotification.requestPermission.mockClear();
  window.localStorage.clear();
});

afterEach(() => {
  delete (window as any).Notification;
});

describe("useSystemNotifications", () => {
  it("começa desligado, com som ligado", () => {
    const { result } = renderHook(() => useSystemNotifications());
    expect(result.current.enabled).toBe(false);
    expect(result.current.sound).toBe(true);
    expect(result.current.permission).toBe("default");
  });

  it("enable pede permissão e só liga quando granted", async () => {
    FakeNotification.requestPermission.mockResolvedValueOnce("denied");
    const { result } = renderHook(() => useSystemNotifications());

    await act(async () => {
      await result.current.enable();
    });
    expect(FakeNotification.requestPermission).toHaveBeenCalledTimes(1);
    expect(result.current.permission).toBe("denied");
    expect(result.current.enabled).toBe(false);

    FakeNotification.requestPermission.mockResolvedValueOnce("granted");
    await act(async () => {
      await result.current.enable();
    });
    expect(result.current.permission).toBe("granted");
    expect(result.current.enabled).toBe(true);
  });

  it("setSound persiste a preferência", () => {
    const { result } = renderHook(() => useSystemNotifications());
    act(() => result.current.setSound(false));
    expect(result.current.sound).toBe(false);
    // O estado sobrevive a uma nova montagem (localStorage).
    const second = renderHook(() => useSystemNotifications());
    expect(second.result.current.sound).toBe(false);
  });

  it("setEnabled(false) desliga sem tocar na permissão", () => {
    const { result } = renderHook(() => useSystemNotifications());
    act(() => result.current.setEnabled(false));
    expect(result.current.enabled).toBe(false);
    expect(FakeNotification.requestPermission).not.toHaveBeenCalled();
  });
});
