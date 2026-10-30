import { useState } from "react";
import { useForm } from "react-hook-form";
import type { Control, UseFormSetValue } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { FileCheck2, Wallet } from "lucide-react";
import { toast } from "sonner";

import { isLikelyProofURL, proofErrorStageMessage } from "../utils/proof";
import { useProofPolicy } from "../hooks/useProofPolicy";
import { useProofUpload } from "../hooks/useProofUpload";
import { Form } from "../ui/form";
import { SelectField } from "../ui/fields";
import { ProofAttachmentFields } from "./lancamentos/ProofAttachmentFields";
import {
  FinancialActionNotice,
  FinancialActionPanel,
  PanelSection,
} from "./finance/FinancialActionPanel";
import type { PayoutSettlementMethod } from "../services/payoutOrder.service";

interface PayoutMarkPaidModalProps {
  open: boolean;
  payoutId: string | null;
  payoutLabel?: string;
  onClose: () => void;
  onConfirm: (input: {
    method: PayoutSettlementMethod;
    proofUrl: string;
    proofFilename: string;
    note?: string;
  }) => Promise<void> | void;
}

/** Valores aceitos pelo serviço (`PayoutSettlementMethod`), na ordem do select. */
const SETTLEMENT_METHODS = ["MANUAL_PIX", "MANUAL_TED", "OTHER"] as const;

const METHOD_LABELS: Record<(typeof SETTLEMENT_METHODS)[number], string> = {
  MANUAL_PIX: "PIX",
  MANUAL_TED: "TED",
  OTHER: "Outro",
};

const METHOD_OPTIONS = SETTLEMENT_METHODS.map((value) => ({ value, label: METHOD_LABELS[value] }));

const schema = z.object({
  method: z.enum(SETTLEMENT_METHODS),
  proof_url: z.string().refine(isLikelyProofURL, "URL deve começar com http:// ou https://"),
  note: z.string(),
});

type FormValues = z.infer<typeof schema>;

/**
 * Baixa manual da ordem de pagamento no shell padrão do financeiro
 * (`FinancialActionPanel`): painel lateral com cabeçalho, seções nomeadas,
 * rodapé fixo e erro da operação com `role="alert"`.
 *
 * O formulário é `react-hook-form` + `zod`, como os demais fluxos de baixa, e o
 * comprovante vem da seção compartilhada (`ProofAttachmentFields` +
 * `useProofUpload`): arquivo (até 20 MB) OU link, em vez de um input de arquivo
 * próprio. O dinheiro saiu do sistema, então o comprovante continua sendo
 * exigido para confirmar — mesma regra de antes desta migração.
 *
 * O painel é sempre-aberto por design quando montado; a prop `open` segue na
 * interface para os call sites atuais e, com `false`, nada é renderizado.
 */
export function PayoutMarkPaidModal({
  open,
  payoutId,
  payoutLabel,
  onClose,
  onConfirm,
}: PayoutMarkPaidModalProps) {
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: "onTouched",
    defaultValues: {
      method: "MANUAL_PIX",
      proof_url: "",
      note: "",
    },
  });

  const [submitError, setSubmitError] = useState<string | null>(null);

  // Estado e envio do comprovante vêm do hook compartilhado (N-06): o arquivo
  // enviado é reaproveitado se a gravação financeira falhar e o operador repetir.
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

  // Política do tenant: o backend aplica a mesma regra na borda HTTP.
  const { proofRequired } = useProofPolicy();

  const onSubmit = async (values: FormValues) => {
    setSubmitError(null);

    // Sem evidência não há baixa: o pagamento aconteceu fora do sistema e o
    // comprovante é o rastro de auditoria da ordem.
    if (!proof.file && !values.proof_url.trim()) {
      const message = "Anexe o comprovante antes de confirmar.";
      setSubmitError(message);
      toast.error(message);
      return;
    }

    // Etapa 1 — envio do arquivo. Uma falha aqui é do armazenamento e o arquivo
    // selecionado permanece para nova tentativa.
    let proofURL: string;
    try {
      proofURL = (await proof.resolveProofURL(values.proof_url)) ?? "";
    } catch (err) {
      const message = proofErrorStageMessage(
        err,
        "upload",
        "Não foi possível marcar como pago.",
      );
      setSubmitError(message);
      toast.error(message);
      console.error("mark paid failed", err);
      return;
    }

    // Etapa 2 — gravação financeira. Não é falha de armazenamento: o
    // comprovante já está guardado e o resultado do registro é incerto, então a
    // orientação é verificar antes de repetir (a baixa não é idempotente).
    try {
      await onConfirm({
        method: values.method,
        proofUrl: proofURL,
        // O backend devolve em `filename` o mesmo nome enviado no multipart,
        // então o nome do arquivo local é o mesmo que o serviço grava.
        proofFilename: proof.file?.name ?? "",
        note: values.note,
      });
      form.reset();
      proof.clearFile();
      onClose();
    } catch (err) {
      const message = proofErrorStageMessage(
        err,
        "mutation",
        "Não foi possível marcar como pago.",
      );
      setSubmitError(message);
      toast.error(message);
      console.error("mark paid failed", err);
    }
  };

  if (!open) return null;

  const formId = `payout-mark-paid-${payoutId ?? "ordem"}`;
  // Enquanto o envio do arquivo ou a gravação está em curso, fechar o painel
  // (X, overlay, Esc) descartaria o contexto da operação. O Dialog antigo
  // bloqueava esse gesto e o bloqueio é mantido aqui, no fluxo que o tinha —
  // o botão Cancelar do rodapé já fica desabilitado pelo próprio shell.
  const submitting = form.formState.isSubmitting || proof.uploading;
  const handleClose = () => {
    if (submitting) return;
    onClose();
  };

  return (
    <FinancialActionPanel
      onClose={handleClose}
      title="Marcar ordem como paga"
      subtitle={payoutLabel ? `Ordem para ${payoutLabel}` : payoutId}
      icon={<Wallet className="size-5" />}
      iconClassName="bg-status-ok-soft text-status-ok"
      formId={formId}
      submitLabel="Confirmar pagamento"
      submitIcon={<FileCheck2 className="size-4" />}
      submitting={submitting}
      submittingLabel={proof.uploading ? "Enviando comprovante…" : "Confirmando…"}
      error={submitError}
      ariaLabel="Marcar ordem como paga"
    >
      <Form {...form}>
        <form id={formId} onSubmit={form.handleSubmit(onSubmit)} className="space-y-5">
          <PanelSection
            title="Dados do pagamento"
            description="Método usado na liquidação da ordem, fora do sistema."
          >
            <SelectField
              control={form.control}
              name="method"
              label="Método"
              required
              options={METHOD_OPTIONS}
            />
          </PanelSection>

          <PanelSection
            title="Comprovante"
            description="Anexe o arquivo (ou informe o link) que comprova o pagamento."
          >
            {/* Componente padrão compartilhado por TODOS os fluxos de baixa. */}
            <ProofAttachmentFields
              control={form.control as Control<any>}
              setValue={form.setValue as UseFormSetValue<any>}
              upload={proof}
            />
          </PanelSection>

          <FinancialActionNotice>
            {proofRequired
              ? "A política da imobiliária exige comprovante nesta baixa manual. A ordem só é confirmada com a evidência do pagamento."
              : "A ordem só é confirmada com a evidência do pagamento: anexe o arquivo do comprovante ou informe o link."}
          </FinancialActionNotice>
        </form>
      </Form>
    </FinancialActionPanel>
  );
}
