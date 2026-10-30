import { useForm } from "react-hook-form";
import type { Control, FieldErrors, UseFormSetValue } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useState } from "react";
import { CheckCircle2, FileCheck2, Paperclip } from "lucide-react";
import { toast } from "sonner";

import { Form } from "../ui/form";
import { useProofUpload } from "../hooks/useProofUpload";
import { useProofPolicy } from "../hooks/useProofPolicy";
import { formatCurrencyBRL } from "../utils/format";
import { isLikelyProofURL, proofErrorStageMessage } from "../utils/proof";
import { ProofAttachmentFields } from "./lancamentos/ProofAttachmentFields";
import {
  FinancialActionNotice,
  FinancialActionPanel,
  PanelSection,
} from "./finance/FinancialActionPanel";

export interface MarkPaidCommissionPayload {
  proof_url?: string;
  note?: string;
}

export type MarkPaidCommissionMode = "mark-paid" | "attach-proof";

interface MarkPaidCommissionModalProps {
  mode?: MarkPaidCommissionMode;
  commissionId: string;
  brokerLabel?: string;
  netAmount?: number;
  onClose: () => void;
  onSave: (commissionId: string, data: MarkPaidCommissionPayload) => Promise<void>;
}

// O comprovante é válido por arquivo OU por link, então o schema não pode exigir
// a URL: quem cobra a evidência é a política do tenant, checada no submit (o
// arquivo não é um campo do formulário). Vazio continua válido aqui.
const schema = z.object({
  proof_url: z.string().refine(isLikelyProofURL, "URL deve começar com http:// ou https://"),
  note: z.string(),
});
type FormValues = z.infer<typeof schema>;

/**
 * Baixa da comissão: marca como paga (`mark-paid`) ou anexa a evidência de uma
 * comissão já paga (`attach-proof`), no shell ÚNICO das ações financeiras.
 *
 * O fluxo mantém o contrato de antes — mesmos campos (comprovante por arquivo ou
 * link + observação), mesma política de comprovante do tenant, mesmo payload e
 * mesma separação entre falha de armazenamento e falha da gravação financeira.
 * O que muda é a forma: painel lateral padrão, formulário em react-hook-form e a
 * seção de comprovante compartilhada com os outros fluxos de baixa.
 */
export function MarkPaidCommissionModal({
  mode = "mark-paid",
  commissionId,
  brokerLabel,
  netAmount,
  onClose,
  onSave,
}: MarkPaidCommissionModalProps) {
  const attachingProof = mode === "attach-proof";

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: "onTouched",
    defaultValues: { proof_url: "", note: "" },
  });

  const [submitError, setSubmitError] = useState<string | null>(null);

  // Estado e envio do comprovante vêm do hook compartilhado (N-06): ele guarda
  // o arquivo, reaproveita o envio anterior entre tentativas e só sinaliza a
  // falha de armazenamento para quem chamou.
  const proof = useProofUpload({
    purpose: "commission",
    onFileAccepted: () => {
      // O arquivo anexado satisfaz a exigência de evidência, então a falha
      // "sem comprovante" da tentativa anterior deixa de valer. Um link
      // inválido digitado continua sinalizado: ele ainda bloqueia o envio.
      if (form.formState.errors.proof_url?.type === "manual") form.clearErrors("proof_url");
      setSubmitError(null);
    },
    onFileCleared: () => setSubmitError(null),
  });

  // A política do tenant decide se o comprovante é obrigatório (o backend
  // aplica a mesma regra). Antes, só o modo attach-proof exigia: confirmar sem
  // evidência era permitido no modo normal, e o servidor passou a recusar.
  const { proofRequired } = useProofPolicy();
  const requiringProof = attachingProof || proofRequired;

  // FIN-06: fechar durante o envio do arquivo OU durante a gravação descarta o
  // contexto de uma operação em curso. O guard cobre as duas etapas e o mesmo
  // estado desabilita "Cancelar" e o botão principal.
  const submitting = form.formState.isSubmitting || proof.uploading;
  const handleClose = () => {
    if (!submitting) onClose();
  };

  const onSubmit = async (values: FormValues) => {
    const typedProofURL = values.proof_url.trim();

    if (requiringProof && !proof.file && !typedProofURL) {
      const message = "Anexe um arquivo ou informe a URL do comprovante.";
      // `setError` no campo (e não só no bloco de erro do painel) mantém o
      // operador no ponto que precisa ser corrigido.
      form.setError("proof_url", { type: "manual", message });
      toast.error(message);
      return;
    }

    setSubmitError(null);

    // Etapa 1 — envio do arquivo. Uma falha aqui é do armazenamento e o arquivo
    // selecionado permanece para nova tentativa.
    let proofURL: string | undefined;
    try {
      proofURL = await proof.resolveProofURL(typedProofURL);
    } catch (err) {
      const message = proofErrorStageMessage(
        err,
        "upload",
        attachingProof
          ? "Não foi possível anexar o comprovante. O arquivo selecionado foi mantido; tente novamente."
          : "Não foi possível confirmar o pagamento. Revise os dados e tente novamente.",
      );
      setSubmitError(message);
      toast.error(message);
      console.error(err);
      return;
    }

    // Etapa 2 — gravação financeira. Não é falha de armazenamento: o
    // comprovante já está guardado e o resultado do registro é incerto, então a
    // orientação é verificar antes de repetir (a mutação não é idempotente).
    try {
      const payload: MarkPaidCommissionPayload = {
        proof_url: proofURL,
        note: values.note.trim() || undefined,
      };
      await onSave(commissionId, payload);
      onClose();
    } catch (err) {
      const message = proofErrorStageMessage(
        err,
        "mutation",
        attachingProof
          ? "Não foi possível anexar o comprovante. Revise os dados e tente novamente."
          : "Não foi possível confirmar o pagamento. Revise os dados e tente novamente.",
      );
      setSubmitError(message);
      toast.error(message);
      console.error(err);
    }
  };

  // A URL inválida é barrada pelo zod e não chega ao submit; o toast é disparado
  // aqui para o operador não depender só da mensagem do campo.
  const onInvalid = (errors: FieldErrors<FormValues>) => {
    if (errors.proof_url) toast.error("URL de comprovante inválida.");
  };

  const formId = `mark-paid-commission-${commissionId}`;
  const title = attachingProof ? "Resolver: comprovante ausente" : "Marcar comissão como paga";
  // Duas casas decimais: é dinheiro sendo baixado (FIN-05/N-04).
  const subtitle = `${brokerLabel ? `Comissão de ${brokerLabel}` : "Pagamento da comissão"}${
    netAmount !== undefined ? ` · líquido ${formatCurrencyBRL(netAmount)}` : ""
  }`;
  // O rótulo compartilhado do comprovante diz "(opcional)"; a exigência real do
  // fluxo (modo de correção ou política do tenant) continua explícita ao lado,
  // como no asterisco que este painel exibia antes.
  const proofHint = attachingProof
    ? "O comprovante é obrigatório para resolver esta pendência."
    : proofRequired
      ? "O comprovante é obrigatório para confirmar esta baixa manual."
      : "Se você concluir sem comprovante, a comissão ficará em “Valor sob revisão” até a evidência ser anexada.";

  return (
    <FinancialActionPanel
      onClose={handleClose}
      title={title}
      subtitle={subtitle}
      icon={attachingProof ? <Paperclip className="size-5" /> : <CheckCircle2 className="size-5" />}
      iconClassName="bg-status-ok-soft text-status-ok"
      formId={formId}
      submitLabel={attachingProof ? "Anexar e resolver" : "Confirmar pagamento"}
      submitIcon={<FileCheck2 className="size-4" />}
      submitting={submitting}
      submittingLabel={proof.uploading ? "Enviando..." : "Salvando..."}
      error={submitError}
      ariaLabel={title}
    >
      <Form {...form}>
        <form id={formId} onSubmit={form.handleSubmit(onSubmit, onInvalid)} className="space-y-5">
          {attachingProof && (
            <FinancialActionNotice>
              <span className="font-bold">Por que esta correção é necessária? </span>
              A comissão já está paga, mas a evidência do pagamento não foi registrada. O comprovante encerra a
              pendência sem alterar o valor ou a data da baixa.
            </FinancialActionNotice>
          )}

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
            {/* O rótulo compartilhado diz "(opcional)" nos dois campos; aqui a
                exigência real do fluxo (modo de correção ou política do tenant)
                continua explícita, como no asterisco que este painel tinha. */}
            <p className="text-xs text-muted-foreground">{proofHint}</p>
          </PanelSection>

          <FinancialActionNotice>
            A correção é registrada no audit trail. O status e o valor financeiro permanecem inalterados ao anexar
            evidência a uma comissão já paga.
          </FinancialActionNotice>
        </form>
      </Form>
    </FinancialActionPanel>
  );
}
