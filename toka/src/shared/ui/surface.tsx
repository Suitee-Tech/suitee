"use client";

import { type ElementType, forwardRef, type HTMLAttributes } from "react";
import { clsx } from "clsx";

import type { ElevationToken, RadiusToken } from "../tokens/types";

const RADIUS_CLASS: Record<RadiusToken, string> = {
  none: "rounded-none",
  xs: "rounded-xs",
  sm: "rounded-sm",
  md: "rounded-md",
  lg: "rounded-lg",
  xl: "rounded-xl",
  "2xl": "rounded-2xl",
  full: "rounded-full",
};

const ELEVATION_CLASS: Record<ElevationToken, string> = {
  none: "shadow-none",
  xs: "shadow-token-xs",
  sm: "shadow-token-sm",
  md: "shadow-token-md",
  lg: "shadow-token-lg",
  xl: "shadow-token-xl",
};

type Tone = "default" | "muted" | "popover";

const TONE_CLASS: Record<Tone, string> = {
  default: "bg-card text-card-foreground",
  muted: "bg-muted text-muted-foreground",
  popover: "bg-popover text-popover-foreground",
};

interface SurfaceProps extends HTMLAttributes<HTMLElement> {
  as?: ElementType;
  tone?: Tone;
  radius?: RadiusToken;
  elevation?: ElevationToken;
  bordered?: boolean;
}

export const Surface = forwardRef<HTMLElement, SurfaceProps>(function Surface(
  {
    as: Component = "div",
    tone = "default",
    radius = "lg",
    elevation = "sm",
    bordered = true,
    className,
    ...rest
  },
  ref,
) {
  return (
    <Component
      ref={ref as any}
      className={clsx(
        TONE_CLASS[tone],
        RADIUS_CLASS[radius],
        ELEVATION_CLASS[elevation],
        bordered && "border border-border",
        className,
      )}
      {...rest}
    />
  );
});
