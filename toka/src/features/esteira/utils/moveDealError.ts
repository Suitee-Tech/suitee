import type { DealDTO, DealStatus, PartyDTO } from "../../../shared/services/deal.service";
import { getStageMeta } from "../../../shared/deals/dealStages";
import { partyRoleLabel } from "../../../shared/deals/partyLabels";

const GENERIC = "Não foi possível mover o negócio. Tente novamente.";

function stageLabel(status: DealStatus): string {
  try {
    return getStageMeta(status).label;
  } catch {
    return "a próxima etapa";
  }
}

function findParty(deal: DealDTO | undefined, partyId: string): PartyDTO | undefined {
  return deal?.parties?.find((p) => p.id === partyId);
}

function partyName(party: PartyDTO | undefined): string {
  return (party?.name ?? "").trim() || (party?.email ?? "").trim();
}

/**
 * Traduz a mensagem crua de erro do backend (retornada ao mover um negócio de
 * etapa na esteira) para uma frase semântica em PT-BR, nomeando a parte e o
 * dado que falta sempre que possível.
 *
 * As sentinelas do backend são envolvidas com `%w` (ex.:
 * `required parties have incomplete data ...: party_id=<uuid> role=tenant missing=document`),
 * então o casamento é por PREFIXO/SUBSTRING — nunca por igualdade exata.
 * Mensagens não reconhecidas são retornadas como vieram, preservando o
 * comportamento atual de surfacing.
 */
export function moveDealErrorMessage(
  rawMessage: string,
  deal: DealDTO | undefined,
  targetStatus: DealStatus,
): string {
  const raw = (rawMessage ?? "").trim();
  if (!raw) return GENERIC;

  // Partes obrigatórias incompletas (nome/e-mail na entrada em Análise; CPF/CNPJ ao ir para Contrato).
  if (raw.startsWith("required parties have incomplete data")) {
    const m = raw.match(/party_id=(\S+)\s+role=([a-z_]+)\s+missing=([a-zA-Z/]+)/);
    const role = m?.[2] ?? "";
    const missing = m?.[3] ?? "";
    const roleLabel = role ? partyRoleLabel(role).toLowerCase() : "uma parte";
    const name = m ? partyName(findParty(deal, m[1])) : "";
    const subject = name ? `${name} (${roleLabel})` : `uma parte obrigatória (${roleLabel})`;
    const missingLabel = missing.includes("document") ? "o CPF/CNPJ" : "o nome ou e-mail";
    return `Não é possível avançar para ${stageLabel(targetStatus)}: falta ${missingLabel} de ${subject}. Complete os dados no painel de Partes do negócio.`;
  }

  // Proposta ainda não aceita por todas as partes obrigatórias.
  if (raw.startsWith("proposal must be accepted")) {
    return "A proposta precisa ser aceita por todas as partes obrigatórias antes de avançar.";
  }

  // Cônjuge não cadastrado para uma parte primária casada.
  if (raw.startsWith("spouse required before advancing")) {
    const m = raw.match(/principal_party_id=(\S+)/);
    const name = m ? partyName(findParty(deal, m[1])) : "";
    const who = name || "uma parte primária";
    return `Cadastre o cônjuge de ${who} (ou marque "Sem cônjuge" no painel de Partes) antes de avançar.`;
  }

  // Valor do negócio acima do limite que exige aprovação.
  if (raw.startsWith("deal value exceeds approval threshold")) {
    return "Este negócio excede o limite de aprovação por causa do valor. É necessária a permissão de aprovação (deals:approve) — solicite a um gestor ou administrador.";
  }

  // Gate de checklist por tenant: itens obrigatórios precisam estar aprovados.
  if (raw.includes("required checklist items must be approved")) {
    return "Todos os itens obrigatórios do checklist precisam estar aprovados antes de mover para Contrato.";
  }

  // Assinaturas pendentes ao concluir o negócio.
  if (raw.includes("all required signers must sign")) {
    return "Todos os signatários obrigatórios precisam assinar antes de concluir.";
  }

  // Transição de etapa inválida (state machine).
  if (raw.includes("invalid status transition")) {
    return "Transição de etapa inválida.";
  }

  // Mensagem desconhecida: preserva o texto do backend.
  return raw;
}
