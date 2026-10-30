import type { ReactNode } from "react";
import { useCan } from "./usePermission";

interface CanProps {
  permission?: string;
  anyOf?: string[];
  fallback?: ReactNode;
  children: ReactNode;
}

export function Can({ permission, anyOf, fallback = null, children }: CanProps) {
  const checkCan = useCan();
  const isAllowed = permission ? checkCan(permission, { anyOf }) : anyOf ? checkCan("", { anyOf }) : false;
  if (isAllowed) return <>{children}</>;
  return <>{fallback}</>;
}
