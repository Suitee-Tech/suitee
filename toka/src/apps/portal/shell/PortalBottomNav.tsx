import { useLocation, Link } from "react-router";
import { clsx } from "clsx";
import { isPortalNavigationItemActive, portalNavItems } from "../navigation";
import { usePortalPersona } from "../hooks/usePortalPersona";

export function PortalBottomNav() {
  const { pathname } = useLocation();
  // Cada persona vai para onde tem imóvel de verdade (busca × meus imóveis).
  const navItems = portalNavItems(usePortalPersona().persona);

  return (
    <nav
      aria-label="Navegação principal do portal"
      className="fixed bottom-0 inset-x-0 z-30 border-t border-border bg-card/95 pb-[env(safe-area-inset-bottom)] shadow-[0_-8px_30px_rgba(15,23,42,0.06)] backdrop-blur-lg"
    >
      <div className="mx-auto flex h-16 max-w-2xl items-stretch justify-around px-2">
        {navItems.map((item) => {
          const isActive = isPortalNavigationItemActive(pathname, item);
          const Icon = item.icon;
          return (
            <Link
              key={item.path}
              to={item.path}
              aria-current={isActive ? "page" : undefined}
              className={clsx(
                "relative flex min-w-0 flex-1 flex-col items-center justify-center gap-1 rounded-xl px-2 text-center transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2",
                isActive ? "text-primary" : "text-muted-foreground hover:text-foreground",
              )}
            >
              {isActive && <span className="absolute inset-x-5 top-0 h-0.5 rounded-full bg-primary" aria-hidden="true" />}
              <Icon className="h-5 w-5" strokeWidth={isActive ? 2.5 : 2} aria-hidden="true" />
              <span className="max-w-full truncate text-[11px] font-semibold">{item.label}</span>
            </Link>
          );
        })}
      </div>
    </nav>
  );
}
