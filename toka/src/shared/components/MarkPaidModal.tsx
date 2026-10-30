import { useForm, useWatch } from "react-hook-form";
import type { Control, UseFormSetValue } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useMemo, useState } from "react";
import { CheckCircle2, FileCheck2 } from "lucide-react";
import { toast } from "sonner";
import type { Charge, MarkPaidDTO } from "../services/charge.service";
import { useProofUpload } from "../hooks/useProofUpload";
import { useProofPolicy } from "../hooks/useProofPolicy";
import { Form } from "../ui/form";
import { DateField, MoneyField, SelectField } from "../ui/fields";
import { computeEconomicBreakdown } from "./ChargeDetailDrawer";
import { SettlementEffects } from "./SettlementEffects";
import type { SettlementEffectRow, SettlementEffectsData } from "../lib/lancamentos/settlement";
import { formatCurrencyBRL } from "../utils/format";
import { getChargeContextLabels } from "../lib/finance/chargeLabels";
import { isLikelyProofURL, proofErrorStageMessage } from "../utils/proof";
import { ProofAttachmentFields } from "./lancamentos/ProofAttachmentFields";
import {
  FinancialActionNotice,
  FinancialActionPanel,
  PanelSection,
} from "./finance/FinancialActionPanel";

interface MarkPaidModalProps {
  chargeId: string;
  defaultAmount: number;
  /** Cobrança completa (opcional) — habilita o painel de efeitos da baixa. */
  charge?: Charge;
  onClose: () => void;
  onSave: (chargeId: string, data: MarkPaidDTO) => Promise<void>;
}

// HTML <input type="date"> usa YYYY-MM-DD local-naive — convertemos pra ISO
// com 12:00 local no submit. 12:00 evita drift de fuso para o dia anterior.
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
  paid_amount: z.number().positive("Valor pago inválido"),
  paid_at: z.string().min(1, "Informe a data do pagamento"),
  proof_url: z.string().refine(isLikelyProofURL, "URL deve começar com http:// ou https://"),
  note: z.string(),
});
type FormValues = z.infer<typeof schema>;

export function MarkPaidModal({ chargeId, defaultAmount, charge, onClose, onSave }: MarkPaidModalProps) {
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: "onTouched",
    defaultValues: {
      payment_method: "pix",
      paid_amount: defaultAmount,
      paid_at: todayISODate(),
      proof_url: "",
      note: "",
    },
  });

  const [submitError, setSubmitError] = useState<string | null>(null);
  const labels = useMemo(() => getChargeContextLabels(charge?.type), [charge?.type]);

  // Estado e envio do comprovante vêm do hook compartilhado (N-06): o arquivo
  // enviado é reaproveitado se a gravação financeira falhar e o operador repetir.
  const proof = useProofUpload({
    purpose: "charge",
    onFileAccepted: () => {
      // Com arquivo anexado, o link deixa de ser a fonte — limpa para evitar
      // conflito de validação.
      form.setValue("proof_url", "", { shouldValidate: false });
      setSubmitError(null);
    },
    onFileCleared: () => setSubmitError(null),
  });

  // N-04: o resumo abaixo é uma prévia do que será gravado, então precisa
  // seguir o campo editável — `defaultAmount` apenas semeia o formulário.
  const paidAmount = useWatch({ control: form.control, name: "paid_amount", defaultValue: defaultAmount });

  // Política do tenant: o backend aplica a mesma regra na borda HTTP.
  const { proofRequired } = useProofPolicy();

  const settlementData = useMemo<SettlementEffectsData | null>(() => {
    if (!charge) return null;
    const bd = computeEconomicBreakdown(charge);
    // Duas casas em todas as linhas: é a divisão do dinheiro confirmado, e o
    // operador precisa enxergar os centavos que está registrando.
    const effects: SettlementEffectRow[] = [{ label: "Valor desta baixa", value: formatCurrencyBRL(paidAmount) }];
    if (bd.desconto > 0) effects.push({ label: "Descontos incidentes", value: `-${formatCurrencyBRL(bd.desconto)}`, accent: "critical" });
    if (bd.imobiliaria > 0) effects.push({ label: labels.agencyFee, value: formatCurrencyBRL(bd.imobiliaria), accent: "primary" });
    if (bd.corretor > 0) effects.push({ label: labels.brokerFee, value: formatCurrencyBRL(bd.corretor), accent: "primary" });
    if (bd.proprietario > 0) effects.push({ label: labels.ownerTransfer, value: formatCurrencyBRL(bd.proprietario), accent: "ok" });
    if (bd.parceiro > 0) effects.push({ label: labels.partnerTransfer, value: formatCurrencyBRL(bd.parceiro) });
    if (bd.tributario > 0) effects.push({ label: labels.taxWithheld, value: formatCurrencyBRL(bd.tributario) });
    if (bd.outros > 0) effects.push({ label: labels.other, value: formatCurrencyBRL(bd.outros), accent: "muted" });
    return {
      title: "Registrar pagamento",
      effects,
      accounting: [
        "Débito: Caixa (valor recebido).",
        "Crédito: Obrigação ao proprietário + Comissão a pagar + Receita da imobiliária.",
      ],
      impactNote:
        "A baixa distribui o valor entre os destinos do valor. Se houver mais de um item em aberto, a alocação é proporcional ao saldo de cada um.",
    };
  }, [charge, paidAmount, labels]);

  const onSubmit = async (values: FormValues) => {
    setSubmitError(null);

    // A política do tenant decide se a baixa manual exige comprovante; o
    // backend aplica a mesma regra. A checagem aqui antecipa a exigência para o
    // operador não enviar o arquivo e só então receber um 400.
    if (proofRequired && !proof.file && !values.proof_url.trim()) {
      const message = "Anexe um comprovante ou informe a URL do comprovante antes de confirmar o pagamento.";
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
      const message = proofErrorStageMessage(
        err,
        "upload",
        "Não foi possível enviar o comprovante. O arquivo selecionado foi mantido; tente novamente.",
      );
      setSubmitError(message);
      toast.error(message);
      console.error(err);
      return;
    }

    // Etapa 2 — gravação financeira. Não é falha de armazenamento: o
    // comprovante já está guardado e o resultado do registro é incerto, então a
    // orientação é verificar antes de repetir (a baixa não é idempotente).
    try {
      // paid_at ISO completo (meio-dia local) — backend aceita time.Time.
      const isoPaidAt = new Date(`${values.paid_at}T12:00:00`).toISOString();
      const payload: MarkPaidDTO = {
        payment_method: values.payment_method,
        paid_amount: values.paid_amount,
        paid_at: isoPaidAt,
        proof_url: proofURL,
        note: values.note.trim() || undefined,
      };
      await onSave(chargeId, payload);
      onClose();
    } catch (err) {
      const message = proofErrorStageMessage(
        err,
        "mutation",
        "Não foi possível confirmar o pagamento. Revise os dados e tente novamente.",
      );
      setSubmitError(message);
      toast.error(message);
      console.error(err);
    }
  };

  const formId = `mark-paid-${chargeId}`;

  return (
    <FinancialActionPanel
      onClose={onClose}
      title="Registrar pagamento"
      subtitle={charge ? `${charge.description} · ${formatCurrencyBRL(defaultAmount)}` : "Confirme o recebimento da cobrança"}
      icon={<CheckCircle2 className="size-5" />}
      iconClassName="bg-status-ok-soft text-status-ok"
      formId={formId}
      submitLabel="Confirmar recebimento"
      submitIcon={<FileCheck2 className="size-4" />}
      submitting={form.formState.isSubmitting}
      submittingLabel={proof.uploading ? "Enviando comprovante…" : "Confirmando…"}
      error={submitError}
      ariaLabel="Registrar pagamento"
    >
      <Form {...form}>
        <form id={formId} onSubmit={form.handleSubmit(onSubmit)} className="space-y-5">
          <PanelSection
            title="Dados do recebimento"
            description="Valor, data e forma de pagamento conferidos com o comprovante."
          >
            <SelectField
              control={form.control}
              name="payment_method"
              label="Método de pagamento"
              required
              options={PAYMENT_METHODS}
            />

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <MoneyField control={form.control} name="paid_amount" label="Valor recebido (R$)" required />
              <DateField
                control={form.control}
                name="paid_at"
                label="Data do pagamento"
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

          <FinancialActionNotice>
            <span className="font-bold">Atenção: </span>
            com comprovante, observação ou data, o registro grava evidência no audit trail (operador + comprovante +
            data do pagamento).
          </FinancialActionNotice>

          {settlementData && (
            <PanelSection
              title="Destinos do valor"
              description="Prévia do que será gravado com o valor informado acima."
            >
              <SettlementEffects data={settlementData} />
            </PanelSection>
          )}
        </form>
      </Form>
    </FinancialActionPanel>
  );
}
