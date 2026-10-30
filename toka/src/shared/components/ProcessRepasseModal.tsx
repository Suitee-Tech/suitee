import { useForm } from "react-hook-form";
import type { Control, UseFormSetValue } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useState } from "react";
import { CheckCircle2, FileCheck2 } from "lucide-react";
import { toast } from "sonner";
import { useProofUpload } from "../hooks/useProofUpload";
import { useProofPolicy } from "../hooks/useProofPolicy";
import type { OwnerTransfer } from "../services/transfer.service";
import { Form } from "../ui/form";
import { formatCurrencyBRL } from "../utils/format";
import { isLikelyProofURL, proofErrorStageMessage } from "../utils/proof";
import { ProofAttachmentFields } from "./lancamentos/ProofAttachmentFields";
import { FinancialActionNotice, FinancialActionPanel, PanelSection } from "./finance/FinancialActionPanel";

interface ProcessRepasseModalProps {
  transferId: string;
  transfer: OwnerTransfer;
  onClose: () => void;
  onProcess: (transferId: string, evidence: { proof_url?: string; note?: string }) => Promise<void>;
}

const schema = z.object({
  proof_url: z.string().refine(isLikelyProofURL, "URL deve começar com http:// ou https://"),
  note: z.string(),
});
type FormValues = z.infer<typeof schema>;

/**
 * Painel para processar (efetivar) um repasse ao proprietário manualmente,
 * quando o depósito depende de fluxo externo (banco/TED/Pix). Permite anexar o
 * comprovante (arquivo ou link) e uma observação. O backend persiste o
 * proof_url/note no repasse e no audit trail (transfer.processed).
 *
 * O repasse é uma operação externa: nada aqui movimenta dinheiro. O que o
 * painel registra é a confirmação manual do operador, com evidência. O shell é
 * o `FinancialActionPanel`, o mesmo de todas as ações financeiras — cabeçalho
 * com ícone/título, corpo em seções e rodapé fixo com a ação primária —, e a
 * seção de comprovante é o `ProofAttachmentFields` compartilhado.
 */
export function ProcessRepasseModal({ transferId, transfer, onClose, onProcess }: ProcessRepasseModalProps) {
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: "onTouched",
    defaultValues: { proof_url: "", note: "" },
  });

  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  // Estado e envio do comprovante vêm do hook compartilhado: o arquivo enviado
  // é reaproveitado se a gravação financeira falhar e o operador repetir.
  const proof = useProofUpload({
    purpose: "payout",
    onFileAccepted: () => {
      // Com arquivo anexado, o link deixa de ser a fonte — limpa para evitar
      // conflito de validação.
      form.setValue("proof_url", "", { shouldValidate: false });
      setSubmitError(null);
    },
    onFileCleared: () => setSubmitError(null),
  });

  const beneficiary = transfer.owner_id || "Proprietário";
  const netAmount = transfer.net_amount;

  // Política do tenant. O backend aplica a mesma regra, então a interface não
  // pode ser nem mais frouxa (400 depois do envio) nem mais rígida do que ele.
  const { proofRequired } = useProofPolicy();

  // FIN-06: fechar durante o envio do arquivo ou durante a gravação descarta o
  // contexto de uma operação em curso e induz nova tentativa com resultado
  // incerto. O X, o Escape e o clique fora convergem neste mesmo pedido de
  // fechamento do painel; o botão "Cancelar" já fica desabilitado.
  const handleClose = () => {
    if (!submitting) onClose();
  };

  const onSubmit = async (values: FormValues) => {
    if (proofRequired && !proof.file && !values.proof_url.trim()) {
      const message = "Anexe um comprovante ou informe a URL do comprovante antes de concluir o repasse.";
      setSubmitError(message);
      toast.error(message);
      return;
    }

    setSubmitting(true);
    setSubmitError(null);

    // Etapa 1 — envio do arquivo. Uma falha aqui é de armazenamento e o
    // arquivo permanece selecionado para nova tentativa.
    let proofURL: string | undefined;
    try {
      proofURL = await proof.resolveProofURL(values.proof_url);
    } catch (err) {
      const message = proofErrorStageMessage(
        err,
        "upload",
        "Não foi possível enviar o comprovante. O arquivo selecionado foi mantido; tente novamente.",
      );
      setSubmitError(message);
      toast.error(message);
      console.error(err);
      setSubmitting(false);
      return;
    }

    // Etapa 2 — gravação financeira. Não é falha de armazenamento: o
    // comprovante já está guardado e o resultado do registro é incerto, então
    // a orientação é verificar antes de repetir (a mutação não é idempotente).
    try {
      await onProcess(transferId, {
        proof_url: proofURL,
        note: values.note.trim() || undefined,
      });
      onClose();
    } catch (err) {
      const message = proofErrorStageMessage(
        err,
        "mutation",
        "Não foi possível processar o repasse. Revise os dados e tente novamente.",
      );
      setSubmitError(message);
      toast.error(message);
      console.error(err);
    } finally {
      setSubmitting(false);
    }
  };

  const formId = `process-repasse-${transferId}`;

  return (
    <FinancialActionPanel
      onClose={handleClose}
      title="Processar Repasse"
      subtitle={beneficiary}
      icon={<CheckCircle2 className="size-5" />}
      iconClassName="bg-status-ok-soft text-status-ok"
      formId={formId}
      submitLabel="Confirmar Repasse"
      submitIcon={<FileCheck2 className="size-4" />}
      submitting={form.formState.isSubmitting || submitting}
      submittingLabel={proof.uploading ? "Enviando comprovante…" : "Processando…"}
      error={submitError}
      ariaLabel="Processar Repasse"
    >
      <Form {...form}>
        <form id={formId} onSubmit={form.handleSubmit(onSubmit)} className="space-y-5">
          <PanelSection
            title="Repasse ao proprietário"
            description="O depósito já foi realizado fora da plataforma (banco, TED ou Pix); aqui fica registrada a confirmação."
          >
            <div className="rounded-xl border border-border bg-muted/30 px-4 py-3 text-sm">
              <div className="flex items-center justify-between gap-3">
                <span className="font-medium text-foreground">Valor líquido do repasse</span>
                {/* Duas casas: o valor confirmado precisa ser o valor repassado. */}
                <span className="font-bold text-foreground">{formatCurrencyBRL(netAmount)}</span>
              </div>
            </div>
          </PanelSection>

          <PanelSection
            title="Comprovante"
            description="Anexe o arquivo (ou informe o link) para o registro ficar auditável."
          >
            {/* Componente padrão compartilhado por TODOS os fluxos de baixa. */}
            <ProofAttachmentFields
              control={form.control as Control<any>}
              setValue={form.setValue as UseFormSetValue<any>}
              upload={proof}
            />
          </PanelSection>

          <FinancialActionNotice>
            O repasse será marcado como <strong>Concluído</strong> e o comprovante/observação gravados no audit trail.
          </FinancialActionNotice>
        </form>
      </Form>
    </FinancialActionPanel>
  );
}
