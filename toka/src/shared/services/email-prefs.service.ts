import api from "./api";

/**
 * Preferências de e-mail transacional do usuário (beta F4).
 * Backend: toca-comm `GET/PUT /notifications/email-prefs`.
 */
export interface EmailPref {
  category: string;
  label: string;
  enabled: boolean;
  /** Travado para a persona (não pode ser desligado — o backend ignora). */
  locked: boolean;
}

interface EmailPrefsResponse {
  success: boolean;
  message: string;
  data: EmailPref[];
}

export const emailPrefsService = {
  async list(): Promise<EmailPref[]> {
    const res = await api.get<EmailPrefsResponse>("/notifications/email-prefs");
    return res.data?.data ?? [];
  },

  async update(categories: Array<{ category: string; enabled: boolean }>): Promise<EmailPref[]> {
    const res = await api.put<EmailPrefsResponse>("/notifications/email-prefs", {
      prefs: categories,
    });
    return res.data?.data ?? [];
  },
};
