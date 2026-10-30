import { useEffect, useState } from "react";
import { ExternalLink, FileText, Loader2, RotateCw } from "lucide-react";
import { documentService, type DocumentDTO } from "../../services/document.service";

type FileKind = "pdf" | "image" | "other";

function inferFileKind(fileType: string | undefined, name: string | undefined): FileKind {
  const contentType = (fileType || "").toLowerCase();
  if (contentType.includes("pdf")) return "pdf";
  if (contentType.startsWith("image/")) return "image";
  const clean = (name || "").split("#")[0]?.split("?")[0] || "";
  const extension = clean.split(".").pop()?.toLowerCase();
  if (extension === "pdf") return "pdf";
  if (extension && ["png", "jpg", "jpeg", "webp", "gif", "bmp", "tiff", "svg"].includes(extension)) return "image";
  return "other";
}

function formatBytes(bytes: number | undefined): string {
  if (!bytes || bytes <= 0) return "";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/**
 * Preview inline para o workspace de revisão.
 *
 * Usa a MESMA rota pré-assinada do “Ver arquivo”/Documentos do negócio, em vez
 * de tentar baixar a URL do checklist com fetch autenticado (que redireciona
 * para o Storage e falhava com “sessão expirada”). A URL pré-assinada já
 * carrega o token e funciona em iframe/img, sem depender de header.
 */
export function DocumentInlinePreview({ document }: { document: DocumentDTO | null }) {
  const [displayUrl, setDisplayUrl] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let cancelled = false;

    if (!document?.id) {
      setDisplayUrl("");
      setLoading(false);
      setError(false);
      return;
    }

    setDisplayUrl("");
    setLoading(true);
    setError(false);

    documentService
      .getPresignedDownloadUrl(document.id)
      .then((url) => {
        if (cancelled) return;
        if (!url) {
          setLoading(false);
          setError(true);
          return;
        }
        setDisplayUrl(url);
        setLoading(false);
      })
      .catch(() => {
        if (cancelled) return;
        setLoading(false);
        setError(true);
      });

    return () => {
      cancelled = true;
    };
  }, [document?.id, reloadKey]);

  const kind = inferFileKind(document?.file_type, document?.name);
  const openUrl = displayUrl;
  const size = formatBytes(document?.file_size);
  const hasDocument = !!document?.id;

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-none items-center justify-between gap-2 border-b border-border bg-card px-3 py-2">
        <div className="min-w-0">
          <p className="truncate text-xs font-bold text-foreground">{document?.name || "Documento"}</p>
          <p className="text-micro text-muted-foreground">
            {kind === "pdf" ? "PDF" : kind === "image" ? "Imagem" : document?.file_type || "Arquivo"}
            {size ? ` · ${size}` : ""}
          </p>
        </div>
        {openUrl && (
          <a
            href={openUrl}
            target="_blank"
            rel="noreferrer"
            className="inline-flex shrink-0 items-center gap-1 text-caption font-semibold text-primary hover:underline"
          >
            <ExternalLink className="h-3 w-3" /> Abrir
          </a>
        )}
      </div>

      <div className="relative flex min-h-0 flex-1 items-center justify-center overflow-hidden bg-muted/30 p-2">
        {!hasDocument && (
          <div className="max-w-[240px] text-center">
            <FileText className="mx-auto mb-2 h-8 w-8 text-muted-foreground/60" />
            <p className="text-xs font-medium text-foreground">Arquivo sem URL de visualização</p>
            <p className="mt-1 text-caption text-muted-foreground">
              Abra o documento pela esteira para conferir a evidência.
            </p>
          </div>
        )}

        {hasDocument && loading && (
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" /> Carregando documento…
          </div>
        )}

        {hasDocument && !loading && error && (
          <div className="max-w-[280px] text-center">
            <FileText className="mx-auto mb-2 h-8 w-8 text-status-critical/60" />
            <p className="text-xs font-medium text-foreground">Não foi possível carregar o documento</p>
            <p className="mt-1 text-caption text-muted-foreground">
              O arquivo pode estar indisponível ou a sessão expirou.
            </p>
            <div className="mt-2 flex items-center justify-center gap-2">
              <button
                type="button"
                onClick={() => setReloadKey((key) => key + 1)}
                className="inline-flex items-center gap-1 rounded-md border border-border px-2 py-1 text-caption font-semibold text-foreground hover:bg-muted"
              >
                <RotateCw className="h-3 w-3" /> Tentar novamente
              </button>
              {openUrl && (
                <a
                  href={openUrl}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-caption font-semibold text-primary hover:underline"
                >
                  <ExternalLink className="h-3 w-3" /> Abrir
                </a>
              )}
            </div>
          </div>
        )}

        {hasDocument && !loading && !error && displayUrl && kind === "pdf" && (
          <iframe
            title={document?.name || "Documento"}
            src={displayUrl}
            className="h-full w-full rounded-md border border-border bg-white"
          />
        )}

        {hasDocument && !loading && !error && displayUrl && kind === "image" && (
          <img
            src={displayUrl}
            alt={document?.name || "Documento"}
            className="max-h-full max-w-full rounded-md object-contain"
          />
        )}

        {hasDocument && !loading && !error && displayUrl && kind === "other" && (
          <div className="max-w-[260px] text-center">
            <FileText className="mx-auto mb-2 h-8 w-8 text-muted-foreground/60" />
            <p className="text-xs font-medium text-foreground">Pré-visualização não disponível</p>
            <p className="mt-1 text-caption text-muted-foreground">
              Este formato não pode ser exibido ao lado dos campos. Use “Abrir” para baixar o arquivo.
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
