import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ChevronLeft,
  ChevronRight,
  Download,
  ExternalLink,
  Keyboard,
  Maximize,
  RotateCcw,
  RotateCw,
  ZoomIn,
  ZoomOut,
} from "lucide-react";
import { Button } from "../ui/button";
import { cn } from "../ui/utils";
import { fetchPrivateContent, getGatewayOrigin, toGatewayUrl } from "../services/api";
import { ExpandableSidePanel } from "./ExpandableSidePanel";
import {
  DocumentErrorState,
  DocumentUnsupportedState,
  ImageDocumentSkeleton,
  PdfPageSkeleton,
} from "./DocumentViewerStates";

type ReactPdfModule = typeof import("react-pdf");

let reactPdfWorkerConfigured = false;

function configurePdfJsWorker(m: ReactPdfModule) {
  if (!reactPdfWorkerConfigured) {
    m.pdfjs.GlobalWorkerOptions.workerSrc = new URL("pdfjs-dist/build/pdf.worker.min.mjs", import.meta.url).toString();
    reactPdfWorkerConfigured = true;
  }
}

function useReactPdf(enabled: boolean) {
  const [mod, setMod] = useState<ReactPdfModule | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    if (!enabled || mod || failed) return;
    let cancelled = false;
    import("react-pdf")
      .then((m) => {
        configurePdfJsWorker(m);
        if (!cancelled) setMod(m);
      })
      .catch(() => {
        if (!cancelled) setFailed(true);
      });
    return () => {
      cancelled = true;
    };
  }, [enabled, mod, failed]);

  return { mod, failed };
}

function useSuperDocEditor(enabled: boolean) {
  const [superDoc, setSuperDoc] = useState<{ Editor: React.ElementType } | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    const Editor = superDoc?.Editor;
    if (!enabled || Editor || failed) return;
    let cancelled = false;
    (async () => {
      try {
        await import("@superdoc-dev/react/style.css");
        const m = await import("@superdoc-dev/react");
        if (!cancelled) setSuperDoc({ Editor: m.SuperDocEditor as unknown as React.ElementType });
      } catch {
        if (!cancelled) setFailed(true);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [enabled, superDoc, failed]);

  return { Editor: superDoc?.Editor ?? null, failed };
}

type FileKind = "pdf" | "image" | "docx" | "unknown";

function inferFileKind(url: string, contentType?: string | null): FileKind {
  const ct = (contentType || "").toLowerCase();
  if (ct.includes("pdf")) return "pdf";
  if (ct.startsWith("image/")) return "image";
  if (ct.includes("wordprocessingml") || ct === "application/msword" || ct.includes("officedocument")) {
    return "docx";
  }

  const clean = url.split("#")[0]?.split("?")[0] || url;
  const ext = clean.split(".").pop()?.toLowerCase();

  if (ext === "pdf") return "pdf";
  if (ext === "docx" || ext === "doc") return "docx";
  if (ext && ["png", "jpg", "jpeg", "webp", "gif", "bmp", "tiff", "svg"].includes(ext)) return "image";
  return "unknown";
}

function clamp(n: number, min: number, max: number) {
  return Math.max(min, Math.min(max, n));
}

function inferFromFilename(filename: string): FileKind {
  const clean = filename.split("#")[0]?.split("?")[0] || filename;
  const ext = clean.split(".").pop()?.toLowerCase();
  if (ext === "pdf") return "pdf";
  if (ext === "docx" || ext === "doc") return "docx";
  if (ext && ["png", "jpg", "jpeg", "webp", "gif", "bmp", "tiff", "svg"].includes(ext)) return "image";
  return "unknown";
}

function parseContentDispositionFilename(headerValue: string | null): string | null {
  if (!headerValue) return null;
  const mStar = headerValue.match(/filename\*\s*=\s*UTF-8''([^;]+)/i);
  if (mStar?.[1]) {
    try {
      return decodeURIComponent(mStar[1].trim().replace(/^"|"$/g, ""));
    } catch {
      return mStar[1].trim().replace(/^"|"$/g, "");
    }
  }
  const m = headerValue.match(/filename\s*=\s*("?)([^";]+)\1/i);
  if (m?.[2]) return m[2].trim();
  return null;
}

function inferFromMagicBytes(bytes: Uint8Array): FileKind {
  if (bytes.length >= 5) {
    if (bytes[0] === 0x25 && bytes[1] === 0x50 && bytes[2] === 0x44 && bytes[3] === 0x46 && bytes[4] === 0x2d) return "pdf";
  }
  if (bytes.length >= 3) {
    if (bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff) return "image";
  }
  if (bytes.length >= 8) {
    if (
      bytes[0] === 0x89 &&
      bytes[1] === 0x50 &&
      bytes[2] === 0x4e &&
      bytes[3] === 0x47 &&
      bytes[4] === 0x0d &&
      bytes[5] === 0x0a &&
      bytes[6] === 0x1a &&
      bytes[7] === 0x0a
    ) {
      return "image";
    }
  }
  if (bytes.length >= 6) {
    if (
      bytes[0] === 0x47 &&
      bytes[1] === 0x49 &&
      bytes[2] === 0x46 &&
      bytes[3] === 0x38 &&
      (bytes[4] === 0x37 || bytes[4] === 0x39) &&
      bytes[5] === 0x61
    ) {
      return "image";
    }
  }
  if (bytes.length >= 12) {
    if (
      bytes[0] === 0x52 &&
      bytes[1] === 0x49 &&
      bytes[2] === 0x46 &&
      bytes[3] === 0x46 &&
      bytes[8] === 0x57 &&
      bytes[9] === 0x45 &&
      bytes[10] === 0x42 &&
      bytes[11] === 0x50
    ) {
      return "image";
    }
  }
  return "unknown";
}

// isPrivateApiUrl reconhece conteúdo servido por rota autenticada da API (não
// Storage público/assinado). Só esse tipo precisa de busca autenticada: as URLs
// do Storage já são portáveis pelo navegador.
function isPrivateApiUrl(url: string): boolean {
  if (!url) return false;
  try {
    const parsed = new URL(url, getGatewayOrigin());
    if (parsed.origin !== getGatewayOrigin()) return false;
    if (parsed.pathname.startsWith("/storage/v1/")) return false;
    return parsed.pathname.startsWith("/api/");
  } catch {
    return false;
  }
}

type PrivateContent = {
  url: string;
  contentType: string | null;
  loading: boolean;
  error: boolean;
  retry: () => void;
};

/**
 * Busca autenticada do conteúdo. O padrão usa o cliente do painel (operadores);
 * o portal do cliente injeta o próprio fetcher — o token do portal não vale no
 * cliente de operação, e era isso que fazia as imagens que o cliente enviou
 * pelo portal falharem ao abrir ("o arquivo não pôde ser baixado desta rota
 * autenticada").
 */
export type DocumentContentFetcher = (
  url: string,
) => Promise<{ blob: Blob; contentType: string | null }>;

// usePrivateContent entrega ao visualizador um blob URL quando o documento é
// privado da API.
//
// O sintoma que isto corrige: comprovante financeiro e arquivo da esteira
// digital aparecem no painel de documentos, mas ao abrir o PDF o react-pdf busca
// `/api/v1/proofs/content/...` sem o header Authorization e recebe 401 — o
// viewer mostra "PDF load error" e nada chega ao servidor como requisição
// autorizada. Buscar pelo cliente HTTP do app resolve para qualquer rota
// autenticada, sem precisar de token na URL (que vazaria em log/referrer).
function usePrivateContent(
  url: string,
  open: boolean,
  fetchContent: DocumentContentFetcher,
): PrivateContent {
  const target = useMemo(() => isPrivateApiUrl(url), [url]);
  const [state, setState] = useState<{ url: string; contentType: string | null; loading: boolean; error: boolean }>({
    url,
    contentType: null,
    loading: false,
    error: false,
  });
  const [nonce, setNonce] = useState(0);

  useEffect(() => {
    if (!open || !url) {
      setState({ url, contentType: null, loading: false, error: false });
      return;
    }
    if (!target) {
      setState({ url, contentType: null, loading: false, error: false });
      return;
    }
    let cancelled = false;
    let objectUrl: string | null = null;
    setState({ url: "", contentType: null, loading: true, error: false });
    fetchContent(url)
      .then(({ blob, contentType }) => {
        if (cancelled) return;
        objectUrl = URL.createObjectURL(blob);
        setState({ url: objectUrl, contentType, loading: false, error: false });
      })
      .catch(() => {
        if (!cancelled) setState({ url: "", contentType: null, loading: false, error: true });
      });
    return () => {
      cancelled = true;
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [url, open, target, nonce, fetchContent]);

  const retry = useCallback(() => setNonce((n) => n + 1), []);
  return { ...state, retry };
}

export type SuperDocUser = { name: string; email: string };

export interface DocumentViewerDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  url: string;
  title?: string;
  contentType?: string | null;
  superDocUser?: SuperDocUser;
  /** Cliente autenticado do conteúdo (padrão: painel do operador). */
  fetchContent?: DocumentContentFetcher;
}

export function DocumentViewerDialog({
  open,
  onOpenChange,
  url,
  title,
  contentType,
  superDocUser,
  fetchContent = fetchPrivateContent,
}: DocumentViewerDialogProps) {
  const effectiveUrl = useMemo(() => toGatewayUrl(url), [url]);
  const privateContent = usePrivateContent(effectiveUrl, open, fetchContent);
  // URLs privadas da API só são renderizáveis pelo blob autenticado; as demais
  // seguem direto para o visualizador.
  const displayUrl = privateContent.url || (privateContent.loading || privateContent.error ? "" : effectiveUrl);
  const contentTypeForKind = privateContent.contentType ?? contentType;

  const [resolvedContentType, setResolvedContentType] = useState<string | null | undefined>(contentType);
  const initialKind = effectiveUrl ? inferFileKind(effectiveUrl, contentTypeForKind) : "unknown";
  const [kind, setKind] = useState<FileKind>(initialKind);
  const [detecting, setDetecting] = useState<boolean>(initialKind === "unknown");
  const [detectionError, setDetectionError] = useState(false);
  const [zoom, setZoom] = useState(1);
  const [rotate, setRotate] = useState(0);

  const [numPages, setNumPages] = useState<number | null>(null);
  const [page, setPage] = useState(1);
  const pdfPageRefs = useRef<Array<HTMLDivElement | null>>([]);
  const pdfScrollerRef = useRef<HTMLDivElement | null>(null);

  const [imgOffset, setImgOffset] = useState({ x: 0, y: 0 });
  const dragRef = useRef<{ active: boolean; startX: number; startY: number; baseX: number; baseY: number } | null>(null);

  const [pdfErrored, setPdfErrored] = useState(false);
  const [pdfErrorDetail, setPdfErrorDetail] = useState<string | null>(null);
  const [imgErrored, setImgErrored] = useState(false);
  const [imgLoaded, setImgLoaded] = useState(false);
  const [pdfLoaded, setPdfLoaded] = useState(false);
  const [retryNonce, setRetryNonce] = useState(0);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);

  const { mod: reactPdf, failed: reactPdfFailed } = useReactPdf(open && kind === "pdf");
  const PdfDocument = reactPdf?.Document;
  const PdfPage = reactPdf?.Page;

  const pdfFile = displayUrl;

  useEffect(() => {
    if (!open) {
      setZoom(1);
      setRotate(0);
      setNumPages(null);
      setPage(1);
      setImgOffset({ x: 0, y: 0 });
      dragRef.current = null;
      setResolvedContentType(contentTypeForKind);
      const reset = effectiveUrl ? inferFileKind(effectiveUrl, contentTypeForKind) : "unknown";
      setKind(reset);
      setDetecting(Boolean(effectiveUrl) && reset === "unknown");
      setDetectionError(false);
      setPdfErrored(false);
      setPdfErrorDetail(null);
      setImgErrored(false);
      setImgLoaded(false);
      setPdfLoaded(false);
      setShortcutsOpen(false);
    }
  }, [open, contentTypeForKind, effectiveUrl]);

  useEffect(() => {
    if (!open) return;
    setResolvedContentType(contentTypeForKind);
    const next = effectiveUrl ? inferFileKind(effectiveUrl, contentTypeForKind) : "unknown";
    setKind(next);
    setDetecting(Boolean(effectiveUrl) && next === "unknown");
    setDetectionError(false);
  }, [open, effectiveUrl, contentTypeForKind]);

  // Conteúdo privado chegou: o content-type do blob decide o formato sem a
  // detecção heurística (que buscaria a URL sem autenticação).
  useEffect(() => {
    if (!privateContent.contentType) return;
    setResolvedContentType(privateContent.contentType);
    const fromCt = inferFileKind(displayUrl, privateContent.contentType);
    if (fromCt !== "unknown") {
      setKind(fromCt);
      setDetecting(false);
      setDetectionError(false);
    }
  }, [privateContent.contentType, displayUrl]);

  useEffect(() => {
    if (!open) return;
    if (kind !== "unknown") return;
    if (!effectiveUrl) {
      setDetecting(false);
      return;
    }
    // Arquivo privado: os bytes já vieram pelo cliente autenticado (e o blob URL
    // não responde a fetch com headers da API).
    if (privateContent.contentType || privateContent.loading || privateContent.error) {
      setDetecting(privateContent.loading);
      return;
    }

    const controller = new AbortController();
    let cancelled = false;
    setDetecting(true);
    setDetectionError(false);

    // A detecção buscava o arquivo sem a autenticação que a API usa: sem
    // `X-Tenant-Id` (e sem cookie de sessão) o backend responde 403/404 para uma
    // URL autenticada, a detecção falha e o comprovante aparece como "formato
    // não identificado" — mesmo existindo. O mesmo vale para o cookie, já que o
    // cliente HTTP do app usa `withCredentials`.
    //
    // Só para a origem da API: enviar cookie e identificador de tenant a um
    // domínio de terceiro seria vazamento, e o provedor externo não os usa.
    //
    // A comparação é contra a origem da API, não a da página: em desenvolvimento
    // elas diferem por porta (página em :3000, API em :8080), e comparar com a
    // página classificaria a própria API como terceiro — deixando a detecção sem
    // autenticação, que é o defeito original.
    const targetsApi = (() => {
      try {
        return new URL(effectiveUrl, getGatewayOrigin()).origin === getGatewayOrigin();
      } catch {
        return false;
      }
    })();

    const tenantId = (() => {
      try {
        return localStorage.getItem("current_company_id");
      } catch {
        return null;
      }
    })();

    const tryDetect = async (fetchInit: RequestInit) => {
      try {
        const res = await fetch(effectiveUrl, {
          ...fetchInit,
          signal: controller.signal,
          credentials: targetsApi ? "include" : "omit",
          headers: {
            ...(fetchInit.headers ?? {}),
            ...(targetsApi && tenantId ? { "X-Tenant-Id": tenantId } : {}),
          },
        });
        if (!res.ok) return false;

        const ct = res.headers.get("content-type");
        const cd = res.headers.get("content-disposition");
        const filename = parseContentDispositionFilename(cd);

        if (ct) setResolvedContentType(ct);
        const fromCt = inferFileKind(effectiveUrl, ct);
        if (fromCt !== "unknown") {
          setKind(fromCt);
          return true;
        }

        if (filename) {
          const fromName = inferFromFilename(filename);
          if (fromName !== "unknown") {
            setKind(fromName);
            return true;
          }
        }

        const buf = await res.arrayBuffer();
        const bytes = new Uint8Array(buf.slice(0, 64));
        const fromMagic = inferFromMagicBytes(bytes);
        if (fromMagic !== "unknown") {
          setKind(fromMagic);
          return true;
        }
        return false;
      } catch {
        return false;
      }
    };

    (async () => {
      // Tenta primeiro com Range header (mais eficiente)
      const rangeSuccess = await tryDetect({ method: "GET", headers: { Range: "bytes=0-1023" } });
      if (cancelled || rangeSuccess) {
        if (!cancelled) setDetecting(false);
        return;
      }

      // Fallback: sem Range header (evita bloqueio de CORS)
      console.warn("[DocumentViewer] Range request failed, retrying without Range header for type detection");
      const noRangeSuccess = await tryDetect({ method: "GET" });
      if (!cancelled) {
        setDetecting(false);
        if (!noRangeSuccess) {
          setDetectionError(true);
        }
      }
    })();

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [open, kind, effectiveUrl, privateContent.contentType, privateContent.loading, privateContent.error]);

  const zoomIn = useCallback(() => setZoom((z) => clamp(Number((z * 1.15).toFixed(4)), 0.25, 6)), []);
  const zoomOut = useCallback(() => setZoom((z) => clamp(Number((z / 1.15).toFixed(4)), 0.25, 6)), []);
  const resetView = useCallback(() => {
    setZoom(1);
    setRotate(0);
    setImgOffset({ x: 0, y: 0 });
  }, []);
  const rotateLeft = useCallback(() => setRotate((r) => (r - 90 + 360) % 360), []);
  const rotateRight = useCallback(() => setRotate((r) => (r + 90) % 360), []);

  const canPrev = page > 1;
  const canNext = numPages ? page < numPages : false;

  const scrollToPage = useCallback((target: number) => {
    if (target < 1) return;
    const el = pdfPageRefs.current[target - 1];
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, []);

  const goPrev = useCallback(() => {
    setPage((p) => {
      const next = Math.max(1, p - 1);
      scrollToPage(next);
      return next;
    });
  }, [scrollToPage]);
  const goNext = useCallback(() => {
    setPage((p) => {
      const next = numPages ? Math.min(numPages, p + 1) : p + 1;
      scrollToPage(next);
      return next;
    });
  }, [numPages, scrollToPage]);

  const retry = useCallback(() => {
    setPdfErrored(false);
    setPdfErrorDetail(null);
    setImgErrored(false);
    setImgLoaded(false);
    setPdfLoaded(false);
    setDetectionError(false);
    setDetecting(false);
    privateContent.retry();
    setRetryNonce((n) => n + 1);
  }, [privateContent]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null;
      if (target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable)) return;
      switch (e.key) {
        case "ArrowLeft":
        case "PageUp":
          if (kind === "pdf" && canPrev) {
            e.preventDefault();
            goPrev();
          }
          break;
        case "ArrowRight":
        case "PageDown":
          if (kind === "pdf" && canNext) {
            e.preventDefault();
            goNext();
          }
          break;
        case "+":
        case "=":
          e.preventDefault();
          zoomIn();
          break;
        case "-":
        case "_":
          e.preventDefault();
          zoomOut();
          break;
        case "0":
          e.preventDefault();
          resetView();
          break;
        case "?":
        case "h":
        case "H":
          setShortcutsOpen((s) => !s);
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, kind, canPrev, canNext, goPrev, goNext, zoomIn, zoomOut, resetView]);

  const handleWheelZoom = useCallback(
    (e: React.WheelEvent) => {
      if (!e.ctrlKey && !e.metaKey) return;
      e.preventDefault();
      const direction = e.deltaY > 0 ? "out" : "in";
      if (direction === "in") zoomIn();
      else zoomOut();
    },
    [zoomIn, zoomOut],
  );

  useEffect(() => {
    if (!open || kind !== "pdf" || !pdfLoaded || !numPages) return;
    const root = pdfScrollerRef.current;
    if (!root) return;

    const inBand = new Set<number>();
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const idxAttr = entry.target.getAttribute("data-page-index");
          if (idxAttr === null) continue;
          const idx = Number(idxAttr);
          if (Number.isNaN(idx)) continue;
          if (entry.isIntersecting) {
            inBand.add(idx);
          } else {
            inBand.delete(idx);
          }
        }
        if (inBand.size === 0) return;
        let smallest = Number.POSITIVE_INFINITY;
        for (const i of inBand) {
          if (i < smallest) smallest = i;
        }
        if (smallest !== Number.POSITIVE_INFINITY) {
          setPage(smallest + 1);
        }
      },
      {
        root,
        rootMargin: "-45% 0px -45% 0px",
        threshold: 0,
      },
    );

    for (const el of pdfPageRefs.current) {
      if (el) observer.observe(el);
    }
    return () => observer.disconnect();
  }, [open, kind, pdfLoaded, numPages]);

  const handleImgPointerDown = useCallback(
    (e: React.PointerEvent) => {
      if (kind !== "image") return;
      (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
      dragRef.current = { active: true, startX: e.clientX, startY: e.clientY, baseX: imgOffset.x, baseY: imgOffset.y };
    },
    [kind, imgOffset.x, imgOffset.y],
  );

  const handleImgPointerMove = useCallback((e: React.PointerEvent) => {
    const st = dragRef.current;
    if (!st?.active) return;
    const dx = e.clientX - st.startX;
    const dy = e.clientY - st.startY;
    setImgOffset({ x: st.baseX + dx, y: st.baseY + dy });
  }, []);

  const handleImgPointerUp = useCallback((e: React.PointerEvent) => {
    const st = dragRef.current;
    if (!st?.active) return;
    (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
    dragRef.current = null;
  }, []);

  return (
    <ExpandableSidePanel
      open={open}
      onClose={() => onOpenChange(false)}
      defaultSize="3xl"
      expandedSize="full"
      renderHeader={({ expandButton, closeButton }) => (
        <div className="px-6 pt-5 pb-4 border-b border-border bg-card">
          <div className="flex items-center gap-3">
            <div className="flex-1 min-w-0">
              <p className="text-base font-bold text-foreground truncate">{title || "Documento"}</p>
              <p className="text-xs text-muted-foreground truncate">{resolvedContentType || "—"}</p>
            </div>
            <div className="flex items-center gap-1.5">
              {expandButton}
              {closeButton}
            </div>
          </div>

          <div className="mt-4 flex flex-wrap items-center gap-2">
            {kind === "pdf" && (
              <ToolbarGroup label="Navegação">
                <Button variant="ghost" size="icon" onClick={goPrev} disabled={!canPrev} aria-label="Página anterior" title="Página anterior (←)">
                  <ChevronLeft />
                </Button>
                <span className="text-xs font-medium tabular-nums text-muted-foreground px-2 select-none">
                  {numPages ? `${page} / ${numPages}` : `${page}`}
                </span>
                <Button variant="ghost" size="icon" onClick={goNext} disabled={!canNext} aria-label="Próxima página" title="Próxima página (→)">
                  <ChevronRight />
                </Button>
              </ToolbarGroup>
            )}

            {kind !== "docx" && (
              <ToolbarGroup label="Visualização">
                <Button variant="ghost" size="icon" onClick={zoomOut} aria-label="Diminuir zoom" title="Diminuir zoom (-)">
                  <ZoomOut />
                </Button>
                <span className="text-xs font-medium tabular-nums text-muted-foreground w-12 text-center select-none">
                  {Math.round(zoom * 100)}%
                </span>
                <Button variant="ghost" size="icon" onClick={zoomIn} aria-label="Aumentar zoom" title="Aumentar zoom (+)">
                  <ZoomIn />
                </Button>
                <span className="w-px h-5 bg-border mx-0.5" aria-hidden="true" />
                <Button variant="ghost" size="icon" onClick={rotateLeft} aria-label="Girar à esquerda" title="Girar à esquerda">
                  <RotateCcw />
                </Button>
                <Button variant="ghost" size="icon" onClick={rotateRight} aria-label="Girar à direita" title="Girar à direita">
                  <RotateCw />
                </Button>
                <Button variant="ghost" size="icon" onClick={resetView} aria-label="Restaurar visualização" title="Restaurar visualização (0)">
                  <Maximize />
                </Button>
              </ToolbarGroup>
            )}

            <ToolbarGroup label="Arquivo">
              <Button variant="ghost" size="icon" asChild aria-label="Abrir em nova aba" title="Abrir em nova aba">
                <a href={displayUrl || effectiveUrl} target="_blank" rel="noreferrer">
                  <ExternalLink />
                </a>
              </Button>
              <Button variant="ghost" size="icon" asChild aria-label="Baixar arquivo" title="Baixar arquivo">
                <a href={displayUrl || effectiveUrl} target="_blank" rel="noreferrer" download>
                  <Download />
                </a>
              </Button>
            </ToolbarGroup>

            <div className="ml-auto">
              <Button
                variant="ghost"
                size="icon"
                onClick={() => setShortcutsOpen((s) => !s)}
                aria-label="Atalhos de teclado"
                title="Atalhos (?)"
                aria-pressed={shortcutsOpen}
              >
                <Keyboard />
              </Button>
            </div>
          </div>

          {shortcutsOpen && <ShortcutsHelp onClose={() => setShortcutsOpen(false)} />}
        </div>
      )}
    >
      <div className="flex-1 bg-muted/30 overflow-hidden relative">
        {privateContent.error && (
          <div className="h-full w-full flex items-center justify-center">
            <DocumentErrorState
              title="Não foi possível abrir este documento"
              description="O arquivo não pôde ser baixado desta rota autenticada. Sua sessão pode ter expirado ou seu perfil não tem permissão de leitura."
              url={effectiveUrl}
              onRetry={retry}
            />
          </div>
        )}

        {privateContent.loading && <ImageDocumentSkeleton />}

        {!privateContent.error && kind === "pdf" && (
          <div ref={pdfScrollerRef} className="h-full w-full overflow-auto" onWheel={handleWheelZoom}>
            <div className="min-h-full w-full flex flex-col items-center gap-4 px-4 py-6">
              {reactPdfFailed ? (
                <DocumentErrorState
                  description="O visualizador de PDF não pôde ser carregado neste navegador. Você pode abrir o arquivo externamente."
                  url={effectiveUrl}
                  onRetry={retry}
                />
              ) : !PdfDocument || !PdfPage ? (
                <PdfPageSkeleton pages={1} />
              ) : pdfErrored ? (
                <DocumentErrorState
                  title="Não foi possível abrir este documento"
                  description={pdfErrorDetail
                    ? `Erro: ${pdfErrorDetail}. Verifique sua conexão e tente novamente.`
                    : "O PDF não pôde ser renderizado neste navegador. Você pode tentar novamente ou abrir o arquivo externamente."}
                  url={effectiveUrl}
                  onRetry={retry}
                />
              ) : (
                <PdfDocument
                  key={retryNonce}
                  file={pdfFile}
                  onLoadSuccess={(info) => {
                    setNumPages(info.numPages);
                    setPage((p) => clamp(p, 1, info.numPages));
                    pdfPageRefs.current = new Array(info.numPages).fill(null);
                    setPdfLoaded(true);
                  }}
                  onLoadError={(err: Error) => {
                    console.error("[DocumentViewer] PDF load error:", err?.message || err);
                    setPdfErrored(true);
                    setPdfErrorDetail(err?.message || String(err));
                    setPdfLoaded(false);
                  }}
                  loading={<PdfPageSkeleton pages={1} />}
                  error={
                    <DocumentErrorState
                      description="O PDF não pôde ser renderizado neste navegador."
                      url={effectiveUrl}
                      onRetry={retry}
                    />
                  }
                >
                  {pdfLoaded && numPages
                    ? Array.from({ length: numPages }, (_, i) => (
                      <LazyPdfPage
                        key={i}
                        pageIndex={i}
                        pageNumber={i + 1}
                        scale={zoom}
                        rotate={rotate}
                        PdfPage={PdfPage}
                        scrollerRef={pdfScrollerRef}
                        pageRefCallback={(el) => {
                          pdfPageRefs.current[i] = el;
                        }}
                      />
                    ))
                    : null}
                </PdfDocument>
              )}
            </div>
          </div>
        )}

        {!privateContent.error && kind === "image" && (
          <div
            className={cn(
              "h-full w-full overflow-hidden flex items-center justify-center",
              zoom > 1 ? "cursor-grab active:cursor-grabbing" : "cursor-default",
            )}
            onWheel={handleWheelZoom}
            onPointerDown={handleImgPointerDown}
            onPointerMove={handleImgPointerMove}
            onPointerUp={handleImgPointerUp}
          >
            {imgErrored ? (
              <DocumentErrorState description="A imagem não pôde ser carregada." url={effectiveUrl} onRetry={retry} />
            ) : (
              <>
                {!imgLoaded && (
                  <div className="absolute inset-0">
                    <ImageDocumentSkeleton />
                  </div>
                )}
                <img
                  key={retryNonce}
                  src={displayUrl}
                  alt={title || "Documento"}
                  draggable={false}
                  onLoad={() => setImgLoaded(true)}
                  onError={() => setImgErrored(true)}
                  style={{
                    transform: `translate(${imgOffset.x}px, ${imgOffset.y}px) scale(${zoom}) rotate(${rotate}deg)`,
                    transformOrigin: "center center",
                    maxWidth: "85%",
                    maxHeight: "85%",
                    userSelect: "none",
                    pointerEvents: "none",
                    opacity: imgLoaded ? 1 : 0,
                    transition: "opacity 200ms ease-out",
                  }}
                />
              </>
            )}
          </div>
        )}

        {!privateContent.error && kind === "docx" && (
          <DocxBody url={effectiveUrl} title={title} retryNonce={retryNonce} onRetry={retry} superDocUser={superDocUser} />
        )}

        {!privateContent.error && kind === "unknown" && detecting && <ImageDocumentSkeleton />}
        {!privateContent.error && kind === "unknown" && !detecting && detectionError && (
          <DocumentErrorState
            title="Não foi possível identificar o formato do documento"
            description="O tipo de arquivo não pôde ser detectado. O link pode estar expirado ou o servidor pode estar indisponível. Você pode tentar novamente ou abrir o arquivo externamente."
            url={effectiveUrl}
            onRetry={retry}
          />
        )}
        {!privateContent.error && kind === "unknown" && !detecting && !detectionError && (
          <DocumentUnsupportedState url={effectiveUrl} detectedType={resolvedContentType} />
        )}
      </div>
    </ExpandableSidePanel>
  );
}

function DocxBody({
  url,
  title,
  retryNonce,
  onRetry,
  superDocUser,
}: {
  url: string;
  title?: string;
  retryNonce: number;
  onRetry: () => void;
  superDocUser?: SuperDocUser;
}) {
  const [file, setFile] = useState<File | null>(null);
  const [errored, setErrored] = useState(false);
  const { Editor: SuperDocEditor, failed: superDocFailed } = useSuperDocEditor(true);

  const effectiveUser = superDocUser ?? { name: "Operador", email: "anon@suitee.com" };

  useEffect(() => {
    let cancelled = false;
    setFile(null);
    setErrored(false);
    (async () => {
      try {
        const res = await fetch(url, { credentials: "omit" });
        if (!res.ok) throw new Error(`status ${res.status}`);
        const blob = await res.blob();
        const name = title || "documento.docx";
        const f = new File([blob], name, {
          type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
        });
        if (!cancelled) setFile(f);
      } catch {
        if (!cancelled) setErrored(true);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [url, title, retryNonce]);

  if (errored) {
    return (
      <div className="h-full w-full flex items-center justify-center">
        <DocumentErrorState description="O documento Word não pôde ser carregado neste navegador." url={url} onRetry={onRetry} />
      </div>
    );
  }
  if (!file) {
    return (
      <div className="h-full w-full">
        <ImageDocumentSkeleton />
      </div>
    );
  }
  if (superDocFailed) {
    return (
      <div className="h-full w-full flex items-center justify-center">
        <DocumentErrorState
          description="O visualizador de DOCX não pôde ser carregado neste navegador. Você pode abrir o arquivo externamente."
          url={url}
          onRetry={onRetry}
        />
      </div>
    );
  }
  if (!SuperDocEditor) {
    return (
      <div className="h-full w-full">
        <ImageDocumentSkeleton />
      </div>
    );
  }
  return (
    <div className="h-full w-full overflow-hidden bg-card">
      <SuperDocEditor document={file} user={effectiveUser} isInternal={true} hideToolbar={true} />
    </div>
  );
}

function ToolbarGroup({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div role="group" aria-label={label} className="flex items-center gap-0.5 px-1 py-0.5 rounded-lg border border-border bg-card/50">
      {children}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Lazy PDF page – only renders when the page container enters the viewport.
// Includes per-page error boundary to prevent a single corrupted page from
// crashing the entire viewer.
// ---------------------------------------------------------------------------

const VISIBLE_PAGE_BUFFER = 2;

function LazyPdfPage({
  pageIndex,
  pageNumber,
  scale,
  rotate,
  PdfPage,
  scrollerRef,
  pageRefCallback,
}: {
  pageIndex: number;
  pageNumber: number;
  scale: number;
  rotate: number;
  PdfPage: React.ElementType;
  scrollerRef: React.RefObject<HTMLDivElement | null>;
  pageRefCallback: (el: HTMLDivElement | null) => void;
}) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [isVisible, setIsVisible] = useState(pageIndex < VISIBLE_PAGE_BUFFER);
  const [pageError, setPageError] = useState(false);

  useEffect(() => {
    if (isVisible) return;
    const el = containerRef.current;
    if (!el) return;
    const root = scrollerRef.current;
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setIsVisible(true);
          observer.disconnect();
        }
      },
      {
        root,
        rootMargin: `${VISIBLE_PAGE_BUFFER * 200}px 0px ${VISIBLE_PAGE_BUFFER * 200}px 0px`,
        threshold: 0,
      },
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [isVisible, scrollerRef]);

  const sharedRef = useCallback(
    (el: HTMLDivElement | null) => {
      containerRef.current = el;
      pageRefCallback(el);
    },
    [pageRefCallback],
  );

  return (
    <div ref={sharedRef} data-page-index={pageIndex} className="shadow-sm" style={{ minHeight: 200 }}>
      {isVisible && !pageError && (
        <PdfPage
          pageNumber={pageNumber}
          scale={scale}
          rotate={rotate}
          renderAnnotationLayer={false}
          renderTextLayer={false}
          onRenderError={() => setPageError(true)}
        />
      )}
      {isVisible && pageError && (
        <div className="flex flex-col items-center justify-center gap-3 py-12 px-6 bg-status-critical-soft/30 rounded-lg border border-status-critical/20 text-center">
          <p className="text-sm font-semibold text-foreground">Erro ao renderizar página {pageNumber}</p>
          <p className="text-xs text-muted-foreground">Esta página do documento pode estar corrompida ou em formato não suportado.</p>
        </div>
      )}
    </div>
  );
}

function ShortcutsHelp({ onClose }: { onClose: () => void }) {
  const items: Array<[string, string]> = [
    ["←  /  →", "Página anterior / próxima"],
    ["PgUp  /  PgDn", "Página anterior / próxima"],
    ["Scroll", "Rolar pelo documento"],
    ["+  /  −", "Aumentar / diminuir zoom"],
    ["Ctrl + scroll", "Zoom suave"],
    ["0", "Restaurar visualização"],
    ["Esc", "Fechar"],
    ["?", "Mostrar / ocultar atalhos"],
  ];
  return (
    <div role="dialog" aria-label="Atalhos de teclado" className="mt-3 rounded-xl border border-border bg-muted/40 p-3">
      <div className="flex items-center justify-between mb-2">
        <p className="text-[11px] font-bold text-muted-foreground uppercase tracking-[0.06em]">Atalhos</p>
        <button onClick={onClose} className="text-[11px] text-muted-foreground hover:text-foreground" aria-label="Fechar atalhos">
          fechar
        </button>
      </div>
      <ul className="grid grid-cols-2 gap-x-6 gap-y-1.5">
        {items.map(([key, label]) => (
          <li key={key} className="flex items-center justify-between text-xs gap-3">
            <kbd className="text-[11px] font-mono font-semibold px-1.5 py-0.5 rounded bg-card border border-border text-foreground">{key}</kbd>
            <span className="text-muted-foreground text-right truncate">{label}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
