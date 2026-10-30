// Formatadores puros do formulário de operações (extraído de Finance.tsx —
// P1-04/T-07): rótulos derivados do preview financeiro e das opções de
// comissão. Sem React — testáveis isoladamente.
import { type FinancialPreview } from "../../services/financialPreview.service";

/** Papéis de corretor disponíveis na composição de comissão. */
export const COMMISSION_BROKER_ROLE_OPTIONS = [
  { value: 'captador', label: 'Captador' },
  { value: 'co_captador', label: 'Co-captador' },
  { value: 'visitador', label: 'Visitador' },
  { value: 'atendente', label: 'Atendente' },
  { value: 'fechamento', label: 'Fechamento' },
] as const;

const brl = new Intl.NumberFormat('pt-BR', { style: 'currency', currency: 'BRL' });

/**
 * Resumo do plano de pagamentos programado na esteira, derivado das cobranças
 * projetadas pelo financial preview. Usado para preencher automaticamente o
 * campo "Parcelas / Recorrência" do formulário de operações.
 */
export function paymentPlanLabelFromPreview(preview: FinancialPreview): string {
  const charges = preview.charges ?? [];
  const dealType = (preview.deal_type ?? '').toLowerCase();
  const isSale = dealType === 'sale' || dealType === 'venda';
  if (!isSale) {
    // Locação é sempre recorrente mensal; mesmo sem cobranças geradas ainda,
    // o contrato tem a configuração recorrente programada na esteira.
    return 'Mensal (recorrente)';
  }
  if (charges.length === 0) return '';
  const installments = charges.filter((c) =>
    (c.description ?? '').toLowerCase().includes('parcela'),
  ).length;
  if (installments === 0) {
    return 'À vista (1x)';
  }
  const firstInstallment = charges.find((c) =>
    (c.description ?? '').toLowerCase().includes('parcela'),
  );
  const amountLabel = firstInstallment
    ? ` de ${brl.format(firstInstallment.amount_cents / 100)}`
    : '';
  return `Entrada + ${installments}x${amountLabel}`;
}

/**
 * Resumo dos descontos/abatimentos programados no plano de venda (dação,
 * troca e saldo) — exibidos como "descontos aplicáveis" no formulário.
 */
export function discountSummaryFromSalePlan(plan: {
  dacao_amount_cents?: number;
  troca_amount_cents?: number;
  saldo_amount_cents?: number;
}): string | undefined {
  const parts: string[] = [];
  if (plan.dacao_amount_cents && plan.dacao_amount_cents > 0) {
    parts.push(`Dação: ${brl.format(plan.dacao_amount_cents / 100)}`);
  }
  if (plan.troca_amount_cents && plan.troca_amount_cents > 0) {
    parts.push(`Troca: ${brl.format(plan.troca_amount_cents / 100)}`);
  }
  if (plan.saldo_amount_cents && plan.saldo_amount_cents > 0) {
    parts.push(`Saldo: ${brl.format(plan.saldo_amount_cents / 100)}`);
  }
  return parts.length > 0 ? parts.join(' · ') : undefined;
}
