/**
 * Distribuição inicial de comissão de um negócio novo.
 *
 * Contexto do defeito que originou este módulo: o modal de criação de negócio
 * rateava a comissão apenas entre os corretores de captação do imóvel, com
 * `100 / quantidade` — regra que faz sentido para LOCAÇÃO (a corretagem da
 * locação pertence a quem captou), mas não para VENDA, onde a distribuição é
 * por papel (captador / fechamento / imobiliária) conforme as regras do tenant.
 *
 * Na venda, um imóvel com um único captador e sem percentual informado gerava
 * `[{captador, 100%}]`: o plano congelado na assinatura dava a comissão inteira
 * a um corretor, sem retenção da imobiliária, e as cobranças de venda nasciam
 * com o pool de comissão inteiro devido a ele.
 *
 * Regra atual (SEMPRE fecha 100%, e um negócio nunca nasce sem rateio):
 *  - VENDA: cada papel com corretor conhecido recebe o percentual configurado
 *    em Configurações › Comissões (o corretor de captação do imóvel para
 *    captador/co-captador/visitador/atendente; o corretor do atendimento, ou o
 *    responsável, para fechamento). O que não tem corretor identificado fica
 *    com a imobiliária, como receita retida — mesma leitura da própria tela de
 *    configuração ("os papéis somam X%; o restante é atribuído à imobiliária").
 *  - LOCAÇÃO: mantém o comportamento anterior (rateio do pool entre os
 *    captadores do imóvel); sem captador, a comissão fica retida na
 *    imobiliária.
 *  - Configuração indisponível ou impossível (soma dos papéis acima de 100%):
 *    a comissão é retida integralmente pela imobiliária, com aviso ao operador
 *    — nunca sobra para um corretor sem regra que o ampare.
 */

export type DealParticipationRole =
  | "captador"
  | "co_captador"
  | "visitador"
  | "atendente"
  | "fechamento"
  | "imobiliaria";

export interface DealBrokerParticipation {
  broker_id: string;
  role: DealParticipationRole;
  percentage: number;
}

export interface CapturingBrokerLike {
  id?: string | null;
  role?: string | null;
  commissionPct?: number | null;
}

export interface BuildDealParticipationsInput {
  dealType: "SALE" | "RENT";
  capturingBrokers?: CapturingBrokerLike[] | null;
  /** Corretor do atendimento/lead — ocupa o papel de fechamento. */
  closingBrokerId?: string | null;
  /** Responsável pelo negócio; usado quando não há corretor do lead. */
  responsibleBrokerId?: string | null;
  /** Regras de comissão por papel do tenant (`/commission-rules`). */
  rolePercentages?: Partial<Record<DealParticipationRole, number>> | null;
}

export interface BuiltDealParticipations {
  participations: DealBrokerParticipation[];
  /** Explica à UI por que o rateio não pôde ser montado (quando vazio). */
  warning?: string;
}

const ROLES: DealParticipationRole[] = [
  "captador",
  "co_captador",
  "visitador",
  "atendente",
  "fechamento",
  "imobiliaria",
];

/** Papéis que só existem com corretor identificado (a imobiliária é retenção). */
const BROKER_ROLES: DealParticipationRole[] = [
  "captador",
  "co_captador",
  "visitador",
  "atendente",
  "fechamento",
];

const TOLERANCE = 0.02;

/** Retenção integral: usada quando não há como distribuir com segurança. */
const AGENCY_ONLY: DealBrokerParticipation = { broker_id: "", role: "imobiliaria", percentage: 100 };

const cleanId = (value?: string | null): string => (value ?? "").trim();

const isRole = (value?: string | null): value is DealParticipationRole =>
  !!value && (ROLES as string[]).includes(value);

const roundToCents = (value: number): number => Math.round(value * 100) / 100;

const positive = (value?: number | null): number =>
  typeof value === "number" && Number.isFinite(value) && value > 0 ? value : 0;

export function buildDealBrokerParticipations(
  input: BuildDealParticipationsInput,
): BuiltDealParticipations {
  const capturingBrokers = (input.capturingBrokers ?? []).filter((broker) => cleanId(broker.id));

  if (input.dealType === "RENT") {
    const participations = buildRentalParticipations(capturingBrokers);
    if (participations.length > 0) return { participations };
    // Locação sem captador cadastrado: a comissão fica retida na imobiliária em
    // vez de o negócio nascer sem rateio.
    return {
      participations: [AGENCY_ONLY],
      warning:
        "Nenhum corretor de captação vinculado ao imóvel: a comissão ficou retida na imobiliária. Ajuste a captação ou o rateio do negócio antes de assinar.",
    };
  }

  const rules = input.rolePercentages ?? {};
  const hasBrokerRule = BROKER_ROLES.some((role) => positive(rules[role]) > 0);
  if (!hasBrokerRule) {
    // Configuração indisponível (leitura falhou ou papéis todos zerados): a
    // comissão fica retida na imobiliária e o operador é avisado. O rateio
    // existe e fecha 100% — o negócio nunca nasce sem distribuição.
    return {
      participations: [AGENCY_ONLY],
      warning:
        "Não foi possível carregar a distribuição de comissão de Configurações › Comissões: a comissão ficou retida na imobiliária. Revise o rateio do negócio antes de assinar.",
    };
  }

  const rows: DealBrokerParticipation[] = [];

  // Papéis preenchidos pelos corretores da captação do imóvel (o papel vem do
  // próprio cadastro do imóvel). O percentual da regra é dividido entre os
  // corretores do mesmo papel.
  for (const role of ["captador", "co_captador", "visitador", "atendente"] as DealParticipationRole[]) {
    const rolePct = positive(rules[role]);
    if (rolePct <= 0) continue;
    const brokers = capturingBrokers.filter((broker) => (broker.role ?? "captador") === role);
    if (brokers.length === 0) continue;
    const share = rolePct / brokers.length;
    for (const broker of brokers) {
      rows.push({ broker_id: cleanId(broker.id), role, percentage: share });
    }
  }

  const closingPct = positive(rules.fechamento);
  const closingBrokerId = cleanId(input.closingBrokerId) || cleanId(input.responsibleBrokerId);
  if (closingPct > 0 && closingBrokerId) {
    rows.push({ broker_id: closingBrokerId, role: "fechamento", percentage: closingPct });
  }

  const brokerSum = rows.reduce((sum, row) => sum + row.percentage, 0);
  if (brokerSum > 100 + TOLERANCE) {
    // Configuração impossível: nenhum corretor recebe acima do configurado e a
    // comissão fica retida até o operador corrigir os percentuais.
    return {
      participations: [AGENCY_ONLY],
      warning:
        "A soma das regras de comissão por papel passa de 100%: a comissão ficou retida na imobiliária. Revise Configurações › Comissões antes de assinar.",
    };
  }

  // A imobiliária fica com a regra própria e com o que não tem corretor
  // identificado — nunca com valor negativo, nunca acima de 100%.
  const agencyPct = Math.max(positive(rules.imobiliaria), 100 - brokerSum);
  if (agencyPct > 0) {
    rows.push({ broker_id: "", role: "imobiliaria", percentage: agencyPct });
  }

  return { participations: normalizeParticipationSum(rows) };
}

function buildRentalParticipations(capturingBrokers: CapturingBrokerLike[]): DealBrokerParticipation[] {
  if (capturingBrokers.length === 0) return [];
  const informedTotal = capturingBrokers.reduce(
    (sum, broker) => sum + Math.max(0, broker.commissionPct ?? 0),
    0,
  );
  const rows = capturingBrokers.map<DealBrokerParticipation>((broker) => ({
    broker_id: cleanId(broker.id),
    role: isRole(broker.role) ? broker.role : "captador",
    percentage:
      informedTotal > 0
        ? (Math.max(0, broker.commissionPct ?? 0) * 100) / informedTotal
        : 100 / capturingBrokers.length,
  }));
  return normalizeParticipationSum(rows);
}

/**
 * Arredonda para duas casas e fecha a soma em 100% na última linha, para o
 * backend (que exige soma 100% ± 0,02) não recusar o rateio por arredondamento.
 */
function normalizeParticipationSum(rows: DealBrokerParticipation[]): DealBrokerParticipation[] {
  if (rows.length === 0) return [];
  const rounded = rows.map((row) => ({ ...row, percentage: roundToCents(row.percentage) }));
  const sum = rounded.reduce((total, row) => total + row.percentage, 0);
  const drift = roundToCents(100 - sum);
  if (drift !== 0) {
    const last = rounded[rounded.length - 1];
    const adjusted = roundToCents(last.percentage + drift);
    if (adjusted > 0) {
      rounded[rounded.length - 1] = { ...last, percentage: adjusted };
    }
  }
  // Participação sem corretor só é válida para a imobiliária (retenção).
  return rounded.filter(
    (row) => row.percentage > 0 && (row.role === "imobiliaria" || row.broker_id !== ""),
  );
}
