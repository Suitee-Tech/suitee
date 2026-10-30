const brl = new Intl.NumberFormat("pt-BR", {
  style: "currency",
  currency: "BRL",
  maximumFractionDigits: 0,
});

const shortDate = new Intl.DateTimeFormat("pt-BR", {
  day: "2-digit",
  month: "short",
  year: "numeric",
});

export function formatBRL(value: number | null | undefined): string {
  if (value == null || Number.isNaN(value)) return brl.format(0);
  return brl.format(value);
}

export type LeadDateVariant = "short" | "numeric";

export function formatLeadDate(value: string | Date | null | undefined, variant: LeadDateVariant = "numeric"): string {
  if (!value) return "—";
  try {
    const d = value instanceof Date ? value : new Date(value);
    if (Number.isNaN(d.getTime())) return typeof value === "string" ? value : "—";
    if (variant === "short") return shortDate.format(d);
    return d.toLocaleDateString("pt-BR");
  } catch {
    return typeof value === "string" ? value : "—";
  }
}
