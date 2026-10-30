// Escolha de cor por CONTRASTE (WCAG 2.1) para o white-label.
//
// O tema injeta a cor da imobiliária em `--primary`, que acumula duas funções:
// FUNDO de botão (com `--primary-foreground` por cima) e TEXTO de destaque sobre
// a superfície do modo (`text-primary` — item ativo do menu, aba selecionada).
//
// Um tom médio de marca não serve para as duas ao mesmo tempo. O caso que
// originou isto: `#1C905B` (marca padrão da plataforma) dava 3.99:1 com texto
// branco no botão, 4.21:1 com quase preto e 4.62:1 como texto sobre o card
// escuro — ou seja, nenhuma das duas funções realmente passava. A regra anterior
// decidia pelo limiar `lightness > 60%`, que não mede contraste nenhum.
//
// Aqui a escala da própria marca é percorrida até achar um tom que passe, então
// marcas escuras (#013529) e claras (#FFD700) também ficam cobertas.

const NEAR_BLACK = "#1c1c1c";
const WHITE = "#ffffff";

/** Superfície do modo noturno: `--card` (222 47% 8%). */
export const DARK_SURFACE = "#0b111e";
/** Superfície do modo claro: `--card` (0 0% 100%). */
export const LIGHT_SURFACE = "#ffffff";
/** Mínimo WCAG AA para texto normal. */
export const AA_TEXT = 4.5;

export function hexToHSL(hex: string): { h: number; s: number; l: number } {
  const result = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex);
  if (!result) return { h: 210, s: 40, l: 40 };

  const r = parseInt(result[1], 16) / 255;
  const g = parseInt(result[2], 16) / 255;
  const b = parseInt(result[3], 16) / 255;

  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  let h = 0;
  let s = 0;
  const l = (max + min) / 2;

  if (max !== min) {
    const d = max - min;
    s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
    switch (max) {
      case r: h = ((g - b) / d + (g < b ? 6 : 0)) / 6; break;
      case g: h = ((b - r) / d + 2) / 6; break;
      case b: h = ((r - g) / d + 4) / 6; break;
    }
  }

  return { h: Math.round(h * 360), s: Math.round(s * 100), l: Math.round(l * 100) };
}

export function hslToHex(h: number, s: number, l: number): string {
  s /= 100;
  l /= 100;
  const a = s * Math.min(l, 1 - l);
  const f = (n: number) => {
    const k = (n + h / 30) % 12;
    const color = l - a * Math.max(Math.min(k - 3, 9 - k, 1), -1);
    return Math.round(255 * color).toString(16).padStart(2, "0");
  };
  return `#${f(0)}${f(8)}${f(4)}`;
}

/** Escala da marca (mesma usada pelo `--color-primary-*`). */
export function generatePalette(hex: string): Record<string, string> {
  const { h, s } = hexToHSL(hex);

  const shades: [string, number, number][] = [
    ["50", Math.min(s, 30), 96],
    ["100", Math.min(s, 35), 92],
    ["200", Math.min(s + 5, 50), 82],
    ["300", Math.min(s + 5, 55), 72],
    ["400", Math.min(s + 5, 60), 60],
    ["500", s, 48],
    ["600", s, 38],
    ["700", s, 30],
    ["800", s, 22],
    ["900", Math.max(s - 5, 15), 15],
    ["950", Math.max(s - 10, 10), 8],
  ];

  const palette: Record<string, string> = {};
  for (const [shade, colorSat, light] of shades) {
    palette[shade] = hslToHex(h, colorSat, light);
  }
  return palette;
}

export function relativeLuminance(hex: string): number {
  const normalized = hex.replace("#", "").padEnd(6, "0");
  const channels = [0, 2, 4].map((offset) => {
    const value = parseInt(normalized.slice(offset, offset + 2), 16) / 255;
    return value <= 0.03928 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
}

/** Razão de contraste WCAG entre duas cores hexadecimais (1 a 21). */
export function contrastRatio(a: string, b: string): number {
  const la = relativeLuminance(a);
  const lb = relativeLuminance(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

/**
 * Cor de texto (tripla HSL) que maximiza o contraste sobre `fill`, com a razão
 * obtida — quem chama decide se ela basta (>= AA_TEXT).
 */
export function readableForeground(fill: string): { color: string; ratio: number } {
  const onWhite = contrastRatio(WHITE, fill);
  const onBlack = contrastRatio(NEAR_BLACK, fill);
  return onBlack > onWhite
    ? { color: "0 0% 11%", ratio: onBlack }
    : { color: "0 0% 100%", ratio: onWhite };
}

const defined = (colors: (string | undefined)[]): string[] =>
  colors.filter((color): color is string => Boolean(color));

export interface PrimaryTokens {
  /** Preenchimento (`bg-primary`): comporta o texto do botão em AA. */
  fill: string;
  /** Texto sobre o preenchimento, como tripla HSL. */
  fillForeground: string;
  /** Texto de destaque sobre a superfície do modo (`text-primary`). */
  accentText: string;
}

/**
 * Deriva os três valores de `--primary*` a partir da cor da marca.
 *
 * - `fill`: fica na marca quando algum dos dois textos já passa sobre ela; senão
 *   DESCE a escala (600 → 900) até o branco passar, preservando o botão com texto
 *   branco. Marca muito clara (amarelo) fica como está, com texto quase preto.
 * - `accentText`: o próprio preenchimento quando ele também passa como texto
 *   sobre a superfície do modo (texto e botão com a mesma cor); senão anda na
 *   direção que aumenta o contraste — tons mais claros no noturno, mais escuros
 *   no claro.
 */
export function pickPrimaryTokens(brand: string, mode: "light" | "dark"): PrimaryTokens {
  const palette = generatePalette(brand);
  const surface = mode === "dark" ? DARK_SURFACE : LIGHT_SURFACE;

  const brandForeground = readableForeground(brand);
  let fill = brand;
  let fillForeground = brandForeground.color;

  if (brandForeground.ratio < AA_TEXT) {
    // A marca não comporta nem branco nem quase preto (caso do verde padrão:
    // 3.99 e 4.21). Escurece até o branco passar; se nem isso (marca clara),
    // fica na marca com o melhor texto possível.
    const darker = defined([palette["600"], palette["700"], palette["800"], palette["900"]])
      .find((candidate) => contrastRatio(WHITE, candidate) >= AA_TEXT);
    if (darker) {
      fill = darker;
      fillForeground = "0 0% 100%";
    }
  }

  let accentText = fill;
  if (contrastRatio(accentText, surface) < AA_TEXT) {
    const towardSurface = mode === "dark"
      ? defined([palette["500"], palette["400"], palette["300"], palette["200"]])
      : defined([palette["600"], palette["700"], palette["800"], palette["900"]]);
    accentText = towardSurface.find((candidate) => contrastRatio(candidate, surface) >= AA_TEXT)
      ?? towardSurface[towardSurface.length - 1]
      ?? fill;
  }

  return { fill, fillForeground, accentText };
}
