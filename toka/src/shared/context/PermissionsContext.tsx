import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { hasTenantWideScope } from "../permissions/accessScope";
import { useDataChangeRefresh } from "../hooks/useDataChangeRefresh";
import { useRequestScopeGuard } from "../hooks/useRequestScopeGuard";
import { useSessionRecovery } from "./SessionRecoveryContext";
import { isRecoverableStatus, recoveryRetryDelay } from "../lib/sessionRecovery";

export type PermissionsPayload = {
  functional_role: string;
  /** Papel base do perfil de acesso (`cr_…`); "" quando o papel é do catálogo. */
  base_role?: string;
  profile: string;
  permissions: string[];
};

export type PermissionsUserLike =
  | {
      current_company_id?: string;
      permission?: string;
      functional_role?: string;
    }
  | null
  | undefined;

export type PermissionsContextValue = {
  functionalRole: string | null;
  /** Papel base do perfil de acesso (`cr_…`) — "" quando o papel é do catálogo. */
  baseRole: string | null;
  /** Mesma regra do servidor (`pkg/auth.HasTenantWideScope`): vê o tenant inteiro. */
  hasTenantWideScope: boolean;
  profile: string | null;
  permissions: Set<string>;
  can: (permissionId: string) => boolean;
  isLoading: boolean;
  refresh: () => Promise<void>;
};

export const PermissionsContext = createContext<PermissionsContextValue>({
  functionalRole: null,
  baseRole: null,
  hasTenantWideScope: false,
  profile: null,
  permissions: new Set(),
  can: () => false,
  isLoading: true,
  refresh: async () => {},
});
const EMPTY_PERMISSIONS = new Set<string>();

export function usePermissionsContext() {
  return useContext(PermissionsContext);
}

export function PermissionsProvider({
  children,
  user,
  getMyPermissions,
}: {
  children: ReactNode;
  user: PermissionsUserLike;
  getMyPermissions?: () => Promise<PermissionsPayload>;
}) {
  const [functionalRole, setFunctionalRole] = useState<string | null>(null);
  const [baseRole, setBaseRole] = useState<string | null>(null);
  const [profile, setProfile] = useState<string | null>(null);
  const [permissions, setPermissions] = useState<Set<string>>(new Set());
  const [isLoading, setIsLoading] = useState(true);
  const [loadedScope, setLoadedScope] = useState<string | null>(null);
  const refreshSequenceRef = useRef(0);
  const currentScope = user?.current_company_id ?? "";
  const beginScopedRequest = useRequestScopeGuard(currentScope);
  const { isRecovering: sessionRecovering, recover } = useSessionRecovery();

  const refresh = useCallback(async () => {
    const isCurrentScope = beginScopedRequest();
    const sequence = refreshSequenceRef.current + 1;
    refreshSequenceRef.current = sequence;
    const isCurrent = () => isCurrentScope() && sequence === refreshSequenceRef.current;

    if (!getMyPermissions || !currentScope) {
      if (!isCurrent()) return;
      setFunctionalRole(null);
      setBaseRole(null);
      setProfile(null);
      setPermissions(new Set());
      setLoadedScope(currentScope);
      setIsLoading(false);
      return;
    }

    setIsLoading(true);
    try {
      // Falha transitória (aba voltando do foco, token vencido, gateway
      // reiniciando) não pode virar "usuário sem permissão": isso pintava a
      // tela de "acesso restrito" enquanto a sessão ainda estava sendo
      // recuperada. Tentamos de novo com backoff e, num 401, renovamos a sessão
      // antes de repetir.
      const maxAttempts = 3;
      for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
        try {
          const data = await getMyPermissions();
          if (!isCurrent()) return;
          setFunctionalRole(data.functional_role);
          setBaseRole(data.base_role ?? "");
          setProfile(data.profile);
          setPermissions(new Set(data.permissions));
          setLoadedScope(currentScope);
          return;
        } catch (error) {
          const status = (error as { response?: { status?: number } })?.response?.status;
          const canRetry = attempt < maxAttempts - 1 && isRecoverableStatus(status);
          if (!canRetry) throw error;
          if (status === 401) {
            // A sessão pode ser renovável: tenta renovar antes de repetir. Se a
            // renovação não estiver disponível (fora do provider) ou falhar, a
            // tentativa seguinte ainda acontece — o estado vazio só é assumido
            // depois de esgotar as tentativas, e não na primeira resposta 401.
            await recover();
          }
          await new Promise((resolve) => setTimeout(resolve, recoveryRetryDelay(attempt)));
        }
      }
    } catch (error) {
      if (!isCurrent()) return;
      console.error("Failed to fetch permissions", error);
      setFunctionalRole(null);
      setBaseRole(null);
      setProfile(null);
      setPermissions(new Set());
      setLoadedScope(currentScope);
    } finally {
      if (isCurrent()) setIsLoading(false);
    }
  }, [beginScopedRequest, currentScope, getMyPermissions, recover]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  // Preferências de visualização também são gravadas em /settings e não
  // alteram nenhuma permissão. Observar `settings` aqui fazia um simples
  // clique em "Mês" recarregar permissões e, em cascata, a agenda inteira.
  useDataChangeRefresh(["permissions", "members", "companies"], refresh, {
    enabled: Boolean(getMyPermissions && user?.current_company_id),
  });

  const hasCurrentScope = loadedScope === currentScope;
  const effectiveFunctionalRole = hasCurrentScope ? functionalRole : null;
  const effectiveBaseRole = hasCurrentScope ? baseRole : null;
  const effectiveProfile = hasCurrentScope ? profile : null;
  const effectivePermissions = hasCurrentScope ? permissions : EMPTY_PERMISSIONS;
  // Enquanto a sessão está sendo restaurada, seguimos "carregando": as telas
  // mostram spinner em vez de "sem acesso" com o contexto ainda por vir.
  const effectiveIsLoading = !hasCurrentScope || isLoading || sessionRecovering;

  const can = useCallback(
    (permissionId: string) => {
      if (!hasCurrentScope) return false;
      if (
        effectiveProfile === "owner" ||
        effectiveProfile === "admin" ||
        user?.permission === "owner" ||
        user?.permission === "admin" ||
        effectiveFunctionalRole === "admin" ||
        user?.functional_role === "admin"
      ) {
        return true;
      }
      return effectivePermissions.has(permissionId);
    },
    [effectiveFunctionalRole, effectivePermissions, effectiveProfile, hasCurrentScope, user],
  );

  return (
    <PermissionsContext.Provider
      value={{
        functionalRole: effectiveFunctionalRole,
        baseRole: effectiveBaseRole,
        hasTenantWideScope: hasTenantWideScope(effectiveProfile, effectiveFunctionalRole, effectiveBaseRole),
        profile: effectiveProfile,
        permissions: effectivePermissions,
        can,
        isLoading: effectiveIsLoading,
        refresh,
      }}
    >
      {children}
    </PermissionsContext.Provider>
  );
}
