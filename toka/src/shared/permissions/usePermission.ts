import { usePermissionsContext } from "../context/PermissionsContext";

export function usePermission(id: string): boolean {
  const { can } = usePermissionsContext();
  return can(id);
}

export function useCan() {
  const { can } = usePermissionsContext();

  return (id: string, opts?: { anyOf?: string[] }): boolean => {
    if (opts?.anyOf) {
      return opts.anyOf.some((permissionId) => can(permissionId));
    }
    return can(id);
  };
}

export function useFunctionalRole(): string | null {
  const { functionalRole } = usePermissionsContext();
  return functionalRole;
}
