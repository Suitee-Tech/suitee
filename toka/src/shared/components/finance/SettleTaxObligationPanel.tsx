import { useState } from 'react';
import { useForm } from 'react-hook-form';
import type { Control, UseFormSetValue } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { FileCheck2, Receipt } from 'lucide-react';
import { toast } from 'sonner';

import { formatCompetencia, formatISODateBR, formatMoneyBR } from '../../utils/format';
import { isLikelyProofURL, proofErrorStageMessage } from '../../utils/proof';
import { useProofUpload } from '../../hooks/useProofUpload';
import { Form } from '../../ui/form';
import { ProofAttachmentFields } from '../lancamentos/ProofAttachmentFields';
import { FinancialActionNotice, FinancialActionPanel, PanelSection } from './FinancialActionPanel';
import {
  taxObligationService,
  type TaxObligation,
  type TaxObligationKind,
} from '../../services/taxObligation.service';

const KIND_LABEL: Record<TaxObligationKind, string> = {
  IRRF: 'IRRF — Imposto de Renda Retido na Fonte',
  INSS: 'INSS — contribuição previdenciária retida',
  ISS: 'ISS — imposto sobre serviços retido',
  OUTROS: 'Outros tributos retidos',
};

const formatCents = (cents: number) => formatMoneyBR(cents / 100);

// O comprovante do recolhimento é SEMPRE obrigatório: sem a guia (DARF/GPS/guia
// municipal) não existe evidência de que o tributo saiu do caixa. A URL só é
// validada quando digitada — o arquivo anexado é a outra forma de cumprir.
const schema = z.object({
  proof_url: z.string().refine(isLikelyProofURL, 'URL deve começar com http:// ou https://'),
  note: z.string(),
});
type FormValues = z.infer<typeof schema>;

export interface SettleTaxObligationPanelProps {
  obligation: TaxObligation;
  onClose: () => void;
  /** Chamado com a obrigação já recolhida (a lista se atualiza por trás). */
  onSettled: (updated: TaxObligation) => void;
}

/**
 * Baixa (recolhimento) de um tributo retido, no shell ÚNICO das ações
 * financeiras (`FinancialActionPanel`): painel lateral, seções nomeadas,
 * rodapé fixo e erro da operação com `role="alert"`.
 *
 * O comprovante vem da seção compartilhada (`ProofAttachmentFields` +
 * `useProofUpload`), a mesma dos demais fluxos de baixa. O `purpose` do upload
 * continua `commission` porque o armazenamento só aceita os propósitos já
 * existentes (`payout`, `charge`, `commission`, `transaction`) — a obrigação,
 * de todo modo, NASCE de um desconto de comissão.
 */
export function SettleTaxObligationPanel({ obligation, onClose, onSettled }: SettleTaxObligationPanelProps) {
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: 'onTouched',
    defaultValues: { proof_url: '', note: '' },
  });

  const [submitError, setSubmitError] = useState<string | null>(null);

  const proof = useProofUpload({
    purpose: 'commission',
    onFileAccepted: () => {
      // Com arquivo anexado o link deixa de ser a fonte; manter os dois cria
      // conflito de validação e deixa dúvida sobre qual evidência foi gravada.
      form.setValue('proof_url', '', { shouldValidate: false });
      setSubmitError(null);
    },
    onFileCleared: () => setSubmitError(null),
  });

  const itemsCount = obligation.items?.length ?? 0;

  const onSubmit = async (values: FormValues) => {
    setSubmitError(null);

    // A checagem antecipa a exigência do backend (`proof_url` é obrigatório no
    // contrato) para o operador não enviar o arquivo e só então receber 400/422.
    if (!proof.file && !values.proof_url.trim()) {
      const message = 'Anexe o comprovante do recolhimento (guia/DARF) antes de confirmar a baixa.';
      setSubmitError(message);
      toast.error(message);
      return;
    }

    // Etapa 1 — envio do arquivo. Falha aqui é do armazenamento e o arquivo
    // selecionado permanece para nova tentativa.
    let proofURL: string;
    try {
      proofURL = (await proof.resolveProofURL(values.proof_url)) ?? '';
    } catch (err) {
      const message = proofErrorStageMessage(err, 'upload', 'Não foi possível enviar o comprovante do recolhimento.');
      setSubmitError(message);
      toast.error(message);
      console.error(err);
      return;
    }

    // Etapa 2 — gravação da baixa: comprovante + histórico + lançamento contábil.
    try {
      const updated = await taxObligationService.settle(obligation.id, {
        proof_url: proofURL,
        proof_filename: proof.file?.name,
        note: values.note.trim() || undefined,
      });
      toast.success('Recolhimento baixado.');
      onSettled(updated);
      onClose();
    } catch (err) {
      const message = proofErrorStageMessage(
        err,
        'mutation',
        'Não foi possível dar baixa no recolhimento. Revise os dados e tente novamente.',
      );
      setSubmitError(message);
      toast.error(message);
      console.error(err);
    }
  };

  const formId = `settle-tax-obligation-${obligation.id}`;
  const submitting = form.formState.isSubmitting || proof.uploading;
  const kindLabel = KIND_LABEL[obligation.kind] ?? obligation.kind;

  return (
    <FinancialActionPanel
      onClose={onClose}
      title="Baixar recolhimento"
      subtitle={`${obligation.kind} · competência ${formatCompetencia(obligation.competence)}`}
      icon={<Receipt className="size-5" />}
      iconClassName="bg-status-ok-soft text-status-ok"
      formId={formId}
      submitLabel="Confirmar baixa"
      submitIcon={<FileCheck2 className="size-4" />}
      submitting={submitting}
      submittingLabel={proof.uploading ? 'Enviando comprovante…' : 'Baixando…'}
      error={submitError}
      ariaLabel="Baixar recolhimento"
    >
      <Form {...form}>
        <form id={formId} onSubmit={form.handleSubmit(onSubmit)} className="space-y-5">
          <PanelSection title="Tributo retido" description="Confira o que será baixado antes de confirmar.">
            <div className="space-y-1.5 rounded-xl border border-border bg-muted/30 p-4 text-sm">
              <div className="flex items-center justify-between gap-3">
                <span className="text-muted-foreground">Tipo</span>
                <span className="truncate font-bold">{kindLabel}</span>
              </div>
              <div className="flex items-center justify-between gap-3">
                <span className="text-muted-foreground">Competência</span>
                <span className="font-bold">{formatCompetencia(obligation.competence)}</span>
              </div>
              <div className="flex items-center justify-between gap-3">
                <span className="text-muted-foreground">Vencimento</span>
                <span className="font-bold">{formatISODateBR(obligation.due_date) || '—'}</span>
              </div>
              {itemsCount > 0 && (
                <div className="flex items-center justify-between gap-3">
                  <span className="text-muted-foreground">Descontos de origem</span>
                  <span className="font-bold">
                    {itemsCount} {itemsCount === 1 ? 'desconto' : 'descontos'}
                  </span>
                </div>
              )}
              <div className="flex items-center justify-between gap-3 border-t border-border pt-2">
                <span className="text-muted-foreground">Valor a recolher</span>
                <span className="font-bold tabular-nums">{formatCents(obligation.amount_cents)}</span>
              </div>
            </div>
          </PanelSection>

          <PanelSection
            title="Comprovante do recolhimento"
            description="Guia paga (DARF, GPS ou guia municipal) — anexe o arquivo ou informe o link."
          >
            {/* Componente padrão compartilhado por TODOS os fluxos de baixa. */}
            <ProofAttachmentFields
              control={form.control as Control<any>}
              setValue={form.setValue as UseFormSetValue<any>}
              upload={proof}
            />
          </PanelSection>

          <FinancialActionNotice>
            Sem o comprovante o tributo continua <span className="font-bold">a recolher</span>. A baixa registra o
            recolhimento no razão (débito em tributos retidos / crédito em caixa) e encerra esta competência.
          </FinancialActionNotice>
        </form>
      </Form>
    </FinancialActionPanel>
  );
}
