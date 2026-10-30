import ptBR from "./pt-BR";

export type Locale = "pt-BR";

const DICTIONARIES: Record<Locale, Record<string, string>> = {
  "pt-BR": ptBR,
};

const DEFAULT_LOCALE: Locale = "pt-BR";

export function t(key: string, vars?: Record<string, string | number>, locale: Locale = DEFAULT_LOCALE): string {
  const dict = DICTIONARIES[locale] ?? DICTIONARIES[DEFAULT_LOCALE];
  let value = dict[key];
  if (value === undefined) {
    if (typeof process !== "undefined" && process.env?.NODE_ENV !== "production") {
      console.warn(`[i18n] missing key for locale ${locale}: ${key}`);
    }
    value = key;
  }
  if (!vars) return value;
  return value.replace(/\{(\w+)\}/g, (_, name) => {
    const v = vars[name];
    return v === undefined || v === null ? `{${name}}` : String(v);
  });
}

export const DICT_PT_BR = ptBR;

