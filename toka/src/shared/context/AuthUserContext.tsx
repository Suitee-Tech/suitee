import { createContext, useContext, type ReactNode } from "react";

export type AuthUserLike = {
  id?: string;
  firstname?: string;
  lastname?: string;
  fullname?: string;
  email?: string;
  role?: string;
  permission?: string;
  avatar_url?: string;
  current_company_id?: string;
  companies?: Array<{ id: string }>;
} | null | undefined;

type AuthUserContextValue = {
  user: AuthUserLike;
};

const AuthUserContext = createContext<AuthUserContextValue | null>(null);

export function AuthUserProvider({ children, user }: { children: ReactNode; user: AuthUserLike }) {
  return <AuthUserContext.Provider value={{ user }}>{children}</AuthUserContext.Provider>;
}

export function useAuthUser() {
  const ctx = useContext(AuthUserContext);
  if (!ctx) throw new Error("useAuthUser must be used within an AuthUserProvider");
  return ctx;
}
