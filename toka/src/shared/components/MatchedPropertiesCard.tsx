import { useEffect, useState } from "react";
import { Link } from "react-router";
import { Sparkles, BedDouble, Maximize2, MapPin, Loader2, AlertTriangle } from "lucide-react";
import { clsx } from "clsx";
import { describeListingPrice, isRentListing } from "../utils/listingPrice";
import { leadService, type MatchedProperty } from "../services/lead.service";
import { MediaImage } from "./MediaImage";
import { toast } from "sonner";
import { notifyApiError } from "../services/errorToast";

interface MatchedPropertiesCardProps {
  leadId: string;
  compact?: boolean;
  refreshKey?: string;
}

export function MatchedPropertiesCard({ leadId, compact = true, refreshKey }: MatchedPropertiesCardProps) {
  const [data, setData] = useState<MatchedProperty[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<{ kind: "no_preferences" | "server" | "other"; detail?: string } | null>(null);

  const fetchMatches = async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await leadService.getMatchedProperties(leadId, compact ? 5 : 20);
      setData(res);
    } catch (err: any) {
      const status = err?.response?.status;
      if (status === 422) {
        setError({ kind: "no_preferences" });
      } else if (status === 500) {
        setError({ kind: "server", detail: err?.response?.data?.message });
      } else {
        setError({ kind: "other", detail: err?.message });
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void fetchMatches();
  }, [leadId, refreshKey]);

  const [recommendingId, setRecommendingId] = useState<string | null>(null);
  const handleRecommend = async (propertyId: string) => {
    setRecommendingId(propertyId);
    try {
      await leadService.setRecommendedProperty(leadId, propertyId);
      toast.success("Imóvel recomendado para este cliente. Ele verá a sugestão no portal.");
    } catch (err: any) {
      notifyApiError(err, { context: "Recomendar imóvel" });
    } finally {
      setRecommendingId(null);
    }
  };

  return (
    <section className="w-full min-w-0 bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
      <header className="px-3 sm:px-5 py-3 border-b border-border flex items-center justify-between gap-3">
        <div className="flex items-center gap-2.5 min-w-0">
          <Sparkles className="w-4 h-4 text-primary" aria-hidden="true" />
          <h3 className="text-label font-bold text-foreground uppercase tracking-wide truncate">Melhores imóveis para esse cliente</h3>
        </div>
        {!loading && data && data.length > 0 && (
          <span className="text-caption text-muted-foreground tabular-nums">
            {data.length} match{data.length === 1 ? "" : "es"}
          </span>
        )}
      </header>

      <div className="min-w-0 p-3 sm:p-5 space-y-3">
        {loading && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground py-2">
            <Loader2 className="w-4 h-4 animate-spin" />
            Cruzando preferências com o portfólio…
          </div>
        )}

        {!loading && error?.kind === "no_preferences" && (
          <div className="rounded-xl border border-dashed border-border bg-muted/40 p-4 text-sm">
            <div className="flex items-start gap-2 text-foreground">
              <AlertTriangle className="w-4 h-4 mt-0.5 text-status-warning shrink-0" />
              <div>
                <p className="font-bold">Cadastro incompleto</p>
                <p className="text-caption text-muted-foreground mt-0.5">
                  Preencha faixa de preço, localização ou dormitórios em <strong>Editar</strong> para gerar sugestões.
                </p>
              </div>
            </div>
          </div>
        )}

        {!loading && error && error.kind !== "no_preferences" && (
          <div className="rounded-xl border border-status-critical/30 bg-status-critical-soft p-4 text-sm">
            <div className="flex items-start justify-between gap-2">
              <div>
                <p className="font-bold text-status-critical">Erro ao buscar sugestões</p>
                {error.detail && <p className="text-caption text-status-critical/85 mt-0.5">{error.detail}</p>}
              </div>
              <button
                onClick={fetchMatches}
                className="text-caption font-bold text-status-critical underline-offset-2 hover:underline shrink-0"
              >
                Tentar de novo
              </button>
            </div>
          </div>
        )}

        {!loading && !error && data && data.length === 0 && (
          <div className="rounded-xl border border-dashed border-border bg-muted/40 p-4 text-sm text-center text-muted-foreground">
            Nenhum imóvel disponível bate com as preferências desse cliente hoje.
          </div>
        )}

        {!loading && !error && data && data.length > 0 && (
          <ul className="min-w-0 space-y-2">
            {data.map((m) => (
              <MatchCardItem key={m.property.id} match={m} onRecommend={handleRecommend} recommending={recommendingId === m.property.id} />
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}

function MatchCardItem({ match, onRecommend, recommending }: { match: MatchedProperty; onRecommend: (id: string) => void; recommending: boolean }) {
  const p = match.property;
  // Finalidade do imóvel define o valor E o rótulo (o card do operador mostra os
  // mesmos imóveis que o cliente vê no portal — os dois têm de dizer a mesma
  // coisa).
  const valor = describeListingPrice(isRentListing(p.listing_type) ? p.rent_price : p.sale_price, p.listing_type);
  const scoreColor =
    match.score >= 80
      ? "bg-status-ok text-white"
      : match.score >= 60
        ? "bg-status-warning text-white"
        : "bg-status-critical/60 text-white";

  const breakdown = `Negócio ${(match.reasons.business ?? 0).toFixed(0)}/${(match.reasons.used_weights.business ?? 0).toFixed(0)} • Preço ${match.reasons.price.toFixed(0)}/${match.reasons.used_weights.price.toFixed(0)} • Loc ${match.reasons.location.toFixed(0)}/${match.reasons.used_weights.location.toFixed(0)} • Área ${match.reasons.area.toFixed(0)}/${match.reasons.used_weights.area.toFixed(0)} • Quartos ${match.reasons.bedrooms.toFixed(0)}/${match.reasons.used_weights.bedrooms.toFixed(0)}`;
  const quality = match.score >= 80 ? "Excelente combinação" : match.score >= 60 ? "Boa combinação" : "Combinação possível";
  const reason = strongestMatchReason(match);

  return (
    <li>
      <Link
        to={`/imoveis/${p.id}`}
        className="flex min-w-0 gap-3 sm:gap-4 p-3 sm:p-4 rounded-2xl border border-border hover:border-primary/40 hover:bg-muted/40 transition-colors group overflow-hidden"
        title={breakdown}
      >
        {p.thumbnail_url ? (
          <MediaImage
            src={p.thumbnail_url}
            alt={p.title}
            className="w-24 max-w-[30%] aspect-[4/3] rounded-xl object-cover border border-border shrink-0"
            loading="lazy"
          />
        ) : (
          <div className="w-24 max-w-[30%] aspect-[4/3] rounded-xl bg-muted border border-border shrink-0 flex items-center justify-center text-muted-foreground">
            <Sparkles className="w-5 h-5" />
          </div>
        )}

        <div className="flex-1 min-w-0">
          <div className="flex min-w-0 items-start gap-2">
            <h4 className="min-w-0 flex-1 text-sm sm:text-base font-bold text-foreground line-clamp-2 group-hover:text-primary transition-colors">{p.title}</h4>
            <span
              className={clsx("shrink-0 text-micro sm:text-caption font-bold px-1.5 sm:px-2 py-1 rounded-lg tabular-nums whitespace-nowrap", scoreColor)}
              aria-label={`Compatibilidade ${match.score.toFixed(0)}%`}
            >
              {match.score.toFixed(0)}%
            </span>
          </div>
          <p className="min-w-0 text-caption sm:text-sm text-muted-foreground flex items-center gap-1 mt-1 truncate">
            <MapPin className="w-3 h-3" aria-hidden="true" />
            <span className="truncate">{[p.neighborhood, p.city].filter(Boolean).join(", ") || "Endereço não informado"}</span>
          </p>
          <p className="min-w-0 text-micro text-status-ok font-semibold mt-1 truncate" title={`${quality}: ${reason}`}>
            {quality} · {reason}
          </p>
          <div className="flex min-w-0 flex-wrap items-end justify-between mt-2 gap-x-3 gap-y-1">
            <div className="min-w-0">
              <span className="text-sm sm:text-base font-bold text-foreground tabular-nums block whitespace-nowrap">
                {valor.label && (
                  <span className="mr-1 text-caption font-bold uppercase tracking-wide text-muted-foreground">{valor.label}</span>
                )}
                {valor.amount}
                {valor.period && <span className="text-caption font-normal text-muted-foreground">{valor.period}</span>}
              </span>
              <span className="text-micro text-muted-foreground flex items-center gap-2 mt-1">
                {typeof p.bedrooms === "number" && p.bedrooms > 0 && (
                  <span className="inline-flex items-center gap-0.5">
                    <BedDouble className="w-3 h-3" /> {p.bedrooms}
                  </span>
                )}
                {typeof p.area_total === "number" && p.area_total > 0 && (
                  <span className="inline-flex items-center gap-0.5">
                    <Maximize2 className="w-3 h-3" /> {p.area_total}m²
                  </span>
                )}
              </span>
            </div>
          </div>
        </div>
      </Link>
      <div className="flex items-center justify-end gap-2 px-3 pb-3 -mt-1">
        <button
          type="button"
          onClick={(e) => { e.preventDefault(); onRecommend(p.id); }}
          disabled={recommending}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-caption font-semibold text-primary border border-primary/30 hover:bg-primary/5 transition-colors disabled:opacity-60"
        >
          {recommending ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Sparkles className="w-3.5 h-3.5" />}
          {recommending ? "Recomendando…" : "Recomendar para este cliente"}
        </button>
      </div>
    </li>
  );
}

function strongestMatchReason(match: MatchedProperty) {
  const reasons = [
    { value: match.reasons.business ?? 0, weight: match.reasons.used_weights.business ?? 0, label: "tipo de negócio" },
    { value: match.reasons.price, weight: match.reasons.used_weights.price, label: "orçamento" },
    { value: match.reasons.location, weight: match.reasons.used_weights.location, label: "região" },
    { value: match.reasons.bedrooms, weight: match.reasons.used_weights.bedrooms, label: "quartos" },
    { value: match.reasons.area, weight: match.reasons.used_weights.area, label: "área" },
  ]
    .filter((item) => item.weight > 0)
    .sort((a, b) => b.value / b.weight - a.value / a.weight);
  return reasons[0] ? `Combina com seu ${reasons[0].label}` : "Escolhido pelo seu interesse";
}
