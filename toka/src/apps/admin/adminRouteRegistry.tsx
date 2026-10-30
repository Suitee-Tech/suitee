import { lazy, Suspense, type ComponentType } from "react";
import { Navigate, type RouteObject } from "react-router";
import { Layout } from "./Layout";
import { RequirePermission } from "../../shared/components/RequirePermission";
import { RouteErrorFallback } from "../../shared/components/RouteErrorFallback";
import { AdminRouteLoading } from "../../shared/components/AdminRouteLoading";
import { RequirePlanFeature } from "../../shared/components/RequirePlanFeature";
import { EsteiraRouteProviders } from "./EsteiraRouteProviders";

const routeError = <RouteErrorFallback />;

const withSuspense = <P extends object>(Component: ComponentType<P>) => {
  return function SuspenseWrapper(props: P) {
    return (
      <Suspense fallback={<AdminRouteLoading />}>
        <Component {...props} />
      </Suspense>
    );
  };
};

const DashboardPage = withSuspense(lazy(() => import("./Dashboard").then((m) => ({ default: m.Dashboard }))));
const LeadsPage = withSuspense(lazy(() => import("./Leads").then((m) => ({ default: m.Leads }))));
const LeadDetailsPage = withSuspense(lazy(() => import("./LeadDetails").then((m) => ({ default: m.LeadDetails }))));
const LeadRoulettePage = withSuspense(lazy(() => import("./pages/LeadRoulette").then((m) => ({ default: m.LeadRoulette }))));
const PropertiesPage = withSuspense(lazy(() => import("./pages/Properties").then((m) => ({ default: m.Properties }))));
const SitePage = withSuspense(lazy(() => import("./pages/Site").then((m) => ({ default: m.Site }))));
const PropertyDetailsPage = withSuspense(lazy(() => import("./pages/PropertyDetails").then((m) => ({ default: m.PropertyDetails }))));
const ContractsPage = withSuspense(lazy(() => import("./pages/Contracts").then((m) => ({ default: m.Contracts }))));
const ContractDetailsPage = withSuspense(lazy(() => import("./pages/ContractDetails").then((m) => ({ default: m.ContractDetails }))));
const ContractTemplatesPage = withSuspense(lazy(() => import("./pages/ContractTemplates").then((m) => ({ default: m.ContractTemplates }))));
const DocumentsPage = withSuspense(lazy(() => import("./pages/Documents").then((m) => ({ default: m.Documents }))));
const EsteiraPage = withSuspense(lazy(() => import("../../features/esteira/pages/Esteira").then((m) => ({ default: m.Esteira }))));
const EsteiraDealDetailPage = withSuspense(lazy(() => import("../../features/esteira/pages/EsteiraDealDetail").then((m) => ({ default: m.EsteiraDealDetail }))));
const FinancePage = withSuspense(lazy(() => import("./pages/Finance").then((m) => ({ default: m.Finance }))));
const LancamentosInboxPage = withSuspense(lazy(() => import("./pages/LancamentosInbox").then((m) => ({ default: m.LancamentosInbox }))));
const FiscalReportsPage = withSuspense(lazy(() => import("./pages/FiscalReports").then((m) => ({ default: m.FiscalReports }))));
const InadimplenciaPage = withSuspense(lazy(() => import("./pages/Inadimplencia").then((m) => ({ default: m.Inadimplencia }))));
const ReportsPage = withSuspense(lazy(() => import("./pages/Reports").then((m) => ({ default: m.Reports }))));
const RentalsPage = withSuspense(lazy(() => import("./pages/Rentals").then((m) => ({ default: m.Rentals }))));
const RentalDetailsPage = withSuspense(lazy(() => import("./pages/RentalDetails").then((m) => ({ default: m.RentalDetails }))));
const MaintenancePage = withSuspense(lazy(() => import("./pages/Maintenance").then((m) => ({ default: m.Maintenance }))));
const ChamadosPage = withSuspense(lazy(() => import("./pages/Chamados")));
const PartnersPage = withSuspense(lazy(() => import("./pages/Partners").then((m) => ({ default: m.Partners }))));
const MessagesPage = withSuspense(lazy(() => import("./pages/Messages").then((m) => ({ default: m.Messages }))));
const CalendarPage = withSuspense(lazy(() => import("./pages/Calendar").then((m) => ({ default: m.Calendar }))));
const SubscriptionPage = withSuspense(lazy(() => import("./pages/Subscription").then((m) => ({ default: m.Subscription }))));
const AuditTrailPage = withSuspense(lazy(() => import("./pages/AuditTrail").then((m) => ({ default: m.AuditTrail }))));
const EmpresasPage = withSuspense(lazy(() => import("./pages/Empresas").then((m) => ({ default: m.Empresas }))));
const SettingsPage = withSuspense(lazy(() => import("./pages/Settings").then((m) => ({ default: m.Settings }))));
const NotificationsPage = withSuspense(lazy(() => import("./pages/Notifications").then((m) => ({ default: m.Notifications }))));
const FinancialSettingsPage = withSuspense(lazy(() => import("./pages/FinancialSettings")));
const PayoutsPage = withSuspense(lazy(() => import("./pages/Payouts")));
const PayoutOrderTablePage = withSuspense(lazy(() => import("./pages/PayoutOrderTablePage")));
const ComissoesPage = withSuspense(lazy(() => import("./pages/ComissoesPage")));
const ImpostosRetidosPage = withSuspense(lazy(() => import("./pages/ImpostosRetidosPage")));
const AdiantamentosPage = withSuspense(lazy(() => import("./pages/AdiantamentosPage")));
const ContasPage = withSuspense(lazy(() => import("./pages/ContasPage")));
const PainelFinanceiroPage = withSuspense(lazy(() => import("./pages/PainelFinanceiroPage")));
const ConciliacaoPage = withSuspense(lazy(() => import("./pages/ConciliacaoPage")));
const RepassesPage = withSuspense(lazy(() => import("./pages/RepassesPage")));
const CobrancasPage = withSuspense(lazy(() => import("./pages/CobrancasPage")));
const ReguaPage = withSuspense(lazy(() => import("./pages/Regua")));
const HelpPage = withSuspense(lazy(() => import("./pages/Help")));
const AccessDeniedPage = withSuspense(lazy(() => import("./pages/AccessDenied")));
const CaptacoesPage = withSuspense(lazy(() => import("./pages/CaptacoesPage").then((m) => ({ default: m.CaptacoesPage }))));
const LegacyLancamentosRedirect = () => <Navigate to="/financeiro/lancamentos" replace />;
const LegacyFinanceDashboardRedirect = () => <Navigate to="/financeiro" replace />;

type AdminRouteDef = {
  index?: boolean;
  path?: string;
  Component: ComponentType<any>;
  permissions?: string[];
  planFeature?: string;
};

function withPermissions(node: React.ReactNode, permissions: string[]) {
  return permissions.reduceRight((acc, perm) => <RequirePermission permission={perm}>{acc}</RequirePermission>, node);
}

const LAYOUT_ROUTES: AdminRouteDef[] = [
  { index: true, Component: DashboardPage },
  { path: "leads", Component: LeadsPage, permissions: ["module:crm", "leads:read"] },
  { path: "leads/:id", Component: LeadDetailsPage, permissions: ["module:crm", "leads:read"] },
  { path: "roleta", Component: LeadRoulettePage, permissions: ["module:crm", "leads:configure_roulette"] },
  { path: "imoveis", Component: PropertiesPage, permissions: ["module:portfolio", "properties:read"] },
  { path: "captacoes", Component: CaptacoesPage, permissions: ["module:portfolio", "properties:capture"] },
  { path: "imoveis/:id", Component: PropertyDetailsPage, permissions: ["module:portfolio", "properties:read"] },
  // ADR 0006: configuração da vitrine (host, branding, páginas). Não confundir
  // com publicar imóvel — isso continua em /imoveis/:id (properties:write).
  { path: "site", Component: SitePage, permissions: ["module:portfolio", "sites:read"] },
  { path: "contratos", Component: ContractsPage, permissions: ["module:portfolio", "contracts:read"] },
  { path: "contratos/:id", Component: ContractDetailsPage, permissions: ["module:portfolio", "contracts:read"] },
  { path: "modelos-contrato", Component: ContractTemplatesPage, permissions: ["module:portfolio", "templates:read"] },
  { path: "documentos", Component: DocumentsPage, permissions: ["module:portfolio", "documents:read"] },
  { path: "esteira", Component: EsteiraPage, permissions: ["module:portfolio", "deals:read"] },
  { path: "esteira/deals/:id", Component: EsteiraDealDetailPage, permissions: ["module:portfolio", "deals:read"] },
  { path: "financeiro", Component: PainelFinanceiroPage, permissions: ["module:finance", "finance:read"] },
  // Áreas canônicas do Financeiro. Os deep-links antigos redirecionam para a
  // mesma experiência para evitar duas implementações concorrentes.
  { path: "financeiro/comissoes", Component: ComissoesPage, permissions: ["module:finance", "finance:read"] },
  { path: "financeiro/impostos", Component: ImpostosRetidosPage, permissions: ["module:finance", "finance:read"] },
  { path: "financeiro/adiantamentos", Component: AdiantamentosPage, permissions: ["module:finance", "finance:read"] },
  { path: "financeiro/contas", Component: ContasPage, permissions: ["module:finance", "finance:read"] },
  { path: "financeiro/conciliacao", Component: ConciliacaoPage, permissions: ["module:finance", "finance:read"] },
  { path: "financeiro/repasses", Component: RepassesPage, permissions: ["module:finance", "finance:read"] },
  { path: "financeiro/cobrancas", Component: CobrancasPage, permissions: ["module:finance", "finance:read"] },
  { path: "financeiro/lancamentos", Component: LancamentosInboxPage, permissions: ["module:finance", "finance:read"] },
  { path: "financeiro/geracao", Component: LegacyLancamentosRedirect, permissions: ["module:finance", "finance:read"] },
  { path: "financeiro/dashboard", Component: LegacyFinanceDashboardRedirect, permissions: ["module:finance", "finance:read"] },
  { path: "financeiro/payouts", Component: PayoutOrderTablePage, permissions: ["module:finance", "finance:read"] },
  { path: "financeiro/payouts-lista", Component: PayoutsPage, permissions: ["module:finance", "finance:read"] },
  { path: "financeiro/regua", Component: ReguaPage, permissions: ["module:finance", "charges:write"] },
  { path: "financeiro/:module", Component: FinancePage, permissions: ["module:finance", "finance:read"] },
  { path: "configuracoes/financeiro", Component: FinancialSettingsPage, permissions: ["module:finance", "finance:write"] },
  { path: "inadimplencia", Component: InadimplenciaPage, permissions: ["module:finance", "charges:read"] },
  { path: "fiscal", Component: FiscalReportsPage, permissions: ["module:finance", "fiscal:read"] },
  { path: "relatorios", Component: ReportsPage, permissions: ["module:intelligence", "reports:read"], planFeature: "reports" },
  // P2/M20 — a rota declarava só `module:intelligence` (o módulo do menu),
  // enquanto a página e a API operam `partnerships:read`/`partnerships:write`.
  // Subdeclarar deixava a URL aberta para quem tem o menu mas não o domínio.
  { path: "parceiros", Component: PartnersPage, permissions: ["module:intelligence", "partnerships:read"] },
  { path: "mensagens", Component: MessagesPage, permissions: ["module:crm", "messages:read"] },
  // P2/M19 — a rota declarava só `module:crm` (o módulo do menu), e `module:*`
  // governa NAVEGAÇÃO: nenhuma rota de backend lê esse prefixo, então a tela
  // ficava guardada apenas pelo menu. A agenda é servida pelo toca-calendar, e
  // desde o P1/A8 os endpoints dela exigem `deals:read` (leitura) e
  // `deals:write` (escrita). A tela declara a permissão de LEITURA, que é a que
  // abre a lista; exigir `deals:write` esconderia a agenda de `financeiro` e
  // `auditor`, que a matriz lista como leitores da esteira.
  { path: "agenda", Component: CalendarPage, permissions: ["module:crm", "deals:read"] },
  { path: "locacao", Component: RentalsPage, permissions: ["module:rentals", "rentals:read"] },
  { path: "locacao/:id", Component: RentalDetailsPage, permissions: ["module:rentals", "rentals:read"] },
  { path: "manutencao", Component: MaintenancePage, permissions: ["module:rentals", "rentals:inspect"] },
  { path: "chamados", Component: ChamadosPage, permissions: ["module:rentals", "tickets:read"] },
  { path: "assinatura", Component: SubscriptionPage, permissions: ["module:admin"] },
  { path: "audit-trail", Component: AuditTrailPage, permissions: ["system:view_audit_trail"] },
  { path: "empresas", Component: EmpresasPage, permissions: ["company:read_members"] },
  { path: "configuracoes", Component: SettingsPage },
  { path: "notificacoes", Component: NotificationsPage },
  // Central de Ajuda: aberta a todos os usuários autenticados (sem permissão).
  { path: "ajuda", Component: HelpPage },
  { path: "access-denied", Component: AccessDeniedPage },
];

export function getAdminLayoutRoutes(): RouteObject[] {
  return LAYOUT_ROUTES.map((r) => {
    if (r.index) {
      return {
        index: true,
        errorElement: routeError,
        element: r.permissions ? withPermissions(<r.Component />, r.permissions) : <r.Component />,
      };
    }
    const isEsteiraRoute = r.path === "esteira" || r.path === "esteira/deals/:id";
    const element = isEsteiraRoute ? (
      <EsteiraRouteProviders>
        <r.Component />
      </EsteiraRouteProviders>
    ) : (
      <r.Component />
    );

    const planGuardedElement = r.planFeature
      ? <RequirePlanFeature feature={r.planFeature}>{element}</RequirePlanFeature>
      : element;
    if (!r.permissions) return { path: r.path, errorElement: routeError, element: planGuardedElement };
    return { path: r.path, errorElement: routeError, element: withPermissions(planGuardedElement, r.permissions) };
  });
}

export function getAdminLayoutRouteObject(): RouteObject {
  return {
    path: "/",
    Component: Layout,
    errorElement: routeError,
    children: getAdminLayoutRoutes(),
  };
}

// NotFoundRoute fica FORA do Layout de propósito (P2/M19): um catch-all dentro
// do shell autenticado renderizava a aplicação inteira — menu, contexto de
// empresa e providers — para qualquer URL desconhecida, inclusive para quem não
// tem o módulo correspondente. Aqui a resposta é uma página neutra, dentro da
// árvore autenticada (ProtectedRoute) e sem o chrome do app.
export function NotFoundRoute() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-8 text-center text-muted-foreground">
      Página não encontrada.
    </div>
  );
}
