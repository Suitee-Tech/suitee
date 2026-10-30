import { useEffect, useMemo, useState } from "react";
import { CheckCircle2, Copy, ExternalLink, FileImage, FileText } from "lucide-react";
import { toast } from "sonner";

import { Button } from "../../ui/button";
import { DocumentViewerDialog } from "../DocumentViewerDialog";
import { getProofBrowserURL, getProofContentPath, proofService } from "../../services/proof.service";

interface FinancialProofCardProps {
  url: string;
  title?: string;
  filename?: string;
}

function proofMeta(rawUrl: string) {
  try {
    const parsed = new URL(rawUrl, typeof window !== "undefined" ? window.location.origin : "http://localhost");
    if (parsed.pathname.includes("/api/v1/proofs/content/")) {
      return { extension: "ARQUIVO", filename: "Comprovante armazenado", isImage: false };
    }
    const segment = parsed.pathname.split("/").filter(Boolean).pop() ?? "";
    const filename = decodeURIComponent(segment);
    const extension = filename.split(".").pop()?.toLowerCase() ?? "";
    const isImage = extension === "jpg" || extension === "jpeg" || extension === "png";
    const isGeneratedName = /^[0-9a-f-]{20,}\.(pdf|jpg|jpeg|png)$/i.test(filename);

    return {
      extension: extension === "jpeg" ? "JPG" : extension.toUpperCase() || "ARQUIVO",
      filename: isGeneratedName ? "Arquivo de comprovante" : filename || "Arquivo de comprovante",
      isImage,
    };
  } catch {
    return { extension: "LINK", filename: "Comprovante externo", isImage: false };
  }
}

export function FinancialProofCard({ url, title = "Comprovante de pagamento", filename }: FinancialProofCardProps) {
  const meta = useMemo(() => proofMeta(url), [url]);
  const Icon = meta.isImage ? FileImage : FileText;
  const displayFilename = filename?.trim() || meta.filename;
  const contentPath = useMemo(() => getProofContentPath(url), [url]);
  const [opening, setOpening] = useState(false);
  const [viewer, setViewer] = useState<{
    url: string;
    contentType?: string;
    title: string;
    revokeOnClose: boolean;
  } | null>(null);

  useEffect(() => {
    return () => {
      if (viewer?.revokeOnClose) URL.revokeObjectURL(viewer.url);
    };
  }, [viewer]);

  const openProof = async () => {
    setOpening(true);
    try {
      if (!contentPath) {
        setViewer({ url, title: displayFilename, revokeOnClose: false });
        return;
      }

      const downloaded = await proofService.download(url);
      const objectURL = URL.createObjectURL(downloaded.blob);
      setViewer({
        url: objectURL,
        contentType: downloaded.contentType,
        title: downloaded.filename || displayFilename,
        revokeOnClose: true,
      });
    } catch {
      toast.error("Não foi possível abrir o comprovante. Verifique sua sessão e tente novamente.");
    } finally {
      setOpening(false);
    }
  };

  const copyLink = async () => {
    try {
      const link = contentPath ? getProofBrowserURL(contentPath) : url;
      await navigator.clipboard.writeText(link);
      toast.success(contentPath ? "Link autenticado do comprovante copiado" : "Link externo do comprovante copiado");
    } catch {
      toast.error("Não foi possível copiar o link");
    }
  };

  return (
    <div className="rounded-xl border border-status-ok/25 bg-status-ok-soft/30 p-4" role="group" aria-label={title}>
      <div className="flex items-start gap-3">
        <div className="rounded-lg bg-background p-2 text-status-ok shadow-sm">
          <Icon className="h-5 w-5" aria-hidden="true" />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-bold text-foreground">{title}</p>
          <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <span className="rounded-md border border-status-ok/25 bg-background px-1.5 py-0.5 font-bold text-status-ok">{meta.extension}</span>
            <span className="truncate">{displayFilename}</span>
          </div>
          <p className="mt-2 inline-flex items-center gap-1 text-xs text-status-ok">
            <CheckCircle2 className="h-3.5 w-3.5" aria-hidden="true" />
            {contentPath ? "Acesso autenticado pela sessão" : "Link externo — acesso controlado pelo provedor"}
          </p>
        </div>
      </div>
      <div className="mt-4 flex flex-wrap gap-2 border-t border-status-ok/15 pt-3">
        <Button type="button" size="sm" variant="default" onClick={openProof} loading={opening} loadingText="Carregando..." aria-label={`Abrir ${title}`}>
          <ExternalLink className="h-4 w-4" /> Visualizar comprovante
        </Button>
        <Button type="button" size="sm" variant="outline" onClick={copyLink} aria-label="Copiar link do comprovante">
          <Copy className="h-4 w-4" /> Copiar link
        </Button>
      </div>
      {viewer && (
        <DocumentViewerDialog
          open
          onOpenChange={(open) => {
            if (!open) setViewer(null);
          }}
          url={viewer.url}
          title={viewer.title}
          contentType={viewer.contentType}
        />
      )}
    </div>
  );
}
