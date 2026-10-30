import { createContext, useContext, useState, useEffect, useCallback, ReactNode } from "react";
import { useOptionalAuth } from "./AuthContext";
import { settingsService } from "../services/settings.service";
import { DEFAULT_LOGIN_BANNER_ID } from "../loginBanners";
import { contrastRatio, generatePalette, hexToHSL, pickPrimaryTokens } from "./themeContrast";
import { mergeServerAppearance, readPendingAppearance } from "../lib/pendingAppearance";
import {
  ensureTenantFont,
  normalizeTenantFont,
  normalizeTenantIconStyle,
  tenantFontStack,
  tenantIconStyle,
  type TenantIconStyle,
} from "../lib/tenantAppearance";

// ============================================================
// TOKEN CONTRACT
// ============================================================
//
// The theme is split into two layers:
//
//   1. BRANDABLE (white-label). Tenant-specific values that flow through
//      ThemeSettings. The provider applies them to the DOM at runtime via
//      `applyThemeToDOM` — they ultimately mutate the shadcn base CSS vars
//      (--primary, --color-primary-50…950, --foreground, etc.).
//
//        primaryColor, accentColor, fontFamily, logoUrl, sidebarStyle,
//        loginTagline, emailFooter, customDomain, theme (light/dark/auto)
//
//   2. FIXED. Centralized design tokens defined in toka/src/app/tokens and
//      backed by CSS vars in styles/tailwind.css. NEVER tenant-customizable:
//
//        - Status colors (--status-ok / warning / pending / critical / info /
//          neutral). "Erro" is always rose, "ok" is always emerald — the
//          reading vocabulary is shared across the platform for safety.
//        - Stage colors (--stage-negotiation … --stage-cancelled). Each
//          stage carries a distinct semantic meaning that the user learns
//          once.
//        - Elevation (--shadow-token-*), radius scale (--radius-*),
//          typography utilities (text-micro, text-caption, …), z-index.
//
// Consequence for component authors: when expressing semantic intent
// ("approved", "rejected", "rejected document") use status tokens — never
// reach for `bg-status-ok` directly. White-label primary should be reached
// via `bg-primary` (or the runtime palette `--color-primary-{50..950}`).
// ============================================================

// ============================================================
// TYPES
// ============================================================

export interface ThemeSettings {
  primaryColor: string;
  accentColor: string;
  sidebarStyle: "default" | "compact" | "minimal";
  theme: "light" | "dark" | "auto";
  fontFamily: string;
  /** Traço global dos ícones Lucide (Aparência → Tipografia & Layout). */
  iconStyle: TenantIconStyle;
  showLogoLogin: boolean;
  darkModeEnabled: boolean;
  customDomain: string;
  domainVerified: boolean;
  loginTagline: string;
  // Painel esquerdo da tela de login (desktop). White-label: a imobiliária
  // edita em Configurações > Aparência (`settings:manage_appearance` = admin e
  // marketing) e o valor vale para todos os usuários do tenant.
  //
  // A tela de login é pré-autenticação, então não há como buscar o tema no
  // servidor: ela lê a última sessão gravada no localStorage — o mesmo caminho
  // que o resto do tema já usa (STORAGE_KEY, abaixo).
  loginSubtitle: string;
  loginBannerPreset: string;
  loginBannerUrl: string;
  // Qual das duas imagens acima está ativa ("custom" | "preset"). As duas ficam
  // guardadas, para a troca entre elas ser um clique; vazio = sessão gravada
  // antes deste campo, resolvido por `effectiveLoginBannerSource`.
  loginBannerSource: string;
  emailFooter: string;
  logoUrl?: string;
}

interface ThemeContextType {
  theme: ThemeSettings;
  updateTheme: <K extends keyof ThemeSettings>(key: K, value: ThemeSettings[K]) => void;
  setTheme: (settings: ThemeSettings) => void;
  resolvedMode: "light" | "dark";
}

// ============================================================
// DEFAULTS
// ============================================================

export const DEFAULT_THEME: ThemeSettings = {
  primaryColor: "#1C905B",
  accentColor: "#10b981",
  sidebarStyle: "default",
  theme: "light",
  fontFamily: "Inter",
  iconStyle: "regular",
  showLogoLogin: true,
  darkModeEnabled: true,
  customDomain: "crm.suaimobiliaria.com.br",
  domainVerified: false,
  // Os três textos abaixo são os MESMOS defaults de toca-core
  // (settings_service.GetAppearance). Antes divergiam: o frontend trazia
  // "O CRM Imobiliario mais inteligente do mercado." e o Login tinha um
  // fallback próprio, então a primeira visita (sem localStorage) mostrava um
  // texto diferente do que a imobiliária via depois de logar.
  loginTagline: "Sua plataforma imobiliária",
  loginSubtitle:
    "Gerencie leads, automatize sua esteira digital e feche mais negócios com inteligência de dados.",
  loginBannerPreset: DEFAULT_LOGIN_BANNER_ID,
  loginBannerUrl: "",
  // Vazio de propósito: `effectiveLoginBannerSource` decide (URL preenchida →
  // custom), mantendo o comportamento de quem já tem sessão gravada.
  loginBannerSource: "",
  emailFooter: "Enviado por suitee - Gestao Inteligente",
};

const STORAGE_KEY = "suitee-theme-settings";

// ============================================================
// COLOR UTILITIES
// ============================================================
//
// A matemática de cor e a escolha por contraste vivem em `themeContrast.ts`,
// onde são testáveis sem DOM. Aqui só se aplica o resultado ao documento.

// ============================================================
// APPLY THEME TO DOM
// ============================================================

function applyThemeToDOM(settings: ThemeSettings, resolvedMode: "light" | "dark") {
  const root = document.documentElement;

  const primaryPalette = generatePalette(settings.primaryColor);
  for (const [shade, color] of Object.entries(primaryPalette)) {
    root.style.setProperty(`--color-primary-${shade}`, color);
  }

  // `--primary` acumula DUAS funções (fundo de botão e texto de destaque) que
  // pedem tons diferentes; `pickPrimaryTokens` percorre a escala da própria
  // marca até achar tons que passem em AA nas duas — a explicação e os testes
  // estão em themeContrast.ts.
  const { fill, fillForeground, accentText } = pickPrimaryTokens(settings.primaryColor, resolvedMode);
  const hslTriple = (hex: string) => {
    const hsl = hexToHSL(hex);
    return `${hsl.h} ${hsl.s}% ${hsl.l}%`;
  };

  root.style.setProperty("--primary", hslTriple(fill));
  root.style.setProperty("--ring", hslTriple(fill));
  root.style.setProperty("--primary-text", hslTriple(accentText));
  root.style.setProperty("--primary-foreground", fillForeground);

  // A fonte é uma preferência de marca: aplicamos tanto na variável do
  // Tailwind quanto no body. Só mexer em `--font-sans` deixava componentes
  // sem a classe `font-sans` presos na fonte padrão do navegador.
  const fontStack = tenantFontStack(settings.fontFamily);
  ensureTenantFont(settings.fontFamily);
  root.style.setProperty("--font-sans", fontStack);
  root.style.setProperty("--default-font-family", fontStack);
  document.body.style.fontFamily = fontStack;

  const icons = tenantIconStyle(settings.iconStyle);
  root.style.setProperty("--icon-stroke", icons.strokeWidth);
  root.style.setProperty("--icon-linecap", icons.linecap);
  root.style.setProperty("--icon-linejoin", icons.linejoin);

  if (resolvedMode === "dark") {
    root.classList.add("dark");
  } else {
    root.classList.remove("dark");
  }
}

// ============================================================
// CONTEXT
// ============================================================

const ThemeContext = createContext<ThemeContextType>({
  theme: DEFAULT_THEME,
  updateTheme: () => { },
  setTheme: () => { },
  resolvedMode: "light",
});

export function ThemeProvider({ children }: { children: ReactNode }) {
  const auth = useOptionalAuth();
  const user = auth?.user ?? null;
  const [settings, setSettings] = useState<ThemeSettings>(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) {
        return { ...DEFAULT_THEME, ...JSON.parse(stored) };
      }
    } catch {
    }
    return DEFAULT_THEME;
  });

  useEffect(() => {
    if (user) {
      Promise.all([
        settingsService.getAppearance().catch(() => null),
        settingsService.getCompanySettings().catch(() => null),
      ]).then(([apiSettings, companySettings]) => {
        if (apiSettings && Object.keys(apiSettings).length > 0 && apiSettings.primaryColor) {
          // O rascunho local da aba Aparência vence o valor do servidor: a aba
          // aplica as mudanças na hora e só as envia no "Salvar Alterações",
          // então uma recarga antes de salvar devolvia o valor do tenant e a
          // configuração parecia não persistir. Salvar limpa o rascunho e o
          // valor do tenant volta a mandar (é o que propaga mudança do admin).
          const pending = readPendingAppearance() as Partial<ThemeSettings> | null;
          setSettings(prev => {
            // O merge já resolve a precedência correta (rascunho > servidor >
            // valor anterior) e NÃO deve normalizar nada aqui: `mergeServerAppearance`
            // sem `pickOne` devolve `{ ...prev, ...apiSettings, ...pending }`.
            // Só o campo `theme` precisa de normalização (aceita apenas
            // light/dark/auto) — os demais (primaryColor, accentColor,
            // fontFamily...) devem adotar o valor do servidor da NOVA empresa.
            //
            // Regressão corrigida: o `pickOne` anterior era aplicado a TODOS os
            // campos, e como ele só deixava passar "light/dark/auto", cores e
            // fontes eram revertidas para o valor anterior — ao trocar de
            // imobiliária o tema ficava o da empresa anterior.
            const merged = mergeServerAppearance(prev, apiSettings as Partial<ThemeSettings>, pending);
            return {
              ...merged,
              theme: (["light", "dark", "auto"].includes(String(merged.theme))
                ? (merged.theme as ThemeSettings["theme"])
                : prev.theme),
              fontFamily: normalizeTenantFont(merged.fontFamily),
              iconStyle: normalizeTenantIconStyle(merged.iconStyle),
              sidebarStyle: ((pending?.sidebarStyle ?? apiSettings.sidebarStyle) as "default" | "compact" | "minimal") || prev.sidebarStyle,
              logoUrl: companySettings?.logoPreview || prev.logoUrl,
            };
          });
        } else if (companySettings?.logoPreview) {
          setSettings(prev => ({
            ...prev,
            logoUrl: companySettings.logoPreview,
          }));
        }
      }).catch(console.error);
    }
  }, [user]);

  const resolvedMode: "light" | "dark" = (() => {
    if (settings.theme === "auto") {
      return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
    }
    return settings.theme;
  })();

  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
    } catch {
    }
  }, [settings]);

  useEffect(() => {
    applyThemeToDOM(settings, resolvedMode);
  }, [settings, resolvedMode]);

  useEffect(() => {
    if (settings.theme !== "auto") return;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const handler = () => applyThemeToDOM(settings, mq.matches ? "dark" : "light");
    mq.addEventListener("change", handler);
    return () => mq.removeEventListener("change", handler);
  }, [settings]);

  const updateTheme = useCallback(<K extends keyof ThemeSettings>(key: K, value: ThemeSettings[K]) => {
    setSettings(prev => ({ ...prev, [key]: value }));
  }, []);

  return (
    <ThemeContext.Provider value={{ theme: settings, updateTheme, setTheme: setSettings, resolvedMode }}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme() {
  return useContext(ThemeContext);
}

