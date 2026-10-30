import {
  FUNCTIONAL_ROLES,
  PERMISSION_CATALOG,
  ROLE_LABELS,
  type FunctionalRole,
} from "../../../lib/permissions/catalog";
import type { CustomRole, Permission } from "../../services/settings.service";

/**
 * Perfis de acesso personalizados — helpers compartilhados entre as duas telas
 * que antes viviam separadas e sem ligação:
 *
 *   - Configurações → Permissões de Acesso → Perfis Personalizados (o PERFIL:
 *     quem pode o quê);
 *   - Configurações → Empresa e Equipe → Grupos de Usuários (a ATRIBUIÇÃO:
 *     quem tem o perfil).
 *
 * O id do perfil usa o prefixo reservado `cr_` (pkg/auth.AccessProfilePrefix):
 * é ele que impede um perfil de colidir com — ou se passar por — um papel do
 * catálogo.
 */
export const ACCESS_PROFILE_PREFIX = "cr_";

export function isAccessProfileId(role?: string | null): boolean {
  return typeof role === "string" && role.startsWith(ACCESS_PROFILE_PREFIX) && role.length > ACCESS_PROFILE_PREFIX.length;
}

export function findAccessProfile(profiles: CustomRole[] | undefined, id?: string | null): CustomRole | undefined {
  if (!id) return undefined;
  return (profiles || []).find((profile) => profile.id === id);
}

/** Rótulo do papel/perfil: papel do catálogo, nome do perfil ou "perfil removido". */
export function roleDisplayName(role: string | undefined | null, profiles: CustomRole[] | undefined): string {
  if (!role) return "—";
  const profile = findAccessProfile(profiles, role);
  if (profile) return profile.name;
  if (isAccessProfileId(role)) return "Perfil removido";
  return ROLE_LABELS[role as FunctionalRole] || role;
}

/**
 * Um id de papel/perfil é atribuível quando existe: papel do catálogo ou perfil
 * do tenant. Perfil apagado com membros ainda apontando para ele aparece na
 * interface como "Perfil removido" — fail-closed visível, em vez de um usuário
 * silenciosamente sem permissão nenhuma.
 */
export function isKnownAssignable(role: string | undefined | null, profiles: CustomRole[] | undefined): boolean {
  if (!role) return false;
  if (FUNCTIONAL_ROLES.includes(role as FunctionalRole)) return true;
  return Boolean(findAccessProfile(profiles, role));
}

export function isOrphanProfile(role: string | undefined | null, profiles: CustomRole[] | undefined): boolean {
  return isAccessProfileId(role) && !findAccessProfile(profiles, role) && roleDisplayName(role, profiles) === "Perfil removido";
}

/** Permissões efetivas de um papel do catálogo na matriz do tenant. */
export function permissionsForCatalogRole(role: string, matrix: Permission[]): Set<string> {
  const granted = new Set<string>();
  matrix.forEach((permission) => {
    if (permission.roles?.[role]) granted.add(permission.id);
  });
  return granted;
}

/** Permissões concedidas ao papel OU perfil, para exibição. */
export function grantedPermissionIds(
  role: string | undefined | null,
  matrix: Permission[],
  profiles: CustomRole[] | undefined,
): Set<string> {
  if (!role) return new Set();
  const profile = findAccessProfile(profiles, role);
  if (profile) return new Set(profile.permissions || []);
  if (isAccessProfileId(role)) return new Set();
  return permissionsForCatalogRole(role, matrix);
}

export function grantedCount(role: string | undefined | null, matrix: Permission[], profiles: CustomRole[] | undefined): number {
  return grantedPermissionIds(role, matrix, profiles).size;
}

export function totalPermissionCount(): number {
  return PERMISSION_CATALOG.length;
}

/** Opções de perfil para selects: papéis do catálogo e perfis do tenant. */
export function assignableRoleOptions(profiles: CustomRole[] | undefined): Array<{ value: string; label: string }> {
  return [
    ...FUNCTIONAL_ROLES.map((role) => ({ value: role, label: ROLE_LABELS[role] })),
    ...(profiles || []).map((profile) => ({ value: profile.id, label: profile.name })),
  ];
}
