/**
 * Fonte única do "quem enxerga o tenant inteiro" no FRONTEND.
 *
 * Espelha `pkg/auth.HasTenantWideScope` (Go) — os três eixos precisam ser os
 * mesmos dos dois lados, senão a tela esconde dados que a API entregou.
 *
 * Incidente de 2026-09-21 ("O perfil de Yuri mesmo sendo administrador não vê
 * leads"): a página de Leads filtrava por corretor no cliente com
 * `user.permission.includes("admin")` — o eixo do PERFIL, que para um membro
 * comum é `member` mesmo quando o PAPEL é `admin`. O resultado era a lista
 * vazia, ainda que a API tivesse devolvido os leads do tenant.
 *
 * Eixos (qualquer um basta):
 *   - perfil (`permission`): admin, owner, manager, gestor, operacoes;
 *   - papel funcional (`functional_role`): os mesmos;
 *   - papel BASE do perfil de acesso (`base_role`): um perfil `cr_…` baseado em
 *     gestor enxerga como o gestor.
 */

const TENANT_WIDE = new Set(["admin", "owner", "manager", "gestor", "operacoes"]);

const normalize = (value?: string | null): string => (value || "").trim().toLowerCase();

export function hasTenantWideScope(
  profile?: string | null,
  functionalRole?: string | null,
  baseRole?: string | null,
): boolean {
  return TENANT_WIDE.has(normalize(profile)) || TENANT_WIDE.has(normalize(functionalRole)) || TENANT_WIDE.has(normalize(baseRole));
}
