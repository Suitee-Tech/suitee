import api from "./api";

export type InAppNotificationType =
  | "chat_message"
  | "lead_assigned"
  | "ticket_assigned"
  | "ticket_comment_added"
  | "ticket_status_changed"
  | "transfer_processed"
  | "system";

export interface InAppNotification {
  id: string;
  tenant_id: string;
  user_id: string;
  type: InAppNotificationType;
  priority: "urgent" | "important" | "normal";
  title: string;
  content: string;
  link?: string;
  data?: any;
  is_read: boolean;
  read_at?: string;
  created_at: string;
}

export const notificationService = {
  async getList(limit = 20, offset = 0) {
    const res = await api.get("/notifications/inapp", {
      params: { limit, offset },
    });
    return res.data;
  },

  async getUnreadCount() {
    const res = await api.get("/notifications/inapp/unread-count");
    return res.data;
  },

  async markAsRead(id: string) {
    const res = await api.put(`/notifications/inapp/${id}/read`);
    return res.data;
  },

  async markAllAsRead() {
    const res = await api.put("/notifications/inapp/read-all");
    return res.data;
  },
};
