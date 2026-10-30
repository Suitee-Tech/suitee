import portalApi from "./portal-api";
import type { EmailPref } from "../../../shared/services/email-prefs.service";

/**
 * Preferências de e-mail transacional no portal do cliente (beta F4).
 * Mesmo endpoint do painel, mas com o axios isolado do portal.
 */
interface EmailPrefsResponse {
  success: boolean;
  message: string;
  data: EmailPref[];
}

export const portalEmailPrefsService = {
  async list(): Promise<EmailPref[]> {
    const res = await portalApi.get<EmailPrefsResponse>("/notifications/email-prefs");
    return res.data?.data ?? [];
  },

  async update(categories: Array<{ category: string; enabled: boolean }>): Promise<EmailPref[]> {
    const res = await portalApi.put<EmailPrefsResponse>("/notifications/email-prefs", {
      prefs: categories,
    });
    return res.data?.data ?? [];
  },
};
