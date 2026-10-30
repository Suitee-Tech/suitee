import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SystemNotificationPermissionPrompt } from "./SystemNotificationPermissionPrompt";

const ASKED_KEY = "suitee.system-notifications.asked.v1";
const PREFS_KEY = "suitee.system-notifications.v1";

class FakeNotification {
  static permission: NotificationPermission = "default";
  // Espelha o browser: conceder atualiza o estado público da permissão.
  static requestPermission = vi.fn(async (): Promise<NotificationPermission> => {
    FakeNotification.permission = "granted";
    return "granted";
  });
  static instances: { title: string; options: NotificationOptions }[] = [];

  constructor(title: string, options?: NotificationOptions) {
    FakeNotification.instances.push({ title, options: options ?? {} });
  }

  addEventListener() {}
}

beforeEach(() => {
  (window as any).Notification = FakeNotification;
  FakeNotification.permission = "default";
  FakeNotification.requestPermission.mockClear();
  FakeNotification.instances = [];
  window.localStorage.clear();
});

afterEach(() => {
  delete (window as any).Notification;
});

const settle = () => new Promise((resolve) => setTimeout(resolve, 20));

describe("SystemNotificationPermissionPrompt", () => {
  it("mostra o soft ask quando a permissão ainda não foi pedida", async () => {
    render(<SystemNotificationPermissionPrompt delayMs={0} />);
    expect(await screen.findByText("Receba alertas importantes")).toBeInTheDocument();
  });

  it("não mostra quando já foi perguntado neste dispositivo", async () => {
    window.localStorage.setItem(ASKED_KEY, "1");
    render(<SystemNotificationPermissionPrompt delayMs={0} />);
    await settle();
    expect(screen.queryByText("Receba alertas importantes")).not.toBeInTheDocument();
  });

  it("não mostra quando a permissão já foi concedida", async () => {
    FakeNotification.permission = "granted";
    render(<SystemNotificationPermissionPrompt delayMs={0} />);
    await settle();
    expect(screen.queryByText("Receba alertas importantes")).not.toBeInTheDocument();
  });

  it("Ativar pede permissão dentro do gesto e confirma com notificação de teste", async () => {
    const user = userEvent.setup();
    render(<SystemNotificationPermissionPrompt delayMs={0} />);
    await user.click(await screen.findByRole("button", { name: "Ativar notificações" }));

    expect(FakeNotification.requestPermission).toHaveBeenCalledTimes(1);
    // enabled salvo + flag de "já perguntado"
    expect(JSON.parse(window.localStorage.getItem(PREFS_KEY) ?? "{}").enabled).toBe(true);
    expect(window.localStorage.getItem(ASKED_KEY)).toBe("1");
    // notificação de teste com som do sistema (silent false)
    await waitFor(() => expect(FakeNotification.instances).toHaveLength(1));
    expect(FakeNotification.instances[0].options.silent).toBe(false);
    expect(screen.queryByText("Receba alertas importantes")).not.toBeInTheDocument();
  });

  it("denied mostra orientação em vez de re-pedir", async () => {
    FakeNotification.requestPermission.mockResolvedValueOnce("denied");
    const user = userEvent.setup();
    render(<SystemNotificationPermissionPrompt delayMs={0} />);
    await user.click(await screen.findByRole("button", { name: "Ativar notificações" }));

    expect(await screen.findByText(/bloqueou a permissão/)).toBeInTheDocument();
    expect(JSON.parse(window.localStorage.getItem(PREFS_KEY) ?? "{}").enabled).not.toBe(true);
    expect(window.localStorage.getItem(ASKED_KEY)).toBe("1");
  });

  it("Agora não dispensa sem disparar o prompt do navegador", async () => {
    const user = userEvent.setup();
    render(<SystemNotificationPermissionPrompt delayMs={0} />);
    await user.click(await screen.findByRole("button", { name: "Agora não" }));

    expect(FakeNotification.requestPermission).not.toHaveBeenCalled();
    expect(window.localStorage.getItem(ASKED_KEY)).toBe("1");
    await waitFor(() => expect(screen.queryByText("Receba alertas importantes")).not.toBeInTheDocument());
  });
});
