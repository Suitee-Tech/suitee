/**
 * Catálogo único das personalizações de marca do painel.
 *
 * A fonte e o traço dos ícones são aplicados pelo ThemeProvider em variáveis
 * CSS no `document.documentElement`. Manter os valores permitidos em um só
 * lugar evita que a UI ofereça uma opção que o backend recusa (ou vice-versa).
 */

export type TenantFont =
  | "Inter"
  | "DM Sans"
  | "Poppins"
  | "Roboto"
  | "Nunito"
  | "Open Sans";

export type TenantIconStyle = "regular" | "light" | "bold" | "sharp";

export interface TenantFontOption {
  value: TenantFont;
  label: string;
  /** Consulta do Google Fonts; Inter já é carregada pelo CSS base. */
  googleFamily?: string;
  stack: string;
}

const SANS_FALLBACK = 'ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif';

export const TENANT_FONTS: TenantFontOption[] = [
  {
    value: "Inter",
    label: "Inter (padrão)",
    stack: `"Inter", ${SANS_FALLBACK}`,
  },
  {
    value: "DM Sans",
    label: "DM Sans",
    googleFamily: "DM+Sans:wght@400;500;600;700;800",
    stack: `"DM Sans", ${SANS_FALLBACK}`,
  },
  {
    value: "Poppins",
    label: "Poppins",
    googleFamily: "Poppins:wght@400;500;600;700;800",
    stack: `"Poppins", ${SANS_FALLBACK}`,
  },
  {
    value: "Roboto",
    label: "Roboto",
    googleFamily: "Roboto:wght@400;500;700;900",
    stack: `"Roboto", ${SANS_FALLBACK}`,
  },
  {
    value: "Nunito",
    label: "Nunito",
    googleFamily: "Nunito:wght@400;500;600;700;800",
    stack: `"Nunito", ${SANS_FALLBACK}`,
  },
  {
    value: "Open Sans",
    label: "Open Sans",
    googleFamily: "Open+Sans:wght@400;500;600;700;800",
    stack: `"Open Sans", ${SANS_FALLBACK}`,
  },
];

export const FONT_OPTIONS = TENANT_FONTS.map(({ value, label }) => ({ value, label }));

export const DEFAULT_TENANT_FONT: TenantFont = "Inter";

export function normalizeTenantFont(value: unknown): TenantFont {
  return TENANT_FONTS.some((font) => font.value === value)
    ? (value as TenantFont)
    : DEFAULT_TENANT_FONT;
}

export function tenantFont(value: unknown): TenantFontOption {
  const normalized = normalizeTenantFont(value);
  return TENANT_FONTS.find((font) => font.value === normalized) ?? TENANT_FONTS[0];
}

export function tenantFontStack(value: unknown): string {
  return tenantFont(value).stack;
}

/** Carrega a fonte escolhida sem bloquear a pintura inicial (display=swap). */
export function ensureTenantFont(value: unknown): void {
  if (typeof document === "undefined") return;
  const font = tenantFont(value);
  if (!font.googleFamily) return;

  const href = `https://fonts.googleapis.com/css2?family=${font.googleFamily}&display=swap`;
  let link = document.getElementById("suitee-tenant-font") as HTMLLinkElement | null;
  if (!link) {
    link = document.createElement("link");
    link.id = "suitee-tenant-font";
    link.rel = "stylesheet";
    document.head.appendChild(link);
  }
  if (link.href !== href) link.href = href;

  // Preconnect é idempotente e reduz a latência do primeiro carregamento.
  for (const [id, crossOrigin] of [["suitee-fonts-google", ""], ["suitee-fonts-gstatic", "anonymous"]] as const) {
    if (document.getElementById(id)) continue;
    const preconnect = document.createElement("link");
    preconnect.id = id;
    preconnect.rel = "preconnect";
    preconnect.href = id === "suitee-fonts-google" ? "https://fonts.googleapis.com" : "https://fonts.gstatic.com";
    if (crossOrigin) preconnect.crossOrigin = crossOrigin;
    document.head.appendChild(preconnect);
  }
}

export interface TenantIconStyleOption {
  value: TenantIconStyle;
  label: string;
  description: string;
  strokeWidth: string;
  linecap: string;
  linejoin: string;
}

export const TENANT_ICON_STYLES: TenantIconStyleOption[] = [
  { value: "regular", label: "Padrão", description: "Traço equilibrado", strokeWidth: "2", linecap: "round", linejoin: "round" },
  { value: "light", label: "Leve", description: "Mais delicado", strokeWidth: "1.5", linecap: "round", linejoin: "round" },
  { value: "bold", label: "Forte", description: "Mais presença", strokeWidth: "2.5", linecap: "round", linejoin: "round" },
  { value: "sharp", label: "Angular", description: "Pontas retas", strokeWidth: "1.8", linecap: "butt", linejoin: "miter" },
];

export const DEFAULT_TENANT_ICON_STYLE: TenantIconStyle = "regular";

export function normalizeTenantIconStyle(value: unknown): TenantIconStyle {
  return TENANT_ICON_STYLES.some((style) => style.value === value)
    ? (value as TenantIconStyle)
    : DEFAULT_TENANT_ICON_STYLE;
}

export function tenantIconStyle(value: unknown): TenantIconStyleOption {
  const normalized = normalizeTenantIconStyle(value);
  return TENANT_ICON_STYLES.find((style) => style.value === normalized) ?? TENANT_ICON_STYLES[0];
}
