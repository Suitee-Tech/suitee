import { createContext, useContext, useState, useEffect, useCallback, useRef, ReactNode } from "react";
import { useOptionalAuth } from "./AuthContext";
import { settingsService } from "../services/settings.service";

interface FocusModeContextType {
  focusMode: boolean;
  toggleFocusMode: () => void;
}

const STORAGE_KEY = "suitee-focus-mode";

const FocusModeContext = createContext<FocusModeContextType>({
  focusMode: false,
  toggleFocusMode: () => {},
});

export function FocusModeProvider({ children }: { children: ReactNode }) {
  const auth = useOptionalAuth();
  const user = auth?.user ?? null;
  const [focusMode, setFocusMode] = useState<boolean>(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      return stored !== null ? stored === "true" : false;
    } catch {
      return false;
    }
  });
  const focusModeRef = useRef(focusMode);
  focusModeRef.current = focusMode;

  useEffect(() => {
    if (user) {
      settingsService.getPreferences().then((prefs) => {
        const value = prefs.focus_mode ?? false;
        setFocusMode(value);
        localStorage.setItem(STORAGE_KEY, JSON.stringify(value));
      }).catch(() => {
      });
    }
  }, [user]);

  const toggleFocusMode = useCallback(() => {
    const current = focusModeRef.current;
    const next = !current;
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
    setFocusMode(next);
    settingsService.updatePreferences({ focus_mode: next }).catch(() => {
      setFocusMode(current);
      localStorage.setItem(STORAGE_KEY, JSON.stringify(current));
    });
  }, []);

  return (
    <FocusModeContext.Provider value={{ focusMode, toggleFocusMode }}>
      {children}
    </FocusModeContext.Provider>
  );
}

export function useFocusMode() {
  return useContext(FocusModeContext);
}

