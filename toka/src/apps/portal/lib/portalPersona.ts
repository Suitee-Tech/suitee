/**
 * Persona do cliente no portal — QUEM ele é naquele negócio.
 *
 * O portal atendia comprador, locatário, vendedor, locador, fiador e testemunha
 * com o mesmo texto: um proprietário que anunciou o imóvel via "Explorar
 * imóveis", "Selecionados para você" e "Agendar visita" — fluxos que não são
 * dele — e um comprador via "Minhas locações" e "Meus boletos". Texto genérico
 * em fluxo específico induz a ação errada.
 *
 * Aqui fica a única decisão de vocabulário: o papel da parte (party role) cruzado
 * com o tipo do negócio (SALE/RENT) define o ator, e o ator define o dicionário
 * de textos. Nenhuma tela deve decidir o vocabulário por conta própria.
 *
 * Papéis do backend (`deal.PartyRole`): tenant, buyer, owner, guarantor,
 * witness, spouse, attorney, legal_representative, guardian, curator.
 * Papel do lead no CRM (`lead.ClientRole`): owner, buyer, tenant, guarantor.
 */

export type PortalActor =
  | "buyer"
  | "tenant"
  | "seller"
  | "landlord"
  /** owner sem tipo de negócio conhecido: não dá para dizer vendedor/locador. */
  | "owner"
  | "guarantor"
  | "witness"
  /** Sem papel conhecido: texto neutro, sem afirmar de que lado o cliente está. */
  | "party";

export type PortalDealType = "SALE" | "RENT" | null;

export interface PortalPersona {
  /** Papel bruto da parte, normalizado (buyer, tenant, owner, ...). */
  role: string | null;
  dealType: PortalDealType;
  actor: PortalActor;
  /** Rótulo do papel no vocabulário do cliente ("Comprador", "Locador"...). */
  label: string;
  /** Procura/recebe o imóvel (comprador ou locatário). */
  isSeeker: boolean;
  /** Oferece o imóvel (vendedor, locador ou proprietário sem tipo). */
  isOwnerSide: boolean;
  isRent: boolean;
  isSale: boolean;
  /** Sem papel identificado: a tela usa vocabulário neutro. */
  isUnidentified: boolean;
  /** "Compra e venda" | "Locação" — só quando o tipo do negócio é conhecido. */
  dealKindLabel: string | null;
}

/** Rótulo do papel da parte — fonte única para selos e listas. */
export const PORTAL_ROLE_LABELS: Record<string, string> = {
  tenant: "Locatário",
  buyer: "Comprador",
  owner: "Proprietário",
  seller: "Vendedor",
  landlord: "Locador",
  guarantor: "Fiador(a)",
  witness: "Testemunha",
  spouse: "Cônjuge",
  attorney: "Procurador(a)",
  legal_representative: "Representante legal",
  guardian: "Tutor(a)",
  curator: "Curador(a)",
};

export function portalRoleLabel(role?: string | null): string {
  const key = String(role ?? "").trim().toLowerCase();
  if (!key) return "Parte do negócio";
  return PORTAL_ROLE_LABELS[key] ?? key;
}

export function normalizeDealType(dealType?: string | null): PortalDealType {
  const value = String(dealType ?? "").trim().toUpperCase();
  if (value === "SALE" || value === "RENT") return value;
  return null;
}

export function resolvePortalPersona(input: {
  role?: string | null;
  dealType?: string | null;
} = {}): PortalPersona {
  const role = String(input.role ?? "").trim().toLowerCase() || null;
  const dealType = normalizeDealType(input.dealType);
  const isRent = dealType === "RENT";
  const isSale = dealType === "SALE";

  let actor: PortalActor;
  switch (role) {
    case "owner":
      // Vendedor × locador NÃO vem do papel (é sempre "owner"), vem do negócio.
      actor = isRent ? "landlord" : isSale ? "seller" : "owner";
      break;
    case "buyer":
      actor = "buyer";
      break;
    case "tenant":
      actor = "tenant";
      break;
    case "guarantor":
      actor = "guarantor";
      break;
    case "witness":
      actor = "witness";
      break;
    default:
      actor = "party";
  }

  const label =
    actor === "seller" ? "Vendedor"
      : actor === "landlord" ? "Locador"
        : portalRoleLabel(role);

  return {
    role,
    dealType,
    actor,
    label,
    isSeeker: actor === "buyer" || actor === "tenant",
    isOwnerSide: actor === "seller" || actor === "landlord" || actor === "owner",
    isRent,
    isSale,
    isUnidentified: actor === "party",
    dealKindLabel: isSale ? "Compra e venda" : isRent ? "Locação" : null,
  };
}

export interface PortalQuickAction {
  key: "search" | "properties" | "finance" | "rentals" | "tickets" | "contact";
  to: string;
  label: string;
}

/**
 * Visitas dentro do detalhe do negócio.
 *
 * `canSchedule` separa quem visita de quem recebe a visita: comprador e
 * locatário agendam; vendedor e locador NÃO agendam visita ao próprio imóvel —
 * a imobiliária conduz com os interessados. Sem isso, o card "Próxima ação"
 * mandava o dono do imóvel "conhecer o imóvel de perto".
 */
export interface PortalVisitCopy {
  canSchedule: boolean;
  /** Texto de abertura do card de visitas na etapa de negociação. */
  sectionIntro: string;
  notScheduled: { title: string; description: string; label: string };
  scheduled: { title: string; description: string; label: string };
}

const SEEKER_VISITS: PortalVisitCopy = {
  canSchedule: true,
  sectionIntro: "Sua transação iniciou. Abaixo você pode conferir as visitas agendadas.",
  notScheduled: {
    title: "Conheça o imóvel de perto",
    description: "Solicite uma data e um horário. A imobiliária confirmará a disponibilidade.",
    label: "Agendar visita",
  },
  scheduled: {
    title: "Acompanhe sua visita",
    description: "Confira o horário e a confirmação da imobiliária. Você também pode solicitar outro horário.",
    label: "Ver agendamento",
  },
};

const NEUTRAL_VISITS: PortalVisitCopy = {
  ...SEEKER_VISITS,
  notScheduled: { ...SEEKER_VISITS.notScheduled, label: "Solicitar visita" },
};

/** Lado do dono: a visita é conduzida pela imobiliária, não marcada por ele. */
const OWNER_VISITS: PortalVisitCopy = {
  canSchedule: false,
  sectionIntro:
    "A imobiliária conduz as visitas ao seu imóvel. Acompanhe aqui o que já foi agendado com os interessados.",
  notScheduled: {
    title: "Visitas ao seu imóvel",
    description: "A imobiliária agenda as visitas com os interessados. O andamento aparece aqui.",
    label: "Acompanhar visitas",
  },
  scheduled: {
    title: "Visitas ao seu imóvel",
    description: "Veja os horários que a imobiliária agendou com os interessados.",
    label: "Ver agendamento",
  },
};

export type PortalStageKey =
  | "NEGOTIATION"
  | "PROPOSAL"
  | "ANALYSIS"
  | "CONTRACT"
  | "CONVERTED"
  | "CANCELLED"
  | string;

export interface PortalCopy {
  /** Navegação: para onde vai "Imóveis" e como se chama. */
  propertiesNavLabel: string;
  propertiesNavPath: string;
  /** Jornada do cliente ainda sem negócio na esteira. */
  journeyEyebrow: string;
  journeyBody: string;
  journeyPrimaryCta: { to: string; label: string };
  journeySecondaryCta: { to: string; label: string };
  /** Home com negócio ativo. */
  overviewEyebrow: string;
  stageHeadline: Record<string, string>;
  primaryAction: Record<string, string>;
  /** null ⇒ a seção de recomendações não se aplica à persona. */
  recommendationsTitle: string | null;
  recommendationsIntro: string;
  quickActions: PortalQuickAction[];
  /** Detalhe do imóvel. */
  propertyVisitCta: string | null;
  propertyVisitReschedule: string;
  /** Explica quem conduz a visita quando a persona não agenda (lado do dono). */
  propertyVisitOwnerHint: string | null;
  propertyContactCta: string;
  propertyAddressHint: string;
  /** Visitas. */
  visitsTitle: string;
  visitsEmptyBody: string;
  /** Meus imóveis. */
  propertiesTitle: string;
  propertiesEmptyTitle: string;
  propertiesEmptyBody: string;
  /** Financeiro. */
  financeTitle: string;
  financeEntryLabel: string;
  repassesTitle: string;
  /** Chamados. */
  ticketsTitle: string;
  /** Propostas. */
  proposalCta: string;
  /** Visitas no detalhe do negócio. */
  visits: PortalVisitCopy;
}

const SEEKER_STAGE_HEADLINES: Record<string, string> = {
  NEGOTIATION: "Vamos acompanhar cada próximo passo.",
  PROPOSAL: "Sua proposta está pronta para revisão.",
  ANALYSIS: "Sua documentação está em análise.",
  CONTRACT: "Seu contrato está na reta final.",
  CONVERTED: "Seu negócio foi concluído.",
  CANCELLED: "Este negócio foi encerrado.",
};

const NEUTRAL_STAGE_HEADLINES: Record<string, string> = {
  NEGOTIATION: "Acompanhe aqui as etapas do seu negócio.",
  PROPOSAL: "A proposta do negócio está em revisão.",
  ANALYSIS: "A documentação do negócio está em análise.",
  CONTRACT: "O contrato está na reta final.",
  CONVERTED: "O negócio foi concluído.",
  CANCELLED: "Este negócio foi encerrado.",
};

function ownerStageHeadlines(isRent: boolean): Record<string, string> {
  return {
    NEGOTIATION: "Acompanhe aqui o andamento da negociação do seu imóvel.",
    PROPOSAL: "Você recebeu uma proposta para o seu imóvel.",
    ANALYSIS: "A documentação do negócio está em análise.",
    CONTRACT: "O contrato está na reta final.",
    CONVERTED: isRent ? "A locação do seu imóvel foi fechada." : "A venda do seu imóvel foi concluída.",
    CANCELLED: "Esta negociação foi encerrada.",
  };
}

function seekerPrimaryActions(isRent: boolean): Record<string, string> {
  return {
    CONTRACT: "Revisar contrato",
    ANALYSIS: "Enviar documentos",
    PROPOSAL: "Ver proposta",
    CONVERTED: isRent ? "Ver contrato de locação" : "Ver contrato",
    NEGOTIATION: "Acompanhar",
  };
}

function ownerPrimaryActions(isRent: boolean): Record<string, string> {
  return {
    CONTRACT: "Revisar contrato",
    ANALYSIS: "Enviar documentos",
    PROPOSAL: "Ver proposta recebida",
    CONVERTED: isRent ? "Ver contrato de locação" : "Ver contrato",
    NEGOTIATION: "Acompanhar negociação",
  };
}

/**
 * Dicionário de textos por persona. Toda tela do portal lê daqui: manter o
 * vocabulário num só lugar é o que impede o texto genérico de voltar.
 */
export function portalCopy(persona: PortalPersona): PortalCopy {
  switch (persona.actor) {
    case "buyer":
      return {
        propertiesNavLabel: "Imóveis",
        propertiesNavPath: "/portal/buscar",
        journeyEyebrow: "Sua jornada de compra",
        journeyBody:
          "Acompanhe as opções separadas para você, fale com seu corretor e escolha o melhor momento para visitar.",
        journeyPrimaryCta: { to: "/portal/buscar", label: "Explorar imóveis" },
        journeySecondaryCta: { to: "/portal/mensagens?contact=1", label: "Falar com meu corretor" },
        overviewEyebrow: "Sua compra",
        stageHeadline: SEEKER_STAGE_HEADLINES,
        primaryAction: seekerPrimaryActions(false),
        recommendationsTitle: "Selecionados para você",
        recommendationsIntro: "Seu corretor separou estas opções com base no que você procura.",
        quickActions: [
          { key: "search", to: "/portal/buscar", label: "Explorar imóveis" },
          { key: "properties", to: "/portal/imoveis", label: "Imóveis do meu atendimento" },
          { key: "finance", to: "/portal/finance", label: "Meus pagamentos" },
          { key: "contact", to: "/portal/mensagens?contact=1", label: "Falar com meu corretor" },
        ],
        propertyVisitCta: "Agendar visita",
        propertyVisitReschedule: "Alterar horário",
        propertyVisitOwnerHint: null,
        propertyContactCta: "Falar com meu corretor",
        propertyAddressHint: "Endereço completo liberado após agendar a visita.",
        visitsTitle: "Sua visita",
        visitsEmptyBody: "Nenhuma visita agendada. Fale com seu corretor para combinar a melhor data.",
        propertiesTitle: "Imóveis do meu atendimento",
        propertiesEmptyTitle: "Nenhum imóvel no seu atendimento",
        propertiesEmptyBody: "Quando seu corretor separar imóveis para você, eles aparecem aqui.",
        financeTitle: "Meus pagamentos",
        financeEntryLabel: "Meus pagamentos",
        repassesTitle: "Meus recebimentos",
        ticketsTitle: "Meus chamados",
        proposalCta: "Ver proposta",
        visits: SEEKER_VISITS,
      };

    case "tenant":
      return {
        propertiesNavLabel: "Imóveis",
        propertiesNavPath: "/portal/buscar",
        journeyEyebrow: "Sua jornada de locação",
        journeyBody:
          "Acompanhe as opções de imóveis para alugar, fale com seu corretor e combine a visita.",
        journeyPrimaryCta: { to: "/portal/buscar", label: "Explorar imóveis" },
        journeySecondaryCta: { to: "/portal/mensagens?contact=1", label: "Falar com meu corretor" },
        overviewEyebrow: "Sua locação",
        stageHeadline: SEEKER_STAGE_HEADLINES,
        primaryAction: seekerPrimaryActions(true),
        recommendationsTitle: "Selecionados para você",
        recommendationsIntro: "Seu corretor separou estas opções com base no que você procura.",
        quickActions: [
          { key: "search", to: "/portal/buscar", label: "Explorar imóveis" },
          { key: "properties", to: "/portal/imoveis", label: "Imóveis do meu atendimento" },
          { key: "finance", to: "/portal/finance", label: "Meus boletos" },
          { key: "tickets", to: "/portal/chamados", label: "Manutenção e chamados" },
          { key: "contact", to: "/portal/mensagens?contact=1", label: "Falar com meu corretor" },
        ],
        propertyVisitCta: "Agendar visita",
        propertyVisitReschedule: "Alterar horário",
        propertyVisitOwnerHint: null,
        propertyContactCta: "Falar com meu corretor",
        propertyAddressHint: "Endereço completo liberado após agendar a visita.",
        visitsTitle: "Sua visita",
        visitsEmptyBody: "Nenhuma visita agendada. Fale com seu corretor para combinar a melhor data.",
        propertiesTitle: "Imóveis do meu atendimento",
        propertiesEmptyTitle: "Nenhum imóvel no seu atendimento",
        propertiesEmptyBody: "Quando seu corretor separar imóveis para você, eles aparecem aqui.",
        financeTitle: "Meus boletos",
        financeEntryLabel: "Meus boletos",
        repassesTitle: "Meus repasses",
        ticketsTitle: "Manutenção e chamados",
        proposalCta: "Ver proposta",
        visits: SEEKER_VISITS,
      };

    case "seller":
    case "landlord": {
      const isRent = persona.actor === "landlord";
      const ownerVocabulary = isRent
        ? {
            journeyEyebrow: "Acompanhamento da locação",
            journeyBody:
              "A imobiliária conduz a locação do seu imóvel. Acompanhe aqui as visitas, as propostas e a documentação.",
            overviewEyebrow: "Locação do seu imóvel",
            propertiesEmptyTitle: "Nenhum imóvel para locação",
            propertiesEmptyBody: "Imóveis que você colocou para alugar aparecem aqui.",
            propertiesTitle: "Meus imóveis",
            financeTitle: "Meus repasses",
            financeEntryLabel: "Meus repasses",
            repassesTitle: "Meus repasses",
            ticketsTitle: "Manutenção e chamados",
          }
        : {
            journeyEyebrow: "Acompanhamento da venda",
            journeyBody:
              "A imobiliária conduz a venda do seu imóvel. Acompanhe aqui as visitas, as propostas e a documentação.",
            overviewEyebrow: "Venda do seu imóvel",
            propertiesEmptyTitle: "Nenhum imóvel para venda",
            propertiesEmptyBody: "Imóveis que você colocou à venda aparecem aqui.",
            propertiesTitle: "Meus imóveis",
            financeTitle: "Meus recebimentos",
            financeEntryLabel: "Meus recebimentos",
            repassesTitle: "Meus recebimentos",
            ticketsTitle: "Meus chamados",
          };

      return {
        propertiesNavLabel: "Meus imóveis",
        propertiesNavPath: "/portal/imoveis",
        journeyEyebrow: ownerVocabulary.journeyEyebrow,
        journeyBody: ownerVocabulary.journeyBody,
        // Sem "Explorar imóveis": quem anuncia não está procurando.
        journeyPrimaryCta: { to: "/portal/mensagens?contact=1", label: "Falar com meu corretor" },
        journeySecondaryCta: { to: "/portal/imoveis", label: "Ver meus imóveis" },
        overviewEyebrow: ownerVocabulary.overviewEyebrow,
        stageHeadline: ownerStageHeadlines(isRent),
        primaryAction: ownerPrimaryActions(isRent),
        recommendationsTitle: null,
        recommendationsIntro: "",
        quickActions: [
          { key: "properties", to: "/portal/imoveis", label: "Meus imóveis" },
          ...(isRent
            ? [
                { key: "rentals" as const, to: "/portal/locacao", label: "Minhas locações" },
                { key: "tickets" as const, to: "/portal/chamados", label: "Manutenção e chamados" },
              ]
            : []),
          { key: "finance", to: "/portal/finance", label: ownerVocabulary.financeEntryLabel },
          { key: "contact", to: "/portal/mensagens?contact=1", label: "Falar com meu corretor" },
        ],
        // As visitas ao imóvel do cliente são conduzidas pela imobiliária: quem
        // anuncia NÃO agenda visita ao próprio imóvel.
        propertyVisitCta: null,
        propertyVisitReschedule: "",
        propertyVisitOwnerHint:
          "As visitas ao seu imóvel são agendadas pela imobiliária com os interessados.",
        propertyContactCta: "Falar com meu corretor",
        propertyAddressHint: "Endereço do imóvel no cadastro.",
        visitsTitle: isRent ? "Visitas ao seu imóvel" : "Visitas ao seu imóvel",
        visitsEmptyBody:
          "A imobiliária agenda as visitas e o andamento aparece aqui.",
        propertiesTitle: ownerVocabulary.propertiesTitle,
        propertiesEmptyTitle: ownerVocabulary.propertiesEmptyTitle,
        propertiesEmptyBody: ownerVocabulary.propertiesEmptyBody,
        financeTitle: ownerVocabulary.financeTitle,
        financeEntryLabel: ownerVocabulary.financeEntryLabel,
        repassesTitle: ownerVocabulary.repassesTitle,
        ticketsTitle: ownerVocabulary.ticketsTitle,
        proposalCta: "Ver proposta recebida",
        visits: OWNER_VISITS,
      };
    }

    case "owner":
      // Proprietário sem tipo de negócio: não afirmamos venda nem locação.
      return {
        propertiesNavLabel: "Meus imóveis",
        propertiesNavPath: "/portal/imoveis",
        journeyEyebrow: "Acompanhamento do seu imóvel",
        journeyBody:
          "A imobiliária conduz o atendimento do seu imóvel. Acompanhe aqui as visitas, as propostas e a documentação.",
        journeyPrimaryCta: { to: "/portal/mensagens?contact=1", label: "Falar com meu corretor" },
        journeySecondaryCta: { to: "/portal/imoveis", label: "Ver meus imóveis" },
        overviewEyebrow: "Seu imóvel",
        stageHeadline: ownerStageHeadlines(false),
        primaryAction: ownerPrimaryActions(false),
        recommendationsTitle: null,
        recommendationsIntro: "",
        quickActions: [
          { key: "properties", to: "/portal/imoveis", label: "Meus imóveis" },
          { key: "finance", to: "/portal/finance", label: "Financeiro" },
          { key: "contact", to: "/portal/mensagens?contact=1", label: "Falar com meu corretor" },
        ],
        propertyVisitCta: null,
        propertyVisitReschedule: "",
        propertyVisitOwnerHint:
          "As visitas ao seu imóvel são agendadas pela imobiliária com os interessados.",
        propertyContactCta: "Falar com meu corretor",
        propertyAddressHint: "Endereço do imóvel no cadastro.",
        visitsTitle: "Visitas ao seu imóvel",
        visitsEmptyBody: "A imobiliária agenda as visitas e o andamento aparece aqui.",
        propertiesTitle: "Meus imóveis",
        propertiesEmptyTitle: "Nenhum imóvel cadastrado",
        propertiesEmptyBody: "Imóveis do seu atendimento aparecem aqui.",
        financeTitle: "Financeiro",
        financeEntryLabel: "Financeiro",
        repassesTitle: "Meus repasses",
        ticketsTitle: "Meus chamados",
        proposalCta: "Ver proposta recebida",
        visits: OWNER_VISITS,
      };

    default:
      // Fiador(a), testemunha ou papel não identificado: texto neutro e nenhuma
      // afirmação sobre procurar ou oferecer o imóvel.
      return {
        propertiesNavLabel: "Imóveis",
        propertiesNavPath: "/portal/imoveis",
        journeyEyebrow: "Seu atendimento",
        journeyBody:
          "Acompanhe aqui as etapas do seu atendimento com a imobiliária e fale com seu corretor quando precisar.",
        journeyPrimaryCta: { to: "/portal/mensagens?contact=1", label: "Falar com meu corretor" },
        journeySecondaryCta: { to: "/portal/imoveis", label: "Ver imóveis do atendimento" },
        overviewEyebrow: "Seu negócio",
        stageHeadline: NEUTRAL_STAGE_HEADLINES,
        primaryAction: ownerPrimaryActions(false),
        recommendationsTitle: "Sugestões do seu corretor",
        recommendationsIntro: "Imóveis relacionados ao seu atendimento.",
        quickActions: [
          { key: "properties", to: "/portal/imoveis", label: "Imóveis do atendimento" },
          { key: "finance", to: "/portal/finance", label: "Financeiro" },
          { key: "tickets", to: "/portal/chamados", label: "Chamados" },
          { key: "contact", to: "/portal/mensagens?contact=1", label: "Falar com meu corretor" },
        ],
        propertyVisitCta: "Solicitar visita",
        propertyVisitReschedule: "Alterar horário",
        propertyVisitOwnerHint: null,
        propertyContactCta: "Falar com meu corretor",
        propertyAddressHint: "Endereço completo liberado após agendar a visita.",
        visitsTitle: "Sua visita",
        visitsEmptyBody: "Nenhuma visita agendada. Fale com seu corretor para combinar a melhor data.",
        propertiesTitle: "Imóveis do atendimento",
        propertiesEmptyTitle: "Nenhum imóvel no atendimento",
        propertiesEmptyBody: "Os imóveis do seu atendimento aparecem aqui.",
        financeTitle: "Financeiro",
        financeEntryLabel: "Financeiro",
        repassesTitle: "Meus repasses",
        ticketsTitle: "Chamados",
        proposalCta: "Ver proposta",
        visits: NEUTRAL_VISITS,
      };
  }
}

/** Papel do cliente nas locações financeiras (`my_role` do toca-financial). */
export function rentalRoleLabel(myRole: string | null | undefined): string {
  switch (String(myRole ?? "").trim().toLowerCase()) {
    case "tenant":
      return "Você é o locatário";
    case "owner":
      return "Você é o locador";
    case "both":
      return "Você é locatário e locador";
    default:
      return "Locação vinculada à sua conta";
  }
}
