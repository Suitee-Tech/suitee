import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { platformSubscriptionService, type PlatformSubscriptionAccess } from "../services/platformSubscription.service";

type SubscriptionAccessContextValue = {
  access: PlatformSubscriptionAccess | null;
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  hasFeature: (feature: string) => boolean;
};

const SubscriptionAccessContext = createContext<SubscriptionAccessContextValue>({
  access: null,
  isLoading: false,
  error: null,
  refresh: async () => {},
  // Until the signed tenant decision is available, UI affordances stay
  // navigable. The API gateway remains the enforcement boundary.
  hasFeature: () => true,
});

export function SubscriptionAccessProvider({
  children,
  active,
  companyId,
}: {
  children: ReactNode;
  active: boolean;
  companyId?: string;
}) {
  const [access, setAccess] = useState<PlatformSubscriptionAccess | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestSequence = useRef(0);
  const activeScope = useRef<string | null>(null);

  const refresh = useCallback(async () => {
    const sequence = ++requestSequence.current;
    if (!active || !companyId) {
      activeScope.current = null;
      setAccess(null);
      setError(null);
      setIsLoading(false);
      return;
    }

    const scopeChanged = activeScope.current !== companyId;
    activeScope.current = companyId;
    if (scopeChanged) setAccess(null);
    setIsLoading(true);
    setError(null);
    try {
      const result = await platformSubscriptionService.getAccess();
      if (sequence === requestSequence.current) setAccess(result);
    } catch {
      if (sequence === requestSequence.current) {
        setError("Não foi possível consultar o acesso do plano.");
      }
    } finally {
      if (sequence === requestSequence.current) setIsLoading(false);
    }
  }, [active, companyId]);

  useEffect(() => {
    void refresh();
    return () => {
      requestSequence.current += 1;
    };
  }, [refresh]);

  // Every company member refreshes the tenant-level entitlement while the app
  // is open, so a plan change propagates without requiring a new login.
  useEffect(() => {
    if (!active || !companyId) return;
    const refreshVisible = () => {
      if (document.visibilityState === "visible") void refresh();
    };
    const interval = window.setInterval(refreshVisible, 30_000);
    window.addEventListener("focus", refreshVisible);
    document.addEventListener("visibilitychange", refreshVisible);
    return () => {
      window.clearInterval(interval);
      window.removeEventListener("focus", refreshVisible);
      document.removeEventListener("visibilitychange", refreshVisible);
    };
  }, [active, companyId, refresh]);

  const hasFeature = useCallback((feature: string) => {
    if (!access?.access_control_enabled) return true;
    return access.entitled && access.features.includes(feature);
  }, [access]);

  return (
    <SubscriptionAccessContext.Provider value={{ access, isLoading, error, refresh, hasFeature }}>
      {children}
    </SubscriptionAccessContext.Provider>
  );
}

export function useSubscriptionAccess() {
  return useContext(SubscriptionAccessContext);
}
