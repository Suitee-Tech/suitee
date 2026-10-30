import { useState } from "react";
import { ChevronDown } from "lucide-react";
import { clsx } from "clsx";

export function PartyChecklistSection({
  title,
  defaultOpen = true,
  children,
}: {
  title: string;
  defaultOpen?: boolean;
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <div className="border border-border rounded-2xl bg-card overflow-hidden">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="w-full flex items-center justify-between px-4 py-3 hover:bg-muted transition-colors"
      >
        <span className="text-sm font-bold text-foreground">{title}</span>
        <ChevronDown className={clsx("w-4 h-4 text-muted-foreground transition-transform", open && "rotate-180")} />
      </button>
      {open && <div className="p-3 space-y-2">{children}</div>}
    </div>
  );
}

