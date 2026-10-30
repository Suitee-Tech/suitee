/**
 * Rótulo de VENCIMENTO de uma cobrança — a régua "vence no dia, atrasa no dia
 * seguinte".
 *
 * Por que existe: o backend marcava a cobrança como OVERDUE no próprio dia do
 * vencimento (o cálculo antigo era `horas / 24` a partir do instante do
 * vencimento, e a data é gravada como meia-noite UTC — às 21h da véspera em
 * Brasília a cobrança do dia seguinte já contava atraso). Resultado em produção:
 * uma conta que vence HOJE aparecia como "Vencido" no financeiro. A regra do
 * produto é de CALENDÁRIO, não de relógio: a cobrança vence NO DIA e só é
 * considerada vencida no dia seguinte — por isso existe "Vence hoje" (tone
 * `today`, atenção) e o atraso só começa em "Venceu ontem" (tone `overdue`,
 * crítico). Sem essa distinção o operador não consegue priorizar: a cobrança do
 * dia entrava na mesma pilha das que já estão atrasadas há semanas.
 *
 * Fonte do número: `days_overdue` do BACKEND (calculado em `pkg/timeutil` no
 * fuso do produto, que pode ser o do tenant) — o cliente NÃO recalcula a régua
 * para não criar uma segunda verdade. O campo é opcional no payload; quando não
 * vem (resposta antiga, lista mockada), derivamos do `due_date`: a data de
 * vencimento é gravada como o DIA em UTC ("2026-10-10T00:00:00Z" significa
 * "vence em 10/10", e é assim que a tela mostra 10/10), enquanto "hoje" é o dia
 * civil do navegador — exatamente a conta que `Charge.OverdueDays` faz no
 * servidor.
 *
 * Status fechado (PAID/CANCELLED/REVERSED) nunca é atrasado: a cobrança saiu da
 * régua de cobrança, então o rótulo é o do próprio status.
 */

const MS_PER_DAY = 86_400_000;

export type ChargeDueTone = 'overdue' | 'today' | 'upcoming' | 'settled' | 'unknown';

export interface ChargeDueLabel {
  /** Texto do badge ("Vence hoje", "Venceu há 12 dias", "Pago"...). */
  label: string;
  /** Tom do badge — ver `CHARGE_DUE_TONE_CLASS`. */
  tone: ChargeDueTone;
  /** Dias de atraso pela régua do produto (0 enquanto não passou do dia). */
  daysOverdue: number;
  /** `true` somente com 1 dia ou mais de atraso — vence hoje NÃO é atraso. */
  overdue: boolean;
}

export interface ChargeDueInput {
  /** Data de vencimento (`yyyy-mm-dd` ou RFC3339; o DIA é lido em UTC). */
  due_date?: string | null;
  /** Status bruto do backend (`PENDING`, `PAID`...) ou normalizado (`vencido`). */
  status?: string | null;
  /** Campo calculado pelo backend; ausente em payload antigo/lista mockada. */
  days_overdue?: number | null;
  /** Injetável para teste; por padrão, agora. */
  now?: Date;
}

/**
 * Estilo do badge por tom, em partes (fundo/texto/borda) porque os badges do
 * produto montam a classe de formas diferentes. Tokens do design system — nada
 * de cor literal.
 */
export interface ChargeDueToneStyle {
  bg: string;
  text: string;
  border: string;
}

export const CHARGE_DUE_TONE_STYLE: Record<ChargeDueTone, ChargeDueToneStyle> = {
  overdue: { bg: 'bg-status-critical-soft', text: 'text-status-critical', border: 'border-status-critical/30' },
  today: { bg: 'bg-status-warning-soft', text: 'text-status-warning', border: 'border-status-warning/30' },
  upcoming: { bg: 'bg-status-info-soft', text: 'text-status-info', border: 'border-status-info/30' },
  settled: { bg: 'bg-status-ok-soft', text: 'text-status-ok', border: 'border-status-ok/30' },
  unknown: { bg: 'bg-muted', text: 'text-muted-foreground', border: 'border-border' },
};

const toneClass = ({ bg, text, border }: ChargeDueToneStyle): string => `${bg} ${text} border ${border}`;

/** Classe única (fundo + texto + borda) para badges que não separam as partes. */
export const CHARGE_DUE_TONE_CLASS: Record<ChargeDueTone, string> = {
  overdue: toneClass(CHARGE_DUE_TONE_STYLE.overdue),
  today: toneClass(CHARGE_DUE_TONE_STYLE.today),
  upcoming: toneClass(CHARGE_DUE_TONE_STYLE.upcoming),
  settled: toneClass(CHARGE_DUE_TONE_STYLE.settled),
  unknown: toneClass(CHARGE_DUE_TONE_STYLE.unknown),
};

const CLOSED_LABEL = {
  PAID: 'Pago',
  CANCELLED: 'Cancelada',
  REVERSED: 'Reversada',
} as const;

/** Status em que a cobrança saiu da régua — atraso não se aplica. */
const CLOSED_STATUS: Record<string, keyof typeof CLOSED_LABEL> = {
  PAID: 'PAID',
  PAGO: 'PAID',
  PAGA: 'PAID',
  RECEBIDO: 'PAID',
  RECEBIDA: 'PAID',
  CANCELLED: 'CANCELLED',
  CANCELED: 'CANCELLED',
  CANCELADO: 'CANCELLED',
  CANCELADA: 'CANCELLED',
  REVERSED: 'REVERSED',
  REVERTIDO: 'REVERSED',
  REVERTIDA: 'REVERSED',
  REVERSADO: 'REVERSED',
  REVERSADA: 'REVERSED',
  ESTORNADO: 'REVERSED',
  ESTORNADA: 'REVERSED',
};

/** Rótulo do status quando não há data para dizer quando vence/venceu. */
const OPEN_STATUS_LABEL: Record<string, string> = {
  PENDING: 'Pendente',
  PENDENTE: 'Pendente',
  OVERDUE: 'Vencido',
  VENCIDO: 'Vencido',
  VENCIDA: 'Vencido',
  'EM ATRASO': 'Vencido',
  PARTIALLY_PAID: 'Parcial',
  PARCIAL: 'Parcial',
};

const normalize = (status: string | null | undefined): string => (status ?? '').trim().toUpperCase();

/** Meia-noite UTC do dia civil da data — o mesmo dia que a tela mostra. */
function dueCivilDay(dueDate: string | null | undefined): number | null {
  const value = (dueDate ?? '').trim();
  if (!value) return null;
  // Mesma leitura de `formatISODateBR`: sem hora, a data é o dia em UTC.
  const parsed = new Date(value.includes('T') ? value : `${value}T00:00:00Z`);
  if (Number.isNaN(parsed.getTime())) return null;
  return Date.UTC(parsed.getUTCFullYear(), parsed.getUTCMonth(), parsed.getUTCDate());
}

/** Meia-noite local do dia civil do navegador, ancorada em UTC para a conta. */
function todayCivilDay(now: Date): number {
  return Date.UTC(now.getFullYear(), now.getMonth(), now.getDate());
}

const lateLabel = (days: number): string => (days === 1 ? 'Venceu ontem' : `Venceu há ${days} dias`);

/**
 * Texto + tom do vencimento de uma cobrança.
 *
 * Precedência: `days_overdue` do backend manda no ATRASO (ele conhece o fuso do
 * produto/tenant); a data de vencimento decide o resto ("vence hoje" vs "vence
 * em N dias"), porque o campo do backend é 0 nos dois casos. Sem data e sem o
 * campo, sobra o rótulo do status — a tela nunca inventa um prazo.
 */
export function chargeDueLabel({
  due_date,
  status,
  days_overdue,
  now = new Date(),
}: ChargeDueInput): ChargeDueLabel {
  const normalizedStatus = normalize(status);
  const closed = CLOSED_STATUS[normalizedStatus];
  if (closed) {
    return { label: CLOSED_LABEL[closed], tone: 'settled', daysOverdue: 0, overdue: false };
  }

  const backendDays =
    typeof days_overdue === 'number' && Number.isFinite(days_overdue) ? Math.trunc(days_overdue) : null;

  // O backend já disse que há atraso: é a mesma régua da cobrança, não recalcule.
  if (backendDays !== null && backendDays >= 1) {
    return { label: lateLabel(backendDays), tone: 'overdue', daysOverdue: backendDays, overdue: true };
  }

  const due = dueCivilDay(due_date);
  if (due !== null) {
    const diff = Math.round((todayCivilDay(now) - due) / MS_PER_DAY);
    if (diff >= 1) {
      // Divergência de fuso entre o navegador e o produto: com `days_overdue`
      // presente e zero, o servidor é a autoridade — a cobrança ainda está no
      // dia dela e não pode aparecer como atrasada.
      if (backendDays === 0) {
        return { label: 'Vence hoje', tone: 'today', daysOverdue: 0, overdue: false };
      }
      return { label: lateLabel(diff), tone: 'overdue', daysOverdue: diff, overdue: true };
    }
    if (diff === 0) {
      return { label: 'Vence hoje', tone: 'today', daysOverdue: 0, overdue: false };
    }
    const ahead = -diff;
    return {
      label: ahead === 1 ? 'Vence amanhã' : `Vence em ${ahead} dias`,
      tone: 'upcoming',
      daysOverdue: 0,
      overdue: false,
    };
  }

  return {
    label: OPEN_STATUS_LABEL[normalizedStatus] ?? 'Pendente',
    tone: 'unknown',
    daysOverdue: 0,
    overdue: false,
  };
}

/** Só o texto — atalho para quem já resolve o tom em outro lugar. */
export function chargeDueLabelText(input: ChargeDueInput): string {
  return chargeDueLabel(input).label;
}
