import { createBrowserRouter } from "react-router";
import { lazy, Suspense, type ComponentType } from "react";
import { RouteErrorFallback } from "./components/RouteErrorFallback";
import { RouteLoading } from "./components/RouteLoading";
import { ProtectedRoute } from "./components/ProtectedRoute";
import { RequirePermission } from "../shared/components/RequirePermission";
import { NotFoundRoute } from "../apps/admin/adminRouteRegistry";
import { AdminRoot } from "../apps/admin/AdminRoot";
import { getAdminLayoutRouteObject } from "../apps/admin/adminRouteRegistry";
import { getPortalRouteObjects } from "../apps/portal/portalRouteRegistry";

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

const LoginPage = withSuspense(lazy(() => import("./pages/Login").then((m) => ({ default: m.Login }))));
const ForgotPasswordPage = withSuspense(lazy(() => import("./pages/ForgotPassword").then((m) => ({ default: m.ForgotPassword }))));
const ResetPasswordPage = withSuspense(lazy(() => import("./pages/ResetPassword").then((m) => ({ default: m.ResetPassword }))));
const AcceptInvitePage = withSuspense(lazy(() => import("./pages/AcceptInvite").then((m) => ({ default: m.AcceptInvite }))));
const CompanySelectionPage = withSuspense(lazy(() => import("./pages/CompanySelection").then((m) => ({ default: m.CompanySelection }))));
const OnboardingPage = withSuspense(lazy(() => import("./pages/Onboarding").then((m) => ({ default: m.Onboarding }))));
const InteligenciaPage = withSuspense(lazy(() => import("./pages/Inteligencia")));

const PrivacyPagePage = withSuspense(lazy(() => import("./pages/PrivacyPage").then((m) => ({ default: m.PrivacyPage }))));
const TermsPagePage = withSuspense(lazy(() => import("./pages/TermsPage").then((m) => ({ default: m.TermsPage }))));

export const router = createBrowserRouter([
  ...getPortalRouteObjects(),
  {
    Component: AdminRoot,
    errorElement: routeError,
    children: [
      { path: "/login", Component: LoginPage },
      { path: "/forgot-password", Component: ForgotPasswordPage },
      { path: "/reset-password", Component: ResetPasswordPage },
      { path: "/invite", Component: AcceptInvitePage },
      { path: "/company-selection", Component: CompanySelectionPage },
      { path: "/onboarding", Component: OnboardingPage },
      { path: "/privacy", Component: PrivacyPagePage },
      { path: "/terms", Component: TermsPagePage },
      {
        element: <ProtectedRoute />,
        errorElement: routeError,
        children: [
          // P2/M19 — a rota declara a permissão que o servidor exige (`ai:use`).
          // A página já mostrava "acesso restrito" desde o P1/A8; declarar aqui
          // impede que a URL abra a casca do app para quem não tem a capacidade.
          {
            path: "inteligencia",
            element: (
              <RequirePermission permission="ai:use">
                <InteligenciaPage />
              </RequirePermission>
            ),
            errorElement: routeError,
          },
          getAdminLayoutRouteObject(),
          { path: "*", Component: NotFoundRoute, errorElement: routeError },
        ],
      },
    ],
  },
]);
