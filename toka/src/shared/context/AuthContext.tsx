import React, { createContext, useContext, useEffect, useState, useCallback, useRef } from 'react';
import {
  User,
  Company,
  RegisterRequest,
  getCurrentUser,
  getUserCompanies,
  login as apiLogin,
  logout as apiLogout,
  refreshTokenRequest,
  register as apiRegister,
  switchContext,
} from '../services/auth.service';
import { clearAgentsCache } from '../lib/agentsCache';
import {
  clearAuthRestoreFailure,
  getAccessToken,
  hasRecentAuthRestoreFailure,
  hasSessionHint,
  markAuthRestoreFailed,
  setAccessToken,
} from '../services/api';
import { settingsService } from '../services/settings.service';
import { isRecoverableStatus, recoveryRetryDelay } from '../lib/sessionRecovery';
import { publishTenantChange } from '../lib/dataSync';
import { useDataChangeRefresh } from '../hooks/useDataChangeRefresh';
import { wsManager } from '../ws/wsManager';

interface AuthContextType {
  user: User | null;
  companies: Company[];
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => void;
  register: (data: RegisterRequest) => Promise<void>;
  refreshUserData: () => Promise<void>;
  selectCompany: (companyId: string) => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

const authChannel = typeof BroadcastChannel !== 'undefined' ? new BroadcastChannel('auth') : null;

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [user, setUser] = useState<User | null>(null);
  const [companies, setCompanies] = useState<Company[]>([]);
  const [isAuthenticated, setIsAuthenticated] = useState<boolean>(false);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const logoutInProgress = useRef(false);
  const fetchInProgress = useRef<Promise<void> | null>(null);

  const performLogout = useCallback(() => {
    if (logoutInProgress.current) return;
    logoutInProgress.current = true;
    setAccessToken(null);
    setUser(null);
    setCompanies([]);
    setIsAuthenticated(false);
    localStorage.removeItem('current_company_id');
    clearAgentsCache();
    wsManager.disconnect();
    logoutInProgress.current = false;
    window.location.href = '/login';
  }, []);

  useEffect(() => {
    if (!authChannel) return;
    const handler = (event: MessageEvent) => {
      if (event.data === 'logout') {
        performLogout();
      }
    };
    authChannel.addEventListener('message', handler);
    return () => authChannel.removeEventListener('message', handler);
  }, [performLogout]);

  const fetchUserData = async (retryCount = 0) => {
    if (fetchInProgress.current) return fetchInProgress.current;
    fetchInProgress.current = (async () => {
      try {
        const userData = await getCurrentUser();

        if (userData?.functional_role === 'cliente') {
          setAccessToken(null);
          setUser(null);
          setCompanies([]);
          setIsAuthenticated(false);
          if (typeof window !== 'undefined' && !window.location.pathname.startsWith('/portal')) {
            window.location.href = '/portal/login';
          }
          return;
        }

        const companiesData = await getUserCompanies(userData);
        const companyList = Array.isArray(companiesData) ? companiesData : [];

        let currentCompanyId = localStorage.getItem('current_company_id');

        if (currentCompanyId && !companyList.find(c => c.id === currentCompanyId)) {
          localStorage.removeItem('current_company_id');
          currentCompanyId = null;
        }

        if (!currentCompanyId && companyList.length > 0) {
          try {
            const prefs = await settingsService.getPreferences();
            const remembered = prefs.last_company_id;
            if (remembered && companyList.find(c => c.id === remembered)) {
              currentCompanyId = remembered;
              localStorage.setItem('current_company_id', remembered);
            }
          } catch {
          }
        }

        if (!currentCompanyId && companyList.length > 0) {
          try {
            const firstCompany = companyList[0];
            const { access_token } = await switchContext(firstCompany.id);
            setAccessToken(access_token);
            localStorage.setItem('current_company_id', firstCompany.id);
            currentCompanyId = firstCompany.id;
          } catch (error) {
            console.error('Auto-select company failed:', error);
            throw error;
          }
        } else if (currentCompanyId) {
          try {
            const { access_token } = await switchContext(currentCompanyId);
            setAccessToken(access_token);
          } catch (error) {
            console.error('Failed to switch context:', error);
            localStorage.removeItem('current_company_id');
            currentCompanyId = null;
            throw error;
          }
        }

        const currentCompany = companyList.find(c => c.id === currentCompanyId);

        setUser({
          ...userData,
          companies: companyList,
          current_company_id: currentCompanyId || undefined,
          permission: currentCompany?.profile,
        });
        setCompanies(companyList);
        setIsAuthenticated(true);
      } catch (error: any) {
        console.error('Failed to fetch user data:', error);

        const status = error?.response?.status;
        if (status === 401 || status === 404) {
          // Encerra a sessão SEM recarregar a página.
          //
          // Antes isto chamava `performLogout`, que faz `window.location.href =
          // '/login'`. Com o vínculo recém-trocado (perfil de acesso alterado), o
          // reload refazia o refresh — que passava —, a chamada autenticada
          // seguinte era recusada de novo e a página recarregava em loop, sempre
          // no "Carregando..." (incidente de 2026-09-21). Limpar o estado aqui
          // deixa o roteador mostrar o login e o usuário retomar o controle.
          markAuthRestoreFailed();
          setAccessToken(null);
          setUser(null);
          setCompanies([]);
          setIsAuthenticated(false);
          localStorage.removeItem('current_company_id');
          clearAgentsCache();
          wsManager.disconnect();
          return;
        }

        // Falha recuperável (gateway subindo, timeout, 429, 401 já tratado pelo
        // cliente HTTP) não pode derrubar a sessão do usuário: repete com
        // backoff antes de considerar a restauração perdida.
        if (isRecoverableStatus(status) && retryCount < 3) {
          await new Promise(resolve => setTimeout(resolve, recoveryRetryDelay(retryCount, 500)));
          fetchInProgress.current = null;
          await fetchUserData(retryCount + 1);
          return;
        }

        throw error;
      }
    })().finally(() => {
      fetchInProgress.current = null;
    });
    return fetchInProgress.current;
  };

  useDataChangeRefresh(['users', 'profile', 'companies'], () => fetchUserData(), {
    enabled: isAuthenticated,
  });

  useEffect(() => {
    if (typeof window !== 'undefined' && typeof window.location?.pathname === 'string' && window.location.pathname.startsWith('/portal')) {
      setAccessToken(null);
      setUser(null);
      setCompanies([]);
      setIsAuthenticated(false);
      setIsLoading(false);
      return;
    }
    (async () => {
      try {
        const existing = getAccessToken();
        if (existing) {
          await fetchUserData();
          return;
        }

        // Sem dica de sessão (visita nova, logout explícito, refresh já falhou)
        // NÃO se tenta restaurar: mostra o formulário. E se a restauração falhou
        // há poucos segundos, também não se tenta de novo — é o que impede o
        // ciclo refresh → 401 → reload → refresh.
        if (!hasSessionHint() || hasRecentAuthRestoreFailure()) {
          setAccessToken(null);
          setIsLoading(false);
          return;
        }

        // Aba nova (ou F5 depois de horas): a sessão pode ser restaurada pelo
        // cookie de refresh. Uma falha transitória aqui (gateway reiniciando,
        // timeout) NÃO pode mandar o usuário para o login — tenta de novo com
        // backoff e só desiste quando a recusa é definitiva.
        let restored = false;
        for (let attempt = 0; attempt < 3 && !restored; attempt += 1) {
          try {
            const { access_token } = await refreshTokenRequest();
            if (access_token) {
              setAccessToken(access_token);
              restored = true;
            } else {
              setAccessToken(null);
            }
          } catch (error) {
            const status = (error as { response?: { status?: number } })?.response?.status;
            if (!isRecoverableStatus(status) || attempt === 2) throw error;
            await new Promise(resolve => setTimeout(resolve, recoveryRetryDelay(attempt, 500)));
          }
        }
        if (restored) {
          await fetchUserData();
        }
      } catch {
        markAuthRestoreFailed();
        setAccessToken(null);
        setUser(null);
        setCompanies([]);
        setIsAuthenticated(false);
      } finally {
        setIsLoading(false);
      }
    })();
  }, []);

  const login = async (email: string, password: string) => {
    setIsLoading(true);
    try {
      const { access_token } = await apiLogin(email, password);
      setAccessToken(access_token);
      clearAuthRestoreFailure();
      await fetchUserData();
    } finally {
      setIsLoading(false);
    }
  };

  const logout = async () => {
    await apiLogout();
    authChannel?.postMessage('logout');
    clearAuthRestoreFailure();
    performLogout();
  };

  const register = async (data: RegisterRequest) => {
    setIsLoading(true);
    try {
      const { access_token } = await apiRegister(data);
      setAccessToken(access_token);
      await fetchUserData();
    } catch (error) {
      console.error('Registration failed:', error);
      throw error;
    } finally {
      setIsLoading(false);
    }
  };

  const refreshUserData = async () => {
    await fetchUserData();
  };

  const selectCompany = async (companyId: string) => {
    setIsLoading(true);
    try {
      const { access_token } = await switchContext(companyId);
      setAccessToken(access_token);
      localStorage.setItem('current_company_id', companyId);
      clearAgentsCache();
      publishTenantChange(companyId);
      wsManager.reconnect();
      settingsService.updatePreferences({ last_company_id: companyId }).catch(() => {
      });

      if (user) {
        setUser({ ...user, current_company_id: companyId });
      }

      await fetchUserData();
    } catch (error) {
      console.error('Failed to switch context:', error);
      throw error;
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <AuthContext.Provider value={{ user, companies, isAuthenticated, isLoading, login, logout, register, refreshUserData, selectCompany }}>
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = (): AuthContextType => {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};

export function useOptionalAuth(): AuthContextType | null {
  return useContext(AuthContext) ?? null;
}
