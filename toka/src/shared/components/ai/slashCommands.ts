export type SlashCommandKind = "prompt" | "action";

export interface SlashCommand {
  command: string;
  label: string;
  description: string;
  kind: SlashCommandKind;
  expand?: string;
  action?: "clear";
}

export const SLASH_COMMANDS: SlashCommand[] = [
  {
    command: "leads",
    label: "/leads",
    description: "Liste os leads recentes",
    kind: "prompt",
    expand: "Liste os 10 leads mais recentes com nome, telefone, corretor e status.",
  },
  {
    command: "leads novos",
    label: "/leads novos",
    description: "Leads em status novo",
    kind: "prompt",
    expand: "Liste os 10 leads mais recentes com status novo, mostrando nome, telefone e corretor responsável.",
  },
  {
    command: "leads sem corretor",
    label: "/leads sem corretor",
    description: "Leads sem responsável",
    kind: "prompt",
    expand: "Liste os leads que ainda não têm corretor designado, ordenados pelos mais recentes.",
  },
  {
    command: "imoveis",
    label: "/imoveis",
    description: "Inventário de imóveis",
    kind: "prompt",
    expand: "Mostre um resumo do meu inventário de imóveis: total por status (disponível, reservado, vendido, alugado).",
  },
  {
    command: "imoveis disponiveis",
    label: "/imoveis disponíveis",
    description: "Imóveis para venda",
    kind: "prompt",
    expand: "Liste os imóveis disponíveis para venda, mostrando título, valor, bairro e quartos.",
  },
  {
    command: "imoveis aluguel",
    label: "/imoveis aluguel",
    description: "Imóveis para locação",
    kind: "prompt",
    expand: "Liste os imóveis disponíveis para aluguel, mostrando título, valor mensal, bairro e quartos.",
  },
  {
    command: "agenda",
    label: "/agenda",
    description: "Compromissos de hoje e amanhã",
    kind: "prompt",
    expand: "Mostre meus compromissos de hoje e amanhã com horário, tipo e contraparte.",
  },
  {
    command: "agenda hoje",
    label: "/agenda hoje",
    description: "Compromissos de hoje",
    kind: "prompt",
    expand: "Mostre meus compromissos de hoje com horário, tipo e contraparte.",
  },
  {
    command: "pipeline",
    label: "/pipeline",
    description: "Visão do pipeline de deals",
    kind: "prompt",
    expand: "Mostre o pipeline de deals com total e valor por etapa.",
  },
  {
    command: "financeiro",
    label: "/financeiro",
    description: "Resumo financeiro do mês",
    kind: "prompt",
    expand: "Mostre o resumo financeiro do mês corrente: receitas, despesas, inadimplência e saldo.",
  },
  {
    command: "inadimplencia",
    label: "/inadimplencia",
    description: "Cobranças em atraso",
    kind: "prompt",
    expand: "Liste as cobranças em atraso, ordenadas por dias de atraso, mostrando contraparte, valor e vencimento.",
  },
  {
    command: "criar lead",
    label: "/criar lead",
    description: "Iniciar criação de lead",
    kind: "prompt",
    expand: "Quero criar um novo lead. Pergunte os dados necessários (nome, telefone, e-mail, origem) e prepare a confirmação.",
  },
  {
    command: "criar agendamento",
    label: "/criar agendamento",
    description: "Agendar visita",
    kind: "prompt",
    expand: "Quero agendar uma visita. Pergunte os dados necessários (lead/cliente, imóvel, data e horário) e prepare a confirmação.",
  },
  {
    command: "criar deal",
    label: "/criar deal",
    description: "Iniciar negociação",
    kind: "prompt",
    expand: "Quero criar um novo deal. Pergunte os dados necessários (lead, imóvel, valor, etapa) e prepare a confirmação.",
  },
  {
    command: "help",
    label: "/help",
    description: "O que posso fazer aqui",
    kind: "prompt",
    expand: "Liste de forma resumida o que posso fazer com você (consultas, criações, ações), agrupado por domínio.",
  },
  {
    command: "ajuda",
    label: "/ajuda",
    description: "Como usar a plataforma (Central de Ajuda)",
    kind: "prompt",
    expand: "Explique como usar a plataforma: consulte a Central de Ajuda (search_help / get_help_topic) e responda com o passo a passo.",
  },
  { command: "clear", label: "/clear", description: "Iniciar nova conversa", kind: "action", action: "clear" },
];

export function filterSlashCommands(query: string): SlashCommand[] {
  const q = query.trim().toLowerCase();
  if (!q) return SLASH_COMMANDS;
  return SLASH_COMMANDS.filter(
    (c) =>
      c.command.toLowerCase().includes(q) ||
      c.label.toLowerCase().includes(q) ||
      c.description.toLowerCase().includes(q),
  );
}

