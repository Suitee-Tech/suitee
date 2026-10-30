import { useEffect, useRef, useState } from "react";
import { clsx } from "clsx";
import { Document, Page, pdfjs } from "react-pdf";
import { FileText, FileSignature, File as FileIcon } from "lucide-react";
import type { ContractDocument } from "../../types/contractDocument";
import { getCategoryMeta } from "../../lib/documents";
import { getSignatureStateMeta, resolveSignatureState } from "../../documents/signature";
import { documentService } from "../../services/document.service";
import { toGatewayUrl } from "../../services/api";

pdfjs.GlobalWorkerOptions.workerSrc = new URL("pdfjs-dist/build/pdf.worker.min.mjs", import.meta.url).toString();

function formatSize(bytes: number): string {
  if (!bytes) return "—";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1048576).toFixed(1)} MB`;
}

function formatDate(iso: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  if (d.getFullYear() < 1970) return "";
  return d.toLocaleDateString("pt-BR", { day: "2-digit", month: "2-digit", year: "numeric" });
}

function inferThumbKind(doc: ContractDocument): "pdf" | "image" | "docx" | "none" {
  const ct = (doc.fileType || "").toLowerCase();
  if (ct.includes("pdf")) return "pdf";
  if (ct.startsWith("image/")) return "image";
  if (ct.includes("wordprocessingml") || ct.includes("officedocument")) return "docx";
  if (!doc.fileUrl) return "none";
  const ext = doc.fileUrl.split("?")[0].split(".").pop()?.toLowerCase();
  if (ext === "pdf") return "pdf";
  if (ext === "docx") return "docx";
  if (ext && ["png", "jpg", "jpeg", "webp", "gif"].includes(ext)) return "image";
  return "none";
}

interface Props {
  doc: ContractDocument;
  reference?: string;
  /** Rótulo da origem (contrato, imóvel, cliente...) mostrado no rodapé. */
  originLabel?: string;
  onClick: () => void;
}

export function DocumentGridCard({ doc, reference, originLabel, onClick }: Props) {
  const cardRef = useRef<HTMLButtonElement | null>(null);
  const [visible, setVisible] = useState(false);
  const [preview, setPreview] = useState<{
    documentId: string;
    status: "loading" | "ready" | "error";
    url: string | null;
  } | null>(null);

  useEffect(() => {
    const el = cardRef.current;
    if (!el || visible) return;
    const io = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            setVisible(true);
            io.disconnect();
            break;
          }
        }
      },
      { rootMargin: "200px" },
    );
    io.observe(el);
    return () => io.disconnect();
  }, [visible]);

  const kind = inferThumbKind(doc);

  useEffect(() => {
    if (!visible || (kind !== "pdf" && kind !== "image")) return;

    let cancelled = false;
    setPreview({ documentId: doc.id, status: "loading", url: null });

    documentService
      .getPresignedDownloadUrl(doc.id)
      .then((url) => {
        if (cancelled) return;
        const normalizedUrl = toGatewayUrl(url);
        if (!normalizedUrl) {
          setPreview({ documentId: doc.id, status: "error", url: null });
          return;
        }
        setPreview({ documentId: doc.id, status: "ready", url: normalizedUrl });
      })
      .catch(() => {
        if (!cancelled) {
          setPreview({ documentId: doc.id, status: "error", url: null });
        }
      });

    return () => {
      cancelled = true;
    };
  }, [doc.id, kind, visible]);

  const previewUrl =
    preview?.documentId === doc.id && preview.status === "ready" ? preview.url : null;
  const failPreview = () => {
    setPreview((current) =>
      current?.documentId === doc.id
        ? { documentId: doc.id, status: "error", url: null }
        : current,
    );
  };

  const categoryMeta = getCategoryMeta(doc.category);
  const signatureMeta = getSignatureStateMeta(resolveSignatureState(doc));
  const SignatureIcon = signatureMeta.icon;

  return (
    <button
      ref={cardRef}
      onClick={onClick}
      type="button"
      className="group text-left bg-card border border-border rounded-2xl overflow-hidden shadow-sm hover:shadow-md hover:border-primary-300 transition-all flex flex-col"
    >
      <div className="aspect-[4/3] bg-muted relative flex items-center justify-center overflow-hidden">
        {(!visible || !previewUrl || kind === "docx" || kind === "none") && (
          <ThumbPlaceholder kind={kind} />
        )}
        {visible && kind === "pdf" && previewUrl && (
          <Document
            file={previewUrl}
            loading={<ThumbPlaceholder kind={kind} />}
            error={<ThumbPlaceholder kind={kind} />}
            onLoadError={failPreview}
            className="w-full h-full flex items-center justify-center"
          >
            <Page
              pageNumber={1}
              width={260}
              renderTextLayer={false}
              renderAnnotationLayer={false}
              className="pointer-events-none"
            />
          </Document>
        )}
        {visible && kind === "image" && previewUrl && (
          <img
            src={previewUrl}
            alt={doc.name}
            className="max-h-full max-w-full object-contain"
            loading="lazy"
            onError={failPreview}
          />
        )}

        <span
          title={signatureMeta.description}
          className={clsx(
            "absolute top-2 right-2 inline-flex items-center gap-1 py-0.5 px-2 rounded-full text-caption font-bold border",
            signatureMeta.badgeClass,
          )}
        >
          <SignatureIcon className="w-3 h-3" />
          {signatureMeta.label}
        </span>
      </div>

      <div className="p-3 flex-1 flex flex-col gap-1">
        <p className="text-sm font-bold text-foreground truncate" title={doc.name}>
          {doc.name}
        </p>
        {reference && (
          <p className="text-xs text-muted-foreground truncate" title={reference}>
            {reference}
          </p>
        )}
        <div className="mt-1 flex items-center gap-2 flex-wrap">
          <span
            className={clsx(
              "inline-flex items-center gap-1 py-0.5 px-2 rounded-full text-caption font-bold border",
              categoryMeta.badgeClass,
            )}
          >
            {categoryMeta.label}
          </span>
          {originLabel && (
            <span className="inline-flex items-center gap-1 py-0.5 px-2 rounded-full text-caption font-medium bg-muted text-muted-foreground border border-border">
              {originLabel}
            </span>
          )}
          {(doc.tags ?? []).slice(0, 2).map((t) => (
            <span
              key={t}
              className="inline-flex items-center py-0.5 px-2 rounded-full text-caption font-medium bg-muted text-muted-foreground border border-border"
            >
              {t}
            </span>
          ))}
          {(doc.tags ?? []).length > 2 && <span className="text-caption text-muted-foreground">+{(doc.tags ?? []).length - 2}</span>}
        </div>
        <div className="mt-1 flex items-center justify-between text-caption text-muted-foreground">
          <span>{formatSize(doc.fileSize)}</span>
          <span>{formatDate(doc.uploadDate)}</span>
        </div>
      </div>
    </button>
  );
}

function ThumbPlaceholder({ kind }: { kind: "pdf" | "image" | "docx" | "none" }) {
  const Icon = kind === "pdf" ? FileText : kind === "docx" ? FileSignature : FileIcon;
  return (
    <div className="flex flex-col items-center justify-center gap-2 text-muted-foreground">
      <Icon className="w-10 h-10" />
      <span className="text-xs uppercase tracking-wider">
        {kind === "pdf" ? "PDF" : kind === "docx" ? "DOCX" : kind === "image" ? "Imagem" : "Sem arquivo"}
      </span>
    </div>
  );
}
