import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";

import { cn } from "./utils";
import { Spinner } from "./spinner";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-[var(--radius)] text-sm font-medium transition-all disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg:not([class*='size-'])]:size-4 shrink-0 [&_svg]:shrink-0 outline-none focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px] aria-invalid:ring-destructive/20 aria-invalid:border-destructive",
  {
    variants: {
      variant: {
        default:
          "bg-primary text-primary-foreground hover:bg-primary/90 shadow-token-sm",
        destructive:
          "bg-destructive text-destructive-foreground hover:bg-destructive/90 focus-visible:ring-destructive/20",
        outline:
          "border border-input bg-background hover:bg-accent hover:text-accent-foreground",
        secondary:
          "bg-secondary text-secondary-foreground hover:bg-secondary/80",
        ghost: "hover:bg-accent hover:text-accent-foreground",
        link: "text-primary underline-offset-4 hover:underline",
        // Semantic variants — soft backgrounds for affirmative/info actions.
        success:
          "bg-status-ok text-status-ok-foreground hover:bg-status-ok/90 shadow-token-sm",
        info: "bg-status-info text-status-info-foreground hover:bg-status-info/90 shadow-token-sm",
        warning:
          "bg-status-warning text-status-warning-foreground hover:bg-status-warning/90 shadow-token-sm",
      },
      size: {
        default: "h-10 px-4 py-2 has-[>svg]:px-3",
        sm: "h-9 rounded-[var(--radius)] gap-1.5 px-3 has-[>svg]:px-2.5",
        lg: "h-11 rounded-[var(--radius)] px-8 has-[>svg]:px-4",
        icon: "size-10 rounded-[var(--radius)]",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
);

// Forward refs so Radix's `asChild` consumers (TooltipTrigger,
// DropdownTrigger, PopoverTrigger, …) can attach the ref they need for
// portaling/focus management. Without forwardRef, "Function components
// cannot be given refs" warnings fire whenever a Button sits inside an
// `asChild` parent — and any focus-management features on that parent
// silently break (e.g. tooltip dismiss, dialog autofocus).
type ButtonProps = React.ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean;
    /** Shows a spinner and disables the button while an async action runs. */
    loading?: boolean;
    /** Replaces the label with this text while loading (spinner stays). */
    loadingText?: string;
  };

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  (
    {
      className,
      variant,
      size,
      asChild = false,
      loading = false,
      loadingText,
      disabled,
      children,
      ...props
    },
    ref,
  ) => {
    const Comp = asChild ? Slot : "button";
    const isDisabled = disabled || loading;

    // `asChild` routes through Radix `Slot`, which accepts exactly one child —
    // injecting a Spinner sibling would throw `React.Children.only`. An
    // asChild button therefore signals loading via aria-busy/disabled only.
    const showSpinner = loading && !asChild;

    // Icon-only buttons swap the icon for the spinner; text buttons keep their
    // label (or show `loadingText`) so the width doesn't jump.
    let content: React.ReactNode = children;
    if (loading && !asChild) {
      if (size === "icon") content = null;
      else if (loadingText) content = loadingText;
    }

    return (
      <Comp
        ref={ref as React.Ref<HTMLButtonElement>}
        data-slot="button"
        data-loading={loading || undefined}
        aria-busy={loading || undefined}
        disabled={isDisabled}
        className={cn(buttonVariants({ variant, size, className }))}
        {...props}
      >
        {showSpinner ? (
          <>
            <Spinner size={size === "lg" ? "md" : "sm"} aria-hidden="true" />
            {content}
          </>
        ) : (
          content
        )}
      </Comp>
    );
  },
);
Button.displayName = "Button";

export { Button, buttonVariants };
