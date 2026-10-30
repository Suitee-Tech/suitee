import { createRequestId } from "./requestId";
import { getEnv } from "../../lib/runtime-env";

type HTTPFailure = { response?: { status?: number; headers?: Record<string, unknown> }; config?: { method?: string; url?: string; headers?: Record<string, unknown> } };
export type ErrorDiagnostic = {
  kind: "http" | "render";
  code: string;
  request_id?: string;
  operation: string;
  status?: number;
  release: string;
};
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i;
const categories = new Set(["appointments", "calendar", "contracts", "deals", "leads", "portal", "financial", "charges", "bills", "auth", "properties", "tickets", "channels"]);
const codes = new WeakMap<object, string>();
let sentAt: number[] = [];

function release() {
  const value = getEnv("VITE_APP_VERSION") ?? "unknown";
  return /^(?:v?\d+\.\d+\.\d+(?:[-+][\w.-]+)?|[a-f0-9]{7,40})$/.test(value) ? value : "unknown";
}
export function supportCode(error: unknown): string | undefined {
  if (!error || typeof error !== "object") return undefined;
  const failure = error as HTTPFailure;
  const id = failure.response?.headers?.["x-request-id"] ?? failure.config?.headers?.["X-Request-ID"];
  return typeof id === "string" && uuid.test(id) ? id : codes.get(error);
}
export function supportErrorMessage(error: unknown, fallback: string): string {
  const code = supportCode(error);
  return code ? `${fallback} Código de suporte: ${code}.` : fallback;
}

// Only fixed categories and transport metadata leave the browser. No URLs,
// error messages, stack traces, tokens, identities or form contents are sent.
export function reportClientError(error: unknown, kind: ErrorDiagnostic["kind"] = "http"): string {
  const failure = (error && typeof error === "object" ? error : {}) as HTTPFailure;
  const code = supportCode(error) ?? createRequestId();
  if (error && typeof error === "object") codes.set(error, code);
  const method = String(failure.config?.method ?? "GET").toUpperCase();
  const validMethod = ["GET", "POST", "PUT", "PATCH", "DELETE"].includes(method) ? method : "OTHER";
  let category = "other";
  try {
    const path = new URL(failure.config?.url ?? "", "https://diagnostic.invalid").pathname;
    category = path.split("/").find((part) => categories.has(part)) ?? "other";
  } catch { /* malformed URL cannot be included in diagnostics */ }
  const diagnostic: ErrorDiagnostic = { kind, code, operation: kind === "render" ? "render" : `${validMethod} ${category}`, release: release() };
  if (kind === "http") {
    diagnostic.request_id = supportCode(error);
    diagnostic.status = failure.response?.status ?? 0;
  }
  const now = Date.now();
  sentAt = sentAt.filter((at) => now - at < 60_000);
  if (sentAt.length >= 20) return code;
  sentAt.push(now);
  console.error("[ClientError]", diagnostic);
  try {
    const base = getEnv("VITE_API_URL") || "/api/v1";
    void fetch(`${base.replace(/\/$/, "")}/client-errors`, {
      method: "POST", credentials: "omit", keepalive: true,
      headers: { "Content-Type": "application/json" }, body: JSON.stringify(diagnostic),
    }).catch(() => {});
  } catch { /* diagnostics must never break the original recovery path */ }
  return code;
}
