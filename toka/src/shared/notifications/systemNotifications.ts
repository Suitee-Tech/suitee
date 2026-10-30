// Notificações de sistema (Web Notifications API) com som — melhores práticas:
//
// 1. PERMISSÃO SÓ VIA GESTO EXPLÍCITO: o prompt de permissão dispara apenas
//    no clique do toggle "Notificações do sistema" — nunca no load da página
//    (prompt automático é spam e leva a bloqueio permanente do domínio).
// 2. CANAL CERTO PARA O MOMENTO CERTO: notificação de sistema só dispara com
//    a aba em segundo plano (`document.hidden`). Com a página visível, o UI
//    in-app (toast + som) é o canal recomendado — evita notificação dupla.
// 3. SOM: a Web Notifications API NÃO aceita som customizado no Chrome — o
//    navegador toca o som do sistema quando `silent` é false. O "ding" do app
//    (WebAudio sintetizado, sem asset externo) cobre a aba visível e substitui
//    o antigo `/notification.mp3` que não existia no bundle.
// 4. DEDUPE E NAVEGAÇÃO SPA: `tag` agrupa/substitui notificações repetidas; o
//    clique foca a janela e navega via evento custom — sem reload de página.
// 5. BLOQUEADO É RESPEITADO: com permissão "denied" o navegador não permite
//    re-pedir — a UI mostra orientação ("habilite nas configurações do site")
//    em vez de re-prompt.
// 6. PUSH (ABA FECHADA): exige backend (VAPID + armazenamento de subscription).
//    O código já prefere `ServiceWorkerRegistration.showNotification` quando
//    existir um service worker — o caminho de upgrade documentado.

export type SystemNotificationPermission = "granted" | "denied" | "default" | "unsupported";

export interface SystemNotificationOptions {
  title: string;
  body?: string;
  /** Chave de agrupamento/dedupe (ex.: `ticket:<id>`). Mesma tag substitui a anterior. */
  tag?: string;
  /** Rota interna (ex.: `/chamados/123`). A navegação usa o router do SPA. */
  link?: string;
  /** false = notificação silenciosa (`silent: true`). */
  sound?: boolean;
  icon?: string;
  badge?: string;
}

export interface SystemNotificationPrefs {
  enabled: boolean;
  sound: boolean;
}

const PREFS_KEY = "suitee.system-notifications.v1";
const ASKED_KEY = "suitee.system-notifications.asked.v1";
const CLICK_EVENT = "suitee:system-notification-click";

const DEFAULT_ICON = "/icon-192x192.png";
const DEFAULT_BADGE = "/favicon-32x32.png";

// ── Permissão ────────────────────────────────────────────────────────────────

export function isSupported(): boolean {
  return typeof window !== "undefined" && "Notification" in window;
}

export function getPermission(): SystemNotificationPermission {
  if (!isSupported()) return "unsupported";
  return window.Notification.permission as SystemNotificationPermission;
}

/**
 * Pede permissão ao navegador. DEVE ser chamado dentro de um gesto do usuário
 * (clique no toggle) — o browser ignora chamadas fora de user activation.
 */
export async function requestPermission(): Promise<SystemNotificationPermission> {
  if (!isSupported()) return "unsupported";
  try {
    const result = await window.Notification.requestPermission();
    return result as SystemNotificationPermission;
  } catch {
    return getPermission();
  }
}

// ── Preferências (por dispositivo) ────────────────────────────────────────────

export function loadPrefs(): SystemNotificationPrefs {
  try {
    const raw = window.localStorage.getItem(PREFS_KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<SystemNotificationPrefs>;
      return { enabled: parsed.enabled === true, sound: parsed.sound !== false };
    }
  } catch {
    // localStorage indisponível (modo privado) — volta para o default.
  }
  return { enabled: false, sound: true };
}

export function savePrefs(prefs: SystemNotificationPrefs): void {
  try {
    window.localStorage.setItem(PREFS_KEY, JSON.stringify(prefs));
  } catch {
    // Sem storage: prefere degradar silenciosamente a quebrar o fluxo.
  }
}

// ── Solicitação de permissão (soft ask, por dispositivo) ──────────────────────
//
// O prompt in-app (SystemNotificationPermissionPrompt) só é mostrado uma vez
// por dispositivo: depois de aceitar, recusar ou dispensar, o flag evita que o
// app volte a incomodar. O toggle no sino continua sendo o caminho manual.

export function hasAskedSystemNotificationPermission(): boolean {
  try {
    return window.localStorage.getItem(ASKED_KEY) === "1";
  } catch {
    return false;
  }
}

export function markSystemNotificationPermissionAsked(): void {
  try {
    window.localStorage.setItem(ASKED_KEY, "1");
  } catch {
    // Sem storage: o prompt reaparece na próxima sessão — aceitável.
  }
}

// ── Som (WebAudio, sem asset) ─────────────────────────────────────────────────

let alertAudioContext: AudioContext | null = null;

type WebkitAudioWindow = Window & { webkitAudioContext?: typeof AudioContext };

function createAudioContext(): AudioContext | null {
  try {
    const Ctor = window.AudioContext ?? (window as WebkitAudioWindow).webkitAudioContext;
    return Ctor ? new Ctor() : null;
  } catch {
    return null;
  }
}

/**
 * Desbloqueia a reprodução de áudio — chamar dentro do gesto que liga as
 * notificações (política de autoplay do Chrome exige interação prévia).
 */
export function unlockAlertAudio(): void {
  try {
    if (!alertAudioContext) alertAudioContext = createAudioContext();
    if (alertAudioContext && alertAudioContext.state === "suspended") {
      void alertAudioContext.resume();
    }
  } catch {
    // Áudio é best-effort: falha aqui não pode afetar a notificação.
  }
}

/**
 * Toca o "ding" de alerta (duas notas curtas, ~800ms) sintetizado em tempo
 * real. Sem download de asset, sem 404, volume discreto. Best-effort: em
 * browsers sem WebAudio (ou contexto suspenso) simplesmente não toca.
 */
export function playAlertChime(): void {
  try {
    if (!alertAudioContext) alertAudioContext = createAudioContext();
    const ctx = alertAudioContext;
    if (!ctx) return;
    if (ctx.state === "suspended") {
      // Sem gesto prévio o resume é rejeitado; nada a fazer além de tentar.
      void ctx.resume();
    }
    const t0 = ctx.currentTime + 0.01;
    chimeTone(ctx, 880, t0, 0.16, 0.16);
    chimeTone(ctx, 1174.66, t0 + 0.16, 0.2, 0.14);
  } catch {
    // Nunca deixa o som derrubar o fluxo de notificação.
  }
}

function chimeTone(ctx: AudioContext, freq: number, start: number, duration: number, peak: number): void {
  const osc = ctx.createOscillator();
  const gain = ctx.createGain();
  osc.type = "sine";
  osc.frequency.value = freq;
  gain.gain.setValueAtTime(0.0001, start);
  gain.gain.exponentialRampToValueAtTime(peak, start + 0.02);
  gain.gain.exponentialRampToValueAtTime(0.0001, start + duration);
  osc.connect(gain);
  gain.connect(ctx.destination);
  osc.start(start);
  osc.stop(start + duration + 0.05);
}

// ── Notificação ───────────────────────────────────────────────────────────────

/**
 * Dispara a notificação de sistema. Permissão "granted" é pré-condição; o
 * caller decide QUANDO (aba oculta) e COM QUAL som. Retorna true quando a
 * notificação foi exibida.
 */
export async function showSystemNotification(opts: SystemNotificationOptions): Promise<boolean> {
  if (!isSupported() || getPermission() !== "granted") return false;
  // `title` é argumento separado nas duas APIs (new Notification e
  // ServiceWorkerRegistration.showNotification); o campo entra no payload
  // só para o showNotification do SW.
  const payload: NotificationOptions & { title: string; data?: { link?: string } } = {
    title: opts.title,
    body: opts.body ?? "",
    tag: opts.tag,
    icon: opts.icon ?? DEFAULT_ICON,
    badge: opts.badge ?? DEFAULT_BADGE,
    // Som do sistema: silent=false deixa o Chrome tocar o som padrão do OS.
    // `renotify` fica no default false (notificações com a mesma tag não
    // re-alertam) — o TS DOM lib deste repo não declara o campo.
    silent: opts.sound !== true,
    data: { link: opts.link },
  };
  try {
    // Upgrade path: com service worker registrado (futuro Push API), a
    // notificação sai pelo SW; sem SW, o construtor cobre aba em background.
    const reg = "serviceWorker" in navigator ? await navigator.serviceWorker.getRegistration() : undefined;
    if (reg) {
      await reg.showNotification(payload.title, payload);
    } else {
      const notification = new window.Notification(payload.title, payload);
      notification.addEventListener("click", () => handleNotificationClick(opts.link));
    }
    return true;
  } catch {
    return false;
  }
}

/** Foca a janela e navega para a rota da notificação (SPA, sem reload). */
export function handleNotificationClick(link?: string): void {
  try {
    const event = new CustomEvent<{ link?: string }>(CLICK_EVENT, {
      detail: { link },
      cancelable: true,
    });
    // dispatchEvent devolve false quando um listener chamou preventDefault
    // (o SPA tratou a navegação). Sem listener, cai no fallback de navegação
    // completa — module usado fora da árvore React.
    const defaultNotPrevented = window.dispatchEvent(event);
    if (defaultNotPrevented && link) {
      window.location.assign(link);
    }
  } catch {
    // Foco mesmo sem navegação — o usuário clicou, quer ver a janela.
  }
  try {
    window.focus();
  } catch {
    // Foco é melhor esforço.
  }
}

/** Listener para usar no React Router: navega sem reload quando a notificação é clicada. */
export function subscribeSystemNotificationClick(navigate: (to: string) => void): () => void {
  const listener = (e: Event) => {
    const custom = e as CustomEvent<{ link?: string }>;
    const link = custom.detail?.link;
    if (link && link.startsWith("/")) {
      e.preventDefault(); // o SPA tratou — desliga o fallback location.assign
      navigate(link);
    }
  };
  window.addEventListener(CLICK_EVENT, listener);
  return () => window.removeEventListener(CLICK_EVENT, listener);
}

/** Política de exibição: notificação de sistema só com a aba em segundo plano. */
export function shouldNotifyFromDocumentState(): boolean {
  return typeof document !== "undefined" && document.hidden;
}
