import { useForm } from "react-hook-form";
import type { Control, UseFormSetValue } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useState } from "react";
import { CheckCircle2, FileCheck2 } from "lucide-react";
import { toast } from "sonner";

import { Form } from "../../ui/form";
import { DateField, SelectField } from "../../ui/fields";
import { transactionService } from "../../services/transaction.service";
import { useProofUpload } from "../../hooks/useProofUpload";
import { useProofPolicy } from "../../hooks/useProofPolicy";
import { formatCurrencyBRL } from "../../utils/format";
import { isLikelyProofURL, isServerError, proofErrorCause, proofErrorStageMessage } from "../../utils/proof";
import { ProofAttachmentFields } from "./ProofAttachmentFields";
import { FinancialActionPanel, PanelSection } from "../finance/FinancialActionPanel";

const todayISODate = () => {
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const dd = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${dd}`;
};

const PAYMENT_METHODS = [
  { value: "pix", label: "PIX" },
  { value: "boleto", label: "Boleto Bancário" },
  { value: "credit_card", label: "Cartão de Crédito" },
  { value: "transfer", label: "Transferência / TED" },
  { value: "cash", label: "Dinheiro" },
];

const schema = z.object({
  payment_method: z.string().min(1, "Selecione o método"),
  payment_date: z.string().min(1, "Informe a data do pagamento"),
  proof_url: z.string().refine(isLikelyProofURL, "URL deve começar com http:// ou https://"),
  note: z.string(),
});
type FormValues = z.infer<typeof schema>;

export interface SettleLancamentoTarget {
  id: string;
  /** receita | despesa | comissao | repasse */
  type: string;
  description: string;
  value: number;
}

const TYPE_LABELS: Record<string, string> = {
  receita: "Entrada",
  despesa: "Saída",
  comissao: "Comissão",
  repasse: "Repasse",
};

export interface SettleLancamentoModalProps {
  lancamento: SettleLancamentoTarget;
  onClose: () => void;
  /** Chamado após a baixa persistida (com comprovante, quando anexado). */
  onSaved: () => void;
}

/**
 * Falha do submit, com a etapa que falhou. 5xx é infraestrutura: vale a
 * mensagem padrão do estágio, que separa upload de comprovante de gravação
 * financeira (FIN-08). Fora disso é regra de negócio e a mensagem do backend —
 * quando existe — orienta melhor o operador, como sempre aconteceu aqui.
 */
const submitFailureMessage = (
  error: unknown,
  stage: "upload" | "mutation",
  fallback: string,
): string => {
  if (isServerError(error)) return proofErrorStageMessage(error, stage, fallback);
  const cause = proofErrorCause(error) as { response?: { data?: { message?: string } } } | undefined;
  return cause?.response?.data?.message ?? proofErrorStageMessage(error, stage, fallback);
};

/**
 * Baixa padronizada de um lançamento (entrada/saída/comissão/repasse):
 * método + data + comprovante (upload ou link) + observação.
 *
 * O fluxo é o mesmo dos demais financeiros — painel lateral do
 * `FinancialActionPanel`, com o alvo da baixa no cabeçalho/corpo, seções
 * nomeadas, rodapé fixo e erro no mesmo lugar —, e o estado do comprovante vem
 * do hook compartilhado `useProofUpload` (N-06), como em cobranças, comissões e
 * repasses. A baixa registra o movimento no caixa (journal) e persiste a
 * evidência no lançamento.
 */
export function SettleLancamentoModal({ lancamento, onClose, onSaved }: SettleLancamentoModalProps) {
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: "onTouched",
    defaultValues: {
      payment_method: "pix",
      payment_date: todayISODate(),
      proof_url: "",
      note: "",
    },
  });

  const [submitError, setSubmitError] = useState<string | null>(null);

  // Estado e envio do comprovante vêm do hook compartilhado (N-06): o arquivo
  // enviado é reaproveitado se a gravação financeira falhar e o operador repetir.
  const proof = useProofUpload({
    purpose: "transaction",
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

  const typeLabel = TYPE_LABELS[lancamento.type] ?? "Lançamento";

  const onSubmit = async (values: FormValues) => {
    setSubmitError(null);

    // A checagem aqui antecipa a exigência para o operador não enviar o arquivo
    // e só então receber um 400 do backend.
    if (proofRequired && !proof.file && !values.proof_url.trim()) {
      const message = "Anexe um comprovante ou informe a URL do comprovante antes de confirmar a baixa.";
      setSubmitError(message);
      toast.error(message);
      return;
    }

    // Etapa 1 — envio do arquivo. Uma falha aqui é do armazenamento e o arquivo
    // selecionado permanece para nova tentativa.
    let proofURL: string | undefined;
    try {
      proofURL = await proof.resolveProofURL(values.proof_url);
    } catch (err) {
      const message = submitFailureMessage(
        err,
        "upload",
        "Não foi possível enviar o comprovante. O arquivo selecionado foi mantido; tente novamente.",
      );
      setSubmitError(message);
      toast.error(message);
      console.error(err);
      return;
    }

    // Etapa 2 — gravação da baixa: movimento no caixa + evidência no lançamento.
    try {
      const isoPaidAt = new Date(`${values.payment_date}T12:00:00`).toISOString();
      await transactionService.settle(lancamento.id, {
        payment_date: isoPaidAt,
        payment_method: values.payment_method,
        proof_url: proofURL,
        note: values.note.trim() || undefined,
      });
      toast.success(`${typeLabel} baixada.`);
      onSaved();
      onClose();
    } catch (err) {
      const message = submitFailureMessage(
        err,
        "mutation",
        "Não foi possível baixar o lançamento. Revise os dados e tente novamente.",
      );
      setSubmitError(message);
      toast.error(message);
      console.error(err);
    }
  };

  const formId = `settle-lancamento-${lancamento.id}`;

  return (
    <FinancialActionPanel
      onClose={onClose}
      title={`Baixar ${typeLabel}`}
      subtitle="Registra o pagamento e o movimento no caixa"
      icon={<CheckCircle2 className="size-5" />}
      iconClassName="bg-status-ok-soft text-status-ok"
      formId={formId}
      submitLabel="Confirmar baixa"
      submitIcon={<FileCheck2 className="size-4" />}
      submitting={form.formState.isSubmitting}
      submittingLabel={proof.uploading ? "Enviando comprovante…" : "Baixando…"}
      error={submitError}
      ariaLabel={`Baixar ${typeLabel}`}
    >
      <Form {...form}>
        <form id={formId} onSubmit={form.handleSubmit(onSubmit)} className="space-y-5">
          <PanelSection title="Lançamento" description="Confira o que será baixado antes de confirmar.">
            <div className="space-y-1.5 rounded-xl border border-border bg-muted/30 p-4 text-sm">
              <div className="flex items-center justify-between gap-3">
                <span className="text-muted-foreground">Descrição</span>
                <span className="truncate font-bold">{lancamento.description || typeLabel}</span>
              </div>
              <div className="flex items-center justify-between gap-3">
                <span className="text-muted-foreground">Valor</span>
                <span className="font-bold tabular-nums">{formatCurrencyBRL(lancamento.value)}</span>
              </div>
            </div>
          </PanelSection>

          <PanelSection
            title="Dados da baixa"
            description="Método e data do pagamento conferidos com o comprovante."
          >
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <SelectField
                control={form.control}
                name="payment_method"
                label="Método de Pagamento"
                required
                options={PAYMENT_METHODS}
              />
              <DateField
                control={form.control}
                name="payment_date"
                label="Data do Pagamento"
                required
                max={todayISODate()}
              />
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
        </form>
      </Form>
    </FinancialActionPanel>
  );
}
