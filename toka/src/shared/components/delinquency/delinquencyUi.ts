import { formatCurrencyBRL, formatDateTimeBR, formatISODateBR } from '../../utils/format';
import type {
  AgreementStatus,
  CollectionAgreement,
  CollectionStatus,
  GuaranteeActivationStatus,
  GuaranteeType,
  JudicialOutcomeStatus,
  JudicialStatus,
} from '../../services/collection.service';
import type { DelinquencyConfig } from '../../services/delinquency.service';

/** Abas reais da tela. Cada uma aplica um filtro de status NO SERVIDOR. */
export type DelinquencyTab = 'inadimplentes' | 'cobranca' | 'garantias' | 'judicial';

/**
 * FALLBACK do limiar de dias de atraso para o caso de cobrança ser criado
 * automaticamente.
 *
 * O valor REAL vem do servidor em `GET /delinquency/dashboard` →
 * `config.auto_create_collection_after_days` (ver `autoCreateCaseAfterDays`).
 * Este número só é usado quando o payload não traz o bloco `config` (servidor
 * antigo, resposta mockada): é o padrão histórico da régua, não uma segunda
 * verdade — se o backend mudar o padrão, o dashboard continua mandando.
 */
export const DEFAULT_AUTO_CREATE_CASE_AFTER_DAYS = 5;

/**
 * Limiar efetivo em dias: o do servidor quando vem um número válido, senão o
 * fallback documentado. Nunca devolve `NaN`/`0` (um limiar zerado diria que o
 * caso nasce no vencimento).
 */
export function autoCreateCaseAfterDays(
  config?: Pick<DelinquencyConfig, 'auto_create_collection_after_days'> | null,
): number {
  const value = config?.auto_create_collection_after_days;
  return typeof value === 'number' && Number.isFinite(value) && value > 0
    ? value
    : DEFAULT_AUTO_CREATE_CASE_AFTER_DAYS;
}

/**
 * Nota que explica por que a cobrança vencida ainda não virou caso. O número e
 * a origem vêm da configuração do servidor: `tenant` é o valor da imobiliária,
 * `global` é o padrão da plataforma e a ausência de `source` (payload antigo)
 * não afirma nenhum dos dois.
 */
export function autoCreateThresholdNote(
  thresholdDays: number,
  source?: DelinquencyConfig['source'] | null,
): string {
  const origin =
    source === 'tenant'
      ? 'limiar configurado pela própria imobiliária'
      : source === 'global'
        ? 'padrão da plataforma'
        : 'limiar da régua';
  return (
    `O caso de cobrança é aberto automaticamente quando o atraso alcança ${thresholdDays} dias (${origin}) ` +
    '— antes disso a cobrança aparece só aqui. O caso também pode ser aberto manualmente.'
  );
}

export const DELINQUENCY_TABS: ReadonlyArray<{ id: DelinquencyTab; label: string; status?: CollectionStatus }> = [
  { id: 'inadimplentes', label: 'Inadimplentes' },
  { id: 'cobranca', label: 'Cobrança', status: 'ACTIVE' },
  { id: 'garantias', label: 'Garantias', status: 'GUARANTEE_ACTIVATED' },
  { id: 'judicial', label: 'Judicial', status: 'JUDICIAL' },
];

export const tabStatus = (tab: DelinquencyTab): CollectionStatus | undefined =>
  DELINQUENCY_TABS.find((item) => item.id === tab)?.status;

export const STATUS_LABEL: Record<CollectionStatus, string> = {
  ACTIVE: 'Em Cobrança',
  NEGOTIATED: 'Negociado',
  GUARANTEE_ACTIVATED: 'Garantia Acionada',
  JUDICIAL: 'Judicial',
  RESOLVED: 'Resolvido',
};

export const statusLabel = (status: string): string => STATUS_LABEL[status as CollectionStatus] ?? status;

export function statusBadgeClass(status: string): string {
  switch (status) {
    case 'ACTIVE':
      return 'bg-primary-100 text-primary-800';
    case 'GUARANTEE_ACTIVATED':
      return 'bg-status-warning-soft text-status-warning border border-status-warning/30';
    case 'JUDICIAL':
      return 'bg-status-critical/90 text-white';
    case 'NEGOTIATED':
      return 'bg-muted text-muted-foreground border border-border';
    case 'RESOLVED':
      return 'bg-status-ok-soft text-status-ok border border-status-ok/30';
    default:
      return 'bg-muted text-muted-foreground';
  }
}

export const GUARANTEE_TYPE_LABEL: Record<GuaranteeType, string> = {
  SEGURO_FIANCA: 'Seguro Fiança',
  FIADOR: 'Fiador',
  CAUCAO: 'Depósito Caução',
};

export const GUARANTEE_STATUS_LABEL: Record<GuaranteeActivationStatus, string> = {
  REQUESTED: 'Acionamento aberto',
  ACCEPTED: 'Aceito pelo garantidor',
  REJECTED: 'Recusado pelo garantidor',
  PAID: 'Valor recuperado',
  CANCELLED: 'Acionamento cancelado',
};

export const JUDICIAL_STATUS_LABEL: Record<JudicialStatus, string> = {
  EM_TRAMITE: 'Em trâmite',
  ACORDO: 'Acordo',
  SENTENCA_PROCEDENTE: 'Sentença procedente',
  SENTENCA_IMPROCEDENTE: 'Sentença improcedente',
  ARQUIVADO: 'Arquivado',
};

export const AGREEMENT_STATUS_LABEL: Record<AgreementStatus, string> = {
  ACTIVE: 'Em andamento',
  FULFILLED: 'Cumprido',
  BROKEN: 'Quebrado',
  CANCELLED: 'Cancelado',
};

export const AGREEMENT_STATUS_BADGE_CLASS: Record<AgreementStatus, string> = {
  ACTIVE: 'bg-primary-100 text-primary-800',
  FULFILLED: 'bg-status-ok-soft text-status-ok border border-status-ok/30',
  BROKEN: 'bg-status-critical-soft text-status-critical border border-status-critical/30',
  CANCELLED: 'bg-muted text-muted-foreground border border-border',
};

/**
 * Efeito de cada DESFECHO judicial sobre o caso — o mesmo mapa que o backend
 * aplica (`JudicialStatus.OutcomeTargetStatus`). O operador vê o que vai
 * acontecer antes de confirmar, porque o desfecho não é um simples rótulo: ele
 * devolve o caso à cobrança (improcedente/arquivado) ou o encaminha ao acordo
 * (acordo homologado).
 */
export const JUDICIAL_OUTCOME_LABEL: Record<JudicialOutcomeStatus, string> = {
  ACORDO: 'Acordo',
  SENTENCA_PROCEDENTE: 'Sentença procedente',
  SENTENCA_IMPROCEDENTE: 'Sentença improcedente',
  ARQUIVADO: 'Arquivado',
};

export const JUDICIAL_OUTCOME_EFFECT: Record<JudicialOutcomeStatus, string> = {
  ACORDO:
    'O caso vai para NEGOTIATED (Negociado) e o acordo é formalizado depois no painel de Acordo, com as parcelas substituindo as cobranças originais.',
  SENTENCA_PROCEDENTE: 'O caso segue em JUDICIAL aguardando o recebimento do valor condenado.',
  SENTENCA_IMPROCEDENTE: 'O caso volta para cobrança ativa (ACTIVE) e a régua de cobrança retoma.',
  ARQUIVADO: 'O caso volta para cobrança ativa (ACTIVE) e a régua de cobrança retoma.',
};

/** Estados do caso que aceitam criar acordo (`POST /agreement`). */
export function canCreateAgreement(status: CollectionStatus | null | undefined): boolean {
  return status === 'ACTIVE' || status === 'NEGOTIATED' || status === 'JUDICIAL';
}

const round2 = (value: number): number => Math.round(value * 100) / 100;

const toAmount = (value: number | null | undefined): number | null =>
  typeof value === 'number' && Number.isFinite(value) ? value : null;

/** Aceita número inteiro vindo do input (`type=number` devolve string) ou vazio. */
const toInteger = (value: number | string | null | undefined): number | null => {
  if (typeof value === 'number') return Number.isInteger(value) ? value : null;
  if (typeof value === 'string') {
    const trimmed = value.trim();
    return /^\d+$/.test(trimmed) ? Number(trimmed) : null;
  }
  return null;
};

const parseDate = (value: string | null | undefined): Date | null => {
  const trimmed = (value ?? '').trim();
  if (!trimmed) return null;
  const parsed = new Date(/^\d{4}-\d{2}-\d{2}$/.test(trimmed) ? `${trimmed}T12:00:00Z` : trimmed);
  return Number.isNaN(parsed.getTime()) ? null : parsed;
};

/** Soma dias a uma data ISO preservando o horário (meio-dia UTC nos inputs). */
export function addDaysISO(value: string, days: number): string | null {
  const parsed = parseDate(value);
  if (!parsed) return null;
  parsed.setUTCDate(parsed.getUTCDate() + days);
  return parsed.toISOString();
}

/** Rascunho do formulário de acordo — os mesmos campos do `CreateAgreementDTO`. */
export interface AgreementDraft {
  totalAmount: number | null | undefined;
  discount?: number | null;
  downPayment?: number | null;
  installments: number | string | null | undefined;
  firstDueDate: string;
  intervalDays?: number | string | null;
}

export interface AgreementPlan {
  totalAmount: number;
  discount: number;
  downPayment: number;
  /** `total_amount` − `discount`. */
  netAmount: number;
  /** O que é dividido nas parcelas: `netAmount` − `down_payment`. */
  financed: number;
  installments: number;
  intervalDays: number;
  firstDueDate: string;
  installmentValue: number;
  /** Última parcela absorve a diferença de centavos da divisão. */
  lastInstallmentValue: number;
}

/**
 * Prévia calculada no CLIENTE (o servidor continua sendo a verdade). Espelha a
 * validação do backend: valor total > 0, desconto < total, entrada menor que o
 * total menos o desconto, 1..36 parcelas e primeira parcela a partir de hoje.
 */
export function agreementDraftError(draft: AgreementDraft, today = todayISO()): string | null {
  const total = toAmount(draft.totalAmount);
  if (total === null || total <= 0) return 'Informe o valor total do acordo (maior que zero).';

  const discount = toAmount(draft.discount) ?? 0;
  if (discount < 0) return 'O desconto não pode ser negativo.';
  if (discount >= total) return 'O desconto não pode ser igual ou maior que o valor total do acordo.';

  const net = round2(total - discount);
  const downPayment = toAmount(draft.downPayment) ?? 0;
  if (downPayment < 0) return 'A entrada não pode ser negativa.';
  if (downPayment >= net) {
    return 'A entrada não pode ser igual ou maior que o valor total menos o desconto: não sobra valor para as parcelas.';
  }

  const installments = toInteger(draft.installments);
  if (installments === null || installments < 1 || installments > 36) {
    return 'O número de parcelas deve ser um inteiro entre 1 e 36.';
  }

  if (!draft.firstDueDate) return 'Informe a data da primeira parcela.';
  if (draft.firstDueDate < today) return 'A data da primeira parcela não pode estar no passado.';

  const interval = draft.intervalDays === undefined || draft.intervalDays === null || draft.intervalDays === ''
    ? 30
    : toInteger(draft.intervalDays);
  if (interval === null || interval < 1) return 'O intervalo entre parcelas deve ser de pelo menos 1 dia.';

  return null;
}

/** Plano do acordo para exibição — `null` quando ainda faltam dados essenciais. */
export function agreementPlan(draft: AgreementDraft): AgreementPlan | null {
  const total = toAmount(draft.totalAmount);
  const installments = toInteger(draft.installments);
  if (total === null || total <= 0 || installments === null || installments < 1) return null;

  const discount = Math.max(toAmount(draft.discount) ?? 0, 0);
  const downPayment = Math.max(toAmount(draft.downPayment) ?? 0, 0);
  const netAmount = round2(total - discount);
  const financed = round2(Math.max(netAmount - downPayment, 0));
  const intervalDays = toInteger(draft.intervalDays) ?? 30;
  const installmentValue = round2(financed / installments);

  return {
    totalAmount: total,
    discount,
    downPayment,
    netAmount,
    financed,
    installments,
    intervalDays,
    firstDueDate: draft.firstDueDate,
    installmentValue,
    lastInstallmentValue: round2(financed - installmentValue * (installments - 1)),
  };
}

/** "4x de R$ 1.000,00 + entrada de R$ 500,00, primeira em 10/11/2026". */
export function agreementPreviewText(plan: AgreementPlan): string {
  const parts = [`${plan.installments}x de ${formatCurrencyBRL(plan.installmentValue)}`];
  if (plan.downPayment > 0) parts.push(`entrada de ${formatCurrencyBRL(plan.downPayment)}`);
  return `${parts.join(' + ')}, primeira em ${formatDate(plan.firstDueDate)}`;
}

export interface AgreementInstallment {
  number: number;
  dueDate: string;
  amount: number;
  chargeId?: string;
}

/**
 * Parcelas do acordo REGISTRADO. O servidor devolve os ids das cobranças
 * geradas (`installment_charge_ids`) e as regras do plano; o valor/dia de cada
 * parcela é derivado aqui para exibição ("previsto") — nenhum número é
 * apresentado como se viesse do servidor.
 */
export function agreementSchedule(
  agreement: Pick<
    CollectionAgreement,
    'first_due_date' | 'installments' | 'interval_days' | 'total_amount' | 'discount' | 'down_payment' | 'installment_charge_ids'
  >,
): AgreementInstallment[] {
  const installments = toInteger(agreement.installments);
  if (installments === null || installments < 1) return [];
  const financed = round2((agreement.total_amount ?? 0) - (agreement.discount ?? 0) - (agreement.down_payment ?? 0));
  const value = round2(financed / installments);
  const rows: AgreementInstallment[] = [];
  for (let index = 0; index < installments; index += 1) {
    const dueDate = addDaysISO(agreement.first_due_date, index * (agreement.interval_days || 30));
    if (!dueDate) break;
    rows.push({
      number: index + 1,
      dueDate,
      amount: index === installments - 1 ? round2(financed - value * (installments - 1)) : value,
      chargeId: agreement.installment_charge_ids?.[index],
    });
  }
  return rows;
}

/**
 * Número do processo: mesma regra do servidor (`collection.ValidateCNJ`) —
 * máscara `NNNNNNN-DD.AAAA.J.TR.OOOO` ou exatamente 20 dígitos.
 */
export function isValidCNJ(value: string): boolean {
  const trimmed = (value ?? '').trim();
  if (/^\d{7}-\d{2}\.\d{4}\.\d\.\d{2}\.\d{4}$/.test(trimmed)) return true;
  return /^\d{20}$/.test(trimmed);
}

/** Valor monetário/metrica do servidor; ausência vira "—", nunca zero inventado. */
export function formatMetric(value: number | null | undefined, currency = false): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '—';
  return currency ? formatCurrencyBRL(value) : String(value);
}

export function formatPercent(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '—';
  return `${new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 1 }).format(value)}%`;
}

export function formatDate(value: string | null | undefined): string {
  if (!value) return '—';
  return formatISODateBR(value) || '—';
}

export function formatDateTime(value: string | null | undefined): string {
  if (!value) return '—';
  return formatDateTimeBR(value) || '—';
}

/**
 * `PUT /collections/:id` grava `notes` por sobrescrita quando o campo vem
 * preenchido. Para não apagar o texto que já estava no caso (e que o usuário
 * não está editando), o texto novo é ANEXADO ao existente.
 */
export function mergeNotes(existing: string | null | undefined, added: string | null | undefined): string | undefined {
  const base = (existing ?? '').trim();
  const extra = (added ?? '').trim();
  if (!extra) return undefined;
  if (!base) return extra;
  if (base.includes(extra)) return undefined;
  return `${base}\n\n${extra}`;
}

/** Data de hoje em ISO (yyyy-mm-dd) para defaults de formulário. */
export function todayISO(): string {
  return new Date().toISOString().slice(0, 10);
}

/** Classes dos campos dos painéis — mesma linguagem visual do restante do app. */
export const LABEL_CLASS = 'block text-sm font-bold text-foreground mb-1.5';
export const FIELD_CLASS =
  'w-full px-4 py-2.5 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none';
export const TEXTAREA_CLASS = `${FIELD_CLASS} resize-none py-3`;
export const FOOTER_CLASS = 'px-6 py-4 border-t border-border flex gap-3 bg-muted';
export const BODY_CLASS = 'flex-1 overflow-y-auto p-6 space-y-6';

/** Converte yyyy-mm-dd (input date) no ISO completo aceito pelo backend. */
export function dateInputToISO(value: string): string | undefined {
  if (!value) return undefined;
  const parsed = new Date(`${value}T12:00:00Z`);
  if (Number.isNaN(parsed.getTime())) return undefined;
  return parsed.toISOString();
}

/**
 * Transições válidas do caso — espelha `collection.CanTransitionTo` para avisar
 * o usuário ANTES de enviar; a resposta do servidor continua sendo a verdade.
 */
export function canTransitionTo(from: CollectionStatus, to: CollectionStatus): boolean {
  if (from === to) return true;
  switch (from) {
    case 'ACTIVE':
      return to === 'NEGOTIATED' || to === 'GUARANTEE_ACTIVATED' || to === 'JUDICIAL' || to === 'RESOLVED';
    case 'NEGOTIATED':
      return to === 'GUARANTEE_ACTIVATED' || to === 'JUDICIAL' || to === 'RESOLVED';
    case 'GUARANTEE_ACTIVATED':
      return to === 'JUDICIAL' || to === 'RESOLVED';
    case 'JUDICIAL':
      return to === 'RESOLVED';
    default:
      return false;
  }
}

/**
 * `PUT /collections/:id` grava `negotiated_amount`/`negotiated_date` por
 * sobrescrita direta (o handler atribui o ponteiro, não só quando vem
 * preenchido). Um PUT de mudança de status sem esses campos APAGA a negociação
 * registrada; por isso toda mutação que não é de negociação reenvia os valores
 * que o caso já tinha.
 */
export function negotiationFields(collection: {
  negotiated_amount?: number;
  negotiated_date?: string;
} | null): { negotiated_amount?: number; negotiated_date?: string } {
  if (!collection || collection.negotiated_amount === undefined) return {};
  return {
    negotiated_amount: collection.negotiated_amount,
    negotiated_date: collection.negotiated_date,
  };
}
