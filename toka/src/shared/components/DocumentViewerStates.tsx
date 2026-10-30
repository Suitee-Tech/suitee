import type { ReactNode } from "react";
import { AlertCircle, Download, ExternalLink, FileQuestion, Loader2, RotateCw, ShieldAlert, Sparkles } from "lucide-react";
import { clsx } from "clsx";
import { Button } from "../ui/button";

export function PdfPageSkeleton({ pages = 1, className }: { pages?: number; className?: string }) {
  return (
    <div className={clsx("flex flex-col items-center gap-6 py-6", className)} aria-busy="true" aria-live="polite">
      {Array.from({ length: pages }).map((_, idx) => (
        <div
          key={idx}
          className="w-full max-w-[680px] aspect-[1/1.41] rounded bg-card border border-border shadow-sm overflow-hidden relative"
        >
          <div className="px-10 pt-10 pb-6 space-y-3">
            <div className="h-3 w-1/2 rounded bg-muted animate-pulse" />
            <div className="h-2.5 w-3/4 rounded bg-muted animate-pulse" />
          </div>
          <div className="px-10 space-y-2.5">
            {Array.from({ length: 14 }).map((_, line) => (
              <div
                key={line}
                className="h-2 rounded bg-muted animate-pulse"
                style={{ width: `${82 - (line % 4) * 7}%` }}
              />
            ))}
          </div>
          <div className="absolute bottom-10 left-10 right-10 space-y-2">
            <div className="h-2 w-1/3 rounded bg-muted animate-pulse" />
            <div className="h-2 w-1/4 rounded bg-muted animate-pulse" />
          </div>
        </div>
      ))}
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <Loader2 className="w-3.5 h-3.5 animate-spin" aria-hidden="true" />
        Renderizando documento…
      </div>
    </div>
  );
}

export function ImageDocumentSkeleton() {
  return (
    <div className="h-full w-full flex items-center justify-center p-6" aria-busy="true" aria-live="polite">
      <div className="w-full max-w-3xl aspect-[16/10] rounded-2xl bg-muted animate-pulse flex items-center justify-center text-muted-foreground">
        <Loader2 className="w-6 h-6 animate-spin" aria-hidden="true" />
      </div>
    </div>
  );
}

export type DocumentProcessingStep = "uploading" | "queued" | "ocr" | "validating" | "ready";

const STEP_META: Record<DocumentProcessingStep, { label: string; description: string }> = {
  uploading: { label: "Enviando", description: "Transferindo arquivo para o servidor." },
  queued: { label: "Na fila", description: "Aguardando processamento." },
  ocr: { label: "Reconhecendo texto", description: "Extraindo conteúdo via OCR." },
  validating: { label: "Validando", description: "Verificando integridade e dados." },
  ready: { label: "Pronto", description: "Documento disponível para visualização." },
};

const STEP_ORDER: DocumentProcessingStep[] = ["uploading", "queued", "ocr", "validating", "ready"];

export function DocumentProcessingState({
  step,
  progress,
  filename,
}: {
  step: DocumentProcessingStep;
  progress?: number;
  filename?: string;
}) {
  const currentIdx = STEP_ORDER.indexOf(step);
  return (
    <div className="h-full w-full flex items-center justify-center p-6">
      <div className="w-full max-w-md rounded-2xl border border-border bg-card shadow-sm p-6 space-y-5">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-full bg-primary-50 text-primary-600 flex items-center justify-center">
            <Sparkles className="w-5 h-5" aria-hidden="true" />
          </div>
          <div className="min-w-0">
            <p className="text-sm font-bold text-foreground">Preparando seu documento</p>
            {filename && <p className="text-xs text-muted-foreground truncate">{filename}</p>}
          </div>
        </div>

        <ol className="space-y-3" role="list">
          {STEP_ORDER.map((s, idx) => {
            const meta = STEP_META[s];
            const completed = idx < currentIdx;
            const active = idx === currentIdx;
            return (
              <li key={s} className="flex items-start gap-3">
                <span
                  aria-current={active ? "step" : undefined}
                  className={clsx(
                    "mt-0.5 w-5 h-5 rounded-full flex items-center justify-center text-caption font-bold flex-shrink-0",
                    completed && "bg-status-ok text-white",
                    active && "bg-primary text-primary-foreground ring-4 ring-primary-100",
                    !completed && !active && "bg-muted text-muted-foreground",
                  )}
                >
                  {completed ? "✓" : idx + 1}
                </span>
                <div className="flex-1 min-w-0">
                  <p
                    className={clsx(
                      "text-sm font-semibold leading-tight",
                      active ? "text-foreground" : completed ? "text-foreground/80" : "text-muted-foreground",
                    )}
                  >
                    {meta.label}
                  </p>
                  {active && <p className="text-xs text-muted-foreground mt-0.5">{meta.description}</p>}
                  {active && typeof progress === "number" && (
                    <div className="mt-2 h-1 w-full rounded-full bg-muted overflow-hidden">
                      <div
                        className="h-full bg-primary-500 transition-[width] duration-200"
                        style={{ width: `${Math.max(0, Math.min(100, progress))}%` }}
                      />
                    </div>
                  )}
                </div>
              </li>
            );
          })}
        </ol>
      </div>
    </div>
  );
}

export function DocumentErrorState({
  title = "Não foi possível abrir este documento",
  description,
  url,
  onRetry,
  extra,
}: {
  title?: string;
  description?: string;
  url?: string;
  onRetry?: () => void;
  extra?: ReactNode;
}) {
  return (
    <div role="alert" className="h-full w-full flex items-center justify-center p-6">
      <div className="w-full max-w-md rounded-2xl border border-status-critical/20 bg-status-critical-soft/40 p-6 flex flex-col items-center text-center gap-4">
        <div className="w-12 h-12 rounded-full bg-status-critical-soft text-status-critical flex items-center justify-center">
          <AlertCircle className="w-6 h-6" aria-hidden="true" />
        </div>
        <div>
          <p className="text-sm font-bold text-foreground">{title}</p>
          {description && <p className="text-xs text-muted-foreground mt-1 max-w-sm">{description}</p>}
        </div>
        <div className="flex flex-wrap items-center justify-center gap-2 pt-1">
          {onRetry && (
            <Button size="sm" variant="outline" onClick={onRetry} className="gap-2">
              <RotateCw className="w-3.5 h-3.5" aria-hidden="true" />
              Tentar novamente
            </Button>
          )}
          {url && (
            <>
              <Button size="sm" variant="ghost" asChild className="gap-2">
                <a href={url} target="_blank" rel="noreferrer">
                  <ExternalLink className="w-3.5 h-3.5" aria-hidden="true" />
                  Abrir em nova aba
                </a>
              </Button>
              <Button size="sm" variant="ghost" asChild className="gap-2">
                <a href={url} target="_blank" rel="noreferrer" download>
                  <Download className="w-3.5 h-3.5" aria-hidden="true" />
                  Baixar
                </a>
              </Button>
            </>
          )}
        </div>
        {extra}
      </div>
    </div>
  );
}

export function DocumentUnsupportedState({ url, detectedType }: { url: string; detectedType?: string | null }) {
  return (
    <div className="h-full w-full flex items-center justify-center p-6">
      <div className="w-full max-w-md rounded-2xl border border-border bg-card shadow-sm p-6 flex flex-col items-center text-center gap-4">
        <div className="w-12 h-12 rounded-full bg-muted text-muted-foreground flex items-center justify-center">
          <FileQuestion className="w-6 h-6" aria-hidden="true" />
        </div>
        <div>
          <p className="text-sm font-bold text-foreground">Pré-visualização indisponível</p>
          <p className="text-xs text-muted-foreground mt-1 max-w-sm">
            Este formato não pode ser exibido aqui, mas você ainda pode abrir ou baixar o arquivo.
          </p>
          {detectedType && <p className="text-[11px] text-muted-foreground mt-2 font-mono">tipo: {detectedType}</p>}
        </div>
        <div className="flex items-center gap-2 pt-1">
          <Button size="sm" asChild className="gap-2">
            <a href={url} target="_blank" rel="noreferrer">
              <ExternalLink className="w-3.5 h-3.5" aria-hidden="true" />
              Abrir em nova aba
            </a>
          </Button>
          <Button size="sm" variant="outline" asChild className="gap-2">
            <a href={url} target="_blank" rel="noreferrer" download>
              <Download className="w-3.5 h-3.5" aria-hidden="true" />
              Baixar
            </a>
          </Button>
        </div>
      </div>
    </div>
  );
}
