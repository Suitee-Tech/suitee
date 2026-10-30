import { useDataChangeRefresh } from "../hooks/useDataChangeRefresh";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  AlertCircle,
  Bath,
  Bed,
  Building2,
  Handshake,
  Loader2,
  MapPin,
  RefreshCw,
  ShieldCheck,
  Square,
} from "lucide-react";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "../ui/sheet";
import { Button } from "../ui/button";
import { EmptyState } from "../ui/EmptyState";
import { PropertyMap } from "./PropertyMap";
import { MediaImage } from "./MediaImage";
import {
  partnershipService,
  grantFrom,
  counterpart,
  type Partnership,
  type PermissionScope,
  type AccessLevel,
} from "../services/partnership.service";
import { mapPropertyDtoToProperty } from "../lib/properties/mappers";
import type { Property } from "../data";
import { formatPropertyAddress } from "../data";

const BRL = new Intl.NumberFormat("pt-BR", {
  style: "currency",
  currency: "BRL",
  maximumFractionDigits: 0,
});

const TYPE_LABELS: Record<string, string> = {
  apartment: "Apartamento",
  house: "Casa",
  land: "Terreno",
  commercial: "Comercial",
};

const SCOPE_LABELS: Record<PermissionScope, string> = {
  shared_portfolio: "Portfólio compartilhado",
  leads: "Leads",
};

const ACCESS_LABELS: Record<AccessLevel, string> = {
  view: "visualizar",
  create: "criar",
  edit: "editar",
  delete: "excluir",
};

interface SharedPortfolioModalProps {
  partnership: Partnership;
  companyId: string;
  partnerName: string;
  onClose: () => void;
}

function RuleRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      <span className="text-xs text-muted-foreground shrink-0">{label}</span>
      <span className="text-xs font-medium text-foreground text-right">
        {value}
      </span>
    </div>
  );
}

export function SharedPortfolioModal({
  partnership,
  companyId,
  partnerName,
  onClose,
}: SharedPortfolioModalProps) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [properties, setProperties] = useState<Property[]>([]);

  const load = useCallback(async () => {
    setLoading(true);
    setProperties([]);
    setError(null);
    try {
      const dtos = await partnershipService.getSharedPortfolio(partnership.id);
      setProperties(
        Array.isArray(dtos) ? dtos.map(mapPropertyDtoToProperty) : [],
      );
    } catch (err: any) {
      setError(
        err?.response?.data?.message ??
          "Não foi possível carregar o portfólio compartilhado.",
      );
    } finally {
      setLoading(false);
    }
  }, [partnership.id]);

  useEffect(() => {
    load();
  }, [load]);

  useDataChangeRefresh(["partnerships", "properties"], load);

  const grant = grantFrom(partnership, counterpart(partnership, companyId));
  const sharing = grant.sharing_config;
  const sharedTypes = sharing?.shared_property_types ?? [];
  const sharedRegions = sharing?.shared_regions ?? [];
  const permissions = grant.permissions;

  const hasProperties = properties.length > 0;
  const located = useMemo(
    () =>
      properties.filter((p) => {
        const lat = parseFloat(String(p.address?.latitude ?? ""));
        const lng = parseFloat(String(p.address?.longitude ?? ""));
        return (
          Number.isFinite(lat) &&
          Number.isFinite(lng) &&
          !(lat === 0 && lng === 0)
        );
      }),
    [properties],
  );

  return (
    <Sheet
      open
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <SheetContent
        side="right"
        className="w-full gap-0 p-0 sm:max-w-2xl flex flex-col"
      >
        <div className="bg-card px-6 py-4 border-b border-border flex items-center gap-3 flex-shrink-0 pr-12">
          <div className="p-2 bg-primary-50 text-primary-700 rounded-lg">
            <Building2 className="w-5 h-5" />
          </div>
          <div className="min-w-0">
            <SheetTitle className="text-lg font-bold text-foreground">
              Portfólio Compartilhado
            </SheetTitle>
            <SheetDescription className="text-sm truncate">
              {partnerName}
            </SheetDescription>
          </div>
        </div>

        <div className="flex-1 overflow-y-auto p-6 space-y-5">
          <div className="rounded-xl border border-border bg-card p-4 space-y-2.5">
            <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider flex items-center gap-1.5">
              <Handshake className="w-3.5 h-3.5" /> Regras do acordo
            </p>
            <RuleRow
              label="Tipos compartilhados"
              value={
                sharedTypes.length
                  ? sharedTypes.map((t) => TYPE_LABELS[t] ?? t).join(", ")
                  : "Todos os tipos"
              }
            />
            <RuleRow
              label="Regiões"
              value={
                sharedRegions.length
                  ? sharedRegions.join(", ")
                  : "Todas as regiões"
              }
            />
            <RuleRow
              label="Contato do proprietário"
              value={
                sharing?.share_owner_contact
                  ? "Visível ao parceiro"
                  : "Oculto por privacidade"
              }
            />
            <RuleRow
              label="Co-corretagem"
              value={
                sharing?.allow_co_brokering ? "Permitida" : "Não permitida"
              }
            />
            <RuleRow
              label="Permissões concedidas"
              value={
                permissions.length
                  ? permissions
                      .map(
                        (p) =>
                          `${SCOPE_LABELS[p.scope] ?? p.scope} (${ACCESS_LABELS[p.access_level] ?? p.access_level})`,
                      )
                      .join(" · ")
                  : "—"
              }
            />
          </div>

          <div className="flex items-start gap-2.5 p-3 rounded-xl bg-primary-50 border border-primary-100">
            <ShieldCheck className="w-4 h-4 text-primary-700 shrink-0 mt-0.5" />
            <p className="text-xs text-primary-900 leading-relaxed">
              {sharing.share_owner_contact
                ? "O parceiro autorizou compartilhar a identificação do proprietário."
                : "A identificação do proprietário está oculta."}{" "}
              A localização é aproximada. Para negociar, entre em contato com a
              imobiliária parceira.
            </p>
          </div>

          {loading ? (
            <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
              <Loader2 className="w-6 h-6 animate-spin mb-3" />
              <p className="text-sm">Carregando portfólio compartilhado…</p>
            </div>
          ) : error ? (
            <EmptyState
              icon={AlertCircle}
              title="Não foi possível carregar"
              message={error}
              className="border-none bg-transparent"
              action={
                <Button onClick={load}>
                  <RefreshCw className="w-4 h-4" /> Tentar novamente
                </Button>
              }
            />
          ) : !hasProperties ? (
            <EmptyState
              icon={Building2}
              title="Nenhum imóvel compartilhado"
              message="Nenhum imóvel deste parceiro atende às regras de compartilhamento do acordo."
              className="border-none bg-transparent"
            />
          ) : (
            <>
              {located.length > 0 && (
                <PropertyMap properties={properties} height={300} />
              )}

              <div>
                <h3 className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-3">
                  {properties.length}{" "}
                  {properties.length === 1
                    ? "imóvel compartilhado"
                    : "imóveis compartilhados"}
                </h3>
                <ul className="space-y-3">
                  {properties.map((property) => (
                    <li
                      key={property.id}
                      className="flex gap-3 p-3 rounded-xl border border-border bg-background/50"
                    >
                      <div className="w-24 h-20 rounded-lg overflow-hidden bg-muted shrink-0">
                        {property.image ? (
                          <MediaImage
                            src={property.image}
                            alt={property.title}
                            className="w-full h-full object-cover"
                          />
                        ) : (
                          <div className="w-full h-full flex items-center justify-center text-muted-foreground/50">
                            <Building2 className="w-6 h-6" />
                          </div>
                        )}
                      </div>
                      <div className="min-w-0 flex-1">
                        <p className="text-sm font-bold text-foreground truncate">
                          {property.title}
                        </p>
                        <p className="text-xs text-muted-foreground flex items-center gap-1 mt-0.5">
                          <MapPin className="w-3 h-3 shrink-0" />
                          <span className="truncate">
                            {formatPropertyAddress(property.address)}
                          </span>
                        </p>
                        <div className="flex items-center gap-3 mt-1.5 text-xs text-muted-foreground">
                          <span className="flex items-center gap-1">
                            <Bed className="w-3.5 h-3.5" /> {property.beds}
                          </span>
                          <span className="flex items-center gap-1">
                            <Bath className="w-3.5 h-3.5" /> {property.baths}
                          </span>
                          <span className="flex items-center gap-1">
                            <Square className="w-3.5 h-3.5" /> {property.area}m²
                          </span>
                        </div>
                      </div>
                      <div className="text-right shrink-0">
                        <p className="text-sm font-bold text-foreground">
                          {BRL.format(property.price || 0)}
                        </p>
                        <span className="inline-block mt-1 px-2 py-0.5 rounded text-caption font-bold uppercase tracking-wider bg-muted text-muted-foreground">
                          {property.type}
                        </span>
                      </div>
                    </li>
                  ))}
                </ul>
              </div>
            </>
          )}
        </div>

        <div className="border-t border-border p-4 flex bg-muted flex-shrink-0">
          <Button variant="outline" onClick={onClose} className="flex-1">
            Fechar
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}
