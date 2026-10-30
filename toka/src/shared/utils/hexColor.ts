/**
 * Cor em hexadecimal — a forma que a API, o tema e o `<input type="color">`
 * aceitam.
 *
 * O seletor nativo do navegador só aceita `#rrggbb` completo e minúsculo. O
 * campo de texto ao lado aceita qualquer coisa enquanto o operador digita, e o
 * banco pode ter valores antigos em caixa alta (os presets do white-label são
 * gravados como `#1C905B`). Sem normalizar, o seletor cai para preto e o tema
 * aplica uma cor diferente da que está escrita na tela.
 */
export function normalizeHexColor(value: string | null | undefined): string | null {
  if (typeof value !== "string") return null;
  const raw = value.trim().replace(/^#/, "");
  // `#abc` é a forma curta de `#aabbcc`.
  const expanded =
    raw.length === 3
      ? raw
          .split("")
          .map((char) => char + char)
          .join("")
      : raw;
  if (!/^[0-9a-fA-F]{6}$/.test(expanded)) return null;
  return `#${expanded.toLowerCase()}`;
}

/** `true` quando o valor é uma cor hexadecimal que o tema consegue aplicar. */
export function isHexColor(value: string | null | undefined): boolean {
  return normalizeHexColor(value) !== null;
}

/**
 * Cor de texto/ícone legível SOBRE `value` (luminância YIQ, a mesma regra que a
 * pré-visualização do white-label já usava): escuro em cor clara, branco em cor
 * escura. Valor inválido cai no escuro, que é o caso do texto sobre `bg-card`.
 */
export function readableTextColor(value: string | null | undefined): "#1E293B" | "#FFFFFF" {
  const hex = normalizeHexColor(value);
  if (!hex) return "#1E293B";
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  const yiq = (r * 299 + g * 587 + b * 114) / 1000;
  return yiq >= 128 ? "#1E293B" : "#FFFFFF";
}
