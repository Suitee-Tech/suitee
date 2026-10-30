import { getEnv } from "../lib/runtime-env";
import { getImgproxyUrl } from "./imgproxy";

const STORAGE_PATH_PREFIX = "/storage/v1/";
const AUTHENTICATED_OBJECT_PREFIX = "/storage/v1/object/authenticated/";
const ABSOLUTE_SCHEME = /^[a-z][a-z\d+.-]*:/i;
const SAFE_INLINE_MEDIA = /^(blob:|data:image\/)/i;
const UNSAFE_MEDIA_SCHEME = /^(javascript:|vbscript:|file:|data:(?!image\/))/i;

const browserOrigin = () =>
  typeof window !== "undefined" && window.location?.origin
    ? window.location.origin
    : "http://localhost";

const configuredUrl = (key: string): URL | null => {
  const value = getEnv(key);
  if (!value) return null;
  try {
    return new URL(value);
  } catch {
    return null;
  }
};

const mediaBaseUrl = () =>
  configuredUrl("VITE_MEDIA_URL") ??
  configuredUrl("VITE_API_URL") ??
  new URL(browserOrigin());

const parseMediaUrl = (raw: string): URL | null => {
  try {
    return new URL(raw, browserOrigin());
  } catch {
    return null;
  }
};

const isStoragePath = (pathname: string) => pathname.startsWith(STORAGE_PATH_PREFIX);

const isInternalStorageHost = (hostname: string) => {
  const host = hostname.toLowerCase().replace(/\.$/, "");
  return (
    host === "host.docker.internal" ||
    host === "localhost" ||
    host === "127.0.0.1" ||
    host === "[::1]" ||
    host === "kong" ||
    host === "storage" ||
    host.startsWith("toca-") ||
    host.startsWith("supabase-")
  );
};

const withPublicSupabaseOrigin = (source: URL, publicProjectUrl: URL): string => {
  const target = new URL(publicProjectUrl.toString());
  const projectPrefix = target.pathname
    .replace(/\/+$/, "")
    .replace(/\/storage\/v1$/i, "");

  target.pathname = `${projectPrefix}${source.pathname}`.replace(/\/{2,}/g, "/");
  target.search = source.search;
  target.hash = source.hash;
  return target.toString();
};

/**
 * Normaliza apenas URLs do Supabase Storage.
 *
 * URLs públicas e assinadas podem ser renderizadas diretamente. A rota
 * `/object/authenticated/`, por outro lado, exige um header Authorization que
 * uma tag <img> não consegue enviar; por isso ela nunca é exposta como URL de
 * mídia renderizável.
 */
export const normalizeStorageUrl = (url: string | null | undefined): string => {
  const raw = String(url ?? "").trim();
  if (!raw) return "";
  if (SAFE_INLINE_MEDIA.test(raw)) return raw;
  if (UNSAFE_MEDIA_SCHEME.test(raw)) return "";

  const parsed = parseMediaUrl(raw);
  if (!parsed || !isStoragePath(parsed.pathname)) return raw;
  if (parsed.pathname.startsWith(AUTHENTICATED_OBJECT_PREFIX)) return "";

  const publicProjectUrl = configuredUrl("VITE_SUPABASE_URL");
  if (!publicProjectUrl) return raw;

  const isRelative = !ABSOLUTE_SCHEME.test(raw) && !raw.startsWith("//");
  const isConfiguredOrigin = parsed.origin === publicProjectUrl.origin;
  const shouldRebase =
    isRelative ||
    (!isConfiguredOrigin && isInternalStorageHost(parsed.hostname));

  return shouldRebase
    ? withPublicSupabaseOrigin(parsed, publicProjectUrl)
    : raw;
};

/**
 * Fonte única para URLs usadas por imagens, avatares, logos e previews.
 * Mantém compatibilidade com uploads legados, mas nunca devolve protocolos
 * inseguros nem uma rota privada do Supabase sem assinatura.
 */
export const resolveMediaUrl = (url: string | null | undefined): string | null => {
  const raw = String(url ?? "").trim();
  if (!raw) return null;
  if (SAFE_INLINE_MEDIA.test(raw)) return raw;
  if (UNSAFE_MEDIA_SCHEME.test(raw)) return null;

  const normalized = normalizeStorageUrl(raw);
  if (!normalized) return null;

  const parsed = parseMediaUrl(normalized);
  if (!parsed) return null;

  if (isStoragePath(parsed.pathname)) {
    return normalized;
  }

  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    return null;
  }

  const isRelative = !ABSOLUTE_SCHEME.test(raw) && !raw.startsWith("//");
  const isLegacyUpload = parsed.pathname.startsWith("/uploads/");
  const isLegacyInternalHost =
    parsed.hostname === "localhost" ||
    parsed.hostname === "127.0.0.1" ||
    parsed.hostname === "[::1]" ||
    parsed.hostname === "host.docker.internal" ||
    parsed.hostname.startsWith("toca-");

  if (isLegacyUpload || (!isRelative && isLegacyInternalHost)) {
    const mediaBase = mediaBaseUrl();
    const target = new URL(mediaBase.toString());
    target.pathname = parsed.pathname;
    target.search = parsed.search;
    target.hash = parsed.hash;
    return target.toString();
  }

  return raw;
};

export const getMediaUrl = (url: string | null | undefined) =>
  resolveMediaUrl(url) ?? "";

export const getAvatarThumbUrl = (url: string | null | undefined, size = 64) => {
  const raw = getMediaUrl(url);
  if (!raw) return "";
  return getImgproxyUrl(raw, { width: size, height: size });
};
