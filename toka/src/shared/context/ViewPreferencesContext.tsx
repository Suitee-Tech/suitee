import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";

export type ViewPreference = Record<string, unknown>;

type ViewPreferencesMap = Record<string, ViewPreference>;

export type RemotePreferencesClient = {
  getPreferences: () => Promise<{ view_preferences?: ViewPreferencesMap }>;
  updatePreferences: (data: { view_preferences: Record<string, ViewPreference> }) => Promise<unknown>;
};

interface ViewPreferencesContextType {
  preferences: ViewPreferencesMap;
  setScopePreference: (scope: string, next: ViewPreference) => void;
}

const STORAGE_KEY = "suitee-view-preferences";
const DEBOUNCE_MS = 400;

const ViewPreferencesContext = createContext<ViewPreferencesContextType>({
  preferences: {},
  setScopePreference: () => {},
});

function readFromStorage(): ViewPreferencesMap {
  try {
    if (typeof localStorage === "undefined") return {};
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed === "object") return parsed as ViewPreferencesMap;
  } catch {
    return {};
  }
  return {};
}

function writeToStorage(prefs: ViewPreferencesMap) {
  try {
    if (typeof localStorage === "undefined") return;
    localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs));
  } catch {
    return;
  }
}

export function ViewPreferencesProvider({
  children,
  userId,
  remoteClient,
}: {
  children: ReactNode;
  userId?: string | null;
  remoteClient?: RemotePreferencesClient;
}) {
  const [preferences, setPreferences] = useState<ViewPreferencesMap>(() => readFromStorage());

  const pendingTimers = useRef<Record<string, ReturnType<typeof setTimeout>>>({});
  const pendingPayloads = useRef<Record<string, ViewPreference>>({});

  useEffect(() => {
    if (!userId || !remoteClient) return;
    remoteClient
      .getPreferences()
      .then((prefs) => {
        const remote = prefs.view_preferences || {};
        setPreferences((prev) => {
          const merged = { ...prev, ...remote };
          writeToStorage(merged);
          return merged;
        });
      })
      .catch(() => {});
  }, [remoteClient, userId]);

  const flushScope = useCallback(
    (scope: string) => {
      const payload = pendingPayloads.current[scope];
      delete pendingPayloads.current[scope];
      delete pendingTimers.current[scope];
      if (!payload || !remoteClient) return;
      remoteClient.updatePreferences({ view_preferences: { [scope]: payload } }).catch(() => {});
    },
    [remoteClient],
  );

  const setScopePreference = useCallback(
    (scope: string, next: ViewPreference) => {
      setPreferences((prev) => {
        const updated = { ...prev, [scope]: next };
        writeToStorage(updated);
        return updated;
      });

      pendingPayloads.current[scope] = next;
      if (pendingTimers.current[scope]) clearTimeout(pendingTimers.current[scope]);
      pendingTimers.current[scope] = setTimeout(() => flushScope(scope), DEBOUNCE_MS);
    },
    [flushScope],
  );

  useEffect(() => {
    const timers = pendingTimers.current;
    return () => {
      for (const key of Object.keys(timers)) clearTimeout(timers[key]);
    };
  }, []);

  const value = useMemo(() => ({ preferences, setScopePreference }), [preferences, setScopePreference]);

  return <ViewPreferencesContext.Provider value={value}>{children}</ViewPreferencesContext.Provider>;
}

export function useViewPreferences<T extends Record<string, unknown>>(scope: string, defaults: T): [T, (patch: Partial<T>) => void] {
  const { preferences, setScopePreference } = useContext(ViewPreferencesContext);

  const stored = preferences[scope] as Partial<T> | undefined;
  const merged = useMemo(() => ({ ...defaults, ...(stored || {}) }) as T, [defaults, stored]);

  const setPartial = useCallback(
    (patch: Partial<T>) => {
      const next = { ...merged, ...patch } as T;
      setScopePreference(scope, next as unknown as ViewPreference);
    },
    [merged, scope, setScopePreference],
  );

  return [merged, setPartial];
}

export function useViewPreferencesContext() {
  return useContext(ViewPreferencesContext);
}

