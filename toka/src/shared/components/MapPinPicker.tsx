import { useMemo, useState } from "react";
import { MapContainer, TileLayer, Marker, useMapEvents } from "react-leaflet";
import L from "leaflet";
import { MapPin, Search, Loader2 } from "lucide-react";
import "leaflet/dist/leaflet.css";

const pinIcon = L.divIcon({
  className: "",
  html: `<div style="
      width:32px;height:32px;transform:translate(-50%,-100%);
      display:flex;align-items:center;justify-content:center;
    ">
      <svg viewBox="0 0 24 24" width="32" height="32" fill="#0d6efd" stroke="#fff" stroke-width="1.5">
        <path d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7z"/>
        <circle cx="12" cy="9" r="2.5" fill="#fff" stroke="none"/>
      </svg>
    </div>`,
  iconSize: [32, 32],
  iconAnchor: [0, 0],
});

const BRAZIL_CENTER: [number, number] = [-15.78, -47.93];

function parseCoord(value?: string): number | null {
  const n = parseFloat(String(value ?? ""));
  return Number.isFinite(n) && n !== 0 ? n : null;
}

interface ClickHandlerProps {
  onPick: (lat: number, lng: number) => void;
}

function ClickHandler({ onPick }: ClickHandlerProps) {
  useMapEvents({
    click(e) {
      onPick(e.latlng.lat, e.latlng.lng);
    },
  });
  return null;
}

export interface MapPinPickerProps {
  latitude?: string;
  longitude?: string;
  onChange: (coords: { latitude: string; longitude: string }) => void;
  addressQuery?: string;
  height?: number;
}

export function MapPinPicker({
  latitude,
  longitude,
  onChange,
  addressQuery,
  height = 280,
}: MapPinPickerProps) {
  const [geocoding, setGeocoding] = useState(false);
  const [geocodeError, setGeocodeError] = useState<string | null>(null);

  const lat = parseCoord(latitude);
  const lng = parseCoord(longitude);
  const hasPin = lat !== null && lng !== null;
  const position: [number, number] = hasPin ? [lat!, lng!] : BRAZIL_CENTER;

  const mapKey = useMemo(() => (hasPin ? "located" : "empty"), [hasPin]);

  const pick = (la: number, ln: number) => {
    onChange({ latitude: la.toFixed(6), longitude: ln.toFixed(6) });
    setGeocodeError(null);
  };

  const handleGeocode = async () => {
    const query = (addressQuery || "").trim();
    if (!query || geocoding) return;
    setGeocoding(true);
    setGeocodeError(null);
    try {
      const url = `https://nominatim.openstreetmap.org/search?format=json&limit=1&q=${encodeURIComponent(
        query,
      )}`;
      const res = await fetch(url, { headers: { Accept: "application/json" } });
      if (!res.ok) throw new Error("geocode failed");
      const data: Array<{ lat: string; lon: string }> = await res.json();
      if (data.length === 0) {
        setGeocodeError("Endereço não localizado. Posicione o pino manualmente.");
        return;
      }
      pick(parseFloat(data[0].lat), parseFloat(data[0].lon));
    } catch {
      setGeocodeError("Não foi possível localizar o endereço. Posicione o pino manualmente.");
    } finally {
      setGeocoding(false);
    }
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2 flex-wrap">
        <p className="text-xs text-muted-foreground flex items-center gap-1.5">
          <MapPin className="w-3.5 h-3.5 shrink-0" />
          Clique no mapa ou arraste o pino para definir a localização.
        </p>
        {addressQuery && addressQuery.trim() && (
          <button
            type="button"
            onClick={handleGeocode}
            disabled={geocoding}
            className="text-xs flex items-center gap-1 text-primary-600 hover:text-primary-700 font-medium disabled:opacity-60"
          >
            {geocoding ? (
              <Loader2 className="w-3.5 h-3.5 animate-spin" />
            ) : (
              <Search className="w-3.5 h-3.5" />
            )}
            Localizar pelo endereço
          </button>
        )}
      </div>

      <div className="rounded-xl overflow-hidden border border-border" style={{ height: `${height}px` }}>
        <MapContainer
          key={mapKey}
          center={position}
          zoom={hasPin ? 15 : 4}
          scrollWheelZoom
          style={{ height: "100%", width: "100%" }}
        >
          <TileLayer
            attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>'
            url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
          />
          <ClickHandler onPick={pick} />
          {hasPin && (
            <Marker
              position={position}
              icon={pinIcon}
              draggable
              eventHandlers={{
                dragend(e) {
                  const m = e.target as L.Marker;
                  const ll = m.getLatLng();
                  pick(ll.lat, ll.lng);
                },
              }}
            />
          )}
        </MapContainer>
      </div>

      {geocodeError && <p className="text-xs text-status-warning">{geocodeError}</p>}

      {hasPin && (
        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          <span>
            Lat: <span className="font-medium text-foreground">{latitude}</span>
          </span>
          <span>
            Long: <span className="font-medium text-foreground">{longitude}</span>
          </span>
          <button
            type="button"
            onClick={() => onChange({ latitude: "", longitude: "" })}
            className="text-status-critical hover:underline ml-auto"
          >
            Limpar
          </button>
        </div>
      )}
    </div>
  );
}
