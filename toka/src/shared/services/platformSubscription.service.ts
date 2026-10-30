import api from "./api";

export type SubscriptionCycle = "MONTHLY" | "ANNUALLY";
export interface PlatformPlan {
  tier: string;
  name: string;
  amount_cents: number;
  cycle: SubscriptionCycle;
  description: string;
  limits: { leads: number; properties: number; users: number };
  contact_only: boolean;
  trial_days: number;
  highlights?: string[];
  features?: string[];
}
export interface PlatformSubscription {
  status: string;
  plan: PlatformPlan | null;
  subscription_id?: string;
  paid_through?: string;
  trial_ends_at?: string;
  /** Set once the free period was served; it is never granted again. */
  trial_granted_at?: string;
  /** No-card first-access trial, kept separate from a provider-backed plan. */
  trial_plan?: PlatformPlan | null;
  cancelled_at?: string;
  cancellation_reason?: string;
  payment_method?: { kind: string; brand?: string; last4?: string; updated_at?: string };
  checkout?: { id: string; url: string; plan: PlatformPlan };
  pending_change?: { id: string; plan: PlatformPlan; requested_at: string };
  dev_mode: boolean;
}
export interface PlatformSubscriptionAccess {
  access_control_enabled: boolean;
  subscription_enabled: boolean;
  entitled: boolean;
  status: string;
  plan: PlatformPlan | null;
  features: string[];
  trial_ends_at?: string;
  paid_through?: string;
  reason?: string;
}
export interface SubscriptionSnapshot {
  subscription: PlatformSubscription;
  access?: PlatformSubscriptionAccess;
  plans: PlatformPlan[];
  payments: { id: string; amount_cents: number; paid_at: string; receipt_url?: string }[];
  usage: { leads: number; properties: number; users: number };
  enabled: boolean;
  access_control_enabled?: boolean;
  dev_mode: boolean;
}

const limitDimensions: Array<keyof PlatformPlan["limits"]> = ["leads", "properties", "users"];

/**
 * Espelha `planAddsBenefits` do backend (toca-financial): o alvo precisa manter
 * TODO limite e recurso atual e crescer em pelo menos um deles. Planos gravados
 * antes do catálogo atual não guardam `features`; nesse caso vale só o que os
 * limites dizem — sem informação de recurso, não se anuncia ganho de recurso.
 */
export function planAddsBenefits(current?: PlatformPlan | null, target?: PlatformPlan | null): boolean {
  if (!current || !target) return false;
  if (limitDimensions.some(key => target.limits[key] < current.limits[key])) return false;
  const addsLimit = limitDimensions.some(key => target.limits[key] > current.limits[key]);
  if (!current.features || !target.features) return addsLimit;

  const currentFeatures = new Set(current.features);
  const targetFeatures = new Set(target.features);
  if ([...currentFeatures].some(feature => !targetFeatures.has(feature))) return false;
  return addsLimit || [...targetFeatures].some(feature => !currentFeatures.has(feature));
}

/** True when the target removes capacity/features and adds nothing in return. */
export function planRemovesBenefits(current?: PlatformPlan | null, target?: PlatformPlan | null): boolean {
  return planAddsBenefits(target, current);
}

/** Reasons offered when an administrator cancels; mirrors the backend allowlist. */
export const cancellationReasons = [
  { value: "too_expensive", label: "Está caro para o momento" },
  { value: "missing_features", label: "Falta um recurso que eu preciso" },
  { value: "not_using", label: "Não estou usando o suficiente" },
  { value: "switching", label: "Vou usar outra ferramenta" },
  { value: "other", label: "Outro motivo" },
] as const;

export function safeSubscriptionURL(raw?: string): string | null {
  if (!raw) return null;
  try {
    const url = new URL(raw);
    return url.protocol === "https:" && url.hostname === "app.abacatepay.com" && !url.port && !url.username && !url.password ? url.href : null;
  } catch { return null; }
}
export const platformSubscriptionService = {
  async getAccess(): Promise<PlatformSubscriptionAccess> {
    const res = await api.get<{ data: PlatformSubscriptionAccess }>("/platform-subscription/access");
    return res.data.data;
  },
  async get(): Promise<SubscriptionSnapshot> {
    const res = await api.get<{ data: SubscriptionSnapshot }>("/platform-subscription");
    return res.data.data;
  },
  async select(tier: string, cycle: SubscriptionCycle): Promise<PlatformSubscription> {
    const res = await api.post<{ data: PlatformSubscription }>("/platform-subscription/checkout", { tier, cycle });
    return res.data.data;
  },
  async cancel(reason?: string): Promise<PlatformSubscription> {
    const res = await api.post<{ data: PlatformSubscription }>("/platform-subscription/cancel", reason ? { reason } : {});
    return res.data.data;
  },
  /** Removes a scheduled change; the plan in force keeps being charged. */
  async cancelChange(): Promise<PlatformSubscription> {
    const res = await api.post<{ data: PlatformSubscription }>("/platform-subscription/change/cancel");
    return res.data.data;
  },
  /** Receipt for one confirmed payment, issued by the platform. */
  async receipt(paymentId: string): Promise<Blob> {
    const res = await api.get(`/platform-subscription/payments/${encodeURIComponent(paymentId)}/receipt`, { responseType: "blob" });
    return res.data as Blob;
  },
  /** Starts a new subscription so the customer can register another card. */
  async changePaymentMethod(): Promise<PlatformSubscription> {
    const res = await api.post<{ data: PlatformSubscription }>("/platform-subscription/payment-method");
    return res.data.data;
  },
};
