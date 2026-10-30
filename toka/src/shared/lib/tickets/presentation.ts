import type { VariantProps } from "class-variance-authority";

import type { badgeVariants } from "../../ui/badge";

type BadgeVariant = VariantProps<typeof badgeVariants>["variant"];

export const ticketStatusLabel = (status?: string) => {
  const s = (status || "").toLowerCase();
  if (s === "open") return "Aberto";
  if (s === "assigned") return "Atribuído";
  if (s === "in_progress") return "Em andamento";
  if (s === "awaiting_client") return "Aguardando cliente";
  if (s === "resolved") return "Resolvido";
  if (s === "closed") return "Fechado";
  return status || "-";
};

export const ticketPriorityLabel = (priority?: string) => {
  const p = (priority || "").toLowerCase();
  if (p === "urgent") return "Urgente";
  if (p === "high") return "Alta";
  if (p === "medium") return "Média";
  if (p === "low") return "Baixa";
  return priority || "-";
};

export const ticketTypeLabel = (type?: string) => {
  const t = (type || "").toLowerCase();
  if (t === "support") return "Suporte";
  if (t === "maintenance") return "Manutenção";
  if (t === "billing") return "Financeiro";
  if (t === "documentation") return "Documentação";
  if (t === "general") return "Geral";
  return type || "-";
};

export const ticketCategoryLabel = (category?: string) => {
  const c = (category || "").toLowerCase();
  if (c === "general") return "Geral";
  if (c === "documentation") return "Documentação";
  if (c === "billing") return "Cobrança";
  if (c === "technical") return "Técnico";
  return category || "-";
};

export const badgeVariantForTicketPriority = (priority?: string): BadgeVariant => {
  const p = (priority || "").toLowerCase();
  if (p === "urgent") return "destructive";
  if (p === "high") return "default";
  if (p === "medium") return "secondary";
  if (p === "low") return "outline";
  return "outline";
};

export const badgeVariantForTicketStatus = (status?: string): BadgeVariant => {
  const s = (status || "").toLowerCase();
  if (s === "open") return "secondary";
  if (s === "assigned") return "secondary";
  if (s === "in_progress") return "default";
  if (s === "awaiting_client") return "secondary";
  if (s === "resolved") return "default";
  if (s === "closed") return "outline";
  if (s === "cancelled") return "destructive";
  return "outline";
};

export const ticketCommentAuthorLabel = (
  comment: { author_id: string; author_name?: string; author_type: "client" | "operator" },
  options?: { currentUserId?: string },
) => {
  if (comment.author_type === "client") return comment.author_name ? `Cliente · ${comment.author_name}` : "Cliente";
  if (options?.currentUserId && comment.author_id === options.currentUserId) return "Você";
  return comment.author_name ? `Operador · ${comment.author_name}` : "Operador";
};
