import { Share2 } from "lucide-react";
import { clsx } from "clsx";

export function SharedBadge({
  companyName,
  className,
  size = "sm",
}: {
  companyName: string;
  className?: string;
  size?: "sm" | "md";
}) {
  return (
    <span
      className={clsx(
        "inline-flex items-center gap-1 rounded-md font-bold uppercase tracking-wider",
        "bg-stage-contract/10 text-stage-contract border border-stage-contract/20",
        size === "sm" ? "px-2 py-0.5 text-caption" : "px-2.5 py-1 text-xs",
        className,
      )}
      title={`Imóvel compartilhado por ${companyName}`}
    >
      <Share2 className={size === "sm" ? "w-3 h-3" : "w-3.5 h-3.5"} />
      <span className="truncate">Compartilhado por {companyName}</span>
    </span>
  );
}
