import { ListOrdered } from "lucide-react";
import { Badge } from "../../ui/badge";

const GROUP_TONES: string[] = [
  "border-primary-200 bg-primary-50 text-primary-700 dark:border-primary-700 dark:bg-primary-900/30 dark:text-primary-300",
  "border-status-info/30 bg-status-info-soft text-status-info",
  "border-status-warning/30 bg-status-warning-soft text-status-warning",
  "border-status-ok/30 bg-status-ok-soft text-status-ok",
  "border-status-critical/30 bg-status-critical-soft text-status-critical",
];

interface Props {
  group: number;
  active?: boolean;
  size?: "sm" | "md";
  suffix?: string;
}

export function SigningGroupBadge({ group, active = false, size = "sm", suffix }: Props) {
  if (group === 0) {
    return (
      <Badge variant="outline" className="font-medium text-muted-foreground">
        Sem ordem
      </Badge>
    );
  }
  const tone = GROUP_TONES[(group - 1) % GROUP_TONES.length];
  const sizeClass = size === "sm" ? "px-2 py-0.5 text-xs" : "px-2.5 py-1 text-sm";
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full border font-semibold ${tone} ${sizeClass} ${active ? "ring-2 ring-offset-1 ring-primary-400/40 dark:ring-primary-500/30" : ""}`}
    >
      <ListOrdered className={size === "sm" ? "w-3 h-3" : "w-3.5 h-3.5"} />
      Grupo {group}
      {suffix ? <span className="font-medium opacity-80"> · {suffix}</span> : null}
    </span>
  );
}

