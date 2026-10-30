import { useCallback, useState } from "react";
import { toast } from "sonner";
import { proofService, type ProofKind } from "../services/proof.service";
import { MAX_PROOF_SIZE_BYTES, ProofUploadError } from "../utils/proof";

interface UseProofUploadOptions {
  /** Propósito gravado no armazenamento; muda por fluxo (payout/commission/charge). */
  purpose: ProofKind;
  /** Chamado quando o arquivo selecionado é aceito. */
  onFileAccepted?: () => void;
  /** Chamado quando o arquivo é rejeitado por tamanho ou limpo. */
  onFileCleared?: () => void;
}

/**
 * Estado e envio do comprovante, compartilhados pelos fluxos de baixa manual.
 *
 * Reproduz a garantia que já existia em cada cópia: o arquivo enviado é
 * reaproveitado quando a mesma seleção é reenviada (`uploadedProof.file === file`),
 * então uma falha na gravação financeira não obriga a subir o arquivo de novo.
 */
export function useProofUpload({ purpose, onFileAccepted, onFileCleared }: UseProofUploadOptions) {
  const [file, setFile] = useState<File | null>(null);
  const [uploadedProof, setUploadedProof] = useState<{ file: File; url: string } | null>(null);
  const [uploading, setUploading] = useState(false);

  const clearFile = useCallback(() => {
    setFile(null);
    setUploadedProof(null);
    onFileCleared?.();
  }, [onFileCleared]);

  const handleFileChange = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      const selected = event.target.files?.[0] ?? null;
      if (selected && selected.size > MAX_PROOF_SIZE_BYTES) {
        toast.error("O comprovante deve ter no máximo 20 MB.");
        event.target.value = "";
        clearFile();
        return;
      }
      setFile(selected);
      setUploadedProof(null);
      if (selected) onFileAccepted?.();
      else onFileCleared?.();
    },
    [clearFile, onFileAccepted, onFileCleared],
  );

  /**
   * Resolve a URL do comprovante para a submissão: reusa o envio anterior
   * quando o arquivo não mudou, envia quando necessário, ou devolve a URL
   * digitada. Lança `ProofUploadError` se o envio falhar, para que o chamador
   * não confunda a etapa com a gravação financeira.
   */
  const resolveProofURL = useCallback(
    async (typedURL?: string): Promise<string | undefined> => {
      const trimmed = (typedURL ?? "").trim() || undefined;
      if (!file) return trimmed;
      if (uploadedProof?.file === file) return uploadedProof.url;

      setUploading(true);
      try {
        const uploaded = await proofService.upload(file, purpose);
        setUploadedProof({ file, url: uploaded.url });
        return uploaded.url;
      } catch (error) {
        throw new ProofUploadError(error);
      } finally {
        setUploading(false);
      }
    },
    [file, purpose, uploadedProof],
  );

  return { file, uploadedProof, uploading, handleFileChange, resolveProofURL, clearFile };
}
