/**
 * Public token types. Components should reference these names rather than
 * importing the runtime values directly when only the type is needed (better
 * tree-shaking, fewer circular imports).
 */

export type StatusTone = "ok" | "warning" | "pending" | "critical" | "info" | "neutral";

export type RadiusToken = "none" | "xs" | "sm" | "md" | "lg" | "xl" | "2xl" | "full";

export type ElevationToken = "none" | "xs" | "sm" | "md" | "lg" | "xl";

export type TypographyToken =
  | "micro"
  | "caption"
  | "label"
  | "body"
  | "bodyLg"
  | "title"
  | "titleLg"
  | "section"
  | "headingSm"
  | "heading"
  | "displaySm"
  | "display";

export type SpacingToken = "xs" | "sm" | "md" | "lg" | "xl" | "2xl";

export type ZToken = "base" | "dropdown" | "sticky" | "overlay" | "modal" | "toast";

export type DealStageToken = "negotiation" | "proposal" | "analysis" | "contract" | "converted" | "cancelled";
