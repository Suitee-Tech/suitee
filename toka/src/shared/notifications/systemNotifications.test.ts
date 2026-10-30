import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  getPermission,
  handleNotificationClick,
  isSupported,
  loadPrefs,
  playAlertChime,
  requestPermission,
  savePrefs,
  shouldNotifyFromDocumentState,
  showSystemNotification,
  subscribeSystemNotificationClick,
  unlockAlertAudio,
} from "./systemNotifications";

// ── Mocks ────────────────────────────────────────────────────────────────────

type ClickListener = (this: FakeNotification) => void;

class FakeNotification {
  static permission: NotificationPermission = "default";
  static requestPermission = vi.fn(async (): Promise<NotificationPermission> => "granted");
  static instances: FakeNotification[] = [];

  title: string;
  options: NotificationOptions;
  onclick: ClickListener | null = null;

  constructor(title: string, options?: NotificationOptions) {
    this.title = title;
    this.options = options ?? {};
    FakeNotification.instances.push(this);
  }

  addEventListener(type: string, listener: unknown) {
    if (type === "click") this.onclick = listener as ClickListener;
  }
}

function installNotificationMock(): void {
  (window as any).Notification = FakeNotification;
}

function removeNotificationMock(): void {
  delete (window as any).Notification;
}

const audioMock = {
  resume: vi.fn(async () => {}),
};

beforeEach(() => {
  FakeNotification.permission = "default";
  FakeNotification.instances = [];
  FakeNotification.requestPermission.mockClear();
  audioMock.resume.mockClear();
  window.localStorage.clear();
  vi.restoreAllMocks();
  installNotificationMock();
});

afterEach(() => {
  removeNotificationMock();
});

// ── Permissão ────────────────────────────────────────────────────────────────

describe("permissão", () => {
  it("detecta navegador sem suporte", async () => {
    removeNotificationMock();
    expect(isSupported()).toBe(false);
    expect(getPermission()).toBe("unsupported");
    expect(await requestPermission()).toBe("unsupported");
    installNotificationMock();
  });

  it("mapeia o estado da permissão do browser", () => {
    FakeNotification.permission = "denied";
    expect(getPermission()).toBe("denied");
    FakeNotification.permission = "granted";
    expect(getPermission()).toBe("granted");
  });

  it("requestPermission devolve o resultado do browser", async () => {
    FakeNotification.requestPermission.mockResolvedValueOnce("denied");
    expect(await requestPermission()).toBe("denied");
    expect(FakeNotification.requestPermission).toHaveBeenCalledTimes(1);
  });

  it("requestPermission degrada para o estado atual em caso de erro", async () => {
    FakeNotification.requestPermission.mockRejectedValueOnce(new Error("blocked"));
    FakeNotification.permission = "default";
    expect(await requestPermission()).toBe("default");
  });
});

// ── Preferências ─────────────────────────────────────────────────────────────

describe("preferências", () => {
  it("default seguro: desligado, som ligado", () => {
    expect(loadPrefs()).toEqual({ enabled: false, sound: true });
  });

  it("persiste e recarrega", () => {
    savePrefs({ enabled: true, sound: false });
    expect(loadPrefs()).toEqual({ enabled: true, sound: false });
  });

  it("ignora JSON corrompido", () => {
    window.localStorage.setItem("suitee.system-notifications.v1", "{corrompido");
    expect(loadPrefs()).toEqual({ enabled: false, sound: true });
  });

  it("não quebra sem localStorage", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("denied");
    });
    expect(loadPrefs()).toEqual({ enabled: false, sound: true });
    expect(() => savePrefs({ enabled: true, sound: true })).not.toThrow();
  });
});

// ── Notificação ──────────────────────────────────────────────────────────────

describe("showSystemNotification", () => {
  it("não notifica sem permissão granted", async () => {
    FakeNotification.permission = "default";
    expect(await showSystemNotification({ title: "x" })).toBe(false);
    FakeNotification.permission = "denied";
    expect(await showSystemNotification({ title: "x" })).toBe(false);
    expect(FakeNotification.instances).toHaveLength(0);
  });

  it("não notifica em navegador sem suporte", async () => {
    removeNotificationMock();
    expect(await showSystemNotification({ title: "x" })).toBe(false);
    installNotificationMock();
  });

  it("exibe com ícones padrão e som do sistema quando sound:true", async () => {
    FakeNotification.permission = "granted";
    const ok = await showSystemNotification({
      title: "Novo lead",
      body: "Maria chegou",
      tag: "lead:1",
      link: "/leads/1",
      sound: true,
    });
    expect(ok).toBe(true);
    const n = FakeNotification.instances[0];
    expect(n.title).toBe("Novo lead");
    expect(n.options.body).toBe("Maria chegou");
    expect(n.options.tag).toBe("lead:1");
    expect(n.options.icon).toBe("/icon-192x192.png");
    expect(n.options.badge).toBe("/favicon-32x32.png");
    // Melhor prática: som do sistema = silent NÃO pode ser true.
    expect(n.options.silent).toBe(false);
  });

  it("respeita sound:false com silent:true", async () => {
    FakeNotification.permission = "granted";
    await showSystemNotification({ title: "x", sound: false });
    expect(FakeNotification.instances[0].options.silent).toBe(true);
  });

  it("swallows falha do browser e devolve false", async () => {
    FakeNotification.permission = "granted";
    vi.spyOn(console, "error").mockImplementation(() => {});
    vi.spyOn(window as any, "Notification").mockImplementation(function () {
      throw new Error("constructor blocked");
    });
    expect(await showSystemNotification({ title: "x" })).toBe(false);
  });
});

/** Substitui window.location por um fake com assign rastreável (jsdom não implementa navegação real). */
function mockLocationAssign() {
  const assign = vi.fn();
  const originalLocation = window.location;
  const fake = Object.create(Object.getPrototypeOf(originalLocation)) as Location;
  Object.assign(fake, { assign });
  Object.defineProperty(window, "location", { configurable: true, writable: true, value: fake });
  return () => {
    Object.defineProperty(window, "location", { configurable: true, writable: true, value: originalLocation });
    return assign;
  };
}

// ── Clique → navegação SPA ───────────────────────────────────────────────────

describe("clique na notificação", () => {
  it("navega pelo listener do app sem location.assign", () => {
    const restore = mockLocationAssign();
    try {
      const navigate = vi.fn();
      const unsubscribe = subscribeSystemNotificationClick(navigate);
      window.dispatchEvent(
        new CustomEvent("suitee:system-notification-click", { detail: { link: "/chamados/7" }, cancelable: true }),
      );
      expect(navigate).toHaveBeenCalledWith("/chamados/7");
      expect((window.location as any).assign).not.toHaveBeenCalled();
      unsubscribe();
    } finally {
      restore();
    }
  });

  it("cai para location.assign quando nenhum listener trata", () => {
    const restore = mockLocationAssign();
    try {
      vi.spyOn(window, "focus").mockImplementation(() => {});
      handleNotificationClick("/leads");
      expect((window.location as any).assign).toHaveBeenCalledWith("/leads");
    } finally {
      restore();
    }
  });

  it("o clique vindo do Notification usa handleNotificationClick", async () => {
    FakeNotification.permission = "granted";
    await showSystemNotification({ title: "x", link: "/mensagens" });
    const n = FakeNotification.instances[0];
    const restore = mockLocationAssign();
    vi.spyOn(window, "focus").mockImplementation(() => {});
    try {
      n.onclick?.call(n);
      expect((window.location as any).assign).toHaveBeenCalledWith("/mensagens");
    } finally {
      restore();
    }
  });
});

// ── Política de exibição ─────────────────────────────────────────────────────

describe("política de exibição", () => {
  it("notifica apenas com a aba em segundo plano", () => {
    Object.defineProperty(document, "hidden", { value: false, configurable: true });
    expect(shouldNotifyFromDocumentState()).toBe(false);
    Object.defineProperty(document, "hidden", { value: true, configurable: true });
    expect(shouldNotifyFromDocumentState()).toBe(true);
    Object.defineProperty(document, "hidden", { value: false, configurable: true });
  });
});

// ── Som ──────────────────────────────────────────────────────────────────────

describe("som (WebAudio)", () => {
  it("unlockAlertAudio não quebra sem AudioContext", () => {
    expect(() => unlockAlertAudio()).not.toThrow();
  });

  it("playAlertChime não quebra sem AudioContext", () => {
    expect(() => playAlertChime()).not.toThrow();
  });

  it("resume um contexto suspenso no unlock", () => {
    const FakeCtx = vi.fn(function (this: any) {
      this.state = "suspended";
      this.resume = audioMock.resume;
      this.currentTime = 0;
      this.destination = {};
      this.createOscillator = () => ({
        type: "",
        frequency: { value: 0 },
        connect: () => ({ connect: () => {} }),
        start: () => {},
        stop: () => {},
      });
      this.createGain = () => ({
        gain: { setValueAtTime: () => {}, exponentialRampToValueAtTime: () => {} },
        connect: () => ({}),
      });
    });
    (window as any).AudioContext = FakeCtx;
    try {
      unlockAlertAudio();
      expect(audioMock.resume).toHaveBeenCalled();
    } finally {
      delete (window as any).AudioContext;
    }
  });
});
