/**
 * Onda 9 — "Operar conteúdo compartilhado".
 *
 * Helpers para transformar os payloads de conteúdo compartilhado
 * (SharedProperty / SharedLead) nas entidades de primeira-classe do app
 * (Property / Lead), marcando cada uma com `sharedFrom`.
 *
 * O backend devolve `source_company_name` VAZIO hoje — então o nome da
 * empresa dona é resolvido no cliente via `getCompany(id)`. A resolução é
 * deduplicada por id num cache de processo: a página de Parceiros já faz
 * o mesmo enriquecimento, e aqui espelhamos esse padrão para não disparar
 * N chamadas idênticas quando vários imóveis vêm da mesma imobiliária.
 */

import type { Lead, Property, SharedFrom } from "../../data";
import type { SharedLead, SharedProperty } from "../../services/partnership.service";
import { getCompany } from "../../services/auth.service";
import { mapPropertyDtoToProperty } from "../properties/mappers";
import { mapApiLeadToLead } from "../leads/mappers";

// Cache de processo: id da empresa -> promessa do nome resolvido. Mantém a
// promessa (não só o valor) para que chamadas concorrentes para o mesmo id
// compartilhem um único request em voo. Vive enquanto a aba estiver aberta.
const companyNamePromiseCache = new Map<string, Promise<string>>();

/**
 * Resolve o nome de uma empresa pelo id, deduplicado. Tolerante a falha —
 * se o lookup falhar, devolve um rótulo neutro em vez de propagar o erro
 * (um nome ausente não deve quebrar a lista inteira de compartilhados).
 */
export async function resolveCompanyName(companyId: string): Promise<string> {
 if (!companyId) return "Empresa parceira";
 const cached = companyNamePromiseCache.get(companyId);
 if (cached) return cached;

 const promise = getCompany(companyId)
 .then((company) => company.name || "Empresa parceira")
 .catch(() => {
 // Remove a promessa rejeitada do cache para permitir nova tentativa
 // num próximo carregamento.
 companyNamePromiseCache.delete(companyId);
 return "Empresa parceira";
 });

 companyNamePromiseCache.set(companyId, promise);
 return promise;
}

/**
 * Mapeia os SharedProperty do backend em Property[], resolvendo os nomes
 * das empresas donas de forma deduplicada. Cada Property recebe
 * `sharedFrom`. O `source_company_name` do payload é usado como atalho
 * quando presente (futuro), caindo para a resolução por id.
 */
export async function mapSharedPropertiesToProperties(
 shared: SharedProperty[],
): Promise<Property[]> {
 if (!Array.isArray(shared) || shared.length === 0) return [];

 // Pré-resolve os nomes únicos numa só passada (dedup via cache).
 const uniqueCompanyIds = Array.from(
 new Set(shared.filter((s) => !s.source_company_name?.trim()).map((s) => s.source_company_id).filter(Boolean)),
 );
 const nameById = new Map<string, string>();
 await Promise.all(
 uniqueCompanyIds.map(async (id) => {
 nameById.set(id, await resolveCompanyName(id));
 }),
 );

 return shared.map((item) => {
 const companyName =
 item.source_company_name?.trim() ||
 nameById.get(item.source_company_id) ||
 "Empresa parceira";
 const sharedFrom: SharedFrom = {
 partnershipId: item.partnership_id,
 companyId: item.source_company_id,
 companyName,
 };
 return { ...mapPropertyDtoToProperty(item.property), sharedFrom };
 });
}

/**
 * Mapeia os SharedLead do backend em Lead[], resolvendo os nomes das
 * empresas donas de forma deduplicada. Cada Lead recebe `sharedFrom`.
 */
export async function mapSharedLeadsToLeads(shared: SharedLead[]): Promise<Lead[]> {
 if (!Array.isArray(shared) || shared.length === 0) return [];

 const uniqueCompanyIds = Array.from(
 new Set(shared.filter((s) => !s.source_company_name?.trim()).map((s) => s.source_company_id).filter(Boolean)),
 );
 const nameById = new Map<string, string>();
 await Promise.all(
 uniqueCompanyIds.map(async (id) => {
 nameById.set(id, await resolveCompanyName(id));
 }),
 );

 return shared.map((item) => {
 const companyName =
 item.source_company_name?.trim() ||
 nameById.get(item.source_company_id) ||
 "Empresa parceira";
 const sharedFrom: SharedFrom = {
 partnershipId: item.partnership_id,
 companyId: item.source_company_id,
 companyName,
 };
 return { ...mapApiLeadToLead(item.lead), sharedFrom };
 });
}

/**
 * Funde a lista própria com a lista compartilhada, deduplicando por id.
 * Itens próprios têm precedência (um imóvel da própria carteira nunca é
 * substituído pela versão projetada de privacidade). A ordem mantém os
 * próprios primeiro, seguidos dos compartilhados ainda não presentes.
 */
export function mergeOwnedAndShared<T extends { id: string }>(
 owned: T[],
 shared: T[],
): T[] {
 if (shared.length === 0) return owned;
 const ownedIds = new Set(owned.map((item) => item.id));
 const extras = shared.filter((item) => !ownedIds.has(item.id));
 return [...owned, ...extras];
}
