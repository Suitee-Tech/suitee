import portalApi from "./portal-api";

export interface PortalNotification {
  id: string;
  type: string;
  title: string;
  message: string;
  content?: string;
  link?: string;
  data?: Record<string, unknown>;
  priority?: "urgent" | "important" | "normal";
  is_read: boolean;
  created_at: string;
}

export type PortalNotificationLink = {
  pathname: string;
  search?: string;
};

const dealLink = (data: Record<string, unknown>): PortalNotificationLink | null => {
  const dealId = typeof data.deal_id === "string" ? data.deal_id : null;
  return dealId ? { pathname: `/portal/deals/${encodeURIComponent(dealId)}` } : null;
};

const PORTAL_PATHS_BY_TYPE: Record<string, (data: Record<string, unknown>) => PortalNotificationLink | null> = {
  chat_message: (data) => {
    const channelId = typeof data.channel_id === "string" ? data.channel_id : null;
    return channelId
      ? { pathname: "/portal/mensagens", search: `?channel=${encodeURIComponent(channelId)}` }
      : { pathname: "/portal/mensagens" };
  },
  deal_status_changed: dealLink,
  "proposal.sent": dealLink,
  "proposal.responded": dealLink,
  "signer.invited": dealLink,
  "checklist.solicited": dealLink,
  "appointment.updated": (data) => dealLink(data) ?? { pathname: "/portal" },
  "appointment.cancelled": (data) => dealLink(data) ?? { pathname: "/portal" },
  "invoice.paid": (data) => {
    const chargeId = typeof data.charge_id === "string" ? data.charge_id : null;
    return chargeId
      ? { pathname: "/portal/financas", search: `?charge=${encodeURIComponent(chargeId)}` }
      : { pathname: "/portal/financas" };
  },
  "invoice.due": (data) => {
    const chargeId = typeof data.charge_id === "string" ? data.charge_id : null;
    return chargeId
      ? { pathname: "/portal/financas", search: `?charge=${encodeURIComponent(chargeId)}` }
      : { pathname: "/portal/financas" };
  },
  "ticket.updated": (data) => {
    const ticketId = typeof data.ticket_id === "string" ? data.ticket_id : null;
    return ticketId
      ? { pathname: "/portal/chamados", search: `?ticket=${encodeURIComponent(ticketId)}` }
      : { pathname: "/portal/chamados" };
  },
};

export const getPortalNotificationLink = (n: PortalNotification): PortalNotificationLink | null => {
  const link = (n.link || "").trim();
  if (link) {
    if (/^https?:\/\//i.test(link)) return null;
    const normalized = link.startsWith("/") ? link : `/${link}`;
    if (normalized.startsWith("/portal/")) {
      const [pathname, search] = normalized.split("?");
      return { pathname, search: search ? `?${search}` : "" };
    }
  }

  const resolver = PORTAL_PATHS_BY_TYPE[n.type];
  if (resolver) return resolver(n.data || {});
  return null;
};

export interface PortalNotificationListResponse {
  items: PortalNotification[];
  total: number;
}

export const portalNotificationService = {
  getList: async (limit: number = 20, offset: number = 0): Promise<PortalNotificationListResponse> => {
    const response = await portalApi.get("/notifications/inapp", { params: { limit, offset } });
    const data = response.data;
    if (data && typeof data === "object" && "data" in data) {
      const inner = (data as any).data;
      const itemsRaw = Array.isArray(inner) ? inner : Array.isArray(inner?.items) ? inner.items : [];
      return {
        items: itemsRaw.map((n: any) => ({
          ...n,
          message: n?.message ?? n?.content ?? "",
        })),
        total:
          typeof (data as any)?.meta?.total === "number"
            ? (data as any).meta.total
            : typeof inner?.total === "number"
              ? inner.total
              : 0,
      };
    }
    return {
      items: (Array.isArray((data as any)?.items) ? (data as any).items : []).map((n: any) => ({
        ...n,
        message: n?.message ?? n?.content ?? "",
      })),
      total: typeof (data as any)?.total === "number" ? (data as any).total : 0,
    };
  },

  getUnreadCount: async (): Promise<number> => {
    const response = await portalApi.get("/notifications/inapp/unread-count");
    const data = response.data;
    if (data && typeof data === "object" && "data" in data) {
      const inner = (data as any).data;
      return typeof inner?.count === "number" ? inner.count : 0;
    }
    return typeof (data as any)?.count === "number" ? (data as any).count : 0;
  },

  markAsRead: async (id: string): Promise<void> => {
    await portalApi.put(`/notifications/inapp/${id}/read`);
  },

  markAllAsRead: async (): Promise<void> => {
    await portalApi.put("/notifications/inapp/read-all");
  },
};
