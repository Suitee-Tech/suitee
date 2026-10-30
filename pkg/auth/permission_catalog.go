package auth

type PermissionDef struct {
	ID           string
	Label        string
	Module       string
	DefaultRoles map[string]bool
}

var PermissionCatalog = []PermissionDef{
	{ID: "module:crm", Label: "Módulo CRM & Relacionamento", Module: "Menus", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true, "operacoes": true, "marketing": true, "auditor": true}},
	{ID: "module:portfolio", Label: "Módulo Portfólio & Negócios", Module: "Menus", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true, "operacoes": true, "financeiro": true, "marketing": true, "auditor": true}},
	{ID: "module:rentals", Label: "Módulo Gestão de Locação", Module: "Menus", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true, "operacoes": true, "financeiro": true, "auditor": true}},
	{ID: "module:finance", Label: "Módulo Financeiro", Module: "Menus", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true, "financeiro": true, "auditor": true}},
	{ID: "module:intelligence", Label: "Módulo Inteligência & Expansão", Module: "Menus", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true, "financeiro": true, "marketing": true, "auditor": true}},
	{ID: "module:admin", Label: "Módulo Administração", Module: "Menus", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true, "auditor": true}},

	// Defaults do módulo Leads alinhados à matriz documentada
	// (docs/features/ROLES-MATRIX.md:40-43): `operacoes` tem W/R no CRM e
	// `auditor` tem R. O backend omitia `operacoes` de todas elas — como as
	// rotas de leads passaram a exigir a permissão explícita (P1/A10), a omissão
	// bloquearia um papel que a documentação (e o produto) trata como usuário
	// pleno de CRM, inclusive nos enriquecimentos de tela de deals/contratos.
	{ID: "leads:read", Label: "Visualizar Leads", Module: "Leads", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true, "operacoes": true, "marketing": true, "auditor": true}},
	{ID: "leads:write", Label: "Editar Leads", Module: "Leads", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true, "operacoes": true}},
	{ID: "leads:reassign", Label: "Reatribuir Leads", Module: "Leads", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true}},
	// Defaults alinhados à matriz documentada (docs/features/ROLES-MATRIX.md:43
	// "Leads — import/export": admin, gestor, operacoes, marketing). O backend só
	// tinha admin/gestor, então operacoes/marketing viam o botão no frontend (que
	// usa estes defaults) e levariam 403 na API.
	// P1/A7 — `leads:delete` foi REMOVIDA do catálogo: não existe rota
	// `DELETE /leads/:id` em nenhum serviço e o frontend não consome a permissão
	// (a exclusão de lead no produto acontece por conversão/anonimização e pelo
	// caminho de LGPD). Manter uma permissão que não governa nada faz o mapa de
	// autorização mentir; se a exclusão for implementada, a permissão volta —
	// o catálogo é gerado e o CI (gen-permission-catalog -check) sincroniza BE/FE.
	{ID: "leads:import_export", Label: "Importar/Exportar Leads", Module: "Leads", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true, "marketing": true}},
	{ID: "leads:configure_roulette", Label: "Configurar Roleta", Module: "Leads", DefaultRoles: map[string]bool{"admin": true, "gestor": true}},
	{ID: "leads:view_roulette", Label: "Visualizar Roleta (somente leitura)", Module: "Leads", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true, "operacoes": true, "auditor": true}},

	{ID: "brokers:read", Label: "Visualizar Corretores", Module: "Corretores", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true}},
	{ID: "brokers:write", Label: "Editar Corretores", Module: "Corretores", DefaultRoles: map[string]bool{"admin": true, "gestor": true}},
	{ID: "brokers:delete", Label: "Excluir Corretores", Module: "Corretores", DefaultRoles: map[string]bool{"admin": true}},

	{ID: "properties:read", Label: "Visualizar Imóveis", Module: "Imóveis", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true, "operacoes": true, "marketing": true}},
	// P1/A7 — `properties:publish` foi REMOVIDA: publicar não tem endpoint próprio
	// nem consumidor no frontend; a publicação é um campo
	// (`publication_status`) gravado pelo update de imóvel, guardado por
	// `properties:write`.
	{ID: "properties:write", Label: "Editar Imóveis", Module: "Imóveis", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true, "operacoes": true}},
	{ID: "properties:delete", Label: "Excluir Imóveis", Module: "Imóveis", DefaultRoles: map[string]bool{"admin": true, "gestor": true}},
	// `properties:capture` is the dedicated permission for the captação flow
	// (registering a new property in the portfolio with owner-lead + capturing
	// brokers). Kept distinct from `properties:write` so a tenant can grant
	// generic editing without granting the right to onboard new portfolio
	// entries — common when ops/marketing edit listings but only senior
	// brokers/managers should captar.
	{ID: "properties:capture", Label: "Captar Imóveis", Module: "Imóveis", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true}},

	// ADR 0006 — vitrine pública da imobiliária (`{marca}.imoveis.suitee.com.br`).
	// `sites:write` governa ligar/desligar o site, escolher a marca do host,
	// branding e as três páginas institucionais. Publicar IMÓVEL continua sendo
	// `properties:write` (o site reage ao evento property.updated); por isso não
	// existe uma permissão de "publicar no site" separada.
	{ID: "sites:read", Label: "Ver Site de Imóveis", Module: "Site de Imóveis", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true, "marketing": true}},
	{ID: "sites:write", Label: "Configurar Site de Imóveis", Module: "Site de Imóveis", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "marketing": true}},

	// P1/A8: `financeiro` e `auditor` entram por alinhamento à matriz
	// (docs/features/ROLES-MATRIX.md, "Esteira (deals) — ver": admin, gestor,
	// corretor, operacoes, financeiro, auditor). Foi necessário antes de aplicar o
	// guard na agenda do toca-calendar, que usa `deals:read` para leitura.
	{ID: "deals:read", Label: "Visualizar Esteira", Module: "Esteira", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true, "operacoes": true, "financeiro": true, "auditor": true}},
	{ID: "deals:write", Label: "Editar Esteira", Module: "Esteira", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true}},
	{ID: "deals:delete", Label: "Excluir Esteira", Module: "Esteira", DefaultRoles: map[string]bool{"admin": true, "gestor": true}},
	{ID: "deals:approve", Label: "Aprovar Etapas", Module: "Esteira", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true}},

	{ID: "contracts:read", Label: "Visualizar Contratos", Module: "Contratos", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true, "financeiro": true}},
	{ID: "contracts:write", Label: "Editar Contratos", Module: "Contratos", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true}},
	{ID: "contracts:delete", Label: "Excluir Contratos", Module: "Contratos", DefaultRoles: map[string]bool{"admin": true, "operacoes": true}},
	// Defaults alinhados à matriz documentada (ROLES-MATRIX.md:59 "Contratos —
	// assinar": admin, gestor, operacoes, financeiro; parceiro/cliente assinam pelo
	// provedor externo, não por esta rota). A rota /contracts/:id/sign passou a
	// exigir esta permissão (P0/C12).
	{ID: "contracts:sign", Label: "Assinar Contratos", Module: "Contratos", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true, "financeiro": true}},

	{ID: "templates:read", Label: "Visualizar Modelos", Module: "Modelos", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true}},
	{ID: "templates:write", Label: "Editar Modelos", Module: "Modelos", DefaultRoles: map[string]bool{"admin": true, "gestor": true}},
	{ID: "templates:delete", Label: "Excluir Modelos", Module: "Modelos", DefaultRoles: map[string]bool{"admin": true}},

	{ID: "documents:read", Label: "Visualizar Documentos", Module: "Documentos", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true, "operacoes": true, "financeiro": true}},
	{ID: "documents:write", Label: "Editar Documentos", Module: "Documentos", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true}},
	{ID: "documents:delete", Label: "Excluir Documentos", Module: "Documentos", DefaultRoles: map[string]bool{"admin": true, "operacoes": true}},
	{ID: "documents:validate", Label: "Validar Documentos", Module: "Documentos", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true}},

	{ID: "finance:read", Label: "Visualizar Financeiro", Module: "Financeiro", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "financeiro": true, "auditor": true}},
	{ID: "finance:write", Label: "Editar Financeiro", Module: "Financeiro", DefaultRoles: map[string]bool{"admin": true, "financeiro": true}},
	// A comissão de uma NEGOCIAÇÃO (percentual e divisão por papel) é decisão de
	// gestão: admin, gestor, operações e financeiro personalizam. O corretor NÃO
	// entra por padrão — antes o gate era `deals:write`, que o corretor tem, e ele
	// conseguia editar o próprio percentual na esteira. O tenant pode ajustar na
	// matriz de permissões.
	{ID: "commissions:write", Label: "Editar Comissão de Negociação", Module: "Financeiro", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true, "financeiro": true}},
	// P1/A7 — capacidades de UI, não de servidor: a exportação de cobranças e de
	// relatórios é gerada no NAVEGADOR (ExportReportModal.tsx monta o CSV/PDF a
	// partir dos dados que a tela já carregou sob a permissão de leitura). Não
	// existe endpoint de exportação a guardar; manter as permissões é o que
	// permite esconder o botão de quem não deve exportar, e é a única enforcement
	// possível. Registrado aqui para ninguém supor guarda de servidor.
	{ID: "finance:export", Label: "Exportar Financeiro", Module: "Financeiro", DefaultRoles: map[string]bool{"admin": true, "financeiro": true, "auditor": true}},
	{ID: "finance:approve_transfer", Label: "Aprovar Transferências", Module: "Financeiro", DefaultRoles: map[string]bool{"admin": true, "financeiro": true}},
	// P2/M11 — segunda assinatura para waive/adjust de valores (cobranças e
	// deduções de comissão acima do limiar). Default SÓ admin de propósito: quem
	// edita valores no dia a dia tem `finance:write`/`charges:write`
	// (admin+financeiro), então conceder a aprovação a `financeiro` por default
	// anularia a segregação de funções que o achado pede. O tenant pode
	// customizar na matriz.
	{ID: "finance:approve_adjustment", Label: "Aprovar Ajustes de Valor", Module: "Financeiro", DefaultRoles: map[string]bool{"admin": true}},

	{ID: "charges:read", Label: "Visualizar Cobranças", Module: "Cobranças", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "financeiro": true, "operacoes": true}},
	// P1/A7: `operacoes` entra por alinhamento à matriz (docs/features/ROLES-MATRIX.md,
	// "Cobranças — editar" = admin, operacoes, financeiro). Sem isso, a tela de
	// locação mostra a `operacoes` os botões de conta/régua (gated por
	// `rentals:write`, removida) e o servidor respondia 403 — a inconsistência que
	// a rodada P1 encontrou.
	{ID: "charges:write", Label: "Editar Cobranças", Module: "Cobranças", DefaultRoles: map[string]bool{"admin": true, "operacoes": true, "financeiro": true}},

	// Inadimplência — permissões DEDICADAS do domínio de cobrança judicializada.
	//
	// Antes, acionar garantia, iniciar processo judicial, fechar acordo e disparar
	// o motor estavam todos sob `charges:write`, a mesma permissão do dia a dia de
	// cobrança: quem lança uma cobrança podia judicializar a dívida de um cliente.
	//
	// A implantação é ADITIVA de propósito: as rotas passam a aceitar a permissão
	// nova OU a antiga (`RequireAnyPermission`), então ninguém perde acesso no dia
	// do deploy e a imobiliária pode dar a granularidade a quem precisa — revogar
	// `charges:write` de um perfil e deixar só `collections:read`, por exemplo.
	// O papel `auditor` recebe apenas leitura; `judicial` fica com admin e gestor
	// (é a ação irreversível de maior alcance).
	{ID: "collections:read", Label: "Visualizar Inadimplência", Module: "Inadimplência", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "financeiro": true, "operacoes": true, "auditor": true}},
	{ID: "collections:write", Label: "Editar Casos de Cobrança", Module: "Inadimplência", DefaultRoles: map[string]bool{"admin": true, "operacoes": true, "financeiro": true}},
	{ID: "collections:negotiate", Label: "Negociar Acordo", Module: "Inadimplência", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "financeiro": true}},
	{ID: "collections:guarantee", Label: "Acionar Garantia", Module: "Inadimplência", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "financeiro": true}},
	{ID: "collections:judicial", Label: "Processo Judicial", Module: "Inadimplência", DefaultRoles: map[string]bool{"admin": true, "gestor": true}},
	{ID: "delinquency:run", Label: "Processar Inadimplência", Module: "Inadimplência", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "financeiro": true}},

	{ID: "fiscal:read", Label: "Visualizar Fiscal", Module: "Fiscal", DefaultRoles: map[string]bool{"admin": true, "financeiro": true, "auditor": true}},
	{ID: "fiscal:generate", Label: "Gerar Notas/DIMOB", Module: "Fiscal", DefaultRoles: map[string]bool{"admin": true, "financeiro": true}},

	{ID: "rentals:read", Label: "Visualizar Locação", Module: "Locação", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true}},
	// Defaults alinhados à matriz documentada (ROLES-MATRIX.md:75 "Locação —
	// vistorias": admin, gestor, corretor, operacoes). O backend omitia `gestor`;
	// como a rota de vistorias agora exige esta permissão (P0/C8), a omissão
	// viraria 403 para um papel que a documentação diz que pode.
	// P1/A7 — `rentals:write` foi REMOVIDA: não há rota de escrita de locação. As
	// escritas que a tela de locação dispara (contas de consumo, régua de
	// lembretes, configuração de cobrança do contrato) são cobrança e já são
	// guardadas por `charges:write` no toca-financial; o frontend passou a usar a
	// mesma permissão que o servidor exige. A linha "Locação — editar" da matriz
	// é servida por `charges:write`.
	{ID: "rentals:inspect", Label: "Vistorias", Module: "Locação", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true, "corretor": true}},

	{ID: "tickets:read", Label: "Ver Chamados", Module: "Chamados", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true, "operacoes": true, "auditor": true}},
	{ID: "tickets:write", Label: "Editar Chamados", Module: "Chamados", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true}},

	// P1/A8 — `messages:*` nasce do achado "toca-comm: 40 rotas sem nenhuma
	// verificação de permissão": busca, autocomplete, métricas e templates de
	// mensagem eram acessíveis a qualquer autenticado não-`cliente`. O chat é uma
	// superfície de CRM (a tela `/mensagens` exige `module:crm`), então o default
	// espelha essa população; `auditor` lê e não escreve.
	{ID: "messages:read", Label: "Ver Mensagens", Module: "Comunicação", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true, "operacoes": true, "marketing": true, "auditor": true}},
	{ID: "messages:write", Label: "Editar Mensagens", Module: "Comunicação", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true, "operacoes": true, "marketing": true}},

	// P1/A8 — `ai:use` nasce do achado "toca-ai: 10 rotas sem permissão por tool".
	// O assistente é um painel no layout do admin (disponível para todo usuário
	// autenticado) além da página `/inteligencia` (que não tinha guarda nenhuma no
	// cliente). O default espelha essa população — inclusive `corretor` —, então
	// nada muda para quem já usa; o ganho é a capacidade passar a ser NOMEADA e
	// revogável por tenant (antes não havia o que revogar).
	// A autorização POR TOOL (cada tool do agente exigindo a permissão do domínio
	// que ela toca) continua pendente e registrada como item de acompanhamento.
	{ID: "ai:use", Label: "Usar Inteligência Artificial", Module: "Inteligência", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true, "operacoes": true, "financeiro": true, "marketing": true, "auditor": true, "parceiro": true}},

	{ID: "customers:read", Label: "Ver Clientes", Module: "Clientes", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true, "operacoes": true, "financeiro": true, "auditor": true}},

	{ID: "reports:read", Label: "Visualizar Relatórios", Module: "Relatórios", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "financeiro": true, "operacoes": true, "marketing": true, "auditor": true}},
	{ID: "reports:export", Label: "Exportar Relatórios", Module: "Relatórios", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "financeiro": true, "auditor": true}},

	// Company
	// P1/A8: defaults alinhados à matriz documentada
	// (docs/features/ROLES-MATRIX.md, "Empresa & Equipe"):
	//   ver      → todos os papéis de operador (a lista de membros alimenta
	//              seletores e nomes de participante no chat, então negar a
	//              leitura a corretor/marketing quebra tela sem ganho de
	//              segurança — os dados são do próprio tenant);
	//   gerenciar → admin, gestor e operacoes.
	{ID: "company:read_members", Label: "Ver Membros", Module: "Empresa & Equipe", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true, "operacoes": true, "financeiro": true, "marketing": true, "auditor": true}},
	{ID: "company:manage_members", Label: "Gerenciar Empresa & Equipe", Module: "Empresa & Equipe", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true}},

	// Partnerships & Franchises — dedicated keys for the partnership lifecycle
	// (create/accept/reject + financial/sharing/permission config) and for
	// reading the agreements list. Distinct from company:manage_members so a
	// tenant can let ops/brokers see partnerships without granting blanket
	// company administration.
	{ID: "partnerships:read", Label: "Ver Parcerias e Franquias", Module: "Parceiros & Franquias", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true, "corretor": true, "auditor": true}},
	{ID: "partnerships:write", Label: "Gerenciar Parcerias e Franquias", Module: "Parceiros & Franquias", DefaultRoles: map[string]bool{"admin": true, "gestor": true}},

	// Shared Content — partner-shared portfolio/leads. `read` gates seeing
	// the aggregated shared properties/leads; `operate` gates acting on them
	// (starting a co-brokering deal with a partner's property/lead).
	{ID: "shared_content:read", Label: "Ver Conteúdo Compartilhado", Module: "Conteúdo Compartilhado", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "operacoes": true, "corretor": true}},
	{ID: "shared_content:operate", Label: "Operar Conteúdo Compartilhado", Module: "Conteúdo Compartilhado", DefaultRoles: map[string]bool{"admin": true, "gestor": true, "corretor": true}},

	{ID: "settings:manage_appearance", Label: "Aparência", Module: "Configurações", DefaultRoles: map[string]bool{"admin": true, "marketing": true}},
	{ID: "settings:manage_permissions", Label: "Permissões", Module: "Configurações", DefaultRoles: map[string]bool{"admin": true}},
	{ID: "settings:manage_general", Label: "Geral", Module: "Configurações", DefaultRoles: map[string]bool{"admin": true}},
	{ID: "settings:manage_notifications", Label: "Notificações", Module: "Configurações", DefaultRoles: map[string]bool{"admin": true}},
	{ID: "settings:manage_integrations", Label: "Integrações", Module: "Configurações", DefaultRoles: map[string]bool{"admin": true}},

	{ID: "system:view_audit_trail", Label: "Auditoria", Module: "Sistema", DefaultRoles: map[string]bool{"admin": true, "auditor": true, "operacoes": true}},
}
