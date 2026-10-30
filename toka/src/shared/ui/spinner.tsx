import * as React from "react";
import { Loader2 } from "lucide-react";

import { cn } from "./utils";

const spinnerSizes = {
  xs: "size-3",
  sm: "size-4",
  md: "size-5",
  lg: "size-6",
} as const;

type SpinnerProps = React.ComponentPropsWithoutRef<typeof Loader2> & {
  size?: keyof typeof spinnerSizes;
  /** Accessible label announced by screen readers. */
  label?: string;
};

/**
 * Single spinner primitive for the whole app. Inherits the parent's text
 * color via `currentColor`, so it works in light and dark mode with no token.
 */
const Spinner = React.forwardRef<SVGSVGElement, SpinnerProps>(
  ({ size = "sm", label = "Carregando", className, ...props }, ref) => (
    <Loader2
      ref={ref}
      role="status"
      aria-label={label}
      className={cn("animate-spin", spinnerSizes[size], className)}
      {...props}
    />
  ),
);
Spinner.displayName = "Spinner";

export { Spinner };
