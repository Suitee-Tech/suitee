import { Link2, Paperclip, Upload } from "lucide-react";
import { toast } from "sonner";
import type { Control, UseFormSetValue } from "react-hook-form";

import { TextField, TextareaField } from "../../ui/fields";
import type { useProofUpload } from "../../hooks/useProofUpload";

export const PROOF_MAX_SIZE_BYTES = 20 * 1024 * 1024;

export interface ProofAttachmentState {
  file: File | null;
  uploadedProof: { file: File; url: string } | null;
  uploading: boolean;
}

/** Máquina de estado compartilhada devolvida por `useProofUpload` (N-06). */
export type ProofUploadState = Pick<
  ReturnType<typeof useProofUpload>,
  "file" | "uploading" | "handleFileChange"
>;

export interface ProofAttachmentFieldsProps {
  control: Control<any>;
  setValue: UseFormSetValue<any>;
  /**
   * Máquina de estado compartilhada do comprovante. Quando informada, é ela
   * quem manda: o fluxo migrado não mantém mais `file`/`uploadedProof` locais,
   * e as props abaixo ficam sem uso.
   */
  upload?: ProofUploadState;
  /** Estado local — apenas fluxos ainda não migrados para `useProofUpload`. */
  state?: ProofAttachmentState;
  setFile?: (file: File | null) => void;
  setUploadedProof?: (proof: { file: File; url: string } | null) => void;
  setSubmitError?: (message: string | null) => void;
}

/**
 * Seção padronizada de comprovante usada por TODOS os fluxos de baixa do
 * financeiro (cobranças, comissões, payouts, repasses e lançamentos):
 * upload de arquivo (PDF/JPG/PNG, até 20 MB) OU link, mais observação.
 *
 * A origem do estado é escolhida por quem renderiza: fluxos migrados passam o
 * hook `useProofUpload` (fonte única da máquina de estado — N-06) e os demais
 * ainda passam `state`/`setFile`/`setUploadedProof` locais.
 */
export function ProofAttachmentFields({
  control,
  setValue,
  state,
  setFile,
  setUploadedProof,
  upload,
  setSubmitError,
}: ProofAttachmentFieldsProps) {
  const file = upload?.file ?? state?.file ?? null;
  const uploading = upload?.uploading ?? state?.uploading ?? false;

  const handleFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    // Fluxos migrados delegam a seleção ao hook: ele aplica o limite de 20 MB,
    // invalida o envio anterior e avisa quem o instanciou.
    if (upload) {
      upload.handleFileChange(event);
      return;
    }
    const selected = event.target.files?.[0] ?? null;
    if (selected && selected.size > PROOF_MAX_SIZE_BYTES) {
      toast.error("O comprovante deve ter no máximo 20 MB.");
      event.target.value = "";
      setFile?.(null);
      setUploadedProof?.(null);
      return;
    }
    setFile?.(selected);
    setUploadedProof?.(null);
    setSubmitError?.(null);
    // Com arquivo anexado, o link deixa de ser a fonte — limpa para evitar
    // conflito de validação.
    if (selected) setValue("proof_url", "", { shouldValidate: false });
  };

  return (
    <>
      {/* Comprovante: upload de arquivo OU link */}
      <div>
        <p className="mb-1.5 flex items-center gap-1 text-sm font-bold text-foreground">
          <Upload className="h-3.5 w-3.5 text-muted-foreground" /> Arquivo do comprovante (opcional)
        </p>
        <label className="flex min-h-24 cursor-pointer flex-col items-center justify-center rounded-xl border border-dashed border-border bg-muted/30 px-4 py-4 text-center transition-colors hover:border-primary-500 hover:bg-muted/50">
          <Paperclip className="mb-2 h-5 w-5 text-primary-600" />
          <span className="text-sm font-bold text-foreground">{file ? file.name : "Selecionar PDF, JPG ou PNG"}</span>
          <span className="mt-1 text-xs text-muted-foreground">Máximo de 20 MB</span>
          <input
            id="proof-file"
            type="file"
            accept=".pdf,.jpg,.jpeg,.png"
            className="sr-only"
            onChange={handleFileChange}
            disabled={uploading}
          />
        </label>
        {uploading && <p className="mt-1 text-xs text-muted-foreground">Enviando comprovante…</p>}
      </div>

      <div className="flex items-center gap-3 text-xs text-muted-foreground before:h-px before:flex-1 before:bg-border after:h-px after:flex-1 after:bg-border">
        ou informe um link
      </div>

      <TextField
        control={control}
        name="proof_url"
        type="url"
        label={
          <span className="flex items-center gap-1">
            <Link2 className="w-3.5 h-3.5 text-muted-foreground" /> URL do comprovante (opcional)
          </span>
        }
        placeholder="https://drive.google.com/… ou https://s3…/recibo.pdf"
      />

      <TextareaField
        control={control}
        name="note"
        label="Observação (opcional)"
        rows={2}
        placeholder="Ex: TED recebido 14:30, conferido com extrato"
      />
    </>
  );
}
