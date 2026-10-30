import { useEffect, useMemo, useState } from "react";
import { MapPinOff } from "lucide-react";
import type { Property } from "../data";

type LeafletModule = typeof import("leaflet");
type ReactLeafletModule = typeof import("react-leaflet");

export function getPropertyCoords(property: Property): [number, number] | null {
  const lat = parseFloat(String(property.address?.latitude ?? ""));
  const lng = parseFloat(String(property.address?.longitude ?? ""));
  if (!Number.isFinite(lat) || !Number.isFinite(lng)) return null;
  if (lat === 0 && lng === 0) return null;
  return [lat, lng];
}

const BRL = new Intl.NumberFormat("pt-BR", {
  style: "currency",
  currency: "BRL",
  maximumFractionDigits: 0,
});

const BRAZIL_CENTER: [number, number] = [-15.78, -47.93];

interface PropertyMapProps {
  properties: Property[];
  height?: string | number;
  className?: string;
  onSelectProperty?: (property: Property) => void;
}

export function PropertyMap({
  properties,
  height = 480,
  className,
  onSelectProperty,
}: PropertyMapProps) {
  const [deps, setDeps] = useState<{
    L: LeafletModule;
    rl: ReactLeafletModule;
  } | null>(null);
  const [depsFailed, setDepsFailed] = useState(false);

  useEffect(() => {
    if (deps || depsFailed) return;
    let cancelled = false;
    (async () => {
      try {
        await import("leaflet/dist/leaflet.css");
        const [leaflet, rl] = await Promise.all([
          import("leaflet"),
          import("react-leaflet"),
        ]);
        const L = (
          (leaflet as unknown as { default?: LeafletModule }).default ?? leaflet
        ) as LeafletModule;
        if (!cancelled) setDeps({ L, rl });
      } catch {
        if (!cancelled) setDepsFailed(true);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [deps, depsFailed]);

  const located = useMemo(
    () =>
      properties
        .map((p) => ({ property: p, coords: getPropertyCoords(p) }))
        .filter(
          (x): x is { property: Property; coords: [number, number] } =>
            x.coords !== null,
        ),
    [properties],
  );

  const missingCount = properties.length - located.length;

  const center = located[0]?.coords ?? BRAZIL_CENTER;
  const zoom = located.length > 0 ? (located.length === 1 ? 14 : 11) : 4;

  const markerIcon = useMemo(() => {
    if (!deps) return null;
    return deps.L.divIcon({
      className: "",
      html: `<div style="
      width:28px;height:28px;transform:translate(-50%,-100%);
      display:flex;align-items:center;justify-content:center;
    ">
      <svg viewBox="0 0 24 24" width="28" height="28" fill="#0d6efd" stroke="#fff" stroke-width="1.5">
        <path d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7z"/>
        <circle cx="12" cy="9" r="2.5" fill="#fff" stroke="none"/>
      </svg>
    </div>`,
      iconSize: [28, 28],
      iconAnchor: [0, 0],
      popupAnchor: [0, -28],
    });
  }, [deps]);

  return (
    <div className={className}>
      <div
        className="rounded-2xl overflow-hidden border border-border"
        style={{ height: typeof height === "number" ? `${height}px` : height }}
      >
        {depsFailed ? (
          <div className="h-full w-full bg-muted flex items-center justify-center">
            <p className="text-sm font-medium text-muted-foreground">
              Não foi possível carregar o mapa.
            </p>
          </div>
        ) : !deps || !markerIcon ? (
          <div className="h-full w-full bg-muted animate-pulse" />
        ) : (
          <deps.rl.MapContainer
            center={center}
            zoom={zoom}
            scrollWheelZoom
            style={{ height: "100%", width: "100%" }}
          >
            <deps.rl.TileLayer
              attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>'
              url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
            />
            {located.map(({ property, coords }) => (
              <deps.rl.Marker
                key={property.id}
                position={coords}
                icon={markerIcon}
                eventHandlers={
                  onSelectProperty
                    ? { click: () => onSelectProperty(property) }
                    : undefined
                }
              >
                <deps.rl.Popup>
                  <div className="min-w-[160px]">
                    <p className="font-bold text-sm text-foreground">
                      {property.title}
                    </p>
                    <p className="text-xs text-muted-foreground mt-0.5">
                      {property.address.neighborhood}
                      {property.address.city ? `, ${property.address.city}` : ""}
                    </p>
                    <p className="text-sm font-bold text-primary-600 mt-1">
                      {BRL.format(property.price || 0)}
                    </p>
                  </div>
                </deps.rl.Popup>
              </deps.rl.Marker>
            ))}
          </deps.rl.MapContainer>
        )}
      </div>

      {missingCount > 0 && (
        <p className="mt-2 flex items-center gap-1.5 text-xs text-muted-foreground">
          <MapPinOff className="w-3.5 h-3.5 shrink-0" />
          {missingCount}{" "}
          {missingCount === 1 ? "imóvel sem localização" : "imóveis sem localização"}{" "}
          no mapa.
        </p>
      )}
    </div>
  );
}
