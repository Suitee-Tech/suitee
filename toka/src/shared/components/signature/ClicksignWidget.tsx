import { useEffect, useId, useRef, useState } from "react";

const SDK_URL = "https://cdn.clicksign.com/embed/v1/embed.js";

type ClicksignSDK = new (signerKey: string) => ClicksignWidgetInstance;

interface ClicksignWidgetInstance {
  endpoint: string;
  origin: string;
  mount(containerId: string): void;
  unmount?: () => void;
  on(event: "signed" | "close" | "loaded" | "error", handler: (...args: unknown[]) => void): void;
}

declare global {
  interface Window {
    Clicksign?: ClicksignSDK;
  }
}

let sdkPromise: Promise<void> | null = null;

function loadSDK(): Promise<void> {
  if (typeof window === "undefined") return Promise.reject(new Error("window unavailable"));
  if (window.Clicksign) return Promise.resolve();
  if (sdkPromise) return sdkPromise;
  sdkPromise = new Promise<void>((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>(`script[src="${SDK_URL}"]`);
    if (existing) {
      existing.addEventListener("load", () => resolve());
      existing.addEventListener("error", () => reject(new Error("failed to load clicksign sdk")));
      return;
    }
    const script = document.createElement("script");
    script.src = SDK_URL;
    script.async = true;
    script.onload = () => resolve();
    script.onerror = () => {
      sdkPromise = null;
      reject(new Error("failed to load clicksign sdk"));
    };
    document.body.appendChild(script);
  });
  return sdkPromise;
}

export type ClicksignWidgetProps = {
  signerKey: string;
  environment?: "sandbox" | "production";
  onSigned?: () => void;
  onClose?: () => void;
  className?: string;
};

const ENDPOINTS = {
  sandbox: "https://sandbox.clicksign.com",
  production: "https://app.clicksign.com",
};

export function ClicksignWidget({ signerKey, environment = "sandbox", onSigned, onClose, className }: ClicksignWidgetProps) {
  const reactId = useId();
  const containerId = `cs-widget-${reactId.replace(/[:]/g, "-")}`;
  const containerRef = useRef<HTMLDivElement>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!signerKey) {
      setError("signer key não disponível");
      setLoading(false);
      return;
    }

    let widget: ClicksignWidgetInstance | undefined;
    let cancelled = false;

    loadSDK()
      .then(() => {
        if (cancelled) return;
        const SDK = window.Clicksign;
        if (!SDK) {
          setError("Clicksign SDK indisponível");
          setLoading(false);
          return;
        }
        try {
          widget = new SDK(signerKey);
          widget.endpoint = ENDPOINTS[environment];
          widget.origin = window.location.origin;
          widget.on("signed", () => onSigned?.());
          widget.on("close", () => onClose?.());
          widget.on("loaded", () => setLoading(false));
          widget.on("error", (...args) => {
            setError(`Erro do widget Clicksign: ${args.map(String).join(" ")}`);
            setLoading(false);
          });
          widget.mount(containerId);
        } catch (e) {
          setError(e instanceof Error ? e.message : "falha ao montar widget");
          setLoading(false);
        }
      })
      .catch((e: Error) => {
        if (!cancelled) {
          setError(e.message);
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
      try {
        widget?.unmount?.();
      } catch {
      }
    };
  }, [signerKey, environment, containerId]);

  return (
    <div className={className ?? "w-full"}>
      {error && <div className="rounded border border-red-300 bg-red-50 p-3 text-sm text-red-700">{error}</div>}
      {loading && !error && <div className="text-sm text-muted-foreground">Carregando assinatura…</div>}
      <div id={containerId} ref={containerRef} className="min-h-[600px] w-full" />
    </div>
  );
}

