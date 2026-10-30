import { Home, Search, MessageSquare, Menu } from "lucide-react";

import { portalCopy, type PortalPersona } from "./lib/portalPersona";

export interface PortalNavigationItem {
  path: string;
  label: string;
  icon: typeof Home;
  activePrefixes?: string[];
}

export const PORTAL_BOTTOM_NAV_ITEMS: PortalNavigationItem[] = [
  { path: "/portal", label: "Início", icon: Home, activePrefixes: ["/portal/deals/", "/portal/atendimentos/"] },
  {
    path: "/portal/buscar",
    label: "Imóveis",
    icon: Search,
    activePrefixes: ["/portal/buscar", "/portal/imoveis"],
  },
  { path: "/portal/mensagens", label: "Mensagens", icon: MessageSquare, activePrefixes: ["/portal/mensagens"] },
  {
    path: "/portal/menu",
    label: "Menu",
    icon: Menu,
    activePrefixes: [
      "/portal/menu",
      "/portal/perfil",
      "/portal/notificacoes",
      "/portal/leads",
      "/portal/locacao",
      "/portal/finance",
      "/portal/informe-rendimentos",
      "/portal/chamados",
      "/portal/suporte",
    ],
  },
];

export function isPortalNavigationItemActive(pathname: string, item: PortalNavigationItem): boolean {
  if (pathname === item.path) return true;
  return (item.activePrefixes ?? []).some((prefix) => pathname.startsWith(prefix));
}

/**
 * Navegação inferior por persona.
 *
 * A aba "Imóveis" abria a BUSCA de imóveis para todo mundo — para quem anuncia
 * (vendedor, locador) isso é um fluxo que não existe, e a carteira dele (os
 * imóveis que a imobiliária captou) ficava atrás de "Menu". Cada lado vai para
 * onde tem imóvel de verdade.
 */
export function portalNavItems(persona: PortalPersona): PortalNavigationItem[] {
  const copy = portalCopy(persona);
  return PORTAL_BOTTOM_NAV_ITEMS.map((item) =>
    item.path === "/portal/buscar"
      ? {
          ...item,
          path: copy.propertiesNavPath,
          label: copy.propertiesNavLabel,
          activePrefixes: ["/portal/buscar", "/portal/imoveis"],
        }
      : item,
  );
}
