import { Building2, ExternalLink } from "lucide-react";
import { useAgencyInfo } from "../hooks/useAgencyInfo";

interface AgencyIdentityProps {
  tenantId?: string;
  compact?: boolean;
  showSiteLink?: boolean;
  fallbackName?: string;
}

export function AgencyIdentity({
  tenantId,
  compact = false,
  showSiteLink = false,
  fallbackName,
}: AgencyIdentityProps) {
  const { agency } = useAgencyInfo(tenantId);
  const name = agency.name === "Imobiliária" && fallbackName ? fallbackName : agency.name;
  const initial = name.trim().charAt(0).toUpperCase() || "I";

  return (
    <div className="flex min-w-0 items-center gap-2">
      <span
        className="inline-flex size-7 shrink-0 items-center justify-center rounded-lg bg-muted text-[11px] font-bold text-foreground"
        style={agency.color ? { backgroundColor: `${agency.color}1a`, color: agency.color } : undefined}
        aria-hidden="true"
      >
        {compact ? initial : <Building2 className="size-3.5" />}
      </span>
      <span className="min-w-0">
        <span className="block truncate text-xs font-semibold text-foreground">{name}</span>
        {!compact && (
          <span className="block text-[11px] text-muted-foreground">Imobiliária responsável</span>
        )}
      </span>
      {showSiteLink && agency.publicUrl ? (
        <a
          href={agency.publicUrl}
          target="_blank"
          rel="noreferrer"
          className="ml-1 inline-flex min-h-8 shrink-0 items-center gap-1 rounded-lg px-2 text-[11px] font-semibold text-primary-700 hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          aria-label={`Abrir site de ${name}`}
        >
          Site
          <ExternalLink className="size-3" aria-hidden="true" />
        </a>
      ) : null}
    </div>
  );
}
