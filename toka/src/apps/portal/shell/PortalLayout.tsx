import { Outlet } from "react-router";
import { TermsAcceptanceModal } from "../../../shared/components/TermsAcceptanceModal";
import { portalService } from "../services/portal.service";
import { usePortalAuth } from "../context/PortalAuthContext";
import { PortalBottomNav } from "./PortalBottomNav";

export function PortalLayout() {
  const { logout } = usePortalAuth();
  return (
    <div className="min-h-dvh bg-muted/30 pb-[calc(4rem+env(safe-area-inset-bottom))] text-foreground">
      <a
        href="#portal-content"
        className="sr-only z-50 rounded-lg bg-card px-4 py-2 font-semibold text-foreground shadow-lg focus:not-sr-only focus:fixed focus:left-4 focus:top-4"
      >
        Ir para o conteúdo
      </a>
      <main id="portal-content" className="min-h-[calc(100dvh-4rem)]" tabIndex={-1}>
        <Outlet />
      </main>
      <PortalBottomNav />
      <TermsAcceptanceModal policyPath="/portal/privacy" termsPath="/portal/terms" onLogout={logout} settings={portalService} />
    </div>
  );
}
