import type {
  BrokerParticipation,
  BrokerParticipationRole,
} from "../../../../shared/services/deal.service";

export const COMMISSION_SPLIT_ROLE_ORDER: BrokerParticipationRole[] = [
  "captador",
  "co_captador",
  "visitador",
  "atendente",
  "fechamento",
  "imobiliaria",
];

export const FALLBACK_COMMISSION_ROLE_PERCENTAGES: Record<BrokerParticipationRole, number> = {
  captador: 30,
  co_captador: 0,
  visitador: 20,
  atendente: 0,
  fechamento: 50,
  imobiliaria: 0,
};

export type CommissionRolePercentages = Partial<Record<BrokerParticipationRole, number>>;

export interface CommissionSplitEditorRow {
  rowKey: string;
  role: BrokerParticipationRole;
  brokerId: string;
  percentage: number;
}

export interface CommissionSplitAutofillParticipant {
  brokerId: string;
  role: BrokerParticipationRole;
  source: "property" | "lead";
}

export interface CommissionSplitRowsEvaluation {
  sum: number;
  sumOk: boolean;
  hasEmptyBroker: boolean;
  hasDuplicateParticipant: boolean;
  hasRowsChanged: boolean;
  canSaveWithoutCommissionChange: boolean;
}

const PERCENTAGE_TOLERANCE = 0.005;
const SUM_TOLERANCE = 0.02;

const PROPERTY_COMMISSION_ROLES = new Set<BrokerParticipationRole>([
  "captador",
  "co_captador",
]);

/**
 * Resolve as fontes canônicas do editor:
 * - os participantes registrados na captação do imóvel mantêm seus papéis;
 * - o atendente atual do lead ocupa o papel de fechamento.
 *
 * A deduplicação é por corretor + papel. O mesmo corretor pode participar em
 * dois papéis diferentes, o que é aceito pelo contrato do backend.
 */
export function buildCommissionSplitAutofillParticipants(
  propertyCapturingBrokers: Array<{ id?: string; role?: string }>,
  leadAssignedBrokerId?: string,
): CommissionSplitAutofillParticipant[] {
  const participants: CommissionSplitAutofillParticipant[] = [];
  const seen = new Set<string>();

  const append = (participant: CommissionSplitAutofillParticipant) => {
    const brokerId = participant.brokerId.trim();
    if (!brokerId) return;
    const key = `${brokerId}\u001f${participant.role}`;
    if (seen.has(key)) return;
    seen.add(key);
    participants.push({ ...participant, brokerId });
  };

  for (const broker of propertyCapturingBrokers) {
    const role = broker.role as BrokerParticipationRole | undefined;
    if (!role || !PROPERTY_COMMISSION_ROLES.has(role)) continue;
    append({ brokerId: broker.id ?? "", role, source: "property" });
  }

  append({
    brokerId: leadAssignedBrokerId ?? "",
    role: "fechamento",
    source: "lead",
  });

  return participants;
}

/**
 * Negócios criados pelo fluxo antigo recebiam apenas os participantes da
 * captação do imóvel, normalizados para 100%. Quando encontramos exatamente
 * esse formato e já conhecemos o atendente do lead, reconstruímos o plano com
 * as regras da imobiliária. Um rateio completo/editado nunca é sobrescrito.
 */
export function shouldRebuildCommissionSplitFromAutofill(
  existingParticipations: BrokerParticipation[],
  autofillParticipants: CommissionSplitAutofillParticipant[],
): boolean {
  if (existingParticipations.length === 0) return false;

  const closing = autofillParticipants.find(
    (participant) => participant.source === "lead" && participant.role === "fechamento",
  );
  if (!closing) return false;

  const alreadyHasClosing = existingParticipations.some(
    (participation) =>
      participation.role === "fechamento" &&
      participation.broker_id === closing.brokerId,
  );
  if (alreadyHasClosing) return false;

  const propertyKeys = new Set(
    autofillParticipants
      .filter((participant) => participant.source === "property")
      .map((participant) => `${participant.brokerId}\u001f${participant.role}`),
  );
  const nonAgencyParticipations = existingParticipations.filter(
    (participation) => participation.role !== "imobiliaria",
  );

  return (
    nonAgencyParticipations.length > 0 &&
    nonAgencyParticipations.every((participation) =>
      propertyKeys.has(`${participation.broker_id}\u001f${participation.role}`),
    )
  );
}

function splitRolePercentage(total: number, count: number, index: number): number {
  if (count <= 1) return total;
  const equalShare = Math.floor((total / count) * 10_000) / 10_000;
  return index === count - 1
    ? Math.round((total - equalShare * (count - 1)) * 10_000) / 10_000
    : equalShare;
}

export function resolveCommissionRoleDefault(
  role: BrokerParticipationRole,
  rolePercentages: CommissionRolePercentages,
): number {
  if (rolePercentages[role] != null) return rolePercentages[role];
  // When the caller provided any configuration at all, a role left out of it
  // is intentionally not part of the split (get 0), rather than inheriting the
  // generic fallback and silently inflating the sum above 100.
  const hasAnyConfig = Object.keys(rolePercentages).length > 0;
  return hasAnyConfig ? 0 : FALLBACK_COMMISSION_ROLE_PERCENTAGES[role];
}

/**
 * Constrói uma linha para cada participação existente, inclusive quando duas
 * ou mais participações têm o mesmo papel. Para papéis ausentes, acrescenta
 * uma linha vazia com o percentual default configurado.
 */
export function buildCommissionSplitEditorRows(
  existingParticipations: BrokerParticipation[],
  rolePercentages: CommissionRolePercentages,
  autofillParticipants: CommissionSplitAutofillParticipant[] = [],
): CommissionSplitEditorRow[] {
  const rebuildFromAutofill = shouldRebuildCommissionSplitFromAutofill(
    existingParticipations,
    autofillParticipants,
  );
  const effectiveParticipations = rebuildFromAutofill
    ? []
    : existingParticipations;
  const editingExistingDistribution = effectiveParticipations.length > 0;

  return COMMISSION_SPLIT_ROLE_ORDER.flatMap((role) => {
    const roleParticipations = effectiveParticipations.filter(
      (participation) => participation.role === role,
    );

    if (roleParticipations.length === 0) {
      const suggestedParticipants = editingExistingDistribution
        ? []
        : autofillParticipants.filter((participant) => participant.role === role);
      if (suggestedParticipants.length > 0) {
        const rolePercentage = resolveCommissionRoleDefault(role, rolePercentages);
        return suggestedParticipants.map((participant, participantIndex) => ({
          rowKey: `autofill:${role}:${participantIndex}:${participant.brokerId}`,
          role,
          brokerId: participant.brokerId,
          percentage: splitRolePercentage(
            rolePercentage,
            suggestedParticipants.length,
            participantIndex,
          ),
        }));
      }

      return [{
        rowKey: `default:${role}`,
        role,
        brokerId: "",
        percentage: editingExistingDistribution
          ? 0
          : resolveCommissionRoleDefault(role, rolePercentages),
      }];
    }

    return roleParticipations.map((participation, roleIndex) => ({
      rowKey: `existing:${role}:${roleIndex}`,
      role,
      brokerId: participation.broker_id ?? "",
      percentage:
        participation.percentage ??
        resolveCommissionRoleDefault(role, rolePercentages),
    }));
  });
}

export function haveCommissionSplitRowsChanged(
  rows: CommissionSplitEditorRow[],
  existingParticipations: BrokerParticipation[],
  rolePercentages: CommissionRolePercentages,
): boolean {
  const initialRows = buildCommissionSplitEditorRows(
    existingParticipations,
    rolePercentages,
  );

  if (rows.length !== initialRows.length) return true;

  return rows.some((row, index) => {
    const initial = initialRows[index];
    if (!initial) return true;

    return (
      row.role !== initial.role ||
      row.brokerId !== initial.brokerId ||
      Math.abs(row.percentage - initial.percentage) > PERCENTAGE_TOLERANCE
    );
  });
}

export function evaluateCommissionSplitRows(
  rows: CommissionSplitEditorRow[],
  existingParticipations: BrokerParticipation[],
  rolePercentages: CommissionRolePercentages,
): CommissionSplitRowsEvaluation {
  const sum = rows.reduce((total, row) => total + row.percentage, 0);
  const sumOk = Math.abs(sum - 100) < SUM_TOLERANCE;
  const hasEmptyBroker = rows.some(
    (row) => row.role !== "imobiliaria" && row.percentage > 0 && !row.brokerId,
  );
  const participantKeys = new Set<string>();
  const hasDuplicateParticipant = rows.some((row) => {
    if (row.role === "imobiliaria" || row.percentage <= 0 || !row.brokerId) return false;
    const key = `${row.brokerId.trim()}\u001f${row.role}`;
    if (participantKeys.has(key)) return true;
    participantKeys.add(key);
    return false;
  });
  const hasRowsChanged = haveCommissionSplitRowsChanged(
    rows,
    existingParticipations,
    rolePercentages,
  );

  return {
    sum,
    sumOk,
    hasEmptyBroker,
    hasDuplicateParticipant,
    hasRowsChanged,
    canSaveWithoutCommissionChange:
      sumOk && !hasEmptyBroker && !hasDuplicateParticipant && hasRowsChanged,
  };
}

export function serializeCommissionSplitRows(
  rows: CommissionSplitEditorRow[],
): BrokerParticipation[] {
  return rows
    .filter(
      (row) => row.percentage > 0 && (row.role === "imobiliaria" || row.brokerId),
    )
    .map((row) => ({
      broker_id: row.brokerId,
      role: row.role,
      percentage: row.percentage,
    }));
}
