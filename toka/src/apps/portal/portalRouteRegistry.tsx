import { lazy, Suspense, type ComponentType } from "react";
import type { RouteObject } from "react-router";
import { RouteErrorFallback } from "../../shared/components/RouteErrorFallback";
import { RouteLoading } from "../../shared/components/RouteLoading";
import { PortalProtectedRoute } from "./components/PortalProtectedRoute";
import { PortalRoot } from "./PortalRoot";
import { PortalLayout } from "./shell/PortalLayout";

const routeError = <RouteErrorFallback />;

const withSuspense = <P extends object>(Component: ComponentType<P>) => {
  return function SuspenseWrapper(props: P) {
    return (
      <Suspense fallback={<RouteLoading />}>
        <Component {...props} />
      </Suspense>
    );
  };
};

const PortalLoginPage = withSuspense(lazy(() => import("./pages/PortalLogin").then((m) => ({ default: m.PortalLogin }))));
const PortalDashboardPage = withSuspense(lazy(() => import("./pages/PortalDashboard").then((m) => ({ default: m.PortalDashboard }))));
const PortalDealDetailPage = withSuspense(lazy(() => import("./pages/PortalDealDetail").then((m) => ({ default: m.PortalDealDetail }))));
const PortalLeadDetailPage = withSuspense(lazy(() => import("./pages/PortalLeadDetail").then((m) => ({ default: m.PortalLeadDetail }))));
const PortalMenuPage = withSuspense(lazy(() => import("./pages/PortalMenu").then((m) => ({ default: m.PortalMenu }))));
const PortalFinancePage = withSuspense(lazy(() => import("./pages/PortalFinance").then((m) => ({ default: m.PortalFinance }))));
const PortalBuscarPage = withSuspense(lazy(() => import("./pages/PortalBuscar").then((m) => ({ default: m.PortalBuscar }))));
const PortalMessagesPage = withSuspense(lazy(() => import("./pages/PortalMessages").then((m) => ({ default: m.PortalMessages }))));
const PortalChamadosPage = withSuspense(lazy(() => import("./pages/PortalChamados").then((m) => ({ default: m.PortalChamados }))));
const PortalProfilePage = withSuspense(lazy(() => import("./pages/PortalProfile").then((m) => ({ default: m.PortalProfile }))));
const PortalLeadsPage = withSuspense(lazy(() => import("./pages/PortalLeads").then((m) => ({ default: m.PortalLeads }))));
const PortalNotificationsPage = withSuspense(lazy(() => import("./pages/PortalNotifications").then((m) => ({ default: m.PortalNotifications }))));
const PortalSupportPage = withSuspense(lazy(() => import("./pages/PortalSupport").then((m) => ({ default: m.PortalSupport }))));
const PortalInformeRendimentosPage = withSuspense(
  lazy(() => import("./pages/PortalInformeRendimentos").then((m) => ({ default: m.PortalInformeRendimentos }))),
);
const PortalRentalsPage = withSuspense(lazy(() => import("./pages/PortalRentals").then((m) => ({ default: m.PortalRentals }))));
const PortalRentalDetailsPage = withSuspense(lazy(() => import("./pages/PortalRentalDetails").then((m) => ({ default: m.PortalRentalDetails }))));
const PortalImoveisPage = withSuspense(lazy(() => import("./pages/PortalImoveis").then((m) => ({ default: m.PortalImoveis }))));
const PortalImovelDetailPage = withSuspense(lazy(() => import("./pages/PortalImovelDetail").then((m) => ({ default: m.PortalImovelDetail }))));
const PrivacyPagePage = withSuspense(lazy(() => import("./pages/PrivacyPage").then((m) => ({ default: m.PrivacyPage }))));
const TermsPagePage = withSuspense(lazy(() => import("./pages/TermsPage").then((m) => ({ default: m.TermsPage }))));

export function getPortalRouteObjects(): RouteObject[] {
  return [
    { path: "/portal/privacy", Component: PrivacyPagePage },
    { path: "/portal/terms", Component: TermsPagePage },
    {
      Component: PortalRoot,
      errorElement: routeError,
      children: [
        { path: "/portal/login", Component: PortalLoginPage },
        {
          element: <PortalProtectedRoute />,
          children: [
            {
              Component: PortalLayout,
              children: [
                { path: "/portal", Component: PortalDashboardPage },
                { path: "/portal/deals/:id", Component: PortalDealDetailPage },
                { path: "/portal/atendimentos/:id", Component: PortalLeadDetailPage },
                { path: "/portal/menu", Component: PortalMenuPage },
                { path: "/portal/finance", Component: PortalFinancePage },
                { path: "/portal/informe-rendimentos", Component: PortalInformeRendimentosPage },
                { path: "/portal/buscar", Component: PortalBuscarPage },
                { path: "/portal/mensagens", Component: PortalMessagesPage },
                { path: "/portal/chamados", Component: PortalChamadosPage },
                { path: "/portal/perfil", Component: PortalProfilePage },
                { path: "/portal/leads", Component: PortalLeadsPage },
                { path: "/portal/locacao", Component: PortalRentalsPage },
                { path: "/portal/locacao/:id", Component: PortalRentalDetailsPage },
                { path: "/portal/imoveis", Component: PortalImoveisPage },
                { path: "/portal/imoveis/:id", Component: PortalImovelDetailPage },
                { path: "/portal/notificacoes", Component: PortalNotificationsPage },
                { path: "/portal/suporte", Component: PortalSupportPage },
              ],
            },
          ],
        },
      ],
    },
  ];
}
