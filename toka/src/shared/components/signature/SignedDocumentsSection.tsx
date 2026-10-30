import { useEffect, useMemo, useState } from "react";
import { Download, ExternalLink, FileCheck, FileText, Loader2, ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "../../ui/badge";
import { Button } from "../../ui/button";
import { DocumentViewerDialog } from "../DocumentViewerDialog";
import { signedContractFilename, useSignedContractDocument } from "./useSignedContractDocument";
import { signatureService } from "../../services/signature.service";
import type { Contract, SignatureEnvelope, SignatureEnvelopeDocument } from "../../contracts/types";

interface Props {
  contract: Contract | null;
  title?: string;
}

interface ViewerState {
  url: string;
  title: string;
}

export function SignedDocumentsSection({ contract, title = "Documentos assinados pelas partes" }: Props) {
  const [envelope, setEnvelope] = useState<SignatureEnvelope | null>(null);
  const [viewer, setViewer] = useState<ViewerState | null>(null);
  const signedDocument = useSignedContractDocument(contract?.id);

  const contractIsSigned = (contract?.status ?? "").toString().toUpperCase() === "SIGNED";

  useEffect(() => {
    if (!contractIsSigned || !contract?.id) {
      setEnvelope(null);
      return;
    }
    let cancelled = false;
    signatureService
      .getByContract(contract.id)
      .then((env) => {
        if (!cancelled) setEnvelope(env);
      })
      .catch(() => {
        if (!cancelled) setEnvelope(null);
      });
    return () => {
      cancelled = true;
    };
  }, [contract?.id, contractIsSigned]);

  const documents = useMemo(() => {
    if (!contractIsSigned || !contract) return [];
    const docs = envelope?.documents ?? (envelope?.document ? [envelope.document] : []);
    if (docs.length === 0) {
      return [
        {
          filename: signedContractFilename(contract),
          isPrimary: true,
          envelopeDoc: undefined as SignatureEnvelopeDocument | undefined,
        },
      ];
    }
    return docs.map((d, idx) => ({
      filename: d.filename || (idx === 0 ? signedContractFilename(contract) : `Anexo ${idx}`),
      isPrimary: idx === 0,
      envelopeDoc: d,
    }));
  }, [contract, contractIsSigned, envelope]);

  if (!contractIsSigned || !contract) {
    return null;
  }

  // Abrir e baixar passam pelo mesmo hook do painel de assinatura: a URL vem
  // nova a cada clique (o link guardado no contrato/envelope expira em minutos).
  const handleOpen = async (filename: string) => {
    const url = await signedDocument.open();
    if (url) setViewer({ url, title: filename });
  };

  const handleDownload = async (filename: string) => {
    await signedDocument.download(filename);
  };

  return (
    <div className="bg-card p-5 rounded-2xl border border-status-ok/30 shadow-sm">
      <header className="flex items-start justify-between gap-3 mb-3">
        <div className="flex items-start gap-3">
          <div className="p-2 rounded-xl bg-status-ok-soft">
            <ShieldCheck className="w-4 h-4 text-status-ok" />
          </div>
          <div>
            <h3 className="text-sm font-bold text-foreground">{title}</h3>
            <p className="text-xs text-muted-foreground mt-0.5">
              PDF final emitido pela Clicksign — inclui as assinaturas, evidências de autenticação e log de auditoria.
            </p>
          </div>
        </div>
        <Badge variant="success">
          {documents.length} arquivo{documents.length === 1 ? "" : "s"}
        </Badge>
      </header>

      <ul className="space-y-2">
        {documents.map((d, idx) => (
          <li
            key={`${d.envelopeDoc?.clicksign_document_key ?? "primary"}-${idx}`}
            className="flex items-center justify-between gap-3 rounded-xl border border-border bg-muted/30 px-3 py-2.5"
          >
            <div className="flex items-center gap-2 min-w-0">
              <FileText className="w-4 h-4 text-muted-foreground shrink-0" />
              <div className="min-w-0">
                <div className="text-sm font-semibold text-foreground truncate">{d.filename}</div>
                <div className="text-xs text-muted-foreground flex items-center gap-2 mt-0.5 flex-wrap">
                  {d.isPrimary ? (
                    <Badge variant="outline" className="font-medium">
                      Contrato principal
                    </Badge>
                  ) : (
                    <Badge variant="outline" className="font-medium">
                      Anexo
                    </Badge>
                  )}
                  <span className="inline-flex items-center gap-1">
                    <FileCheck className="w-3 h-3" />
                    Assinado
                  </span>
                  {typeof d.envelopeDoc?.size_bytes === "number" && d.envelopeDoc.size_bytes > 0 && (
                    <span>{formatFileSize(d.envelopeDoc.size_bytes)}</span>
                  )}
                </div>
              </div>
            </div>
            <div className="flex items-center gap-1.5 shrink-0">
              <Button
                variant="outline"
                size="sm"
                disabled={signedDocument.busy === "open" || !d.isPrimary}
                onClick={() => handleOpen(d.filename)}
                title={d.isPrimary ? "Abrir o PDF assinado" : "Anexo signed-URL ainda não exposto pelo envelope"}
              >
                {signedDocument.busy === "open" && d.isPrimary ? (
                  <Loader2 className="w-3.5 h-3.5 animate-spin" />
                ) : (
                  <ExternalLink className="w-3.5 h-3.5" />
                )}
                Visualizar
              </Button>
              {d.isPrimary && (
                <Button
                  variant="outline"
                  size="sm"
                  disabled={signedDocument.busy === "download"}
                  onClick={() => handleDownload(d.filename)}
                  title="Baixar o PDF assinado"
                >
                  {signedDocument.busy === "download" ? (
                    <Loader2 className="w-3.5 h-3.5 animate-spin" />
                  ) : (
                    <Download className="w-3.5 h-3.5" />
                  )}
                  Baixar
                </Button>
              )}
            </div>
          </li>
        ))}
      </ul>

      {viewer && (
        <DocumentViewerDialog
          open={!!viewer}
          onOpenChange={(o) => !o && setViewer(null)}
          url={viewer.url}
          title={viewer.title}
          contentType="application/pdf"
        />
      )}
    </div>
  );
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

