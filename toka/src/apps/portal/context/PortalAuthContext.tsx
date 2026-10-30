import React, { createContext, useContext, useEffect, useState, useCallback, useRef } from "react";
import { portalService, type PortalUser } from "../services/portal.service";
import { setPortalAccessToken } from "../services/portal-api";
import { notifyApiError } from "../../../shared/services/errorToast";

export type { PortalUser };

interface PortalAuthContextType {
  user: PortalUser | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  requestOtp: (contact: string, method?: "email" | "whatsapp") => Promise<void>;
  verifyOtp: (contact: string, code: string) => Promise<void>;
  logout: () => void;
  refreshUserData: () => Promise<void>;
}

const PortalAuthContext = createContext<PortalAuthContextType | undefined>(undefined);

export const PortalAuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [user, setUser] = useState<PortalUser | null>(null);
  const [isAuthenticated, setIsAuthenticated] = useState<boolean>(false);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const logoutInProgress = useRef(false);

  const performLogout = useCallback(() => {
    if (logoutInProgress.current) return;
    logoutInProgress.current = true;
    setPortalAccessToken(null);
    setUser(null);
    setIsAuthenticated(false);
    logoutInProgress.current = false;
    window.location.href = "/portal/login";
  }, []);

  const fetchUserData = async (options?: { silent?: boolean }) => {
    const userData = await portalService.getCurrentUser(options);
    setUser(userData);
    setIsAuthenticated(true);
  };

  useEffect(() => {
    const initAuth = async () => {
      try {
        if (
          typeof window !== "undefined" &&
          window.location.pathname === "/portal/login"
        ) {
          if (new URLSearchParams(window.location.search).has("email")) {
            try {
              await portalService.logout();
            } catch {}
          }
          // Na tela de login não há sessão para restaurar — evita o
          // GET /auth/me com token obsoleto (404 ruído no console).
          setPortalAccessToken(null);
          setUser(null);
          setIsAuthenticated(false);
          return;
        }

        // P2/M18: o access token não é mais persistido, mas a restauração NÃO
        // pode depender do hint: o cookie HttpOnly é a fonte de verdade. Sem
        // cookie, o /auth/me devolve 401 e o interceptor tenta o refresh uma vez
        // (silencioso nesta chamada), terminando no estado não autenticado.
        await fetchUserData({ silent: true });
      } catch (e) {
        notifyApiError(e, { context: "Carregar usuário do portal", silent: true });
        setPortalAccessToken(null);
        setUser(null);
        setIsAuthenticated(false);
      } finally {
        setIsLoading(false);
      }
    };

    initAuth();
  }, []);

  const requestOtp = async (contact: string, method: "email" | "whatsapp" = "email") => {
    setIsLoading(true);
    try {
      await portalService.requestOtp(contact, method);
    } finally {
      setIsLoading(false);
    }
  };

  const verifyOtp = async (contact: string, code: string) => {
    setIsLoading(true);
    try {
      const response = await portalService.verifyOtp(contact, code);
      setPortalAccessToken(response.access_token);
      await fetchUserData();
    } finally {
      setIsLoading(false);
    }
  };

  const logout = async () => {
    try {
      await portalService.logout();
    } catch {} finally {
      performLogout();
    }
  };

  const refreshUserData = async () => {
    await fetchUserData();
  };

  return (
    <PortalAuthContext.Provider value={{ user, isAuthenticated, isLoading, requestOtp, verifyOtp, logout, refreshUserData }}>
      {children}
    </PortalAuthContext.Provider>
  );
};

export const usePortalAuth = (): PortalAuthContextType => {
  const context = useContext(PortalAuthContext);
  if (!context) {
    throw new Error("usePortalAuth must be used within a PortalAuthProvider");
  }
  return context;
};
