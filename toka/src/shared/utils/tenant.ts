import type { Company, User } from "../services/auth.service";

/**
 * Resolves the active company id used as the tenant segment of the cache key.
 *
 * The write side (DealsContext.upsertDeal via `companyId`) and the read side
 * (`useDealQuery`/`useDealsQuery`, the deal side panel) must resolve this value
 * identically, otherwise the mutation response is written to a cache key that
 * the panel never observes and the UI stays stale until a full page refresh.
 *
 * `current_company_id` is authoritative; when it is not yet set (e.g. before a
 * company is selected) we fall back to the first available company id — the
 * same fallback used by DealsProvider/AdminProviders.
 */
export function getActiveCompanyId(user: User | null | undefined): string | undefined {
  return (user?.current_company_id as string | undefined) || user?.companies?.[0]?.id;
}

/**
 * Resolves the company (imobiliária) the user is currently working in.
 *
 * Prefer the list loaded by AuthContext over `user.companies` because it is
 * refreshed after Configurações is saved; `user.companies` is only the
 * fallback for screens rendered before the companies payload arrives.
 */
export function getActiveCompany(
  user: User | null | undefined,
  companies: Company[] | null | undefined,
): Company | undefined {
  const list = companies?.length ? companies : user?.companies;
  if (!list?.length) return undefined;
  const activeId = getActiveCompanyId(user);
  return list.find((company) => company.id === activeId) ?? list[0];
}

/**
 * Display name of the agency. Trade name (`name`) is what the user sees in
 * Configurações → Empresa, so documents should mirror it; the legal name is a
 * fallback for companies that only filled the razão social.
 */
export function getCompanyDisplayName(company: Company | null | undefined): string | undefined {
  return company?.name?.trim() || company?.legal_name?.trim() || undefined;
}
