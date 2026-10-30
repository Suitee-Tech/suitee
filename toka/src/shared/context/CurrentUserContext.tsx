import { createContext, useContext, type ReactNode } from "react";

type CurrentUserContextValue = {
  userId?: string;
};

const CurrentUserContext = createContext<CurrentUserContextValue | null>(null);

export function CurrentUserProvider({
  children,
  userId,
}: {
  children: ReactNode;
  userId?: string;
}) {
  return <CurrentUserContext.Provider value={{ userId }}>{children}</CurrentUserContext.Provider>;
}

export function useCurrentUser() {
  const ctx = useContext(CurrentUserContext);
  if (!ctx) throw new Error("useCurrentUser must be used within CurrentUserProvider");
  return ctx;
}
