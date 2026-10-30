import type {
  DealStageToken,
  ElevationToken,
  RadiusToken,
  SpacingToken,
  StatusTone,
  TypographyToken,
  ZToken,
} from "./types";

const cssVar = (name: string) => `hsl(var(${name}))`;
const cssRaw = (name: string) => `var(${name})`;

export const statusTokens: Record<
  StatusTone,
  {
    solid: string;
    soft: string;
    text: string;
    onSolid: string;
    tw: { bg: string; bgSoft: string; text: string; textOnSolid: string; border: string; ring: string };
  }
> = {
  ok: {
    solid: cssVar("--status-ok"),
    soft: cssVar("--status-ok-soft"),
    text: cssVar("--status-ok"),
    onSolid: cssVar("--status-ok-foreground"),
    tw: {
      bg: "bg-status-ok",
      bgSoft: "bg-status-ok-soft",
      text: "text-status-ok",
      textOnSolid: "text-status-ok-foreground",
      border: "border-status-ok",
      ring: "ring-status-ok",
    },
  },
  warning: {
    solid: cssVar("--status-warning"),
    soft: cssVar("--status-warning-soft"),
    text: cssVar("--status-warning"),
    onSolid: cssVar("--status-warning-foreground"),
    tw: {
      bg: "bg-status-warning",
      bgSoft: "bg-status-warning-soft",
      text: "text-status-warning",
      textOnSolid: "text-status-warning-foreground",
      border: "border-status-warning",
      ring: "ring-status-warning",
    },
  },
  pending: {
    solid: cssVar("--status-pending"),
    soft: cssVar("--status-pending-soft"),
    text: cssVar("--status-pending"),
    onSolid: cssVar("--status-pending-foreground"),
    tw: {
      bg: "bg-status-pending",
      bgSoft: "bg-status-pending-soft",
      text: "text-status-pending",
      textOnSolid: "text-status-pending-foreground",
      border: "border-status-pending",
      ring: "ring-status-pending",
    },
  },
  critical: {
    solid: cssVar("--status-critical"),
    soft: cssVar("--status-critical-soft"),
    text: cssVar("--status-critical"),
    onSolid: cssVar("--status-critical-foreground"),
    tw: {
      bg: "bg-status-critical",
      bgSoft: "bg-status-critical-soft",
      text: "text-status-critical",
      textOnSolid: "text-status-critical-foreground",
      border: "border-status-critical",
      ring: "ring-status-critical",
    },
  },
  info: {
    solid: cssVar("--status-info"),
    soft: cssVar("--status-info-soft"),
    text: cssVar("--status-info"),
    onSolid: cssVar("--status-info-foreground"),
    tw: {
      bg: "bg-status-info",
      bgSoft: "bg-status-info-soft",
      text: "text-status-info",
      textOnSolid: "text-status-info-foreground",
      border: "border-status-info",
      ring: "ring-status-info",
    },
  },
  neutral: {
    solid: cssVar("--status-neutral"),
    soft: cssVar("--status-neutral-soft"),
    text: cssVar("--status-neutral"),
    onSolid: cssVar("--status-neutral-foreground"),
    tw: {
      bg: "bg-status-neutral",
      bgSoft: "bg-status-neutral-soft",
      text: "text-status-neutral",
      textOnSolid: "text-status-neutral-foreground",
      border: "border-status-neutral",
      ring: "ring-status-neutral",
    },
  },
};

export const stageTokens: Record<DealStageToken, { solid: string; tw: { bg: string; text: string; ring: string } }> = {
  negotiation: { solid: cssVar("--stage-negotiation"), tw: { bg: "bg-stage-negotiation", text: "text-stage-negotiation", ring: "ring-stage-negotiation" } },
  proposal:    { solid: cssVar("--stage-proposal"),    tw: { bg: "bg-stage-proposal",    text: "text-stage-proposal",    ring: "ring-stage-proposal" } },
  analysis:    { solid: cssVar("--stage-analysis"),    tw: { bg: "bg-stage-analysis",    text: "text-stage-analysis",    ring: "ring-stage-analysis" } },
  contract:    { solid: cssVar("--stage-contract"),    tw: { bg: "bg-stage-contract",    text: "text-stage-contract",    ring: "ring-stage-contract" } },
  converted:   { solid: cssVar("--stage-converted"),   tw: { bg: "bg-stage-converted",   text: "text-stage-converted",   ring: "ring-stage-converted" } },
  cancelled:   { solid: cssVar("--stage-cancelled"),   tw: { bg: "bg-stage-cancelled",   text: "text-stage-cancelled",   ring: "ring-stage-cancelled" } },
};

export const surfaceTokens = {
  app: { value: cssVar("--background"), tw: "bg-background" },
  card: { value: cssVar("--card"), tw: "bg-card" },
  muted: { value: cssVar("--muted"), tw: "bg-muted" },
  popover: { value: cssVar("--popover"), tw: "bg-popover" },
  border: { value: cssVar("--border"), tw: "border-border" },
} as const;

export const radiusTokens: Record<RadiusToken, { value: string; tw: string }> = {
  none: { value: "0", tw: "rounded-none" },
  xs:   { value: cssRaw("--radius-xs"), tw: "rounded-xs" },
  sm:   { value: cssRaw("--radius-sm"), tw: "rounded-sm" },
  md:   { value: cssRaw("--radius-md"), tw: "rounded-md" },
  lg:   { value: cssRaw("--radius-lg"), tw: "rounded-lg" },
  xl:   { value: cssRaw("--radius-xl"), tw: "rounded-xl" },
  "2xl":{ value: cssRaw("--radius-2xl"), tw: "rounded-2xl" },
  full: { value: "9999px", tw: "rounded-full" },
};

export const elevationTokens: Record<ElevationToken, { value: string; tw: string }> = {
  none: { value: "none", tw: "shadow-none" },
  xs:   { value: cssRaw("--shadow-xs"), tw: "shadow-token-xs" },
  sm:   { value: cssRaw("--shadow-sm"), tw: "shadow-token-sm" },
  md:   { value: cssRaw("--shadow-md"), tw: "shadow-token-md" },
  lg:   { value: cssRaw("--shadow-lg"), tw: "shadow-token-lg" },
  xl:   { value: cssRaw("--shadow-xl"), tw: "shadow-token-xl" },
};

export const typographyTokens: Record<TypographyToken, string> = {
  micro: "text-micro",
  caption: "text-caption",
  label: "text-label",
  body: "text-body",
  bodyLg: "text-body-lg",
  title: "text-title",
  titleLg: "text-title-lg",
  section: "text-section",
  headingSm: "text-heading-sm",
  heading: "text-heading",
  displaySm: "text-display-sm",
  display: "text-display",
};

export const spacingTokens: Record<SpacingToken, { rem: string; tw: { p: string; gap: string; m: string } }> = {
  xs: { rem: "0.25rem", tw: { p: "p-1", gap: "gap-1", m: "m-1" } },
  sm: { rem: "0.5rem",  tw: { p: "p-2", gap: "gap-2", m: "m-2" } },
  md: { rem: "0.75rem", tw: { p: "p-3", gap: "gap-3", m: "m-3" } },
  lg: { rem: "1rem",    tw: { p: "p-4", gap: "gap-4", m: "m-4" } },
  xl: { rem: "1.5rem",  tw: { p: "p-6", gap: "gap-6", m: "m-6" } },
  "2xl": { rem: "2rem", tw: { p: "p-8", gap: "gap-8", m: "m-8" } },
};

export const zTokens: Record<ZToken, { value: number; tw: string }> = {
  base:     { value: 0,  tw: "z-0" },
  dropdown: { value: 20, tw: "z-20" },
  sticky:   { value: 30, tw: "z-30" },
  overlay:  { value: 40, tw: "z-40" },
  modal:    { value: 50, tw: "z-50" },
  toast:    { value: 60, tw: "z-[60]" },
};

export const tokens = {
  color: {
    status: statusTokens,
    stage: stageTokens,
    surface: surfaceTokens,
  },
  radius: radiusTokens,
  elevation: elevationTokens,
  typography: typographyTokens,
  spacing: spacingTokens,
  z: zTokens,
} as const;

export type { DealStageToken, ElevationToken, RadiusToken, SpacingToken, StatusTone, TypographyToken, ZToken };
