const EXACT_TITLES: Record<string, string> = {
  "/": "Painel",
  "/login": "Entrar",
  "/forgot-password": "Recuperar senha",
  "/reset-password": "Redefinir senha",
  "/invite": "Aceitar convite",
  "/company-selection": "Selecionar empresa",
  "/onboarding": "Onboarding",
  "/inteligencia": "Inteligência artificial",
  "/privacy": "Política de Privacidade",
  "/terms": "Termos de Uso",
  "/access-denied": "Acesso negado",

  "/leads": "Leads (CRM)",
  "/roleta": "Roleta de Leads",
  "/mensagens": "Mensagens",
  "/agenda": "Agenda",
  "/imoveis": "Imóveis",
  "/site": "Site de imóveis",
  "/captacoes": "Captações",
  "/contratos": "Contratos",
  "/modelos-contrato": "Modelos de Contrato",
  "/documentos": "Documentos",
  "/esteira": "Esteira Digital",
  "/financeiro": "Painel financeiro",
  "/financeiro/comissoes": "Comissões",
  "/financeiro/impostos": "Impostos retidos",
  "/financeiro/adiantamentos": "Adiantamentos",
  "/financeiro/contas": "Contas & encargos",
  "/financeiro/conciliacao": "Conferência financeira",
  "/financeiro/repasses": "Repasses",
  "/financeiro/cobrancas": "Recebimentos",
  "/financeiro/lancamentos": "Lançamentos",
  "/financeiro/payouts": "Ordens de pagamento",
  "/financeiro/payouts-lista": "Ordens de pagamento",
  "/financeiro/regua": "Régua de cobrança",
  "/financeiro/geracao": "Financeiro",
  "/financeiro/dashboard": "Painel financeiro",
  "/configuracoes/financeiro": "Configurações financeiras",
  "/inadimplencia": "Inadimplência",
  "/fiscal": "Fiscal",
  "/relatorios": "Relatórios",
  "/parceiros": "Parceiros & Franquias",
  "/locacao": "Contratos de locação",
  "/manutencao": "Manutenção",
  "/chamados": "Chamados",
  "/assinatura": "Assinatura & Consumo",
  "/audit-trail": "Trilha de auditoria",
  "/empresas": "Empresas",
  "/configuracoes": "Configurações",
  "/notificacoes": "Notificações",
  "/ajuda": "Ajuda",

  "/portal": "Portal do Cliente",
  "/portal/login": "Entrar no portal",
  "/portal/menu": "Menu",
  "/portal/finance": "Financeiro",
  "/portal/informe-rendimentos": "Informe de rendimentos",
  "/portal/buscar": "Buscar",
  "/portal/mensagens": "Mensagens",
  "/portal/chamados": "Chamados",
  "/portal/perfil": "Perfil",
  "/portal/leads": "Meus contatos",
  "/portal/locacao": "Meus contratos",
  "/portal/imoveis": "Meus imóveis",
  "/portal/notificacoes": "Notificações",
  "/portal/suporte": "Suporte",
  "/portal/privacy": "Política de Privacidade",
  "/portal/terms": "Termos de Uso",
};

const PATTERN_TITLES: Array<[RegExp, string]> = [
  [/^\/leads\/[^/]+$/, "Detalhes do lead"],
  [/^\/imoveis\/[^/]+$/, "Detalhes do imóvel"],
  [/^\/contratos\/[^/]+$/, "Detalhes do contrato"],
  [/^\/locacao\/[^/]+$/, "Detalhes da locação"],
  [/^\/esteira\/deals\/[^/]+$/, "Detalhes do negócio"],
  [/^\/portal\/deals\/[^/]+$/, "Detalhes do negócio"],
  [/^\/portal\/imoveis\/[^/]+$/, "Detalhes do imóvel"],
  [/^\/portal\/locacao\/[^/]+$/, "Detalhes da locação"],
  [/^\/financeiro\/[^/]+$/, "Financeiro"],
];

/**
 * Título da página a partir do pathname. É o mínimo para que cada rota tenha
 * identidade no histórico, na aba do navegador e em leitores de tela (WCAG
 * 2.4.2). Rotas dinâmicas recebem um título de contexto, sem inventar dado que
 * o shell ainda não carregou.
 */
export function titleForPath(pathname: string): string {
  const normalized = pathname !== "/" ? pathname.replace(/\/+$/, "") : "/";
  const exact = EXACT_TITLES[normalized];
  if (exact) return exact;
  for (const [pattern, title] of PATTERN_TITLES) {
    if (pattern.test(normalized)) return title;
  }
  return "Página não encontrada";
}

export const APP_TITLE = "suitee - Gestão Imobiliária Inteligente";

export function formatDocumentTitle(pathname: string): string {
  return `${titleForPath(pathname)} · suitee`;
}
