import type { SignatureEnvelope, SignatureEnvelopeSigner } from "../../contracts/types";

/**
 * Regras do estado PARCIAL da assinatura.
 *
 * O contrato tem três estados de assinatura e a interface só cobria dois:
 * rascunho/enviado (`PENDING_SIGNATURE`) e assinado (`SIGNED`). No meio existe
 * `PARTIALLY_SIGNED` — o contrato vira esse estado no primeiro signatário que
 * assina (toca-deal promove em `MarkSignerSignedExternally`) e permanece nele até
 * o último assinar. Com grupos de assinatura o estado parcial é ainda mais
 * longo: o Grupo 1 assina, o Grupo 2 só é notificado depois, e nesse intervalo a
 * tela ficava sem representação nenhuma — o polling parava, o selo caía no
 * fallback e o operador não sabia quem já assinou nem quem ainda nem recebeu o
 * pedido.
 *
 * Aqui ficam as regras em um lugar só (funções puras, testadas), para a página
 * do contrato, o painel de assinatura e as listas contarem a mesma história.
 */

/** Estados em que a assinatura está EM ANDAMENTO (envelope existe e pode avançar). */
export const SIGNATURE_IN_FLIGHT_STATUSES = ["DRAFT", "PENDING_SIGNATURE", "PARTIALLY_SIGNED"] as const;

/** Estados do envelope em que ainda há algo para assinar/sincronizar. */
export const ENVELOPE_ACTIVE_STATUSES = ["PENDING", "DRAFT", "RUNNING", "PARTIALLY_SIGNED"] as const;

/** Estados terminais do envelope — nada mais vai acontecer sem nova tentativa. */
export const ENVELOPE_TERMINAL_STATUSES = ["FINISHED", "REFUSED", "CANCELED", "EXPIRED", "ERROR"] as const;

export function isSignatureInFlight(status: string | null | undefined): boolean {
  return SIGNATURE_IN_FLIGHT_STATUSES.includes((status ?? "").toUpperCase() as (typeof SIGNATURE_IN_FLIGHT_STATUSES)[number]);
}

export function isEnvelopeActive(status: string | null | undefined): boolean {
  return ENVELOPE_ACTIVE_STATUSES.includes((status ?? "").toUpperCase() as (typeof ENVELOPE_ACTIVE_STATUSES)[number]);
}

export function isEnvelopeTerminal(status: string | null | undefined): boolean {
  return ENVELOPE_TERMINAL_STATUSES.includes((status ?? "").toUpperCase() as (typeof ENVELOPE_TERMINAL_STATUSES)[number]);
}

/**
 * Vez de cada signatário.
 *
 * `WAITING_TURN` é o que faltava: um signatário de grupo posterior está
 * pendente, mas NÃO foi notificado — a Clicksign só convida o grupo da vez. Sem
 * essa distinção ele aparecia igual a quem já recebeu o pedido e não assinou.
 */
export type SignerTurn = "SIGNED" | "REFUSED" | "SIGNING" | "WAITING_TURN" | "PENDING";

export interface SignatureProgress {
  total: number;
  signed: number;
  refused: number;
  pending: number;
  /** Há ordem de assinatura configurada (algum signatário com grupo > 0). */
  hasSigningOrder: boolean;
  /** Grupo assinando agora (menor grupo com alguém pendente); 0 = nenhum. */
  activeGroup: number;
  /** Grupos que ainda aguardam a vez (maiores que o grupo ativo). */
  waitingGroups: number[];
  /** Ordem configurada e nenhum grupo pendente. */
  allGroupsDone: boolean;
}

const emptyProgress: SignatureProgress = {
  total: 0,
  signed: 0,
  refused: 0,
  pending: 0,
  hasSigningOrder: false,
  activeGroup: 0,
  waitingGroups: [],
  allGroupsDone: false,
};

function normalize(status: string | undefined): string {
  return (status ?? "").toUpperCase();
}

function groupOf(signer: Pick<SignatureEnvelopeSigner, "group">): number {
  return signer.group != null && signer.group > 0 ? signer.group : 0;
}

export function signatureProgress(envelope: SignatureEnvelope | null | undefined): SignatureProgress {
  const signers = envelope?.signers ?? [];
  if (signers.length === 0) return emptyProgress;

  let signed = 0;
  let refused = 0;
  const pendingGroups = new Set<number>();
  for (const signer of signers) {
    const status = normalize(signer.status);
    if (status === "SIGNED") {
      signed += 1;
      continue;
    }
    if (status === "REFUSED") {
      refused += 1;
      continue;
    }
    const group = groupOf(signer);
    if (group > 0) pendingGroups.add(group);
  }

  const hasSigningOrder = signers.some((signer) => groupOf(signer) > 0);
  const activeGroup = pendingGroups.size === 0 ? 0 : Math.min(...pendingGroups);
  const waitingGroups = [...pendingGroups].filter((group) => group > activeGroup).sort((a, b) => a - b);

  return {
    total: signers.length,
    signed,
    refused,
    pending: signers.length - signed - refused,
    hasSigningOrder,
    activeGroup,
    waitingGroups,
    allGroupsDone: hasSigningOrder && pendingGroups.size === 0,
  };
}

export function signerTurnState(
  envelope: SignatureEnvelope | null | undefined,
  signer: { status?: string; group?: number } | undefined,
): SignerTurn {
  if (!signer) return "PENDING";
  const status = normalize(signer.status);
  if (status === "SIGNED") return "SIGNED";
  if (status === "REFUSED") return "REFUSED";

  const progress = signatureProgress(envelope);
  const group = signer.group != null && signer.group > 0 ? signer.group : 0;
  if (group > 0 && progress.activeGroup > 0 && group > progress.activeGroup) return "WAITING_TURN";
  if (group > 0 && group === progress.activeGroup) return "SIGNING";
  return "PENDING";
}

/** "1 de 2 assinaram" — a leitura que o operador procura no estado parcial. */
export function signatureProgressLabel(progress: SignatureProgress): string {
  if (progress.total === 0) return "Nenhum signatário no envelope";
  const verb = progress.signed === 1 ? "assinou" : "assinaram";
  return `${progress.signed} de ${progress.total} ${verb}`;
}

/** Texto do grupo que está com a vez. */
export function activeGroupLabel(progress: SignatureProgress): string | null {
  if (!progress.hasSigningOrder) return null;
  if (progress.activeGroup > 0) return `Grupo ${progress.activeGroup} em andamento`;
  return "Todos os grupos concluídos";
}

/** Texto do signatário que ainda não foi convidado. */
export function waitingTurnLabel(group: number, activeGroup: number): string {
  if (activeGroup > 0) return `Aguardando o Grupo ${activeGroup}`;
  return group > 0 ? `Aguardando o Grupo ${group}` : "Aguardando assinatura";
}
